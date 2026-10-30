// Package activity keeps the server's activity log, as Jellyfin's: who
// signed in or failed to, what was played, and the changes administrators
// made to users, settings and addons. Entries are named in the server
// language when they are written, as Jellyfin names them, and kept
// Retention.
package activity

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/i18n"
)

// Retention is how long entries are kept.
const Retention = 30 * 24 * time.Hour

// SweepInterval is how often entries older than Retention are deleted.
const SweepInterval = 24 * time.Hour

// Severities, as Jellyfin's LogLevel names.
const (
	Information = "Information"
	Warning     = "Warning"
	Error       = "Error"
)

// Entry is one thing that happened.
type Entry struct {
	ID            int64
	Name          string
	Overview      *string
	ShortOverview *string
	// Type is the kind of entry, as Jellyfin's activity types where it has
	// one.
	Type   string
	ItemID *string
	// UserID is the user the entry is about, nil when none is.
	UserID   *accounts.ID
	Date     time.Time
	Severity string
}

// Store records and lists entries.
type Store struct {
	db       *pgxpool.Pool
	settings func() accounts.Settings
	logger   *slog.Logger
	now      func() time.Time
}

// New returns a store; settings gives the language entries are named in.
func New(db *pgxpool.Pool, settings func() accounts.Settings, logger *slog.Logger) *Store {
	return &Store{db: db, settings: settings, logger: logger, now: time.Now}
}

