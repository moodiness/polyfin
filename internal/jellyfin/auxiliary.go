package jellyfin

import (
	"bytes"
	"crypto/md5"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	mathrand "math/rand/v2"
	"mime"
	"net/http"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
)

// auxiliaryRoutes registers the endpoints Jellyfin apps call around
// browsing: display preferences, network and bitrate probes, and lists
// Polyfin has no content for yet, which answer like a Jellyfin server
// without such content.
func (h *Handler) auxiliaryRoutes(rt *router) {
	signedIn := func(method, pattern string, handler http.HandlerFunc) {
		rt.handle(method, pattern, h.authenticated(handler))
	}
	signedIn(http.MethodGet, "/DisplayPreferences/{displayPreferencesId}", h.displayPreferences)
	signedIn(http.MethodPost, "/DisplayPreferences/{displayPreferencesId}", h.updateDisplayPreferences)
	signedIn(http.MethodGet, "/System/Endpoint", h.endpointInfo)
	signedIn(http.MethodGet, "/Playback/BitrateTest", h.bitrateTest)

	// Polyfin has no studio list or artists.
	signedIn(http.MethodGet, "/Studios", h.emptyQueryResult)
	signedIn(http.MethodGet, "/Artists", h.emptyQueryResult)
	// Jellyfin answers similar titles under each of these routes.
	for _, similar := range []string{"/Items/{itemId}/Similar", "/Movies/{itemId}/Similar", "/Shows/{itemId}/Similar", "/Trailers/{itemId}/Similar"} {
		signedIn(http.MethodGet, similar, h.similarItems)
	}
	signedIn(http.MethodGet, "/Items/{itemId}/Collections", h.itemCollections)
	// Addons give trailers, which apps get as RemoteTrailers, and no other
	// extras: no local trailers, special features or theme media.
	signedIn(http.MethodGet, "/Items/{itemId}/SpecialFeatures", h.itemExtras)
	signedIn(http.MethodGet, "/Items/{itemId}/LocalTrailers", h.itemExtras)
	signedIn(http.MethodGet, "/Items/{itemId}/ThemeMedia", h.themeMedia)
	signedIn(http.MethodGet, "/Items/{itemId}/Intros", h.intros)
	signedIn(http.MethodGet, "/Users/{userId}/Items/{itemId}/Intros", h.intros)
}

// problemDetails is the RFC 9457 body ASP.NET answers with when it rejects
// a request before reaching Jellyfin's code.
type problemDetails struct {
	Type    string              `json:"type"`
	Title   string              `json:"title"`
	Status  int                 `json:"status"`
	Errors  map[string][]string `json:"errors,omitempty"`
	TraceID string              `json:"traceId"`
}

// traceID returns a W3C trace context identifier, as ASP.NET puts in its
// problem details.
func traceID() string {
	var raw [24]byte
	_, _ = rand.Read(raw[:])
	return "00-" + hex.EncodeToString(raw[:16]) + "-" + hex.EncodeToString(raw[16:]) + "-00"
}

// validationProblem answers like ASP.NET when request parameters or the
// body do not bind: errors maps each parameter (or JSON path) to messages.
func validationProblem(w http.ResponseWriter, errors map[string][]string) {
	writeJSON(w, http.StatusBadRequest, problemDetails{
		Type:    "https://tools.ietf.org/html/rfc9110#section-15.5.1",
		Title:   "One or more validation errors occurred.",
		Status:  http.StatusBadRequest,
		Errors:  errors,
		TraceID: traceID(),
	})
}

// notFoundProblem answers like Jellyfin for an unknown item or user.
func notFoundProblem(w http.ResponseWriter) {
	writeJSON(w, http.StatusNotFound, problemDetails{
		Type:    "https://tools.ietf.org/html/rfc9110#section-15.5.5",
		Title:   "Not Found",
		Status:  http.StatusNotFound,
		TraceID: traceID(),
	})
}

func unsupportedMediaTypeProblem(w http.ResponseWriter) {
	writeJSON(w, http.StatusUnsupportedMediaType, problemDetails{
		Type:    "https://tools.ietf.org/html/rfc9110#section-15.5.16",
		Title:   "Unsupported Media Type",
		Status:  http.StatusUnsupportedMediaType,
		TraceID: traceID(),
	})
}

// queryParam returns the first value of a query parameter matched without
// regard to case, and whether the parameter was sent at all.
func queryParam(r *http.Request, name string) (string, bool) {
	values := r.URL.Query()
	if value, ok := values[name]; ok && len(value) > 0 {
		return value[0], true
	}
	for key, value := range values {
		if strings.EqualFold(key, name) && len(value) > 0 {
			return value[0], true
		}
	}
	return "", false
}

// bindErrors collects the errors of binding optional query parameters the
// way ASP.NET does: a missing, empty or blank value leaves the parameter
// unset; a value that does not convert is an error named after the
// parameter.
type bindErrors map[string][]string

