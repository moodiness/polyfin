package playback

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/moodiness/polyfin/internal/drm"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
)

// A channel's DASH stream (an MPD manifest) is read by FFmpeg's dash
// demuxer through the loopback relay, as an HLS playlist is, its manifest
// rewritten so that every file it names is fetched through Polyfin too
// (see rewriteManifest), and plays converted into HLS: apps play no DASH.
// Its list may give ClearKey keys, or a ClearKey licence server, which
// FFmpeg decrypts it with (see clearKeys); any other licence is a DRM a
// server cannot have.

var (
	// ErrLiveDRM reports a stream that needs a DRM Polyfin does not
	// support: its list names another licence than ClearKey (Widevine,
	// PlayReady...), which only a content decryption module licensed to
	// browsers and devices opens, or it is encrypted and its list gives no
	// ClearKey keys. It is no network failure: it is kept as the stream's
	// failure, not asked again at the next start.
	ErrLiveDRM = errors.New("the stream needs a DRM Polyfin does not support")
	// ErrLiveKeys reports a stream the ClearKey keys of its list, or of
	// its licence server, do not decrypt.
	ErrLiveKeys = errors.New("the stream's ClearKey keys do not decrypt it")
	// errPlayerManifest reports a DASH manifest asked by a player, which
	// plays it only converted: it is relayed to FFmpeg only.
	errPlayerManifest = errors.New("a DASH manifest is not relayed to players")
)

// licenseWait bounds a ClearKey licence server's answer, and maxLicense
// its size.
const (
	licenseWait = 10 * time.Second
	maxLicense  = 64 << 10
)

// DASH reports whether a live analysis is of a DASH manifest, which plays
// only converted into HLS.
func DASH(analysis media.Analysis) bool {
	return strings.Contains(","+analysis.Format+",", ",dash,")
}

// dashStart reports whether head, the first bytes of an answer, starts a
// DASH manifest: its MPD element, after a byte order mark, white space,
// an XML declaration or comments.
func dashStart(head []byte) bool {
	text := bytes.TrimLeft(bytes.TrimPrefix(head, []byte("\xef\xbb\xbf")), " \t\r\n")
	if bytes.HasPrefix(text, []byte("<MPD")) {
		return true
	}
	return (bytes.HasPrefix(text, []byte("<?xml")) || bytes.HasPrefix(text, []byte("<!--"))) && bytes.Contains(text, []byte("<MPD"))
}

// dashAnswer reports whether a response is a DASH manifest: by its type,
// or by its first bytes.
func dashAnswer(response *http.Response, body *bufio.Reader) bool {
	if strings.Contains(strings.ToLower(response.Header.Get("Content-Type")), "dash+xml") {
		return true
	}
	head, _ := body.Peek(1 << 10)
	return dashStart(head)
}

// manifestInfo is what a DASH manifest says of its encryption: the
// protection systems it names (ContentProtection), and the key IDs of its
// tracks (cenc:default_KID), in hexadecimal.
type manifestInfo struct {
	systems []string
	kids    []string
}

// Systems Polyfin names when a stream it cannot decrypt names them, by
// their DASH scheme.
var systemNames = map[string]string{
	"urn:uuid:edef8ba9-79d6-4ace-a3c8-27dcd51d21ed": "Widevine",
	"urn:uuid:9a04f079-9840-4286-ab92-e65be0885f95": "PlayReady",
	"urn:uuid:94ce86fb-07ff-4f43-adb8-93d2fa968ca2": "FairPlay",
	"urn:uuid:e2719d58-a985-b3c9-781a-b030af78d30e": "ClearKey",
	"urn:uuid:1077efec-c0b2-4d02-ace3-3c1e52e2fb4b": "ClearKey",
}

