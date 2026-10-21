package notifications

import (
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/i18n"
)

// AdminApp is the app of a sign-in through the admin app, which sends no
// app name and no device.
const AdminApp = "Polyfin admin"

// SignInJSON is a sign-in, or the attempt that blocked an account: the
// client's address as Polyfin resolves it, the app and its version, and
// the device's name and identifier as the app sent them, empty when it
// sent none. The admin app is AdminApp, with no device.
type SignInJSON struct {
	Address    string `json:"address"`
	App        string `json:"app"`
	AppVersion string `json:"appVersion"`
	Device     string `json:"device"`
	DeviceID   string `json:"deviceId"`
}

// BlockJSON is the block of an account for wrong passwords: until when,
// and after how many in a row.
type BlockJSON struct {
	Until    time.Time `json:"until"`
	Attempts int       `json:"attempts"`
}

func newSignInJSON(info accounts.DeviceInfo) *SignInJSON {
	return &SignInJSON{Address: info.RemoteAddress, App: info.Client, AppVersion: info.ClientVersion, Device: info.DeviceName,
		DeviceID: info.DeviceID}
}

// AccountBlocked tells that wrong passwords blocked user's account, the
// last of them from attempt: to the server's targets and administrators'
// own, linking to the user's page, where Unblock is, and to the user's
// own. It returns at once: who the administrators are is read again, and
// the messages sent, in the background.
func (s *Service) AccountBlocked(user accounts.User, attempt accounts.DeviceInfo) {
	if user.BlockedUntil == nil {
		return
	}
	administrators, own := s.accountBlockedEvent(user, attempt, true), s.accountBlockedEvent(user, attempt, false)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.spawn(func() {
		// The targets read last stay in use when the database does not answer.
		if err := s.reload(s.ctx); err != nil && s.ctx.Err() == nil {
			s.logger.Debug("The notification targets could not be read again", "error", err)
		}
		s.dispatchAdministrators(administrators)
		s.dispatch(own, func(t target) bool { return t.owner != nil && *t.owner == user.ID && !s.admins[user.ID] })
	})
}

// NewDevice tells user, on their own targets only, that their account
// signed in from device, which it never used. It returns at once.
func (s *Service) NewDevice(user accounts.User, device accounts.Device) {
	ev := s.newEvent(NewDevice)
	ev.User = &UserJSON{ID: user.ID.String(), Name: user.Name}
	ev.SignIn = newSignInJSON(device.DeviceInfo)
	ev.Title = s.phrase(i18n.Text{"Sign-in on a new device", "Connexion depuis un nouvel appareil", "Anmeldung auf einem neuen Gerät", "Inicio de sesión en un dispositivo nuevo", "Accesso da un nuovo dispositivo", "Login em um novo dispositivo"})
	ev.Message = s.phrase(i18n.Text{"%s signed in to %s from %s. If it was not you, change your password: a new password signs every other device out.", "%s s’est connecté au compte %s depuis %s. Si ce n’était pas vous, changez votre mot de passe : un nouveau mot de passe déconnecte tous les autres appareils.", "Neue Anmeldung beim Konto %[2]s von %[3]s aus, mit %[1]s. Wenn du das nicht warst, ändere dein Passwort: Ein neues Passwort meldet alle anderen Geräte ab.", "%s ha iniciado sesión en la cuenta %s desde %s. Si no ha sido usted, cambie su contraseña: una contraseña nueva cierra la sesión de todos los demás dispositivos.", "%s ha effettuato l’accesso all’account %s da %s. Se non sei stato tu, cambia la password: una nuova password disconnette tutti gli altri dispositivi.", "%s entrou na conta %s a partir de %s. Se não foi você, altere sua senha: uma nova senha desconecta todos os outros dispositivos."},
		s.appText(device.DeviceInfo), user.Name, s.addressText(device.RemoteAddress))
	ev.URL = s.adminLink("/me/account#password")
	s.dispatch(ev, func(t target) bool { return t.owner != nil && *t.owner == user.ID })
}