func (b bindErrors) add(name, message string) {
	b[name] = append(b[name], message)
}

// int32 binds an optional integer. Like .NET's Int32Converter it accepts
// surrounding blanks, a sign, and hexadecimal after #, 0x or &h.
func (b bindErrors) int32(r *http.Request, name string) (int, bool) {
	raw, _ := queryParam(r, name)
	if strings.TrimSpace(raw) == "" {
		return 0, false
	}
	value, message := convertInt32(raw)
	if message != "" {
		b.add(name, message)
		return 0, false
	}
	return value, true
}

// guid binds an optional identifier, in any of the forms .NET parses. The
// empty identifier counts as unset, as Jellyfin treats it.
func (b bindErrors) guid(r *http.Request, name string) (accounts.ID, bool) {
	raw, _ := queryParam(r, name)
	if strings.TrimSpace(raw) == "" {
		return accounts.ID{}, false
	}
	id, ok := parseGUID(raw)
	if !ok {
		b.add(name, notValid(raw))
		return accounts.ID{}, false
	}
	return id, id != accounts.ID{}
}

// bool binds an optional boolean: true or false in any letter case.
func (b bindErrors) bool(r *http.Request, name string) (value, set bool) {
	raw, _ := queryParam(r, name)
	if strings.TrimSpace(raw) == "" {
		return false, false
	}
	value, ok := parseDotNetBool(raw)
	if !ok {
		b.add(name, notValid(raw))
		return false, false
	}
	return value, true
}

func notValid(raw string) string {
	return fmt.Sprintf("The value '%s' is not valid.", raw)
}

// convertInt32 converts a query value as .NET's Int32Converter does and
// returns ASP.NET's message when it cannot.
func convertInt32(raw string) (int, string) {
	text := strings.TrimSpace(raw)
	digits, hexadecimal := text, false
	for _, prefix := range []string{"#", "0x", "0X", "&h", "&H"} {
		if rest, ok := strings.CutPrefix(text, prefix); ok {
			digits, hexadecimal = rest, true
			break
		}
	}
	if hexadecimal {
		digits = strings.TrimSpace(digits)
		if digits == "" {
			return 0, "The input was not valid."
		}
		trimmed := strings.TrimLeft(digits, "0")
		value, err := strconv.ParseUint(digits, 16, 64)
		if err != nil || len(trimmed) > 8 {
			return 0, notValid(raw)
		}
		return int(int32(uint32(value))), ""
	}
	value, ok := parseDotNetInt(text)
	if !ok {
		return 0, notValid(raw)
	}
	return value, ""
}

// parseDotNetInt parses an Int32 like .NET's int.Parse: surrounding
// blanks and one leading sign are allowed.
func parseDotNetInt(text string) (int, bool) {
	text = strings.TrimSpace(text)
	digits := strings.TrimLeft(text, "+-")
	if len(text)-len(digits) > 1 || digits == "" || strings.Trim(digits, "0123456789") != "" {
		return 0, false
	}
	value, err := strconv.ParseInt(text, 10, 32)
	if err != nil {
		return 0, false
	}
	return int(value), true
}

// parseDotNetBool parses like .NET's bool.Parse.
func parseDotNetBool(text string) (bool, bool) {
	switch strings.ToLower(strings.Trim(text, " \t\n\v\f\r\x00")) {
	case "true":
		return true, true
	case "false":
		return false, true
	}
	return false, false
}

// parseGUID accepts the identifier forms of .NET's Guid.Parse that apps
// send: 32 digits, hyphenated, and hyphenated within braces or
// parentheses, with surrounding blanks.
func parseGUID(value string) (accounts.ID, bool) {
	value = strings.TrimSpace(value)
	if len(value) == 38 && (value[0] == '{' && value[37] == '}' || value[0] == '(' && value[37] == ')') {
		value = value[1:37]
	}
	id, err := accounts.ParseID(value)
	return id, err == nil
}

// hyphenated formats an identifier as .NET's Guid.ToString() does.
func hyphenated(id accounts.ID) string {
	s := id.String()
	return s[:8] + "-" + s[8:12] + "-" + s[12:16] + "-" + s[16:20] + "-" + s[20:]
}

// targetUser resolves the userId query parameter Jellyfin endpoints accept
// in place of the caller. Only administrators may act for another user; an
// unknown user is answered by unknown. ok is false once w was answered.
func (h *Handler) targetUser(w http.ResponseWriter, r *http.Request, id accounts.ID, set bool,
	unknown func(http.ResponseWriter)) (accounts.User, bool) {
	caller := callerFrom(r.Context()).User
	if !set || id == caller.ID {
		return caller, true
	}
	if !caller.IsAdministrator {
		processingError(w, http.StatusForbidden)
		return accounts.User{}, false
	}
	user, err := h.Accounts.User(r.Context(), id)
	if errors.Is(err, accounts.ErrNotFound) {
		unknown(w)
		return accounts.User{}, false
	}
	if err != nil {
		h.internalError(w, r, err)
		return accounts.User{}, false
	}
	return user, true
}