// named lists the systems of a manifest Polyfin knows by name, once each.
func (m manifestInfo) named() string {
	var names []string
	for _, system := range m.systems {
		if name, ok := systemNames[system]; ok && !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	return strings.Join(names, ", ")
}

// readManifest reads what a DASH manifest says of its encryption.
func readManifest(data []byte) (manifestInfo, error) {
	var info manifestInfo
	d := manifestDecoder(data)
	for {
		token, err := d.RawToken()
		if errors.Is(err, io.EOF) {
			return info, nil
		}
		if err != nil {
			return manifestInfo{}, err
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "ContentProtection" {
			continue
		}
		for _, a := range start.Attr {
			switch a.Name.Local {
			case "schemeIdUri":
				info.systems = append(info.systems, strings.ToLower(strings.TrimSpace(a.Value)))
			case "default_KID":
				if raw, ok := drm.Bytes(a.Value); ok {
					if kid := fmt.Sprintf("%x", raw); !slices.Contains(info.kids, kid) {
						info.kids = append(info.kids, kid)
					}
				}
			}
		}
	}
}

// manifestDecoder reads the tokens of a manifest as written, its bytes in
// whatever encoding it declares taken as they are: only its addresses
// matter, which are ASCII.
func manifestDecoder(data []byte) *xml.Decoder {
	d := xml.NewDecoder(bytes.NewReader(data))
	d.CharsetReader = func(_ string, input io.Reader) (io.Reader, error) { return input, nil }
	return d
}

// addressAttributes are the attributes of a DASH manifest's elements that
// hold an address, a template's included.
var addressAttributes = map[string][]string{
	"SegmentTemplate":     {"media", "initialization", "index"},
	"SegmentURL":          {"media", "index"},
	"Initialization":      {"sourceURL"},
	"RepresentationIndex": {"sourceURL"},
	"BitstreamSwitching":  {"sourceURL"},
}

// manifestFrame is an element of a manifest being rewritten: inherited is
// the base address its parent gives it, base its own once its first
// BaseURL is read.
type manifestFrame struct {
	name            string
	inherited, base *url.URL
	based           bool
}

// rewriteManifest names every address of a DASH manifest, whose own
// address is base, on the loopback interface, so that FFmpeg's dash
// demuxer fetches its files through Polyfin too, with the headers the
// source needs and within its confinement. folder names an origin's root
// there (scheme://host), whose paths follow it as they are, so that the
// addresses relative to a folder resolve there as they would at the
// source:
//
//   - every BaseURL is resolved against its parent's base, as DASH does,
//     and written as the loopback's; the MPD is given its own folder for
//     BaseURL when it names none, which relative addresses then resolve
//     against;
//   - a template's or a segment list's address that is absolute, or an
//     absolute path, is written as the loopback's; a relative one stays.
//
// A dynamic manifest (live) of several periods keeps its last one, the
// one that plays now: FFmpeg's demuxer plays one period, the longest
// (dashdec.c, parse_manifest), where a live stream's current period has
// no duration yet. Any address that is not HTTP refuses the manifest, as
// FFmpeg would read it itself.
func rewriteManifest(data []byte, base *url.URL, folder func(origin string) string) ([]byte, error) {
	link := func(target *url.URL) (string, error) {
		if target.Scheme != "http" && target.Scheme != "https" || target.Host == "" {
			return "", fmt.Errorf("%w: %q", ErrUnsafePlaylist, target.Redacted())
		}
		rest := strings.TrimPrefix(target.EscapedPath(), "/")
		if target.RawQuery != "" {
			rest += "?" + target.RawQuery
		}
		return folder(target.Scheme+"://"+target.Host) + rest, nil
	}
	named, err := mpdBase(data)
	if err != nil {
		return nil, err
	}
	d := manifestDecoder(data)
	var out bytes.Buffer
	var stack []manifestFrame
	var periods [][2]int
	periodStart, dynamic := 0, false
	var text strings.Builder
	last := int64(0)
	for {
		token, err := d.RawToken()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		raw := data[last:d.InputOffset()]
		last = d.InputOffset()
		inherited := base
		if len(stack) > 0 {
			inherited = stack[len(stack)-1].base
		}
		switch t := token.(type) {
		case xml.StartElement:
			if len(stack) == 1 && t.Name.Local == "Period" {
				periodStart = out.Len()
			}
			changed := false
			for i, a := range t.Attr {
				if !slices.Contains(addressAttributes[t.Name.Local], a.Name.Local) {
					continue
				}
				rewritten, err := rewriteAddress(a.Value, inherited, link)
				if err != nil {
					return nil, err
				}
				if rewritten != a.Value {
					t.Attr[i].Value, changed = rewritten, true
				}
			}
			if changed {
				writeStart(&out, t, bytes.HasSuffix(raw, []byte("/>")))
			} else {
				out.Write(raw)
			}
			stack = append(stack, manifestFrame{name: t.Name.Local, inherited: inherited, base: inherited})
			if len(stack) == 1 && t.Name.Local == "MPD" {
				for _, a := range t.Attr {
					dynamic = dynamic || a.Name.Local == "type" && strings.TrimSpace(a.Value) == "dynamic"
				}
				if !named {
					own, err := link(base.ResolveReference(&url.URL{Path: "./"}))
					if err != nil {
						return nil, err
					}
					out.WriteString("<BaseURL>")
					_ = xml.EscapeText(&out, []byte(own))
					out.WriteString("</BaseURL>")
				}
			}
			text.Reset()
		case xml.EndElement:
			if len(stack) == 0 {
				return nil, fmt.Errorf("%w: a manifest that is no XML document", ErrUnsafePlaylist)
			}
			frame := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if frame.name == "BaseURL" {
				ref, err := url.Parse(strings.TrimSpace(text.String()))
				if err != nil {
					return nil, fmt.Errorf("%w: a BaseURL that does not parse", ErrUnsafePlaylist)
				}
				resolved := frame.inherited
				if len(stack) > 0 {
					resolved = stack[len(stack)-1].inherited
				}
				resolved = resolved.ResolveReference(ref)
				address, err := link(resolved)
				if err != nil {
					return nil, err
				}
				_ = xml.EscapeText(&out, []byte(address))
				if parent := len(stack) - 1; parent >= 0 && !stack[parent].based {
					stack[parent].base, stack[parent].based = resolved, true
				}
			}
			out.Write(raw)
			if len(stack) == 1 && frame.name == "Period" {
				periods = append(periods, [2]int{periodStart, out.Len()})
			}
		case xml.CharData:
			if len(stack) > 0 && stack[len(stack)-1].name == "BaseURL" {
				text.Write(t)
				continue
			}
			out.Write(raw)
		default:
			out.Write(raw)
		}
	}
	rewritten := out.Bytes()
	if !dynamic || len(periods) < 2 {
		return rewritten, nil
	}
	kept := periods[len(periods)-1]
	return slices.Concat(rewritten[:periods[0][0]], rewritten[kept[0]:]), nil
}

// mpdBase reports whether a manifest's MPD element has a BaseURL.
func mpdBase(data []byte) (bool, error) {
	d := manifestDecoder(data)
	depth := 0
	for {
		token, err := d.RawToken()
		if errors.Is(err, io.EOF) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		switch t := token.(type) {
		case xml.StartElement:
			if depth == 1 && t.Name.Local == "BaseURL" {
				return true, nil
			}
			depth++
		case xml.EndElement:
			depth--
		}
	}
}

// rewriteAddress names on the loopback interface, by link, an address of a
// template or a segment list whose base is base: an absolute one, or an
// absolute path. A relative one, resolved by FFmpeg against the BaseURL,
// stays. The rest of the address, its template identifiers included, is
// kept as written.
func rewriteAddress(value string, base *url.URL, link func(*url.URL) (string, error)) (string, error) {
	address := strings.TrimSpace(value)
	origin := ""
	switch {
	case address == "":
		return value, nil
	case strings.HasPrefix(address, "//"):
		origin = base.Scheme + ":" + address
	case strings.HasPrefix(address, "/"):
		origin = base.Scheme + "://" + base.Host + address
	case hasScheme(address):
		origin = address
	default:
		return value, nil
	}
	// The scheme and host parse; what follows, a template, is kept whole.
	scheme, rest, _ := strings.Cut(origin, "://")
	host, path, _ := strings.Cut(rest, "/")
	if i := strings.IndexAny(host, "?#"); i >= 0 {
		host, path = host[:i], host[i:]
	}
	target := &url.URL{Scheme: strings.ToLower(scheme), Host: host}
	root, err := link(target)
	if err != nil {
		return "", err
	}
	return root + path, nil
}

// hasScheme reports whether an address starts with a scheme, as
// "https:" or "data:" do.
func hasScheme(address string) bool {
	i := strings.IndexAny(address, ":/?#$")
	if i <= 0 || address[i] != ':' {
		return false
	}
	for j, r := range address[:i] {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || j > 0 && (r >= '0' && r <= '9' || r == '+' || r == '-' || r == '.')) {
			return false
		}
	}
	return true
}

// writeStart writes a start tag, its names as written, its attributes'
// values escaped.
func writeStart(out *bytes.Buffer, start xml.StartElement, empty bool) {
	name := func(n xml.Name) string {
		if n.Space != "" {
			return n.Space + ":" + n.Local
		}
		return n.Local
	}
	out.WriteString("<" + name(start.Name))
	for _, a := range start.Attr {
		out.WriteString(" " + name(a.Name) + `="`)
		_ = xml.EscapeText(out, []byte(a.Value))
		out.WriteString(`"`)
	}
	if empty {
		out.WriteString("/")
	}
	out.WriteString(">")
}

// folderURL is the loopback address of an origin's root (scheme://host)
// for a live stream registered under key: what follows it is the path of
// a file at the origin, fetched with the stream's headers.
func (l *loopback) folderURL(key, origin string) string {
	return "http://" + l.listener.Addr().String() + "/live/" + key + "/~" + base64.RawURLEncoding.EncodeToString([]byte(origin)) + "/"
}

// folderTarget is the address of a file of an origin a request names on
// the loopback interface (see folderURL): the path and query that follow
// the origin's encoding, as FFmpeg asked them. It reports false for an
// address that is not of the origin.
func folderTarget(r *http.Request, key, encoded string) (string, bool) {
	origin, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(encoded, "~"))
	prefix := "/live/" + key + "/" + encoded + "/"
	rest, found := strings.CutPrefix(r.RequestURI, prefix)
	if err != nil || !found {
		return "", false
	}
	target := string(origin) + "/" + rest
	parsed, err := url.Parse(target)
	if err != nil || parsed.Scheme+"://"+parsed.Host != string(origin) {
		return "", false
	}
	return target, true
}

