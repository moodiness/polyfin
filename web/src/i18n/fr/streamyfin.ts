import type en from '../en/streamyfin'

const streamyfin: typeof en = {
  streamyfin: {
    title: 'Accueil Streamyfin',
    description:
      'Choisissez les rangées de l’écran d’accueil de Streamyfin, une appli Jellyfin, pour tous les utilisateurs.',
    rowsTitle: 'Rangées',
    rowsHelp:
      'Streamyfin affiche ces rangées sur son écran d’accueil, dans cet ordre. Sans rangée, Streamyfin affiche son propre écran d’accueil. Chaque utilisateur ne voit que les rangées des bibliothèques et des collections qu’il peut voir.',
    refreshHelp:
      'Les changements arrivent dans Streamyfin quand il actualise ses réglages : quand l’appli revient au premier plan, ou quand on tire l’écran d’accueil vers le bas.',
    loading: 'Chargement des rangées…',
    listLabel: 'Rangées de l’écran d’accueil de Streamyfin',
    columnRow: 'Rangée',
    titleLabel: 'Titre',
    kinds: {
      resume: 'Reprendre',
      nextUp: 'À suivre',
      library: 'Bibliothèque',
      collection: 'Collection',
    },
    inLibrary: (name: string) => `Dans ${name}`,
    unavailable: 'Indisponible',
    unavailableHelp:
      'Sa bibliothèque ou sa collection n’existe plus ou est désactivée : la rangée n’affiche rien.',
    removeLabel: (name: string) => `Retirer ${name}`,
    removedLive: (name: string) => `${name} a été retiré des rangées.`,
    addedLive: (name: string, position: number) => `${name} a été ajouté en rangée ${position}.`,
    emptyTitle: 'Aucune rangée',
    empty: 'Streamyfin affiche son propre écran d’accueil.',
    emptyDraft: 'Une fois enregistré, Streamyfin affiche son propre écran d’accueil.',
    suggest: 'Rangées suggérées',
    suggestedLive: (count: number) =>
      `${count} rangées suggérées ajoutées. Enregistrez pour les afficher dans Streamyfin.`,
    addTitle: 'Ajouter une rangée',
    addHelp: 'Choisissez ce que la rangée affiche. Elle est ajoutée à la fin de la liste.',
    shows: 'Affiche',
    choices: {
      resume: 'Reprendre',
      nextUp: 'À suivre',
      library: 'Une bibliothèque',
      collection: 'Les titres d’une collection',
    },
    choiceHelp: {
      resume: 'Les titres que chaque utilisateur a commencés sans les finir.',
      nextUp: 'Les prochains épisodes des séries que chaque utilisateur regarde.',
      library:
        'Une bibliothèque de films ou de séries affiche ses titres. Une bibliothèque de collections affiche ses collections.',
      collection: 'Les films et les séries d’une collection.',
    },
    library: 'Bibliothèque',
    chooseLibrary: 'Choisissez une bibliothèque',
    collectionLibrary: 'Bibliothèque de collections',
    collection: 'Collection',
    chooseCollection: 'Choisissez une collection',
    collectionsLoading: 'Chargement des collections…',
    noLibraries: 'Le serveur n’a pas encore de bibliothèque de films, de séries ou de collections.',
    noCollectionLibraries: 'Le serveur n’a pas de bibliothèque de collections.',
    noCollections: 'Cette bibliothèque n’a pas de collection.',
    added: (name: string) => `${name} (ajoutée)`,
    add: 'Ajouter la rangée',
    full: (max: number) =>
      `L’accueil de Streamyfin compte au plus ${max} rangées. Retirez-en une pour en ajouter une autre.`,
    saved: 'Accueil Streamyfin enregistré.',
  },
}

export default streamyfin