// Display preferences.
//
// Jellyfin keeps, per user, preference id and client, a fixed set of view
// settings, a few app settings carried in CustomPrefs, the sections of the
// home screen, and any other CustomPrefs entries. Polyfin stores the same
// state, normalised, as JSON.

var (
	scrollDirections   = []string{"Horizontal", "Vertical"}
	sortOrders         = []string{"Ascending", "Descending"}
	indexingKinds      = []string{"PremiereDate", "ProductionYear", "CommunityRating"}
	chromecastVersions = []string{"Stable", "Unstable"}
	homeSectionTypes   = []string{"None", "SmallLibraryTiles", "LibraryButtons", "ActiveRecordings", "Resume",
		"ResumeAudio", "LatestMedia", "NextUp", "LiveTv", "ResumeBook"}
	// defaultHomeSections are the sections, by position, that an
	// unrecognised home section value falls back to; later positions are
	// None.
	defaultHomeSections = []int{1, 4, 5, 9, 8, 7, 6}
)

const homeSectionPrefix = "homesection"

type displayState struct {
	SortBy                     string
	IndexBy                    *string
	RememberIndexing           bool
	ScrollDirection            int
	ShowBackdrop               bool
	RememberSorting            bool
	SortOrder                  int
	ShowSidebar                bool
	HomeSections               []homeSection
	ChromecastVersion          int
	SkipForwardLength          int
	SkipBackLength             int
	EnableNextVideoInfoOverlay bool
	TvHome                     *string
	DashboardTheme             *string
	Custom                     map[string]*string
}

type homeSection struct {
	Index int
	Type  int
}

// unsavedDisplayState is what Jellyfin answers for preferences never saved.
func unsavedDisplayState() displayState {
	return displayState{
		SortBy:            "SortName",
		ShowBackdrop:      true,
		SkipForwardLength: 30000,
		SkipBackLength:    10000,
	}
}

type displayPreferencesDto struct {
	Id                 string
	SortBy             string
	IndexBy            *string `json:",omitempty"`
	RememberIndexing   bool
	PrimaryImageHeight int
	PrimaryImageWidth  int
	CustomPrefs        orderedPrefs
	ScrollDirection    json.RawMessage
	ShowBackdrop       bool
	RememberSorting    bool
	SortOrder          json.RawMessage
	ShowSidebar        bool
	Client             string
}

type orderedPrefs []pref

type pref struct {
	key   string
	value *string
}

func (p orderedPrefs) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, entry := range p {
		if i > 0 {
			b.WriteByte(',')
		}
		key, _ := json.Marshal(entry.key)
		value, _ := json.Marshal(entry.value)
		b.Write(key)
		b.WriteByte(':')
		b.Write(value)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

// enumName names an enum value as .NET formats it: its name when defined,
// its number otherwise.
func enumName(value int, names []string) string {
	if value >= 0 && value < len(names) {
		return names[value]
	}
	return strconv.Itoa(value)
}

func enumJSON(value int, names []string) json.RawMessage {
	if value >= 0 && value < len(names) {
		encoded, _ := json.Marshal(names[value])
		return encoded
	}
	return json.RawMessage(strconv.Itoa(value))
}

// parseEnum parses like .NET's Enum.TryParse ignoring case: a number, or
// names separated by commas, combined.
func parseEnum(text string, names []string) (int, bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0, false
	}
	if c := text[0]; c >= '0' && c <= '9' || c == '-' || c == '+' {
		return parseDotNetInt(text)
	}
	combined := 0
	for part := range strings.SplitSeq(text, ",") {
		index := slices.IndexFunc(names, func(name string) bool { return strings.EqualFold(name, strings.TrimSpace(part)) })
		if index < 0 {
			return 0, false
		}
		combined |= index
	}
	return combined, true
}