// drmFailure is the failure of a stream whose list names a licence that
// is not ClearKey.
func drmFailure(p *drm.Protection) error {
	return fmt.Errorf("%w: its list names a %s licence", ErrLiveDRM, p.Name())
}

// dashSupport is what ffprobe's DASH demuxer can do, asked once.
type dashSupport struct {
	once       sync.Once
	dash, keys bool
}

// dashDemuxer reports whether ffprobe, and the FFmpeg beside it, has a
// DASH demuxer, and whether it takes keys by key ID.
func (s *Service) dashDemuxer(ctx context.Context) (dash, keys bool) {
	s.dash.once.Do(func() {
		s.dash.dash, s.dash.keys = s.ffprobe().DASHKeys(context.WithoutCancel(ctx))
	})
	return s.dash.dash, s.dash.keys
}

// dashOptions are FFmpeg's and ffprobe's options for reading version's
// DASH stream, and whether they decrypt it: its ClearKey keys by key ID,
// its dash demuxer's cenc_decryption_keys, which its MP4 demuxer decrypts
// each track with (as decryption_keys); none when its list gives no
// ClearKey licence. The option is checked to be there first: a demuxer
// without it would refuse it, quoting its value.
func (s *Service) dashOptions(ctx context.Context, version library.Version) ([]string, bool, error) {
	keys, err := s.clearKeys(ctx, version)
	if err != nil || len(keys) == 0 {
		return nil, false, err
	}
	if _, takesKeys := s.dashDemuxer(ctx); !takesKeys {
		return nil, false, fmt.Errorf("%w: this server's FFmpeg cannot decrypt DASH streams", ErrLiveDead)
	}
	return []string{"-cenc_decryption_keys", drm.Option(keys)}, true, nil
}

