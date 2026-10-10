import type en from '../en/statistics'

const statistics: typeof en = {
  statistics: {
    title: 'Statistiques',
    description:
      'Comment le serveur est utilisé : heures regardées, titres, applis et appareils, et comment les vidéos arrivent aux applis. Les morceaux et les livres audio ne sont pas comptés.',
    period: 'Période',
    periods: { '7d': '7 jours', '30d': '30 jours', year: 'Année', all: 'Tout' },
    user: 'Utilisateur',
    allUsers: 'Tous les utilisateurs',
    exportCsv: 'Exporter en CSV',
    historyOff: {
      server:
        'L’historique de lecture est désactivé : les nouvelles lectures ne sont pas comptées.',
      own: 'L’historique de lecture est désactivé sur ce serveur : les nouvelles lectures ne sont pas comptées.',
    },
    openSettings: 'Ouvrir Paramètres › Lecture',
    empty: 'Aucune lecture sur cette période',
    emptyHint:
      'Les vidéos lues au moins 10 secondes dans une appli Jellyfin apparaissent ici une fois arrêtées.',
    figures: { played: 'Heures regardées', plays: 'Lectures', users: 'Utilisateurs' },
    perDay: 'Heures regardées par jour',
    perMonth: 'Heures regardées par mois',
    perUser: 'Heures regardées par utilisateur',
    movies: 'Films les plus regardés',
    series: 'Séries les plus regardées',
    channels: 'Chaînes les plus regardées',
    apps: 'Applis',
    devices: 'Appareils',
    methods: 'Comment les vidéos sont lues',
    methodNames: {
      direct_play: 'Lecture directe',
      direct_stream: 'Remux',
      conversion: 'Conversion',
      '': 'Inconnu',
    },
    hours: 'Heures les plus chargées',
    hoursHelp: 'Heures regardées à chaque heure de la semaine, dans votre fuseau horaire.',
    hoursCell: (day: string, hour: string, time: string) => `${day} ${hour} : ${time}`,
    plays: (count: number) => (count <= 1 ? `${count} lecture` : `${count} lectures`),
    users: (count: number) => (count <= 1 ? `${count} utilisateur` : `${count} utilisateurs`),
    share: (percent: string) => `${percent} des lectures`,
    none: 'Aucun sur cette période.',
    unknownApp: 'Appli inconnue',
    unknownDevice: 'Appareil inconnu',
    bar: (label: string, time: string) => `${label} : ${time}`,
    history: 'Lectures récentes',
    historyEmpty: 'Aucune lecture sur cette période.',
    columns: {
      started: 'Début',
      user: 'Utilisateur',
      title: 'Titre',
      device: 'Appli et appareil',
      played: 'Durée regardée',
      method: 'Mode de lecture',
    },
    appOn: (app: string, device: string) =>
      app && device ? `${app} sur ${device}` : app || device,
    showMore: 'Afficher plus',
    loadingMore: 'Chargement…',
  },
}

export default statistics
