package logs

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"strings"
	"testing"
)

// serverLogger logs as the server does: a text handler writing each record
// to standard error, here a buffer, and to the ring administrators read.
func serverLogger() (*slog.Logger, *bytes.Buffer, *Ring) {
	stderr := &bytes.Buffer{}
	ring := NewRing(Capacity)
	return slog.New(slog.NewTextHandler(io.MultiWriter(stderr, ring), &slog.HandlerOptions{Level: slog.LevelDebug})), stderr, ring
}

// Each record is logged the way a call site would log it: standard error
// has it whole, and the ring has it without its secret.
func TestTheRingHidesTheSecretsOfARecord(t *testing.T) {
	t.Parallel()
	form := errors.New("form: client_secret=CS-1a2b3c&code=AUTHCODE-77&refresh_token=RT-9f8e7d&grant_type=refresh_token")
	apiKeys := "ApiKey=K1-aaaa&api_key=K2-bbbb&apikey=K3-cccc"
	attributes := func(l *slog.Logger) { l.Warn("x", "token", "ATTR-tok1", "password", "ATTR-pw1") }
	for name, c := range map[string]struct {
		log    func(*slog.Logger)
		secret string
	}{
		// Addresses, as the errors of Polyfin's requests carry them.
		"url.Error": {func(l *slog.Logger) {
			l.Warn("x", "error", &url.Error{Op: "Get", URL: "https://api.example/3/find?api_key=TMDB-112233", Err: io.EOF})
		}, "TMDB-112233"},
		"path of an address": {func(l *slog.Logger) {
			l.Warn("x", "error", fmt.Errorf("open http://p.example/live/alice/XT-pw/1.ts: EOF"))
		}, "XT-pw"},
		"user of an address": {func(l *slog.Logger) { l.Warn("x", "address", "smb://bob:SMB-pw@nas.local/share") }, "SMB-pw"},
		"database address": {func(l *slog.Logger) {
			l.Warn("x", "error", "connect postgresql://polyfin:PG-pw@db:5432/polyfin failed")
		}, "PG-pw"},
		"bot path": {func(l *slog.Logger) {
			l.Warn("x", "error", "Post https://api.telegram.org/bot123:TG-key/sendMessage: EOF")
		}, "TG-key"},
		"FFmpeg's input": {func(l *slog.Logger) {
			l.Warn("x", "error", "FFmpeg failed: exit status 8: Error opening input file https://cdn.example/dl/DEBRID-key/movie.mkv.")
		}, "DEBRID-key"},
		"quote in a query": {func(l *slog.Logger) {
			l.Warn("x", "error", errors.New("open http://provider.example/get.php?username=a&password=p'TAIL-9z failed"))
		}, "TAIL-9z"},
		"address in a query": {func(l *slog.Logger) {
			l.Warn("x", "magnet", "magnet:?xt=urn:btih:00&tr=http%3A%2F%2Ftracker.example%2FPASSKEY-5566%2Fannounce")
		}, "PASSKEY-5566"},
		"address without a scheme": {func(l *slog.Logger) { l.Warn("x", "input", "provider.example:8080/live/alice/XTREAM-pw1/42.ts") }, "XTREAM-pw1"},
		"user without a scheme":    {func(l *slog.Logger) { l.Warn("x", "input", "alice:XTREAM-pw3@provider.example:8080/get.php") }, "XTREAM-pw3"},
		"address from its slashes": {func(l *slog.Logger) { l.Warn("x", "input", "//provider.example/live/alice/XTREAM-pw2/42.ts") }, "XTREAM-pw2"},

		// Names, whatever starts them.
		"database pairs": {func(l *slog.Logger) { l.Warn("x", "error", "host=db user=polyfin password=PG-pw2 dbname=polyfin") }, "PG-pw2"},
		"ApiKey":         {func(l *slog.Logger) { l.Warn("x", "query", apiKeys) }, "K1-aaaa"},
		"api_key":        {func(l *slog.Logger) { l.Warn("x", "query", apiKeys) }, "K2-bbbb"},
		"apikey":         {func(l *slog.Logger) { l.Warn("x", "query", apiKeys) }, "K3-cccc"},
		"token":          {attributes, "ATTR-tok1"},
		"password":       {attributes, "ATTR-pw1"},
		"refresh_token":  {func(l *slog.Logger) { l.Warn("x", "error", form) }, "RT-9f8e7d"},
		"client_secret":  {func(l *slog.Logger) { l.Warn("x", "error", form) }, "CS-1a2b3c"},
		"code":           {func(l *slog.Logger) { l.Warn("x", "error", form) }, "AUTHCODE-77"},
		"id_token":       {func(l *slog.Logger) { l.Warn("x", "answer", "id_token=eyJhbGciOiJSUzI1NiJ9.e30.sig") }, "eyJhbGciOiJSUzI1NiJ9.e30.sig"},
		"key":            {func(l *slog.Logger) { l.Warn("x", "query", "key=K-112233&id=tt1") }, "K-112233"},
		"pass":           {func(l *slog.Logger) { l.Warn("x", "query", "user=bob&pass=hunter2") }, "hunter2"},
		"passwd":         {func(l *slog.Logger) { l.Warn("x", "dsn", "host=db user=polyfin passwd=hunter3") }, "hunter3"},
		"PGPASSWORD":     {func(l *slog.Logger) { l.Warn("x", "environment", "PGHOST=db PGPASSWORD=hunter5") }, "hunter5"},
		"passphrase":     {func(l *slog.Logger) { l.Warn("x", "settings", "{BackupPassphrase:hunter6 Language:en}") }, "hunter6"},
		"sig":            {func(l *slog.Logger) { l.Warn("x", "query", "expires=1&sig=SIG-112233") }, "SIG-112233"},
		"signature":      {func(l *slog.Logger) { l.Warn("x", "query", "expires=1&signature=SIG-445566") }, "SIG-445566"},
		"auth":           {func(l *slog.Logger) { l.Warn("x", "query", "id=1&auth=AUTH-778899") }, "AUTH-778899"},
		"encoded pair":   {func(l *slog.Logger) { l.Warn("x", "next", "/web/?redirect=%2Fx%3Fapi_key%3DAK-998877") }, "AK-998877"},

		// Headers.
		"Plex's header":    {func(l *slog.Logger) { l.Warn("x", "header", "X-Plex-Token: PLEX-abcdef") }, "PLEX-abcdef"},
		"Gotify's header":  {func(l *slog.Logger) { l.Warn("x", "header", "X-Gotify-Key: GOTIFY-123456") }, "GOTIFY-123456"},
		"Trakt's header":   {func(l *slog.Logger) { l.Warn("x", "header", "trakt-api-key: TRAKT-654321") }, "TRAKT-654321"},
		"Bearer":           {func(l *slog.Logger) { l.Warn("x", "header", "Authorization: Bearer BEARER-abcdef") }, "BEARER-abcdef"},
		"Basic":            {func(l *slog.Logger) { l.Warn("x", "header", "Authorization: Basic dXNlcjpwYXNz") }, "dXNlcjpwYXNz"},
		"Bearer in a map":  {func(l *slog.Logger) { l.Warn("x", "header", "map[Authorization:[Bearer BEARER-fedcba]]") }, "BEARER-fedcba"},
		"Bearer by itself": {func(l *slog.Logger) { l.Warn("x", "sent", "Bearer 0123abcd4567ef") }, "0123abcd4567ef"},
		"Cookie":           {func(l *slog.Logger) { l.Warn("x", "header", "Cookie: theme=dark; polyfin_session=SESSION-778899") }, "SESSION-778899"},
		"Set-Cookie": {func(l *slog.Logger) {
			l.Warn("x", "header", "Set-Cookie: polyfin_session=SESSION-112233; Path=/; HttpOnly")
		}, "SESSION-112233"},
		"cookie in quotes": {func(l *slog.Logger) { l.Warn("x", "header", `Cookie: polyfin_session="SESSION 445566"`) }, "445566"},
		"MediaBrowser":     {func(l *slog.Logger) { l.Warn("x", "header", `Authorization: MediaBrowser Token="MB-d4e5f6"`) }, "MB-d4e5f6"},
		"MediaBrowser twice": {func(l *slog.Logger) {
			l.Warn("x", "document", `{"headers":"X-Emby-Authorization: MediaBrowser Token=\"MB-778899\""}`)
		}, "MB-778899"},

		// Quoted values: the text handler escapes their quotes.
		"JSON":              {func(l *slog.Logger) { l.Warn("x", "body", `{"access_token":"AT-a1b2c3","token_type":"bearer"}`) }, "AT-a1b2c3"},
		"JSON with a quote": {func(l *slog.Logger) { l.Warn("x", "body", `{"password":"hunter\"7 TAIL-8y","user":"bob"}`) }, "TAIL-8y"},
		"quoted attribute":  {func(l *slog.Logger) { l.Warn("x", "password", `hunter "9 TAIL-7x`) }, "TAIL-7x"},
		"single quotes":     {func(l *slog.Logger) { l.Warn("x", "dsn", "host=db password='hunter 4x'") }, "4x"},
		"quotes left open":  {func(l *slog.Logger) { l.Warn("x", "body", `{"access_token":"AT-d4e5f6`) }, "AT-d4e5f6"},
	} {
		logger, stderr, ring := serverLogger()
		c.log(logger)
		if !strings.Contains(stderr.String(), c.secret) {
			t.Errorf("%s: the record does not hold %q: %s", name, c.secret, stderr.String())
		}
		if got := string(ring.Contents()); strings.Contains(got, c.secret) {
			t.Errorf("%s: the ring keeps %q: %s", name, c.secret, got)
		}
	}
}

