package jellyfin

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/recordings"
)

// recordingsFolderID identifies the folder recordings are listed in, the
// same on every server.
var recordingsFolderID, _ = accounts.ParseID(nameID("view", "recordings"))

// timerServiceName is the name of the service Jellyfin's timers belong to,
// its own recorder's.
const timerServiceName = "Emby"

// defaultTimerID stands for the identifier Jellyfin derives for the
// defaults of a new timer, which have none of their own.
var defaultTimerID = nameID("timer", "defaults")

// recordingRoutes registers Jellyfin's DVR API. Without a recordings
// folder, Polyfin answers as a server that records nothing: its lists are
// empty, what they would hold is not found, and new timers are refused as
// Jellyfin refuses a timer it cannot make (400).
func (h *Handler) recordingRoutes(rt *router, signedIn func(method, pattern string, handler http.HandlerFunc)) {
	// Like Jellyfin's LiveTvManagement policy, these need the permission to
	// manage recordings, and no more.
	managing := func(method, pattern string, handler http.HandlerFunc) {
		rt.handle(method, pattern, h.authenticated(liveTvManagement(handler)))
	}
	signedIn(http.MethodGet, "/LiveTv/Recordings", h.recordings)
	signedIn(http.MethodGet, "/LiveTv/Recordings/Folders", h.recordingFolders)
	signedIn(http.MethodGet, "/LiveTv/Recordings/{recordingId}", h.recording)
	managing(http.MethodDelete, "/LiveTv/Recordings/{recordingId}", h.deleteRecording)
	signedIn(http.MethodGet, "/LiveTv/Timers", h.timers)
	managing(http.MethodPost, "/LiveTv/Timers", h.createTimer)
	signedIn(http.MethodGet, "/LiveTv/Timers/Defaults", h.timerDefaults)
	signedIn(http.MethodGet, "/LiveTv/Timers/{timerId}", h.timer)
	managing(http.MethodPost, "/LiveTv/Timers/{timerId}", h.updateTimer)
	managing(http.MethodDelete, "/LiveTv/Timers/{timerId}", h.cancelTimer)
	signedIn(http.MethodGet, "/LiveTv/SeriesTimers", h.seriesTimers)
	managing(http.MethodPost, "/LiveTv/SeriesTimers", h.createSeriesTimer)
	signedIn(http.MethodGet, "/LiveTv/SeriesTimers/{timerId}", h.seriesTimer)
	managing(http.MethodPost, "/LiveTv/SeriesTimers/{timerId}", h.updateSeriesTimer)
	managing(http.MethodDelete, "/LiveTv/SeriesTimers/{timerId}", h.cancelSeriesTimer)
	// Players fetch it without credentials: see liveRecordingFile.
	rt.handle(http.MethodGet, "/LiveTv/LiveRecordings/{recordingId}/stream", http.HandlerFunc(h.liveRecordingFile))
}

