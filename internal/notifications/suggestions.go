package notifications

import (
	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/homerows"
	"github.com/moodiness/polyfin/internal/i18n"
)

// SuggestionJSON is the home row a titles_suggested event tells of: its
// identifier in the admin API, its name, the administrator's note, empty
// for none, and the administrator who signs it, null when none does.
type SuggestionJSON struct {
	RowID string    `json:"rowId"`
	Name  string    `json:"name"`
	Note  string    `json:"note"`
	From  *UserJSON `json:"from"`
}

// SuggestionWords returns the title and the message that tell of a
// suggestion, in the server language: "Morgan suggests: For the weekend"
// and the note, or, without a note, that the row is on the home page.
func (s *Service) SuggestionWords(sg homerows.Suggestion) (title, message string) {
	row := sg.Row
	if sg.From.ID != (accounts.ID{}) {
		title = s.phrase(i18n.Text{"%s suggests: %s", "%s vous suggère\u202f: %s", "%s schlägt dir vor: %s", "%s te sugiere: %s",
			"%s ti suggerisce: %s", "%s sugere para você: %s"}, sg.From.Name, row.Name)
	} else {
		title = s.phrase(i18n.Text{"Suggested for you: %s", "Suggéré pour vous\u202f: %s", "Vorschlag für dich: %s", "Sugerido para ti: %s",
			"Suggerito per te: %s", "Sugerido para você: %s"}, row.Name)
	}
	message = row.Note
	if message == "" {
		message = s.phrase(i18n.Text{"%s is on your home page.", "%s est sur votre page d’accueil.", "%s ist auf deiner Startseite.",
			"%s está en tu página de inicio.", "%s è nella tua pagina iniziale.", "%s está na sua página inicial."}, row.Name)
	}
	return title, message
}

// TitlesSuggested tells user, on their own targets only, of a home row
// shown to them that tells its users (see homerows.Teller), and returns
// the event's title and message, which the user's apps may show too. The
// link opens the web player's home page, where the row is. It returns at
// once: the message is sent in the background.
func (s *Service) TitlesSuggested(user accounts.User, sg homerows.Suggestion) (title, message string) {
	ev := s.newEvent(TitlesSuggested)
	ev.User = &UserJSON{ID: user.ID.String(), Name: user.Name}
	ev.Suggestion = &SuggestionJSON{RowID: sg.Row.ID.String(), Name: sg.Row.Name, Note: sg.Row.Note}
	if sg.From.ID != (accounts.ID{}) {
		ev.Suggestion.From = &UserJSON{ID: sg.From.ID.String(), Name: sg.From.Name}
	}
	ev.Title, ev.Message = s.SuggestionWords(sg)
	if base := s.accounts.Settings().PublicAddress; base != "" && s.webClient {
		link := base + "/web/#/home"
		ev.URL = &link
	}
	s.dispatch(ev, func(t target) bool { return t.owner != nil && *t.owner == user.ID })
	return ev.Title, ev.Message
}