// What an administrator reads the log for stays as it was logged: words,
// identifiers, durations, file paths, the hosts of addresses, and the two
// values Polyfin logs for them to read.
func TestTheRingKeepsWhatAdministratorsRead(t *testing.T) {
	t.Parallel()
	for name, log := range map[string]func(*slog.Logger){
		"started": func(l *slog.Logger) { l.Info("Polyfin started", "address", "[::]:8096", "version", "0.82.0") },
		"reset PIN": func(l *slog.Logger) {
			l.Info("A password reset PIN was made", "user", "root", "pin", "12-34-56", "expires", "2026-01-01T00:30:00Z")
		},
		"setup code": func(l *slog.Logger) {
			l.Warn("No administrator yet: open /admin/ and create one with this setup code", "setup_code", "ABCD-EFGH")
		},
		"API key made": func(l *slog.Logger) { l.Info("An API key was made", "app", "Requests", "by", "root") },
		"identifiers": func(l *slog.Logger) {
			l.Warn("A stream could not be served", "addon", "Example", "title", "tt0111161", "item", "0123456789abcdef0123456789abcdef", "took", "1.25s")
		},
		"file": func(l *slog.Logger) {
			l.Error("A file could not be read", "file", "/media/Movies/Heat (1995)/Heat.mkv", "error", "open /media/Movies/Heat (1995)/Heat.mkv: no such file or directory")
		},
		"network share": func(l *slog.Logger) { l.Warn("A folder is away", "folder", "/mnt/share/tv", "address", "//nas/media") },
		"refused connection": func(l *slog.Logger) {
			l.Error("The database does not answer", "error", "dial tcp 127.0.0.1:5432: connect: connection refused")
		},
		"route": func(l *slog.Logger) {
			l.Error("Admin API request failed", "method", "GET", "path", "/admin/api/invite/{token}", "error", "closed pool")
		},
		"status": func(l *slog.Logger) {
			l.Warn("An addon refused", "error", `status 403: {"code":403,"message":"Forbidden"}`, "code", 502)
		},
		"exit code": func(l *slog.Logger) { l.Error("FFmpeg failed", "error", "exit code: 137") },
		"lookup pass": func(l *slog.Logger) {
			l.Warn("A pass of TMDB lookups failed", "pass", "ratings", "error", "context deadline exceeded")
		},
		"bearer token": func(l *slog.Logger) { l.Warn("The metrics need the bearer token of the settings", "status", 401) },
		"ratio": func(l *slog.Logger) {
			l.Info("Chose a version", "aspect", "16:9", "bitrate", "8.5 Mbit/s", "at", "12:30/13:00")
		},
	} {
		logger, stderr, ring := serverLogger()
		log(logger)
		if got := string(ring.Contents()); got != stderr.String() || got == "" {
			t.Errorf("%s: the ring has\n%sof\n%s", name, got, stderr.String())
		}
	}
}

