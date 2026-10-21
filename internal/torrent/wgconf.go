package torrent

import (
	"encoding/base64"
	"encoding/hex"
	"net"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

// A WireGuard configuration is the .conf providers hand out, as wg-quick
// reads it: an [Interface] section, Polyfin's side of the tunnel, and one
// [Peer] section, the provider's server. Comments start with #, and keys
// and section names are read whatever their case.

// MaxWireGuardConfigBytes bounds a WireGuard configuration: a provider's
// takes a few hundred bytes.
const MaxWireGuardConfigBytes = 8 << 10

// DefaultWireGuardMTU is the tunnel's MTU when the configuration sets none,
// as wg-quick's.
const DefaultWireGuardMTU = 1420

// WireGuardConfig is a WireGuard configuration that was read. The private
// and preshared keys are unexported, and the type has no String method:
// printing it never shows them.
type WireGuardConfig struct {
	privateKey [32]byte
	// Addresses are the tunnel's addresses in Polyfin, one IPv4 address at
	// least; DNS the name servers tracker names are looked up by, over
	// the tunnel, none for no lookup.
	Addresses []netip.Prefix
	DNS       []netip.Addr
	// MTU is 0 for DefaultWireGuardMTU, ListenPort 0 for a port the
	// system chooses.
	MTU, ListenPort int

	publicKey, presharedKey [32]byte
	// Endpoint is the server, host:port, the host a name or an address.
	Endpoint string
	// AllowedIPs are what the server is sent, 0.0.0.0/0 among them.
	AllowedIPs []netip.Prefix
	// PersistentKeepalive is how often, in seconds, the tunnel is kept
	// alive while idle, 0 for never.
	PersistentKeepalive int
}

// The reasons of a WireGuardConfigError.
const (
	// WireGuardTooLong: the configuration is longer than
	// MaxWireGuardConfigBytes, or not UTF-8 text.
	WireGuardTooLong = "too_long"
	// WireGuardSyntax: a line is neither a section, Key = Value, nor a
	// comment.
	WireGuardSyntax = "syntax"
	// WireGuardOutside: a key comes before any section.
	WireGuardOutside = "outside"
	// WireGuardUnknown: a section or a key WireGuard does not have.
	WireGuardUnknown = "unknown"
	// WireGuardRepeated: a section, or a key that is not a list, comes
	// twice; a second [Peer] among them, Polyfin using one server.
	WireGuardRepeated = "repeated"
	// WireGuardMissing: a section or a key needed is not there.
	WireGuardMissing = "missing"
	// WireGuardInvalid: a value is not what its key takes.
	WireGuardInvalid = "invalid"
	// WireGuardNoIPv4: Address holds no IPv4 address, which UDP trackers
	// and the DHT need.
	WireGuardNoIPv4 = "no_ipv4"
	// WireGuardNotAll: AllowedIPs leaves out 0.0.0.0/0, while the peers
	// of a torrent are anywhere.
	WireGuardNotAll = "not_all"
)

// WireGuardConfigError tells why a WireGuard configuration is refused:
// Reason, one of the constants above, about Key (such as "PrivateKey", or
// "Interface" and "Peer" for a section) at Line, counted from 1, 0 for the
// configuration as a whole. It never holds a value of the configuration.
type WireGuardConfigError struct {
	Line   int
	Key    string
	Reason string
}

func (e *WireGuardConfigError) Error() string {
	var text string
	switch e.Reason {
	case WireGuardTooLong:
		text = "it is not text of at most " + strconv.Itoa(MaxWireGuardConfigBytes) + " bytes"
	case WireGuardSyntax:
		text = "the line is not a [section], a Key = Value, or a # comment"
	case WireGuardOutside:
		text = e.Key + " comes before [Interface] or [Peer]"
	case WireGuardUnknown:
		text = e.Key + " is not part of a WireGuard configuration"
	case WireGuardRepeated:
		if e.Key == "Peer" {
			text = "only one [Peer] is supported"
		} else {
			text = e.Key + " is given twice"
		}
	case WireGuardMissing:
		text = e.Key + " is missing"
	case WireGuardNoIPv4:
		text = "Address needs an IPv4 address, which UDP trackers and the DHT use"
	case WireGuardNotAll:
		text = "AllowedIPs needs 0.0.0.0/0: the peers of a torrent are anywhere"
	default:
		text = e.Key + " must be " + wireGuardExpected[e.Key]
	}
	if e.Line > 0 {
		return "WireGuard configuration, line " + strconv.Itoa(e.Line) + ": " + text
	}
	return "WireGuard configuration: " + text
}

// wireGuardExpected tells what each key takes.
var wireGuardExpected = map[string]string{
	"PrivateKey":          "a key of 32 bytes in base64",
	"Address":             "IP addresses, with their prefix length or without, separated by commas",
	"DNS":                 "IP addresses separated by commas",
	"MTU":                 "a number from 576 to 65535",
	"ListenPort":          "a port from 0 to 65535",
	"PublicKey":           "a key of 32 bytes in base64",
	"PresharedKey":        "a key of 32 bytes in base64",
	"Endpoint":            "a host name or an IP address with a port, such as vpn.example.com:51820",
	"AllowedIPs":          "IP addresses with their prefix length, separated by commas",
	"PersistentKeepalive": "off, or a number of seconds from 0 to 65535",
}

// wireGuardKeys names the keys of each section by their lower case. The
// keys of wg-quick that set the host up (Table, FwMark, SaveConfig and the
// PreUp to PostDown commands) are read and left aside: Polyfin sets
// nothing up on its host.
var wireGuardKeys = map[string]map[string]string{
	"Interface": {"privatekey": "PrivateKey", "address": "Address", "dns": "DNS", "mtu": "MTU", "listenport": "ListenPort",
		"table": "", "fwmark": "", "saveconfig": "", "preup": "", "postup": "", "predown": "", "postdown": ""},
	"Peer": {"publickey": "PublicKey", "presharedkey": "PresharedKey", "endpoint": "Endpoint", "allowedips": "AllowedIPs",
		"persistentkeepalive": "PersistentKeepalive"},
}

// wireGuardLists are the keys that may come more than once, their values
// adding up, as wg-quick reads them.
var wireGuardLists = map[string]bool{"Address": true, "DNS": true, "AllowedIPs": true}

// ParseWireGuardConfig reads a WireGuard configuration, a
// *WireGuardConfigError when it is refused.
func ParseWireGuardConfig(text string) (*WireGuardConfig, error) {
	if len(text) > MaxWireGuardConfigBytes || !utf8.ValidString(text) || strings.ContainsRune(text, 0) {
		return nil, &WireGuardConfigError{Reason: WireGuardTooLong}
	}
	var c WireGuardConfig
	var section string
	seen := map[string]int{}
	for i, line := range strings.Split(text, "\n") {
		n := i + 1
		fail := func(key, reason string) (*WireGuardConfig, error) {
			return nil, &WireGuardConfigError{Line: n, Key: key, Reason: reason}
		}
		if comment := strings.IndexByte(line, '#'); comment >= 0 {
			line = line[:comment]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") {
			if !strings.HasSuffix(line, "]") {
				return fail("", WireGuardSyntax)
			}
			switch strings.ToLower(strings.TrimSpace(line[1 : len(line)-1])) {
			case "interface":
				section = "Interface"
			case "peer":
				section = "Peer"
			default:
				return fail(shortKey(line), WireGuardUnknown)
			}
			if seen[section] != 0 {
				return fail(section, WireGuardRepeated)
			}
			seen[section] = n
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if !ok || key == "" {
			return fail("", WireGuardSyntax)
		}
		if section == "" {
			return fail(shortKey(key), WireGuardOutside)
		}
		name, known := wireGuardKeys[section][strings.ToLower(key)]
		if !known {
			return fail(shortKey(key), WireGuardUnknown)
		}
		if name == "" {
			continue
		}
		if seen[name] != 0 && !wireGuardLists[name] {
			return fail(name, WireGuardRepeated)
		}
		seen[name] = n
		if !c.set(name, value) {
			return fail(name, WireGuardInvalid)
		}
	}
	for _, need := range []struct{ key, section string }{{"Interface", ""}, {"PrivateKey", "Interface"}, {"Address", "Interface"},
		{"Peer", ""}, {"PublicKey", "Peer"}, {"Endpoint", "Peer"}, {"AllowedIPs", "Peer"}} {
		if seen[need.key] == 0 {
			return nil, &WireGuardConfigError{Line: seen[need.section], Key: need.key, Reason: WireGuardMissing}
		}
	}
	if !c.ipv4().IsValid() {
		return nil, &WireGuardConfigError{Line: seen["Address"], Key: "Address", Reason: WireGuardNoIPv4}
	}
	if !slices.Contains(c.AllowedIPs, netip.PrefixFrom(netip.IPv4Unspecified(), 0)) {
		return nil, &WireGuardConfigError{Line: seen["AllowedIPs"], Key: "AllowedIPs", Reason: WireGuardNotAll}
	}
	return &c, nil
}

// shortKey bounds what an error repeats of a key it does not know.
func shortKey(key string) string {
	if len(key) > 40 {
		key = strings.ToValidUTF8(key[:40], "") + "…"
	}
	return key
}

// set reads the value of key, false when it is not valid.
func (c *WireGuardConfig) set(key, value string) bool {
	switch key {
	case "PrivateKey":
		return parseWireGuardKey(value, &c.privateKey)
	case "PublicKey":
		return parseWireGuardKey(value, &c.publicKey)
	case "PresharedKey":
		return parseWireGuardKey(value, &c.presharedKey)
	case "Address":
		return eachListed(value, func(item string) bool {
			prefix, ok := parseWireGuardPrefix(item, true)
			if ok && !prefix.Addr().IsUnspecified() {
				c.Addresses = append(c.Addresses, prefix)
			}
			return ok && !prefix.Addr().IsUnspecified()
		})
	case "DNS":
		return eachListed(value, func(item string) bool {
			addr, err := netip.ParseAddr(item)
			if err != nil || addr.Zone() != "" || addr.IsUnspecified() {
				return false
			}
			c.DNS = append(c.DNS, addr.Unmap())
			return true
		})
	case "AllowedIPs":
		return eachListed(value, func(item string) bool {
			prefix, ok := parseWireGuardPrefix(item, false)
			if ok {
				c.AllowedIPs = append(c.AllowedIPs, prefix.Masked())
			}
			return ok
		})
	case "MTU":
		n, err := strconv.Atoi(value)
		c.MTU = n
		return err == nil && n >= 576 && n <= 65535
	case "ListenPort":
		n, err := strconv.Atoi(value)
		c.ListenPort = n
		return err == nil && n >= 0 && n <= 65535
	case "PersistentKeepalive":
		if strings.EqualFold(value, "off") {
			c.PersistentKeepalive = 0
			return true
		}
		n, err := strconv.Atoi(value)
		c.PersistentKeepalive = n
		return err == nil && n >= 0 && n <= 65535
	case "Endpoint":
		endpoint, ok := parseWireGuardEndpoint(value)
		c.Endpoint = endpoint
		return ok
	}
	return false
}

// parseWireGuardKey reads a key in base64 into key. A private key of zeros
// is no key.
func parseWireGuardKey(value string, key *[32]byte) bool {
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil || len(decoded) != len(key) {
		return false
	}
	copy(key[:], decoded)
	return *key != [32]byte{}
}

// eachListed calls read with each item of a list separated by commas,
// false when an item is empty or read refuses one.
func eachListed(value string, read func(string) bool) bool {
	for item := range strings.SplitSeq(value, ",") {
		if item = strings.TrimSpace(item); item == "" || !read(item) {
			return false
		}
	}
	return true
}

// parseWireGuardPrefix reads an address with its prefix length, or
// without when bare is true, an address alone then.
func parseWireGuardPrefix(item string, bare bool) (netip.Prefix, bool) {
	if !strings.Contains(item, "/") {
		addr, err := netip.ParseAddr(item)
		if !bare || err != nil || addr.Zone() != "" {
			return netip.Prefix{}, false
		}
		addr = addr.Unmap()
		return netip.PrefixFrom(addr, addr.BitLen()), true
	}
	prefix, err := netip.ParsePrefix(item)
	if err != nil {
		return netip.Prefix{}, false
	}
	if prefix.Addr().Is4In6() {
		bits := prefix.Bits() - 96
		if bits < 0 {
			return netip.Prefix{}, false
		}
		prefix = netip.PrefixFrom(prefix.Addr().Unmap(), bits)
	}
	return prefix, true
}

// parseWireGuardEndpoint reads host:port, the host a name or an address,
// an IPv6 address in brackets, and returns it as written again.
func parseWireGuardEndpoint(value string) (string, bool) {
	host, port, err := net.SplitHostPort(value)
	if err != nil {
		return "", false
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return "", false
	}
	if addr, err := netip.ParseAddr(host); err == nil {
		if addr.Zone() != "" || addr.IsUnspecified() {
			return "", false
		}
		return netip.AddrPortFrom(addr.Unmap(), uint16(n)).String(), true
	}
	if !validHostName(host) {
		return "", false
	}
	return net.JoinHostPort(strings.ToLower(host), strconv.Itoa(n)), true
}

// validHostName reports whether host is a DNS name: labels of letters,
// digits and hyphens, neither starting nor ending with one, 253 bytes at
// most, its last label not all digits.
func validHostName(host string) bool {
	host = strings.TrimSuffix(host, ".")
	if host == "" || len(host) > 253 {
		return false
	}
	labels := strings.Split(host, ".")
	for _, label := range labels {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, r := range label {
			if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-') {
				return false
			}
		}
	}
	_, err := strconv.Atoi(labels[len(labels)-1])
	return err != nil
}

// ipv4 is the tunnel's first IPv4 address, invalid without one.
func (c *WireGuardConfig) ipv4() netip.Addr {
	for _, prefix := range c.Addresses {
		if prefix.Addr().Is4() {
			return prefix.Addr()
		}
	}
	return netip.Addr{}
}

// hasIPv6 tells whether the tunnel has an IPv6 address.
func (c *WireGuardConfig) hasIPv6() bool {
	return slices.ContainsFunc(c.Addresses, func(prefix netip.Prefix) bool { return prefix.Addr().Is6() })
}

func joinPrefixes(prefixes []netip.Prefix) string {
	items := make([]string, len(prefixes))
	for i, prefix := range prefixes {
		items[i] = prefix.String()
	}
	return strings.Join(items, ", ")
}

// Marshal writes the configuration as it is stored: its keys alone, in a
// fixed order, without comments. It holds the private key: it is stored
// as a secret, and never logged.
func (c *WireGuardConfig) Marshal() string {
	var b strings.Builder
	line := func(key, value string) {
		b.WriteString(key + " = " + value + "\n")
	}
	b.WriteString("[Interface]\n")
	line("PrivateKey", base64.StdEncoding.EncodeToString(c.privateKey[:]))
	line("Address", joinPrefixes(c.Addresses))
	if len(c.DNS) > 0 {
		dns := make([]string, len(c.DNS))
		for i, addr := range c.DNS {
			dns[i] = addr.String()
		}
		line("DNS", strings.Join(dns, ", "))
	}
	if c.MTU != 0 {
		line("MTU", strconv.Itoa(c.MTU))
	}
	if c.ListenPort != 0 {
		line("ListenPort", strconv.Itoa(c.ListenPort))
	}
	b.WriteString("\n[Peer]\n")
	line("PublicKey", base64.StdEncoding.EncodeToString(c.publicKey[:]))
	if c.presharedKey != [32]byte{} {
		line("PresharedKey", base64.StdEncoding.EncodeToString(c.presharedKey[:]))
	}
	line("Endpoint", c.Endpoint)
	line("AllowedIPs", joinPrefixes(c.AllowedIPs))
	if c.PersistentKeepalive != 0 {
		line("PersistentKeepalive", strconv.Itoa(c.PersistentKeepalive))
	}
	return b.String()
}

// mtu is the tunnel's MTU.
func (c *WireGuardConfig) mtu() int {
	if c.MTU == 0 {
		return DefaultWireGuardMTU
	}
	return c.MTU
}

// uapi is the configuration as wireguard-go's device takes it, with the
// server at endpoint.
func (c *WireGuardConfig) uapi(endpoint netip.AddrPort) string {
	var b strings.Builder
	b.WriteString("private_key=" + hex.EncodeToString(c.privateKey[:]) + "\n")
	b.WriteString("listen_port=" + strconv.Itoa(c.ListenPort) + "\n")
	b.WriteString("replace_peers=true\n")
	b.WriteString("public_key=" + hex.EncodeToString(c.publicKey[:]) + "\n")
	if c.presharedKey != [32]byte{} {
		b.WriteString("preshared_key=" + hex.EncodeToString(c.presharedKey[:]) + "\n")
	}
	b.WriteString("endpoint=" + endpoint.String() + "\n")
	b.WriteString("persistent_keepalive_interval=" + strconv.Itoa(c.PersistentKeepalive) + "\n")
	b.WriteString("replace_allowed_ips=true\n")
	for _, prefix := range c.AllowedIPs {
		b.WriteString("allowed_ip=" + prefix.String() + "\n")
	}
	return b.String()
}