func (s displayState) dto(id, client string) displayPreferencesDto {
	prefs := orderedPrefs{}
	for _, section := range s.HomeSections {
		value := strings.ToLower(enumName(section.Type, homeSectionTypes))
		prefs = append(prefs, pref{homeSectionPrefix + strconv.Itoa(section.Index), &value})
	}
	chromecast := strings.ToLower(enumName(s.ChromecastVersion, chromecastVersions))
	skipForward := strconv.Itoa(s.SkipForwardLength)
	skipBack := strconv.Itoa(s.SkipBackLength)
	overlay := "False"
	if s.EnableNextVideoInfoOverlay {
		overlay = "True"
	}
	prefs = append(prefs,
		pref{"chromecastVersion", &chromecast},
		pref{"skipForwardLength", &skipForward},
		pref{"skipBackLength", &skipBack},
		pref{"enableNextVideoInfoOverlay", &overlay},
		pref{"tvhome", s.TvHome},
		pref{"dashboardTheme", s.DashboardTheme},
	)
	keys := make([]string, 0, len(s.Custom))
	for key := range s.Custom {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		prefs = append(prefs, pref{key, s.Custom[key]})
	}
	return displayPreferencesDto{
		Id:                 id,
		SortBy:             s.SortBy,
		IndexBy:            s.IndexBy,
		RememberIndexing:   s.RememberIndexing,
		PrimaryImageHeight: 250,
		PrimaryImageWidth:  250,
		CustomPrefs:        prefs,
		ScrollDirection:    enumJSON(s.ScrollDirection, scrollDirections),
		ShowBackdrop:       s.ShowBackdrop,
		RememberSorting:    s.RememberSorting,
		SortOrder:          enumJSON(s.SortOrder, sortOrders),
		ShowSidebar:        s.ShowSidebar,
		Client:             client,
	}
}

// displayPreferencesID is the identifier Jellyfin files preferences under:
// the item identifier when the id is one, otherwise an MD5 digest of the
// id's UTF-16 encoding read as a .NET Guid. It is also the Id answered.
func displayPreferencesID(id string) string {
	if parsed, ok := parseGUID(id); ok {
		return hyphenated(parsed)
	}
	units := utf16.Encode([]rune(id))
	encoded := make([]byte, 2*len(units))
	for i, unit := range units {
		binary.LittleEndian.PutUint16(encoded[2*i:], unit)
	}
	sum := md5.Sum(encoded)
	// .NET reads the first three fields of a Guid as little-endian.
	var guid accounts.ID
	copy(guid[:], sum[:])
	slices.Reverse(guid[0:4])
	slices.Reverse(guid[4:6])
	slices.Reverse(guid[6:8])
	return hyphenated(guid)
}

// displayPreferencesRequest binds the query of both display preferences
// endpoints. ok is false once w was answered.
func (h *Handler) displayPreferencesRequest(w http.ResponseWriter, r *http.Request, bodyErrors map[string][]string) (
	user accounts.User, id, client string, ok bool) {
	errs := bindErrors{}
	client, _ = queryParam(r, "client")
	if strings.TrimSpace(client) == "" {
		errs.add("client", "The client field is required.")
	}
	userID, set := errs.guid(r, "userId")
	for name, messages := range bodyErrors {
		errs[name] = append(errs[name], messages...)
	}
	if len(errs) > 0 {
		validationProblem(w, errs)
		return accounts.User{}, "", "", false
	}
	// Jellyfin fails when an administrator names an unknown user.
	user, ok = h.targetUser(w, r, userID, set, func(w http.ResponseWriter) {
		processingError(w, http.StatusInternalServerError)
	})
	return user, displayPreferencesID(r.PathValue("displayPreferencesId")), client, ok
}

func (h *Handler) loadDisplayState(r *http.Request, user accounts.User, id, client string) (displayState, bool, error) {
	raw, found, err := h.Preferences.Get(r.Context(), user.ID, id, client)
	if err != nil || !found {
		return unsavedDisplayState(), false, err
	}
	var state displayState
	if err := json.Unmarshal(raw, &state); err != nil {
		return displayState{}, false, fmt.Errorf("display preferences %s of user %s: %w", id, user.ID, err)
	}
	return state, true, nil
}

func (h *Handler) displayPreferences(w http.ResponseWriter, r *http.Request) {
	user, id, client, ok := h.displayPreferencesRequest(w, r, nil)
	if !ok {
		return
	}
	state, _, err := h.loadDisplayState(r, user, id, client)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, state.dto(id, client))
}