// clearKeys returns the ClearKey keys that decrypt version's stream:
// those its list gives, else those its licence server answers for the key
// IDs its manifest names; none when its list names no ClearKey licence. A
// licence that is not ClearKey is ErrLiveDRM.
func (s *Service) clearKeys(ctx context.Context, version library.Version) ([]drm.Key, error) {
	p := version.Protection
	switch {
	case p.Unsupported():
		return nil, drmFailure(p)
	case !p.Decrypted():
		return nil, nil
	case len(p.Keys) > 0:
		return p.Keys, nil
	}
	info, err := s.dashManifest(ctx, version)
	if err != nil {
		return nil, err
	}
	if len(info.kids) == 0 {
		return nil, fmt.Errorf("%w: its manifest names no key ID to ask its licence server for", ErrLiveKeys)
	}
	return s.license(ctx, version, info.kids)
}

// dashManifest reads what version's DASH manifest says of its encryption,
// asked of its source with the headers it needs.
func (s *Service) dashManifest(ctx context.Context, version library.Version) (manifestInfo, error) {
	header := http.Header{}
	for name, value := range version.RequestHeaders() {
		header.Set(name, value)
	}
	response, err := s.opener.Open(ctx, http.MethodGet, version.URL, header, version.Confined)
	if err != nil {
		return manifestInfo{}, fmt.Errorf("%w: %v", ErrLiveTimeout, err)
	}
	defer response.Body.Close()
	if err := statusFailure(response); err != nil {
		return manifestInfo{}, err
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxPlaylist))
	if err != nil {
		return manifestInfo{}, fmt.Errorf("%w: %v", ErrLiveTimeout, err)
	}
	info, err := readManifest(data)
	if err != nil {
		return manifestInfo{}, fmt.Errorf("%w: a manifest that is no XML document", ErrLiveDead)
	}
	return info, nil
}

