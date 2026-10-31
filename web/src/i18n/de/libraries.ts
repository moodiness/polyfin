import type en from '../en/libraries'

const libraries: typeof en = {
  libraries: {
    title: 'Bibliotheken',
    description:
      'Wähle, welche Kataloge der Server-Add-ons für alle Benutzer als Bibliotheken in Jellyfin-Apps erscheinen.',
    shownTitle: 'In Jellyfin-Apps angezeigt',
    shownHelp:
      'Bibliotheken erscheinen in dieser Reihenfolge. Lass einen Namen leer, um den Katalognamen zu verwenden. Bei Bibliotheken mit gleichem Namen wird in den Apps der Typ ergänzt, zum Beispiel „Beliebt (Filme)“.',
    shownHelpMine:
      'Bibliotheken erscheinen in dieser Reihenfolge, nach den Bibliotheken des Servers, falls du sie verwendest. Lass einen Namen leer, um den Katalognamen zu verwenden. Bei Bibliotheken mit gleichem Namen wird in den Apps der Typ ergänzt, zum Beispiel „Beliebt (Filme)“.',
    defaultHelp:
      'Wenn ein Add-on installiert wird, werden seine Sammlungskataloge zu Bibliotheken: Jede zeigt ihre Sammlungen (zum Beispiel Genres oder Jahrzehnte), die sich mit ihren Filmen und Serien öffnen. Bei einem Add-on ohne Sammlungen werden seine Film-, Serien- und TV-Kataloge zu Bibliotheken, bis zu 20. Das ist nur ein Ausgangspunkt: Aktiviere so viele Kataloge, wie du möchtest. Jede Bibliothek fügt dem Startbildschirm der Jellyfin-Apps eine Zeile hinzu, außer TV-Kataloge, deren Kanäle unter Live-TV erscheinen.',
    manyWarning: 'Mehr als 20 Bibliotheken können den Startbildschirm der Jellyfin-Apps überladen.',
    noAddons: 'Füge zuerst eine Quelle hinzu: Ihre Kataloge werden dann hier aufgelistet.',
    shownEmpty: 'Keine Bibliothek: Jellyfin-Apps zeigen nichts aus diesen Add-ons an.',
    name: 'Name in Apps',
    appName: (name: string) => `In Apps als „${name}“ angezeigt.`,
    liveTv: 'Seine Kanäle erscheinen in Jellyfin-Apps unter Live-TV.',
    iptvVod: 'Aus einer IPTV-Quelle.',
    iptvVodLink: 'Ihre Importoptionen',
    addonOff: 'Add-on deaktiviert',
    missing: 'Nicht mehr verfügbar',
    hideInMenus: 'In der oberen Leiste des Web-Players ausblenden',
    hideInMenusHelp:
      'Ihre Zeile auf dem Startbildschirm bleibt erhalten. Auch das Seitenmenü des Web-Players blendet sie aus; andere Jellyfin-Apps listen sie weiterhin auf. Gilt ab dem nächsten Laden des Web-Players.',
    hiddenInMenus: 'In der oberen Leiste ausgeblendet',
    remove: 'Entfernen',
    removeLabel: (name: string) => `${name} entfernen`,
    moreActions: (name: string) => `Weitere Aktionen für ${name}`,
    userOrders: {
      help: 'Wer die Bibliotheken in den Einstellungen › Startseite des Web-Players selbst sortiert hat, behält diese Reihenfolge, bis „Diese Reihenfolge auf alle Benutzer anwenden“ oder das eigene „Meine Einstellungen zurücksetzen“.',
      apply: 'Diese Reihenfolge auf alle Benutzer anwenden',
      saveFirst: 'Speichere zuerst die Liste: Angewendet wird die gespeicherte Reihenfolge.',
      title: 'Diese Reihenfolge auf alle Benutzer anwenden?',
      counting: 'Benutzer mit eigener Reihenfolge werden gezählt…',
      none: 'Alle Benutzer folgen bereits dieser Reihenfolge: Niemand hat die Bibliotheken in den Einstellungen › Startseite des Web-Players sortiert.',
      some: (count: number) =>
        `${count === 1 ? '1 Benutzer hat' : `${count} Benutzer haben`} die Bibliotheken in den Einstellungen › Startseite des Web-Players selbst sortiert. Ihre Reihenfolge wird verworfen, und ihre Apps zeigen die Bibliotheken in dieser, der gespeicherten Reihenfolge; der Web-Player spätestens nach einer Minute. Ihre übrigen Einstellungen bleiben.`,
      applied: (count: number) =>
        count === 0
          ? 'Alle Benutzer folgen dieser Reihenfolge.'
          : count === 1
            ? 'Diese Reihenfolge gilt jetzt für alle: Die eigene Reihenfolge von 1 Benutzer wurde verworfen.'
            : `Diese Reihenfolge gilt jetzt für alle: Die eigene Reihenfolge von ${count} Benutzern wurde verworfen.`,
    },
    refresh: {
      start: 'Beschreibungen aktualisieren',
      stop: 'Aktualisierung stoppen',
      queued: 'Wartet auf die Aktualisierung seiner Beschreibungen',
      running: (done: string, titles: string) =>
        `Beschreibungen werden aktualisiert: ${done} von ${titles}`,
      done: (done: string, failed: number, figure: string) =>
        failed === 0
          ? `Beschreibungen aktualisiert: ${done}`
          : `Beschreibungen aktualisiert: ${done}, ${figure} nicht beschrieben`,
      stopped: (done: string, titles: string) =>
        `Aktualisierung bei ${done} von ${titles} gestoppt`,
      failed: 'Seine Beschreibungen konnten nicht aktualisiert werden',
      started: (name: string) => `${name}: Die Beschreibungen werden neu gelesen, 30 pro Minute.`,
      progress: (name: string) => `Aktualisierung von ${name}`,
    },
    removedLive: (name: string) => `${name} aus den Bibliotheken entfernt.`,
    dragHandle: (name: string) => `${name} verschieben`,
    availableTitle: 'Verfügbare Kataloge',
    availableHelp:
      'Ein hinzugefügter Katalog wird als Bibliothek am Ende der obigen Liste angezeigt.',
    filter: 'Filter',
    filterPlaceholder: 'Katalog- oder Add-on-Name',
    type: 'Typ',
    allTypes: 'Alle Typen',
    add: 'Hinzufügen',
    addLabel: (name: string) => `${name} hinzufügen`,
    addedLive: (name: string, position: number) =>
      `${name} als Bibliothek ${position} hinzugefügt.`,
    notBrowsable: 'Nicht verfügbar',
    notBrowsableHelp:
      'Benötigt eine Suche oder einen anderen Wert und kann daher keine Bibliothek sein.',
    availableEmpty: 'Alle Kataloge werden bereits angezeigt.',
    noMatch: 'Kein Katalog entspricht diesem Filter.',
    reset: 'Zurücksetzen',
    saved: 'Bibliotheken gespeichert.',
    guideAfterSave:
      'Speichere die Bibliotheken, um diesem Katalog ein Fernsehprogramm hinzuzufügen.',
    guideFetched: 'Zuletzt abgerufen',
    guideNever: 'Noch nicht abgerufen.',
    loading: 'Bibliotheken werden geladen…',
    listLabel: 'Bibliotheken in der Reihenfolge, in der Apps sie anzeigen',
    columnCatalog: 'Katalog',
    shownEmptyTitle: 'Noch keine Bibliothek',
    shownEmptyHow: 'Füge Kataloge aus der Liste unten hinzu: Jeder wird zu einer Bibliothek.',
    noAddonsTitle: 'Noch kein Katalog',
    addSource: 'Quelle hinzufügen',
    defaultTitle: 'Wie die ersten Bibliotheken ausgewählt werden',
    availableLabel: (addon: string) => `Kataloge von ${addon}`,
    music: {
      library: {
        music: 'Musikbibliothek',
        audiobook: 'Buchbibliothek',
        podcast: 'Musikbibliothek',
      } as Record<string, string>,
      help: {
        music: 'Seine Elemente erscheinen in der Musikansicht der Jellyfin-Apps.',
        audiobook: 'Seine Hörbücher erscheinen in einer Buchbibliothek der Jellyfin-Apps.',
        podcast: 'Seine Folgen erscheinen in der Musikansicht der Jellyfin-Apps.',
      } as Record<string, string>,
    },
    image: {
      label: 'Bild in Apps',
      edit: (name: string, state: string) => `Bild von ${name}: ${state}`,
      choices: {
        none: 'Keins',
        automatic: 'Automatisch',
        generated: 'Generiert',
        custom: 'Eigenes',
      },
      help: {
        none: 'Apps zeigen diese Bibliothek wie bisher ohne Bild an.',
        automatic:
          'Polyfin übernimmt es aus dem Katalog: den Hintergrund eines der ersten Titel, sonst ein Poster.',
        generated:
          'Polyfin erstellt es aus den Postern der ersten Titel des Katalogs (oder der ersten Sammlungen einer Sammlungsbibliothek) zusammen mit dem Namen der Bibliothek, sobald es sie gelesen hat.',
        custom:
          'Ein Bild, das du hochlädst, oder eines, das Polyfin einmalig von einer Adresse herunterlädt. Breite Bilder im Format 16:9 passen am besten zu Bibliothekskacheln. JPEG, PNG oder WebP, höchstens 10 MB.',
      },
      parental: 'Benutzer mit Kindersicherung oder gesperrten Genres sehen es nicht.',
      notFound: {
        automatic:
          'In diesem Katalog wurde vorerst nichts gefunden, daher zeigen Apps kein Bild an.',
        generated:
          'Aus diesem Katalog wurde noch kein Poster gelesen, daher zeigen Apps kein Bild an. Polyfin erstellt es, sobald die Bibliothek in einer App geöffnet wird.',
      },
      noImage: 'Kein Bild',
      upload: 'Bild hochladen',
      uploadAnother: 'Anderes Bild hochladen',
      address: 'Oder eine Bildadresse',
      addressHelp: 'Polyfin lädt es einmalig herunter und speichert eine Kopie.',
      useAddress: 'Diese Adresse verwenden',
      remove: 'Bild entfernen',
      removeTitle: (name: string) => `Bild von ${name} entfernen?`,
      removeToNone:
        'Das hochgeladene Bild wird gelöscht. Apps zeigen diese Bibliothek ohne Bild an.',
      removeToAutomatic:
        'Das hochgeladene Bild wird gelöscht. Apps zeigen stattdessen das im Katalog gefundene Bild an.',
      removeToGenerated:
        'Das hochgeladene Bild wird gelöscht. Apps zeigen stattdessen das von Polyfin erstellte Bild an.',
      removeConfirm: 'Entfernen',
      saved: 'Bild gespeichert.',
      removed: 'Bild entfernt.',
      atOnce: 'Bildänderungen werden sofort gespeichert.',
      afterSave: 'Speichere zuerst die Bibliotheken und wähle dann das Bild dieser Bibliothek.',
      close: (name: string) => `Bild von ${name} schließen`,
    },
    narrow: {
      title: 'Genre und Maximum',
      edit: (name: string) => `Genre und Maximum von ${name}`,
      close: (name: string) => `Genre und Maximum von ${name} schließen`,
      staged: 'Wird wie die Namen mit der Schaltfläche „Speichern“ gespeichert.',
      genre: 'Genre',
      allGenres: 'Alle Genres',
      genreHelp: 'Zeigt nur die Titel dieses Genres an.',
      maxItems: 'Maximale Anzahl Titel',
      noMax: 'Kein Maximum',
      maxHelp: (max: string, where: string) =>
        `Zeigt höchstens so viele Titel in der Bibliothek an, und ebenso viele in jeder ihrer Sammlungen, anstelle des Katalog-Limits in ${where}. Von 1 bis ${max}. Leer: Dieses Limit gilt.`,
      maxHelpMine: (max: string) =>
        `Zeigt höchstens so viele Titel in der Bibliothek an, und ebenso viele in jeder ihrer Sammlungen, anstelle des Server-Limits. Von 1 bis ${max}. Leer: Das Server-Limit gilt.`,
      maxRange: (max: string) =>
        `Gib eine ganze Zahl von 1 bis ${max} ein oder lass das Feld leer.`,
      maxBadge: (count: string) => `max. ${count}`,
    },
  },
  smartLists: {
    title: 'Intelligente Listen',
    help: 'Listen aus Regeln über die Titel der Kataloge des Servers, etwa „Französische Filme der 1990er, noch nicht gesehen“. Alle Benutzer sehen sie, innerhalb der Bibliotheken, die sie sehen dürfen.',
    helpMine:
      'Listen aus Regeln über die Titel deiner Kataloge und der Kataloge des Servers, solange du sie nutzt. Nur du siehst sie.',
    add: 'Intelligente Liste hinzufügen',
    emptyTitle: 'Noch keine intelligente Liste',
    emptyHow:
      'Eine intelligente Liste enthält die Titel der gewählten Kataloge, die ihren Regeln entsprechen, und folgt diesen Katalogen, wenn sie sich ändern.',
    loading: 'Intelligente Listen werden geladen…',
    shownAsLibrary: 'Bibliothek',
    shownAsCollection: 'Sammlung',
    catalogCount: (count: number) => (count === 1 ? '1 Katalog' : `${count} Kataloge`),
    ruleCount: (count: number) => (count === 1 ? '1 Regel' : `${count} Regeln`),
    maxBadge: (count: string) => `max. ${count}`,
    newTitle: 'Neue intelligente Liste',
    back: 'Bibliotheken',
    backMine: 'Meine Quellen',
    lede: 'Wähle die Kataloge, aus denen sie schöpft, und die Regeln, denen ihre Titel entsprechen. Die Vorschau zeigt ihre Titel im aktuellen Stand, bevor du speicherst.',
    notFound: 'Diese intelligente Liste existiert nicht.',
    name: 'Name',
    nameHelp: 'Der Name, den Jellyfin-Apps anzeigen.',
    shownAs: 'Anzeigen als',
    shownAsChoices: { library: 'Eine Bibliothek', collection: 'Eine Sammlung' },
    shownAsHelp: {
      library:
        'Eine eigene Bibliothek in Jellyfin-Apps, mit eigener Zeile auf dem Startbildschirm: eine Film- oder Serienbibliothek, wenn alle ihre Kataloge vom selben Typ sind.',
      collection:
        'Eine Sammlung in der Ansicht „Sammlungen“, neben denen der Benutzer. Apps können keine Titel hinzufügen oder entfernen.',
    },
    catalogs: 'Kataloge',
    catalogsHelp:
      'Ihre Titel stammen aus diesen Katalogen, jeweils bis zu ihrem Maximum gelesen: dem der Bibliothek, wenn der Katalog eine deiner Bibliotheken ist, sonst „Gelesene Titel pro Film- und Serienkatalog“.',
    catalogFilter: 'Kataloge filtern',
    catalogFilterPlaceholder: 'Katalog- oder Add-on-Name',
    serverBadge: 'Server',
    noCatalogs: 'Noch kein Katalog verfügbar: Füge zuerst eine Quelle hinzu.',
    noCatalogMatch: 'Kein Katalog entspricht diesem Filter.',
    rules: 'Regeln',
    rulesHelp: 'Ohne Regeln enthält die Liste alle Titel ihrer Kataloge.',
    exclude: 'Ausschlüsse',
    excludeHelp:
      'Titel, die diese Regeln erfüllen, werden weggelassen, was auch immer die Regeln oben sagen. Ohne Ausschlüsse wird keiner weggelassen.',
    excludeMatch: 'Titel weglassen, die erfüllen',
    match: 'Titel erfüllen',
    matchChoices: { all: 'Alle Regeln', any: 'Mindestens eine Regel' },
    groupMatch: 'Diese Gruppe erfüllt',
    group: 'Gruppe',
    addRule: 'Regel hinzufügen',
    addGroup: 'Gruppe hinzufügen',
    removeRule: 'Diese Regel entfernen',
    removeGroup: 'Diese Gruppe entfernen',
    field: 'Prüft',
    fields: {
      genre: 'Genre',
      year: 'Jahr',
      rating: 'IMDb-Bewertung',
      language: 'Sprache',
      type: 'Film oder Serie',
      rank: 'Platz im Katalog',
      state: 'Wiedergabestatus',
      released: 'Erscheinungsdatum',
      homeRelease: 'Heimkino-Veröffentlichung',
      votes: 'IMDb-Stimmen',
      runtime: 'Laufzeit',
      contentRating: 'Altersfreigabe',
      person: 'Schauspieler oder Regie',
      studio: 'Studio oder Sender',
    } as Record<string, string>,
    negation: 'Trifft zu oder nicht',
    is: 'ist',
    isNot: 'ist nicht',
    genre: 'Genre',
    from: 'Von',
    to: 'Bis',
    language: 'Sprache',
    rankMax: 'Unter den ersten',
    minutes: 'Minuten',
    atMost: 'Höchstens',
    releasedWhen: 'Wann',
    releasedWithin: { last: 'In den letzten', next: 'In den nächsten' } as Record<string, string>,
    homeReleaseWithin: {
      released: 'Erschienen in den letzten',
      upcoming: 'Erscheint in den nächsten',
      both: 'Innerhalb von',
    } as Record<string, string>,
    days: 'Tagen',
    namePlaceholder: { person: 'Name', studio: 'Name' } as Record<string, string>,
    types: { movie: 'Ein Film', series: 'Eine Serie' } as Record<string, string>,
    states: {
      played: 'Gesehen',
      unplayed: 'Nicht gesehen',
      started: 'Begonnen, nicht beendet',
      favorite: 'Favorit',
    } as Record<string, string>,
    stateHelp:
      'Regeln zu „Gesehen“, „Begonnen“ und „Favorit“ richten sich nach dem, was jeder Benutzer getan hat: Jeder sieht seine eigene Liste.',
    describedHelp:
      'Die Sprache eines Titels, und seine Laufzeit, Altersfreigabe, Personen und sein Erscheinungsdatum, wenn sein Katalog sie nicht angibt, kommen mit seiner Beschreibung, die Polyfin im Hintergrund liest: Ein noch nicht beschriebener Titel wird weggelassen, bis er es ist. Ein Titel, dem fehlt, worauf eine Regel schaut, erfüllt weder die Regel noch ihr Gegenteil.',
    votesHelp:
      'IMDb-Stimmen kommen von MDBList, mit dem MDBList-API-Schlüssel für Bewertungen unter Einstellungen › Inhalte, im Hintergrund abgefragt. Ohne diesen Schlüssel hat kein Titel Stimmen.',
    studioHelp:
      'Studios und Sender kommen von TMDB, mit dem TMDB-API-Schlüssel unter Einstellungen › Inhalte, im Hintergrund abgefragt und behalten: Die Liste wächst, während sie eintreffen. Ohne diesen Schlüssel zählen nur die Studios, die ein Administrator in den Metadaten eines Titels gesetzt hat.',
    homeReleaseHelp:
      'Die Heimkino-Veröffentlichung eines Films ist seine erste digitale oder physische Veröffentlichung, sonst 90 Tage nach dem Kinostart, laut TMDB mit dem TMDB-API-Schlüssel unter Einstellungen › Inhalte, im Hintergrund abgefragt und behalten: Die Liste wächst, während sie eintreffen. Bei einer Serie ist es ihr Premierendatum. Mit „Auf die digitale Veröffentlichung warten“ erscheint ein noch nicht erschienener Film nur in Listen mit einer Regel für kommende Titel.',
    sort: 'Sortierung',
    sorts: {
      catalog: 'Wie ihre Kataloge sie auflisten, abwechselnd einer aus jedem',
      premiere: 'Erscheinungsdatum, neueste zuerst',
      latestEpisode: 'Neueste ausgestrahlte Folge, neueste zuerst',
      seasonPremiere: 'Neuester Staffelstart, neueste zuerst',
      rating: 'IMDb-Bewertung, höchste zuerst',
      name: 'Name',
      random: 'Zufällig, täglich neu gemischt',
    } as Record<string, string>,
    episodeSortHelp:
      'Serien kommen nach ihrer neuesten regulären ausgestrahlten Folge oder ihrem neuesten Staffelstart, wie ihre Beschreibung sagt, ohne Specials und kommende Folgen; Filme nach ihrem Erscheinungsdatum. Eine Serie, deren Beschreibung noch nicht gelesen ist, kommt zuletzt, bis sie es ist.',
    maxItems: 'Maximale Anzahl Titel',
    maxHelp: (max: string) => `Von 1 bis ${max}. Leer: kein Maximum.`,
    noMax: 'Kein Maximum',
    create: 'Liste erstellen',
    save: 'Speichern',
    created: 'Intelligente Liste erstellt.',
    saved: 'Intelligente Liste gespeichert.',
    delete: 'Löschen',
    deleteTitle: (name: string) => `${name} löschen?`,
    deleteBody: 'Jellyfin-Apps zeigen sie nicht mehr an. Ihre Titel bleiben in ihren Katalogen.',
    deleted: 'Intelligente Liste gelöscht.',
    preview: 'Vorschau',
    previewHelp:
      'Die Titel, die dir die Liste jetzt zeigt. Jeder Benutzer sieht sie mit seinen eigenen gesehenen Titeln.',
    refresh: 'Aktualisieren',
    listed: (count: string) => (count === '1' ? '1 Titel' : `${count} Titel`),
    firstOf: (shown: string, listed: string) => `Die ersten ${shown} von ${listed}.`,
    unknown: (count: string) =>
      count === '1'
        ? '1 Titel wartet noch auf das, worauf seine Regeln schauen: seine Beschreibung, seine Stimmen oder seine Studios. Die Liste wächst, während sie eintreffen.'
        : `${count} Titel warten noch auf das, worauf ihre Regeln schauen: ihre Beschreibung, ihre Stimmen oder ihre Studios. Die Liste wächst, während sie eintreffen.`,
    incomplete:
      'Einige Kataloge haben nicht rechtzeitig geantwortet: Die Liste zeigt, was bisher gelesen wurde, und liest den Rest.',
    previewEmpty: 'Derzeit passt kein Titel zu diesen Regeln.',
    previewNeedsCatalog: 'Wähle mindestens einen Katalog aus, um die Liste zu sehen.',
    previewLabel: 'Titel der Liste',
    columns: {
      title: 'Titel',
      type: 'Typ',
      year: 'Jahr',
      rating: 'IMDb',
      genres: 'Genres',
      languages: 'Sprache',
    },
  },
  /** Episode orders: the order a series shows its seasons and episodes in, for every user. */
  episodeOrders: {
    title: 'Episodenreihenfolgen',
    help: 'Eine Serie zeigt ihre Staffeln und Folgen so, wie ihr Addon sie nummeriert, in Ausstrahlungsreihenfolge. Zeige eine Serie für alle Nutzer in einer anderen Reihenfolge von TMDB, etwa DVD, absolut oder Handlungsbogen, wie es die Anzeigereihenfolge im Metadaten-Editor von jellyfin-web tut. Ihre Folgen behalten für Wiedergabe, Verlauf und Tracking ihre eigene Staffel und Nummer.',
    loading: 'Episodenreihenfolgen werden geladen…',
    emptyTitle: 'Alle Serien erscheinen in Ausstrahlungsreihenfolge',
    emptyHow: 'Suche unten eine Serie, um sie in einer anderen Reihenfolge zu zeigen.',
    search: 'Serie finden',
    searchPlaceholder: 'Name der Serie',
    searchButton: 'Suchen',
    searching: 'Suche läuft…',
    results: 'Gefundene Serien',
    noSeries: 'Keine Serie trägt diesen Namen.',
    editTitle: (name: string) => `Episodenreihenfolge von ${name}`,
    order: 'Reihenfolge',
    orderHelp:
      'Die Episodengruppen, die TMDB für diese Serie kennt. Die Teile einer Gruppe werden ihre Staffeln.',
    aired: 'Ausstrahlung',
    types: {
      originalAirDate: 'Erstausstrahlung',
      absolute: 'Absolut',
      dvd: 'DVD',
      digital: 'Digital',
      storyArc: 'Handlungsbogen',
      production: 'Produktion',
      tv: 'TV',
    },
    counts: (seasons: number, episodes: number) =>
      `${seasons === 1 ? '1 Staffel' : `${seasons} Staffeln`}, ${episodes === 1 ? '1 Folge' : `${episodes} Folgen`}`,
    lookup: {
      no_key:
        'Füge unter Einstellungen › Inhalte einen TMDB-Schlüssel hinzu, um die Reihenfolgen dieser Serie aufzulisten.',
      unknown: 'TMDB kennt diese Serie nicht: Sie erscheint in Ausstrahlungsreihenfolge.',
      failed:
        'TMDB war nicht erreichbar, um die Reihenfolgen dieser Serie aufzulisten. Versuche es gleich noch einmal.',
    },
    specials: 'Specials',
    specialsHelp:
      'Wo Staffel 0 unter den Staffeln erscheint. Ausgeblendete Specials lassen sich weiter aus Weiterschauen oder über einen Link abspielen.',
    specialsFirst: 'Zuerst',
    specialsLast: 'Zuletzt',
    specialsHidden: 'Ausgeblendet',
    specialsSummary: { last: 'Specials zuletzt', hidden: 'Specials ausgeblendet' },
    saved: (name: string) => `${name}: Episodenreihenfolge gespeichert.`,
  },
}

export default libraries
