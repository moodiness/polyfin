package https

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/acme"
)

// testCA is a minimal ACME server (RFC 8555) for tests: it registers
// accounts, takes orders for DNS names, checks their HTTP-01 and
// TLS-ALPN-01 challenges for real, at the addresses resolve gives, and
// issues certificates signed by its own root for lifetime. Like pebble,
// it answers a finalization with a processing order and no Location
// header, which RFC 8555 does not require there, so a client must poll
// the order's own URL. It does not check the requests' signatures:
// golang.org/x/crypto/acme, which signs them, has its own tests.
type testCA struct {
	t        *testing.T
	server   *httptest.Server
	root     *x509.Certificate
	rootKey  *ecdsa.PrivateKey
	lifetime time.Duration
	now      func() time.Time
	// resolve returns the address, host:port, a challenge of type for host
	// is checked at.
	resolve func(host, challengeType string) string

	mu       sync.Mutex
	nonce    int
	accounts map[string]string // thumbprint by account URL
	orders   []*caOrder
	authzs   []*caAuthz
	issued   int
}

type caOrder struct {
	account     string
	hosts       []string
	authzs      []int
	status      string
	certificate []byte
}

type caAuthz struct {
	account string
	host    string
	token   string
	status  string
	// failure is why its check failed, empty for none.
	failure string
}

// newTestCA starts a test ACME server, stopped at the end of the test.
func newTestCA(t *testing.T, now func() time.Time, lifetime time.Duration, resolve func(host, challengeType string) string) *testCA {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Test ACME root"},
		NotBefore: now().Add(-time.Hour), NotAfter: now().Add(10 * 365 * 24 * time.Hour), IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageCertSign}
	der, err := x509.CreateCertificate(rand.Reader, template, template, key.Public(), key)
	if err != nil {
		t.Fatal(err)
	}
	root, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	ca := &testCA{t: t, root: root, rootKey: key, lifetime: lifetime, now: now, resolve: resolve, accounts: map[string]string{}}
	ca.server = httptest.NewTLSServer(http.HandlerFunc(ca.serve))
	t.Cleanup(ca.server.Close)
	return ca
}

// directory is the ACME directory's address.
func (ca *testCA) directory() string { return ca.server.URL + "/dir" }

// issuedCount is how many certificates were issued.
func (ca *testCA) issuedCount() int {
	ca.mu.Lock()
	defer ca.mu.Unlock()
	return ca.issued
}

// pool trusts the test root.
func (ca *testCA) pool() *x509.CertPool {
	pool := x509.NewCertPool()
	pool.AddCert(ca.root)
	return pool
}

