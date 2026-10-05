package backup

import (
	"errors"
	"net/url"
	"strings"
)

// libpqKeywords are the connection parameters pg_dump's libpq reads. pgx
// reads others, such as pool_max_conns, which libpq refuses, and passes
// the rest to the server as settings, which pg_dump makes its own: both
// are left out. replication, which no dump uses, is too.
var libpqKeywords = map[string]bool{
	"host": true, "hostaddr": true, "port": true, "dbname": true, "user": true, "passfile": true,
	"require_auth": true, "channel_binding": true, "connect_timeout": true, "client_encoding": true, "options": true,
	"application_name": true, "fallback_application_name": true, "keepalives": true, "keepalives_idle": true,
	"keepalives_interval": true, "keepalives_count": true, "tcp_user_timeout": true, "gssencmode": true,
	"sslmode": true, "requiressl": true, "sslnegotiation": true, "sslcompression": true, "sslcert": true, "sslkey": true,
	"sslkeylogfile": true, "sslpassword": true, "sslcertmode": true, "sslrootcert": true, "sslcrl": true, "sslcrldir": true,
	"sslsni": true, "requirepeer": true, "ssl_min_protocol_version": true, "ssl_max_protocol_version": true,
	"min_protocol_version": true, "max_protocol_version": true, "krbsrvname": true, "gsslib": true, "gssdelegation": true,
	"service": true, "servicefile": true, "target_session_attrs": true, "load_balance_hosts": true,
	"oauth_issuer": true, "oauth_client_id": true, "oauth_client_secret": true, "oauth_scope": true,
	"scram_client_key": true, "scram_server_key": true,
}

// errConnection reports a database URL pg_dump cannot be given. It never
// quotes the URL, which holds the password.
var errConnection = errors.New("the database URL cannot be passed to pg_dump")

// splitPassword returns the connection string connection, a URL or
// keyword=value pairs as pgx reads them, without its password, for
// pg_dump's command line, and the password apart. The parameters libpq
// does not read are left out.
func splitPassword(connection string) (conninfo, password string, err error) {
	if strings.HasPrefix(connection, "postgres://") || strings.HasPrefix(connection, "postgresql://") {
		return splitURLPassword(connection)
	}
	pairs, err := parseKeywordValues(connection)
	if err != nil {
		return "", "", err
	}
	var kept []string
	for _, pair := range pairs {
		switch {
		case pair[0] == "password":
			password = pair[1]
		case libpqKeywords[pair[0]]:
			kept = append(kept, pair[0]+"='"+strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(pair[1])+"'")
		}
	}
	return strings.Join(kept, " "), password, nil
}

func splitURLPassword(connection string) (conninfo, password string, err error) {
	parsed, err := url.Parse(connection)
	if err != nil {
		return "", "", errConnection
	}
	if parsed.User != nil {
		password, _ = parsed.User.Password()
		if user := parsed.User.Username(); user != "" {
			parsed.User = url.User(user)
		} else {
			parsed.User = nil
		}
	}
	query, err := url.ParseQuery(parsed.RawQuery)
	if err != nil {
		return "", "", errConnection
	}
	// As with pgx, the query's password wins over the user's.
	if values, ok := query["password"]; ok && len(values) > 0 {
		password = values[len(values)-1]
	}
	for key := range query {
		if !libpqKeywords[key] {
			query.Del(key)
		}
	}
	parsed.RawQuery = query.Encode()
	return parsed.String(), password, nil
}

// parseKeywordValues reads keyword=value pairs as libpq does: a value is
// a word, or quoted between single quotes, with backslash escapes.
func parseKeywordValues(text string) ([][2]string, error) {
	var pairs [][2]string
	const spaces = " \t\n\r\v\f"
	text = strings.TrimLeft(text, spaces)
	for text != "" {
		key, rest, ok := strings.Cut(text, "=")
		key = strings.TrimRight(key, spaces)
		if !ok || key == "" || strings.ContainsAny(key, spaces) {
			return nil, errConnection
		}
		rest = strings.TrimLeft(rest, spaces)
		var value strings.Builder
		if strings.HasPrefix(rest, "'") {
			i := 1
			for ; i < len(rest) && rest[i] != '\''; i++ {
				if rest[i] == '\\' {
					i++
					if i == len(rest) {
						return nil, errConnection
					}
				}
				value.WriteByte(rest[i])
			}
			if i == len(rest) {
				return nil, errConnection
			}
			rest = rest[i+1:]
		} else {
			i := 0
			for ; i < len(rest) && !strings.ContainsRune(spaces, rune(rest[i])); i++ {
				if rest[i] == '\\' {
					// A trailing backslash ends the value, as in libpq.
					if i++; i == len(rest) {
						break
					}
				}
				value.WriteByte(rest[i])
			}
			rest = rest[i:]
		}
		pairs = append(pairs, [2]string{key, value.String()})
		text = strings.TrimLeft(rest, spaces)
	}
	return pairs, nil
}
