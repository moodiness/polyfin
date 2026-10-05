import type en from '../en/sources'

const sources: typeof en = {
  addons: {
    title: 'Addons',
    description:
      'Les addons Stremio installés ici sont partagés avec tous les utilisateurs de ce serveur. Chacun peut les désactiver et ajouter ses propres addons depuis « Mes addons ».',
    installTitle: 'Installer un addon',
    manifestUrl: 'URL du manifeste',
    manifestUrlPlaceholder: 'https://…/manifest.json',
    manifestUrlHint:
      'Copiez le lien d’installation depuis la page de configuration de l’addon. Il contient souvent vos réglages ou vos clés : Polyfin le garde privé et n’en affiche jamais qu’une version abrégée.',
    install: 'Installer',
    installing: 'Installation…',
    installed: (name: string) => `${name} a été installé.`,
    listTitleShared: 'Addons du serveur',
    listTitleMine: 'Vos addons',
    listHelp:
      'Les addons sont utilisés dans cet ordre. En désactiver un masque ses bibliothèques dans les applications Jellyfin sans les perdre.',
    emptyShared: 'Aucun addon n’est encore installé sur ce serveur.',
    emptyMine: 'Vous n’avez encore installé aucun addon.',
    version: (version: string) => `Version ${version}`,
    off: 'Désactivé',
    enabled: 'Activé',
    provides: 'Fournit',
    types: 'Types',
    catalogs: 'Catalogues',
    catalogCount: (count: number) => (count <= 1 ? `${count} catalogue` : `${count} catalogues`),
    lastRefresh: 'Dernière actualisation',
    refresh: 'Actualiser',
    refreshing: 'Actualisation…',
    refreshLabel: (name: string) => `Actualiser ${name}`,
    refreshed: 'Le manifeste a été téléchargé à nouveau.',
    replace: 'Remplacer l’URL',
    replaceLabel: (name: string) => `Remplacer l’URL de ${name}`,
    newManifestUrl: 'Nouvelle URL du manifeste',
    replaceHint:
      'À utiliser après avoir reconfiguré l’addon sur sa page de configuration. Ses bibliothèques sont conservées.',
    replaceSubmit: 'Remplacer',
    replacing: 'Remplacement…',
    replaced: 'L’URL du manifeste a été remplacée.',
    remove: 'Supprimer',
    removing: 'Suppression…',
    removeConfirmShared: (name: string) =>
      `Supprimer ${name} ? Ses bibliothèques disparaîtront des applications Jellyfin de tous les utilisateurs, et cette action est irréversible.`,
    removeConfirmMine: (name: string) =>
      `Supprimer ${name} ? Ses bibliothèques disparaîtront de vos applications Jellyfin, et cette action est irréversible.`,
    removed: (name: string) => `${name} a été supprimé.`,
  },
  myAddons: {
    title: 'Mes addons',
    description:
      'Ajoutez vos propres addons Stremio et choisissez quels catalogues apparaissent comme bibliothèques dans vos applications Jellyfin.',
    preferenceTitle: 'Addons du serveur',
    useShared: 'Utiliser les addons du serveur',
    useSharedHelp:
      'Activé : vous profitez des addons et bibliothèques du serveur, suivis des vôtres. Désactivé : vous ne voyez que vos propres addons et bibliothèques.',
    preferenceSaved: 'Préférence enregistrée.',
    parentalControl:
      'Le contrôle parental s’applique à votre compte : vos applications Jellyfin n’affichent que les addons et bibliothèques du serveur, qui donnent les classifications sur lesquelles il s’appuie. Vos propres addons sont conservés mais pas utilisés.',
    personalAddonsOff:
      'Un administrateur a désactivé vos propres addons : vos applications Jellyfin n’affichent que les addons et bibliothèques du serveur. Vos propres addons sont conservés mais pas utilisés.',
  },
  music: {
    content: { music: 'Musique', audiobook: 'Livres audio', podcast: 'Podcasts' },
    library: {
      music: 'Bibliothèque musicale',
      audiobook: 'Bibliothèque de livres',
      podcast: 'Bibliothèque musicale',
    },
    libraryHelp: {
      music: 'Ses éléments apparaissent dans la musique des applis Jellyfin.',
      audiobook:
        'Ses livres audio apparaissent dans une bibliothèque de livres des applis Jellyfin.',
      podcast: 'Ses épisodes apparaissent dans la musique des applis Jellyfin.',
    },
    settings: 'Réglages',
    settingsLabel: (name: string) => `Réglages de ${name}`,
    settingsTitle: 'Réglages de l’addon',
    settingsHelp:
      'Proposés par l’addon et envoyés avec chaque requête que Polyfin lui fait. Ils peuvent contenir les infos de votre compte : seuls vous, ou les administrateurs pour les addons du serveur, pouvez les voir.',
    noSettings: 'Cet addon n’a aucun réglage.',
    defaultValue: (value: string) => `Par défaut : ${value}`,
    defaultEmpty: 'Par défaut : vide',
    on: 'Activé',
    off: 'Désactivé',
    range: (min: string, max: string) => `De ${min} à ${max}.`,
    atLeast: (min: string) => `Au moins ${min}.`,
    atMost: (max: string) => `Au plus ${max}.`,
    stepOf: (step: string) => `Par pas de ${step}.`,
    maxLength: (count: number) => `${count} caractères au plus.`,
    tooLong: (count: number) => `Utilisez ${count} caractères au plus.`,
    notNumber: 'Saisissez un nombre.',
    belowMin: (min: string) => `Saisissez ${min} ou plus.`,
    aboveMax: (max: string) => `Saisissez ${max} ou moins.`,
    offStep: (step: string) => `Utilisez un multiple de ${step}.`,
    resetDefaults: 'Remettre les valeurs par défaut',
    saved: 'Réglages enregistrés. L’addon les reçoit dès sa prochaine requête.',
  },
}

export default sources
