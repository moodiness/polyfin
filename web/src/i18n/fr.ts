import type { Messages } from './en'

const fr: Messages = {
  documentTitle: 'État du serveur · Polyfin',
  header: {
    productName: 'Polyfin',
    subtitle: 'Administration',
  },
  language: {
    label: 'Langue de l’interface',
    en: { short: 'EN', name: 'English' },
    fr: { short: 'FR', name: 'Français' },
  },
  status: {
    title: 'État du serveur',
    description: 'L’essentiel sur le fonctionnement de ce serveur Polyfin.',
    version: 'Version',
    serverId: 'Identifiant du serveur',
    database: 'Base de données',
    databaseReady: 'Prête',
    databaseUnavailable: 'Indisponible',
    loading: 'Chargement de l’état du serveur…',
    errorTitle: 'Serveur injoignable',
    errorBody:
      'L’API d’administration ne répond pas. Vérifiez que Polyfin est bien lancé, puis réessayez.',
    staleWarning:
      'La dernière actualisation a échoué. Les informations ci-dessous ne sont peut-être plus à jour.',
    retry: 'Réessayer',
    retrying: 'Nouvelle tentative…',
    autoRefresh: 'Actualisation automatique toutes les 10 secondes.',
    updatedAt: (time: string) => `Dernière mise à jour à ${time}`,
  },
  footer: {
    sourceCode: 'Code source sur GitHub',
  },
}

export default fr