func TestRedactKeepsThePublicPartsOfALine(t *testing.T) {
	t.Parallel()
	for line, want := range map[string]string{
		// Addresses keep their host, with or without a scheme.
		`open http://provider.example/get.php?username=a&password=p'q failed`: `open http://provider.example/… failed`,
		`tr=http%3A%2F%2Ftracker.example%3A6969%2Fkey%2Fannounce&dn=Name`:     `tr=http://tracker.example:6969/…&dn=Name`,
		`input=provider.example:8080/live/alice/pw/42.ts status=404`:          `input=provider.example:8080/… status=404`,
		`input=alice:pw@10.0.0.7:8080/get.php status=404`:                     `input=10.0.0.7:8080/… status=404`,
		`input="//provider.example/live/alice/pw/42.ts" status=404`:           `input="//provider.example/…" status=404`,
		`https://tenant.addon.example/manifest.json`:                          `https://tenant.addon.example/…`,
		// Twice redacted, as an app's log is: nothing more is lost.
		`url=https://addon.example/… token=<redacted> input=provider.example:8080/…`: `url=https://addon.example/… token=<redacted> input=provider.example:8080/…`,
		// A value ends where the next one starts.
		`refresh_token=abc&grant_type=refresh_token&scope=read`:            `refresh_token=<redacted>&grant_type=refresh_token&scope=read`,
		`{"access_token":"abc","token_type":"bearer","expires_in":3600}`:   `{"access_token":<redacted>,"token_type":"bearer","expires_in":3600}`,
		`body="{\"access_token\":\"abc\",\"expires_in\":3600}" status=200`: `body="{\"access_token\":<redacted>,\"expires_in\":3600}" status=200`,
		`body="{\"password\":\"a\\\"b\",\"user\":\"bob\"}" status=200`:     `body="{\"password\":<redacted>,\"user\":\"bob\"}" status=200`,
		`{"headers":"X-Emby-Authorization: MediaBrowser Token=\"abc\""}`:   `{"headers":"X-Emby-Authorization: <redacted> Token=<redacted>"}`,
		`password="a \"quoted\" word" user=bob`:                            `password=<redacted> user=bob`,
		`password='two words' user=bob`:                                    `password=<redacted> user=bob`,
		`next=/web/?redirect=%2Fx%3Fapi_key%3Dabc status=302`:              `next=/web/?redirect=%2Fx%3Fapi_key%3D<redacted> status=302`,
		`header="Authorization: Bearer abc.def.ghi" status=401`:            `header="Authorization: <redacted>" status=401`,
		`header="Cookie: a=b; c=d" status=401`:                             `header="Cookie: <redacted>" status=401`,
		`Set-Cookie: session=abc; Path=/; HttpOnly and the rest`:           `Set-Cookie: <redacted> and the rest`,
		`sent Bearer 0123abcd to the tool`:                                 `sent Bearer <redacted> to the tool`,
		// The codes of an answer and of a process are not secrets.
		`code=404 code: 137 code=AUTHCODE code=1234`: `code=404 code: 137 code=<redacted> code=<redacted>`,
		// A line with nothing to hide is the line.
		`level=INFO msg="A listing was slow" folder=Movies checked=1.2s read=3.4s described=0.9s`: `level=INFO msg="A listing was slow" folder=Movies checked=1.2s read=3.4s described=0.9s`,
		`open /media/share/Films/a.mkv: permission denied`:                                        `open /media/share/Films/a.mkv: permission denied`,
		`pin=12-34-56 setup_code=ABCD-EFGH pass=ratings`:                                          `pin=12-34-56 setup_code=ABCD-EFGH pass=ratings`,
	} {
		if got := Redact(line); got != want {
			t.Errorf("Redact(%s)\n got %s\nwant %s", line, got, want)
		}
	}
}

