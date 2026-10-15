package dlna

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/playback"
)

const (
	// Prefix is where the routes of DLNA live on the HTTP port.
	Prefix          = "/dlna/"
	descriptionPath = Prefix + "description.xml"
	// maxControl bounds the body of a SOAP request.
	maxControl = 64 << 10
	// artWidth is the width posters are sent at.
	artWidth = 320
)

// Handler serves the routes under /dlna/: the description, the services'
// descriptions, their control, the events devices subscribe to, and the
// media. They answer 404 while sharing is off, and 403 to any address
// outside the local network and to a blocked device.
func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+descriptionPath, s.description)
	mux.HandleFunc("GET "+Prefix+"ContentDirectory.xml", s.scpd(contentDirectorySCPD))
	mux.HandleFunc("GET "+Prefix+"ConnectionManager.xml", s.scpd(connectionManagerSCPD))
	mux.HandleFunc("POST "+Prefix+"control/ContentDirectory", s.control(s.contentDirectoryAction))
	mux.HandleFunc("POST "+Prefix+"control/ConnectionManager", s.control(s.connectionManagerAction))
	mux.HandleFunc(Prefix+"event/{service}", s.event)
	mux.HandleFunc("GET "+Prefix+"media/{file}", s.media)
	mux.HandleFunc("HEAD "+Prefix+"media/{file}", s.media)
	mux.HandleFunc("GET "+Prefix+"captions/{file}", s.captions)
	mux.HandleFunc("GET "+Prefix+"art/{file}", s.art)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.options.Settings().DLNA {
			http.NotFound(w, r)
			return
		}
		c, err := s.clientOf(r)
		switch {
		case errors.Is(err, errRefused):
			w.WriteHeader(http.StatusForbidden)
			return
		case err != nil:
			if r.Context().Err() == nil {
				s.options.Logger.Warn("A DLNA device could not be recorded", "error", err)
			}
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Server", s.serverHeader())
		mux.ServeHTTP(w, r.WithContext(withClient(r.Context(), c)))
	})
}

// description answers the device description.
func (s *Service) description(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", `text/xml; charset="utf-8"`)
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="utf-8"?>` + "\n")
	b.WriteString(`<root xmlns="urn:schemas-upnp-org:device-1-0" xmlns:dlna="urn:schemas-dlna-org:device-1-0">`)
	b.WriteString(`<specVersion><major>1</major><minor>0</minor></specVersion><device>`)
	b.WriteString(`<dlna:X_DLNADOC>DMS-1.50</dlna:X_DLNADOC>`)
	b.WriteString(`<deviceType>` + deviceType + `</deviceType>`)
	writeElement(&b, "friendlyName", s.name())
	b.WriteString(`<manufacturer>Polyfin</manufacturer><manufacturerURL>https://github.com/moodiness/polyfin</manufacturerURL>`)
	b.WriteString(`<modelDescription>Polyfin media server</modelDescription><modelName>Polyfin</modelName>`)
	writeElement(&b, "modelNumber", s.options.Version)
	b.WriteString(`<modelURL>https://github.com/moodiness/polyfin</modelURL>`)
	writeElement(&b, "serialNumber", s.options.ServerID)
	writeElement(&b, "UDN", "uuid:"+s.udn())
	b.WriteString(`<serviceList>`)
	for _, service := range []struct{ kind, id, name string }{
		{contentDirectory, "urn:upnp-org:serviceId:ContentDirectory", "ContentDirectory"},
		{connectionManager, "urn:upnp-org:serviceId:ConnectionManager", "ConnectionManager"},
	} {
		fmt.Fprintf(&b, `<service><serviceType>%s</serviceType><serviceId>%s</serviceId><SCPDURL>%s%s.xml</SCPDURL>`+
			`<controlURL>%scontrol/%s</controlURL><eventSubURL>%sevent/%s</eventSubURL></service>`,
			service.kind, service.id, Prefix, service.name, Prefix, service.name, Prefix, service.name)
	}
	b.WriteString(`</serviceList></device></root>`)
	_, _ = io.WriteString(w, b.String())
}

// writeElement writes an element holding text, escaped.
func writeElement(b *strings.Builder, name, text string) {
	b.WriteString("<" + name + ">")
	_ = xml.EscapeText(b, []byte(text))
	b.WriteString("</" + name + ">")
}

// scpd answers a service's description.
func (s *Service) scpd(document string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", `text/xml; charset="utf-8"`)
		_, _ = io.WriteString(w, document)
	}
}

