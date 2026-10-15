package jellyfin

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/iptv"
	"github.com/moodiness/polyfin/internal/stremio"
)

// Live TV tuners: the server's HDHomeRun tuners, which are IPTV sources of
// the shared scope (see iptv.Service.Tuners), shown as Jellyfin's tuner
// hosts of type hdhomerun. Only administrators add, change and remove
// them, as in Jellyfin.

// tunerType is Jellyfin's type of HDHomeRun tuner hosts.
const tunerType = "hdhomerun"

// TunerHostType is a kind of tuner the server takes.
type TunerHostType struct {
	Name string
	Id   string
}

// TunerHostInfo is Jellyfin's tuner host, in its order. Polyfin's tuners
// take Jellyfin's defaults for the settings it does not have: streams are
// shared between viewers, as Polyfin always does.
type TunerHostInfo struct {
	Id                            *string
	Url                           string
	Type                          string
	DeviceId                      string
	FriendlyName                  string
	ImportFavoritesOnly           bool
	AllowHWTranscoding            bool
	AllowFmp4TranscodingContainer bool
	AllowStreamSharing            bool
	FallbackMaxStreamingBitrate   int
	EnableStreamLooping           bool
	Source                        *string
	TunerCount                    int
	UserAgent                     *string
	IgnoreDts                     bool
	ReadAtNativeFramerate         bool
}

// newTunerHostInfo is a tuner host with Jellyfin's defaults.
func newTunerHostInfo(tuner iptv.Tuner) TunerHostInfo {
	return TunerHostInfo{Url: tuner.BaseURL, Type: tunerType, DeviceId: tuner.DeviceID, FriendlyName: tuner.FriendlyName,
		AllowHWTranscoding: true, AllowStreamSharing: true, FallbackMaxStreamingBitrate: 30000000, TunerCount: tuner.TunerCount,
		IgnoreDts: true}
}

// tunerHostInfo describes a tuner of the server.
func tunerHostInfo(source iptv.Source) TunerHostInfo {
	tuner := iptv.Tuner{BaseURL: source.Addon.ManifestURL, FriendlyName: source.Addon.Manifest.Name}
	if source.Tuner != nil {
		tuner.DeviceID = source.Tuner.Device
	}
	if source.MaxConnections != nil {
		tuner.TunerCount = *source.MaxConnections
	}
	info := newTunerHostInfo(tuner)
	id := source.Addon.ID.String()
	info.Id = &id
	info.ImportFavoritesOnly = source.Options.FavoritesOnly
	return info
}

// LiveTvOptions is Jellyfin's Live TV configuration, in its order: its
// defaults, with the server's tuners. Polyfin's recordings and guides
// have their own settings, in the admin app.
type LiveTvOptions struct {
	GuideDays                                *int
	RecordingPath                            *string
	MovieRecordingPath                       *string
	SeriesRecordingPath                      *string
	EnableRecordingSubfolders                bool
	EnableOriginalAudioWithEncodedRecordings bool
	TunerHosts                               []TunerHostInfo
	ListingProviders                         []struct{}
	PrePaddingSeconds                        int
	PostPaddingSeconds                       int
	MediaLocationsCreated                    []string
	RecordingPostProcessor                   *string
	RecordingPostProcessorArguments          string
	SaveRecordingNFO                         bool
	SaveRecordingImages                      bool
}

// liveTvConfiguration answers /System/Configuration/livetv. Its tuners are
// listed to administrators only: a tuner's address is on the server's
// local network, which members have no use for.
func (h *Handler) liveTvConfiguration(w http.ResponseWriter, r *http.Request) {
	options := LiveTvOptions{TunerHosts: []TunerHostInfo{}, ListingProviders: []struct{}{}, MediaLocationsCreated: []string{},
		RecordingPostProcessorArguments: `"{path}"`, SaveRecordingNFO: true, SaveRecordingImages: true}
	if h.IPTV != nil && callerFrom(r.Context()).User.IsAdministrator {
		tuners, err := h.IPTV.Tuners(r.Context())
		if err != nil {
			h.internalError(w, r, err)
			return
		}
		for _, tuner := range tuners {
			options.TunerHosts = append(options.TunerHosts, tunerHostInfo(tuner))
		}
	}
	writeJSON(w, http.StatusOK, options)
}

// tunerHostTypes lists the kinds of tuner the server takes: HDHomeRun.
func (h *Handler) tunerHostTypes(w http.ResponseWriter, _ *http.Request) {
	types := []TunerHostType{}
	if h.IPTV != nil {
		types = append(types, TunerHostType{Name: "HD Homerun", Id: tunerType})
	}
	writeJSON(w, http.StatusOK, types)
}

// discoverTuners lists the tuners that answer HDHomeRun's discovery on the
// local network; with newDevicesOnly, those that are not sources yet.
func (h *Handler) discoverTuners(w http.ResponseWriter, r *http.Request) {
	hosts := []TunerHostInfo{}
	if h.IPTV != nil {
		found, err := h.IPTV.FindTuners(r.Context())
		if err != nil {
			h.internalError(w, r, err)
			return
		}
		newOnly, _ := boolQuery(r, "newDevicesOnly")
		for _, tuner := range found {
			if newOnly && tuner.Source != nil {
				continue
			}
			hosts = append(hosts, newTunerHostInfo(tuner.Tuner))
		}
	}
	writeJSON(w, http.StatusOK, hosts)
}

