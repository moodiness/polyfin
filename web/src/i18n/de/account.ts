import type en from '../en/account'

const account: typeof en = {
  account: {
    title: 'Mein Konto',
    signedInAs: 'Angemeldet als ',
    sectionsLabel: 'Kontobereiche',
    sections: {
      statistics: 'Statistiken',
      tracking: 'Tracking',
      resume: 'Weiterschauen',
      hidden: 'Ausgeblendete Titel',
      homeRows: 'Startseitenzeilen',
      metadataLanguage: 'Metadatensprache',
      webTheme: 'Theme des Web-Players',
      audioRegion: 'Audioregion',
      sharedLinks: 'Freigabelinks',
      reminders: 'Erinnerungen',
      import: 'Von einem anderen Server importieren',
      appImport: 'Aus einer App importieren',
      notifications: 'Benachrichtigungen',
      devices: 'Geräte',
      password: 'Passwort',
      twoStep: 'Zweistufige Anmeldung',
    },
    metadataLanguage: {
      help: 'Die Sprache der Namen, Beschreibungen und Bilder von Titeln in deinen Apps, sofern eines der Add-ons des Servers sie in dieser Sprache beschreibt. Titel, die kein Add-on darin beschreibt, behalten die Beschreibung des Servers. Genres, Altersfreigaben und Personen bleiben für alle gleich. Namen, die Polyfin selbst erzeugt, etwa für Staffeln, folgen ihr, wenn Polyfin sie spricht: Englisch, Französisch, Deutsch, Spanisch, Italienisch oder Portugiesisch.',
      label: 'Metadatensprache',
      server: 'Die des Servers',
      noLanguage:
        'Noch kein Add-on des Servers hat eine Sprache, daher werden Titel in der Sprache des Servers beschrieben. Ein Administrator weist Add-ons unter Inhalte › Quellen eine Sprache zu.',
      saved: 'Metadatensprache gespeichert.',
    },
    webTheme: {
      help: 'Das Aussehen des Web-Players in jedem Browser, in dem du dich anmeldest. Das Aussehen von jellyfin-web ist leichter, für Fernseher und ältere Telefone; Browser, die für LumaaGlaass zu alt sind, erhalten es von selbst. Ein Browser zeigt eine Änderung, sobald der Web-Player dort wieder geöffnet wird oder du dich dort anmeldest.',
      label: 'Theme des Web-Players',
      server: (theme: string) => `Das des Servers (${theme})`,
      choices: {
        lumaaglaass: 'LumaaGlaass',
        none: 'Aussehen von jellyfin-web',
        custom: 'Benutzerdefiniert',
      },
      locked: 'Dein Administrator hat das Theme deines Web-Players gewählt.',
      saved: 'Theme des Web-Players gespeichert.',
    },
    audioRegion: {
      help: 'Die Synchronfassung, die du bevorzugst, wenn eine Sprache für eine Region eigens synchronisiert wird: Französisch für Frankreich oder für Québec, Spanisch für Spanien oder für Lateinamerika, Portugiesisch für Brasilien oder für Portugal. Sie gilt für die Audiosprache, die du in deinen Apps gewählt hast, sonst die des Servers, wenn es die Sprache der Region ist, und legt sie fest, wenn keine gewählt ist. In dieser Sprache werden die Audiospuren der Region zuerst abgespielt, und ihre Versionen kommen zuerst, wenn der Server Versionen nach Sprache sortiert.',
      label: 'Audioregion',
      none: 'Keine',
      saved: 'Audioregion gespeichert.',
      regions: {
        'fr-FR': 'Französisch (Frankreich)',
        'fr-CA': 'Französisch (Québec)',
        'es-ES': 'Spanisch (Spanien)',
        'es-419': 'Spanisch (Lateinamerika)',
        'pt-BR': 'Portugiesisch (Brasilien)',
        'pt-PT': 'Portugiesisch (Portugal)',
      },
    },
    homeRows: {
      help: 'Die Zeilen, die der Administrator für dich zur Startseite des Web-Players hinzufügt. Schalte eine aus, um sie von deiner Startseite zu entfernen, und auch aus deinen Bibliotheken in anderen Apps, wenn sie dort ebenfalls eine ist. Die eigenen Bereiche des Web-Players bleiben in seinen Einstellungen › Startseite.',
      empty: 'Keine Startseitenzeile',
      emptyHelp:
        'Der Administrator hat für dich keine Zeile zur Startseite des Web-Players hinzugefügt.',
      shown: (name: string) => `${name} auf deiner Startseite anzeigen`,
      on: (name: string) => `${name} ist wieder auf deiner Startseite.`,
      off: (name: string) => `${name} ist nicht mehr auf deiner Startseite.`,
    },
    watching: {
      resumeHelp:
        'Die begonnenen Titel in deinem Weiterschauen, für Apps, die keinen daraus entfernen können. Entfernen nimmt einen Titel heraus, ohne zu ändern, ob er als gesehen gilt. Ein Import aus einem Tracking-Dienst oder einer App bringt ihn nur mit einem neueren Fortsetzungspunkt zurück.',
      hiddenHelp:
        'Die Titel, die du in deinen Bibliotheken, Startseitenzeilen, Suchen und ähnlichen Titeln ausgeblendet hast. Über einen Link lassen sie sich weiterhin öffnen. Im Web-Player blendet das ⋮-Menü eines Titels ihn aus.',
      remove: 'Entfernen',
      removeLabel: (name: string) => `${name} aus Weiterschauen entfernen`,
      removed: (name: string) => `${name} ist nicht mehr in Weiterschauen.`,
      resumeEmpty: 'Nichts begonnen',
      resumeEmptyHelp: 'Filme und Folgen, die du beginnst, erscheinen hier, bis du sie beendest.',
      progress: (position: string, runtime: string) => `${position} von ${runtime}`,
      showAgain: 'Wieder anzeigen',
      showLabel: (name: string) => `${name} wieder anzeigen`,
      shown: (name: string) => `${name} ist wieder in deinen Listen.`,
      hiddenEmpty: 'Kein ausgeblendeter Titel',
      hiddenEmptyHelp: 'Blende einen Titel über sein ⋮-Menü im Web-Player aus.',
      hiddenOn: 'Ausgeblendet',
      kinds: {
        movie: 'Film',
        series: 'Serie',
        episode: 'Folge',
        video: 'Heimvideo',
        replay: 'Replay',
      } as Record<string, string>,
    },
    reminders: {
      help: 'Die Live-TV-Sendungen, an die du dich über ihre Seite im Web-Player erinnern lassen wolltest. Jede Erinnerung erscheint auf deinen Zielen, die „Sendungsbeginn“ gewählt haben, so lange vorher, wie du es unter Benachrichtigungen eingestellt hast.',
      empty: 'Keine Erinnerung',
      emptyHelp:
        'Auf der Seite einer kommenden Sendung im Web-Player legt „Erinnere mich“ eine an.',
      everyAiring: (time: string) => `Jede Ausstrahlung gegen ${time}`,
      next: 'Nächste ',
      noAiring: 'Keine Ausstrahlung im Fernsehprogramm',
      delete: 'Löschen',
      deleteLabel: (name: string) => `Erinnerung an ${name} löschen`,
      deleted: (name: string) => `Die Erinnerung an ${name} wurde gelöscht.`,
      settingsTitle: 'Erinnerungen',
      before: 'Erinnere mich vorher',
      beforeHelp: 'Wie lange vor Beginn einer Sendung ihre Erinnerung erscheint.',
      minutes: (minutes: number) =>
        minutes === 1 ? '1 Minute' : minutes === 60 ? '1 Stunde' : `${minutes} Minuten`,
      onApps: 'Erinnerungen auch in meinen Apps anzeigen',
      onAppsHelp:
        'Jede Erinnerung erscheint auch in deinen Jellyfin-Apps, die Nachrichten annehmen, etwa im Web-Player, solange er geöffnet ist, so wie „Nachricht senden“ eine anzeigt.',
      noTarget:
        'Keines deiner Ziele hat „Sendungsbeginn“ gewählt, und deine Apps sind ausgeschaltet: Deine Erinnerungen würden dich nirgends erreichen.',
      saved: 'Erinnerungseinstellungen gespeichert.',
    },
    importEvery: (minutes: number) =>
      minutes < 60
        ? `alle ${minutes} Minuten`
        : minutes === 60
          ? 'stündlich'
          : `alle ${minutes / 60} Stunden`,
    serverImport: {
      description:
        'Übernimm deine gesehenen Filme und Folgen, Fortsetzungspunkte und Favoriten von einem Jellyfin-, Emby- oder Plex-Server, den du früher genutzt hast, oder deine Filme aus deinem Letterboxd-Export. Sie landen nur in deinem Konto und ergänzen nur, was Polyfin bereits hat. Auf den anderen Server wird nichts geschrieben.',
      serverKind: 'Server',
      address: 'Serveradresse',
      addressHelp:
        'Die Adresse, unter der sich der Server im Browser öffnet, z. B. http://192.168.1.10:8096.',
      name: 'Dein Benutzername dort',
      password: 'Dein Passwort dort',
      passwordHelp:
        'Wird nur für diesen Import verwendet und nie gespeichert. Polyfin meldet sich wie die Apps des Servers mit deinem Konto an und danach wieder ab. Lass das Feld leer, wenn dein Konto kein Passwort hat.',
      start: 'Meinen Wiedergabeverlauf importieren',
      starting: 'Wird gestartet…',
      started: 'Der Import wurde gestartet. Du kannst diese Seite verlassen: Er läuft weiter.',
      lastTitle: 'Dein letzter Import',
      from: (server: string) => `Von ${server}`,
    },
    plexImport: {
      help: 'Melde dich auf der Seite von Plex bei Plex an und wähle dann den Server. Polyfin liest dort in deinem Namen die Filme und Folgen, die du abgespielt, als gesehen markiert oder synchronisiert hast, deine Fortsetzungspunkte und Bewertungen sowie deine Plex-Watchlist. Polyfin sieht dein Plex-Passwort nie: Was Plex ihm gibt, wird nur für diesen Import verwendet und danach abgemeldet.',
      signIn: 'Mit Plex anmelden',
      signingIn: 'Plex wird gefragt…',
      approve:
        'Melde dich auf der Seite von Plex an, die sich in einem neuen Tab öffnet, und komm dann hierher zurück.',
      open: 'Anmeldeseite von Plex öffnen',
      waiting: 'Warte auf deine Anmeldung bei Plex…',
      signedInAs: (name: string) => `Bei Plex angemeldet als ${name}.`,
      profile: 'Plex-Home-Profil',
      profileHelp:
        'Das Profil, dessen Wiedergabedaten übernommen werden. Ein verwaltetes Profil hat keine eigene Anmeldung: Wähle es hier.',
      managed: (name: string) => `${name} (verwaltet)`,
      pin: 'PIN des Profils',
      pinHelp: 'Die PIN, die dieses Profil bei Plex verlangt.',
      useProfile: 'Dieses Profil verwenden',
      switching: 'Wird gewechselt…',
      server: 'Server',
      serverHelp:
        'Die Plex-Server, auf die dieses Profil zugreift. Polyfin erreicht den gewählten unter den Adressen, die Plex für ihn angibt.',
      owned: (name: string) => `${name} (deiner)`,
      noServers: 'Dieses Profil greift auf keinen Plex-Server zu.',
      watchlist: 'Watchlist in meine Merkliste',
      watchlistHelp:
        'Die Filme und Serien deiner Plex-Watchlist kommen in deine Merkliste in Polyfin.',
      cancel: 'Abbrechen',
    },
    letterboxd: {
      option: 'Letterboxd (Exportdatei)',
      help: 'Öffne in Letterboxd Settings › Import & Export und wähle Export your data: Letterboxd erstellt eine ZIP-Datei. Wähle sie hier unverändert aus. Polyfin liest sie im Arbeitsspeicher und speichert sie nie.',
      watchedHelp:
        'Die Filme, die du gesehen hast, werden als gesehen markiert, mit dem letzten Datum aus deinem Tagebuch und so oft, wie es sie verzeichnet. Jeder Film wird anhand von Name und Jahr in den Metadaten-Add-ons des Servers gesucht und übernommen, wenn diese genau einen Film mit diesem Namen und Jahr kennen.',
      noSearch:
        'Kein Metadaten-Add-on dieses Servers sucht nach Filmen: Kein Film aus dem Export kann gefunden werden. Bitte einen Administrator, eines hinzuzufügen.',
      file: 'Exportdatei',
      fileHelp: 'Die ZIP-Datei von Letterboxd, höchstens 20 MB groß.',
      choose: 'Datei auswählen',
      chooseAnother: 'Andere Datei auswählen',
      tooLarge: 'Diese Datei ist größer als 20 MB: Wähle die ZIP-Datei von Letterboxd aus.',
      watchlist: 'Watchlist in meine Merkliste',
      watchlistHelp:
        'Die Filme deiner Letterboxd-Watchlist kommen in deine Merkliste in den Jellyfin-Apps, bis du sie startest.',
      ratings: 'Bewertungen',
      ratingsHelp:
        'Deine Sterne werden zu deiner Bewertung von 10: 3,5 Sterne ergeben 7. Eine Bewertung, die du in Polyfin bereits hast, bleibt erhalten.',
      likes: 'Gelikte Filme als Favoriten',
      likesHelp: 'Die Filme, die du auf Letterboxd gelikt hast, werden zu deinen Favoriten.',
      forUser: {
        watchedHelp:
          'Die gesehenen Filme werden als gesehen markiert, mit dem letzten Datum aus dem Tagebuch und so oft, wie es sie verzeichnet. Jeder Film wird anhand von Name und Jahr in den Metadaten-Add-ons des Servers gesucht und übernommen, wenn diese genau einen Film mit diesem Namen und Jahr kennen.',
        watchlist: 'Watchlist in die Merkliste des Benutzers',
        watchlistHelp:
          'Die Filme der Letterboxd-Watchlist kommen in die Merkliste des Benutzers in den Jellyfin-Apps, bis sie gestartet werden.',
        ratingsHelp:
          'Die Sterne werden zur Bewertung von 10: 3,5 Sterne ergeben 7. Eine bereits vorhandene Bewertung in Polyfin bleibt erhalten.',
        likesHelp: 'Die auf Letterboxd gelikten Filme werden zu Favoriten des Benutzers.',
      },
      start: 'Von Letterboxd importieren',
      starting: 'Wird gesendet…',
      started: 'Der Import wurde gestartet. Du kannst diese Seite verlassen: Er läuft weiter.',
      currentTitle: 'Import läuft',
      lastTitle: 'Letzter Import',
      ownLastTitle: 'Dein letzter Import',
      from: 'Aus einem Letterboxd-Export',
      startedLabel: 'Gestartet',
      endedLabel: 'Beendet',
      startedBy: (name: string) => `Gestartet von ${name}`,
      states: { running: 'Läuft', done: 'Fertig', failed: 'Fehlgeschlagen' },
      problem:
        'Bei Polyfin ist während des Imports ein Fehler aufgetreten: Details stehen im Serverprotokoll. Bereits Importiertes bleibt erhalten.',
      runningHelp:
        'Du kannst diese Seite verlassen: Der Import läuft weiter. Wird Polyfin in der Zwischenzeit neu gestartet, bricht der Import ab: Bereits Importiertes bleibt erhalten, und ein erneuter Import fügt nichts doppelt hinzu.',
      searching: (searched: number, movies: number) =>
        `Filme werden gesucht: ${searched} von ${movies}.`,
      searchingLabel: 'Gesuchte Filme',
      summary: (movies: number, added: string) =>
        `${movies === 1 ? '1 Film' : `${movies} Filme`}: ${added}.`,
      played: (count: number) => `${count} als gesehen markiert`,
      addedToWatchlist: (count: number) => `${count} zur Merkliste hinzugefügt`,
      rated: (count: number) => `${count} bewertet`,
      favorites: (count: number) => `${count} zu Favoriten gemacht`,
      unmatched: (count: number) =>
        count === 1 ? '1 Film nicht gefunden' : `${count} Filme nicht gefunden`,
      unmatchedTitle: 'Nicht gefundene Filme',
      unmatchedHelp:
        'Diese Filme wurden nicht importiert. Jeder Film wird anhand von Name und Jahr gesucht und nur übernommen, wenn die Metadaten-Add-ons des Servers genau einen Film mit diesem Namen und Jahr kennen. Öffne sie auf Letterboxd, um sie zu prüfen.',
      movieColumn: 'Film',
      reasonColumn: 'Grund',
      onLetterboxd: 'Auf Letterboxd',
      reasons: {
        not_found: 'Nicht gefunden',
        ambiguous: 'Mehrere Filme mit diesem Namen und Jahr',
        search_failed: 'Suche fehlgeschlagen',
      },
    },
    appImport: {
      description:
        'Übernimm, was du in einer App auf Basis von Add-ons geschaut hast: die abgespielten Filme und Folgen mit ihren Daten, deine Fortsetzungspunkte und die Bibliothek der App, die in Jellyfin-Apps in deine Merkliste kommt, zugeordnet über IMDb, TMDB oder Kitsu. Sie ergänzen nur, was Polyfin bereits hat, und in die App wird nichts geschrieben.',
      app: 'App',
      apps: { nuvio: 'Nuvio', stremio: 'Stremio' },
      backend: 'Nuvio-Backend',
      backendHelp:
        'Leer lassen für das eigene Backend von Nuvio. Ein selbst gehostetes Backend funktioniert ebenfalls, unter seiner Adresse, auch im lokalen Netzwerk.',
      email: 'E-Mail',
      password: 'Passwort',
      passwordHelp:
        'Wird nur zum Lesen des Kontos verwendet und nie gespeichert. Polyfin meldet sich wie die Apps von Nuvio an, liest das Konto und meldet sich dann wieder ab, außer du bleibst verbunden.',
      signIn: 'Bei Nuvio anmelden',
      signingIn: 'Wird angemeldet…',
      chooseTitle: 'Auswählen, was importiert wird',
      chooseHelp: {
        nuvio:
          'Nuvio führt für jedes Profil einen eigenen Verlauf: Wähle das Profil aus, das importiert werden soll. Polyfin behält das Gelesene nur wenige Minuten.',
        stremio:
          'Stremio führt für jedes Profil einen eigenen Verlauf, oder einen einzigen für das gesamte Konto ohne Profile: Wähle den, der importiert werden soll. Polyfin behält das Gelesene nur wenige Minuten.',
      },
      stayConnected: 'Verbunden bleiben',
      stayConnectedHelp: {
        nuvio: (every: string) =>
          `Polyfin speichert die Sitzung verschlüsselt, niemals dein Passwort, und importiert das gewählte Profil ${every} erneut. Das Konto wird nur gelesen. Du kannst die Verbindung jederzeit trennen.`,
        stremio: (every: string) =>
          `Polyfin speichert den Authentifizierungsschlüssel verschlüsselt, bei einem Profil mit PIN auch die Sitzung dieses Profils, niemals die PIN, und importiert das gewählte Profil ${every} erneut. Das Konto wird nur gelesen. Du kannst die Verbindung jederzeit trennen. Die Nutzungsbedingungen von Stremio verbieten das Auslesen der Daten mit automatisierten Mitteln: Dein Konto ${every} auszulesen, macht diese Grauzone deutlicher als ein einmaliger Import.`,
      },
      staysConnected: (every: string) =>
        `Du bleibst verbunden: Dieses Profil wird ${every} auf die gleiche Weise erneut importiert. Add-ons werden nur jetzt installiert.`,
      wholeAccount: 'Das gesamte Konto',
      profile: 'Profil',
      profileNumber: (id: string) => `Profil ${id}`,
      profileCounts: (played: number, resumes: number, library: number) =>
        `${played} gesehen, ${resumes === 1 ? '1 Fortsetzungspunkt' : `${resumes} Fortsetzungspunkte`}, ${library} in der Bibliothek.`,
      lockedProfile: (name: string) => `${name} (PIN)`,
      pin: 'PIN',
      pinHelp:
        'Dieses Profil verlangt seine PIN. Polyfin sendet sie nur an Stremio, um das Profil zu öffnen, und speichert sie nie.',
      unlock: 'Dieses Profil öffnen',
      unlocking: 'Wird geöffnet…',
      addons: 'Add-ons',
      addonsHelp:
        'Jedes angehakte Add-on wird zu einem eigenen Add-on des Kontos, unter „Meine Quellen“. Seine Adresse kann Einstellungen und Schlüssel enthalten: Polyfin zeigt sie nie vollständig an.',
      addonsOff:
        'Eigene Add-ons sind für dieses Konto deaktiviert: Die Add-ons der App werden nicht angeboten.',
      noAddons: 'Dieses Profil hat keine Add-ons.',
      back: 'Erneut anmelden',
      start: 'Dieses Profil importieren',
      starting: 'Wird gestartet…',
      started: 'Der Import wurde gestartet. Du kannst diese Seite verlassen: Er läuft weiter.',
      currentTitle: 'Import läuft',
      lastTitle: 'Letzter Import',
      ownLastTitle: 'Dein letzter Import',
      from: (app: string, profile: string) =>
        profile === '' ? `Aus ${app}` : `Aus ${app}, Profil ${profile}`,
      startedLabel: 'Gestartet',
      endedLabel: 'Beendet',
      startedBy: (name: string) => `Gestartet von ${name}`,
      states: { running: 'Läuft', done: 'Abgeschlossen', failed: 'Fehlgeschlagen' },
      problem:
        'Beim Import ist in Polyfin ein Fehler aufgetreten: Mehr dazu steht im Serverprotokoll. Was bereits importiert wurde, bleibt erhalten.',
      runningHelp:
        'Du kannst diese Seite verlassen: Der Import läuft weiter. Wird Polyfin in der Zwischenzeit neu gestartet, bricht der Import ab: Was bereits importiert wurde, bleibt erhalten, und ein erneuter Import fügt nichts doppelt hinzu.',
      read: (count: number) => (count === 1 ? '1 Titel gelesen.' : `${count} Titel gelesen.`),
      counts: (read: number, played: number, resumed: number, watchlist: number) =>
        `${read === 1 ? '1 Titel gelesen' : `${read} Titel gelesen`}: ${played} als gesehen markiert, ${resumed === 1 ? '1 Fortsetzungspunkt' : `${resumed} Fortsetzungspunkte`}, ${watchlist} zur Merkliste hinzugefügt.`,
      unmatched: (count: number) =>
        count === 1 ? '1 Titel nicht gefunden' : `${count} Titel nicht gefunden`,
      unmatchedTitle: 'Nicht gefundene Titel',
      unmatchedHelp:
        'Diese Titel wurden nicht importiert: Die App bezeichnet sie mit einer Kennung, die Polyfin nicht zuordnet, oder kein Titel in Polyfin hat diese Kennung. Bei einer Serie kann die App die gesehenen Folgen auch über ihre Position in einer Folgenliste angeben, die die Add-ons von Polyfin anders aufführen.',
      titleColumn: 'Titel',
      kindColumn: 'Art',
      reasonColumn: 'Grund',
      reasons: {
        other_identifier: 'Keine IMDb-, TMDB- oder Kitsu-Kennung',
        not_found: 'Nicht in Polyfin',
        episode_list:
          'Die Folgen stimmen nicht mit der Liste von Polyfin überein: Keine wurde als gesehen markiert',
      },
      addonInstalled: 'hinzugefügt',
      addonWaiting: 'wartet',
      addonProblems: {
        not_allowed: 'nicht hinzugefügt: Eigene Add-ons wurden deaktiviert',
        exists: 'bereits installiert',
        invalid_address: 'nicht hinzugefügt: keine Add-on-Adresse',
        private_network:
          'nicht hinzugefügt: in einem lokalen Netzwerk, das nur der Import eines Administrators erreicht',
        unreachable: 'nicht hinzugefügt: Das Add-on hat nicht geantwortet',
        invalid_manifest: 'nicht hinzugefügt: kein Add-on',
        internal: 'nicht hinzugefügt: In Polyfin ist ein Fehler aufgetreten',
      },
      stremio: {
        way: 'Anmelden mit',
        ways: {
          link: 'Einem Verknüpfungscode',
          password: 'E-Mail und Passwort',
          key: 'Einem Auth-Schlüssel',
        },
        linkHelp:
          'Polyfin fragt Stremio nach einem Code, wie es die TV-Apps von Stremio tun. Du bestätigst ihn in einem Browser, in dem du bei Stremio angemeldet bist: Das funktioniert unabhängig davon, wie sich dein Konto anmeldet, auch mit Facebook und Apple, und Polyfin sieht dein Passwort nie.',
        getCode: 'Verknüpfungscode anfordern',
        gettingCode: 'Stremio wird gefragt…',
        codeLabel: 'Verknüpfungscode',
        approve:
          'Öffne den Link in einem Browser, in dem du bei Stremio angemeldet bist, prüfe, ob dort dieser Code angezeigt wird, und bestätige ihn.',
        openLink: 'Link öffnen',
        waiting:
          'Warten auf deine Bestätigung. Der Code ist 5 Minuten gültig. Sobald du ihn bestätigst, liest Polyfin das Konto und meldet sich dann ab, außer du bleibst verbunden.',
        cancel: 'Abbrechen',
        email: 'E-Mail',
        password: 'Passwort',
        passwordHelp:
          'Wird nur zum Lesen des Kontos verwendet und nie gespeichert. Polyfin meldet sich an wie die Apps von Stremio, liest das Konto und meldet sich dann ab, außer du bleibst verbunden. Ein mit Facebook oder Apple erstelltes Konto hat kein Passwort: Verwende einen Verknüpfungscode.',
        signIn: 'Bei Stremio anmelden',
        signingIn: 'Anmeldung läuft…',
        key: 'Auth-Schlüssel',
        keyHelp:
          'Der Schlüssel, den die Web-App von Stremio nach der Anmeldung speichert: In der Konsole deines Browsers liefert ihn dort JSON.parse(localStorage.getItem("profile")).auth.key. Wird zum Lesen des Kontos verwendet und nur gespeichert, wenn du verbunden bleibst. Polyfin meldet ihn nie ab, da dies deinen Browser abmelden würde.',
        readKey: 'Konto lesen',
        reading: 'Wird gelesen…',
      },
      connections: {
        title: 'Verbundene Apps',
        help: (every: string) =>
          `Werden ${every} importiert, so wie beim ersten Import: dasselbe Profil, ohne ihre Add-ons. Polyfin liest sie nur.`,
        connectedAs: 'Verbunden als ',
        connected: 'Verbunden',
        connectedOn: (date: string) => ` seit ${date}`,
        connectedBy: (name: string) => ` von ${name}`,
        status: {
          connected: 'Verbunden',
          importing: 'Import läuft',
          paused: 'Pausiert',
          stopped: 'Erneut anmelden',
        },
        never: 'Noch nicht importiert.',
        lastImport: 'Letzter Import ',
        colon: ': ',
        nextImport: 'Nächster Import ',
        paused:
          'Der Server erlaubt Benutzern nicht mehr, ihren Wiedergabeverlauf zu importieren: Diese Verbindung ist pausiert, bis er es wieder erlaubt.',
        stopped: (app: string) =>
          `${app} hat die von Polyfin gespeicherte Anmeldung abgelehnt: Es wird nichts importiert, bis du dich unten erneut anmeldest, mit aktiviertem „Verbunden bleiben“. Durch Trennen wird sie verworfen.`,
        problems: {
          unreachable: (app: string) =>
            `${app} hat beim letzten Import nicht geantwortet. Der nächste versucht es erneut.`,
          rate_limited: (app: string) =>
            `${app} hat Polyfin beim letzten Import gebeten zu warten. Der nächste versucht es erneut.`,
          unexpected: (app: string) =>
            `${app} hat beim letzten Import auf eine Weise geantwortet, die diese Version von Polyfin nicht lesen kann. Der nächste versucht es erneut.`,
          profile_missing: (app: string) =>
            `Das importierte Profil ist nicht mehr im ${app}-Konto vorhanden. Melde dich erneut an, um ein anderes auszuwählen.`,
          profile_locked: (app: string) =>
            `Das importierte Profil verlangt in ${app} jetzt eine PIN oder eine andere. Melde dich erneut an, um es mit seiner PIN auszuwählen.`,
          sign_in_refused: (app: string) =>
            `${app} hat die von Polyfin gespeicherte Anmeldung abgelehnt.`,
          internal: () =>
            'Bei Polyfin ist während des letzten Imports ein Fehler aufgetreten: Das Serverprotokoll enthält weitere Details. Der nächste versucht es erneut.',
        },
        importNow: 'Jetzt importieren',
        importing: 'Wird importiert…',
        disconnect: 'Trennen',
        disconnectTitle: (app: string) => `${app} trennen?`,
        disconnectBody: (app: string) =>
          `Polyfin beendet den Import aus diesem ${app}-Konto und verwirft die zum Lesen gespeicherten Daten, nachdem die geöffnete Sitzung abgemeldet wurde. Ein eingefügter Auth-Schlüssel wird nur verworfen. Bereits Importiertes bleibt erhalten.`,
        disconnected: (app: string) => `${app} getrennt.`,
      },
    },
    sharedLinksHelp:
      'Teile einen Film oder eine Folge mit jemandem ohne Konto: Die Person schaut über einen Link, der abläuft, im Browser und zählt sofort zu deinen Wiedergaben.',
    statisticsHelp:
      'Was du auf diesem Server geschaut hast: Stunden, Titel, Apps und Geräte sowie deine letzten Wiedergaben.',
    notificationsHelp:
      'Hier informiert dich Polyfin über neue Folgen der Serien, denen du folgst, über deine Aufnahmen und Wiedergaben, abbrechende Verbindungen, Sperren deines Kontos und Anmeldungen auf neuen Geräten, erinnert dich an die Sendungen, die du ausgewählt hast, und schickt dir deine Wochenzusammenfassung sowie deinen Monats- und Jahresrückblick.',
    devicesHelp:
      'Die Apps, die mit deinem Konto angemeldet sind. Ein Gerät, das du abmeldest, fragt beim nächsten Öffnen nach deinem Passwort.',
    passwordHelp: 'Wenn du dein Passwort änderst, werden alle deine Jellyfin-Geräte abgemeldet.',
    currentPassword: 'Aktuelles Passwort',
    newPassword: 'Neues Passwort',
    confirmPassword: 'Neues Passwort bestätigen',
    changePassword: 'Passwort ändern',
    changingPassword: 'Wird geändert…',
    passwordChanged: 'Dein Passwort wurde geändert.',
    twoStep: {
      help: 'Nach deinem Passwort fragt diese Admin-App nach einem Code aus einer Authenticator-App auf deinem Smartphone. Jellyfin-Apps können keinen Code abfragen: Sobald die Funktion aktiv ist, melden sie sich über Quick Connect an, das du hier bestätigst, und dein Passwort allein reicht für sie nicht mehr aus. Bereits angemeldete Apps und Browser bleiben angemeldet.',
      off: 'Aus',
      on: 'Ein',
      offText: 'Du meldest dich nur mit deinem Passwort an.',
      onText:
        'Du meldest dich mit deinem Passwort und danach mit einem Code aus deiner Authenticator-App an.',
      required:
        'Administratoren müssen auf diesem Server die zweistufige Anmeldung verwenden. Aktiviere sie jetzt, sonst wirst du bei deiner nächsten Anmeldung dazu aufgefordert.',
      requiredOn:
        'Administratoren müssen auf diesem Server die zweistufige Anmeldung verwenden: Sie bleibt aktiviert.',
      unreadable:
        'POLYFIN_SECRET_KEY kann den Schlüssel deiner Authenticator-App nicht mehr lesen, etwa nach einer Wiederherstellung mit einem anderen Schlüssel: Du kannst dich nur noch mit deinen Wiederherstellungscodes anmelden. Deaktiviere die zweistufige Anmeldung mit deinem Passwort und aktiviere sie dann erneut.',
      turnOn: 'Aktivieren',
      starting: 'Wird vorbereitet…',
      scan: 'Scanne diesen QR-Code mit deiner Authenticator-App oder gib den Schlüssel manuell ein. Polyfin hat den Code selbst erstellt: Der Schlüssel wird nirgendwohin übertragen.',
      qrLabel: 'QR-Code des Schlüssels für deine Authenticator-App',
      key: 'Schlüssel',
      openInApp: 'In einer Authenticator-App auf diesem Gerät öffnen',
      code: 'Code aus der App',
      codeHelp:
        'Der 6-stellige Code, den die App für Polyfin anzeigt. Er ändert sich alle 30 Sekunden.',
      confirm: 'Aktivieren',
      confirming: 'Wird geprüft…',
      turnedOn: 'Die zweistufige Anmeldung ist aktiviert.',
      recoveryTitle: 'Deine Wiederherstellungscodes',
      recoveryHelp:
        'Mit jedem Code kannst du dich einmal anmelden, statt mit einem Code aus deiner App, falls du dein Smartphone verlierst. Bewahre sie sicher auf, zum Beispiel in einem Passwort-Manager: Sie werden nur jetzt angezeigt.',
      copy: 'Codes kopieren',
      copied: 'Die Codes wurden kopiert.',
      copyFailed: 'Die Codes konnten nicht kopiert werden: Markiere sie und kopiere sie manuell.',
      saved: 'Ich habe sie gespeichert',
      codesLeft: (n: number) =>
        n === 1 ? '1 Wiederherstellungscode übrig' : `${n} Wiederherstellungscodes übrig`,
      newCodes: 'Neue Wiederherstellungscodes',
      newCodesTitle: 'Neue Wiederherstellungscodes erstellen?',
      newCodesText: 'Die neuen Codes machen die bisherigen ungültig.',
      making: 'Wird erstellt…',
      turnOff: 'Deaktivieren',
      turnOffTitle: 'Zweistufige Anmeldung deaktivieren?',
      turnOffText: 'Du meldest dich dann nur mit deinem Passwort an, hier und in Jellyfin-Apps.',
      turningOff: 'Wird deaktiviert…',
      turnedOff: 'Die zweistufige Anmeldung ist deaktiviert.',
      confirmWith: 'Bestätigen mit',
      withCode: 'Einem Code',
      withPassword: 'Meinem Passwort',
      codeOrRecovery: 'Code aus der App oder Wiederherstellungscode',
      password: 'Passwort',
    },
    tracking: {
      title: 'Tracking',
      description:
        'Verbinde die Dienste, die festhalten, was du schaust und hörst. Polyfin teilt jedem verbundenen Dienst die Filme und Folgen mit, die du in deinen Jellyfin-Apps schaust, oder die Songs, die du abspielst.',
      loading: 'Tracking-Dienste werden geladen…',
      status: {
        connected: 'Verbunden',
        notConnected: 'Nicht verbunden',
        waiting: 'Warten auf den Code',
        waitingSignIn: 'Warten auf dich',
        reconnect: 'Erneute Verbindung nötig',
        unreachable: 'Wird erneut versucht',
        unavailable: 'Nicht eingerichtet',
        appRefused: 'App abgelehnt',
      },
      unavailable: (name: string) =>
        `${name} ist auf diesem Server noch nicht eingerichtet: Ein Administrator muss zuerst die ${name}-App in den Einstellungen hinzufügen.`,
      setUpApp: (name: string) => `${name}-App einrichten`,
      codeIntro: (name: string) =>
        `Verbinde dein ${name}-Konto: Polyfin gibt dir einen Code, den du auf der ${name}-Website eingibst.`,
      keyIntro: (name: string) => `Verbinde dein ${name}-Konto mit deinem ${name}-API-Schlüssel.`,
      signInIntro: (name: string) =>
        `Verbinde dein ${name}-Konto: Melde dich auf der ${name}-Website an und erlaube Polyfin, die Songs zu scrobbeln, die du abspielst.`,
      tokenIntro: (name: string) =>
        `Verbinde dein ${name}-Konto mit deinem ${name}-Benutzertoken, damit die Songs, die du abspielst, gescrobbelt werden.`,
      keyLabel: (name: string) => `${name}-API-Schlüssel`,
      tokenLabel: (name: string) => `${name}-Benutzertoken`,
      savedKeyHelp:
        'Nur du kannst ihn anzeigen. Um einen anderen Schlüssel zu verwenden, trenne die Verbindung und verbinde dann erneut.',
      keyHelp: {
        mdblist: 'Du findest ihn auf mdblist.com unter Preferences, im Bereich API key.',
        publicmetadb: 'Du findest ihn in deinem PublicMetaDB-Konto.',
        listenbrainz:
          'Du findest ihn auf listenbrainz.org in deinen Einstellungen, unter User token.',
      },
      connect: 'Verbinden',
      connecting: 'Wird verbunden…',
      checking: 'Schlüssel wird geprüft…',
      reconnect: 'Erneut verbinden',
      newCode: 'Neuen Code anfordern',
      signInAgain: 'Neu starten',
      enterCode: (site: string) => `Öffne ${site}, melde dich an und gib diesen Code ein:`,
      codeLabel: 'Einzugebender Code',
      openSite: (site: string) => `${site} öffnen`,
      waiting: 'Diese Seite aktualisiert sich von selbst, sobald du ihn eingegeben hast.',
      expires: (when: string) => `Der Code läuft ${when} ab.`,
      codeEnded:
        'Der Code ist abgelaufen oder wurde abgelehnt, bevor er eingegeben wurde. Fordere einen neuen Code an, um es erneut zu versuchen.',
      signInStep: (site: string) =>
        `Öffne ${site}, melde dich bei Bedarf an und erlaube Polyfin, dein Konto zu verwenden.`,
      signInWaiting: 'Diese Seite aktualisiert sich von selbst, sobald du es erlaubt hast.',
      signInExpires: (when: string) => `Diese Anfrage läuft ${when} ab.`,
      signInEnded:
        'Polyfin wurde nicht rechtzeitig erlaubt oder wurde abgelehnt. Starte neu, um es noch einmal zu versuchen.',
      pinIntro: (name: string) =>
        `Verbinde deine ${name}-Listen: Erlaube Polyfin auf der ${name}-Website und füge dann hier den Code ein, der dir angezeigt wird.`,
      redirectIntro: (name: string) =>
        `Verbinde deine ${name}-Liste: Erlaube Polyfin auf der ${name}-Website, die dich danach hierher zurückbringt.`,
      pinStep: (site: string) =>
        `Öffne ${site}, melde dich bei Bedarf an, erlaube Polyfin und kopiere dann den angezeigten Code hierher.`,
      redirectStep: (site: string) =>
        `Öffne ${site}, melde dich bei Bedarf an und erlaube Polyfin: Danach wirst du hierher zurückgebracht.`,
      codeField: (name: string) => `${name}-Code`,
      codeFieldHelp: (name: string) =>
        `Der Code, den ${name} anzeigt, sobald du Polyfin erlaubt hast.`,
      addressField: 'Adresse oder Code',
      addressFieldHelp: (name: string) =>
        `Falls dein Browser die Seite, an die ${name} ihn weitergeleitet hat, nicht öffnen konnte, kopiere die Adresse dieser Seite aus der Adressleiste hierher.`,
      pasteEnded:
        'Diese Anfrage ist abgelaufen, bevor Polyfin erlaubt wurde. Starte neu, um es noch einmal zu versuchen.',
      invalidCode: (name: string) =>
        `${name} hat diesen Code nicht akzeptiert, oder er gehört zu einer früheren Anfrage. Starte neu, um einen neuen zu erhalten.`,
      tokenExpires: (date: string) => `, bis ${date}`,
      returning: (name: string) => `Dein ${name}-Konto wird verbunden…`,
      connectedAs: 'Verbunden als ',
      connectedWithKey: 'Mit deinem API-Schlüssel verbunden',
      connected: 'Verbunden',
      connectedOn: (date: string) => ` am ${date}`,
      lastSent: ', zuletzt gesendet ',
      nothingSent: ', noch nichts gesendet',
      connectedToast: (name: string) => `${name} verbunden.`,
      disconnectedToast: (name: string) => `${name} getrennt.`,
      problemReconnect: (name: string) =>
        `${name} akzeptiert diese Verbindung nicht mehr, daher wird nichts mehr gesendet. Verbinde erneut, um wieder zu senden, was du schaust.`,
      problemUnreachable: (name: string) =>
        `${name} war in letzter Zeit nicht erreichbar. Polyfin versucht es weiter und sendet, was du geschaut hast, sobald der Dienst antwortet.`,
      disconnect: 'Trennen',
      disconnectTitle: (name: string) => `${name} trennen?`,
      disconnectBody: (name: string) =>
        `Polyfin teilt dem Dienst nicht mehr mit, was du schaust. Was bereits übertragen wurde, bleibt bei ${name}.`,
      invalidKey: (name: string) =>
        `${name} hat diesen Schlüssel nicht akzeptiert. Kopiere ihn erneut aus deinem ${name}-Konto.`,
      invalidToken: (name: string) =>
        `${name} hat dieses Token nicht akzeptiert. Kopiere es erneut aus deinen ${name}-Einstellungen.`,
      serviceUnreachable: (name: string) =>
        `${name} war nicht erreichbar. Versuche es in ein paar Minuten erneut.`,
      notAvailable: (name: string) =>
        `${name} ist auf diesem Server nicht eingerichtet: Ein Administrator muss zuerst die ${name}-App hinzufügen.`,
      appRefused: (name: string) =>
        `${name} hat die App dieses Servers abgelehnt. Ein Administrator muss ihre Zugangsdaten unter Einstellungen › Tracking prüfen.`,
      history: {
        toggle: (name: string) => `Meinen ${name}-Verlauf importieren`,
        toggleHelp: (name: string, every: string) =>
          `Markiert in Polyfin als gesehen, was du auf ${name} geschaut hast, und übernimmt, wo du aufgehört hast, damit „Weiterschauen“ und „Als Nächstes“ übereinstimmen. Wird ${every} importiert; nichts wird an ${name} zurückgesendet.`,
        listsToggle: (name: string) => `Meine ${name}-Listen importieren`,
        listsToggleHelp: (name: string, every: string) =>
          `Markiert in Polyfin die Folgen als gesehen, die deine ${name}-Listen als geschaut zählen: bis zum Fortschritt des jeweiligen Eintrags bei Watching, On hold und Dropped, alle bei Completed. Wird ${every} importiert; es wird nur hinzugefügt.`,
        listCounts: (played: number, unmapped: number) =>
          `${played === 1 ? '1 Titel' : `${played} Titel`} als gesehen markiert, ${unmapped === 1 ? '1 Titel' : `${unmapped} Titel`} nicht in Polyfin gefunden.`,
        importNow: 'Jetzt importieren',
        importing: 'Wird importiert…',
        importingStatus: 'Dein Verlauf wird importiert…',
        never: 'Noch nicht importiert.',
        lastImport: 'Letzter Import',
        colon: ': ',
        nextImport: 'Nächster Import',
        counts: (played: number, resumed: number, unmapped: number) =>
          `${played === 1 ? '1 Titel' : `${played} Titel`} als gesehen markiert, ${resumed === 1 ? '1 Fortsetzungspunkt' : `${resumed} Fortsetzungspunkte`}, ${unmapped === 1 ? '1 Titel' : `${unmapped} Titel`} nicht in Polyfin gefunden.`,
        problem: {
          reconnect: (name: string) =>
            `${name} hat die Verbindung beim letzten Import abgelehnt. Verbinde dich erneut, um deinen Verlauf zu importieren.`,
          unreachable: (name: string) =>
            `${name} konnte beim letzten Import nicht vollständig gelesen werden. Was gelesen wurde, wurde importiert; der nächste Import versucht es erneut.`,
          rate_limited: (name: string) =>
            `${name} hat Polyfin beim letzten Import gebeten zu warten. Was gelesen wurde, wurde importiert; der nächste Import versucht es erneut.`,
        },
        watchlist: {
          lists: {
            trakt: 'Watchlist',
            simkl: 'Plan-to-Watch-Liste',
            anilist: 'Plan-to-Watch-Liste',
            myanimelist: 'Plan-to-Watch-Liste',
          },
          toggle: (name: string, list: string) => `Meine ${name}-${list} importieren`,
          toggleHelp: (name: string, list: string) =>
            `Übernimmt bei jedem Import die Filme und Serien deiner ${name}-${list} in die Ansicht „Merkliste“ deiner Jellyfin-Apps. Ein Titel verschwindet daraus, sobald du ihn startest oder sobald ${name} ihn nicht mehr aufführt. Nichts wird an ${name} zurückgesendet.`,
          counts: (added: number, unmapped: number) =>
            `${added} zur Merkliste hinzugefügt, ${unmapped === 1 ? '1 Titel' : `${unmapped} Titel`} nicht in Polyfin gefunden.`,
          added: (added: number) => `${added} zur Merkliste hinzugefügt.`,
        },
        ratings: {
          toggle: 'Meine Bewertungen synchronisieren',
          toggleHelp: (name: string) =>
            `Sendet die Bewertungen, die du Filmen und Serien gibst, als deine Bewertungen an ${name} und überträgt bei jedem Import deine ${name}-Bewertungen auf deine unbewerteten Titel. Eine Serie, die ${name} in mehrere Einträge aufteilt, wird übersprungen.`,
          counts: (rated: number, skipped: number) =>
            `${rated === 1 ? '1 Titel' : `${rated} Titel`} bewertet, ${skipped === 1 ? '1 Bewertung' : `${skipped} Bewertungen`} übersprungen.`,
        },
      },
    },
  },
  shareLinks: {
    share: 'Titel teilen',
    off: 'Du kannst derzeit keine Freigabelinks erstellen: Ein Administrator hat sie für diesen Server oder für dein Konto deaktiviert. Noch aktive Links werden in der Zwischenzeit nicht abgespielt.',
    empty: 'Kein Freigabelink',
    emptyHelp:
      '„Titel teilen“ gibt einen Film oder eine Folge über einen Link frei. Im Web-Player bietet auch die Seite eines Films oder einer Folge „Teilen“ an.',
    userEmpty: 'Dieser Benutzer hat keinen Freigabelink.',
    expiresLabel: 'Läuft ab',
    expiredLabel: 'Abgelaufen',
    revokedLabel: 'Widerrufen',
    plays: (count: number) => (count === 1 ? '1 Wiedergabe' : `${count} Wiedergaben`),
    viewers: (count: number) =>
      count === 1 ? '1 Zuschauer gleichzeitig' : `${count} Zuschauer gleichzeitig`,
    states: { active: 'Aktiv', expired: 'Abgelaufen', revoked: 'Widerrufen' },
    revoke: 'Widerrufen',
    revokeLabel: (title: string) => `Link zu ${title} widerrufen`,
    revokeTitle: 'Diesen Freigabelink widerrufen?',
    revokeConfirm:
      'Der Link funktioniert sofort nicht mehr, und wer gerade darüber schaut, wird innerhalb von Sekunden gestoppt.',
    revoking: 'Wird widerrufen…',
    revoked: 'Der Freigabelink wurde widerrufen.',
    search: 'Deine Bibliotheken durchsuchen',
    searchHelp: 'Einen Film oder eine Serie, um eine ihrer Folgen auszuwählen.',
    searchButton: 'Suchen',
    searching: 'Wird gesucht…',
    noTitles: 'Kein Film und keine Serie gefunden.',
    titlesFound: (term: string) => `Gefundene Titel für ${term}`,
    pickTitle: (name: string, details: string) => `${name} teilen, ${details}`,
    episodesOf: (series: string) => `Folgen von ${series}`,
    noEpisodes: 'Diese Serie hat keine Folge, die du teilen kannst.',
    back: 'Zurück zu den Ergebnissen',
    chosen: 'Titel',
    change: 'Ändern',
    expires: 'Läuft ab nach',
    expiresHelp: 'Danach kann niemand den Link mehr öffnen. Du kannst ihn auch früher widerrufen.',
    after: (hours: number) =>
      hours === 1 ? '1 Stunde' : hours === 24 ? '1 Tag' : `${hours / 24} Tagen`,
    viewersLabel: 'Gleichzeitige Zuschauer',
    viewersHelp:
      'Wie viele Personen gleichzeitig über den Link schauen dürfen. Jede zählt als eine deiner gleichzeitigen Wiedergaben.',
    submit: 'Link erstellen',
    creating: 'Wird erstellt…',
    createdTitle: 'Link erstellt',
    createdNotice: (title: string) =>
      `Jeder mit diesem Link kann ${title} bis zum Ablauf in einem Browser ansehen, ohne Konto. Kopiere ihn jetzt: Er wird nur dieses eine Mal angezeigt.`,
    copy: 'Link kopieren',
    copied: 'Link kopiert.',
    copyFailed: 'Der Link konnte nicht kopiert werden. Markiere ihn und kopiere ihn manuell.',
    done: 'Fertig',
  },
  resetPreferences: {
    button: 'Meine Einstellungen zurücksetzen',
    title: 'Ihre Einstellungen zurücksetzen?',
    help: 'Was Sie selbst gewählt haben, erhält wieder die Standardwerte des Servers:',
    metadataLanguage: 'Metadatensprache: Die des Servers',
    webTheme: 'Theme des Web-Players: Das des Servers, außer Ihr Administrator hat Ihres gewählt',
    audioRegion: 'Audioregion: Keine',
    homeRows: 'Startseitenzeilen: Alle Zeilen werden wieder angezeigt',
    appPreferences:
      'Die Einstellungen, die Jellyfin-Apps in Ihrem Konto gespeichert haben: Audio- und Untertitelsprachen und -modus, Startbildschirm- und Anzeigeeinstellungen',
    kept: 'Was Sie angesehen haben, Ihre Favoriten, Fortsetzungspunkte, Verbindungen und Importe bleiben.',
    confirm: 'Zurücksetzen',
    resetting: 'Wird zurückgesetzt…',
    done: 'Ihre Einstellungen wurden zurückgesetzt.',
  },
}

export default account
