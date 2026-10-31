package admin

import (
	"context"
	"net/http"
	"strings"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/iptv"
	"github.com/moodiness/polyfin/internal/library"
)

// ChannelChoices lists the channel groups and channels of the server's
// Live TV sources, which a user's or a group's channel rule names.
type ChannelChoices interface {
	ChannelChoices(ctx context.Context) ([]library.ChannelGroupChoice, []library.ChannelChoice, error)
}

// channelRuleJSON is a user's or a group's Live TV channel rule (see
// accounts.ChannelRule): mode is all, only or except; groups and channels
// are the keys channel-choices gives. With hideNewGroups, except also hides
// the groups not among knownGroups, which saving sets to the groups the
// server's sources have then, and had before.
type channelRuleJSON struct {
	Mode          string   `json:"mode"`
	Groups        []string `json:"groups"`
	Channels      []string `json:"channels"`
	HideNewGroups bool     `json:"hideNewGroups"`
	KnownGroups   []string `json:"knownGroups"`
}

func newChannelRuleJSON(rule accounts.ChannelRule) channelRuleJSON {
	mode := rule.Mode
	if mode == "" {
		mode = accounts.ChannelsAll
	}
	return channelRuleJSON{Mode: mode, Groups: append([]string{}, rule.Groups...), Channels: append([]string{}, rule.Channels...),
		HideNewGroups: rule.HideNewGroups, KnownGroups: append([]string{}, rule.KnownGroups...)}
}

// rule is the channel rule a change sets; its known groups are the
// handler's to set (see knownChannelGroups).
func (c channelRuleJSON) rule() accounts.ChannelRule {
	return accounts.ChannelRule{Mode: c.Mode, Groups: c.Groups, Channels: c.Channels, HideNewGroups: c.HideNewGroups}
}

// knownChannelGroups gives a channel rule that hides the groups added later
// the groups known now: those of the server's sources, and those the rule
// it replaces knew, so that a source that could not be read meanwhile
// keeps its groups known. It answers the request with its error and
// returns false when they cannot be read, as for an account not found.
func (h *handler) knownChannelGroups(w http.ResponseWriter, r *http.Request, rule *accounts.ChannelRule,
	previous func() (accounts.ChannelRule, error)) bool {
	if rule == nil || rule.Mode != accounts.ChannelsExcept || !rule.HideNewGroups {
		return true
	}
	before, err := previous()
	if accountError(w, err) {
		return false
	}
	if err != nil {
		h.internalError(w, r, err)
		return false
	}
	// The groups of now come first: past the bound, the oldest groups no
	// source has any more are forgotten.
	var known []string
	seen := map[string]bool{}
	add := func(key string) {
		if !seen[key] && len(known) < accounts.MaxChannelKeys {
			seen[key] = true
			known = append(known, key)
		}
	}
	if h.ChannelChoices != nil {
		groups, _, err := h.ChannelChoices.ChannelChoices(r.Context())
		if err != nil {
			h.internalError(w, r, err)
			return false
		}
		for _, group := range groups {
			add(group.Key)
		}
	}
	for _, key := range before.KnownGroups {
		add(key)
	}
	rule.KnownGroups = known
	return true
}

type channelGroupChoiceJSON struct {
	Key      string `json:"key"`
	Name     string `json:"name"`
	Source   string `json:"source"`
	Channels int    `json:"channels"`
}

type channelChoiceJSON struct {
	Key    string `json:"key"`
	Name   string `json:"name"`
	Number string `json:"number"`
	Group  string `json:"group"`
}

type channelChoicesJSON struct {
	// Groups are every channel group of the server's sources.
	Groups []channelGroupChoiceJSON `json:"groups"`
	// Channels are the page of the channels asked for, of Total.
	Channels []channelChoiceJSON `json:"channels"`
	Total    int                 `json:"total"`
	Offset   int                 `json:"offset"`
	Limit    int                 `json:"limit"`
}

// channelChoices answers the channel groups of the server's Live TV
// sources, and a page of their channels: those whose name or number holds
// q, in any case and without accents, of the group named, or with the keys
// listed (key, repeated), in the order Live TV lists them.
func (h *handler) channelChoices(w http.ResponseWriter, r *http.Request) {
	offset, limit, ok := paging(w, r)
	if !ok {
		return
	}
	result := channelChoicesJSON{Groups: []channelGroupChoiceJSON{}, Channels: []channelChoiceJSON{}, Offset: offset, Limit: limit}
	if h.ChannelChoices == nil {
		writeJSON(w, http.StatusOK, result)
		return
	}
	groups, channels, err := h.ChannelChoices.ChannelChoices(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	for _, g := range groups {
		result.Groups = append(result.Groups, channelGroupChoiceJSON{Key: g.Key, Name: g.Name, Source: g.Source, Channels: g.Channels})
	}
	query := r.URL.Query()
	folded := iptv.Fold(strings.TrimSpace(query.Get("q")))
	group := query.Get("group")
	var keys map[string]bool
	if listed := query["key"]; len(listed) > 0 {
		keys = make(map[string]bool, len(listed))
		for _, key := range listed {
			keys[key] = true
		}
	}
	for _, c := range channels {
		if group != "" && c.Group != group || keys != nil && !keys[c.Key] ||
			folded != "" && !strings.Contains(iptv.Fold(c.Name), folded) && !strings.Contains(c.Number, folded) {
			continue
		}
		if result.Total >= offset && len(result.Channels) < limit {
			result.Channels = append(result.Channels, channelChoiceJSON{Key: c.Key, Name: c.Name, Number: c.Number, Group: c.Group})
		}
		result.Total++
	}
	writeJSON(w, http.StatusOK, result)
}