func (ca *testCA) serve(w http.ResponseWriter, r *http.Request) {
	ca.mu.Lock()
	ca.nonce++
	w.Header().Set("Replay-Nonce", "nonce-"+strconv.Itoa(ca.nonce))
	ca.mu.Unlock()
	base := ca.server.URL
	switch {
	case r.URL.Path == "/dir":
		writeACME(w, http.StatusOK, map[string]any{"newNonce": base + "/nonce", "newAccount": base + "/new-account",
			"newOrder": base + "/new-order", "revokeCert": base + "/revoke", "keyChange": base + "/key-change",
			"meta": map[string]any{"termsOfService": base + "/terms"}})
		return
	case r.URL.Path == "/nonce":
		w.WriteHeader(http.StatusOK)
		return
	case r.Method != http.MethodPost:
		http.NotFound(w, r)
		return
	}
	header, payload, err := readJWS(r)
	if err != nil {
		ca.problem(w, http.StatusBadRequest, "malformed", err.Error())
		return
	}
	ca.mu.Lock()
	defer ca.mu.Unlock()
	if r.URL.Path == "/new-account" {
		thumbprint, err := thumbprintOf(header.JWK)
		if err != nil {
			ca.problem(w, http.StatusBadRequest, "badPublicKey", err.Error())
			return
		}
		for url, known := range ca.accounts {
			if known == thumbprint {
				w.Header().Set("Location", url)
				writeACME(w, http.StatusOK, map[string]any{"status": "valid"})
				return
			}
		}
		url := base + "/account/" + strconv.Itoa(len(ca.accounts))
		ca.accounts[url] = thumbprint
		w.Header().Set("Location", url)
		writeACME(w, http.StatusCreated, map[string]any{"status": "valid"})
		return
	}
	account := header.KID
	if _, ok := ca.accounts[account]; !ok {
		ca.problem(w, http.StatusUnauthorized, "accountDoesNotExist", "unknown account")
		return
	}
	kind, number, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/"), "/")
	id, _ := strconv.Atoi(number)
	switch kind {
	case "account":
		writeACME(w, http.StatusOK, map[string]any{"status": "valid"})
	case "new-order":
		var request struct {
			Identifiers []struct{ Type, Value string }
		}
		if err := json.Unmarshal(payload, &request); err != nil || len(request.Identifiers) == 0 {
			ca.problem(w, http.StatusBadRequest, "malformed", "no identifiers")
			return
		}
		order := &caOrder{account: account, status: acme.StatusPending}
		for _, identifier := range request.Identifiers {
			order.hosts = append(order.hosts, identifier.Value)
			ca.authzs = append(ca.authzs, &caAuthz{account: account, host: identifier.Value, token: rand.Text(), status: acme.StatusPending})
			order.authzs = append(order.authzs, len(ca.authzs)-1)
		}
		ca.orders = append(ca.orders, order)
		ca.writeOrder(w, http.StatusCreated, len(ca.orders)-1, true)
	case "order":
		if id >= len(ca.orders) {
			http.NotFound(w, r)
			return
		}
		// An order processing since its finalization is issued when read.
		if ca.orders[id].status == acme.StatusProcessing {
			ca.orders[id].status = acme.StatusValid
		}
		ca.writeOrder(w, http.StatusOK, id, true)
	case "authz":
		if id >= len(ca.authzs) {
			http.NotFound(w, r)
			return
		}
		a := ca.authzs[id]
		challenges := []map[string]any{}
		for i, challengeType := range []string{ChallengeHTTP01, ChallengeTLSALPN01} {
			challenge := map[string]any{"type": challengeType, "url": base + "/challenge/" + strconv.Itoa(id*2+i), "token": a.token,
				"status": a.status}
			if a.failure != "" {
				challenge["error"] = map[string]any{"type": "urn:ietf:params:acme:error:connection", "detail": a.failure}
			}
			challenges = append(challenges, challenge)
		}
		writeACME(w, http.StatusOK, map[string]any{"status": a.status, "identifier": map[string]string{"type": "dns", "value": a.host},
			"challenges": challenges})
	case "challenge":
		if id/2 >= len(ca.authzs) {
			http.NotFound(w, r)
			return
		}
		a := ca.authzs[id/2]
		challengeType := []string{ChallengeHTTP01, ChallengeTLSALPN01}[id%2]
		if a.status == acme.StatusPending {
			// Checked at once, before answering: the authorization is
			// valid or invalid when the client asks for it again.
			keyAuthorization := a.token + "." + ca.accounts[a.account]
			address := ca.resolve(a.host, challengeType)
			ca.mu.Unlock()
			err := check(challengeType, address, a.host, a.token, keyAuthorization)
			ca.mu.Lock()
			a.status = acme.StatusValid
			if err != nil {
				ca.t.Logf("test ACME server: %s for %s failed: %v", challengeType, a.host, err)
				a.status, a.failure = acme.StatusInvalid, err.Error()
			}
		}
		writeACME(w, http.StatusOK, map[string]any{"type": challengeType, "url": base + r.URL.Path, "token": a.token, "status": a.status})
	case "finalize":
		if id >= len(ca.orders) {
			http.NotFound(w, r)
			return
		}
		ca.finalize(w, id, payload)
	case "certificate":
		if id >= len(ca.orders) || ca.orders[id].certificate == nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/pem-certificate-chain")
		_, _ = w.Write(ca.orders[id].certificate)
	default:
		http.NotFound(w, r)
	}
}

// writeOrder answers order id, its status following its authorizations,
// with the order's URL in a Location header when location is true.
func (ca *testCA) writeOrder(w http.ResponseWriter, status, id int, location bool) {
	base := ca.server.URL
	order := ca.orders[id]
	if order.status == acme.StatusPending {
		ready := true
		for _, a := range order.authzs {
			switch ca.authzs[a].status {
			case acme.StatusInvalid:
				order.status = acme.StatusInvalid
			case acme.StatusPending:
				ready = false
			}
		}
		if ready && order.status == acme.StatusPending {
			order.status = acme.StatusReady
		}
	}
	identifiers, authorizations := []map[string]string{}, []string{}
	for i, host := range order.hosts {
		identifiers = append(identifiers, map[string]string{"type": "dns", "value": host})
		authorizations = append(authorizations, base+"/authz/"+strconv.Itoa(order.authzs[i]))
	}
	body := map[string]any{"status": order.status, "identifiers": identifiers, "authorizations": authorizations,
		"finalize": base + "/finalize/" + strconv.Itoa(id)}
	if order.status == acme.StatusValid {
		body["certificate"] = base + "/certificate/" + strconv.Itoa(id)
	}
	if location {
		w.Header().Set("Location", base+"/order/"+strconv.Itoa(id))
	}
	writeACME(w, status, body)
}