func (h *Handler) updateDisplayPreferences(w http.ResponseWriter, r *http.Request) {
	if !jsonContent(r.Header.Get("Content-Type")) {
		unsupportedMediaTypeProblem(w)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		processingError(w, http.StatusRequestEntityTooLarge)
		return
	}
	if err != nil {
		processingError(w, http.StatusBadRequest)
		return
	}
	update, bodyErrors := parseDisplayPreferences(body)
	user, id, client, ok := h.displayPreferencesRequest(w, r, bodyErrors)
	if !ok {
		return
	}
	existing, _, err := h.loadDisplayState(r, user, id, client)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	state, status := update.apply(existing.HomeSections)
	if status != 0 {
		processingError(w, status)
		return
	}
	value, _ := json.Marshal(state)
	if err := h.Preferences.Put(r.Context(), user.ID, id, client, value); err != nil {
		h.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// jsonContent reports whether ASP.NET's JSON input formatter reads a body
// of this content type.
func jsonContent(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	return mediaType == "application/json" || mediaType == "text/json" ||
		strings.HasPrefix(mediaType, "application/") && strings.HasSuffix(mediaType, "+json")
}

// displayUpdate is a DisplayPreferencesDto as an app posts it.
type displayUpdate struct {
	sortBy           *string
	indexBy          *string
	rememberIndexing bool
	scrollDirection  int
	showBackdrop     bool
	rememberSorting  bool
	sortOrder        int
	showSidebar      bool
	customPrefs      []pref // in document order
}

// parseDisplayPreferences reads the body as Jellyfin's JSON settings do:
// property names in any letter case, the last of duplicates winning,
// strings also read from numbers and booleans, integers also from strings,
// enums from names or numbers. Errors are keyed like ASP.NET's.
func parseDisplayPreferences(body []byte) (displayUpdate, map[string][]string) {
	update := displayUpdate{showBackdrop: true}
	required := []string{"The displayPreferences field is required."}
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return update, map[string][]string{
			"":                   {"A non-empty request body is required."},
			"displayPreferences": required,
		}
	}
	fail := func(path, message string, offset int64) (displayUpdate, map[string][]string) {
		line, column := jsonPosition(body, offset)
		return update, map[string][]string{
			path:                 {fmt.Sprintf("%s Path: %s | LineNumber: %d | BytePositionInLine: %d.", message, path, line, column)},
			"displayPreferences": required,
		}
	}
	failSyntax := func(err error, expectingName bool) (displayUpdate, map[string][]string) {
		message, offset := syntaxProblem(body, err, expectingName)
		return fail("$", message, offset)
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	if token, err := decoder.Token(); err != nil {
		return failSyntax(err, false)
	} else if token != json.Delim('{') {
		return fail("$", "The JSON value could not be converted to MediaBrowser.Model.Dto.DisplayPreferencesDto.", decoder.InputOffset())
	}
	customPrefsNull := false
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return failSyntax(err, true)
		}
		name := token.(string)
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return failSyntax(err, false)
		}
		end, path := decoder.InputOffset(), "$."+name
		message := ""
		switch strings.ToLower(name) {
		case "sortby":
			update.sortBy, message = jsonString(raw)
		case "indexby":
			var text *string
			text, message = jsonString(raw)
			update.indexBy = nil
			if text != nil {
				if kind, ok := parseEnum(*text, indexingKinds); ok {
					kindName := enumName(kind, indexingKinds)
					update.indexBy = &kindName
				}
			}
		case "id", "viewtype", "client":
			_, message = jsonString(raw)
		case "rememberindexing":
			update.rememberIndexing, message = jsonBool(raw)
		case "showbackdrop":
			update.showBackdrop, message = jsonBool(raw)
		case "remembersorting":
			update.rememberSorting, message = jsonBool(raw)
		case "showsidebar":
			update.showSidebar, message = jsonBool(raw)
		case "primaryimageheight", "primaryimagewidth":
			// Jellyfin answers 250 whatever was saved.
			_, message = jsonInt32(raw)
		case "scrolldirection":
			update.scrollDirection, message = jsonEnum(raw, scrollDirections, "Jellyfin.Database.Implementations.Enums.ScrollDirection")
		case "sortorder":
			update.sortOrder, message = jsonEnum(raw, sortOrders, "Jellyfin.Database.Implementations.Enums.SortOrder")
		case "customprefs":
			var prefs []pref
			var errPath string
			var errOffset int64
			prefs, errPath, errOffset, message = jsonPrefs(raw)
			if errPath != "" {
				path += "." + errPath
				end += errOffset - int64(len(raw))
			}
			customPrefsNull = message == "" && prefs == nil
			update.customPrefs = prefs
		}
		if message != "" {
			return fail(path, message, end)
		}
	}
	if _, err := decoder.Token(); err != nil {
		return failSyntax(err, true)
	}
	// .NET reports the first byte after the object.
	trailing := bytes.TrimLeft(body[decoder.InputOffset():], " \t\r\n")
	if len(trailing) > 0 {
		offset := int64(len(body) - len(trailing))
		return fail("$", fmt.Sprintf("'%c' is invalid after a single JSON value. Expected end of data.", trailing[0]), offset)
	}
	if customPrefsNull {
		return update, map[string][]string{"CustomPrefs": {"The CustomPrefs field is required."}}
	}
	return update, nil
}

// syntaxProblem describes malformed JSON as .NET does, with the offset of
// the offending byte. expectingName tells whether a property name was due.
func syntaxProblem(body []byte, err error, expectingName bool) (string, int64) {
	var syntax *json.SyntaxError
	end := int64(len(bytes.TrimRight(body, " \t\r\n")))
	if !errors.As(err, &syntax) || syntax.Offset <= 0 || syntax.Offset > end || strings.Contains(syntax.Error(), "unexpected end") {
		if expectingName {
			return "Expected start of a property name or value, but instead reached end of data.", max(end-1, 0)
		}
		return "Expected a value, but instead reached end of data.", end
	}
	offset := syntax.Offset - 1
	if expectingName {
		return fmt.Sprintf("'%c' is an invalid start of a property name. Expected a '\"'.", body[offset]), offset
	}
	return fmt.Sprintf("'%c' is an invalid start of a value.", body[offset]), offset
}

