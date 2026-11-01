import type en from '../en/libraries'

const libraries: typeof en = {
  libraries: {
    title: 'Bibliothèques',
    description:
      'Choisissez quels catalogues des addons du serveur apparaissent comme bibliothèques dans les applications Jellyfin, pour tous les utilisateurs.',
    shownTitle: 'Affichées dans les applications Jellyfin',
    shownHelp:
      'Les bibliothèques apparaissent dans cet ordre. Laissez un nom vide pour utiliser celui du catalogue. Les bibliothèques de même nom reçoivent leur type dans les applications, par exemple « Populaires (Films) ».',
    shownHelpMine:
      'Les bibliothèques apparaissent dans cet ordre, après celles du serveur si vous les utilisez. Laissez un nom vide pour utiliser celui du catalogue. Les bibliothèques de même nom reçoivent leur type dans les applications, par exemple « Populaires (Films) ».',
    defaultHelp:
      'À l’installation d’un addon, ses catalogues de collections deviennent des bibliothèques : chacune affiche ses collections (par exemple des genres ou des décennies), qui s’ouvrent sur leurs films et séries. Un addon sans collections voit ses catalogues de films, de séries et de TV devenir des bibliothèques, jusqu’à 20. Ce n’est qu’un point de départ : activez autant de catalogues que vous voulez. Chaque bibliothèque ajoute une rangée à l’écran d’accueil des applications Jellyfin, sauf les catalogues TV, dont les chaînes apparaissent dans la TV en direct.',
    manyWarning:
      'Plus de 20 bibliothèques peuvent alourdir l’écran d’accueil des applications Jellyfin.',
    noAddons: 'Ajoutez d’abord une source : ses catalogues s’afficheront ici.',
    shownEmpty: 'Aucune bibliothèque : les applications Jellyfin n’affichent rien de ces addons.',
    name: 'Nom dans les applications',
    appName: (name: string) => `Affichée dans les applications sous le nom « ${name} ».`,
    liveTv: 'Ses chaînes apparaissent dans la TV en direct des applications Jellyfin.',
    iptvVod: 'D’une source IPTV.',
    iptvVodLink: 'Ses options d’import',
    addonOff: 'Addon désactivé',
    missing: 'Plus disponible',
    hideInMenus: 'Masquer dans la barre du haut du lecteur web',
    hideInMenusHelp:
      'Elle garde sa rangée sur l’accueil. Le menu latéral du lecteur web la masque aussi ; les autres applis Jellyfin la listent toujours. S’applique au prochain chargement du lecteur web.',
    hiddenInMenus: 'Masquée dans la barre du haut',
    skipLocal: 'Omettre les titres que contiennent les dossiers locaux',
    skipLocalHelp:
      'Un film ou une série que liste la bibliothèque d’un dossier local, lié au même identifiant, n’apparaît que là. Ses versions et son état de lecture ne changent pas, et la recherche le trouve une fois. Un utilisateur qui ne voit pas la bibliothèque du dossier le trouve toujours ici.',
    skippedLocal: 'Sans les titres des dossiers locaux',
    showAsTitles: 'Afficher comme films et séries',
    showAsTitlesHelp:
      'Pour les applis Jellyfin qui ne listent que des films et des séries : un titre avec plusieurs vidéos, ou une seule numérotée comme épisode, devient une série, ses vidéos ses épisodes ; un titre sans vidéo, ou avec un seul fichier, un film. Les vidéos vues, les reprises et les favoris les suivent à l’enregistrement.',
    shownAsTitles: 'Affichée comme films et séries',
    remove: 'Retirer',
    removeLabel: (name: string) => `Retirer ${name}`,
    moreActions: (name: string) => `Autres actions pour ${name}`,
    userOrders: {
      help: 'Un utilisateur qui a ordonné les bibliothèques dans Paramètres › Accueil du lecteur web garde cet ordre, jusqu’à « Appliquer cet ordre à tous les utilisateurs » ou son propre « Rétablir mes préférences ».',
      apply: 'Appliquer cet ordre à tous les utilisateurs',
      saveFirst: 'Enregistrez d’abord la liste : c’est l’ordre enregistré qui est appliqué.',
      title: 'Appliquer cet ordre à tous les utilisateurs ?',
      counting: 'Décompte des utilisateurs qui ont leur propre ordre…',
      none: 'Tous les utilisateurs suivent déjà cet ordre : aucun n’a ordonné les bibliothèques dans Paramètres › Accueil du lecteur web.',
      some: (count: number) =>
        `${count === 1 ? '1 utilisateur a ordonné' : `${count} utilisateurs ont ordonné`} les bibliothèques dans Paramètres › Accueil du lecteur web. Leur ordre est oublié, et leurs applis listent les bibliothèques dans cet ordre, celui enregistré ; le lecteur web en une minute au plus. Leurs autres réglages restent.`,
      applied: (count: number) =>
        count === 0
          ? 'Tous les utilisateurs suivent cet ordre.'
          : count === 1
            ? 'Cet ordre s’applique désormais à tous : l’ordre propre d’un utilisateur a été oublié.'
            : `Cet ordre s’applique désormais à tous : l’ordre propre de ${count} utilisateurs a été oublié.`,
    },
    refresh: {
      start: 'Actualiser les descriptions',
      stop: 'Arrêter l’actualisation',
      queued: 'En attente d’actualiser ses descriptions',
      running: (done: string, titles: string) =>
        `Actualisation des descriptions : ${done} sur ${titles}`,
      done: (done: string, failed: number, figure: string) =>
        failed === 0
          ? `Descriptions actualisées : ${done}`
          : `Descriptions actualisées : ${done}, ${figure} non décrites`,
      stopped: (done: string, titles: string) => `Actualisation arrêtée à ${done} sur ${titles}`,
      failed: 'Ses descriptions n’ont pas pu être actualisées',
      started: (name: string) => `${name} : ses descriptions sont relues, 30 par minute.`,
      progress: (name: string) => `Actualisation de ${name}`,
    },
    removedLive: (name: string) => `${name} a été retiré des bibliothèques.`,
    dragHandle: (name: string) => `Déplacer ${name}`,
    availableTitle: 'Catalogues disponibles',
    availableHelp:
      'Ajouter un catalogue l’affiche comme bibliothèque, à la fin de la liste ci-dessus.',
    filter: 'Filtrer',
    filterPlaceholder: 'Nom du catalogue ou de l’addon',
    type: 'Type',
    allTypes: 'Tous les types',
    add: 'Ajouter',
    addLabel: (name: string) => `Ajouter ${name}`,
    addedLive: (name: string, position: number) =>
      `${name} a été ajouté comme bibliothèque n° ${position}.`,
    notBrowsable: 'Indisponible',
    notBrowsableHelp:
      'Nécessite une recherche ou une autre valeur : il ne peut pas devenir une bibliothèque.',
    availableEmpty: 'Tous les catalogues sont déjà affichés.',
    noMatch: 'Aucun catalogue ne correspond à ce filtre.',
    reset: 'Réinitialiser',
    saved: 'Bibliothèques enregistrées.',
    guideAfterSave:
      'Enregistrez les bibliothèques pour ajouter un guide des programmes à ce catalogue.',
    guideFetched: 'Dernière récupération',
    guideNever: 'Pas encore récupéré.',
    loading: 'Chargement des bibliothèques…',
    listLabel: 'Bibliothèques, dans l’ordre des applications',
    columnCatalog: 'Catalogue',
    shownEmptyTitle: 'Aucune bibliothèque pour l’instant',
    shownEmptyHow:
      'Ajoutez des catalogues depuis la liste ci-dessous : chacun devient une bibliothèque.',
    noAddonsTitle: 'Aucun catalogue pour l’instant',
    addSource: 'Ajouter une source',
    defaultTitle: 'Comment les premières bibliothèques sont choisies',
    availableLabel: (addon: string) => `Catalogues de ${addon}`,
    music: {
      library: {
        music: 'Bibliothèque musicale',
        audiobook: 'Bibliothèque de livres',
        podcast: 'Bibliothèque musicale',
      },
      help: {
        music: 'Ses éléments apparaissent dans la musique des applis Jellyfin.',
        audiobook:
          'Ses livres audio apparaissent dans une bibliothèque de livres des applis Jellyfin.',
        podcast: 'Ses épisodes apparaissent dans la musique des applis Jellyfin.',
      },
    },
    image: {
      label: 'Image dans les applications',
      edit: (name: string, state: string) => `Image de ${name} : ${state}`,
      choices: {
        none: 'Aucune',
        automatic: 'Automatique',
        generated: 'Générée',
        custom: 'Personnalisée',
      },
      help: {
        none: 'Les applications affichent cette bibliothèque sans image, comme avant.',
        automatic:
          'Polyfin la prend dans le catalogue : l’arrière-plan d’un de ses premiers titres, sinon une affiche.',
        generated:
          'Polyfin la dessine avec les affiches des premiers titres du catalogue, ou des premières collections d’une bibliothèque de collections, et le nom de la bibliothèque, une fois qu’il les a lus.',
        custom:
          'Une image que vous envoyez, ou que Polyfin télécharge une fois depuis une adresse. Les images larges en 16:9 conviennent le mieux aux vignettes des bibliothèques. JPEG, PNG ou WebP, 10 Mo au plus.',
      },
      parental:
        'Les utilisateurs sous contrôle parental ou qui bloquent des genres ne la voient pas.',
      notFound: {
        automatic:
          'Rien trouvé dans ce catalogue pour l’instant : les applications n’affichent pas d’image.',
        generated:
          'Aucune affiche lue dans ce catalogue pour l’instant : les applications n’affichent pas d’image. Polyfin la dessine une fois la bibliothèque ouverte dans une application.',
      },
      noImage: 'Aucune image',
      upload: 'Envoyer une image',
      uploadAnother: 'Envoyer une autre image',
      address: 'Ou une adresse d’image',
      addressHelp: 'Polyfin la télécharge une fois et en garde une copie.',
      useAddress: 'Utiliser cette adresse',
      remove: 'Retirer l’image',
      removeTitle: (name: string) => `Retirer l’image de ${name} ?`,
      removeToNone:
        'L’image envoyée est supprimée. Les applications affichent cette bibliothèque sans image.',
      removeToAutomatic:
        'L’image envoyée est supprimée. Les applications affichent l’image trouvée dans le catalogue.',
      removeToGenerated:
        'L’image envoyée est supprimée. Les applications affichent l’image que Polyfin dessine.',
      removeConfirm: 'Retirer',
      saved: 'Image enregistrée.',
      removed: 'Image retirée.',
      atOnce: 'Les changements d’image sont enregistrés aussitôt.',
      afterSave: 'Enregistrez d’abord les bibliothèques, puis choisissez l’image de celle-ci.',
      close: (name: string) => `Fermer l’image de ${name}`,
    },
    narrow: {
      title: 'Genre et maximum',
      edit: (name: string) => `Genre et maximum de ${name}`,
      close: (name: string) => `Fermer le genre et le maximum de ${name}`,
      staged: 'Enregistrés avec le bouton Enregistrer, comme les noms.',
      genre: 'Genre',
      allGenres: 'Tous les genres',
      genreHelp: 'Affiche seulement les titres de ce genre.',
      maxItems: 'Nombre maximum de titres',
      noMax: 'Aucun maximum',
      maxHelp: (max: string, where: string) =>
        `Affiche au plus ce nombre de titres dans la bibliothèque, et autant dans chacune de ses collections, à la place de la limite des catalogues dans ${where}. De 1 à ${max}. Vide : cette limite s’applique.`,
      maxHelpMine: (max: string) =>
        `Affiche au plus ce nombre de titres dans la bibliothèque, et autant dans chacune de ses collections, à la place de la limite du serveur. De 1 à ${max}. Vide : la limite du serveur s’applique.`,
      maxRange: (max: string) => `Saisissez un nombre entier de 1 à ${max}, ou laissez vide.`,
      maxBadge: (count: string) => `${count} max`,
    },
  },
  smartLists: {
    title: 'Listes intelligentes',
    help: 'Des listes faites de règles sur les titres des catalogues du serveur, comme « Films français des années 1990 pas encore vus ». Tous les utilisateurs les voient, parmi les bibliothèques qu’ils peuvent voir.',
    helpMine:
      'Des listes faites de règles sur les titres de vos catalogues, et de ceux du serveur tant que vous les utilisez. Vous seul les voyez.',
    add: 'Ajouter une liste intelligente',
    emptyTitle: 'Aucune liste intelligente',
    emptyHow:
      'Une liste intelligente garde les titres des catalogues choisis qui suivent ses règles, et suit ces catalogues quand ils changent.',
    loading: 'Chargement des listes intelligentes…',
    shownAsLibrary: 'Bibliothèque',
    shownAsCollection: 'Collection',
    catalogCount: (count: number) => (count === 1 ? '1 catalogue' : `${count} catalogues`),
    ruleCount: (count: number) => (count === 1 ? '1 règle' : `${count} règles`),
    maxBadge: (count: string) => `${count} max`,
    newTitle: 'Nouvelle liste intelligente',
    back: 'Bibliothèques',
    backMine: 'Mes sources',
    lede: 'Choisissez les catalogues dont elle tire ses titres et les règles qu’ils doivent suivre. L’aperçu montre ses titres tels qu’ils sont maintenant, avant d’enregistrer.',
    notFound: 'Cette liste intelligente n’existe pas.',
    name: 'Nom',
    nameHelp: 'Le nom que montrent les applications Jellyfin.',
    shownAs: 'Affichée comme',
    shownAsChoices: { library: 'Une bibliothèque', collection: 'Une collection' },
    shownAsHelp: {
      library:
        'Une bibliothèque à part entière dans les applications Jellyfin, avec sa rangée sur l’écran d’accueil : une bibliothèque de films ou de séries quand tous ses catalogues sont d’un même type.',
      collection:
        'Une collection dans la vue Collections, à côté de celles créées par les utilisateurs. Les applications ne peuvent pas y ajouter ni en retirer de titres.',
    },
    catalogs: 'Catalogues',
    catalogsHelp:
      'Ses titres viennent de ces catalogues, chacun lu jusqu’à son maximum : celui de la bibliothèque quand le catalogue en est une, sinon Titres lus par catalogue de films et séries.',
    catalogFilter: 'Filtrer les catalogues',
    catalogFilterPlaceholder: 'Nom du catalogue ou de l’addon',
    serverBadge: 'Serveur',
    noCatalogs: 'Aucun catalogue pour l’instant : ajoutez d’abord une source.',
    noCatalogMatch: 'Aucun catalogue ne correspond à ce filtre.',
    rules: 'Règles',
    rulesHelp: 'Sans règle, la liste garde tous les titres de ses catalogues.',
    exclude: 'Exclusions',
    excludeHelp:
      'Les titres qui suivent ces règles sont écartés, quoi que disent les règles ci-dessus. Sans exclusion, aucun ne l’est.',
    excludeMatch: 'Écarter les titres qui suivent',
    match: 'Les titres suivent',
    matchChoices: { all: 'Toutes les règles', any: 'Au moins une règle' },
    groupMatch: 'Ce groupe suit',
    group: 'Groupe',
    addRule: 'Ajouter une règle',
    addGroup: 'Ajouter un groupe',
    removeRule: 'Retirer cette règle',
    removeGroup: 'Retirer ce groupe',
    field: 'Porte sur',
    fields: {
      genre: 'Genre',
      year: 'Année',
      rating: 'Note IMDb',
      language: 'Langue',
      type: 'Film ou série',
      rank: 'Place dans son catalogue',
      state: 'État de visionnage',
      released: 'Date de sortie',
      homeRelease: 'Sortie en vidéo',
      votes: 'Votes IMDb',
      runtime: 'Durée',
      contentRating: 'Classification',
      person: 'Acteur ou réalisateur',
      studio: 'Studio ou chaîne',
    } as Record<string, string>,
    negation: 'Suit ou non',
    is: 'est',
    isNot: 'n’est pas',
    genre: 'Genre',
    from: 'De',
    to: 'À',
    language: 'Langue',
    rankMax: 'Parmi ses premiers',
    minutes: 'minutes',
    atMost: 'Au plus',
    releasedWhen: 'Quand',
    releasedWithin: { last: 'Depuis', next: 'D’ici' } as Record<string, string>,
    homeReleaseWithin: {
      released: 'Sortie depuis',
      upcoming: 'Sortie d’ici',
      both: 'À moins de',
    } as Record<string, string>,
    days: 'jours',
    namePlaceholder: { person: 'Son nom', studio: 'Son nom' } as Record<string, string>,
    types: { movie: 'Un film', series: 'Une série' } as Record<string, string>,
    states: {
      played: 'Vu',
      unplayed: 'Pas vu',
      started: 'Commencé, pas fini',
      favorite: 'Favori',
    } as Record<string, string>,
    stateHelp:
      'Les règles vu, commencé et favori suivent ce qu’a fait chaque utilisateur : chacun voit sa propre liste.',
    describedHelp:
      'La langue d’un titre, et sa durée, sa classification, ses personnes et sa date de sortie quand son catalogue ne les donne pas, viennent avec sa description, que Polyfin lit en arrière-plan : un titre pas encore décrit est écarté jusqu’à ce qu’il le soit. Un titre à qui manque ce que regarde une règle ne suit ni la règle ni son contraire.',
    votesHelp:
      'Les votes IMDb viennent de MDBList, avec la clé d’API MDBList des notes dans Paramètres › Contenu, cherchés en arrière-plan. Sans cette clé, aucun titre n’a de votes.',
    studioHelp:
      'Les studios et les chaînes viennent de TMDB, avec la clé d’API TMDB dans Paramètres › Contenu, cherchés en arrière-plan et gardés : la liste grandit à mesure qu’ils arrivent. Sans cette clé, seuls comptent les studios qu’un administrateur a donnés dans les métadonnées d’un titre.',
    homeReleaseHelp:
      'La sortie en vidéo d’un film est sa première sortie numérique ou physique, sinon 90 jours après sa sortie en salles, selon TMDB, avec la clé d’API TMDB dans Paramètres › Contenu, cherchée en arrière-plan et gardée : la liste grandit à mesure qu’elles arrivent. Celle d’une série est sa date de première diffusion. Avec « Attendre la sortie numérique », un film pas encore sorti n’apparaît que dans les listes avec une règle de titres à venir.',
    sort: 'Tri',
    sorts: {
      catalog: 'Comme ses catalogues les listent, un de chaque tour à tour',
      premiere: 'Date de sortie, la plus récente d’abord',
      latestEpisode: 'Dernier épisode diffusé, le plus récent d’abord',
      seasonPremiere: 'Dernier début de saison, le plus récent d’abord',
      rating: 'Note IMDb, la plus haute d’abord',
      name: 'Nom',
      random: 'Au hasard, tiré à nouveau chaque jour',
    } as Record<string, string>,
    episodeSortHelp:
      'Les séries viennent selon leur dernier épisode régulier diffusé, ou le début de leur dernière saison, comme le dit leur description, sans les épisodes spéciaux ni ceux à venir ; les films selon leur date de sortie. Une série dont la description n’est pas encore lue vient en dernier jusqu’à ce qu’elle le soit.',
    maxItems: 'Nombre maximum de titres',
    maxHelp: (max: string) => `De 1 à ${max}. Vide : pas de maximum.`,
    noMax: 'Pas de maximum',
    create: 'Créer la liste',
    save: 'Enregistrer',
    created: 'Liste intelligente créée.',
    saved: 'Liste intelligente enregistrée.',
    delete: 'Supprimer',
    deleteTitle: (name: string) => `Supprimer ${name} ?`,
    deleteBody:
      'Les applications Jellyfin ne la montrent plus. Ses titres restent dans leurs catalogues.',
    deleted: 'Liste intelligente supprimée.',
    preview: 'Aperçu',
    previewHelp:
      'Les titres que la liste vous montre maintenant. Chaque utilisateur la voit avec ses propres titres vus.',
    refresh: 'Actualiser',
    listed: (count: string) => (count === '1' ? '1 titre' : `${count} titres`),
    firstOf: (shown: string, listed: string) => `Les ${shown} premiers sur ${listed}.`,
    unknown: (count: string) =>
      count === '1'
        ? '1 titre attend encore ce que regardent ses règles : sa description, ses votes ou ses studios. La liste grandit à mesure qu’ils arrivent.'
        : `${count} titres attendent encore ce que regardent leurs règles : leur description, leurs votes ou leurs studios. La liste grandit à mesure qu’ils arrivent.`,
    incomplete:
      'Certains catalogues n’ont pas répondu à temps : la liste montre ce qui a été lu jusqu’ici, et lit le reste.',
    previewEmpty: 'Aucun titre ne suit ces règles pour l’instant.',
    previewNeedsCatalog: 'Choisissez au moins un catalogue pour voir la liste.',
    previewLabel: 'Titres de la liste',
    columns: {
      title: 'Titre',
      type: 'Type',
      year: 'Année',
      rating: 'IMDb',
      genres: 'Genres',
      languages: 'Langue',
    },
  },
  /** Episode orders: the order a series shows its seasons and episodes in, for every user. */
  episodeOrders: {
    title: 'Ordres des épisodes',
    help: 'Une série présente ses saisons et ses épisodes comme son addon les numérote, dans l’ordre de diffusion. Présentez-en une dans un autre ordre de TMDB, comme DVD, absolu ou arc narratif, pour tous les utilisateurs, comme le fait l’ordre d’affichage de l’éditeur de métadonnées de jellyfin-web. Ses épisodes gardent leur propre saison et leur numéro pour la lecture, l’historique et le suivi.',
    loading: 'Chargement des ordres des épisodes…',
    emptyTitle: 'Toutes les séries sont dans l’ordre de diffusion',
    emptyHow: 'Cherchez une série ci-dessous pour la présenter dans un autre ordre.',
    search: 'Trouver une série',
    searchPlaceholder: 'Nom de la série',
    searchButton: 'Rechercher',
    searching: 'Recherche…',
    results: 'Séries trouvées',
    noSeries: 'Aucune série ne porte ce nom.',
    editTitle: (name: string) => `Ordre des épisodes de ${name}`,
    order: 'Ordre',
    orderHelp:
      'Les groupes d’épisodes que TMDB donne pour cette série. Les parties d’un groupe deviennent ses saisons.',
    aired: 'Ordre de diffusion',
    types: {
      originalAirDate: 'Date de diffusion originale',
      absolute: 'Absolu',
      dvd: 'DVD',
      digital: 'Numérique',
      storyArc: 'Arc narratif',
      production: 'Production',
      tv: 'TV',
    },
    counts: (seasons: number, episodes: number) =>
      `${seasons === 1 ? '1 saison' : `${seasons} saisons`}, ${episodes === 1 ? '1 épisode' : `${episodes} épisodes`}`,
    lookup: {
      no_key:
        'Ajoutez une clé TMDB dans Paramètres › Contenu pour lister les ordres de cette série.',
      unknown: 'TMDB ne connaît pas cette série : elle reste dans l’ordre de diffusion.',
      failed:
        'TMDB n’a pas pu être joint pour lister les ordres de cette série. Réessayez dans un instant.',
    },
    specials: 'Épisodes spéciaux',
    specialsHelp:
      'Où la saison 0 apparaît parmi les saisons. Les épisodes spéciaux masqués se lisent encore depuis Reprendre ou un lien.',
    specialsFirst: 'En premier',
    specialsLast: 'En dernier',
    specialsHidden: 'Masqués',
    specialsSummary: { last: 'Épisodes spéciaux en dernier', hidden: 'Épisodes spéciaux masqués' },
    saved: (name: string) => `${name} : ordre des épisodes enregistré.`,
  },
}

export default libraries
