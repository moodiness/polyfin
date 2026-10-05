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
    count: (count: number) => (count <= 1 ? `${count} bibliothèque` : `${count} bibliothèques`),
    manyWarning:
      'Plus de 20 bibliothèques peuvent alourdir l’écran d’accueil des applications Jellyfin.',
    noAddons: 'Installez d’abord un addon : ses catalogues s’afficheront ici.',
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
    unsaved: 'Modifications non enregistrées',
    upToDate: 'Aucune modification en attente',
    reset: 'Réinitialiser',
    saved: 'Bibliothèques enregistrées.',
    guideAfterSave:
      'Enregistrez les bibliothèques pour ajouter un guide des programmes à ce catalogue.',
    guideNone: 'Pas de guide des programmes.',
    guideFetched: 'Dernière récupération',
    guideNever: 'Pas encore récupéré.',
    guideErrors: {
      unreachable:
        'La dernière récupération a échoué : impossible de télécharger le guide. Vérifiez l’adresse et réessayez.',
      private_network:
        'La dernière récupération a échoué : ce guide se trouve à une adresse du réseau local. Seuls les administrateurs peuvent utiliser ce type d’adresse.',
      too_large: 'La dernière récupération a échoué : le guide dépasse 300 Mo.',
      malformed: 'La dernière récupération a échoué : ce fichier n’est pas un guide XMLTV.',
      channels_unreachable:
        'La dernière récupération a échoué : l’addon n’a pas donné les chaînes de ce catalogue. Réessayez plus tard.',
    },
  },
}

export default libraries