// jsonPosition converts a byte offset into .NET's zero-based line number
// and byte position in that line.
func jsonPosition(body []byte, offset int64) (line, column int) {
	before := body[:min(int(offset), len(body))]
	line = bytes.Count(before, []byte{'\n'})
	return line, len(before) - (bytes.LastIndexByte(before, '\n') + 1)
}

func jsonCannotConvert(typeName string) string {
	return "The JSON value could not be converted to " + typeName + "."
}

// jsonString reads a string as Jellyfin's string converter does: numbers
// and booleans give their JSON text.
func jsonString(raw json.RawMessage) (*string, string) {
	switch raw[0] {
	case 'n':
		return nil, ""
	case '"':
		var text string
		_ = json.Unmarshal(raw, &text)
		return &text, ""
	case '{', '[':
		return nil, "The converter 'Jellyfin.Extensions.Json.Converters.JsonStringConverter' read too much or not enough."
	default:
		text := string(raw)
		return &text, ""
	}
}

func jsonBool(raw json.RawMessage) (bool, string) {
	switch string(raw) {
	case "true":
		return true, ""
	case "false":
		return false, ""
	}
	return false, jsonCannotConvert("System.Boolean")
}

func jsonInt32(raw json.RawMessage) (int, string) {
	text := string(raw)
	if raw[0] == '"' {
		_ = json.Unmarshal(raw, &text)
	}
	digits := strings.TrimPrefix(text, "-")
	if digits != "" && strings.Trim(digits, "0123456789") == "" {
		if value, err := strconv.ParseInt(text, 10, 32); err == nil {
			return int(value), ""
		}
	}
	return 0, jsonCannotConvert("System.Int32")
}

func jsonEnum(raw json.RawMessage, names []string, typeName string) (int, string) {
	switch raw[0] {
	case '"':
		var text string
		_ = json.Unmarshal(raw, &text)
		if value, ok := parseEnum(text, names); ok {
			return value, ""
		}
	case '-', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		if value, err := strconv.ParseInt(string(raw), 10, 32); err == nil {
			return int(value), ""
		}
	}
	return 0, jsonCannotConvert(typeName)
}