// A long line of one character, or of the starts of what Redact looks
// for, is read as a short one is: the patterns are Go's regular
// expressions, which read a line once whatever it holds, and Redact goes
// over nothing but their matches. Each line is 16 kB, for the race
// detector's sake; hidden counts what is replaced in it.
func TestRedactReadsALongLineOnce(t *testing.T) {
	t.Parallel()
	const size = 1 << 14
	// half is one value for two units: the second closes the quotes that
	// the first opens, and the last one's stay open.
	half := func(unit string) int { return (size/len(unit) + 1) / 2 }
	for unit, hidden := range map[string]int{
		"a": 0, `\`: 0, `"`: 0, `'`: 0, "=": 0, ":": 0, "/": 0, "%": 0, ";": 0, "&": 0, ".": 0, "-": 0, "@": 0, " ": 0,
		"key": 0, "bearer ": 0, "://": 0, "code=1 ": 0,
		"pw=1 ": size / 5, "code=x ": size / 7, "u:p@a.b:80/x ": size / 13,
		`pw=\"`: half(`pw=\"`), `pw="\" `: half(`pw="\" `), "pw='": half("pw='"), `key=\\\\\\\"`: half(`key=\\\\\\\"`), `token: \\\" `: half(`token: \\\" `),
		// One value, or one address, to the end of the line.
		"cookie=a;": 1, "key=%3D": 1, "a.b:80/": 1, "//a.b/": 1, "x://y/": 1, "x%3A%2F%2Fy/": 1,
	} {
		line := strings.Repeat(unit, size/len(unit))
		got := Redact(line)
		if count := strings.Count(got, "<redacted>") + strings.Count(got, "…"); count != hidden || hidden == 0 && got != line {
			t.Errorf("a line of %q: %d replaced for %d, in %d bytes starting %.60q", unit, count, hidden, len(got), got)
		}
	}
}
