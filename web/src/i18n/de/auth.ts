import type en from '../en/auth'

const auth: typeof en = {
  setup: {
    title: 'Willkommen bei Polyfin',
    description:
      'Es gibt noch keinen Administrator. Erstelle das erste Konto, um diesen Server zu verwalten und dich in Jellyfin-Apps anzumelden.',
    codeHelp: {
      container:
        'Aus Sicherheitsgründen hat Polyfin beim Start einen einmaligen Einrichtungscode in sein Protokoll geschrieben. Führe docker logs <container> aus (öffne unter Unraid das Container-Protokoll) und suche die Zeile mit dem Einrichtungscode.',
      computer:
        'Aus Sicherheitsgründen hat Polyfin beim Start einen einmaligen Einrichtungscode in sein Protokoll geschrieben. Wähle in Polyfin Desktop im Menü „Einrichtungscode“, um ihn zu kopieren. Andernfalls suche im Protokoll von Polyfin die Zeile mit dem Einrichtungscode.',
    },
    setupCode: 'Einrichtungscode',
    setupCodeHint:
      'Format XXXX-XXXX. Groß- und Kleinschreibung sowie Bindestrich spielen keine Rolle.',
    name: 'Name des Administrators',
    password: 'Passwort',
    confirmPassword: 'Passwort bestätigen',
    submit: 'Administrator erstellen',
    submitting: 'Wird erstellt…',
  },
  login: {
    title: 'Anmelden',
    description: 'Melde dich mit deinem Polyfin-Konto an, um fortzufahren.',
    name: 'Name',
    password: 'Passwort',
    submit: 'Anmelden',
    submitting: 'Wird angemeldet…',
    codeTitle: 'Zweistufige Anmeldung',
    codeDescription:
      'Gib den 6-stelligen Code ein, den deine Authenticator-App für Polyfin anzeigt.',
    code: 'Code',
    codeHelp: 'Handy verloren? Gib stattdessen einen deiner Wiederherstellungscodes ein.',
    verify: 'Bestätigen',
    verifying: 'Wird überprüft…',
    restart: 'Als andere Person anmelden',
    setupTitle: 'Zweistufige Anmeldung einrichten',
    setupDescription:
      'Administratoren müssen auf diesem Server die zweistufige Anmeldung verwenden. Füge dein Konto zu einer Authenticator-App auf deinem Handy hinzu und gib dann den angezeigten Code ein.',
    recoveryTitle: 'Bewahre deine Wiederherstellungscodes auf',
    continue: 'Weiter',
    providerSignIn: (name: string) => `Mit ${name} anmelden`,
    providerOutcomes: {
      provider_refused: 'Die Anmeldung beim Identitätsanbieter wurde abgebrochen.',
      provider_no_account:
        'Kein Konto dieses Servers passt zu deinem Konto beim Identitätsanbieter.',
      account_disabled: 'Dieses Konto ist deaktiviert.',
      outside_allowed_hours: 'Dieses Konto darf sich zu dieser Zeit nicht anmelden.',
      too_many_attempts: 'Zu viele Versuche. Versuche es später erneut.',
      provider_failed:
        'Die Anmeldung beim Identitätsanbieter ist fehlgeschlagen. Versuche es erneut.',
    },
  },
  invite: {
    title: (server: string) => `${server} beitreten`,
    description:
      'Wähle den Namen und das Passwort für dein Konto. Du verwendest sie auch in Jellyfin-Apps.',
    name: 'Dein Name',
    password: 'Passwort',
    confirmPassword: 'Passwort bestätigen',
    submit: 'Mein Konto erstellen',
    submitting: 'Wird erstellt…',
    gone: {
      invite_unknown: {
        title: 'Dieser Einladungslink funktioniert nicht',
        description:
          'Prüfe, ob der vollständige Link kopiert wurde, oder bitte die Person, die ihn dir geschickt hat, um einen neuen.',
      },
      invite_used_up: {
        title: 'Dieser Einladungslink ist aufgebraucht',
        description:
          'Er hat bereits alle möglichen Konten erstellt. Bitte die Person, die ihn dir geschickt hat, um einen neuen.',
      },
      invite_expired: {
        title: 'Dieser Einladungslink ist abgelaufen',
        description: 'Bitte die Person, die ihn dir geschickt hat, um einen neuen.',
      },
      invite_revoked: {
        title: 'Dieser Einladungslink wurde widerrufen',
        description:
          'Er funktioniert nicht mehr. Bitte die Person, die ihn dir geschickt hat, um einen neuen.',
      },
    },
  },
}

export default auth
