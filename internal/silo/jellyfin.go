package silo

import (
	"bytes"
	"cmp"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/playback"
)

// The apps' sessions are Polyfin devices, of this app when the app does
// not name itself.
const appName = "Silo"

// answer is what the Jellyfin API's handler answered a call.
type answer struct {
	status int
	header http.Header
	body   []byte
}

func (a answer) ok() bool { return a.status >= 200 && a.status < 300 }

// recorder keeps an answer of the Jellyfin API's handler.
type recorder struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (r *recorder) Header() http.Header { return r.header }

func (r *recorder) WriteHeader(status int) {
	if r.status == 0 {
		r.status = status
	}
}

func (r *recorder) Write(p []byte) (int, error) {
	r.WriteHeader(http.StatusOK)
	return r.body.Write(p)
}

// authorization is the Jellyfin Authorization header of a device, with its
// token when signed in.
func authorization(device accounts.DeviceInfo, token string) string {
	quote := func(value string) string { return `"` + url.QueryEscape(value) + `"` }
	header := "MediaBrowser Client=" + quote(device.Client) + ", Device=" + quote(device.DeviceName) +
		", DeviceId=" + quote(device.DeviceID) + ", Version=" + quote(device.ClientVersion)
	if token != "" {
		header += ", Token=" + quote(token)
	}
	return header
}

// jellyfinRequest is r made into a request of the Jellyfin API: the same
// context, client address and host, the device's credentials, and only
// the headers named in keep.
func jellyfinRequest(r *http.Request, method, target string, auth string, body []byte, keep ...string) *http.Request {
	var reader io.Reader = http.NoBody
	if body != nil {
		reader = bytes.NewReader(body)
	}
	path, query, _ := strings.Cut(target, "?")
	request, err := http.NewRequestWithContext(r.Context(), method, "/", reader)
	if err != nil {
		// The method is one of the API's own, which is valid.
		panic(err)
	}
	request.URL = &url.URL{Path: path, RawQuery: query}
	request.Host, request.RemoteAddr, request.TLS = r.Host, r.RemoteAddr, r.TLS
	request.Proto, request.ProtoMajor, request.ProtoMinor = r.Proto, r.ProtoMajor, r.ProtoMinor
	for _, name := range keep {
		if value := r.Header.Values(name); len(value) > 0 {
			request.Header[http.CanonicalHeaderKey(name)] = value
		}
	}
	request.Header.Set("Authorization", auth)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	return request
}

// call asks the Jellyfin API's handler for target as v's session, with
// body as JSON when not nil, and decodes a successful answer into out
// when not nil.
func (s *Server) call(r *http.Request, v viewer, method, target string, body, out any) (answer, error) {
	return s.callAs(r, v.device.DeviceInfo, v.token, method, target, body, out)
}

func (s *Server) callAs(r *http.Request, device accounts.DeviceInfo, token, method, target string, body, out any) (answer, error) {
	var data []byte
	if body != nil {
		var err error
		if data, err = json.Marshal(body); err != nil {
			return answer{}, err
		}
	}
	rec := &recorder{header: http.Header{}}
	s.options.Jellyfin.ServeHTTP(rec, jellyfinRequest(r, method, target, authorization(device, token), data, "Accept-Language"))
	a := answer{status: cmp.Or(rec.status, http.StatusOK), header: rec.header, body: rec.body.Bytes()}
	if a.ok() && out != nil && len(a.body) > 0 {
		if err := json.Unmarshal(a.body, out); err != nil {
			return a, err
		}
	}
	return a, nil
}

// relay serves the Jellyfin API's answer for target to the app, as it
// is: media and artwork.
func (s *Server) relay(w http.ResponseWriter, r *http.Request, target, auth string) {
	s.options.Jellyfin.ServeHTTP(w, jellyfinRequest(r, r.Method, target, auth, nil,
		"Range", "If-Range", "If-None-Match", "If-Modified-Since", "Accept-Encoding", "Accept-Language"))
}

// The Jellyfin API's descriptions, of which the apps' answers are made:
// the fields read, under Jellyfin's names.

type queryResult struct {
	Items            []item
	TotalRecordCount int
}

type item struct {
	Id                      string
	Name                    string
	OriginalTitle           string
	Type                    string
	CollectionType          string
	Overview                *string
	Taglines                []string
	Genres                  []string
	Studios                 []struct{ Name string }
	OfficialRating          string
	CommunityRating         *float64
	CriticRating            *float64
	RunTimeTicks            *int64
	ProductionYear          *int
	PremiereDate            *time.Time
	EndDate                 *time.Time
	DateCreated             *time.Time
	IndexNumber             *int
	ParentIndexNumber       *int
	SeriesId                string
	SeriesName              string
	SeasonId                string
	Status                  string
	ProviderIds             map[string]string
	People                  []person
	UserData                *userData
	ChildCount              *int
	RecursiveItemCount      *int
	ImageTags               map[string]string
	BackdropImageTags       []string
	ParentBackdropItemId    string
	ParentBackdropImageTags []string
	ParentLogoItemId        string
	ParentLogoImageTag      string
	SeriesPrimaryImageTag   string
	MediaSources            []mediaSource
}

type person struct {
	Name, Id, Role, Type, PrimaryImageTag string
}

type userData struct {
	PlaybackPositionTicks int64
	PlayCount             int
	IsFavorite            bool
	Played                bool
	UnplayedItemCount     *int
	LastPlayedDate        *time.Time
}

type mediaSource struct {
	Id                         string
	Name                       string
	Container                  string
	Size                       *int64
	Bitrate                    *int64
	RunTimeTicks               *int64
	SupportsDirectPlay         bool
	SupportsDirectStream       bool
	SupportsTranscoding        bool
	MediaStreams               []playback.MediaStream
	TranscodingUrl             string
	TranscodingSubProtocol     string
	TranscodingContainer       string
	DefaultAudioStreamIndex    *int
	DefaultSubtitleStreamIndex *int
}

type userDto struct {
	Id     string
	Name   string
	Policy struct {
		IsAdministrator          bool
		EnableContentDownloading bool
		MaxParentalRating        *int
		EnableAllFolders         bool
		EnabledFolders           []string
	}
	Configuration struct {
		AudioLanguagePreference    string
		SubtitleLanguagePreference string
		SubtitleMode               string
	}
}
