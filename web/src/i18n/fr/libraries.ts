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
    remove: 'Retirer',
    removeLabel: (name: string) => `Retirer ${name}`,
    removedLive: (name: string) => `${name} a été retiré des bibliothèques.`,
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
        custom: 'Personnalisée',
      },
      help: {
        none: 'Les applications affichent cette bibliothèque sans image, comme avant.',
        automatic:
          'Polyfin la prend dans le catalogue : l’arrière-plan d’un de ses premiers titres, sinon une affiche.',
        custom:
          'Une image que vous envoyez, ou que Polyfin télécharge une fois depuis une adresse. Les images larges en 16:9 conviennent le mieux aux vignettes des bibliothèques. JPEG, PNG ou WebP, 10 Mo au plus.',
      },
      parental:
        'Les utilisateurs sous contrôle parental ou qui bloquent des genres ne la voient pas.',
      notFound:
        'Rien trouvé dans ce catalogue pour l’instant : les applications n’affichent pas d’image.',
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
    } as Record<string, string>,
    negation: 'Suit ou non',
    is: 'est',
    isNot: 'n’est pas',
    genre: 'Genre',
    from: 'De',
    to: 'À',
    language: 'Langue',
    rankMax: 'Parmi ses premiers',
    types: { movie: 'Un film', series: 'Une série' } as Record<string, string>,
    states: {
      played: 'Vu',
      unplayed: 'Pas vu',
      started: 'Commencé, pas fini',
      favorite: 'Favori',
    } as Record<string, string>,
    stateHelp:
      'Les règles vu, commencé et favori suivent ce qu’a fait chaque utilisateur : chacun voit sa propre liste.',
    languageHelp:
      'La langue vient avec la description d’un titre, que Polyfin lit en arrière-plan : un titre pas encore décrit est écarté jusqu’à ce qu’il le soit.',
    sort: 'Tri',
    sorts: {
      catalog: 'Comme ses catalogues les listent, un de chaque tour à tour',
      premiere: 'Date de sortie, la plus récente d’abord',
      rating: 'Note IMDb, la plus haute d’abord',
      name: 'Nom',
      random: 'Au hasard, tiré à nouveau chaque jour',
    } as Record<string, string>,
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
        ? '1 titre attend encore sa description, qui donne sa langue : la liste grandit à mesure que les descriptions arrivent.'
        : `${count} titres attendent encore leur description, qui donne leur langue : la liste grandit à mesure que les descriptions arrivent.`,
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
}

export default libraries
