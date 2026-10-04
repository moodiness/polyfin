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
    apiKeys: 'Clés d’API',
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
    invalid_catalog_limit:
      'Le nombre de titres lus par catalogue doit être un nombre entier de 100 à 20 000.',
    invalid_channel_limit:
      'Le nombre de chaînes lues par catalogue de TV en direct doit être un nombre entier de 100 à 50 000.',
    invalid_played_percent:
      'Le pourcentage « Marqué comme vu après » doit être un nombre entier de 50 à 100.',
    invalid_resume_percent:
      'Le pourcentage « Point de reprise gardé après » doit être un nombre entier de 0 à 50.',
    resume_not_below_played:
      'Le pourcentage « Point de reprise gardé après » doit être plus petit que « Marqué comme vu après ».',
    invalid_version_list_minutes:
      'La durée de conservation des listes de versions doit être un nombre entier de minutes de 1 à 360.',
    invalid_catalog_refresh_minutes:
      'Le délai de rafraîchissement des catalogues doit être un nombre entier de minutes de 1 à 1 440.',
    invalid_analysis_timeout:
      'Le temps pour analyser une version doit être un nombre entier de secondes de 5 à 120.',
    invalid_version_attempts:
      'Le nombre de versions essayées doit être un nombre entier de 1 à 10.',
    invalid_max_conversions:
      'Le nombre de conversions vidéo en même temps doit être un nombre entier de 0 à 32.',
    invalid_max_conversion_height: 'Choisissez la qualité max des vidéos converties dans la liste.',
    parental_control:
      'Le contrôle parental s’applique à ce compte : il garde les addons du serveur, qui donnent les classifications sur lesquelles il s’appuie.',
    invalid_parental_control:
      'Ce réglage du contrôle parental n’est pas pris en charge. Rechargez la page.',
    invalid_max_playbacks:
      'Le nombre de lectures en même temps doit être un nombre entier de 0 à 20.',
    invalid_max_bitrate: 'Choisissez la qualité maximale dans la liste.',
    invalid_sync_play: 'Choisissez une option de « Regarder ensemble » dans la liste.',
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
    invalid_login_attempts:
      'Le nombre de mots de passe faux avant blocage doit être 0, ou un nombre entier de 3 à 20.',
    invalid_inactive_device_days:
      'Le nombre de jours avant de déconnecter les appareils inutilisés doit être un nombre entier de 0 à 365.',
    personal_addons_disabled:
      'Un administrateur a désactivé vos propres addons. Ils sont conservés, mais vous ne pouvez pas en ajouter, ni les actualiser, ni les activer.',
    invalid_hidden_libraries: 'La liste des bibliothèques a changé entre-temps. Rechargez la page.',
    invalid_blocked_genres:
      'Un genre doit comporter de 1 à 100 caractères, et 100 genres au plus peuvent être bloqués.',
    invalid_access_schedules:
      'Les horaires vont de 00:00 à 24:00, et chaque fin doit venir après son début (50 lignes au plus).',
    outside_allowed_hours:
      'Ce compte ne peut pas être utilisé à cette heure-ci. Réessayez pendant ses horaires autorisés.',
    invalid_guide_url:
      'Saisissez une adresse de guide commençant par https:// ou http://, de 4 096 caractères au plus.',
    invalid_app:
      'Le nom de l’application doit comporter de 1 à 64 caractères, sans caractères spéciaux.',
    invalid_trickplay_interval:
      'Le temps entre deux miniatures doit être un nombre entier de secondes de 5 à 60.',
    invalid_trickplay_width: 'Choisissez la largeur des miniatures dans la liste.',
    invalid_thumbnail_storage_gb:
      'La place pour les images doit être un nombre entier de Go de 1 à 50.',
    invalid_recording_padding:
      'Les minutes avant et après un enregistrement doivent être un nombre entier de 0 à 60.',
    invalid_recording_retention_days:
      'Les jours de conservation des enregistrements doivent être un nombre entier de 0 à 3 650.',
    invalid_quality_group: 'Choisissez le groupe de qualité dans la liste.',
    invalid_live_tv_refresh_hours:
      'L’intervalle d’actualisation doit être un nombre entier d’heures de 1 à 168.',
    invalid_source_name: 'Le nom d’une source doit faire de 1 à 64 caractères.',
    invalid_source_address:
      'Saisissez une adresse commençant par https:// ou http://, et pour un compte Xtream Codes un identifiant et un mot de passe.',
    invalid_channel_list:
      'Cette adresse n’a pas renvoyé de liste de chaînes. Vérifiez-la, et que le compte est toujours valable.',
    channel_list_too_large:
      'Cette liste de chaînes est trop grande (100 Mo ou 100 000 chaînes au plus).',
    iptv_login_refused: 'Le serveur IPTV a refusé cet identifiant ou ce mot de passe.',
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
    canAddAddons: 'Peut ajouter ses propres addons',
    canAddAddonsHelp:
      'Cet utilisateur peut ajouter ses propres addons Stremio depuis sa page Mes addons. Sinon, ses addons sont conservés mais pas utilisés.',
    noPersonalAddons: 'Sans addons personnels',
    blockedUntil: (time: string) => `Bloqué jusqu’à ${time}`,
    blockedHelp:
      'Trop de mots de passe faux ont été saisis pour ce compte : il ne peut pas se connecter avant cette heure, même avec le bon mot de passe.',
    unblock: 'Débloquer',
    unblocking: 'Déblocage…',
    unblocked: 'Le compte est débloqué.',
    playbackAccessTitle: 'Lecture et accès',
    maxPlaybacks: 'Lectures en même temps (0 = pas de limite)',
    maxPlaybacksHelp:
      'Le nombre d’appareils de cet utilisateur qui peuvent lire en même temps. Il peut toujours se connecter sur d’autres appareils.',
    maxBitrate: 'Qualité maximale',
    maxBitrateHelp:
      'Au-delà, la vidéo est convertie si cet utilisateur peut utiliser la conversion ; sinon, une version plus légère est lue.',
    bitrateNoLimit: 'Pas de limite',
    bitrate4k: '40 Mbit/s (4K)',
    bitrate1080High: '20 Mbit/s (1080p, haute qualité)',
    bitrate1080: '10 Mbit/s (1080p)',
    bitrate720: '4 Mbit/s (720p)',
    bitrate480: '2 Mbit/s (480p)',
    bitrateOther: (mbits: string) => `${mbits} Mbit/s`,
    liveTv: 'TV en direct',
    liveTvHelp: 'Montre les chaînes de TV en direct à cet utilisateur.',
    syncPlay: 'Regarder ensemble',
    syncPlayHelp:
      'Lire le même titre en même temps que d’autres, dans les applications Jellyfin qui le proposent.',
    syncPlayCreateAndJoin: 'Créer et rejoindre des groupes',
    syncPlayJoin: 'Rejoindre seulement',
    syncPlayNone: 'Non',
    remoteControl: 'Peut contrôler les applis des autres utilisateurs',
    remoteControlHelp:
      'Permet à cet utilisateur de lancer, mettre en pause ou envoyer des messages aux applications Jellyfin des autres utilisateurs. Chacun peut contrôler ses propres applications.',
    liveTvManagement: 'Peut enregistrer la TV en direct',
    liveTvManagementHelp: 'Programmer et supprimer des enregistrements.',
    savePlaybackAccess: 'Enregistrer la lecture et l’accès',
    visibleLibrariesTitle: 'Bibliothèques visibles',
    visibleLibrariesHelp:
      'Les bibliothèques du serveur que les applications de cet utilisateur affichent. Les bibliothèques ajoutées plus tard sont affichées. Ce choix ne bloque aucun titre : on peut toujours les trouver dans d’autres bibliothèques, par la recherche ou par un lien. Pour bloquer des titres, utilisez le contrôle parental ou les genres bloqués.',
    noServerLibraries: 'Le serveur n’a pas encore de bibliothèque.',
    saveVisibleLibraries: 'Enregistrer les bibliothèques visibles',
    blockedGenresTitle: 'Genres bloqués',
    blockedGenresHelp:
      'Les titres de l’un de ces genres sont masqués partout pour cet utilisateur, comme ceux qui dépassent la limite du contrôle parental.',
    genre: 'Genre',
    genreHint: 'Choisissez un genre des bibliothèques du serveur, ou tapez-en un.',
    addGenre: 'Ajouter',
    removeGenre: (genre: string) => `Retirer ${genre}`,
    noBlockedGenres: 'Aucun genre n’est bloqué.',
    saveBlockedGenres: 'Enregistrer les genres bloqués',
    allowedHoursTitle: 'Horaires autorisés',
    allowedHoursHelp:
      'Si des horaires sont définis, cet utilisateur ne peut se connecter et utiliser le serveur que pendant ces horaires, à l’heure du serveur. En dehors, ses applications sont refusées.',
    noAllowedHours: 'Aucun horaire : cet utilisateur peut utiliser le serveur à toute heure.',
    day: 'Jour',
    from: 'De',
    to: 'À',
    addHours: 'Ajouter un horaire',
    removeHours: 'Retirer',
    hoursOrder: 'Chaque heure de fin doit venir après son heure de début.',
    saveAllowedHours: 'Enregistrer les horaires autorisés',
    days: {
      Sunday: 'Dimanche',
      Monday: 'Lundi',
      Tuesday: 'Mardi',
      Wednesday: 'Mercredi',
      Thursday: 'Jeudi',
      Friday: 'Vendredi',
      Saturday: 'Samedi',
      Everyday: 'Tous les jours',
      Weekday: 'En semaine (du lundi au vendredi)',
      Weekend: 'Le week-end',
    },
    canManageCollections: 'Peut gérer les collections',
    canManageCollectionsHelp:
      'Permet à cet utilisateur de créer des collections de titres dans les applications Jellyfin, d’y ajouter ou d’en retirer des titres, et de les supprimer. Tous les utilisateurs voient les collections, chacun avec seulement les titres qu’il a le droit de voir.',
    resetPinRequested: 'Réinitialisation du mot de passe demandée : code',
    resetPinValidUntil: (time: string) => `, valable jusqu’à ${time}`,
    resetPinHelp:
      'Donnez ce code à l’utilisateur : il se connecte avec, et ce code devient son nouveau mot de passe.',
    canManageSubtitles: 'Peut gérer les sous-titres',
    canManageSubtitlesHelp:
      'Permet à cet utilisateur d’ajouter des fichiers de sous-titres aux titres depuis ses applications Jellyfin, et d’y chercher les sous-titres des addons.',
    qualityGroup: 'Groupe de qualité',
    qualityGroupHelp:
      'La meilleure définition proposée à cet utilisateur. Les versions de définition supérieure sont écartées tant qu’une autre convient ; si aucune ne convient, elles sont converties, ou refusées si cet utilisateur ne peut pas utiliser la conversion. La TV en direct est aussi convertie, et seules les versions qui conviennent peuvent être téléchargées.',
    qualityGroupOriginal: 'D’origine (sans limite)',
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
    analysisTimeout: 'Temps max pour analyser une version',
    analysisTimeoutHelp:
      'Avant une première lecture, Polyfin analyse le fichier ou la chaîne pour savoir comment le lire. Si la source ne répond pas dans ce délai, en secondes, Polyfin abandonne cette version et passe à la suivante. Un nombre plus petit passe plus vite à la suivante, mais peut abandonner des sources lentes qui auraient marché. De 5 à 120 secondes ; 45 par défaut.',
    versionAttempts: 'Versions essayées quand une ne marche pas',
    versionAttemptsHelp:
      'Quand une application lit un titre sans choisir de version, Polyfin essaie les versions dans l’ordre jusqu’à en trouver une qui marche, sans en analyser plus que ce nombre. Les versions déjà analysées sont aussi essayées, car elles ne coûtent rien. Un nombre plus grand trouve plus souvent une version qui marche, mais un titre qui ne se lit pas met plus longtemps à le dire. De 1 à 10 ; 3 par défaut.',
    preferDirectPlay: 'Préférer les versions lues sans conversion',
    preferDirectPlayHelp:
      'Quand une application lit un titre sans choisir de version, Polyfin prend la première version que l’application lit telle quelle ou simplement présentée autrement, plutôt que la première qui se lit tout court. La toute première lecture d’un titre peut être un peu plus lente, car plus de versions peuvent être analysées ; rien ne change une fois qu’elles sont connues.',
    maxConversions: 'Conversions vidéo en même temps (0 = pas de limite)',
    maxConversionsHelp:
      'Convertir l’image est le travail le plus lourd du serveur. Quand ce nombre de lectures avec image convertie est atteint (sous-titres incrustés dans l’image compris), une nouvelle lecture prend une version qui n’a pas besoin de conversion, ou ne démarre pas. Les lectures en cours ne sont jamais coupées. La TV en direct garde aussi ses propres limites : 4 chaînes par utilisateur et 16 pour le serveur. De 0 à 32 ; 0 par défaut.',
    maxConversionHeight: 'Qualité max des vidéos converties',
    maxConversionHeightHelp:
      'Les vidéos converties sont réduites à cette hauteur au plus, sans être déformées, pour passer mieux sur une connexion lente. Les fichiers lus tels quels ou simplement présentés autrement gardent leur qualité. Polyfin ne convertit jamais au-delà de 1080p : les choix plus élevés ne changent rien pour l’instant.',
    conversionHeightOriginal: 'Originale',
    conversionHeight: (height: number) => `${height}p`,
    catalogsTitle: 'Catalogues',
    catalogLimit: 'Titres lus par catalogue de films et séries',
    catalogLimitHelp:
      'Certains catalogues sont presque sans fin : Polyfin arrête donc de lire un catalogue après ce nombre de titres. Un nombre plus élevé affiche plus de titres, mais les listes se chargent plus lentement et l’addon reçoit plus de demandes. De 100 à 20 000 ; 2 000 par défaut.',
    channelLimit: 'Chaînes lues par catalogue de TV en direct',
    channelLimitHelp:
      'Polyfin arrête de lire un catalogue de TV en direct après ce nombre de chaînes, et lit au plus ce nombre de programmes par jour pour le guide. Un nombre plus élevé affiche plus de contenu, mais se charge plus lentement et l’addon reçoit plus de demandes. De 100 à 50 000 ; 10 000 par défaut.',
    contentTitle: 'Contenu',
    skipButtons: 'Boutons « Passer l’intro » et « Passer le générique »',
    skipButtonsHelp:
      'Les applications proposent de passer les intros, les résumés et les génériques, repérés dans des bases de données communautaires. Si cette option est désactivée, les applications n’affichent pas ces boutons et ces bases ne sont pas consultées.',
    similarTitles: 'Titres similaires',
    similarTitlesHelp:
      'La page d’un titre montre des titres proches, trouvés dans les catalogues des addons. Si cette option est désactivée, la liste est vide et les addons reçoivent moins de demandes.',
    playedPercent: 'Marqué comme vu après (%)',
    playedPercentHelp:
      'Un titre est marqué comme vu dès que la lecture dépasse cette part de sa durée. De 50 à 100 ; 90 par défaut, comme dans Jellyfin.',
    resumePercent: 'Point de reprise gardé après (%)',
    resumePercentHelp:
      'L’endroit où la lecture s’est arrêtée est gardé, pour reprendre de là, dès qu’il dépasse cette part de la durée du titre. Ce nombre doit être plus petit que celui de « Marqué comme vu après ». De 0 à 50 ; 5 par défaut, comme dans Jellyfin. Un titre de moins de 5 minutes est marqué comme vu dès qu’il dépasse ce point.',
    versionListMinutes: 'Garder les listes de versions pendant (minutes)',
    versionListMinutesHelp:
      'Combien de temps Polyfin garde les versions et les sous-titres que les addons donnent pour un titre. Les garder plus longtemps envoie moins de demandes à l’addon qui donne les versions, ce qui aide avec les fournisseurs qui refusent trop de demandes, mais les nouvelles versions apparaissent plus tard. De 1 à 360 ; 10 par défaut.',
    catalogRefreshMinutes: 'Rafraîchir les catalogues toutes les (minutes)',
    catalogRefreshMinutesHelp:
      'Combien de temps Polyfin garde les pages de catalogue lues auprès des addons, guide de la TV en direct compris, avant de les relire. Une durée plus longue envoie moins de demandes aux addons, mais les nouveaux titres apparaissent plus tard. De 1 à 1 440 (un jour) ; 10 par défaut.',
    securityTitle: 'Sécurité',
    personalAddons: 'Autoriser les addons personnels des utilisateurs',
    personalAddonsHelp:
      'Les utilisateurs peuvent ajouter leurs propres addons Stremio, en plus de ceux du serveur. Sinon, leurs addons sont conservés mais pas utilisés, et leurs applications Jellyfin n’affichent que les addons du serveur.',
    loginAttempts: 'Bloquer un compte après ce nombre de mots de passe faux (0 = jamais)',
    loginAttemptsHelp:
      'Après ce nombre de mots de passe faux à la suite, le compte ne peut plus se connecter pendant 15 minutes, même avec le bon mot de passe. Un administrateur peut le débloquer plus tôt depuis la page Utilisateurs. 0, ou de 3 à 20.',
    inactiveDeviceDays: 'Déconnecter les appareils inutilisés depuis (jours, 0 = jamais)',
    inactiveDeviceDaysHelp:
      'Les applications Jellyfin qui n’ont pas servi depuis ce nombre de jours sont déconnectées et doivent se reconnecter. La vérification a lieu toutes les heures. La connexion à cette page d’administration n’est pas concernée. De 0 à 365.',
    detailedLog: 'Journal détaillé (pour diagnostiquer un problème)',
    detailedLogHelp:
      'Polyfin écrit beaucoup plus de détails dans son journal, tout de suite et sans redémarrage. Désactivez cette option une fois le problème trouvé.',
    saved: 'Paramètres enregistrés.',
    thumbnailsTitle: 'Miniatures',
    thumbnailsHelp:
      'Des images tirées des titres eux-mêmes, après leur visionnage. Pour les faire, Polyfin lit de petits morceaux du fichier à la source, doucement, quand personne ne regarde depuis cette source : au plus 60 demandes par titre, une toutes les 3 secondes, et 120 par heure et par source, jamais le fichier entier. Une source qui demande de ralentir ne reçoit plus de demande d’images pendant 2 heures. Désactivé par défaut.',
    trickplay: 'Miniatures quand on avance dans un titre',
    trickplayHelp:
      'Les applications montrent une petite image du moment visé dans la barre de lecture. Polyfin les fait une fois le titre regardé, et pour l’épisode suivant quand la lecture est préparée à l’avance. Les films longs ont des miniatures plus espacées : quelques minutes plutôt que quelques secondes.',
    trickplayInterval: 'Une miniature toutes les (secondes)',
    trickplayIntervalHelp:
      'Tous les combien la barre de lecture change d’image. Polyfin lit au plus 60 images par titre : sur les titres longs, la même image couvre plusieurs pas. De 5 à 60 ; 10 par défaut, comme Jellyfin.',
    trickplayWidth: 'Largeur des miniatures',
    trickplayWidthHelp:
      'Des miniatures plus larges sont plus nettes sur un grand écran, mais prennent plus de place. Un changement fait de nouvelles miniatures à la prochaine lecture d’un titre.',
    pixels: (width: number) => `${width} pixels`,
    chapterImages: 'Images des chapitres',
    chapterImagesHelp:
      'Les applications montrent une image pour chaque chapitre d’un titre, dans sa liste de scènes. Polyfin prend l’image lue la plus proche du début de chaque chapitre, avec les mêmes lectures que les miniatures, une fois le titre regardé.',
    thumbnailStorage: 'Place pour les images (Go)',
    thumbnailStorageHelp:
      'Les miniatures et les images des chapitres sont gardées dans la base de données. Au-delà de cette taille, les images des titres regardés il y a le plus longtemps sont effacées ; elles sont refaites si le titre est relu. De 1 à 50 ; 2 par défaut.',
    recordingsTitle: 'Enregistrements',
    recordingsFolder: (folder: string) => `Les enregistrements sont gardés dans ${folder}.`,
    recordingsOff:
      'L’enregistrement est désactivé. Indiquez un dossier dans POLYFIN_RECORDINGS_DIR pour l’activer.',
    recordingPrePadding: 'Commencer les enregistrements avant (minutes)',
    recordingPrePaddingHelp:
      'Combien de minutes avant l’émission un enregistrement commence. De 0 à 60 ; 0 par défaut.',
    recordingPostPadding: 'Continuer les enregistrements après (minutes)',
    recordingPostPaddingHelp:
      'Combien de minutes après l’émission un enregistrement continue. De 0 à 60 ; 0 par défaut.',
    recordingRetentionDays: 'Garder les enregistrements (jours, 0 = toujours)',
    recordingRetentionDaysHelp:
      'Les enregistrements plus anciens sont supprimés. La vérification a lieu chaque jour. De 0 à 3 650.',
    liveTvTitle: 'TV en direct',
    liveTvRefreshHours: 'Actualiser les listes et les guides de TV toutes les (heures)',
    liveTvRefreshHoursHelp:
      'À quelle fréquence les listes de chaînes IPTV et les guides des programmes XMLTV sont téléchargés à nouveau. De 1 à 168 ; 12 par défaut.',
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
    guideTitle: 'Guide des programmes (XMLTV)',
    guideHelp:
      'Certains fournisseurs publient leur guide des programmes dans un fichier XMLTV. Ses programmes complètent les chaînes pour lesquelles l’addon ne donne pas de guide. Le guide est téléchargé à nouveau selon l’intervalle choisi dans Paramètres › TV en direct.',
    guideAfterSave:
      'Enregistrez les bibliothèques pour ajouter un guide des programmes à ce catalogue.',
    guideNone: 'Pas de guide des programmes.',
    guideAddress: 'Adresse du guide',
    guideAddressHint: 'Fichier XMLTV, compressé (.gz) ou non, jusqu’à 300 Mo.',
    guidePlaceholder: 'https://…/guide.xml.gz',
    guideAdd: 'Ajouter un guide',
    guideChange: 'Changer d’adresse',
    guideRemove: 'Retirer le guide',
    guideSave: 'Enregistrer et récupérer',
    guideFetching: 'Récupération du guide…',
    guideRefresh: 'Actualiser le guide',
    guideRefreshLabel: (name: string) => `Actualiser le guide des programmes de ${name}`,
    guideFetched: 'Dernière récupération',
    guideNever: 'Pas encore récupéré.',
    guideMatched: (matched: number, channels: number) =>
      `${matched} ${matched <= 1 ? 'chaîne' : 'chaînes'} sur ${channels} ${matched <= 1 ? 'a' : 'ont'} un guide.`,
    guideRemoved: 'Guide des programmes retiré.',
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
    guideNext: 'Prochaine récupération',
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
  apiKeys: {
    title: 'Clés d’API',
    description:
      'Les clés permettent à des outils et applications, comme des gestionnaires de demandes ou des scripts, d’utiliser l’API Jellyfin de ce serveur avec les droits d’administrateur. Toute personne qui a une clé a ces droits : gardez vos clés secrètes et révoquez celles que vous n’utilisez plus.',
    listTitle: 'Clés',
    empty: 'Aucune clé d’API pour l’instant.',
    created: 'Créée',
    lastUsed: 'Dernière utilisation',
    createTitle: 'Créer une clé',
    app: 'Nom de l’application',
    appHint: 'Qui va utiliser cette clé, pour la reconnaître plus tard. De 1 à 64 caractères.',
    create: 'Créer la clé',
    creating: 'Création…',
    newKeyTitle: (app: string) => `Clé pour ${app}`,
    newKeyNotice: 'Copiez cette clé maintenant : elle ne sera plus affichée.',
    copy: 'Copier',
    copied: 'Copiée.',
    copyFailed: 'Impossible de copier la clé. Sélectionnez-la et copiez-la vous-même.',
    done: 'Je l’ai copiée',
    revoke: 'Révoquer',
    revoking: 'Révocation…',
    revokeConfirm: (app: string) =>
      `Révoquer la clé pour ${app} ? Les outils et applications qui l’utilisent ne fonctionneront plus. Impossible de revenir en arrière.`,
    revoked: (app: string) => `La clé pour ${app} a été révoquée.`,
  },
  activity: {
    title: 'Activité récente',
    empty: 'Aucune activité pour l’instant.',
    warning: 'Avertissement',
    error: 'Erreur',
    count: (shown: number, total: number) => `Les ${shown} derniers événements sur ${total}.`,
    autoRefresh: 'Actualisation automatique toutes les 30 secondes.',
  },
  iptv: {
    addTitle: 'Ajouter une source IPTV',
    addHelp:
      'Importez directement les chaînes d’une playlist M3U ou d’un compte Xtream Codes. Elles apparaissent dans la TV en direct comme un catalogue TV, que vous pouvez ranger dans Bibliothèques. La liste est téléchargée à nouveau selon l’intervalle choisi dans Paramètres › TV en direct.',
    name: 'Nom',
    kind: 'Type',
    kindM3u: 'Playlist M3U',
    kindXtream: 'Compte Xtream Codes',
    playlistUrl: 'Adresse de la playlist',
    playlistUrlHint:
      'L’adresse M3U donnée par votre fournisseur. Elle peut contenir vos identifiants : elle n’est jamais affichée en entier.',
    server: 'Adresse du serveur',
    serverHint: 'Par exemple https://serveur:8080, telle que votre fournisseur la donne.',
    username: 'Identifiant',
    password: 'Mot de passe',
    keepHint: 'Laissez vide pour garder l’actuelle.',
    passwordKeepHint: 'Laissez vide pour garder le mot de passe actuel.',
    providerGuide: 'Utiliser le guide des programmes du fournisseur',
    providerGuideHelp:
      'Les serveurs Xtream Codes publient un guide pour le compte : il remplit les programmes des chaînes.',
    guideUrl: 'Adresse du guide des programmes (XMLTV, facultatif)',
    guideUrlHint:
      'Fichier XMLTV, compressé ou non. Vous pouvez aussi l’ajouter ou le changer plus tard dans Bibliothèques.',
    add: 'Ajouter la source',
    adding: 'Téléchargement de la liste…',
    added: (name: string, channels: number) =>
      `${name} a été ajoutée avec ${channels <= 1 ? `${channels} chaîne` : `${channels} chaînes`}.`,
    address: 'Adresse',
    channels: 'Chaînes',
    channelCount: (count: number) => (count <= 1 ? `${count} chaîne` : `${count} chaînes`),
    groupsShown: (shown: number, total: number) => `${shown} groupes affichés sur ${total}`,
    lastFetch: 'Dernier téléchargement',
    nextFetch: 'Prochain téléchargement',
    never: 'Jamais',
    edit: 'Modifier',
    editLabel: (name: string) => `Modifier ${name}`,
    saved: 'Source enregistrée.',
    groups: 'Groupes affichés',
    allGroups: 'Tous les groupes',
    allGroupsHelp: 'Les groupes ajoutés plus tard à la liste sont aussi affichés.',
    filterGroups: 'Filtrer les groupes',
    checkShown: 'Cocher les groupes listés',
    uncheckShown: 'Décocher les groupes listés',
    noGroup: 'Sans groupe',
    errors: {
      unreachable:
        'Le dernier téléchargement a échoué : impossible de joindre le serveur. Les chaînes précédentes sont gardées.',
      private_network:
        'Le dernier téléchargement a échoué : cette adresse est sur un réseau local. Seuls les administrateurs peuvent utiliser ce type d’adresse.',
      too_large:
        'Le dernier téléchargement a échoué : la liste est trop grande. Les chaînes précédentes sont gardées.',
      malformed:
        'Le dernier téléchargement a échoué : le serveur n’a pas renvoyé de liste de chaînes, ou a refusé les identifiants. Les chaînes précédentes sont gardées.',
    },
  },
}

export default fr