// jsonPrefs reads CustomPrefs: null, or an object of string values. On
// error it returns the failing key, relative to CustomPrefs, and the offset
// in raw .NET reports for it: just past the start of the value.
func jsonPrefs(raw json.RawMessage) (prefs []pref, errPath string, errOffset int64, message string) {
	if string(raw) == "null" {
		return nil, "", 0, ""
	}
	dictionary := "System.Collections.Generic.Dictionary`2[System.String,System.String]"
	if raw[0] != '{' {
		return nil, "", 0, jsonCannotConvert(dictionary)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	_, _ = decoder.Token()
	prefs = []pref{}
	for decoder.More() {
		token, _ := decoder.Token()
		key := token.(string)
		var value json.RawMessage
		_ = decoder.Decode(&value)
		text, message := jsonString(value)
		if message != "" {
			return nil, key, decoder.InputOffset() - int64(len(value)) + 1, message
		}
		prefs = append(prefs, pref{key, text})
	}
	return prefs, "", 0, ""
}

// apply turns an update into the state Jellyfin saves, merging home
// sections into the existing ones. A non-zero status reports the failure
// Jellyfin answers for an unreadable app setting.
func (u displayUpdate) apply(sections []homeSection) (displayState, int) {
	state := displayState{
		SortBy:                     "SortName",
		IndexBy:                    u.indexBy,
		RememberIndexing:           u.rememberIndexing,
		ScrollDirection:            u.scrollDirection,
		ShowBackdrop:               u.showBackdrop,
		RememberSorting:            u.rememberSorting,
		SortOrder:                  u.sortOrder,
		ShowSidebar:                u.showSidebar,
		HomeSections:               slices.Clone(sections),
		SkipForwardLength:          15000,
		SkipBackLength:             15000,
		EnableNextVideoInfoOverlay: true,
		TvHome:                     new(""),
		DashboardTheme:             new(""),
		Custom:                     map[string]*string{},
	}
	if u.sortBy != nil {
		state.SortBy = *u.sortBy
	}
	known := map[string]*string{}
	var homeSections []pref
	for _, entry := range u.customPrefs {
		switch {
		case len(entry.key) >= len(homeSectionPrefix) && strings.EqualFold(entry.key[:len(homeSectionPrefix)], homeSectionPrefix):
			homeSections = append(homeSections, entry)
		case slices.Contains([]string{"chromecastVersion", "skipForwardLength", "skipBackLength",
			"enableNextVideoInfoOverlay", "tvhome", "dashboardTheme"}, entry.key):
			known[entry.key] = entry.value
		default:
			state.Custom[entry.key] = entry.value
		}
	}
	text := func(key string) (string, bool) {
		value := known[key]
		if value == nil || *value == "" {
			return "", false
		}
		return *value, true
	}
	if value, ok := text("chromecastVersion"); ok {
		version, valid := parseEnum(value, chromecastVersions)
		if !valid {
			return displayState{}, http.StatusBadRequest
		}
		state.ChromecastVersion = version
	}
	for key, length := range map[string]*int{"skipForwardLength": &state.SkipForwardLength, "skipBackLength": &state.SkipBackLength} {
		if value, ok := text(key); ok {
			parsed, valid := parseDotNetInt(value)
			if !valid {
				return displayState{}, http.StatusInternalServerError
			}
			*length = parsed
		}
	}
	if value, ok := text("enableNextVideoInfoOverlay"); ok {
		enabled, valid := parseDotNetBool(value)
		if !valid {
			return displayState{}, http.StatusInternalServerError
		}
		state.EnableNextVideoInfoOverlay = enabled
	}
	for key, setting := range map[string]**string{"tvhome": &state.TvHome, "dashboardTheme": &state.DashboardTheme} {
		if value, sent := known[key]; sent {
			*setting = value
		}
	}
	for _, entry := range homeSections {
		index, valid := parseDotNetInt(entry.key[len(homeSectionPrefix):])
		if !valid {
			return displayState{}, http.StatusInternalServerError
		}
		kind, recognised := 0, false
		if entry.value != nil {
			kind, recognised = parseEnum(*entry.value, homeSectionTypes)
		}
		if !recognised {
			// Jellyfin fails to find a default for a negative position.
			if index < 0 {
				return displayState{}, http.StatusInternalServerError
			}
			kind = 0
			if index < len(defaultHomeSections) {
				kind = defaultHomeSections[index]
			}
		}
		position := slices.IndexFunc(state.HomeSections, func(s homeSection) bool { return s.Index == index })
		if position >= 0 {
			state.HomeSections[position].Type = kind
		} else {
			state.HomeSections = append(state.HomeSections, homeSection{Index: index, Type: kind})
		}
	}
	return state, 0
}

// endpointInfo tells an app whether it reaches the server from the same
// machine or from the local network.
func (h *Handler) endpointInfo(w http.ResponseWriter, r *http.Request) {
	address, err := netip.ParseAddr(remoteAddress(r))
	address = address.Unmap()
	local := err == nil && address.IsLoopback()
	inNetwork := local || err == nil && (address.IsPrivate() || address.IsLinkLocalUnicast())
	writeJSON(w, http.StatusOK, struct {
		IsLocal     bool
		IsInNetwork bool
	}{local, inNetwork})
}

const (
	defaultBitrateTestSize = 102400
	maxBitrateTestSize     = 100_000_000
)

// bitrateTest sends random bytes for apps to measure the connection. Like
// Jellyfin, it sends the requested size rounded up to a power of two, and
// at least 16 bytes.
func (h *Handler) bitrateTest(w http.ResponseWriter, r *http.Request) {
	size := defaultBitrateTestSize
	if raw, sent := queryParam(r, "size"); sent {
		if strings.TrimSpace(raw) == "" {
			validationProblem(w, map[string][]string{"size": {fmt.Sprintf("The value '%s' is invalid.", raw)}})
			return
		}
		value, message := convertInt32(raw)
		if message == "" && (value < 1 || value > maxBitrateTestSize) {
			message = fmt.Sprintf("The requested size must be greater than or equal to 1 and less than or equal to %d", maxBitrateTestSize)
		}
		if message != "" {
			validationProblem(w, map[string][]string{"size": {message}})
			return
		}
		size = value
	}
	length := 16
	for length < size {
		length *= 2
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.Itoa(length))
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	var seed [32]byte
	_, _ = rand.Read(seed[:])
	source := mathrand.NewChaCha8(seed)
	buffer := make([]byte, min(length, 64<<10))
	for remaining := length; remaining > 0; remaining -= len(buffer) {
		buffer = buffer[:min(remaining, len(buffer))]
		_, _ = source.Read(buffer)
		if _, err := w.Write(buffer); err != nil {
			return
		}
	}
}

// emptyResult is a QueryResult without items.
type emptyResult struct {
	Items            []struct{}
	TotalRecordCount int
	StartIndex       int
}

// emptyPage answers an empty QueryResult after binding the paging and user
// parameters these endpoints share.
func emptyPage(w http.ResponseWriter, r *http.Request, errs bindErrors) {
	startIndex, _ := errs.int32(r, "startIndex")
	errs.int32(r, "limit")
	errs.guid(r, "userId")
	if len(errs) > 0 {
		validationProblem(w, errs)
		return
	}
	writeJSON(w, http.StatusOK, emptyResult{Items: []struct{}{}, StartIndex: startIndex})
}

func (h *Handler) emptyQueryResult(w http.ResponseWriter, r *http.Request) {
	emptyPage(w, r, bindErrors{})
}

// itemRequest binds the item of an /Items/{itemId}/… endpoint, with the
// parameters bind adds, and answers 404 for an item the caller cannot
// reach. The empty identifier, which Jellyfin accepts, is returned as is.
// ok is false once w was answered.
func (h *Handler) itemRequest(w http.ResponseWriter, r *http.Request, bind func(bindErrors)) (accounts.ID, bool) {
	errs := bindErrors{}
	raw := r.PathValue("itemId")
	id, valid := parseGUID(raw)
	if !valid {
		errs.add("itemId", notValid(raw))
	}
	errs.guid(r, "userId")
	bind(errs)
	if len(errs) > 0 {
		validationProblem(w, errs)
		return accounts.ID{}, false
	}
	if id == (accounts.ID{}) {
		return id, true
	}
	exists, err := h.auxiliaryItemExists(r, id)
	if err != nil {
		h.internalError(w, r, err)
		return accounts.ID{}, false
	}
	if !exists {
		notFoundProblem(w)
		return accounts.ID{}, false
	}
	return id, true
}

// auxiliaryItemExists reports whether an item exists for the caller, a
// version of a title counting as an item.
func (h *Handler) auxiliaryItemExists(r *http.Request, id accounts.ID) (bool, error) {
	_, err := h.Library.Item(r.Context(), callerFrom(r.Context()).User, id)
	if errors.Is(err, library.ErrNotFound) {
		_, isVersion := h.Library.VersionOwner(id)
		return isVersion, nil
	}
	return err == nil, err
}

// similarLimit is how many similar titles Jellyfin answers when the app
// does not say.
const similarLimit = 50

// similarItems lists titles close to a movie or series (see
// library.Service.Similar). Like Jellyfin, it leaves out the titles the user
// played, and describes the titles with their provider identifiers, asked
// for or not. Other items have none.
func (h *Handler) similarItems(w http.ResponseWriter, r *http.Request) {
	limit, limited := 0, false
	// Similar items take a limit but no start index: StartIndex is 0.
	id, ok := h.itemRequest(w, r, func(errs bindErrors) { limit, limited = errs.int32(r, "limit") })
	if !ok {
		return
	}
	if !limited || limit < 0 {
		limit = similarLimit
	}
	user := callerFrom(r.Context()).User
	var items []library.Item
	if id != (accounts.ID{}) {
		// Twice the limit leaves room for the played titles left out.
		found, err := h.Library.Similar(r.Context(), user, id, 2*limit)
		if err != nil && !errors.Is(err, library.ErrNotFound) {
			h.browseError(w, r, err)
			return
		}
		items = found
	}
	state, err := h.userState(r.Context(), user, items)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	items = slices.DeleteFunc(items, func(item library.Item) bool { return state.of(item).Played })
	items = items[:min(limit, len(items))]
	fields := requestedFields(r)
	fields["providerids"] = true
	writeJSON(w, http.StatusOK, QueryResult{Items: h.listDtos(r, user, items, fields, state), TotalRecordCount: len(items)})
}

func (h *Handler) itemCollections(w http.ResponseWriter, r *http.Request) {
	startIndex := 0
	id, ok := h.itemRequest(w, r, func(errs bindErrors) {
		startIndex, _ = errs.int32(r, "startIndex")
		errs.int32(r, "limit")
	})
	if !ok {
		return
	}
	if id == (accounts.ID{}) {
		// Jellyfin fails to find the collections of no item.
		processingError(w, http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, emptyResult{Items: []struct{}{}, StartIndex: startIndex})
}

func (h *Handler) itemExtras(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.itemRequest(w, r, func(bindErrors) {}); ok {
		writeJSON(w, http.StatusOK, []struct{}{})
	}
}

// intros lists the videos played before an item: none in Polyfin.
func (h *Handler) intros(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.itemRequest(w, r, func(bindErrors) {}); ok {
		writeJSON(w, http.StatusOK, emptyResult{Items: []struct{}{}})
	}
}

type themeMediaResult struct {
	OwnerId          string
	Items            []struct{}
	TotalRecordCount int
	StartIndex       int
}

func (h *Handler) themeMedia(w http.ResponseWriter, r *http.Request) {
	id, ok := h.itemRequest(w, r, func(errs bindErrors) { errs.bool(r, "inheritFromParent") })
	if !ok {
		return
	}
	owner := themeMediaResult{OwnerId: id.String(), Items: []struct{}{}}
	writeJSON(w, http.StatusOK, struct {
		ThemeVideosResult     themeMediaResult
		ThemeSongsResult      themeMediaResult
		SoundtrackSongsResult themeMediaResult
	}{owner, owner, themeMediaResult{OwnerId: accounts.ID{}.String(), Items: []struct{}{}}})
}