// liveTvManagement refuses the routes managing recordings to users without
// the permission, as Jellyfin's LiveTvManagement policy refuses users
// without EnableLiveTvManagement.
func liveTvManagement(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !callerFrom(r.Context()).User.LiveTvManagement {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

// recordable reports whether the server records.
func (h *Handler) recordable() bool {
	return h.Recordings.Available()
}

// BaseTimerInfo is what Jellyfin's timers and series timers share, in its
// field order.
type BaseTimerInfo struct {
	Id                      string
	Type                    string
	ServerId                string
	ExternalId              string `json:",omitempty"`
	ChannelId               string
	ExternalChannelId       string `json:",omitempty"`
	ChannelName             string `json:",omitempty"`
	ChannelPrimaryImageTag  string `json:",omitempty"`
	ProgramId               string `json:",omitempty"`
	ExternalProgramId       string `json:",omitempty"`
	Name                    string `json:",omitempty"`
	Overview                string `json:",omitempty"`
	StartDate               Time
	EndDate                 Time
	ServiceName             string
	Priority                int
	PrePaddingSeconds       int
	PostPaddingSeconds      int
	IsPrePaddingRequired    bool
	ParentBackdropItemId    string   `json:",omitempty"`
	ParentBackdropImageTags []string `json:",omitempty"`
	IsPostPaddingRequired   bool
	KeepUntil               string
}

// TimerInfoDto is Jellyfin's timer.
type TimerInfoDto struct {
	BaseTimerInfo
	Status                string
	SeriesTimerId         string       `json:",omitempty"`
	ExternalSeriesTimerId string       `json:",omitempty"`
	RunTimeTicks          *int64       `json:",omitempty"`
	ProgramInfo           *BaseItemDto `json:",omitempty"`
}

// SeriesTimerInfoDto is Jellyfin's series timer.
type SeriesTimerInfoDto struct {
	BaseTimerInfo
	RecordAnyTime            bool
	SkipEpisodesInLibrary    bool
	RecordAnyChannel         bool
	KeepUpTo                 int
	RecordNewOnly            bool
	Days                     []string
	DayPattern               string `json:",omitempty"`
	ImageTags                map[string]string
	ParentThumbItemId        string `json:",omitempty"`
	ParentThumbImageTag      string `json:",omitempty"`
	ParentPrimaryImageItemId string `json:",omitempty"`
	ParentPrimaryImageTag    string `json:",omitempty"`
}

type timerQueryResult struct {
	Items            []TimerInfoDto
	TotalRecordCount int
	StartIndex       int
}

type seriesTimerQueryResult struct {
	Items            []SeriesTimerInfoDto
	TotalRecordCount int
	StartIndex       int
}

// timerUpdate is a TimerInfoDto or SeriesTimerInfoDto an app posts: the
// defaults Timers/Defaults gave, or a timer it read, changed.
type timerUpdate struct {
	ChannelId             string
	ProgramId             string
	Name                  string
	Overview              string
	StartDate             *Time
	EndDate               *Time
	PrePaddingSeconds     *int
	PostPaddingSeconds    *int
	Priority              int
	KeepUntil             *enumValue
	RecordAnyTime         *bool
	RecordAnyChannel      *bool
	RecordNewOnly         *bool
	SkipEpisodesInLibrary *bool
	KeepUpTo              int
	Days                  []enumValue
}

// enumValue reads an enumeration as Jellyfin's JSON does: its name, or its
// number (see resolve).
type enumValue string

func (e *enumValue) UnmarshalJSON(data []byte) error {
	*e = enumValue(strings.Trim(string(data), `"`))
	return nil
}

// resolve maps a name or number to one of names, ok false for neither.
func (e enumValue) resolve(names []string) (string, bool) {
	text := strings.TrimSpace(string(e))
	if n, err := strconv.Atoi(text); err == nil && n >= 0 && n < len(names) {
		return names[n], true
	}
	i := slices.IndexFunc(names, func(name string) bool { return strings.EqualFold(text, name) })
	if i < 0 {
		return "", false
	}
	return names[i], true
}

// timerOptions reads the options of a posted timer, with the settings'
// padding for those left out.
func (h *Handler) timerOptions(body timerUpdate) (recordings.Options, bool) {
	settings := h.Accounts.Settings()
	o := recordings.Options{PrePadding: settings.RecordingPrePadding, PostPadding: settings.RecordingPostPadding,
		KeepUntil: recordings.KeepUntil[0], Priority: body.Priority}
	if body.PrePaddingSeconds != nil {
		o.PrePadding = *body.PrePaddingSeconds
	}
	if body.PostPaddingSeconds != nil {
		o.PostPadding = *body.PostPaddingSeconds
	}
	if body.KeepUntil != nil {
		keep, ok := body.KeepUntil.resolve(recordings.KeepUntil)
		if !ok {
			return o, false
		}
		o.KeepUntil = keep
	}
	return o, o.PrePadding >= 0 && o.PrePadding <= recordings.MaxPadding && o.PostPadding >= 0 && o.PostPadding <= recordings.MaxPadding
}

// seriesOptions reads what a posted series timer records, Jellyfin's
// defaults standing for what it leaves out.
func seriesOptions(body timerUpdate) (recordings.SeriesOptions, bool) {
	so := recordings.SeriesOptions{RecordAnyTime: true, RecordNewOnly: true, KeepUpTo: body.KeepUpTo, Days: []string{}}
	for flag, value := range map[*bool]*bool{&so.RecordAnyTime: body.RecordAnyTime, &so.RecordAnyChannel: body.RecordAnyChannel,
		&so.RecordNewOnly: body.RecordNewOnly} {
		if value != nil {
			*flag = *value
		}
	}
	so.SkipEpisodesInLibrary = so.RecordNewOnly
	if body.SkipEpisodesInLibrary != nil {
		so.SkipEpisodesInLibrary = *body.SkipEpisodesInLibrary
	}
	for _, raw := range body.Days {
		day, ok := raw.resolve(recordings.Days)
		if !ok {
			return so, false
		}
		if !slices.Contains(so.Days, day) {
			so.Days = append(so.Days, day)
		}
	}
	return so, so.KeepUpTo >= 0 && so.KeepUpTo <= recordings.MaxKeepUpTo
}

// readTimer reads a posted timer, which every route posting one requires.
// ok is false once w was answered.
func readTimer(w http.ResponseWriter, r *http.Request, parameter string) (timerUpdate, bool) {
	var body timerUpdate
	present, ok := readJSONBody(w, r, &body, parameter)
	if ok && !present {
		requireBody(w, parameter)
		return body, false
	}
	return body, ok
}

// reachedChannels maps the channels of ids user reaches to their items.
func (h *Handler) reachedChannels(ctx context.Context, user accounts.User, ids []accounts.ID) (map[accounts.ID]library.Item, error) {
	slices.SortFunc(ids, func(a, b accounts.ID) int { return strings.Compare(a.String(), b.String()) })
	items, err := h.Library.Items(ctx, user, slices.Compact(ids))
	result := map[accounts.ID]library.Item{}
	for _, item := range items {
		if item.Kind == library.KindChannel {
			result[item.ID] = item
		}
	}
	return result, err
}

// visibleTimers keeps the timers user sees: on channels they reach, of
// programmes their parental control and blocked genres let them reach. It
// returns the channels of those kept.
func (h *Handler) visibleTimers(ctx context.Context, user accounts.User, timers []recordings.Timer) ([]recordings.Timer, map[accounts.ID]library.Item, error) {
	ids := make([]accounts.ID, 0, len(timers))
	for _, t := range timers {
		ids = append(ids, t.Channel)
	}
	channels, err := h.reachedChannels(ctx, user, ids)
	if err != nil {
		return nil, nil, err
	}
	return slices.DeleteFunc(timers, func(t recordings.Timer) bool {
		_, reached := channels[t.Channel]
		return !reached || !recordings.Allows(user, recordings.ProgramKind, t.Rating, t.Genres)
	}), channels, nil
}

// visibleTimer returns a timer user sees.
func (h *Handler) visibleTimer(ctx context.Context, user accounts.User, raw string) (recordings.Timer, library.Item, error) {
	id, ok := parseGUID(raw)
	if !ok || !h.recordable() {
		return recordings.Timer{}, library.Item{}, recordings.ErrNotFound
	}
	t, err := h.Recordings.Timer(ctx, id)
	if err != nil {
		return t, library.Item{}, err
	}
	kept, channels, err := h.visibleTimers(ctx, user, []recordings.Timer{t})
	if err != nil {
		return t, library.Item{}, err
	}
	if len(kept) == 0 {
		return t, library.Item{}, recordings.ErrNotFound
	}
	return t, channels[t.Channel], nil
}

// baseTimer fills what timers and series timers share.
func (h *Handler) baseTimer(kind string, id accounts.ID, channel library.Item, p recordings.Programme, o recordings.Options) BaseTimerInfo {
	base := BaseTimerInfo{Id: id.String(), Type: kind, ServerId: h.ServerID, ExternalId: id.String(), ChannelId: p.Channel.String(),
		ExternalChannelId: p.Channel.String(), ChannelName: channel.Name, ChannelPrimaryImageTag: library.ImageTag(channel.Images.Primary),
		Name: p.Name, Overview: p.Overview, StartDate: Time(p.Start), EndDate: Time(p.End), ServiceName: timerServiceName,
		Priority: o.Priority, PrePaddingSeconds: o.PrePadding, PostPaddingSeconds: o.PostPadding, KeepUntil: o.KeepUntil}
	if p.Program != nil {
		base.ProgramId, base.ExternalProgramId = p.Program.String(), p.Program.String()
	}
	return base
}

// programItem describes the programme a timer records as the guide did.
func programItem(t recordings.Timer, channel library.Item) library.Item {
	start, end := t.Start, t.End
	item := library.Item{Kind: library.KindProgram, Name: t.Name, ParentID: channel.ID, Overview: t.Overview, Genres: t.Genres,
		OfficialRating: t.Rating, StartDate: &start, EndDate: &end, Runtime: end.Sub(start), Images: library.Images{Primary: t.Image},
		Available: true, Channel: &channel}
	if t.Program != nil {
		item.ID = *t.Program
	}
	return item
}

// timerDto describes a timer, with its programme, as Jellyfin does.
func (h *Handler) timerDto(t recordings.Timer, channel library.Item) TimerInfoDto {
	dto := TimerInfoDto{BaseTimerInfo: h.baseTimer("Timer", t.ID, channel, t.Programme, t.Options), Status: string(t.Status),
		RunTimeTicks: new(int64(t.End.Sub(t.Start) / 100))}
	if t.SeriesTimer != nil {
		dto.SeriesTimerId, dto.ExternalSeriesTimerId = t.SeriesTimer.String(), t.SeriesTimer.String()
	}
	if t.Program != nil {
		program := h.newItemDto(programItem(t, channel), nil, false, userState{})
		if t.Status != recordings.StatusCancelled && t.Status != recordings.StatusError {
			program.TimerId, program.Status = dto.Id, string(t.Status)
		}
		program.SeriesTimerId = dto.SeriesTimerId
		dto.ProgramInfo = &program
	}
	return dto
}

// seriesTimerDto describes a series timer as Jellyfin does.
func (h *Handler) seriesTimerDto(st recordings.SeriesTimer, channel library.Item) SeriesTimerInfoDto {
	dto := SeriesTimerInfoDto{BaseTimerInfo: h.baseTimer("SeriesTimer", st.ID, channel, st.Programme, st.Options),
		RecordAnyTime: st.RecordAnyTime, SkipEpisodesInLibrary: st.SkipEpisodesInLibrary, RecordAnyChannel: st.RecordAnyChannel,
		KeepUpTo: st.KeepUpTo, RecordNewOnly: st.RecordNewOnly, Days: nonNil(st.Days), DayPattern: dayPattern(st.Days),
		ImageTags: map[string]string{}}
	return dto
}

// dayPattern names a set of days as Jellyfin does: every day, the
// weekdays or the weekend, else none.
func dayPattern(days []string) string {
	has := func(names ...string) bool {
		return !slices.ContainsFunc(names, func(name string) bool { return !slices.Contains(days, name) })
	}
	switch {
	case len(days) == 7:
		return "Daily"
	case len(days) == 2 && has("Saturday", "Sunday"):
		return "Weekends"
	case len(days) == 5 && has("Monday", "Tuesday", "Wednesday", "Thursday", "Friday"):
		return "Weekdays"
	}
	return ""
}

// recordingError answers what the recordings service refused.
func (h *Handler) recordingError(w http.ResponseWriter, r *http.Request, err error, notFound func(http.ResponseWriter)) {
	switch {
	case errors.Is(err, recordings.ErrNotFound), errors.Is(err, library.ErrNotFound):
		notFound(w)
	case errors.Is(err, recordings.ErrScheduled), errors.Is(err, recordings.ErrInvalid), errors.Is(err, recordings.ErrUnavailable):
		// Jellyfin's ArgumentException.
		processingError(w, http.StatusBadRequest)
	case r.Context().Err() != nil:
	default:
		h.internalError(w, r, err)
	}
}

// resourceNotFound answers as Jellyfin's ResourceNotFoundException does.
func resourceNotFound(w http.ResponseWriter) {
	processingError(w, http.StatusNotFound)
}

// timers lists the timers the caller sees, not completed, by start.
func (h *Handler) timers(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	active, activeSet := b.bool(r, "isActive")
	scheduled, scheduledSet := b.bool(r, "isScheduled")
	if len(b) > 0 {
		validationProblem(w, b)
		return
	}
	result := timerQueryResult{Items: []TimerInfoDto{}}
	if !h.recordable() {
		writeJSON(w, http.StatusOK, result)
		return
	}
	user := callerFrom(r.Context()).User
	timers, err := h.Recordings.Timers(r.Context())
	var channels map[accounts.ID]library.Item
	if err == nil {
		timers, channels, err = h.visibleTimers(r.Context(), user, timers)
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	channel, channelSet := parseGUID(query(r, "channelId"))
	series, seriesSet := parseGUID(query(r, "seriesTimerId"))
	for _, t := range timers {
		if activeSet && active != (t.Status == recordings.StatusInProgress) || scheduledSet && scheduled != (t.Status == recordings.StatusNew) ||
			channelSet && t.Channel != channel || seriesSet && (t.SeriesTimer == nil || *t.SeriesTimer != series) {
			continue
		}
		result.Items = append(result.Items, h.timerDto(t, channels[t.Channel]))
	}
	result.TotalRecordCount = len(result.Items)
	writeJSON(w, http.StatusOK, result)
}

// timer describes a timer; like Jellyfin, one that does not exist is
// answered with no content.
func (h *Handler) timer(w http.ResponseWriter, r *http.Request) {
	t, channel, err := h.visibleTimer(r.Context(), callerFrom(r.Context()).User, r.PathValue("timerId"))
	if errors.Is(err, recordings.ErrNotFound) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err != nil {
		h.recordingError(w, r, err, resourceNotFound)
		return
	}
	writeJSON(w, http.StatusOK, h.timerDto(t, channel))
}

// guideProgram returns a programme of user's guide that their parental
// control and blocked genres let them reach.
func (h *Handler) guideProgram(ctx context.Context, user accounts.User, raw string) (library.Item, error) {
	id, ok := parseGUID(raw)
	if !ok {
		return library.Item{}, library.ErrNotFound
	}
	program, err := h.Library.Item(ctx, user, id)
	if err == nil && (program.Kind != library.KindProgram || program.Channel == nil ||
		!recordings.Allows(user, recordings.ProgramKind, program.OfficialRating, program.Genres)) {
		err = library.ErrNotFound
	}
	return program, err
}

// timerDefaults describes a new timer, for a programme when programId names
// one: the settings' padding, recording at any time, new programmes only,
// on every day. Jellyfin fails (500) on a programme it does not know;
// Polyfin answers 404.
func (h *Handler) timerDefaults(w http.ResponseWriter, r *http.Request) {
	settings := h.Accounts.Settings()
	dto := SeriesTimerInfoDto{
		BaseTimerInfo: BaseTimerInfo{Id: defaultTimerID, Type: "SeriesTimer", ServerId: h.ServerID, ChannelId: accounts.ID{}.String(),
			StartDate: Time(time.Time{}), EndDate: Time(time.Time{}), ServiceName: timerServiceName,
			PrePaddingSeconds: settings.RecordingPrePadding, PostPaddingSeconds: settings.RecordingPostPadding, KeepUntil: recordings.KeepUntil[0]},
		RecordAnyTime: true, RecordNewOnly: true, SkipEpisodesInLibrary: true, Days: slices.Clone(recordings.Days), DayPattern: "Daily",
		ImageTags: map[string]string{},
	}
	if raw := query(r, "programId"); raw != "" {
		program, err := h.guideProgram(r.Context(), callerFrom(r.Context()).User, raw)
		if err != nil {
			h.recordingError(w, r, err, resourceNotFound)
			return
		}
		dto.Name, dto.Overview = program.Name, program.Overview
		dto.ChannelId, dto.ChannelName = program.Channel.ID.String(), program.Channel.Name
		dto.StartDate, dto.EndDate = Time(*program.StartDate), Time(*program.EndDate)
		dto.ProgramId, dto.ExternalProgramId = program.ID.String(), program.ID.String()
	}
	writeJSON(w, http.StatusOK, dto)
}

// timerProgramme finds what a posted timer records in user's guide: the
// programme ProgramId names, else the programme of the channel starting
// within three minutes of StartDate, as Jellyfin looks for it.
func (h *Handler) timerProgramme(ctx context.Context, user accounts.User, body timerUpdate) (recordings.Programme, error) {
	if body.ProgramId != "" {
		if program, err := h.guideProgram(ctx, user, body.ProgramId); err == nil {
			return recordings.ProgrammeOf(program), nil
		} else if !errors.Is(err, library.ErrNotFound) {
			return recordings.Programme{}, err
		}
	}
	channel, ok := parseGUID(body.ChannelId)
	if !ok || body.StartDate == nil {
		return recordings.Programme{}, recordings.ErrInvalid
	}
	start := time.Time(*body.StartDate)
	programs, err := h.Library.Programs(ctx, user, start.Add(-3*time.Minute), start.Add(3*time.Minute))
	if err != nil {
		return recordings.Programme{}, err
	}
	for _, program := range programs {
		if program.Channel.ID == channel && program.StartDate.Sub(start).Abs() <= 3*time.Minute &&
			recordings.Allows(user, recordings.ProgramKind, program.OfficialRating, program.Genres) {
			return recordings.ProgrammeOf(program), nil
		}
	}
	return recordings.Programme{}, recordings.ErrInvalid
}

// createTimer schedules the recording of a programme of the caller's guide.
func (h *Handler) createTimer(w http.ResponseWriter, r *http.Request) {
	body, ok := readTimer(w, r, "timerInfo")
	if !ok {
		return
	}
	user := callerFrom(r.Context()).User
	options, valid := h.timerOptions(body)
	if !valid || !h.recordable() {
		processingError(w, http.StatusBadRequest)
		return
	}
	programme, err := h.timerProgramme(r.Context(), user, body)
	if err == nil {
		_, err = h.Recordings.CreateTimer(r.Context(), user.ID, programme, options)
	}
	if err != nil {
		h.recordingError(w, r, err, func(w http.ResponseWriter) { processingError(w, http.StatusBadRequest) })
		return
	}
	h.Activity.RecordingScheduled(r.Context(), user, programme.Name)
	w.WriteHeader(http.StatusNoContent)
}

// updateTimer changes a timer's padding. Jellyfin reads the timer from the
// body; Polyfin from the route, as the body's is the same.
func (h *Handler) updateTimer(w http.ResponseWriter, r *http.Request) {
	body, ok := readTimer(w, r, "timerInfo")
	if !ok {
		return
	}
	t, _, err := h.visibleTimer(r.Context(), callerFrom(r.Context()).User, r.PathValue("timerId"))
	if err == nil {
		pre, post := t.PrePadding, t.PostPadding
		if body.PrePaddingSeconds != nil {
			pre = *body.PrePaddingSeconds
		}
		if body.PostPaddingSeconds != nil {
			post = *body.PostPaddingSeconds
		}
		err = h.Recordings.UpdateTimer(r.Context(), t.ID, pre, post)
	}
	if err != nil {
		h.recordingError(w, r, err, resourceNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// cancelTimer cancels a timer; its recording, if it started, is kept.
func (h *Handler) cancelTimer(w http.ResponseWriter, r *http.Request) {
	t, _, err := h.visibleTimer(r.Context(), callerFrom(r.Context()).User, r.PathValue("timerId"))
	if err == nil {
		err = h.Recordings.CancelTimer(r.Context(), t.ID)
	}
	if err != nil {
		h.recordingError(w, r, err, resourceNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// visibleSeriesTimers keeps the series timers user sees: those recording
// on a channel they reach, or on any channel.
func (h *Handler) visibleSeriesTimers(ctx context.Context, user accounts.User, series []recordings.SeriesTimer) ([]recordings.SeriesTimer, map[accounts.ID]library.Item, error) {
	ids := make([]accounts.ID, 0, len(series))
	for _, st := range series {
		ids = append(ids, st.Channel)
	}
	channels, err := h.reachedChannels(ctx, user, ids)
	if err != nil {
		return nil, nil, err
	}
	return slices.DeleteFunc(series, func(st recordings.SeriesTimer) bool {
		_, reached := channels[st.Channel]
		return !reached && !st.RecordAnyChannel
	}), channels, nil
}

// visibleSeriesTimer returns a series timer user sees.
func (h *Handler) visibleSeriesTimer(ctx context.Context, user accounts.User, raw string) (recordings.SeriesTimer, library.Item, error) {
	id, ok := parseGUID(raw)
	if !ok || !h.recordable() {
		return recordings.SeriesTimer{}, library.Item{}, recordings.ErrNotFound
	}
	st, err := h.Recordings.SeriesTimer(ctx, id)
	if err != nil {
		return st, library.Item{}, err
	}
	kept, channels, err := h.visibleSeriesTimers(ctx, user, []recordings.SeriesTimer{st})
	if err == nil && len(kept) == 0 {
		err = recordings.ErrNotFound
	}
	return st, channels[st.Channel], err
}

// seriesTimers lists the series timers the caller sees, by name, or by
// priority when sortBy asks, as Jellyfin sorts them.
func (h *Handler) seriesTimers(w http.ResponseWriter, r *http.Request) {
	result := seriesTimerQueryResult{Items: []SeriesTimerInfoDto{}}
	if !h.recordable() {
		writeJSON(w, http.StatusOK, result)
		return
	}
	series, err := h.Recordings.SeriesTimers(r.Context())
	var channels map[accounts.ID]library.Item
	if err == nil {
		series, channels, err = h.visibleSeriesTimers(r.Context(), callerFrom(r.Context()).User, series)
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	descending := strings.EqualFold(query(r, "sortOrder"), "Descending")
	byName := func(a, b recordings.SeriesTimer) int {
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	}
	if strings.EqualFold(query(r, "sortBy"), "Priority") {
		// Jellyfin puts the highest priority first, unless descending.
		slices.SortStableFunc(series, func(a, b recordings.SeriesTimer) int {
			if a.Priority != b.Priority {
				return b.Priority - a.Priority
			}
			return byName(a, b)
		})
	} else {
		slices.SortStableFunc(series, byName)
	}
	if descending {
		slices.Reverse(series)
	}
	for _, st := range series {
		result.Items = append(result.Items, h.seriesTimerDto(st, channels[st.Channel]))
	}
	result.TotalRecordCount = len(result.Items)
	writeJSON(w, http.StatusOK, result)
}

// seriesTimer describes a series timer.
func (h *Handler) seriesTimer(w http.ResponseWriter, r *http.Request) {
	st, channel, err := h.visibleSeriesTimer(r.Context(), callerFrom(r.Context()).User, r.PathValue("timerId"))
	if err != nil {
		h.recordingError(w, r, err, notFoundProblem)
		return
	}
	writeJSON(w, http.StatusOK, h.seriesTimerDto(st, channel))
}

// createSeriesTimer records the programmes of the title of the programme
// the body names. Jellyfin fails (500) on a programme it does not know;
// Polyfin answers 400.
func (h *Handler) createSeriesTimer(w http.ResponseWriter, r *http.Request) {
	body, ok := readTimer(w, r, "seriesTimerInfo")
	if !ok {
		return
	}
	user := callerFrom(r.Context()).User
	options, valid := h.timerOptions(body)
	series, seriesValid := seriesOptions(body)
	if !valid || !seriesValid || !h.recordable() {
		processingError(w, http.StatusBadRequest)
		return
	}
	program, err := h.guideProgram(r.Context(), user, body.ProgramId)
	if err == nil {
		_, err = h.Recordings.CreateSeriesTimer(r.Context(), user.ID, recordings.ProgrammeOf(program), options, series)
	}
	if err != nil {
		h.recordingError(w, r, err, func(w http.ResponseWriter) { processingError(w, http.StatusBadRequest) })
		return
	}
	h.Activity.RecordingScheduled(r.Context(), user, program.Name)
	w.WriteHeader(http.StatusNoContent)
}

// updateSeriesTimer changes what a series timer records; like Jellyfin,
// one that does not exist is answered with no content.
func (h *Handler) updateSeriesTimer(w http.ResponseWriter, r *http.Request) {
	body, ok := readTimer(w, r, "seriesTimerInfo")
	if !ok {
		return
	}
	options, valid := h.timerOptions(body)
	series, seriesValid := seriesOptions(body)
	if !valid || !seriesValid {
		processingError(w, http.StatusBadRequest)
		return
	}
	st, _, err := h.visibleSeriesTimer(r.Context(), callerFrom(r.Context()).User, r.PathValue("timerId"))
	if err == nil {
		err = h.Recordings.UpdateSeriesTimer(r.Context(), st.ID, options, series)
	}
	if err != nil && !errors.Is(err, recordings.ErrNotFound) {
		h.recordingError(w, r, err, resourceNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// cancelSeriesTimer deletes a series timer and its timers; the recordings
// they made are kept.
func (h *Handler) cancelSeriesTimer(w http.ResponseWriter, r *http.Request) {
	st, _, err := h.visibleSeriesTimer(r.Context(), callerFrom(r.Context()).User, r.PathValue("timerId"))
	if err == nil {
		err = h.Recordings.CancelSeriesTimer(r.Context(), st.ID)
	}
	if err != nil {
		h.recordingError(w, r, err, resourceNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// visibleRecordings keeps the recordings user sees: those of the timers
// they made, and those of channels they reach, which their parental
// control and blocked genres let them reach. It returns the channels they
// reach.
func (h *Handler) visibleRecordings(ctx context.Context, user accounts.User, list []recordings.Recording) ([]recordings.Recording, map[accounts.ID]library.Item, error) {
	ids := make([]accounts.ID, 0, len(list))
	for _, rec := range list {
		ids = append(ids, rec.Channel)
	}
	channels, err := h.reachedChannels(ctx, user, ids)
	if err != nil {
		return nil, nil, err
	}
	return slices.DeleteFunc(list, func(rec recordings.Recording) bool {
		_, reached := channels[rec.Channel]
		mine := rec.User != nil && *rec.User == user.ID
		return !reached && !mine || !recordings.Allows(user, recordings.RecordingKind, rec.Rating, rec.Genres)
	}), channels, nil
}

// visibleRecording returns a recording user sees, with its channel when
// they reach it.
func (h *Handler) visibleRecording(ctx context.Context, user accounts.User, id accounts.ID) (recordings.Recording, library.Item, error) {
	if !h.recordable() || !user.LiveTv {
		return recordings.Recording{}, library.Item{}, recordings.ErrNotFound
	}
	rec, err := h.Recordings.Recording(ctx, id)
	if err != nil {
		return rec, library.Item{}, err
	}
	kept, channels, err := h.visibleRecordings(ctx, user, []recordings.Recording{rec})
	if err == nil && len(kept) == 0 {
		err = recordings.ErrNotFound
	}
	return rec, channels[rec.Channel], err
}

// recordingItem describes a recording as an item: a video of the
// recordings folder.
func recordingItem(rec recordings.Recording, channel library.Item) library.Item {
	start, end := rec.Start, rec.End
	runtime := end.Sub(start)
	if rec.Ended != nil {
		runtime = rec.Ended.Sub(rec.Started)
	}
	item := library.Item{ID: rec.ID, Kind: library.KindRecording, Name: rec.Name, ParentID: recordingsFolderID, Overview: rec.Overview,
		Genres: rec.Genres, OfficialRating: rec.Rating, Runtime: runtime, Images: library.Images{Primary: rec.Image}, Available: true,
		StartDate: &start, EndDate: &end}
	if channel.ID != (accounts.ID{}) {
		item.Channel = &channel
	}
	return item
}

// recordingDto describes a recording as Jellyfin describes a recorded
// video, with what describes a recording under way.
func (h *Handler) recordingDto(r *http.Request, user accounts.User, rec recordings.Recording, channel library.Item, fields fieldSet,
	detail bool, state userState) BaseItemDto {
	item := recordingItem(rec, channel)
	dto := h.newItemDto(item, fields, detail, state)
	dto.ChannelId = new(rec.Channel.String())
	if detail || fields.has("ChannelInfo") {
		dto.ChannelName, dto.ChannelNumber = channel.Name, channel.Number
	}
	dto.ChannelPrimaryImageTag = library.ImageTag(channel.Images.Primary)
	dto.StartDate, dto.EndDate = new(Time(rec.Start)), new(Time(rec.End))
	flags := programFlags(item)
	for flag, field := range map[string]**bool{"movie": &dto.IsMovie, "series": &dto.IsSeries, "news": &dto.IsNews,
		"kids": &dto.IsKids, "sports": &dto.IsSports} {
		if flags[flag] {
			*field = new(true)
		}
	}
	if rec.SeriesTimer != nil {
		dto.SeriesTimerId = rec.SeriesTimer.String()
	}
	if rec.InProgress() {
		dto.Status = string(recordings.StatusInProgress)
		if rec.Timer != nil {
			dto.TimerId = rec.Timer.String()
		}
		if total := rec.End.Sub(rec.Start); total > 0 {
			dto.CompletionPercentage = new(min(100, float64(time.Since(rec.Start))/float64(total)*100))
		}
	}
	if detail || fields.has("DateCreated") {
		dto.DateCreated = new(Time(rec.Started))
	}
	if detail || fields.has("CanDelete") {
		dto.CanDelete = new(user.LiveTvManagement)
	}
	if detail {
		h.addMediaSources(r, user, &dto, item, rec.ID, true)
	} else if fields.has("MediaSources") || fields.has("MediaStreams") {
		h.addMediaSources(r, user, &dto, item, rec.ID, false)
		if !fields.has("MediaSources") {
			dto.MediaSources = nil
		}
		if !fields.has("MediaStreams") {
			dto.MediaStreams = nil
		}
	}
	return dto
}

// listedRecordings are the recordings the caller sees, the latest first.
func (h *Handler) listedRecordings(ctx context.Context, user accounts.User) ([]recordings.Recording, map[accounts.ID]library.Item, error) {
	if !h.recordable() || !user.LiveTv {
		return nil, nil, nil
	}
	list, err := h.Recordings.Recordings(ctx)
	if err != nil {
		return nil, nil, err
	}
	return h.visibleRecordings(ctx, user, list)
}

// recordingDtos describes listed recordings.
func (h *Handler) recordingDtos(r *http.Request, user accounts.User, list []recordings.Recording, channels map[accounts.ID]library.Item,
	fields fieldSet) ([]BaseItemDto, error) {
	items := make([]library.Item, 0, len(list))
	for _, rec := range list {
		items = append(items, library.Item{ID: rec.ID, Kind: library.KindRecording})
	}
	state, err := h.userState(r.Context(), user, items)
	if err != nil {
		return nil, err
	}
	dtos := make([]BaseItemDto, 0, len(list))
	for _, rec := range list {
		dtos = append(dtos, h.recordingDto(r, user, rec, channels[rec.Channel], fields, false, state))
	}
	return dtos, nil
}

// recordings lists the recordings the caller sees, the latest first, as
// Jellyfin lists the videos of its recordings folders.
func (h *Handler) recordings(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	start, limit := b.paging(r, -1)
	inProgress, inProgressSet := b.bool(r, "isInProgress")
	flagFilters := map[string]bool{}
	for name, flag := range map[string]string{"isMovie": "movie", "isSeries": "series", "isKids": "kids", "isSports": "sports", "isNews": "news"} {
		if value, ok := b.bool(r, name); ok {
			flagFilters[flag] = value
		}
	}
	user, ok := h.viewer(w, r, b, unknownListingUser)
	if !ok {
		return
	}
	list, channels, err := h.listedRecordings(r.Context(), user)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	channel, channelSet := parseGUID(query(r, "channelId"))
	series, seriesSet := parseGUID(query(r, "seriesTimerId"))
	list = slices.DeleteFunc(list, func(rec recordings.Recording) bool {
		flags := programFlags(library.Item{Genres: rec.Genres})
		for flag, want := range flagFilters {
			if flags[flag] != want {
				return true
			}
		}
		return inProgressSet && rec.InProgress() != inProgress || channelSet && rec.Channel != channel ||
			seriesSet && (rec.SeriesTimer == nil || *rec.SeriesTimer != series)
	})
	from, to := bounds(len(list), start, limit)
	fields := requestedFields(r)
	dtos, err := h.recordingDtos(r, user, list[from:to], channels, fields)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	if enabled, set := boolQuery(r, "enableUserData"); set && !enabled {
		for i := range dtos {
			dtos[i].UserData = UserItemData{}
		}
	}
	writeJSON(w, http.StatusOK, QueryResult{Items: dtos, TotalRecordCount: len(list), StartIndex: start})
}

// recordingsFolder describes the folder recordings are listed in, as
// Jellyfin describes its recordings library.
func (h *Handler) recordingsFolder(r *http.Request, user accounts.User) (BaseItemDto, error) {
	folder := library.Item{ID: recordingsFolderID, Kind: library.KindLibrary, Name: "Recordings"}
	dtos, err := h.folderDtos(r, user, []library.Item{folder})
	if err != nil {
		return BaseItemDto{}, err
	}
	dtos[0].DateLastMediaAdded = nil
	return dtos[0], nil
}

// recordingFolders lists the recordings folder, when the server records.
func (h *Handler) recordingFolders(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	user, ok := h.viewer(w, r, b, unknownListingUser)
	if !ok {
		return
	}
	result := QueryResult{Items: []BaseItemDto{}}
	if h.recordable() && user.LiveTv {
		folder, err := h.recordingsFolder(r, user)
		if err != nil {
			h.internalError(w, r, err)
			return
		}
		result.Items, result.TotalRecordCount = append(result.Items, folder), 1
	}
	writeJSON(w, http.StatusOK, result)
}

// recordingListing answers listings of the recordings folder. It reports
// whether it answered.
func (h *Handler) recordingListing(w http.ResponseWriter, r *http.Request, user accounts.User, parent accounts.ID, hasParent bool, start, limit int) bool {
	if !hasParent || parent != recordingsFolderID || !h.recordable() {
		return false
	}
	list, channels, err := h.listedRecordings(r.Context(), user)
	if err != nil {
		h.internalError(w, r, err)
		return true
	}
	from, to := bounds(len(list), start, limit)
	dtos, err := h.recordingDtos(r, user, list[from:to], channels, requestedFields(r))
	if err != nil {
		h.internalError(w, r, err)
		return true
	}
	writeJSON(w, http.StatusOK, QueryResult{Items: dtos, TotalRecordCount: len(list), StartIndex: start})
	return true
}

// describeRecording answers the description of a recording, or of the
// recordings folder. It reports whether id is one the caller sees.
func (h *Handler) describeRecording(w http.ResponseWriter, r *http.Request, user accounts.User, id accounts.ID) bool {
	if !h.recordable() || !user.LiveTv {
		return false
	}
	if id == recordingsFolderID {
		folder, err := h.recordingsFolder(r, user)
		if err != nil {
			h.internalError(w, r, err)
			return true
		}
		writeJSON(w, http.StatusOK, folder)
		return true
	}
	rec, channel, err := h.visibleRecording(r.Context(), user, id)
	if errors.Is(err, recordings.ErrNotFound) {
		return false
	}
	if err != nil {
		h.internalError(w, r, err)
		return true
	}
	state, err := h.userState(r.Context(), user, []library.Item{{ID: rec.ID, Kind: library.KindRecording}})
	if err != nil {
		h.internalError(w, r, err)
		return true
	}
	writeJSON(w, http.StatusOK, h.recordingDto(r, user, rec, channel, requestedFields(r), true, state))
	return true
}

// recording describes a recording.
func (h *Handler) recording(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	id := b.pathID(r, "recordingId")
	user, ok := h.viewer(w, r, b, notFoundProblem)
	if !ok {
		return
	}
	if !h.describeRecording(w, r, user, id) {
		notFoundProblem(w)
	}
}

// deleteRecording deletes a recording and its file, stopping it first when
// it is under way.
func (h *Handler) deleteRecording(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	id := b.pathID(r, "recordingId")
	if len(b) > 0 {
		validationProblem(w, b)
		return
	}
	rec, _, err := h.visibleRecording(r.Context(), callerFrom(r.Context()).User, id)
	if err == nil {
		err = h.Recordings.DeleteRecording(r.Context(), rec.ID)
	}
	if err != nil {
		h.recordingError(w, r, err, notFoundProblem)
		return
	}
	h.Activity.RecordingDeleted(r.Context(), callerFrom(r.Context()).User, rec.Name)
	w.WriteHeader(http.StatusNoContent)
}

// recordingTitle resolves a recording a player opens, as an item. Only
// finished recordings have a file to play.
func (h *Handler) recordingTitle(ctx context.Context, user accounts.User, id accounts.ID) (library.Item, bool) {
	rec, channel, err := h.visibleRecording(ctx, user, id)
	if err != nil {
		return library.Item{}, false
	}
	return recordingItem(rec, channel), true
}

// recordingVersions is the version a finished recording plays: its file.
func (h *Handler) recordingVersions(ctx context.Context, item library.Item) []library.Version {
	rec, err := h.Recordings.Recording(ctx, item.ID)
	if err != nil || rec.InProgress() {
		return nil
	}
	return []library.Version{h.Playback.FileVersion(rec.ID, rec.Name, h.Recordings.Path(rec), rec.Size)}
}

// version returns the version of item the player asks for: a title's
// version, or a recording's file.
func (h *Handler) version(ctx context.Context, user accounts.User, item library.Item, id accounts.ID) (library.Version, error) {
	if item.Kind != library.KindRecording {
		return h.Library.Version(ctx, user, item.ID, id)
	}
	versions := h.recordingVersions(ctx, item)
	if len(versions) == 0 {
		return library.Version{}, library.ErrNotFound
	}
	return versions[0], nil
}

// serveRecording serves a finished recording's file as it is.
func (h *Handler) serveRecording(w http.ResponseWriter, r *http.Request, version library.Version, contentType string) {
	file, err := os.Open(h.Recordings.Path(recordings.Recording{File: version.Filename}))
	if err != nil {
		processingError(w, http.StatusNotFound)
		return
	}
	defer file.Close()
	if contentType == "" {
		contentType = mimeTypes[containerOfName(version.Filename)]
	}
	w.Header().Set("Content-Type", contentType)
	http.ServeContent(w, r, "", time.Time{}, file)
}

// liveRecordingFile streams a recording under way as it is written, from
// its start; recordingId names the recording or its timer. Jellyfin
// serves it to anyone; Polyfin to the players of users who see it, as
// other media (see streamAccess).
func (h *Handler) liveRecordingFile(w http.ResponseWriter, r *http.Request) {
	id, ok := parseGUID(r.PathValue("recordingId"))
	if !ok || !h.recordable() {
		notFoundProblem(w)
		return
	}
	user, _, ok := h.streamAccess(r)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	part, done, ok := h.Recordings.Active(id)
	if !ok {
		notFoundProblem(w)
		return
	}
	const liveWaitForFile = 30 * time.Second
	// FFmpeg creates the file once the stream opens: it is waited for
	// while the recording goes on.
	file, err := os.Open(part)
	for wait := time.Now().Add(liveWaitForFile); errors.Is(err, os.ErrNotExist) && time.Now().Before(wait); file, err = os.Open(part) {
		select {
		case <-r.Context().Done():
			return
		case <-done:
			wait = time.Time{}
		case <-time.After(200 * time.Millisecond):
		}
	}
	if err != nil {
		notFoundProblem(w)
		return
	}
	defer file.Close()
	if !h.mayWatchActive(r.Context(), user, id) {
		notFoundProblem(w)
		return
	}
	w.Header().Set("Content-Type", "video/mp2t")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)
	buffer := make([]byte, 256<<10)
	for {
		n, err := file.Read(buffer)
		if n > 0 {
			if _, err := w.Write(buffer[:n]); err != nil {
				return
			}
			continue
		}
		if err != nil && !errors.Is(err, io.EOF) {
			return
		}
		if flusher != nil {
			flusher.Flush()
		}
		// The end of what is written: more comes until the recording stops.
		select {
		case <-r.Context().Done():
			return
		case <-done:
			_, _ = io.Copy(w, file)
			return
		case <-time.After(500 * time.Millisecond):
		}
	}
}

// mayWatchActive reports whether user sees the recording under way id
// names, by itself or by its timer.
func (h *Handler) mayWatchActive(ctx context.Context, user accounts.User, id accounts.ID) bool {
	if _, _, err := h.visibleRecording(ctx, user, id); err == nil {
		return true
	}
	list, err := h.Recordings.Recordings(ctx)
	if err != nil {
		return false
	}
	for _, rec := range list {
		if rec.InProgress() && rec.Timer != nil && *rec.Timer == id {
			_, _, err := h.visibleRecording(ctx, user, rec.ID)
			return err == nil
		}
	}
	return false
}

// addTimers adds to the DTOs of programmes their timers and series timers,
// as Jellyfin describes guide programmes.
func (h *Handler) addTimers(ctx context.Context, dtos []BaseItemDto) {
	if !h.recordable() {
		return
	}
	var ids []accounts.ID
	for _, dto := range dtos {
		if id, ok := parseGUID(dto.Id); ok && dto.Type == "Program" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return
	}
	timers, err := h.Recordings.ProgramTimers(ctx, ids)
	var series []recordings.SeriesTimer
	if err == nil {
		series, err = h.Recordings.SeriesTimers(ctx)
	}
	if err != nil {
		if ctx.Err() == nil {
			h.Logger.Warn("The timers of programmes could not be listed", "error", err)
		}
		return
	}
	for i := range dtos {
		id, ok := parseGUID(dtos[i].Id)
		if !ok || dtos[i].Type != "Program" {
			continue
		}
		if t, ok := timers[id]; ok {
			if t.Status != recordings.StatusCancelled && t.Status != recordings.StatusError {
				dtos[i].TimerId, dtos[i].Status = t.ID.String(), string(t.Status)
			}
			if t.SeriesTimer != nil {
				dtos[i].SeriesTimerId = t.SeriesTimer.String()
				continue
			}
		}
		for _, st := range series {
			if strings.EqualFold(strings.TrimSpace(st.Name), strings.TrimSpace(dtos[i].Name)) &&
				(st.RecordAnyChannel || dtos[i].ChannelId != nil && *dtos[i].ChannelId == st.Channel.String()) {
				dtos[i].SeriesTimerId = st.ID.String()
				break
			}
		}
	}
}

// recordingArtwork is the artwork of a recording: its programme's, read
// through the library so that it stays within its addon's confinement,
// else its channel's.
func (h *Handler) recordingArtwork(ctx context.Context, id accounts.ID, imageType string) (url string, confined bool, err error) {
	if !h.recordable() {
		return "", false, library.ErrNotFound
	}
	rec, err := h.Recordings.Recording(ctx, id)
	if errors.Is(err, recordings.ErrNotFound) {
		return "", false, library.ErrNotFound
	}
	if err != nil {
		return "", false, err
	}
	for _, source := range []*accounts.ID{rec.Program, &rec.Channel} {
		if source == nil {
			continue
		}
		if url, confined, err := h.Library.Artwork(ctx, *source, imageType); err == nil {
			return url, confined, nil
		}
	}
	return "", false, library.ErrNotFound
}