// accountBlockedEvent tells that user's account is blocked, the last wrong
// password coming from attempt: to administrators, or to the user.
func (s *Service) accountBlockedEvent(user accounts.User, attempt accounts.DeviceInfo, administrators bool) Event {
	ev := s.newEvent(AccountBlocked)
	ev.User = &UserJSON{ID: user.ID.String(), Name: user.Name}
	ev.SignIn = newSignInJSON(attempt)
	ev.Block = &BlockJSON{Until: user.BlockedUntil.UTC(), Attempts: user.InvalidLoginAttempts}
	until, app, address := user.BlockedUntil.In(s.zone).Format("15:04"), s.appText(attempt), s.addressText(attempt.RemoteAddress)
	if administrators {
		ev.Title = s.phrase(i18n.Text{"Account blocked: %s", "Compte bloqué : %s", "Konto gesperrt: %s", "Cuenta bloqueada: %s", "Account bloccato: %s", "Conta bloqueada: %s"}, user.Name)
		ev.Message = s.phrase(i18n.Text{"%d wrong passwords in a row: %s cannot sign in until %s. Last attempt from %s, with %s. Unblock it from its page.", "%d mots de passe erronés de suite : %s ne peut plus se connecter avant %s. Dernière tentative depuis %s, avec %s. Débloquez-le depuis sa page.", "%d falsche Passwörter in Folge: %s kann sich bis %s Uhr nicht anmelden. Letzter Versuch von %s, mit %s. Entsperre das Konto auf seiner Seite.", "%d contraseñas incorrectas seguidas: %s no puede iniciar sesión hasta las %s. Último intento desde %s, con %s. Desbloquee la cuenta desde su página.", "%d password errate di seguito: %s non può accedere fino alle %s. Ultimo tentativo da %s, con %s. Sblocca l’account dalla sua pagina.", "%d senhas erradas seguidas: %s não pode entrar até as %s. Última tentativa de %s, com %s. Desbloqueie a conta na página dela."},
			user.InvalidLoginAttempts, user.Name, until, address, app)
		ev.URL = s.adminLink("/users/" + user.ID.String())
		return ev
	}
	ev.Title = s.phrase(i18n.Text{"Your account is blocked", "Votre compte est bloqué", "Dein Konto ist gesperrt", "Su cuenta está bloqueada", "Il tuo account è bloccato", "Sua conta está bloqueada"})
	ev.Message = s.phrase(i18n.Text{"%d wrong passwords in a row: your account %s cannot sign in until %s. Last attempt from %s, with %s. If it was not you, someone is trying your password.", "%d mots de passe erronés de suite : votre compte %s ne peut plus se connecter avant %s. Dernière tentative depuis %s, avec %s. Si ce n’était pas vous, quelqu’un essaie votre mot de passe.", "%d falsche Passwörter in Folge: Dein Konto %s kann sich bis %s Uhr nicht anmelden. Letzter Versuch von %s, mit %s. Wenn du das nicht warst, probiert jemand dein Passwort aus.", "%d contraseñas incorrectas seguidas: su cuenta %s no puede iniciar sesión hasta las %s. Último intento desde %s, con %s. Si no ha sido usted, alguien está probando su contraseña.", "%d password errate di seguito: il tuo account %s non può accedere fino alle %s. Ultimo tentativo da %s, con %s. Se non sei stato tu, qualcuno sta provando la tua password.", "%d senhas erradas seguidas: sua conta %s não pode entrar até as %s. Última tentativa de %s, com %s. Se não foi você, alguém está tentando adivinhar sua senha."},
		user.InvalidLoginAttempts, user.Name, until, address, app)
	ev.URL = s.adminLink("/me/account#password")
	return ev
}

// appText names the app and device of a sign-in, such as "Jellyfin Web on
// Chrome", or the admin app.
func (s *Service) appText(info accounts.DeviceInfo) string {
	switch {
	case info.Client == AdminApp:
		return s.phrase(i18n.Text{"the admin app", "l’appli d’administration", "der Admin-App", "la aplicación de administración", "l’applicazione di amministrazione", "o aplicativo de administração"})
	case info.Client == "":
		return s.phrase(i18n.Text{"an unnamed app", "une appli sans nom", "einer unbenannten App", "una aplicación sin nombre", "un’applicazione senza nome", "um aplicativo sem nome"})
	case info.DeviceName == "":
		return info.Client
	}
	return s.phrase(i18n.Text{"%s on %s", "%s sur %s", "%s auf %s", "%s en %s", "%s su %s", "%s em %s"}, info.Client, info.DeviceName)
}

// addressText is a client's address, or says it is unknown.
func (s *Service) addressText(address string) string {
	if address == "" {
		return s.phrase(i18n.Text{"an unknown address", "une adresse inconnue", "einer unbekannten Adresse", "una dirección desconocida", "un indirizzo sconosciuto", "um endereço desconhecido"})
	}
	return address
}
