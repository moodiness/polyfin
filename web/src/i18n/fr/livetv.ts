import type en from '../en/livetv'

const livetv: typeof en = {
  livetv: {
    title: 'TV en direct',
    description:
      'Les catalogues TV viennent de vos sources. Ouvrez les guides d’un catalogue pour donner un programme à ses chaînes.',
    recordings: 'Réglages des enregistrements',
    settings: 'Réglages de la TV en direct',
    catalogsTitle: 'Catalogues TV',
    catalogsHelp:
      'Un catalogue TV affiché comme bibliothèque met ses chaînes dans la TV en direct des applications Jellyfin. Ses guides donnent leurs programmes aux chaînes.',
    catalogsLabel: 'Catalogues TV',
    catalogsLoading: 'Chargement des catalogues TV…',
    catalogsEmptyTitle: 'Aucun catalogue TV pour l’instant',
    catalogsEmpty:
      'Installez un addon qui a un catalogue TV, ou ajoutez une source IPTV, dans Sources : ses chaînes sont listées ici.',
    addSource: 'Ajouter une source',
    kinds: { stremio: 'Addon Stremio', iptv: 'Source IPTV' },
    guides: (count: number) => (count <= 1 ? `${count} guide` : `${count} guides`),
    failing: (count: number) =>
      count === 1
        ? 'Le dernier téléchargement d’un guide a échoué.'
        : `Le dernier téléchargement de ${count} guides a échoué.`,
    guideNone: 'Pas de guide des programmes',
    mapped: (mapped: string, channels: string) => `${mapped} chaînes sur ${channels} avec un guide`,
    guideLine: (position: number, message: string) => `Guide ${position} : ${message}`,
    manage: 'Guides et correspondance',
    lineup: 'Grille et guides',
    lineupLabel: (name: string) => `Grille et guides de ${name}`,
    manageLabel: (name: string) => `Guides et correspondance de ${name}`,
    notShownHelp:
      'Affichez-le comme bibliothèque pour mettre ses chaînes dans la TV en direct et lui donner des guides.',
    openLibraries: 'Ouvrir les bibliothèques',
    states: {
      notShown: 'Hors de la TV en direct',
      addonOff: 'Addon désactivé',
      missing: 'Plus disponible',
      failed: 'Échec du guide',
      noGuide: 'Sans guide',
      notFetched: 'Pas encore récupéré',
      incomplete: 'Guide incomplet',
      complete: 'Guide complet',
    },
    iptvTitle: 'Chaînes IPTV',
    iptvHelp: 'Les chaînes en direct de chaque source IPTV, telles que sa grille les garde.',
    iptvLoading: 'Chargement des sources IPTV…',
    iptvEmptyTitle: 'Aucune source IPTV pour l’instant',
    iptvEmpty:
      'Ajoutez une playlist M3U ou un compte Xtream Codes dans Sources : ses chaînes sont comptées ici.',
    kindM3u: 'Playlist M3U',
    kindXtream: 'Compte Xtream Codes',
    turnedOff: 'Désactivée',
    lastDownload: 'Dernier téléchargement',
    nextDownload: 'prochain',
    never: 'jamais',
    figuresLabel: (name: string) => `Chaînes de ${name}`,
    figures: {
      entries: 'Entrées en direct',
      enabled: 'Chaînes actives',
      shown: 'Affichées',
      mapped: 'Avec programme',
      unmapped: 'Sans programme',
    },
    noLiveTv: 'Cette source n’importe aucune chaîne en direct.',
    importOptions: 'Options d’import',
    channels: 'Chaînes',
    channelsLabel: (name: string) => `Chaînes de ${name}`,
    openMapping: 'Ouvrir les correspondances',
    recordingsTitle: 'Enregistrements',
    recordingsLoading: 'Chargement des enregistrements…',
    recordingsOffTitle: 'L’enregistrement est désactivé',
    recordingsOff:
      'Activez-le, et choisissez les marges et la durée de conservation, dans Paramètres › Enregistrements.',
    noTimersTitle: 'Aucun enregistrement prévu',
    noTimers:
      'Les enregistrements se programment depuis le guide des programmes des applications Jellyfin. Ils sont listés ici jusqu’à leur fin.',
    recordingNow: 'En cours',
    scheduled: 'Prévu',
    failed: 'Échec',
    series: 'Série',
    scheduledBy: (name: string) => `Prévu par ${name}`,
    unknownChannel: 'Chaîne introuvable',
    guidesOf: (name: string) => `Guides de ${name}`,
  },
}

export default livetv