// record writes an entry. The activity log is a record, not part of what
// is being done: a failure is logged, never returned.
func (s *Store) record(ctx context.Context, e Entry) {
	if s == nil {
		return
	}
	if e.Severity == "" {
		e.Severity = Information
	}
	_, err := s.db.Exec(context.WithoutCancel(ctx), `INSERT INTO activity_log (name, overview, short_overview, type, item_id, user_id, date, severity)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		clip(e.Name, 1024), clipped(e.Overview, 4096), clipped(e.ShortOverview, 1024), e.Type, e.ItemID, e.UserID, s.now(), e.Severity)
	if err != nil {
		s.logger.Warn("An activity could not be recorded", "type", e.Type, "error", err)
	}
}

// clip shortens s to at most n characters.
func clip(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n-1]) + "…"
}

func clipped(s *string, n int) *string {
	if s == nil {
		return nil
	}
	return new(clip(*s, n))
}

// Query selects entries, as Jellyfin's activity log query does.
type Query struct {
	Start int
	// Limit is how many entries to return; 0 returns all.
	Limit int
	// MinDate keeps entries from that date; the zero time keeps all.
	MinDate time.Time
	// HasUserID keeps the entries about a user when true, the others when
	// false; nil keeps both.
	HasUserID *bool
	// Types and Severities keep the entries of these types and severities;
	// empty keeps all. Jellyfin's query has neither: the admin app uses
	// them.
	Types      []string
	Severities []string
}

// Page is the entries a query selected, newest first, with how many it
// selects in all.
type Page struct {
	Entries []Entry
	Total   int
}

// Entries lists the entries q selects; a nil store has none.
func (s *Store) Entries(ctx context.Context, q Query) (Page, error) {
	if s == nil {
		return Page{Entries: []Entry{}}, nil
	}
	var minDate *time.Time
	if !q.MinDate.IsZero() {
		minDate = &q.MinDate
	}
	var limit *int
	if q.Limit > 0 {
		limit = &q.Limit
	}
	var types, severities []string
	if len(q.Types) > 0 {
		types = q.Types
	}
	if len(q.Severities) > 0 {
		severities = q.Severities
	}
	const where = `WHERE ($1::timestamptz IS NULL OR date >= $1) AND ($2::boolean IS NULL OR (user_id IS NOT NULL) = $2)
		AND ($3::text[] IS NULL OR type = ANY($3)) AND ($4::text[] IS NULL OR severity = ANY($4))`
	rows, err := s.db.Query(ctx, `SELECT id, name, overview, short_overview, type, item_id, user_id, date, severity
		FROM activity_log `+where+` ORDER BY date DESC, id DESC OFFSET $5 LIMIT $6`, minDate, q.HasUserID, types, severities, max(q.Start, 0), limit)
	if err != nil {
		return Page{}, err
	}
	entries, err := pgx.CollectRows(rows, pgx.RowToStructByPos[Entry])
	if err != nil {
		return Page{}, err
	}
	page := Page{Entries: append([]Entry{}, entries...)}
	err = s.db.QueryRow(ctx, "SELECT count(*) FROM activity_log "+where, minDate, q.HasUserID, types, severities).Scan(&page.Total)
	return page, err
}

// Sweep deletes the entries older than Retention and returns how many.
func (s *Store) Sweep(ctx context.Context) (int64, error) {
	tag, err := s.db.Exec(ctx, "DELETE FROM activity_log WHERE date < $1", s.now().Add(-Retention))
	return tag.RowsAffected(), err
}

// phrase names an entry in the server language, from its text in each
// language, with %s for args. The methods below record nothing on a nil
// store.
func (s *Store) phrase(text i18n.Text, args ...any) string {
	if s == nil {
		return ""
	}
	return fmt.Sprintf(text.In(s.settings().Language), args...)
}

func (s *Store) address(address string) *string {
	return new(s.phrase(i18n.Text{"Address: %s", "Adresse : %s", "Adresse: %s", "Dirección: %s", "Indirizzo: %s", "Endereço: %s"}, address))
}

// SignedIn records that user signed in from address, from a Jellyfin app
// or the admin interface.
func (s *Store) SignedIn(ctx context.Context, user accounts.User, address string) {
	s.record(ctx, Entry{Name: s.phrase(i18n.Text{"%s signed in", "Connexion de %s", "%s hat sich angemeldet", "%s ha iniciado sesión", "%s ha effettuato l’accesso", "%s entrou"}, user.Name), Type: "AuthenticationSucceeded",
		UserID: &user.ID, ShortOverview: s.address(address)})
}

// SignInFailed records a sign-in refused for name from address.
func (s *Store) SignInFailed(ctx context.Context, name, address string) {
	s.record(ctx, Entry{Name: s.phrase(i18n.Text{"Failed sign-in as %s", "Échec de connexion en tant que %s", "Fehlgeschlagene Anmeldung als %s", "Inicio de sesión fallido como %s", "Accesso non riuscito come %s", "Falha no login como %s"}, name), Type: "AuthenticationFailed",
		ShortOverview: s.address(address), Severity: Error})
}

// SignInRefusedRemote records that user, who proved who they are, was
// refused a sign-in from address, outside the local network, which they
// may not connect from (see accounts.User.RemoteAccess). It is a failed
// sign-in, about the user. Jellyfin only logs it.
func (s *Store) SignInRefusedRemote(ctx context.Context, user accounts.User, address string) {
	s.record(ctx, Entry{Name: s.phrase(i18n.Text{
		"%s may not sign in from outside the local network",
		"%s ne peut pas se connecter depuis l’extérieur du réseau local",
		"%s darf sich nicht von außerhalb des lokalen Netzwerks anmelden",
		"%s no puede iniciar sesión desde fuera de la red local",
		"%s non può accedere dall’esterno della rete locale",
		"%s não pode entrar de fora da rede local",
	}, user.Name), Type: "AuthenticationFailed", UserID: &user.ID, ShortOverview: s.address(address), Severity: Warning})
}

// PlaybackStarted records that user started playing title, the item item,
// on device.
func (s *Store) PlaybackStarted(ctx context.Context, user accounts.User, title, item, device string) {
	s.record(ctx, Entry{Name: s.phrase(i18n.Text{"%s is playing %s on %s", "%s regarde %s sur %s", "%s schaut %s auf %s", "%s está viendo %s en %s", "%s sta guardando %s su %s", "%s está assistindo %s em %s"}, user.Name, title, device),
		Type: "VideoPlayback", UserID: &user.ID, ItemID: &item})
}

// PlaybackStopped records that user stopped playing title on device.
func (s *Store) PlaybackStopped(ctx context.Context, user accounts.User, title, item, device string) {
	s.record(ctx, Entry{Name: s.phrase(i18n.Text{"%s stopped playing %s on %s", "%s a arrêté %s sur %s", "%s hat %s auf %s beendet", "%s ha dejado de ver %s en %s", "%s ha smesso di guardare %s su %s", "%s parou de assistir %s em %s"}, user.Name, title, device),
		Type: "VideoPlaybackStopped", UserID: &user.ID, ItemID: &item})
}

// AudioStarted records that user started playing a track or an audiobook,
// title, the item item, on device, as Jellyfin records audio.
func (s *Store) AudioStarted(ctx context.Context, user accounts.User, title, item, device string) {
	s.record(ctx, Entry{Name: s.phrase(i18n.Text{"%s is playing %s on %s", "%s écoute %s sur %s", "%s hört %s auf %s", "%s está escuchando %s en %s", "%s sta ascoltando %s su %s", "%s está ouvindo %s em %s"}, user.Name, title, device),
		Type: "AudioPlayback", UserID: &user.ID, ItemID: &item})
}

// AudioStopped records that user stopped playing a track or an audiobook.
func (s *Store) AudioStopped(ctx context.Context, user accounts.User, title, item, device string) {
	s.record(ctx, Entry{Name: s.phrase(i18n.Text{"%s has finished playing %s on %s", "%s a fini d'écouter %s sur %s", "%s hat %s auf %s fertig gehört", "%s ha terminado de escuchar %s en %s", "%s ha finito di ascoltare %s su %s", "%s terminou de ouvir %s em %s"}, user.Name, title, device),
		Type: "AudioPlaybackStopped", UserID: &user.ID, ItemID: &item})
}

// UserCreated records a new account.
func (s *Store) UserCreated(ctx context.Context, user accounts.User) {
	s.record(ctx, Entry{Name: s.phrase(i18n.Text{"User %s was created", "Utilisateur %s créé", "Benutzer %s wurde erstellt", "Se ha creado el usuario %s", "Utente %s creato", "Usuário %s criado"}, user.Name), Type: "UserCreated", UserID: &user.ID})
}

// UserJoined records an account a guest created from address through
// invite, naming the administrator who created the invite, if they still
// exist.
func (s *Store) UserJoined(ctx context.Context, user accounts.User, invite accounts.Invite, address string) {
	name := s.phrase(i18n.Text{"%s joined through an invite", "%s a rejoint le serveur grâce à une invitation", "%s ist über eine Einladung beigetreten", "%s se ha unido mediante una invitación", "%s ha aderito tramite un invito", "%s entrou por um convite"}, user.Name)
	if creator := invite.CreatedBy; creator != nil {
		name = s.phrase(i18n.Text{"%s joined through %s's invite", "%s a rejoint le serveur grâce à l’invitation de %s", "%s ist über die Einladung von %s beigetreten", "%s se ha unido mediante la invitación de %s", "%s ha aderito tramite l’invito di %s", "%s entrou pelo convite de %s"}, user.Name, creator.Name)
	}
	s.record(ctx, Entry{Name: name, Type: "UserCreated", UserID: &user.ID, ShortOverview: s.address(address)})
}

// UserDeleted records a deleted account. Like Jellyfin's, the entry is
// about no user: the user no longer exists.
func (s *Store) UserDeleted(ctx context.Context, name string) {
	s.record(ctx, Entry{Name: s.phrase(i18n.Text{"User %s was deleted", "Utilisateur %s supprimé", "Benutzer %s wurde gelöscht", "Se ha borrado el usuario %s", "Utente %s eliminato", "Usuário %s excluído"}, name), Type: "UserDeleted"})
}

// UserChanged records a change to an account other than its password.
func (s *Store) UserChanged(ctx context.Context, user accounts.User) {
	s.record(ctx, Entry{Name: s.phrase(i18n.Text{"User %s was changed", "Utilisateur %s modifié", "Benutzer %s wurde geändert", "Se ha modificado el usuario %s", "Utente %s modificato", "Usuário %s alterado"}, user.Name), Type: "UserPolicyUpdated", UserID: &user.ID})
}

// AccountEnded records that user's account reached its end date, from
// which it is refused as a disabled one is. Jellyfin has no such entry.
func (s *Store) AccountEnded(ctx context.Context, user accounts.User) {
	s.record(ctx, Entry{Name: s.phrase(i18n.Text{"The account of %s ended", "Le compte de %s a pris fin", "Das Konto von %s ist abgelaufen", "La cuenta de %s ha finalizado", "L’account di %s è scaduto", "A conta de %s foi encerrada"}, user.Name), Type: "UserEnded",
		UserID: &user.ID})
}

// PasswordChanged records a new password for user.
func (s *Store) PasswordChanged(ctx context.Context, user accounts.User) {
	s.record(ctx, Entry{Name: s.phrase(i18n.Text{"The password of %s was changed", "Mot de passe de %s changé", "Das Passwort von %s wurde geändert", "Se ha cambiado la contraseña de %s", "Password di %s modificata", "Senha de %s alterada"}, user.Name),
		Type: "UserPasswordChanged", UserID: &user.ID})
}

// TwoStepTurnedOn records that user turned two-step sign-in on.
func (s *Store) TwoStepTurnedOn(ctx context.Context, user accounts.User) {
	s.record(ctx, Entry{Name: s.phrase(i18n.Text{"%s turned two-step sign-in on", "%s a activé la connexion en deux étapes", "%s hat die zweistufige Anmeldung aktiviert", "%s ha activado el inicio de sesión en dos pasos", "%s ha attivato l’accesso in due passaggi", "%s ativou o login em duas etapas"}, user.Name),
		Type: "UserTwoStepOn", UserID: &user.ID})
}

// TwoStepTurnedOff records that two-step sign-in was turned off for user,
// by them or by the administrator named by.
func (s *Store) TwoStepTurnedOff(ctx context.Context, user accounts.User, by string) {
	name := s.phrase(i18n.Text{"%s turned two-step sign-in off", "%s a désactivé la connexion en deux étapes", "%s hat die zweistufige Anmeldung deaktiviert", "%s ha desactivado el inicio de sesión en dos pasos", "%s ha disattivato l’accesso in due passaggi", "%s desativou o login em duas etapas"}, user.Name)
	severity := ""
	if by != user.Name {
		name = s.phrase(i18n.Text{"%s turned two-step sign-in off for %s", "%s a désactivé la connexion en deux étapes de %s", "%s hat die zweistufige Anmeldung für %s deaktiviert", "%s ha desactivado el inicio de sesión en dos pasos de %s", "%s ha disattivato l’accesso in due passaggi di %s", "%s desativou o login em duas etapas de %s"}, by, user.Name)
		severity = Warning
	}
	s.record(ctx, Entry{Name: name, Type: "UserTwoStepOff", UserID: &user.ID, Severity: severity})
}

// RecoveryCodeUsed records that user signed in from address with a
// recovery code instead of their authenticator app.
func (s *Store) RecoveryCodeUsed(ctx context.Context, user accounts.User, address string) {
	s.record(ctx, Entry{Name: s.phrase(i18n.Text{"%s signed in with a recovery code", "%s s’est connecté avec un code de secours", "%s hat sich mit einem Wiederherstellungscode angemeldet", "%s ha iniciado sesión con un código de recuperación", "%s ha effettuato l’accesso con un codice di recupero", "%s entrou com um código de recuperação"}, user.Name),
		Type: "UserRecoveryCodeUsed", UserID: &user.ID, ShortOverview: s.address(address), Severity: Warning})
}

// sectionNames name the sections of a group's page, by the names of
// accounts.GroupChanges.Sections.
var sectionNames = map[string]i18n.Text{
	accounts.SectionName:      {"Name", "Nom", "Name", "Nombre", "Nome", "Nome"},
	accounts.SectionAccess:    {"Access", "Accès", "Zugriff", "Acceso", "Accesso", "Acesso"},
	accounts.SectionPlayback:  {"Playback and access", "Lecture et accès", "Wiedergabe und Zugriff", "Reproducción y acceso", "Riproduzione e accesso", "Reprodução e acesso"},
	accounts.SectionParental:  {"Parental control", "Contrôle parental", "Kindersicherung", "Control parental", "Controllo parentale", "Controle parental"},
	accounts.SectionLibraries: {"Visible libraries", "Bibliothèques visibles", "Sichtbare Bibliotheken", "Bibliotecas visibles", "Librerie visibili", "Bibliotecas visíveis"},
	accounts.SectionGenres:    {"Blocked genres", "Genres bloqués", "Gesperrte Genres", "Géneros bloqueados", "Generi bloccati", "Gêneros bloqueados"},
	accounts.SectionHours:     {"Allowed hours", "Horaires autorisés", "Erlaubte Zeiten", "Horario permitido", "Orari consentiti", "Horários permitidos"},
}

// GroupCreated records a new user group.
func (s *Store) GroupCreated(ctx context.Context, group accounts.Group) {
	s.record(ctx, Entry{Name: s.phrase(i18n.Text{"Group %s was created", "Groupe %s créé", "Gruppe %s wurde erstellt", "Se ha creado el grupo %s", "Gruppo %s creato", "Grupo %s criado"}, group.Name), Type: "GroupCreated"})
}

// GroupChanged records a change to a user group, naming the sections of
// its page changed (accounts.GroupChanges.Sections).
func (s *Store) GroupChanged(ctx context.Context, group accounts.Group, sections []string) {
	names := make([]string, 0, len(sections))
	for _, section := range sections {
		if name, ok := sectionNames[section]; ok {
			section = s.phrase(name)
		}
		names = append(names, section)
	}
	s.record(ctx, Entry{Name: s.phrase(i18n.Text{"Group %s was changed: %s", "Groupe %s modifié : %s", "Gruppe %s wurde geändert: %s", "Se ha modificado el grupo %s: %s", "Gruppo %s modificato: %s", "Grupo %s alterado: %s"}, group.Name, strings.Join(names, ", ")),
		Type: "GroupChanged"})
}

// GroupDeleted records a deleted user group, whose members kept its values
// as their own.
func (s *Store) GroupDeleted(ctx context.Context, name string) {
	s.record(ctx, Entry{Name: s.phrase(i18n.Text{"Group %s was deleted", "Groupe %s supprimé", "Gruppe %s wurde gelöscht", "Se ha borrado el grupo %s", "Gruppo %s eliminato", "Grupo %s excluído"}, name), Type: "GroupDeleted"})
}

// GroupMemberAdded records that user joined the group named group.
func (s *Store) GroupMemberAdded(ctx context.Context, user accounts.User, group string) {
	s.record(ctx, Entry{Name: s.phrase(i18n.Text{"%s joined the group %s", "%s a rejoint le groupe %s", "%s ist der Gruppe %s beigetreten", "%s se ha unido al grupo %s", "%s fa ora parte del gruppo %s", "%s entrou no grupo %s"}, user.Name, group),
		Type: "GroupMemberAdded", UserID: &user.ID})
}

// GroupMemberRemoved records that user left the group named group, keeping
// its values as their own.
func (s *Store) GroupMemberRemoved(ctx context.Context, user accounts.User, group string) {
	s.record(ctx, Entry{Name: s.phrase(i18n.Text{"%s left the group %s", "%s a quitté le groupe %s", "%s hat die Gruppe %s verlassen", "%s ha dejado el grupo %s", "%s ha lasciato il gruppo %s", "%s saiu do grupo %s"}, user.Name, group),
		Type: "GroupMemberRemoved", UserID: &user.ID})
}

// GroupMemberDetached records that user no longer follows the group named
// group: they keep their own settings. app names the Jellyfin app a change
// detached them from, empty when an administrator turned following off.
func (s *Store) GroupMemberDetached(ctx context.Context, user accounts.User, group, app string) {
	name := s.phrase(i18n.Text{"%s no longer follows the group %s", "%s ne suit plus le groupe %s", "%s folgt der Gruppe %s nicht mehr", "%s ya no sigue el grupo %s", "%s non segue più il gruppo %s", "%s não segue mais o grupo %s"}, user.Name, group)
	if app != "" {
		name = s.phrase(i18n.Text{"%s no longer follows the group %s: changed from %s", "%s ne suit plus le groupe %s : modifié depuis %s", "%s folgt der Gruppe %s nicht mehr: geändert in %s", "%s ya no sigue el grupo %s: cambiado desde %s", "%s non segue più il gruppo %s: modificato da %s", "%s não segue mais o grupo %s: alterado em %s"},
			user.Name, group, app)
	}
	s.record(ctx, Entry{Name: name, Type: "GroupMemberDetached", UserID: &user.ID})
}

// LimitsReset records that the administrator named by put the settings a
// group holds of user back to a new user's (Reset to defaults). Jellyfin
// has no such entry: it is a change of the user's policy.
func (s *Store) LimitsReset(ctx context.Context, user accounts.User, by string) {
	s.record(ctx, Entry{Name: s.phrase(i18n.Text{"%s reset the settings of %s to their defaults", "%s a rétabli les réglages par défaut de %s", "%s hat die Einstellungen von %s auf die Standardwerte zurückgesetzt", "%s ha restablecido los ajustes predeterminados de %s", "%s ha ripristinato le impostazioni predefinite di %s", "%s redefiniu as configurações de %s para o padrão"}, by, user.Name),
		Type: "UserPolicyUpdated", UserID: &user.ID})
}

// GroupReset records that the administrator named by put the settings of
// group back to a new user's.
func (s *Store) GroupReset(ctx context.Context, group accounts.Group, by string) {
	s.record(ctx, Entry{Name: s.phrase(i18n.Text{"%s reset the settings of the group %s to their defaults", "%s a rétabli les réglages par défaut du groupe %s", "%s hat die Einstellungen der Gruppe %s auf die Standardwerte zurückgesetzt", "%s ha restablecido los ajustes predeterminados del grupo %s", "%s ha ripristinato le impostazioni predefinite del gruppo %s", "%s redefiniu as configurações do grupo %s para o padrão"}, by, group.Name),
		Type: "GroupChanged"})
}

// SettingsSaved records that administrator saved the server settings.
func (s *Store) SettingsSaved(ctx context.Context, administrator string) {
	s.record(ctx, Entry{Name: s.phrase(i18n.Text{"%s saved the server settings", "%s a enregistré les paramètres du serveur", "%s hat die Servereinstellungen gespeichert", "%s ha guardado los ajustes del servidor", "%s ha salvato le impostazioni del server", "%s salvou as configurações do servidor"}, administrator),
		Type: "ServerConfigurationUpdated"})
}

// LibraryOrderApplied records that administrator applied the order of
// Content › Libraries to every user, clearing the orders users saved
// themselves, users of them. Jellyfin has no such entry: it has no order
// an administrator sets for every user.
func (s *Store) LibraryOrderApplied(ctx context.Context, administrator string, users int) {
	s.record(ctx, Entry{Name: s.phrase(i18n.Text{"%s applied the order of the libraries to every user (%d with an order of their own)",
		"%s a appliqué l’ordre des bibliothèques à tous les utilisateurs (%d avec un ordre à eux)",
		"%s hat die Reihenfolge der Bibliotheken auf alle Benutzer angewendet (%d mit eigener Reihenfolge)",
		"%s ha aplicado el orden de las bibliotecas a todos los usuarios (%d con un orden propio)",
		"%s ha applicato l’ordine delle librerie a tutti gli utenti (%d con un ordine proprio)",
		"%s aplicou a ordem das bibliotecas a todos os usuários (%d com uma ordem própria)"}, administrator, users),
		Type: "ServerConfigurationUpdated"})
}

// MetricsTokenMade records that administrator made a new metrics token,
// never the token.
func (s *Store) MetricsTokenMade(ctx context.Context, administrator string) {
	s.record(ctx, Entry{Name: s.phrase(i18n.Text{"%s made a new metrics token", "%s a créé un nouveau jeton des métriques", "%s hat ein neues Metrik-Token erstellt", "%s ha creado un nuevo token de métricas", "%s ha creato un nuovo token delle metriche", "%s criou um novo token de métricas"}, administrator),
		Type: "ServerConfigurationUpdated"})
}

// BackupRestored records, in the database restored, that administrator
// restored the database backup file.
func (s *Store) BackupRestored(ctx context.Context, administrator, file string) {
	s.record(ctx, Entry{Name: s.phrase(i18n.Text{"%s restored the database backup %s", "%s a restauré la sauvegarde de la base de données %s", "%s hat das Datenbank-Backup %s wiederhergestellt", "%s ha restaurado la copia de seguridad de la base de datos %s", "%s ha ripristinato il backup del database %s", "%s restaurou o backup do banco de dados %s"},
		administrator, file), Type: "BackupRestored", Severity: Warning})
}

// BackupRestoredAtSetup records, in the database restored, that the
// database backup file was restored from the setup page, before any
// administrator existed.
func (s *Store) BackupRestoredAtSetup(ctx context.Context, file string) {
	s.record(ctx, Entry{Name: s.phrase(i18n.Text{"The database backup %s was restored from the setup page", "La sauvegarde de la base de données %s a été restaurée depuis la page d’installation", "Das Datenbank-Backup %s wurde auf der Einrichtungsseite wiederhergestellt", "La copia de seguridad de la base de datos %s se ha restaurado desde la página de configuración", "Il backup del database %s è stato ripristinato dalla pagina di configurazione", "O backup do banco de dados %s foi restaurado na página de configuração"},
		file), Type: "BackupRestored", Severity: Warning})
}

// ServerRestarted records that administrator restarted Polyfin from the
// admin app, before it stops.
func (s *Store) ServerRestarted(ctx context.Context, administrator string) {
	s.record(ctx, Entry{Name: s.phrase(i18n.Text{"%s restarted Polyfin", "%s a redémarré Polyfin", "%s hat Polyfin neu gestartet", "%s ha reiniciado Polyfin", "%s ha riavviato Polyfin", "%s reiniciou o Polyfin"}, administrator),
		Type: "ServerRestarted"})
}

// DesktopUpdateStarted records that administrator had Polyfin Desktop
// install version, which stops Polyfin.
func (s *Store) DesktopUpdateStarted(ctx context.Context, administrator, version string) {
	s.record(ctx, Entry{Name: s.phrase(i18n.Text{"%s had Polyfin Desktop install Polyfin %s", "%s a fait installer Polyfin %s à Polyfin Desktop", "%s hat Polyfin Desktop Polyfin %s installieren lassen", "%s ha hecho que Polyfin Desktop instale Polyfin %s", "%s ha fatto installare Polyfin %s a Polyfin Desktop", "%s fez o Polyfin Desktop instalar o Polyfin %s"}, administrator, version),
		Type: "ServerUpdateStarted"})
}

// UpdateToolAsked records that administrator asked the update tool to
// update now, before the request is sent: a tool that replaces Polyfin's
// own container may stop Polyfin before it answers.
func (s *Store) UpdateToolAsked(ctx context.Context, administrator string) {
	s.record(ctx, Entry{Name: s.phrase(i18n.Text{"%s asked the update tool to update now", "%s a demandé à l’outil de mise à jour de mettre à jour maintenant", "%s hat das Update-Werkzeug um ein sofortiges Update gebeten", "%s ha pedido a la herramienta de actualización que actualice ahora", "%s ha chiesto allo strumento di aggiornamento di aggiornare ora", "%s pediu à ferramenta de atualização que atualize agora"}, administrator),
		Type: "ServerUpdateStarted"})
}

// secretNames name the secrets that can be revealed: the server's, by
// their admin API name, and the users' own API keys, by service.
var secretNames = map[string]i18n.Text{
	"publicMetaDbKey":   {"the PublicMetaDB key", "la clé PublicMetaDB", "den PublicMetaDB-Schlüssel", "la clave de PublicMetaDB", "la chiave PublicMetaDB", "a chave do PublicMetaDB"},
	"theIntroDbKey":     {"the TheIntroDB key", "la clé TheIntroDB", "den TheIntroDB-Schlüssel", "la clave de TheIntroDB", "la chiave TheIntroDB", "a chave do TheIntroDB"},
	"mdblistRatingsKey": {"the MDBList API key for ratings", "la clé d’API MDBList des notes", "den MDBList-API-Schlüssel für Bewertungen", "la clave API de MDBList para las valoraciones", "la chiave API MDBList per i voti", "a chave da API do MDBList para avaliações"},
	"tmdbKey":           {"the TMDB key", "la clé TMDB", "den TMDB-Schlüssel", "la clave de TMDB", "la chiave TMDB", "a chave do TMDB"},
	"traktClientSecret": {"the Trakt client secret", "le client secret Trakt", "das Trakt-Client-Secret", "el secreto de cliente de Trakt", "il client secret di Trakt", "o segredo do cliente do Trakt"},
	"lastFmSecret":      {"the Last.fm shared secret", "le secret partagé Last.fm", "das gemeinsame Last.fm-Secret", "el secreto compartido de Last.fm", "il segreto condiviso di Last.fm", "o segredo compartilhado do Last.fm"},
	"mdblist":           {"their MDBList API key", "sa clé d’API MDBList", "den eigenen MDBList-API-Schlüssel", "su clave API de MDBList", "la propria chiave API MDBList", "a própria chave da API do MDBList"},
	"publicmetadb":      {"their PublicMetaDB API key", "sa clé d’API PublicMetaDB", "den eigenen PublicMetaDB-API-Schlüssel", "su clave API de PublicMetaDB", "la propria chiave API PublicMetaDB", "a própria chave da API do PublicMetaDB"},
	"listenbrainz":      {"their ListenBrainz user token", "son jeton d’utilisateur ListenBrainz", "das eigene ListenBrainz-Benutzertoken", "su token de usuario de ListenBrainz", "il proprio token utente ListenBrainz", "o próprio token de usuário do ListenBrainz"},
	"metricsToken":      {"the metrics token", "le jeton des métriques", "das Metrik-Token", "el token de métricas", "il token delle metriche", "o token de métricas"},
	"updateToolToken":   {"the update tool's token", "le jeton de l’outil de mise à jour", "das Token des Update-Werkzeugs", "el token de la herramienta de actualización", "il token dello strumento di aggiornamento", "o token da ferramenta de atualização"},
}

// SecretRevealed records that user had a stored secret shown, by its name
// in secretNames, never its value: one of the server's, or, own, their
// own API key.
func (s *Store) SecretRevealed(ctx context.Context, user accounts.User, secret string, own bool) {
	if name, ok := secretNames[secret]; ok {
		secret = s.phrase(name)
	}
	entry := Entry{Name: s.phrase(i18n.Text{"%s revealed %s", "%s a affiché %s", "%s hat %s angezeigt", "%s ha mostrado %s", "%s ha mostrato %s", "%s exibiu %s"}, user.Name, secret), Type: "SecretRevealed"}
	if own {
		entry.UserID = &user.ID
	}
	s.record(ctx, entry)
}

// AddonInstalled records that by installed the addon name at version,
// for the server or for themselves.
func (s *Store) AddonInstalled(ctx context.Context, by accounts.User, name, version string, shared bool) {
	text := i18n.Text{"%s installed the addon %s for themselves", "%s a installé l’addon %s pour son compte", "%s hat das Add-on %s für sich installiert", "%s ha instalado la extensión %s para su cuenta", "%s ha installato l’add-on %s per il proprio account", "%s instalou o complemento %s para a própria conta"}
	if shared {
		text = i18n.Text{"%s installed the addon %s for the server", "%s a installé l’addon %s pour le serveur", "%s hat das Add-on %s für den Server installiert", "%s ha instalado la extensión %s para el servidor", "%s ha installato l’add-on %s per il server", "%s instalou o complemento %s para o servidor"}
	}
	s.record(ctx, Entry{Name: s.phrase(text, by.Name, name), Type: "AddonInstalled",
		ShortOverview: new(s.phrase(i18n.Text{"Version %s", "Version %s", "Version %s", "Versión %s", "Versione %s", "Versão %s"}, version)), UserID: s.owner(by, shared)})
}

// AddonRemoved records that by removed the addon name.
func (s *Store) AddonRemoved(ctx context.Context, by accounts.User, name string, shared bool) {
	text := i18n.Text{"%s removed the addon %s for themselves", "%s a retiré l’addon %s pour son compte", "%s hat das Add-on %s für sich entfernt", "%s ha quitado la extensión %s de su cuenta", "%s ha rimosso l’add-on %s dal proprio account", "%s removeu o complemento %s da própria conta"}
	if shared {
		text = i18n.Text{"%s removed the addon %s for the server", "%s a retiré l’addon %s pour le serveur", "%s hat das Add-on %s für den Server entfernt", "%s ha quitado la extensión %s del servidor", "%s ha rimosso l’add-on %s dal server", "%s removeu o complemento %s do servidor"}
	}
	s.record(ctx, Entry{Name: s.phrase(text, by.Name, name), Type: "AddonUninstalled", UserID: s.owner(by, shared)})
}

// owner is the user an addon entry is about: the user whose own addon it
// is, none for the server's.
func (s *Store) owner(by accounts.User, shared bool) *accounts.ID {
	if shared {
		return nil
	}
	return &by.ID
}

// RecordingScheduled records that user scheduled the recording of title,
// one programme or a series.
func (s *Store) RecordingScheduled(ctx context.Context, user accounts.User, title string) {
	s.record(ctx, Entry{Name: s.phrase(i18n.Text{"%s scheduled the recording of %s", "%s a programmé l’enregistrement de %s", "%s hat die Aufnahme von %s geplant", "%s ha programado la grabación de %s", "%s ha pianificato la registrazione di %s", "%s agendou a gravação de %s"}, user.Name, title),
		Type: "RecordingScheduled", UserID: &user.ID})
}

// RecordingDeleted records that user deleted the recording of title.
func (s *Store) RecordingDeleted(ctx context.Context, user accounts.User, title string) {
	s.record(ctx, Entry{Name: s.phrase(i18n.Text{"%s deleted the recording of %s", "%s a supprimé l’enregistrement de %s", "%s hat die Aufnahme von %s gelöscht", "%s ha borrado la grabación de %s", "%s ha eliminato la registrazione di %s", "%s excluiu a gravação de %s"}, user.Name, title),
		Type: "RecordingDeleted", UserID: &user.ID})
}

// The types of the entries about share links, which Jellyfin does not
// have.
const (
	ShareLinkCreated = "ShareLinkCreated"
	ShareLinkRevoked = "ShareLinkRevoked"
	ShareLinkExpired = "ShareLinkExpired"
	SharedPlayback   = "SharedLinkPlayback"
)

// ShareLinkMade records that the user of link shared its title.
func (s *Store) ShareLinkMade(ctx context.Context, link accounts.ShareLink) {
	s.record(ctx, Entry{Name: s.phrase(i18n.Text{"%s shared %s through a link", "%s a partagé %s par un lien", "%s hat %s per Link geteilt", "%s ha compartido %s mediante un enlace", "%s ha condiviso %s tramite un link", "%s compartilhou %s por um link"}, link.User.Name, link.Title),
		Type: ShareLinkCreated, UserID: &link.User.ID, ItemID: new(link.Item.String())})
}

// ShareLinkRevokedBy records that by revoked link, theirs or another
// user's.
func (s *Store) ShareLinkRevokedBy(ctx context.Context, link accounts.ShareLink, by accounts.User) {
	e := Entry{Name: s.phrase(i18n.Text{"The link %s shared for %s was revoked", "Le lien partagé par %s pour %s a été révoqué", "Der Link, den %s für %s geteilt hat, wurde widerrufen", "Se ha revocado el enlace que %s compartió para %s", "Il link condiviso da %s per %s è stato revocato", "O link compartilhado por %s para %s foi revogado"}, link.User.Name, link.Title),
		Type: ShareLinkRevoked, UserID: &link.User.ID, ItemID: new(link.Item.String())}
	if by.ID != link.User.ID {
		e.ShortOverview = new(s.phrase(i18n.Text{"Revoked by %s", "Révoqué par %s", "Widerrufen von %s", "Revocado por %s", "Revocato da %s", "Revogado por %s"}, by.Name))
	}
	s.record(ctx, e)
}

// ShareLinkExpiredAt records that link expired.
func (s *Store) ShareLinkExpiredAt(ctx context.Context, link accounts.ShareLink) {
	s.record(ctx, Entry{Name: s.phrase(i18n.Text{"The link %s shared for %s expired", "Le lien partagé par %s pour %s a expiré", "Der Link, den %s für %s geteilt hat, ist abgelaufen", "Ha caducado el enlace que %s compartió para %s", "Il link condiviso da %s per %s è scaduto", "O link compartilhado por %s para %s expirou"}, link.User.Name, link.Title),
		Type: ShareLinkExpired, UserID: &link.User.ID, ItemID: new(link.Item.String())})
}

// SharedPlayed records that a guest without an account, from address,
// played the title of link.
func (s *Store) SharedPlayed(ctx context.Context, link accounts.ShareLink, address string) {
	s.record(ctx, Entry{Name: s.phrase(i18n.Text{"A guest played %[2]s through the link shared by %[1]s", "Un invité a lu %[2]s grâce au lien partagé par %[1]s", "Ein Gast hat %[2]s über den von %[1]s geteilten Link abgespielt", "Un invitado ha reproducido %[2]s mediante el enlace compartido por %[1]s", "Un ospite ha riprodotto %[2]s tramite il link condiviso da %[1]s", "Um convidado reproduziu %[2]s pelo link compartilhado por %[1]s"},
		link.User.Name, link.Title),
		Type: SharedPlayback, UserID: &link.User.ID, ItemID: new(link.Item.String()), ShortOverview: s.address(address)})
}
