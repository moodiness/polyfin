package notifications

import (
	"bytes"
	"fmt"
	"html/template"
	"strings"

	"github.com/moodiness/polyfin/internal/accounts"
)

// summaryHTML is the HTML part of a weekly summary's email, laid out as
// the other emails (see emailHTML): its week and figures, then a list for
// each part of it, each title linking to its page.
var summaryHTML = template.Must(template.New("summary").Parse(`<!DOCTYPE html>
<html lang="{{.Language}}">
<head><meta charset="utf-8"><title>{{.Title}}</title></head>
<body style="font-family: system-ui, sans-serif; color: #1f2328;">
<h1 style="font-size: 18px;">{{.Title}}</h1>
<p>{{.Week}}<br><strong>{{.Played}}</strong></p>
{{range .Sections}}<h2 style="font-size: 15px; margin: 20px 0 6px;">{{.Heading}}</h2>
<ul style="margin: 0; padding-left: 20px;">
{{range .Items}}<li>{{if .URL}}<a href="{{.URL}}">{{.Name}}</a>{{else}}{{.Name}}{{end}}{{if .Detail}} <span style="color: #6b7280;">· {{.Detail}}</span>{{end}}</li>
{{end}}</ul>
{{if .More}}<p style="color: #6b7280; margin: 4px 0 0;">{{.More}}</p>
{{end}}{{end}}{{if .URL}}<p style="margin-top: 20px;"><a href="{{.URL}}">{{.Open}}</a></p>
{{end}}<p style="color: #6b7280; font-size: 12px;">{{.Footer}}</p>
</body>
</html>
`))

// summarySection is a part of a summary's email: a heading, its items, and
// how many more there are.
type summarySection struct {
	Heading string
	Items   []summaryItem
	More    string
}

// summaryItem is a title, a user or a problem, its page when there is
// one, and what to say of it.
type summaryItem struct {
	Name, URL, Detail string
}

// summaryEmail writes the HTML of ev's weekly summary to page, and returns
// its plain text, without footer: the week, the figures, and its parts as
// lists, with the address of each title when the public address is set.
func (s *Service) summaryEmail(page *bytes.Buffer, language string, ev Event, footer string) (string, error) {
	sum := *ev.Summary
	user := sum.Scope == ScopeUser
	var sections []summarySection
	titles := func(heading string, list []SummaryTitleJSON) {
		if len(list) == 0 {
			return
		}
		section := summarySection{Heading: heading}
		for _, t := range list {
			section.Items = append(section.Items, summaryItem{Name: t.Name, URL: s.itemLink(t.ID),
				Detail: s.duration(t.Played) + ", " + s.playsText(t.Plays)})
		}
		sections = append(sections, section)
	}
	if user {
		titles(s.phrase("Movies watched", "Films regardés"), sum.Movies)
		titles(s.phrase("Series watched", "Séries regardées"), sum.Series)
	} else {
		titles(s.phrase("Most watched movies", "Films les plus regardés"), sum.Movies)
		titles(s.phrase("Most watched series", "Séries les plus regardées"), sum.Series)
	}
	if sum.NewEpisodes > 0 {
		section := summarySection{Heading: s.episodesHeading(sum)}
		for _, e := range sum.Episodes {
			name := fmt.Sprintf("%s S%02dE%02d", e.SeriesName, e.Season, e.Number)
			section.Items = append(section.Items, summaryItem{Name: name, URL: s.itemLink(e.ID), Detail: e.Name})
		}
		if more := sum.NewEpisodes - len(sum.Episodes); more > 0 {
			section.More = s.count(more, "And %d more.", "And %d more.", "Et %d autre.", "Et %d autres.")
		}
		sections = append(sections, section)
	}
	if len(sum.Users) > 0 {
		section := summarySection{Heading: s.phrase("New users", "Nouveaux utilisateurs")}
		for _, u := range sum.Users {
			item := summaryItem{Name: u.Name}
			if link := s.adminLink("/users/" + u.ID); link != nil {
				item.URL = *link
			}
			if u.InvitedBy != nil {
				item.Detail = s.phrase("invited by %s", "invité par %s", u.InvitedBy.Name)
			}
			section.Items = append(section.Items, item)
		}
		sections = append(sections, section)
	}
	if sum.AddedFiles > 0 {
		section := summarySection{Heading: s.addedHeading(sum)}
		for _, a := range sum.Added {
			item := summaryItem{Name: a.Name}
			if a.Kind == "show" || a.Files > 1 {
				item.Detail = s.count(a.Files, "%d file", "%d files", "%d fichier", "%d fichiers")
			}
			section.Items = append(section.Items, item)
		}
		sections = append(sections, section)
	}
	if len(sum.Problems) > 0 {
		section := summarySection{Heading: s.problemsHeading(sum)}
		for _, p := range sum.Problems {
			item := summaryItem{Name: p.Text, Detail: s.phrase("Warning", "Avertissement")}
			if p.Severity == SeverityError {
				item.Detail = s.phrase("Error", "Erreur")
			}
			if link := s.healthLink(p.page); link != nil {
				item.URL = *link
			}
			section.Items = append(section.Items, item)
		}
		sections = append(sections, section)
	}
	url := ""
	if ev.URL != nil {
		url = *ev.URL
	}
	open := s.phrase("Open the statistics", "Ouvrir les statistiques")
	if err := summaryHTML.Execute(page, map[string]any{"Language": language, "Title": ev.Title, "Week": s.weekLine(sum),
		"Played": s.playedLine(sum), "Sections": sections, "URL": url, "Open": open, "Footer": footer}); err != nil {
		return "", err
	}

	var text strings.Builder
	text.WriteString(s.weekLine(sum) + "\n" + s.playedLine(sum) + "\n")
	for _, section := range sections {
		text.WriteString("\n" + section.Heading + "\n")
		for _, item := range section.Items {
			text.WriteString("- " + item.Name)
			if item.Detail != "" {
				text.WriteString(" · " + item.Detail)
			}
			text.WriteString("\n")
			if item.URL != "" {
				text.WriteString("  " + item.URL + "\n")
			}
		}
		if section.More != "" {
			text.WriteString(section.More + "\n")
		}
	}
	if url != "" {
		text.WriteString("\n" + s.phrase("%s: %s", "%s : %s", open, url) + "\n")
	}
	return text.String(), nil
}

// itemLink is the address of the page of the item id in the web client,
// empty without one.
func (s *Service) itemLink(id string) string {
	parsed, err := accounts.ParseID(id)
	if err != nil {
		return ""
	}
	if link := s.webLink(parsed); link != nil {
		return *link
	}
	return ""
}