// readsDASH reports whether ffprobe, and the FFmpeg beside it, has a DASH
// demuxer: FFmpeg built without libxml2 has none.
func (s *Service) readsDASH(ctx context.Context) bool {
	dash, _ := s.dashDemuxer(ctx)
	return dash
}

// dashInput are FFmpeg's options for reading version's DASH stream: its
// keys (see dashOptions), and reading at the stream's own pace, after a
// burst of dashBurst for the first segments. FFmpeg's demuxer starts a
// live manifest up to a minute behind its end (dashdec.c,
// calc_cur_seg_no), then reads as fast as it can: it would reach the
// segment that does not exist yet within a second, whose source answers
// 404, and end the stream, then start again a minute behind, over and
// over. A static manifest it would read whole at once.
func (s *Service) dashInput(ctx context.Context, version library.Version) ([]string, error) {
	options, _, err := s.dashOptions(ctx, version)
	if err != nil {
		return nil, err
	}
	return append(options, "-readrate", "1", "-readrate_initial_burst", dashBurst), nil
}

// dashBurst is how much of a DASH stream, in seconds, FFmpeg reads at
// once before keeping to its pace: the first segments the app waits for
// (see hls.Manager.LivePlaylist), less than a live manifest's window,
// which lasts 30 seconds or more.
const dashBurst = "6"

// licensePoster posts a ClearKey licence request; stremio.Client does,
// within a source's confinement.
type licensePoster interface {
	Post(ctx context.Context, target string, header http.Header, body []byte, confined bool) (*http.Response, error)
}

// license asks version's ClearKey licence server for the keys of kids: a
// W3C Encrypted Media Extensions Clear Key licence request (9.1.3), posted
// with the stream's headers and those its list gives the licence server,
// answered with a JSON Web Key set (9.1.4). Its address, which may hold a
// token, is never told.
func (s *Service) license(ctx context.Context, version library.Version, kids []string) ([]drm.Key, error) {
	poster, ok := s.opener.(licensePoster)
	if !ok {
		return nil, fmt.Errorf("%w: its licence server cannot be asked", ErrLiveKeys)
	}
	header := http.Header{}
	for name, value := range version.RequestHeaders() {
		header.Set(name, value)
	}
	for name, value := range version.Protection.LicenseHeaders {
		header.Set(name, value)
	}
	header.Set("Content-Type", "application/json")
	ctx, cancel := context.WithTimeout(ctx, licenseWait)
	defer cancel()
	response, err := poster.Post(ctx, version.Protection.LicenseURL, header, drm.LicenseRequest(kids), version.Confined)
	if err != nil {
		return nil, fmt.Errorf("%w: its ClearKey licence server: %v", ErrLiveTimeout, err)
	}
	defer response.Body.Close()
	if err := statusFailure(response); err != nil {
		return nil, fmt.Errorf("its ClearKey licence server: %w", err)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxLicense))
	if err != nil {
		return nil, fmt.Errorf("%w: its ClearKey licence server: %v", ErrLiveTimeout, err)
	}
	keys, err := drm.ParseLicense(data)
	if err != nil {
		return nil, fmt.Errorf("%w: its ClearKey licence server: %w", ErrLiveKeys, err)
	}
	return keys, nil
}

// checkDASH tells a DASH stream ffprobe read that would play wrong:
// encrypted while its list gives no keys (ErrLiveDRM), or whose keys do
// not decrypt its video (ErrLiveKeys), whose first packet then decodes
// into no frame. options are those it was read with, keyed when they
// decrypt it, target where.
func (s *Service) checkDASH(ctx context.Context, version library.Version, analysis media.Analysis, target string, options []string, keyed bool) error {
	if !keyed {
		info, err := s.dashManifest(ctx, version)
		if err != nil {
			return err
		}
		if len(info.systems) == 0 {
			return nil
		}
		if named := info.named(); named != "" {
			return fmt.Errorf("%w: it is encrypted (%s), and its list gives no ClearKey keys", ErrLiveDRM, named)
		}
		return fmt.Errorf("%w: it is encrypted, and its list gives no ClearKey keys", ErrLiveDRM)
	}
	video, ok := LiveVideo(analysis)
	if !ok {
		return nil
	}
	decodes, err := s.ffprobe().Decodes(ctx, media.Input{Path: target, Options: options}, video.Index)
	if err != nil {
		return err
	}
	if !decodes {
		return ErrLiveKeys
	}
	return nil
}