// saveTunerHost adds a tuner, or changes the one its Id names: its address
// and whether only its favorite channels are kept. A tuner is read first,
// and saved only if it answers. Another type than hdhomerun is unknown, as
// in Jellyfin.
func (h *Handler) saveTunerHost(w http.ResponseWriter, r *http.Request) {
	var info struct {
		Id                  string
		Url                 string
		Type                string
		FriendlyName        string
		ImportFavoritesOnly bool
	}
	present, ok := readJSONBody(w, r, &info, "tunerHostInfo")
	if !ok {
		return
	}
	if !present {
		requireBody(w, "tunerHostInfo")
		return
	}
	if h.IPTV == nil || !strings.EqualFold(info.Type, tunerType) {
		processingError(w, http.StatusNotFound)
		return
	}
	// The tuner is worth keeping when the app stops waiting.
	ctx := context.WithoutCancel(r.Context())
	user := callerFrom(ctx).User
	options := &iptv.OptionsPatch{FavoritesOnly: &info.ImportFavoritesOnly}
	var id accounts.ID
	var err error
	if info.Id == "" {
		var addon addons.Addon
		addon, err = h.IPTV.Add(ctx, addons.Shared(), iptv.NewSource{Name: info.FriendlyName,
			Account: iptv.Account{Kind: addons.KindHDHomeRun, URL: info.Url}, Options: options}, false)
		if err == nil {
			id = addon.ID
			h.Activity.AddonInstalled(ctx, user, addon.Manifest.Name, addon.Manifest.Version, true)
		}
	} else {
		var current iptv.Source
		if current, err = h.tunerHost(ctx, info.Id); err == nil {
			id = current.Addon.ID
			changes := iptv.Changes{Options: options}
			// Another address is read before it is kept.
			if url := strings.TrimSuffix(strings.TrimSpace(info.Url), "/"); url != "" && !strings.EqualFold(url, current.Addon.ManifestURL) {
				changes.Account = &iptv.Account{URL: info.Url}
			}
			err = h.IPTV.Update(ctx, addons.Shared(), id, changes, false)
		}
	}
	var source iptv.Source
	if err == nil {
		source, err = h.IPTV.Source(ctx, addons.Shared(), id)
	}
	switch {
	case errors.Is(err, addons.ErrNotFound), errors.Is(err, iptv.ErrNotTuner):
		processingError(w, http.StatusNotFound)
	case errors.Is(err, iptv.ErrInvalidAddress), errors.Is(err, iptv.ErrInvalidOptions), errors.Is(err, iptv.ErrTunerAdded),
		errors.Is(err, addons.ErrExists):
		processingError(w, http.StatusBadRequest)
	case errors.Is(err, stremio.ErrUnreachable), errors.Is(err, iptv.ErrInvalidList), errors.Is(err, iptv.ErrTooLarge):
		// Jellyfin fails the same way when the tuner does not answer.
		processingError(w, http.StatusInternalServerError)
	case err != nil:
		h.internalError(w, r, err)
	default:
		writeJSON(w, http.StatusOK, tunerHostInfo(source))
	}
}

// tunerHost finds the tuner a tuner host's Id names.
func (h *Handler) tunerHost(ctx context.Context, raw string) (iptv.Source, error) {
	id, ok := parseGUID(raw)
	if !ok {
		return iptv.Source{}, addons.ErrNotFound
	}
	source, err := h.IPTV.Source(ctx, addons.Shared(), id)
	if err == nil && !source.Addon.Tuner() {
		err = iptv.ErrNotTuner
	}
	return source, err
}

// deleteTunerHost removes the tuner id names. Like Jellyfin, it answers
// 204 for a tuner that is not there; an IPTV source that is not a tuner
// is left be.
func (h *Handler) deleteTunerHost(w http.ResponseWriter, r *http.Request) {
	id, ok := parseGUID(query(r, "id"))
	if !ok || h.IPTV == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	addon, err := h.IPTV.RemoveTuner(r.Context(), id)
	switch {
	case errors.Is(err, addons.ErrNotFound), errors.Is(err, iptv.ErrNotTuner):
	case err != nil:
		h.internalError(w, r, err)
		return
	default:
		h.Activity.AddonRemoved(r.Context(), callerFrom(r.Context()).User, addon.Manifest.Name, true)
		// The images uploaded for its library go with the tuner, even when
		// the request is abandoned; the tuner is gone either way.
		if h.Library != nil {
			if err := h.Library.DeleteLibraryImages(context.WithoutCancel(r.Context()), addon); err != nil {
				h.Logger.Warn("The images of a removed tuner's library could not be deleted", "tuner", addon.Manifest.Name, "error", err)
			}
		}
	}
	w.WriteHeader(http.StatusNoContent)
}
