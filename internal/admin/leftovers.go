package admin

import (
	"context"
	"fmt"
	"net/http"
	"slices"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/i18n"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/notifications"
)

// Leftovers counts what removing a source loses, and finds the sources own
// channels air that are gone (see package library's leftovers.go).
type Leftovers interface {
	RemovalLoss(ctx context.Context, scope addons.Scope, id accounts.ID) (library.RemovalLoss, error)
	MissingSources(ctx context.Context, ids []accounts.ID) ([]accounts.ID, error)
}

// The codes of the problems what removed sources left makes: a smart list
// left with no catalog, Name and Owner (nil for the server's) being the
// list's; an own channel airing a source gone, Name being the channel's.
const (
	problemSmartListEmpty       = "smart_list_empty"
	problemVirtualChannelSource = "virtual_channel_source"
)

// addonRemoval answers what removing the scope's addon would lose, for the
// confirmation to tell (see library.RemovalLoss).
func (h *handler) addonRemoval(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scope(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var loss library.RemovalLoss
	var err error
	if h.Leftovers != nil {
		loss, err = h.Leftovers.RemovalLoss(r.Context(), scope, id)
	}
	if addonError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		HomeRows        int `json:"homeRows"`
		Libraries       int `json:"libraries"`
		HiddenLibraries int `json:"hiddenLibraries"`
		Recordings      int `json:"recordings"`
	}{loss.HomeRows, loss.Libraries, loss.HiddenLibraries, loss.Recordings})
}

// leftoverProblems finds the smart lists left with no catalog, the
// server's first, and the own channels airing a source gone.
func (h *handler) leftoverProblems(ctx context.Context) ([]healthProblemJSON, error) {
	var problems []healthProblemJSON
	if h.SmartLists != nil {
		emptied, err := h.SmartLists.Emptied(ctx)
		if err != nil {
			return nil, err
		}
		var owners map[accounts.ID]string
		for _, l := range emptied {
			p := healthProblemJSON{Key: problemSmartListEmpty + ":" + l.ID.String(), Code: problemSmartListEmpty, Tone: notifications.SeverityWarning,
				To: "/libraries/smart/" + l.ID.String(), Name: l.Name}
			if l.Owner != nil {
				if owners == nil {
					if owners, err = h.userNames(ctx); err != nil {
						return nil, err
					}
				}
				p.Owner, p.To = &ownerJSON{ID: l.Owner.String(), Name: owners[*l.Owner]}, "/users/"+l.Owner.String()
			}
			problems = append(problems, p)
		}
	}
	if h.VirtualChannels == nil || h.Leftovers == nil {
		return problems, nil
	}
	channels, err := h.VirtualChannels.Channels(ctx)
	if err != nil {
		return nil, err
	}
	var ids []accounts.ID
	for _, c := range channels {
		for _, item := range c.Source.Items {
			ids = append(ids, item.ID)
		}
		for _, b := range c.Blocks {
			for _, item := range b.Source.Items {
				ids = append(ids, item.ID)
			}
		}
	}
	if len(ids) == 0 {
		return problems, nil
	}
	missing, err := h.Leftovers.MissingSources(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, c := range channels {
		airs := slices.Clone(c.Source.Items)
		for _, b := range c.Blocks {
			airs = append(airs, b.Source.Items...)
		}
		for _, item := range airs {
			if slices.Contains(missing, item.ID) {
				problems = append(problems, healthProblemJSON{Key: problemVirtualChannelSource + ":" + c.ID.String(), Code: problemVirtualChannelSource,
					Tone: notifications.SeverityWarning, To: "/live-tv/channels/" + c.ID.String(), Name: c.Name})
				break
			}
		}
	}
	return problems, nil
}

// userNames names every user by identifier.
func (h *handler) userNames(ctx context.Context) (map[accounts.ID]string, error) {
	users, err := h.Accounts.Users(ctx)
	if err != nil {
		return nil, err
	}
	names := make(map[accounts.ID]string, len(users))
	for _, u := range users {
		names[u.ID] = u.Name
	}
	return names, nil
}

// leftoverProblemText tells a problem of leftoverProblems in a server
// language, empty for another problem.
func leftoverProblemText(p healthProblemJSON, language string) string {
	switch p.Code {
	case problemSmartListEmpty:
		if p.Owner != nil {
			return fmt.Sprintf(i18n.Text{
				"%[2]s’s smart list %[1]s lists nothing: the catalogs it drew from are gone. Other catalogs are to be chosen for it.",
				"La liste intelligente %[1]s de %[2]s ne liste rien\u202f: les catalogues dont elle tirait ses titres ont disparu. Il faut lui en choisir d’autres.",
				"Die intelligente Liste %[1]s von %[2]s listet nichts: Die Kataloge, aus denen sie ihre Titel nahm, sind verschwunden. Für sie sind andere Kataloge zu wählen.",
				"La lista inteligente %[1]s de %[2]s no lista nada: los catálogos de los que tomaba sus títulos han desaparecido. Hay que elegirle otros.",
				"La lista intelligente %[1]s di %[2]s non elenca nulla: i cataloghi da cui prendeva i titoli sono spariti. Bisogna sceglierne altri.",
				"A lista inteligente %[1]s de %[2]s não lista nada: os catálogos de onde tirava os títulos sumiram. É preciso escolher outros para ela.",
			}.In(language), p.Name, p.Owner.Name)
		}
		return fmt.Sprintf(i18n.Text{
			"The smart list %s lists nothing: the catalogs it drew from are gone. Choose other catalogs for it.",
			"La liste intelligente %s ne liste rien\u202f: les catalogues dont elle tirait ses titres ont disparu. Choisissez-lui d’autres catalogues.",
			"Die intelligente Liste %s listet nichts: Die Kataloge, aus denen sie ihre Titel nahm, sind verschwunden. Wähle andere Kataloge für sie.",
			"La lista inteligente %s no lista nada: los catálogos de los que tomaba sus títulos han desaparecido. Elija otros catálogos para ella.",
			"La lista intelligente %s non elenca nulla: i cataloghi da cui prendeva i titoli sono spariti. Scegli altri cataloghi per lei.",
			"A lista inteligente %s não lista nada: os catálogos de onde tirava os títulos sumiram. Escolha outros catálogos para ela.",
		}.In(language), p.Name)
	case problemVirtualChannelSource:
		return fmt.Sprintf(i18n.Text{
			"The channel %s airs a source that no longer exists. Choose another source for it.",
			"La chaîne %s diffuse une source qui n’existe plus. Choisissez-lui une autre source.",
			"Der Kanal %s sendet eine Quelle, die es nicht mehr gibt. Wähle eine andere Quelle für ihn.",
			"El canal %s emite una fuente que ya no existe. Elija otra fuente para él.",
			"Il canale %s trasmette una fonte che non esiste più. Scegli un’altra fonte per lui.",
			"O canal %s transmite uma fonte que não existe mais. Escolha outra fonte para ele.",
		}.In(language), p.Name)
	}
	return ""
}