// event answers the subscriptions to a service's events. The services
// send none: a subscription is accepted, so that devices that ask for one
// go on, and renewed as they wish.
func (s *Service) event(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "SUBSCRIBE":
		sid := r.Header.Get("Sid")
		if sid == "" {
			sid = "uuid:" + randomUUID()
		}
		w.Header().Set("Sid", sid)
		w.Header().Set("Timeout", "Second-1800")
		w.WriteHeader(http.StatusOK)
	case "UNSUBSCRIBE":
		w.WriteHeader(http.StatusOK)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// soapAction is a SOAP request: its action and arguments.
type soapAction struct {
	name string
	args map[string]string
}

// upnpError is a UPnP error a control answers with.
type upnpError struct {
	code        int
	description string
}

var (
	errInvalidAction = upnpError{401, "Invalid Action"}
	errInvalidArgs   = upnpError{402, "Invalid Args"}
	errNoSuchObject  = upnpError{701, "No such object"}
	errActionFailed  = upnpError{501, "Action Failed"}
)

// readAction reads a SOAP request's action and arguments.
func readAction(r io.Reader) (soapAction, error) {
	decoder := xml.NewDecoder(io.LimitReader(r, maxControl))
	action := soapAction{args: map[string]string{}}
	depth, inBody := 0, false
	var current string
	var text strings.Builder
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return action, err
		}
		switch t := token.(type) {
		case xml.StartElement:
			depth++
			switch {
			case t.Name.Local == "Body" && depth == 2:
				inBody = true
			case inBody && depth == 3:
				action.name = t.Name.Local
			case inBody && depth == 4:
				current = t.Name.Local
				text.Reset()
			}
		case xml.CharData:
			if current != "" {
				text.Write(t)
			}
		case xml.EndElement:
			if inBody && depth == 4 && current != "" {
				action.args[current] = text.String()
				current = ""
			}
			if depth == 2 && t.Name.Local == "Body" {
				inBody = false
			}
			depth--
		}
	}
	if action.name == "" {
		return action, errors.New("no action")
	}
	return action, nil
}

// control answers a service's SOAP requests with handle.
func (s *Service) control(handle func(r *http.Request, action soapAction) ([][2]string, *upnpError)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		action, err := readAction(r.Body)
		w.Header().Set("Content-Type", `text/xml; charset="utf-8"`)
		w.Header().Set("Ext", "")
		if err != nil {
			writeFault(w, errInvalidAction)
			return
		}
		service := contentDirectory
		if strings.HasSuffix(r.URL.Path, "ConnectionManager") {
			service = connectionManager
		}
		out, failure := handle(r, action)
		if failure != nil {
			writeFault(w, *failure)
			return
		}
		var b strings.Builder
		b.WriteString(`<?xml version="1.0" encoding="utf-8"?>` + "\n")
		b.WriteString(`<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/"><s:Body>`)
		fmt.Fprintf(&b, `<u:%sResponse xmlns:u="%s">`, action.name, service)
		for _, arg := range out {
			writeElement(&b, arg[0], arg[1])
		}
		fmt.Fprintf(&b, `</u:%sResponse></s:Body></s:Envelope>`, action.name)
		_, _ = io.WriteString(w, b.String())
	}
}

// writeFault answers a SOAP request with a UPnP error.
func writeFault(w http.ResponseWriter, failure upnpError) {
	w.WriteHeader(http.StatusInternalServerError)
	fmt.Fprintf(w, `<?xml version="1.0" encoding="utf-8"?>`+"\n"+
		`<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/"><s:Body>`+
		`<s:Fault><faultcode>s:Client</faultcode><faultstring>UPnPError</faultstring><detail>`+
		`<UPnPError xmlns="urn:schemas-upnp-org:control-1-0"><errorCode>%d</errorCode><errorDescription>%s</errorDescription></UPnPError>`+
		`</detail></s:Fault></s:Body></s:Envelope>`, failure.code, failure.description)
}

// connectionManagerAction answers ConnectionManager: the protocols the
// server sends, and its one connection.
func (s *Service) connectionManagerAction(_ *http.Request, action soapAction) ([][2]string, *upnpError) {
	switch action.name {
	case "GetProtocolInfo":
		return [][2]string{{"Source", sourceProtocols}, {"Sink", ""}}, nil
	case "GetCurrentConnectionIDs":
		return [][2]string{{"ConnectionIDs", "0"}}, nil
	case "GetCurrentConnectionInfo":
		if action.args["ConnectionID"] != "0" {
			return nil, &upnpError{706, "Invalid connection reference"}
		}
		return [][2]string{{"RcsID", "-1"}, {"AVTransportID", "-1"}, {"ProtocolInfo", ""}, {"PeerConnectionManager", ""},
			{"PeerConnectionID", "-1"}, {"Direction", "Output"}, {"Status", "OK"}}, nil
	}
	return nil, &errInvalidAction
}

// sourceProtocols are the protocols the server sends.
const sourceProtocols = "http-get:*:video/mpeg:*,http-get:*:video/mp4:*,http-get:*:video/x-matroska:*,http-get:*:video/x-msvideo:*," +
	"http-get:*:audio/mpeg:*,http-get:*:audio/flac:*,http-get:*:audio/mp4:*,http-get:*:image/jpeg:*,http-get:*:text/srt:*"

// mediaFile is a media address's file name: an item's ID and an
// extension.
var mediaFile = regexp.MustCompile(`^([0-9a-fA-F-]{32,36})\.([a-z0-9]{1,5})$`)