// finalize issues the certificate of a ready order for the CSR of payload.
func (ca *testCA) finalize(w http.ResponseWriter, id int, payload []byte) {
	order := ca.orders[id]
	ca.writeOrderStatus(id)
	if order.status != acme.StatusReady {
		ca.problem(w, http.StatusForbidden, "orderNotReady", "the order is "+order.status)
		return
	}
	var request struct{ CSR string }
	if err := json.Unmarshal(payload, &request); err != nil {
		ca.problem(w, http.StatusBadRequest, "malformed", err.Error())
		return
	}
	der, err := base64.RawURLEncoding.DecodeString(request.CSR)
	if err != nil {
		ca.problem(w, http.StatusBadRequest, "badCSR", err.Error())
		return
	}
	csr, err := x509.ParseCertificateRequest(der)
	if err != nil || csr.CheckSignature() != nil || !slices.Equal(slices.Sorted(slices.Values(csr.DNSNames)), slices.Sorted(slices.Values(order.hosts))) {
		ca.problem(w, http.StatusBadRequest, "badCSR", "the CSR does not name the order's hosts")
		return
	}
	now := ca.now()
	template := &x509.Certificate{SerialNumber: big.NewInt(int64(1000 + ca.issued)), Subject: pkix.Name{CommonName: order.hosts[0]},
		DNSNames: order.hosts, NotBefore: now, NotAfter: now.Add(ca.lifetime), KeyUsage: x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	leaf, err := x509.CreateCertificate(rand.Reader, template, ca.root, csr.PublicKey, ca.rootKey)
	if err != nil {
		ca.problem(w, http.StatusInternalServerError, "serverInternal", err.Error())
		return
	}
	ca.issued++
	order.certificate = append(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leaf}),
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.root.Raw})...)
	order.status = acme.StatusProcessing
	ca.writeOrder(w, http.StatusOK, id, false)
}

// writeOrderStatus brings an order's status up to its authorizations'.
func (ca *testCA) writeOrderStatus(id int) {
	ca.writeOrder(httptest.NewRecorder(), http.StatusOK, id, false)
}

func (ca *testCA) problem(w http.ResponseWriter, status int, kind, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"type": "urn:ietf:params:acme:error:" + kind, "detail": detail})
}

func writeACME(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// jwsHeader is what the test server reads of a request's protected header.
type jwsHeader struct {
	JWK map[string]string `json:"jwk"`
	KID string            `json:"kid"`
}

// readJWS reads a request's protected header and payload.
func readJWS(r *http.Request) (jwsHeader, []byte, error) {
	var body struct{ Protected, Payload string }
	data, err := io.ReadAll(r.Body)
	if err != nil {
		return jwsHeader{}, nil, err
	}
	if err := json.Unmarshal(data, &body); err != nil {
		return jwsHeader{}, nil, err
	}
	protected, err := base64.RawURLEncoding.DecodeString(body.Protected)
	if err != nil {
		return jwsHeader{}, nil, err
	}
	var header jwsHeader
	if err := json.Unmarshal(protected, &header); err != nil {
		return jwsHeader{}, nil, err
	}
	payload, err := base64.RawURLEncoding.DecodeString(body.Payload)
	return header, payload, err
}

// thumbprintOf is the RFC 7638 thumbprint of an EC JWK.
func thumbprintOf(jwk map[string]string) (string, error) {
	if jwk["kty"] != "EC" {
		return "", fmt.Errorf("unsupported key type %q", jwk["kty"])
	}
	canonical := fmt.Sprintf(`{"crv":%q,"kty":"EC","x":%q,"y":%q}`, jwk["crv"], jwk["x"], jwk["y"])
	sum := sha256.Sum256([]byte(canonical))
	return base64.RawURLEncoding.EncodeToString(sum[:]), nil
}

// idPeACMEIdentifier is the extension of TLS-ALPN-01 certificates (RFC
// 8737).
var idPeACMEIdentifier = asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 1, 31}

// check checks a challenge for host at address, as an ACME server does.
func check(challengeType, address, host, token, keyAuthorization string) error {
	switch challengeType {
	case ChallengeHTTP01:
		request, err := http.NewRequest(http.MethodGet, "http://"+address+challengePrefix+token, nil)
		if err != nil {
			return err
		}
		request.Host = host
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			return err
		}
		defer response.Body.Close()
		body, _ := io.ReadAll(response.Body)
		if response.StatusCode != http.StatusOK || strings.TrimSpace(string(body)) != keyAuthorization {
			return fmt.Errorf("answered %d %q", response.StatusCode, body)
		}
		return nil
	case ChallengeTLSALPN01:
		conn, err := tls.Dial("tcp", address, &tls.Config{ServerName: host, NextProtos: []string{acme.ALPNProto}, InsecureSkipVerify: true})
		if err != nil {
			return err
		}
		defer conn.Close()
		state := conn.ConnectionState()
		if state.NegotiatedProtocol != acme.ALPNProto || len(state.PeerCertificates) == 0 {
			return fmt.Errorf("negotiated %q", state.NegotiatedProtocol)
		}
		certificate := state.PeerCertificates[0]
		if !slices.Equal(certificate.DNSNames, []string{host}) {
			return fmt.Errorf("a certificate for %v", certificate.DNSNames)
		}
		want := sha256.Sum256([]byte(keyAuthorization))
		for _, extension := range certificate.Extensions {
			if !extension.Id.Equal(idPeACMEIdentifier) {
				continue
			}
			var got []byte
			if _, err := asn1.Unmarshal(extension.Value, &got); err != nil || !extension.Critical || string(got) != string(want[:]) {
				return fmt.Errorf("a wrong acmeIdentifier")
			}
			return nil
		}
		return fmt.Errorf("no acmeIdentifier")
	}
	return fmt.Errorf("unknown challenge %s", challengeType)
}
