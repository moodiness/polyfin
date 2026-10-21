import type en from '../en/notifications'

const notifications: typeof en = {
  notifications: {
    title: 'Benachrichtigungen',
    loading: 'Ziele werden geladen…',
    empty: 'Noch kein Ziel.',
    emptyHint: {
      server:
        'Füge ein Ziel hinzu, etwa eine E-Mail-Adresse, einen Telegram-Chat oder ein ntfy-Topic, um über neue Folgen, Aufnahmen und Probleme im Systemzustand informiert zu werden.',
      own: 'Füge ein Ziel hinzu, etwa eine E-Mail-Adresse, einen Telegram-Chat oder ein ntfy-Topic, um informiert zu werden, wenn eine neue Folge einer Serie erscheint, der du folgst, oder wenn deine Aufnahmen enden.',
    },
    serverTargets: 'Serverziele',
    serverTargetsHelp:
      'Sie erhalten die Ereignisse aller Benutzer: eine Nachricht pro neuer Folge, egal wer der Serie folgt, jede Aufnahme und jede Wiedergabe. Jeder Benutzer kann unter Mein Konto › Benachrichtigungen eigene Ziele hinzufügen.',
    add: 'Ziel hinzufügen',
    addTitle: 'Ziel hinzufügen',
    editTitle: (name: string) => `${name} bearbeiten`,
    kind: 'Art',
    kinds: {
      webhook: 'Webhook',
      discord: 'Discord',
      ntfy: 'ntfy',
      email: 'E-Mail',
      telegram: 'Telegram',
      gotify: 'Gotify',
      pushover: 'Pushover',
    },
    kindHelp: {
      webhook:
        'Polyfin sendet jedes Ereignis als JSON an diese Adresse, beschrieben in der Dokumentation für App-Entwickler.',
      discord: 'Polyfin sendet jedes Ereignis als Nachricht mit Link an einen Discord-Kanal.',
      ntfy: 'Polyfin veröffentlicht jedes Ereignis in einem ntfy-Topic, das die ntfy-App auf deinem Handy anzeigt.',
      email: 'Polyfin sendet jedes Ereignis per E-Mail an diese Adresse, als Nur-Text und HTML.',
      telegram: 'Polyfin sendet jedes Ereignis über deinen Bot mit Link an einen Telegram-Chat.',
      gotify: 'Polyfin sendet jedes Ereignis an eine Anwendung deines Gotify-Servers.',
      pushover:
        'Polyfin sendet jedes Ereignis über eine deiner Anwendungen an deine Pushover-Geräte.',
    },
    emailUnavailable: {
      server:
        'E-Mail benötigt einen SMTP-Server: Speichere oben unter E-Mail einen mit Absenderadresse und füge dann das Ziel hinzu.',
      own: 'E-Mail benötigt einen SMTP-Server, den ein Administrator unter Einstellungen › Benachrichtigungen einrichtet.',
    },
    name: 'Name',
    nameHelp: 'Wird nur hier angezeigt, 1 bis 64 Zeichen.',
    webhookAddress: 'Webhook-Adresse',
    discordAddress: 'Discord-Webhook-Adresse',
    addressHelp: {
      webhook: 'Sie wird verschlüsselt gespeichert und nie wieder angezeigt, nur ihr Host.',
      discord:
        'In Discord: Kanaleinstellungen, Integrationen, Webhooks, Webhook-URL kopieren. Sie wird verschlüsselt gespeichert und nie wieder angezeigt.',
    },
    savedAddress: (host: string) => `Gespeichert, bei ${host}.`,
    ntfyServer: 'ntfy-Server',
    ntfyServerHelp: 'Leer lassen für https://ntfy.sh.',
    topic: 'Topic',
    topicHelp:
      'Das Topic, das du in der ntfy-App abonnierst. Jeder, der ein Topic auf einem öffentlichen Server kennt, kann es lesen: Wähle eines, das schwer zu erraten ist.',
    token: 'Zugriffstoken',
    tokenHelp:
      'Für ein Topic, das eine Anmeldung erfordert. Es wird verschlüsselt gespeichert und nie wieder angezeigt.',
    emailAddress: 'E-Mail-Adresse',
    emailAddressHelp: 'Wohin die Nachrichten gehen.',
    chat: 'Chat',
    chatHelp:
      'Die Nummer des Chats, etwa -1001234567890, oder der Name eines öffentlichen Kanals, etwa @news_example. Füge den Bot zuerst zum Chat hinzu.',
    botToken: 'Bot-Token',
    botTokenHelp:
      'BotFather gibt es beim Erstellen des Bots aus. Es wird verschlüsselt gespeichert und nie wieder angezeigt.',
    gotifyServer: 'Gotify-Server',
    gotifyServerHelp: 'Seine Adresse, etwa https://gotify.example.org.',
    appToken: 'Anwendungstoken',
    appTokenHelp: {
      gotify:
        'In Gotify: Apps, Create Application, dann das Token kopieren. Es wird verschlüsselt gespeichert und nie wieder angezeigt.',
      pushover:
        'Das API-Token einer auf Pushover erstellten Anwendung. Es wird verschlüsselt gespeichert und nie wieder angezeigt.',
    },
    userKey: 'Benutzerschlüssel',
    userKeyHelp:
      'Wird nach der Anmeldung im Pushover-Dashboard angezeigt, oder ein Gruppenschlüssel. Er wird verschlüsselt gespeichert und nie wieder angezeigt.',
    events: 'Ereignisse',
    eventsHelp: {
      server: 'Worüber es informiert wird, für alle Benutzer.',
      own: 'Worüber es informiert wird, was dich betrifft.',
    },
    eventNames: {
      new_episode: 'Neue Folgen',
      recording_finished: 'Aufnahme beendet',
      recording_failed: 'Aufnahme fehlgeschlagen',
      health_problem: 'Problem mit dem Systemzustand gefunden',
      health_solved: 'Problem mit dem Systemzustand behoben',
      user_joined: 'Benutzer beigetreten',
      new_version: 'Neue Version',
      playback_started: 'Wiedergabe gestartet',
      playback_paused: 'Wiedergabe pausiert',
      playback_resumed: 'Wiedergabe fortgesetzt',
      playback_stopped: 'Wiedergabe beendet',
      weekly_summary: 'Wochenzusammenfassung',
      monthly_summary: 'Monatsrückblick',
      yearly_summary: 'Jahresrückblick',
      connection_stopped: 'Verbindung beendet',
      account_blocked: 'Konto gesperrt',
      new_device: 'Anmeldung auf einem neuen Gerät',
      share_played: 'Freigabelink abgespielt',
      programme_reminder: 'Sendung beginnt',
    },
    eventHelp: {
      new_episode:
        'Eine Folge einer gesehenen oder als Favorit markierten Serie ist erschienen: einmal pro Folge, nie für bereits erschienene.',
      recording_finished: 'Eine Live-TV-Aufnahme ist beendet, vollständig oder mit fehlendem Teil.',
      recording_failed: 'Eine Live-TV-Aufnahme hat nichts aufgenommen.',
      health_problem:
        'System › Systemzustand hat bei zwei Prüfungen in Folge ein Problem gefunden.',
      health_solved: 'Ein von System › Systemzustand gefundenes Problem ist behoben.',
      user_joined: 'Jemand hat sein Konto über einen Einladungslink erstellt.',
      new_version:
        'Eine neue Version von Polyfin ist erschienen, gefunden von der täglichen Prüfung: einmal pro Version, mit den Versionshinweisen.',
      playback_started:
        'Ein Video oder ein Song wird in einer Jellyfin-App gestartet, mit Angabe, wer ihn abspielt, auf welchem Gerät und ob er konvertiert wird.',
      playback_paused:
        'Eine Wiedergabe wird pausiert. Positionen während der Wiedergabe werden nie gesendet.',
      playback_resumed: 'Eine pausierte Wiedergabe läuft weiter.',
      playback_stopped:
        'Eine Wiedergabe endet, oder ihre App hat sie fünf Minuten lang nicht mehr gemeldet.',
      weekly_summary:
        'Einmal pro Woche, an dem Tag und zu der Uhrzeit, die der Server festlegt: die geschauten Stunden, die Titel und die neuen Folgen der Woche. Die Ziele des Servers und der Administratoren erhalten die Woche des Servers, mit den beigetretenen Benutzern, den zu lokalen Ordnern hinzugefügten Dateien und den gefundenen Problemen; die anderen erhalten ihre eigene. Für eine Woche ohne Neuigkeiten wird nichts gesendet.',
      monthly_summary:
        'Am 1. jedes Monats, zur Uhrzeit der Wochenzusammenfassung: der Inhalt der Wochenzusammenfassung für den vergangenen Monat. Für einen Monat ohne Neuigkeiten wird nichts gesendet.',
      yearly_summary:
        'Am 1. Januar, zur Uhrzeit der Wochenzusammenfassung: die geschauten Stunden, die zehn am längsten abgespielten Filme und Serien und der Monat mit den meisten Stunden des vergangenen Jahres. Die Ziele des Servers und der Administratoren erhalten das Jahr des Servers, mit den beigetretenen Benutzern und den zu lokalen Ordnern hinzugefügten Dateien; die anderen erhalten ihr eigenes. Für ein Jahr ohne Neuigkeiten wird nichts gesendet.',
      connection_stopped:
        'Eine App-Verbindung oder ein Tracking-Dienst hat abgelehnt, was Polyfin gespeichert hatte, und wurde beendet: einmal pro Abbruch, mit Hinweisen, wie sie wieder verbunden wird.',
      account_blocked:
        'Falsche Passwörter haben ein Konto vorübergehend gesperrt: einmal pro Sperre, mit Angabe, bis wann, sowie Adresse und App des letzten Versuchs. Die Ziele des Servers und der Administratoren erfahren von jedem Konto, die anderen von ihrem eigenen.',
      new_device:
        'Dein Konto hat sich über eine Jellyfin-App oder den Web-Client auf einem bisher nie genutzten Gerät angemeldet, mit App, Gerät und Adresse. Das erste Gerät eines Kontos wird nicht gemeldet.',
      share_played:
        'Jemand ohne Konto hat über einen Freigabelink einen Film oder eine Folge gestartet: höchstens einmal pro Stunde und Link, mit dem Titel und wer ihn geteilt hat. Die Ziele des Servers erfahren von jedem Link, die anderen von ihren eigenen.',
      programme_reminder:
        'Eine Live-TV-Sendung, für die du im Web-Player eine Erinnerung gesetzt hast, beginnt bald, so lange vorher, wie „Remind me before“ angibt, mit einem Link zu ihrem Kanal.',
    },
    enabled: 'Nachrichten an dieses Ziel senden',
    create: 'Hinzufügen',
    creating: 'Wird hinzugefügt…',
    save: 'Speichern',
    saving: 'Wird gespeichert…',
    created: (name: string) => `${name} hinzugefügt.`,
    saved: (name: string) => `${name} gespeichert.`,
    test: 'Test senden',
    testing: 'Wird gesendet…',
    testDelivered: (name: string) => `${name} hat die Testnachricht erhalten.`,
    testFailed: (name: string) => `${name} hat die Testnachricht nicht angenommen.`,
    edit: 'Bearbeiten',
    delete: 'Löschen',
    deleteTitle: (name: string) => `${name} löschen?`,
    deleteBody:
      'Es werden keine Nachrichten mehr dorthin gesendet, und wartende Nachrichten werden verworfen.',
    deleted: (name: string) => `${name} gelöscht.`,
    status: {
      working: 'Funktioniert',
      waiting: 'Noch nichts gesendet',
      off: 'Aus',
      refused: 'Abgelehnt',
      rejected: 'Nachricht abgelehnt',
      unreachable: 'Nicht erreicht',
      unreadable: 'Erneut eingeben',
    },
    answer: (code: string, email: boolean) => (email ? `SMTP ${code}` : `HTTP ${code}`),
    problem: {
      refused: (answer: string) =>
        `Das Ziel hat Polyfin abgelehnt (${answer}): Prüfe Adresse, Token oder Schlüssel.`,
      refusedEmail: (answer: string) =>
        `Der SMTP-Server hat Polyfin abgelehnt (${answer}): Prüfe Benutzer und Passwort, die Absenderadresse und diese Adresse.`,
      rejected: (answer: string) => `Das Ziel hat die letzte Nachricht abgelehnt (${answer}).`,
      unreachable:
        'Die letzten Nachrichten konnten nicht zugestellt werden: Das Ziel hat nicht oder mit Fehlern geantwortet. Jede Nachricht wird eine Stunde lang erneut versucht.',
      unreadable:
        'Adresse, Token oder Schlüssel können mit POLYFIN_SECRET_KEY nicht entschlüsselt werden: Bis zur erneuten Eingabe wird nichts dorthin gesendet.',
    },
    lastSent: 'Letzte Nachricht ',
    nothingSent: 'Noch keine Nachricht gesendet',
    noEvents: 'Kein Ereignis ausgewählt',
  },
}

export default notifications
