package jellyfin

import (
	"net/http"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/streamyfin"
)

// Streamyfin, a Jellyfin app, asks the server for the settings of its
// server plugin, /Streamyfin/config, each time it comes back to the
// foreground. Polyfin answers them with the rows of its home screen that an
// administrator chose (see streamyfin.Store): Streamyfin then shows these
// rows instead of its own home screen. The rows are locked, as a plugin
// setting the user may not change: once Polyfin answers none, Streamyfin
// shows its own home screen again, which an unlocked setting, kept by the
// app, would not do.

// streamyfinConfig is the answer of /Streamyfin/config: the plugin's
// settings, each locked or not.
type streamyfinConfig struct {
	Settings map[string]streamyfinSetting `json:"settings"`
}

type streamyfinSetting struct {
	Locked bool `json:"locked"`
	Value  any  `json:"value"`
}

type streamyfinHome struct {
	Sections []streamyfinSection `json:"sections"`
}

// streamyfinSection is a row of Streamyfin's home screen: its items are
// the user's next episodes (NextUp), a listing of /Items (Items), or the
// answer of a Jellyfin endpoint (Custom). Orientation "horizontal" shows
// wide images, "vertical" posters.
type streamyfinSection struct {
	Title       string             `json:"title"`
	Orientation string             `json:"orientation"`
	Items       *streamyfinItems   `json:"items,omitempty"`
	NextUp      *struct{}          `json:"nextUp,omitempty"`
	Custom      *streamyfinRequest `json:"custom,omitempty"`
}

// streamyfinItems is the /Items listing of a row, which Streamyfin makes
// recursive.
type streamyfinItems struct {
	ParentID         string   `json:"parentId"`
	IncludeItemTypes []string `json:"includeItemTypes,omitempty"`
}

type streamyfinRequest struct {
	Endpoint string            `json:"endpoint"`
	Query    map[string]string `json:"query,omitempty"`
}

// streamyfinConfigOf answers /Streamyfin/config: the rows the user can see
// of those an administrator chose, in order. Without any, it answers 404,
// as a server without Streamyfin's plugin does, and Streamyfin shows its
// own home screen.
func (h *Handler) streamyfinConfigOf(w http.ResponseWriter, r *http.Request) {
	user := callerFrom(r.Context()).User
	if h.Streamyfin == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	rows, err := h.Streamyfin.Rows(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	sections, err := h.streamyfinSections(r, user, rows)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	if len(sections) == 0 {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, streamyfinConfig{Settings: map[string]streamyfinSetting{
		"home": {Locked: true, Value: streamyfinHome{Sections: sections}},
	}})
}

// streamyfinSections makes the rows user can see into sections: the rows
// of their own lists, and those of the libraries they see and of the
// collections in them.
func (h *Handler) streamyfinSections(r *http.Request, user accounts.User, rows []streamyfin.Row) ([]streamyfinSection, error) {
	libraries, err := h.Library.Libraries(r.Context(), user)
	if err != nil {
		return nil, err
	}
	visible := make(map[accounts.ID]library.Item, len(libraries))
	for _, l := range libraries {
		visible[l.ID] = l
	}
	language := h.Accounts.Settings().Language
	sections := make([]streamyfinSection, 0, len(rows))
	for _, row := range rows {
		section := streamyfinSection{Title: row.Title}
		switch row.Kind {
		case streamyfin.KindResume:
			section.Orientation = "horizontal"
			section.Custom = &streamyfinRequest{Endpoint: "/UserItems/Resume", Query: map[string]string{
				"mediaTypes": "Video", "fields": "PrimaryImageAspectRatio", "enableImageTypes": "Primary,Backdrop,Thumb", "imageTypeLimit": "1"}}
		case streamyfin.KindNextUp:
			section.Orientation = "horizontal"
			section.NextUp = &struct{}{}
		case streamyfin.KindLibrary:
			l, ok := visible[row.Item]
			if !ok {
				continue
			}
			if section.Title == "" {
				section.Title = l.Name
			}
			// A collection library lists its collections, whose images
			// are wide; any other its titles' posters.
			section.Orientation = "vertical"
			if l.CollectionType == "boxsets" {
				section.Orientation = "horizontal"
			}
			section.Items = &streamyfinItems{ParentID: row.Item.String()}
		case streamyfin.KindCollection:
			collection, ok := h.visibleCollection(r, user, row.Item, visible)
			if !ok {
				continue
			}
			if section.Title == "" {
				section.Title = collection.Name
			}
			section.Orientation = "vertical"
			section.Items = &streamyfinItems{ParentID: row.Item.String(), IncludeItemTypes: []string{"Movie", "Series"}}
		default:
			continue
		}
		if section.Title == "" {
			section.Title = streamyfin.DefaultName(row.Kind, language)
		}
		sections = append(sections, section)
	}
	return sections, nil
}

// visibleCollection returns a collection user can open, under one of the
// libraries they see.
func (h *Handler) visibleCollection(r *http.Request, user accounts.User, id accounts.ID, libraries map[accounts.ID]library.Item) (library.Item, bool) {
	item, err := h.Library.Item(r.Context(), user, id)
	if err != nil || item.Kind != library.KindCollection {
		return library.Item{}, false
	}
	ancestors, err := h.Library.Ancestors(r.Context(), user, id)
	if err != nil {
		return library.Item{}, false
	}
	for _, ancestor := range ancestors {
		if _, ok := libraries[ancestor.ID]; ok {
			return item, true
		}
	}
	return library.Item{}, false
}
