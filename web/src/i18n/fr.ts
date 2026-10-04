import type { Messages } from './en'

const fr: Messages = {
  documentTitle: 'Administration Polyfin',
  header: {
    productName: 'Polyfin',
    subtitle: 'Administration',
  },
  language: {
    label: 'Langue de l’interface',
    en: { short: 'EN', name: 'English' },
    fr: { short: 'FR', name: 'Français' },
  },
  nav: {
    label: 'Navigation principale',
    status: 'État',
    quickConnect: 'Quick Connect',
    account: 'Mon compte',
    users: 'Utilisateurs',
    myAddons: 'Mes addons',
    addons: 'Addons',
    libraries: 'Bibliothèques',
    settings: 'Paramètres',
    signedInAs: (name: string) => `Connecté en tant que ${name}`,
    signOut: 'Se déconnecter',
    signingOut: 'Déconnexion…',
  },
  common: {
    loading: 'Chargement…',
    save: 'Enregistrer',
    saving: 'Enregistrement…',
    cancel: 'Annuler',
    confirm: 'Confirmer',
    retry: 'Réessayer',
    never: 'Jamais',
    passwordRule: 'Au moins 8 caractères.',
    nameRule: "De 1 à 64 caractères : lettres, chiffres, espaces et - _ ' . @ +",
    passwordMismatch: 'Les deux mots de passe ne correspondent pas.',
    moveUp: (name: string) => `Monter ${name}`,
    moveDown: (name: string) => `Descendre ${name}`,
    moved: (name: string, position: number, count: number) =>
      `${name} est maintenant en position ${position} sur ${count}.`,
  },
  errors: {
    generic: 'Une erreur est survenue. Veuillez réessayer.',
    network: 'Impossible de joindre le serveur. Vérifiez que Polyfin est bien lancé.',
    unauthenticated: 'Votre session a expiré. Veuillez vous reconnecter.',
    forbidden: 'Vous n’êtes pas autorisé à effectuer cette action.',
    cross_origin: 'La requête a été refusée, car elle ne provient pas de cette page.',
    invalid_setup_code:
      'Ce code d’installation n’est pas valide. Copiez-le à nouveau depuis le journal du serveur.',
    invalid_name:
      "Le nom doit comporter de 1 à 64 caractères : lettres, chiffres, espaces et - _ ' . @ +",
    invalid_password: 'Le mot de passe doit comporter au moins 8 caractères.',
    setup_complete: 'L’installation est déjà terminée. Connectez-vous.',
    too_many_attempts: 'Trop de tentatives. Patientez quelques minutes avant de réessayer.',
    invalid_credentials: 'Nom ou mot de passe incorrect.',
    account_disabled: 'Ce compte est désactivé. Demandez à un administrateur de le réactiver.',
    wrong_password: 'Votre mot de passe actuel est incorrect.',
    not_found: 'Cet élément n’existe plus. La liste a été actualisée.',
    unknown_code: 'Aucun appareil n’attend avec ce code. Il a peut-être expiré ou été mal saisi.',
    quick_connect_disabled: 'Quick Connect est désactivé sur ce serveur.',
    name_taken: 'Ce nom est déjà utilisé par un autre utilisateur.',
    last_administrator:
      'C’est le dernier administrateur actif : il ne peut être ni supprimé, ni rétrogradé, ni désactivé.',
    invalid_server_name: 'Le nom du serveur doit comporter de 1 à 64 caractères.',
    invalid_language: 'Choisissez la langue du serveur dans la liste.',
    parental_control:
      'Le contrôle parental s’applique à ce compte : il garde les addons du serveur, qui donnent les classifications sur lesquelles il s’appuie.',
    invalid_parental_control:
      'Ce réglage du contrôle parental n’est pas pris en charge. Rechargez la page.',
    invalid_manifest_url:
      'Saisissez l’URL du manifeste d’un addon, commençant par https://, http:// ou stremio:// et se terminant par /manifest.json.',
    addon_exists: 'Cet addon est déjà installé ici.',
    addon_unreachable:
      'Impossible de joindre l’addon. Vérifiez l’URL et que l’addon est en ligne, puis réessayez.',
    invalid_manifest: 'Cette URL n’a pas renvoyé de manifeste d’addon Stremio.',
    private_network:
      'Cet addon se trouve à une adresse du réseau local. Seuls les administrateurs peuvent installer ce type d’addon.',
    invalid_order: 'La liste des addons a changé entre-temps. Elle a été actualisée : réessayez.',
    invalid_library:
      'Une bibliothèque fait référence à un catalogue qui n’existe plus ou ne peut pas être parcouru. Retirez les bibliothèques signalées comme plus disponibles, puis enregistrez à nouveau.',
    invalid_library_name: 'Le nom d’une bibliothèque doit comporter de 1 à 64 caractères.',
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
  setup: {
    title: 'Bienvenue dans Polyfin',
    description:
      'Aucun administrateur n’existe encore. Créez le premier compte pour gérer ce serveur et vous connecter depuis les applications Jellyfin.',
    codeHelp:
      'Par sécurité, Polyfin a inscrit un code d’installation à usage unique dans son journal au démarrage. Lancez docker logs <conteneur> (sur Unraid, ouvrez le journal du conteneur) et repérez la ligne contenant le code d’installation.',
    setupCode: 'Code d’installation',
    setupCodeHint: 'Format XXXX-XXXX. Majuscules et tiret facultatifs.',
    name: 'Nom de l’administrateur',
    password: 'Mot de passe',
    confirmPassword: 'Confirmer le mot de passe',
    submit: 'Créer l’administrateur',
    submitting: 'Création…',
  },
  login: {
    title: 'Connexion',
    description: 'Connectez-vous avec votre compte Polyfin pour continuer.',
    name: 'Nom',
    password: 'Mot de passe',
    submit: 'Se connecter',
    submitting: 'Connexion…',
  },
  quickConnect: {
    title: 'Quick Connect',
    description:
      'Connectez une application TV ou mobile sans saisir votre mot de passe : choisissez Quick Connect dans l’application, puis saisissez le code à 6 chiffres qu’elle affiche.',
    code: 'Code Quick Connect',
    codeHint: 'Les codes expirent au bout de 10 minutes.',
    lookingUp: 'Recherche de l’appareil…',
    requestTitle: 'Appareil en attente d’autorisation',
    device: 'Appareil',
    app: 'Application',
    requested: 'Demande',
    approve: 'Autoriser et connecter cet appareil',
    approving: 'Autorisation…',
    approved: (device: string, user: string) =>
      `${device} est maintenant connecté en tant que ${user}.`,
    another: 'Autoriser un autre appareil',
  },
  account: {
    title: 'Mon compte',
    description: 'Gérez votre mot de passe et les applications connectées avec votre compte.',
    passwordTitle: 'Changer de mot de passe',
    passwordHelp: 'Changer de mot de passe déconnecte tous vos appareils Jellyfin.',
    currentPassword: 'Mot de passe actuel',
    newPassword: 'Nouveau mot de passe',
    confirmPassword: 'Confirmer le nouveau mot de passe',
    passwordChanged: 'Votre mot de passe a été modifié.',
    devicesTitle: 'Mes appareils',
  },
  devices: {
    empty: 'Aucun appareil connecté.',
    lastActivity: 'Dernière activité',
    address: 'Adresse',
    signOut: 'Déconnecter',
    signOutConfirm: (device: string) =>
      `Déconnecter « ${device} » ? L’application devra se reconnecter.`,
    signingOut: 'Déconnexion…',
    signedOut: (device: string) => `« ${device} » a été déconnecté.`,
  },
  users: {
    title: 'Utilisateurs',
    description: 'Créez et gérez les comptes autorisés à utiliser ce serveur.',
    listTitle: 'Comptes',
    empty: 'Aucun utilisateur pour le moment.',
    administrator: 'Administrateur',
    hidden: 'Masqué sur l’écran de connexion',
    disabled: 'Désactivé',
    you: 'Vous',
    lastSignIn: 'Dernière connexion',
    createTitle: 'Créer un utilisateur',
    name: 'Nom',
    password: 'Mot de passe',
    isAdministrator: 'Administrateur',
    isAdministratorHelp:
      'Les administrateurs peuvent gérer les utilisateurs et les paramètres du serveur.',
    showOnSignIn: 'Afficher sur l’écran de connexion',
    showOnSignInHelp:
      'Les utilisateurs visibles apparaissent sur l’écran de connexion des applications Jellyfin. Les autres saisissent leur nom, comme par défaut dans Jellyfin.',
    create: 'Créer l’utilisateur',
    creating: 'Création…',
    created: (name: string) => `${name} a été créé.`,
    edit: 'Modifier',
    close: 'Fermer',
    editTitle: (name: string) => `Modifier ${name}`,
    rename: 'Renommer',
    renamed: 'Le nom a été modifié.',
    resetPassword: 'Réinitialiser le mot de passe',
    newPassword: 'Nouveau mot de passe',
    resetPasswordHelp:
      'Changer le mot de passe déconnecte tous les appareils Jellyfin de cet utilisateur.',
    passwordReset: 'Le mot de passe a été réinitialisé.',
    accessTitle: 'Accès',
    isDisabled: 'Désactivé',
    isDisabledHelp:
      'Un utilisateur désactivé ne peut plus se connecter, et tous ses appareils sont déconnectés.',
    canTranscode: 'Peut utiliser la conversion (transcodage)',
    canTranscodeHelp:
      'Le serveur peut convertir l’image et le son quand l’application de cet utilisateur ne sait pas lire un fichier tel quel. Sinon, l’image et le son ne sont jamais modifiés.',
    canDownload: 'Peut télécharger',
    canDownloadHelp:
      'Permet à cet utilisateur d’enregistrer des titres dans les applications Jellyfin pour les regarder hors connexion.',
    noTranscoding: 'Sans conversion',
    noDownloads: 'Sans téléchargement',
    lastAdminHelp:
      'Le dernier administrateur actif ne peut être ni supprimé, ni rétrogradé, ni désactivé.',
    updated: 'Modifications enregistrées.',
    parentalTitle: 'Contrôle parental',
    maxRating: 'Classification maximale',
    maxRatingHelp: 'Les titres classés au-delà de cette limite sont masqués pour cet utilisateur.',
    noLimit: 'Aucune limite',
    blockUnratedMovies: 'Bloquer les films non classés',
    blockUnratedShows: 'Bloquer les séries non classées',
    ratingLimit: (rating: string) => `Jusqu’à ${rating}`,
    saveParental: 'Enregistrer le contrôle parental',
    devicesTitle: 'Appareils',
    delete: 'Supprimer l’utilisateur',
    deleteConfirm: (name: string) =>
      `Supprimer ${name} ? Ses appareils seront déconnectés et cette action est irréversible.`,
    deleting: 'Suppression…',
    deleted: (name: string) => `${name} a été supprimé.`,
  },
  settings: {
    title: 'Paramètres',
    description: 'Options qui s’appliquent à l’ensemble du serveur.',
    serverName: 'Nom du serveur',
    serverNameHelp: 'De 1 à 64 caractères, affiché dans les applications Jellyfin.',
    quickConnect: 'Autoriser Quick Connect',
    quickConnectHelp:
      'Permet aux applications TV et mobiles de se connecter avec un code à 6 chiffres autorisé ici.',
    language: 'Langue des noms générés',
    languageHelp:
      'Polyfin nomme lui-même certains éléments dans les applications Jellyfin : les saisons (« Saison 1 », « Épisodes spéciaux »), les épisodes sans titre et le type ajouté aux bibliothèques de même nom (« Populaires (Films) »).',
    legacyAuthorization: 'Autoriser l’authentification héritée',
    legacyAuthorizationHelp:
      'Accepte les anciens en-têtes X-Emby-*, le paramètre api_key et le schéma Emby pour les applications qui en ont encore besoin. Désactivé par défaut, comme dans Jellyfin 12.1.',
    legacyWarningTitle: 'Avertissement de sécurité',
    legacyWarning:
      'Les méthodes héritées peuvent transmettre les identifiants dans les URL, qui se retrouvent alors dans les journaux, l’historique du navigateur et les proxys. N’activez cette option que si une de vos applications ne parvient pas à se connecter autrement.',
    playbackTitle: 'Lecture',
    chapters: 'Afficher les chapitres',
    chaptersHelp:
      'Polyfin lit les chapitres en même temps qu’il analyse le fichier, ce qu’il fait de toute façon avant une première lecture : ils ne retardent donc jamais la lecture. Désactivés, ils sont simplement masqués dans les applications.',
    prepareAhead: 'Préparer la lecture à l’avance',
    prepareAheadHelp:
      'Polyfin lit le fichier dès que la page d’un titre s’ouvre, et prépare l’épisode suivant vers la fin de celui en cours, pour que la lecture démarre tout de suite. En contrepartie, vos sources reçoivent un peu plus de demandes, y compris pour les titres ouverts mais pas regardés.',
    transcoding: 'Conversion (transcodage)',
    transcodingHelp:
      'Convertit l’image et le son pour les applications qui ne savent pas lire un fichier tel quel. Si cette option est désactivée, les fichiers sont lus tels quels ou simplement présentés autrement, sans toucher à l’image ni au son. Un titre qu’une application ne peut pas lire ainsi ne démarrera pas sur cette application.',
    downloads: 'Téléchargements',
    downloadsHelp:
      'Permet aux utilisateurs d’enregistrer des titres dans les applications Jellyfin pour les regarder hors connexion. Si cette option est désactivée, personne ne peut télécharger, quelle que soit son autorisation personnelle.',
    saved: 'Paramètres enregistrés.',
  },
  stremioTypes: {
    movie: 'Films',
    series: 'Séries',
    channel: 'Chaînes',
    tv: 'TV',
  },
  stremioResources: {
    catalog: 'Catalogues',
    meta: 'Fiches',
    stream: 'Flux',
    subtitles: 'Sous-titres',
    addon_catalog: 'Catalogues d’addons',
  },
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
  },
  notFound: {
    title: 'Page introuvable',
    description: 'Cette page n’existe pas.',
    home: 'Retour à l’état du serveur',
  },
  forbidden: {
    title: 'Accès refusé',
    description: 'Seuls les administrateurs peuvent ouvrir cette page.',
    home: 'Retour à l’état du serveur',
  },
  time: {
    justNow: 'à l’instant',
  },
  footer: {
    sourceCode: 'Code source sur GitHub',
  },
}

export default fr
