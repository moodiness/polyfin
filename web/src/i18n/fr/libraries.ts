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
}

export default libraries