// played reads the play a media request is for: its item, signed for the
// device's user.
func (s *Service) played(w http.ResponseWriter, r *http.Request) (Play, bool) {
	c := clientFrom(r.Context())
	match := mediaFile.FindStringSubmatch(r.PathValue("file"))
	if match == nil {
		w.WriteHeader(http.StatusNotFound)
		return Play{}, false
	}
	id, err := accounts.ParseID(match[1])
	grant, grantErr := s.options.Signer.Verify(r.URL.Query().Get("g"))
	// The address is signed for the device's user: one given another user
	// since gets nothing.
	if err != nil || grantErr != nil || grant.Version != id || !c.hasUser || grant.User != c.user.ID {
		w.WriteHeader(http.StatusForbidden)
		return Play{}, false
	}
	item, err := s.options.Library.Item(r.Context(), c.user, id)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return Play{}, false
	}
	play := Play{Device: c.device, User: c.user, Item: item, Profile: c.profile, Converted: r.URL.Query().Get("c") == "1"}
	if raw := r.Header.Get("TimeSeekRange.dlna.org"); raw != "" {
		start, ok := parseTimeSeek(raw)
		if !ok {
			w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			return Play{}, false
		}
		play.Start, play.Seek = start, true
	}
	return play, true
}

// media serves an item's bytes to a device.
func (s *Service) media(w http.ResponseWriter, r *http.Request) {
	play, ok := s.played(w, r)
	if !ok {
		return
	}
	header := w.Header()
	header.Set("TransferMode.DLNA.ORG", "Streaming")
	if r.Header.Get("getCaptionInfo.sec") == "1" && play.Profile.TakesCaptions() {
		header.Set("CaptionInfo.sec", mediaURL("http://"+r.Host, "captions", play.User.ID, play.Item.ID, "srt", s, false))
	}
	s.options.Player.Play(w, r, play)
}

// captions serves the text subtitle of an item.
func (s *Service) captions(w http.ResponseWriter, r *http.Request) {
	play, ok := s.played(w, r)
	if !ok {
		return
	}
	s.options.Player.Captions(w, r, play)
}

// art serves an item's poster as a JPEG, to a device that shows a user's
// libraries.
func (s *Service) art(w http.ResponseWriter, r *http.Request) {
	c := clientFrom(r.Context())
	name, ok := strings.CutSuffix(r.PathValue("file"), ".jpg")
	id, err := accounts.ParseID(name)
	if !ok || err != nil || !c.hasUser {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	s.options.Player.Art(w, r, id, artWidth)
}

// parseTimeSeek reads the start of a TimeSeekRange.dlna.org header, as in
// npt=10.5- or npt=0:01:02.500-0:02:00.
func parseTimeSeek(header string) (time.Duration, bool) {
	value, ok := strings.CutPrefix(strings.TrimSpace(header), "npt=")
	if !ok {
		return 0, false
	}
	start, _, _ := strings.Cut(value, "-")
	return parseNPT(strings.TrimSpace(start))
}

// parseNPT reads a normal play time: seconds, or H:MM:SS, with a fraction.
func parseNPT(text string) (time.Duration, bool) {
	parts := strings.Split(text, ":")
	if len(parts) > 3 || text == "" {
		return 0, false
	}
	var total float64
	for _, part := range parts {
		value, err := strconv.ParseFloat(part, 64)
		if err != nil || value < 0 {
			return 0, false
		}
		total = total*60 + value
	}
	return time.Duration(total * float64(time.Second)), true
}

// NPT writes a duration as a normal play time, H:MM:SS.mmm.
func NPT(d time.Duration) string {
	d = max(d, 0)
	ms := d.Milliseconds()
	return fmt.Sprintf("%d:%02d:%02d.%03d", ms/3_600_000, ms/60_000%60, ms/1000%60, ms%1000)
}

// FileFlags are the DLNA flags of a file sent as it is, which takes byte
// ranges; ConvertedFlags those of a conversion, which takes time seeks.
const (
	FileFlags      = "DLNA.ORG_OP=01;DLNA.ORG_CI=0;DLNA.ORG_FLAGS=01700000000000000000000000000000"
	ConvertedFlags = "DLNA.ORG_OP=10;DLNA.ORG_CI=1;DLNA.ORG_FLAGS=01300000000000000000000000000000"
)

// ContentFeatures is the contentFeatures.dlna.org of a resource.
func ContentFeatures(resource Resource) string {
	flags := FileFlags
	if resource.Converted {
		flags = ConvertedFlags
	}
	if resource.OrgPn != "" {
		return "DLNA.ORG_PN=" + resource.OrgPn + ";" + flags
	}
	return flags
}

// grantFor signs a media address for user: the grant names the item, and
// tells Polyfin to relay the bytes, as devices may not follow redirects.
func grantFor(user, item accounts.ID) playback.Grant {
	return playback.Grant{Version: item, User: user, Relay: true}
}

// xmlEscape writes text escaped for XML.
func xmlEscape(b *strings.Builder, text string) error {
	return xml.EscapeText(b, []byte(text))
}
