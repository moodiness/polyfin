package notifications

import (
	"bytes"
	"fmt"
	"html/template"
	"strings"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/i18n"
)

// summaryHTML is the HTML part of a summary's email, laid out as the other
// emails (see emailHTML): its period and figures, then a list for each
// part of it, each title linking to its page.
var summaryHTML = template.Must(template.New("summary").Parse(`<!DOCTYPE html>
<html lang="{{.Language}}">
<head><meta charset="utf-8"><title>{{.Title}}</title></head>
<body style="font-family: system-ui, sans-serif; color: #1f2328;">
<h1 style="font-size: 18px;">{{.Title}}</h1>
<p>{{.Period}}<br><strong>{{.Played}}</strong></p>
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

// summaryEmail writes the HTML of ev's summary, of a week, a month or a
// year, to page, and returns its plain text, without footer: the period,
// the figures, and its parts as lists, with the address of each title
// when the public address is set.
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
		titles(s.phrase(i18n.Text{"Movies watched", "Films regardés", "Geschaute Filme", "Películas vistas", "Film visti", "Filmes assistidos"}), sum.Movies)
		titles(s.phrase(i18n.Text{"Series watched", "Séries regardées", "Geschaute Serien", "Series vistas", "Serie TV viste", "Séries assistidas"}), sum.Series)
	} else {
		titles(s.phrase(i18n.Text{"Most watched movies", "Films les plus regardés", "Meistgesehene Filme", "Películas más vistas", "Film più visti", "Filmes mais assistidos"}), sum.Movies)
		titles(s.phrase(i18n.Text{"Most watched series", "Séries les plus regardées", "Meistgesehene Serien", "Series más vistas", "Serie TV più viste", "Séries mais assistidas"}), sum.Series)
	}
	if sum.TopMonth != nil {
		sections = append(sections, summarySection{Heading: s.phrase(i18n.Text{"Most watched month", "Mois le plus regardé", "Meistgesehener Monat", "Mes con más visionado", "Mese più visto", "Mês mais assistido"}),
			Items: []summaryItem{{Name: s.topMonthText(*sum.TopMonth, true), Detail: s.playsText(sum.TopMonth.Plays)}}})
	}
	if sum.NewEpisodes > 0 {
		section := summarySection{Heading: s.episodesHeading(sum)}
		for _, e := range sum.Episodes {
			name := fmt.Sprintf("%s S%02dE%02d", e.SeriesName, e.Season, e.Number)
			section.Items = append(section.Items, summaryItem{Name: name, URL: s.itemLink(e.ID), Detail: e.Name})
		}
		if more := sum.NewEpisodes - len(sum.Episodes); more > 0 {
			section.More = s.count(more, i18n.Text{"And %d more.", "Et %d autre.", "Und %d weitere.", "Y %d más.", "E ancora %d.", "E mais %d."},
				i18n.Text{"And %d more.", "Et %d autres.", "Und %d weitere.", "Y %d más.", "E ancora %d.", "E mais %d."})
		}
		sections = append(sections, section)
	}
	if len(sum.Users) > 0 {
		section := summarySection{Heading: s.phrase(i18n.Text{"New users", "Nouveaux utilisateurs", "Neue Benutzer", "Nuevos usuarios", "Nuovi utenti", "Novos usuários"})}
		for _, u := range sum.Users {
			item := summaryItem{Name: u.Name}
			if link := s.adminLink("/users/" + u.ID); link != nil {
				item.URL = *link
			}
			if u.InvitedBy != nil {
				item.Detail = s.phrase(i18n.Text{"invited by %s", "invité par %s", "eingeladen von %s", "invitado por %s", "invitato da %s", "convidado por %s"}, u.InvitedBy.Name)
			}
			section.Items = append(section.Items, item)
		}
		sections = append(sections, section)
	}
	if sum.AddedFiles > 0 {
		section := summarySection{Heading: s.addedHeading(sum)}
		for _, a := range sum.Added {
			item := summaryItem{Name: a.Name}
			switch {
			case a.Kind == "album":
				item.Detail = s.count(a.Files, i18n.Text{"%d track", "%d piste", "%d Titel", "%d pista", "%d traccia", "%d faixa"},
					i18n.Text{"%d tracks", "%d pistes", "%d Titel", "%d pistas", "%d tracce", "%d faixas"})
			case a.Kind == "show" || a.Files > 1:
				item.Detail = s.count(a.Files, i18n.Text{"%d file", "%d fichier", "%d Datei", "%d archivo", "%d file", "%d arquivo"},
					i18n.Text{"%d files", "%d fichiers", "%d Dateien", "%d archivos", "%d file", "%d arquivos"})
			}
			section.Items = append(section.Items, item)
		}
		sections = append(sections, section)
	}
	if len(sum.Problems) > 0 {
		section := summarySection{Heading: s.problemsHeading(sum)}
		for _, p := range sum.Problems {
			item := summaryItem{Name: p.Text, Detail: s.phrase(i18n.Text{"Warning", "Avertissement", "Warnung", "Advertencia", "Avviso", "Aviso"})}
			if p.Severity == SeverityError {
				item.Detail = s.phrase(i18n.Text{"Error", "Erreur", "Fehler", "Error", "Errore", "Erro"})
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
	open := s.phrase(i18n.Text{"Open the statistics", "Ouvrir les statistiques", "Statistiken öffnen", "Abrir las estadísticas", "Apri le statistiche", "Abrir as estatísticas"})
	if err := summaryHTML.Execute(page, map[string]any{"Language": language, "Title": ev.Title, "Period": s.periodLine(ev.Type, sum),
		"Played": s.playedLine(sum), "Sections": sections, "URL": url, "Open": open, "Footer": footer}); err != nil {
		return "", err
	}

	var text strings.Builder
	text.WriteString(s.periodLine(ev.Type, sum) + "\n" + s.playedLine(sum) + "\n")
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
		text.WriteString("\n" + s.phrase(i18n.Text{"%s: %s", "%s : %s", "%s: %s", "%s: %s", "%s: %s", "%s: %s"}, open, url) + "\n")
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
