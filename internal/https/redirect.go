package https

import (
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"

	"github.com/moodiness/polyfin/internal/forwarded"
)

// challengePrefix is the path of the ACME server's HTTP-01 checks.
const challengePrefix = "/.well-known/acme-challenge/"

// Handler has next serve every request but, over HTTP, the ACME server's
// HTTP-01 checks of the tokens under way, which it answers, and, while
// Redirect is on and HTTPS is served, the requests of clients on the
// internet, which get a 308 to the same address on https://, on the port
// the internet reaches HTTPS at (see publicPort). The local network
// (private IPv4 ranges, IPv6 unique local, link-local and loopback
// addresses, as discovery reads them), the ACME server's checks, and
// requests from a known proxy that say they came over HTTPS
// (X-Forwarded-Proto) are never redirected. It needs the client
// forwarded.Proxies resolves.
func (s *Service) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS != nil {
			next.ServeHTTP(w, r)
			return
		}
		if token, ok := strings.CutPrefix(r.URL.Path, challengePrefix); ok {
			if response, ok := s.answerChallenge(token); ok && (r.Method == http.MethodGet || r.Method == http.MethodHead) {
				w.Header().Set("Content-Type", "text/plain")
				_, _ = w.Write([]byte(response))
				return
			}
			next.ServeHTTP(w, r)
			return
		}
		if target, ok := s.redirect(r); ok {
			http.Redirect(w, r, target, http.StatusPermanentRedirect)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// redirect returns where a request over HTTP is sent, and whether it is.
func (s *Service) redirect(r *http.Request) (string, bool) {
	config := s.serving.Load()
	if config == nil || !config.Redirect {
		return "", false
	}
	if forwarded.ViaProxy(r) && strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https") {
		return "", false
	}
	client, err := netip.ParseAddr(forwarded.Client(r))
	if err != nil || onLocalNetwork(client.Unmap()) {
		return "", false
	}
	host := r.Host
	if name, _, err := net.SplitHostPort(host); err == nil {
		host = name
	}
	host = strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
	if host == "" {
		return "", false
	}
	authority := host
	if strings.Contains(host, ":") {
		authority = "[" + host + "]"
	}
	if port := publicPort(*config); port != 443 {
		authority += ":" + strconv.Itoa(port)
	}
	path := r.RequestURI
	if !strings.HasPrefix(path, "/") {
		path = r.URL.RequestURI()
	}
	return "https://" + authority + path, true
}

// publicPort returns the port the internet reaches HTTPS at: the port of
// config's PublicAddress when it is an https:// address, 443 when it names
// none, else the HTTPS port.
func publicPort(config Config) int {
	address, err := url.Parse(config.PublicAddress)
	if err != nil || !strings.EqualFold(address.Scheme, "https") || address.Hostname() == "" {
		return config.Port
	}
	if address.Port() == "" {
		return 443
	}
	if port, err := strconv.Atoi(address.Port()); err == nil && port >= 1 && port <= 65535 {
		return port
	}
	return config.Port
}

// onLocalNetwork reports whether address is on the local network, as
// discovery reads it: a private IPv4 range, an IPv6 unique local address,
// a link-local one or a loopback one.
func onLocalNetwork(address netip.Addr) bool {
	return address.IsPrivate() || address.IsLinkLocalUnicast() || address.IsLoopback()
}
