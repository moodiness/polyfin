import type en from '../en/sources'

const sources: typeof en = {
  sources: {
    title: 'Sources',
    lede: 'Addons Stremio, addons Eclipse, sources IPTV et dossiers locaux, réunis dans une seule liste. Les sources du serveur servent à tous ; celles d’un membre ne servent qu’à lui.',
    add: 'Ajouter une source',
    listLabel: 'Sources',
    listNote: 'Les sources personnelles des membres ne servent qu’à leur propriétaire.',
    loading: 'Chargement des sources',
    kindFilter: 'Filtrer par type',
    all: 'Toutes',
    kinds: { stremio: 'Stremio', eclipse: 'Eclipse', iptv: 'IPTV', local: 'Dossiers' },
    owner: 'Propriétaire',
    everyOwner: 'Tous les propriétaires',
    server: 'Le serveur',
    search: 'Rechercher dans les sources',
    searchPlaceholder: 'Rechercher par nom',
    clearFilters: 'Effacer les filtres',
    emptyTitle: 'Aucune source pour l’instant',
    emptyShared:
      'Ajoutez un addon Stremio, un addon Eclipse, une source IPTV ou un dossier local : il servira à tous les utilisateurs de ce serveur.',
    emptyMine:
      'Ajoutez un addon Stremio, un addon Eclipse ou une source IPTV : il ne servira qu’à vous, après ceux du serveur.',
    noMatchTitle: 'Aucune source ne correspond',
    noMatch: 'Changez le type, le propriétaire ou la recherche.',
    tag: {
      stremio: 'Stremio',
      eclipse: 'Eclipse',
      m3u: 'M3U',
      xtream: 'Xtream',
      hdhomerun: 'HDHomeRun',
      local: 'Dossier',
    },
    kindName: {
      stremio: 'Addon Stremio',
      eclipse: 'Addon Eclipse',
      m3u: 'Playlist M3U',
      xtream: 'Compte Xtream Codes',
      hdhomerun: 'Tuner HDHomeRun',
      local: 'Dossier local',
    },
    ownedBy: (name: string) => `Source de ${name}`,
    yours: 'Votre propre source',
    active: 'Active',
    off: 'Désactivée',
    failed: 'Échec du téléchargement',
    channelsShown: (count: string) => `${count} chaînes affichées`,
    notDownloaded: 'Pas encore téléchargée',
    moreActions: (name: string) => `Plus d’actions pour ${name}`,
    openPage: 'Ouvrir sa page',
    enabled: 'Activée',
    enabledHelp:
      'Désactiver une source masque ses bibliothèques dans les applications Jellyfin sans les perdre.',
    order: 'Ordre',
    position: (position: number, count: number) => `${position} sur ${count}`,
    moveUp: (name: string) => `Monter ${name}`,
    moveDown: (name: string) => `Descendre ${name}`,
    moved: (name: string, position: number, count: number) =>
      `${name} est maintenant en position ${position} sur ${count}.`,
    description: 'Description',
    details: 'Détails',
    manifestUrl: 'Adresse du manifeste',
    address: 'Adresse',
    version: 'Version',
    provides: 'Fournit',
    types: 'Types',
    catalogs: 'Catalogues',
    catalogCount: (count: number) => (count <= 1 ? `${count} catalogue` : `${count} catalogues`),
    lastRefresh: 'Dernière actualisation',
    channels: 'Chaînes',
    notImported: 'Non importées',
    shownOf: (shown: string, total: string) => `${shown} affichées sur ${total}`,
    vod: 'Films et séries',
    vodShort: (movies: string, series: string) => `${movies} films et ${series} séries importés`,
    lastFetch: 'Dernier téléchargement',
    nextFetch: 'Prochain téléchargement',
    never: 'Jamais',
    figures: {
      entries: 'Entrées dans la source',
      shown: 'Chaînes affichées',
      mapped: 'Avec programme',
      unmapped: 'Sans programme',
    },
    openSource: 'Ouvrir la source',
    iptvHint:
      'Ce qu’il faut importer, les catégories, chaînes, numéros et guides se règlent sur la page de la source.',
    refresh: 'Actualiser',
    refreshLabel: (name: string) => `Actualiser ${name}`,
    refreshed: (name: string) => `${name} : le manifeste a été téléchargé à nouveau.`,
    refreshedList: (name: string) => `${name} : la liste a été téléchargée à nouveau.`,
    replace: 'Remplacer l’adresse',
    newManifestUrl: 'Nouvelle adresse du manifeste',
    replaceHint:
      'À utiliser après avoir reconfiguré l’addon sur sa page de configuration. Ses bibliothèques sont conservées.',
    replaceSubmit: 'Remplacer',
    replaced: 'L’adresse du manifeste a été remplacée.',
    language: 'Langue de ses descriptions',
    languageHelp:
      'Laissez Aucune sur le seul addon de métadonnées du serveur, quelle que soit la langue réglée sur sa page de configuration : un addon avec une langue ne décrit les titres que pour les utilisateurs qui ont choisi cette langue, sauf si aucun addon laissé sur Aucune ne les décrit. Pour que les utilisateurs lisent les titres dans d’autres langues, ajoutez le même addon de métadonnées une fois de plus par langue, configuré dans cette langue sur sa page de configuration, et indiquez ici la langue sur ces copies supplémentaires seulement : les utilisateurs qui choisissent cette langue des métadonnées voient ses noms, ses résumés et ses images. Un addon avec une langue et sans bibliothèque à lui ne fait que décrire les titres.',
    languageNone: 'Aucune',
    languageSaved: (name: string) => `${name} : la langue de ses descriptions a été enregistrée.`,
    edit: 'Modifier le compte',
    saved: 'Source enregistrée.',
    settings: 'Réglages',
    remove: 'Supprimer',
    removeTitle: (name: string) => `Supprimer ${name} ?`,
    removeShared:
      'Ses bibliothèques disparaîtront des applications Jellyfin de tous les utilisateurs. Cette action est irréversible.',
    removeMine:
      'Ses bibliothèques disparaîtront de vos applications Jellyfin. Cette action est irréversible.',
    removed: (name: string) => `${name} a été supprimée.`,
    readOnly: (name: string) =>
      `C’est une source personnelle de ${name}. Seul son propriétaire peut la modifier, depuis sa page Mes sources.`,
    back: 'Sources',
    backMine: 'Mes sources',
    notFoundTitle: 'Source introuvable',
    notFound: 'Cette source n’existe pas, ou vous ne pouvez pas la gérer.',
  },
  sourceAdd: {
    title: 'Ajouter une source',
    kind: 'Type de source',
    kinds: {
      stremio: 'Addon Stremio',
      music: 'Addon Eclipse',
      iptv: 'Source IPTV',
      tuner: 'Tuner HDHomeRun',
      local: 'Dossier',
    },
    stremioHelp:
      'Un addon Stremio apporte des catalogues, des métadonnées et des flux. Collez son lien d’installation.',
    musicHelp:
      'Un addon de musique Eclipse apporte de la musique, des livres audio ou des podcasts. Collez l’adresse de son manifeste.',
    manifestUrl: 'Adresse du manifeste',
    manifestUrlPlaceholder: 'https://…/manifest.json',
    manifestUrlHint:
      'Copiez le lien d’installation depuis la page de configuration de l’addon. Il contient souvent vos réglages ou vos clés : Polyfin le garde privé et n’en affiche jamais qu’une version abrégée.',
    install: 'Installer',
    installed: (name: string) => `${name} a été installé.`,
    iptvHelp:
      'Importez directement les chaînes d’une playlist M3U ou d’un compte Xtream Codes. Elles apparaissent dans la TV en direct comme un catalogue TV, que vous pouvez ranger dans Bibliothèques. La liste est téléchargée à nouveau selon l’intervalle choisi dans Paramètres › TV en direct.',
    steps: 'Étapes',
    stepAccount: 'Compte',
    stepImport: 'À importer',
    name: 'Nom',
    account: 'Compte',
    m3u: 'Playlist M3U',
    xtream: 'Compte Xtream Codes',
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
    next: 'Suivant : choisir quoi importer',
    back: 'Retour au compte',
    import: 'Importer',
    added: (name: string, parts: string[]) => `${name} a été ajoutée : ${parts.join(', ')}.`,
    channels: (count: string) => `${count} chaînes`,
    movies: (count: string) => `${count} films`,
    series: (count: string) => `${count} séries`,
    tunerHelp:
      'Un tuner HDHomeRun de votre réseau apporte les chaînes qu’il reçoit par antenne ou par câble. Elles apparaissent dans la TV en direct comme celles d’une source IPTV, et le tuner lit autant de chaînes à la fois qu’il a de tuners.',
    findTuners: 'Chercher les tuners',
    findTunersHelp:
      'Demande au réseau local quels tuners s’y trouvent ; cela prend quelques secondes.',
    finding: 'Recherche sur le réseau local…',
    foundTitle: 'Tuners du réseau',
    tunerCount: (count: number, formatted: string) =>
      count === 1 ? '1 tuner' : `${formatted} tuners`,
    addTuner: 'Ajouter',
    addTunerLabel: (name: string) => `Ajouter ${name}`,
    tunerAdded: 'Ajouté',
    openTuner: (name: string) => `Ouvrir ${name}`,
    noTuners: 'Aucun tuner n’a répondu.',
    noTunersHelp:
      'Sous Docker, Polyfin n’entend les tuners qu’en réseau hôte (network_mode: host). Vous pouvez aussi ajouter un tuner par son adresse.',
    byAddress: 'Ajouter par adresse',
    tunerAddress: 'Adresse du tuner',
    tunerAddressHint: 'Son adresse IP, comme 192.168.1.20, ou son adresse http://.',
    tunerAddressInvalid:
      'Saisissez l’adresse IP du tuner, comme 192.168.1.20, ou son adresse http://.',
    tunerName: 'Nom (facultatif)',
    tunerNameHint: 'Laissé vide, le nom que le tuner se donne.',
    favoritesOnly: 'Seulement les chaînes favorites du tuner',
    favoritesOnlyHelp:
      'Garde les chaînes marquées favorites sur le tuner, dans son application ou sa page web, et laisse de côté les autres.',
  },
  localFolders: {
    help: {
      container:
        'Un dossier monté dans le conteneur de Polyfin, en lecture seule suffit. Ses vidéos sont associées à des titres d’après leur nom et se lisent comme des versions de ces titres, à côté des flux des addons ; un dossier de musique devient une bibliothèque musicale. Polyfin tourne sous l’utilisateur 65532, qui doit pouvoir le lire.',
      computer:
        'Un dossier de l’ordinateur où tourne Polyfin, ou d’un disque ou d’un partage qu’il atteint, en lecture seule suffit. Ses vidéos sont associées à des titres d’après leur nom et se lisent comme des versions de ces titres, à côté des flux des addons ; un dossier de musique devient une bibliothèque musicale. Le compte sous lequel tourne Polyfin doit pouvoir le lire.',
    },
    location: 'Emplacement',
    locations: {
      local: 'Dossier local',
      smb: 'Partage SMB',
      webdav: 'Dossier WebDAV',
      s3: 'Bucket S3',
      sftp: 'Dossier SFTP',
      rclone: 'Remote rclone',
    },
    shareHelp: {
      smb: 'Un dossier partagé en SMB (partage de fichiers Windows) par un NAS ou un autre ordinateur, que Polyfin lit lui-même, sans le monter. Ses vidéos sont associées à des titres d’après leur nom et se lisent comme des versions de ces titres, à côté des flux des addons ; un dossier de musique devient une bibliothèque musicale.',
      webdav:
        'Un dossier qu’un serveur partage en WebDAV, que Polyfin lit lui-même, sans le monter. Ses vidéos sont associées à des titres d’après leur nom et se lisent comme des versions de ces titres, à côté des flux des addons ; un dossier de musique devient une bibliothèque musicale.',
      s3: 'Un préfixe d’un bucket de stockage compatible S3, que Polyfin lit lui-même, sans le monter. Ses vidéos sont associées à des titres d’après leur nom et se lisent comme des versions de ces titres, à côté des flux des addons, lues à travers le cache des fichiers en cours de lecture ; un dossier de musique devient une bibliothèque musicale.',
      sftp: 'Un dossier d’un serveur joint en SSH, que Polyfin lit lui-même, sans le monter. Ses vidéos sont associées à des titres d’après leur nom et se lisent comme des versions de ces titres, à côté des flux des addons, lues à travers le cache des fichiers en cours de lecture ; un dossier de musique devient une bibliothèque musicale.',
      rclone:
        'Un dossier de l’un des stockages que rclone sait joindre, lu à travers une instance de rclone qui exécute rclone rcd --rc-serve, comme un conteneur à côté de Polyfin. Ses vidéos sont associées à des titres d’après leur nom et se lisent comme des versions de ces titres, à côté des flux des addons, lues à travers le cache des fichiers en cours de lecture ; un dossier de musique devient une bibliothèque musicale.',
    },
    address: 'Adresse',
    addressHint: {
      smb: 'smb:// puis le serveur, le partage, et le dossier qu’il contient s’il y a lieu. Ajoutez :port après le serveur pour un autre port que 445.',
      webdav:
        'L’adresse WebDAV du dossier. Préférez https://, car http:// envoie le mot de passe sans chiffrement.',
      s3: 's3:// puis le bucket, et le préfixe qu’il contient s’il y a lieu. Ses objets sont listés comme des dossiers par /.',
      sftp: 'sftp:// puis le serveur, et le chemin complet du dossier sur celui-ci. Ajoutez :port après le serveur pour un autre port que 22.',
      rclone: '',
    },
    addressPlaceholder: {
      smb: 'smb://nas.local/media/films',
      webdav: 'https://nas.local/dav/films',
      s3: 's3://media/films',
      sftp: 'sftp://nas.local/srv/media/films',
      rclone: '',
    },
    user: 'Utilisateur',
    userHint: {
      smb: 'Vide pour un partage ouvert aux invités. DOMAINE\\utilisateur pour un compte de domaine.',
      webdav: 'Vide pour un dossier ouvert à tous.',
      s3: 'Les deux vides pour un bucket que tout le monde peut lire.',
      sftp: 'Le compte sous lequel Polyfin se connecte.',
      rclone: 'L’utilisateur du contrôle à distance de rclone (--rc-user), vide s’il n’y en a pas.',
    },
    password: 'Mot de passe',
    passwordHint: 'Polyfin ne l’affiche plus jamais.',
    sftpPasswordHint: 'Vide quand la clé privée suffit. Polyfin ne l’affiche plus jamais.',
    accessKey: 'Clé d’accès',
    secretKey: 'Clé secrète',
    endpoint: 'Point de terminaison',
    endpointHint:
      'L’adresse du service S3 du stockage. Polyfin nomme le bucket dans le chemin, comme tout stockage compatible S3 l’accepte, sauf aux points de terminaison d’Amazon.',
    region: 'Région',
    regionHint: 'Vide pour us-east-1, que la plupart des stockages compatibles S3 acceptent.',
    privateKey: 'Clé privée',
    privateKeyHint:
      'Une clé privée OpenSSH ou PEM, à la place du mot de passe ou avec lui. Polyfin ne l’affiche plus jamais.',
    privateKeySaved: 'Une clé est enregistrée. Collez-en une autre pour la remplacer.',
    privateKeyNotSent:
      'La clé enregistrée n’est pas envoyée à un autre serveur : collez-la à nouveau, sinon elle est supprimée à l’enregistrement.',
    secretNotSent:
      'La valeur enregistrée n’est pas envoyée à un autre serveur : saisissez-la à nouveau, sinon elle est supprimée à l’enregistrement.',
    privateKeyRemove: 'Supprimer la clé enregistrée',
    passphrase: 'Phrase de passe',
    passphraseHint: 'Seulement pour une clé privée qui en a une.',
    hostKey: 'Clé d’hôte',
    hostKeyHint:
      'Enregistrée à la première connexion. Polyfin refuse le serveur quand il en présente une autre.',
    hostKeyNone: 'Enregistrée à la première connexion',
    hostKeyOffered: (fingerprint: string) =>
      `Le serveur présente désormais une autre clé d’hôte, ${fingerprint}, et Polyfin la refuse. Acceptez-la seulement si le serveur a été réinstallé ou si sa clé a été changée volontairement.`,
    acceptHostKey: 'Accepter la nouvelle clé',
    hostKeyAccepted: 'La nouvelle clé d’hôte est acceptée. Polyfin analyse à nouveau le dossier.',
    rcloneInstance: 'Adresse de rclone',
    rcloneInstanceHint:
      'Là où répond rclone rcd --rc-serve, jointe en http://, comme http://rclone:5572 pour un conteneur à côté de Polyfin.',
    rcloneRemote: 'Remote',
    rcloneRemoteHint: 'L’un des remotes de la configuration de rclone.',
    listRemotes: 'Lister les remotes',
    remotesListed: (count: number, figure: string) =>
      count === 1 ? 'rclone a un remote.' : `rclone a ${figure} remotes.`,
    noRemotes: 'rclone n’a aucun remote dans sa configuration.',
    remotesFailed: {
      unreachable: 'rclone ne répond pas à cette adresse.',
      refused: 'rclone refuse cet utilisateur ou ce mot de passe.',
    } as Record<string, string>,
    rcloneFolder: 'Chemin dans le remote',
    rcloneFolderHint: 'Le dossier du remote à lire, vide pour sa racine.',
    pollMinutes: 'Chercher les changements toutes les (minutes, 0 = seulement aux analyses)',
    pollMinutesHint: {
      s3: 'Les dossiers du bucket sont listés à nouveau à cet intervalle pour trouver les fichiers ajoutés et supprimés. Les fournisseurs peuvent facturer chaque listage. 30 par défaut.',
      sftp: 'Le dossier et ses sous-dossiers sont listés à nouveau à cet intervalle pour trouver les fichiers ajoutés et supprimés. 2 par défaut.',
      rclone:
        'Les dossiers du remote sont listés à nouveau à cet intervalle pour trouver les fichiers ajoutés et supprimés. 2 par défaut.',
    },
    name: 'Nom',
    nameHint: 'Le nom de sa bibliothèque dans les applications Jellyfin.',
    path: { container: 'Chemin dans le conteneur', computer: 'Chemin' },
    pathHint: {
      container:
        'L’endroit où le dossier est monté dans le conteneur de Polyfin, pas son chemin sur l’hôte.',
      linux: 'Le chemin complet du dossier sur l’ordinateur où tourne Polyfin.',
      macos:
        'Le chemin complet du dossier sur le Mac où tourne Polyfin ; les dossiers d’un disque externe sont sous /Volumes.',
      windows:
        'Le chemin complet du dossier sur le PC où tourne Polyfin : sur un lecteur, comme D:\\Films, ou sur un partage, comme \\\\nas\\media\\Films.',
    },
    pathPlaceholder: {
      container: '/media/films',
      linux: '/srv/media/films',
      macos: '/Users/moi/Films',
      windows: 'D:\\Films',
    },
    kind: 'Il contient',
    kinds: {
      movies: 'Films',
      shows: 'Séries',
      music: 'Musique',
      homevideos: 'Vidéos personnelles',
    },
    kindHelp: {
      movies:
        'Un fichier par film, seul ou dans un dossier à lui, nommé d’après le film et son année.',
      shows:
        'Un dossier par série, avec la saison et l’épisode dans le nom de chaque fichier, comme S01E02.',
      music:
        'Des fichiers audio lus d’après leurs tags, sinon leurs noms, comme Artiste/Album/01 Titre.flac, regroupés en albums et en artistes.',
      homevideos:
        'Chaque vidéo est un titre à part, nommé d’après son fichier, ou son dossier quand elle y est seule, et daté par son fichier NFO, son enregistrement ou le fichier. Aucun addon de métadonnées n’est interrogé.',
    },
    kindNouns: {
      movies: 'des films',
      shows: 'des séries',
      music: 'de la musique',
      homevideos: 'des vidéos personnelles',
    },
    changeKind: 'Changer',
    changeKindTitle: (name: string) => `Changer le contenu de ${name} ?`,
    changeKindScan: (from: string, to: string) =>
      `Il contient ${from} ; il contiendra ${to}. Le dossier et sa bibliothèque gardent leurs identifiants, son adresse et ses réglages, et Polyfin l’analyse à nouveau comme contenant ${to}, en arrière-plan.`,
    changeKindFollows:
      'Ce que les utilisateurs ont fait de chaque vidéo, son état de lecture, son point de reprise et son favori, la suit vers son nouveau titre : ce qui a été vu et les favoris lui sont donnés, et l’ancien titre les garde ; le point de reprise suit le fichier, et l’ancien titre le perd.',
    changeKindNothing:
      'Rien ne suit : les morceaux et les vidéos sont d’autres fichiers. Ce que les utilisateurs ont fait de ses titres leur reste attaché et ne s’affiche plus.',
    changeKindBack:
      'Ses vidéos personnelles sont gardées en mémoire 30 jours : s’il contient à nouveau des vidéos personnelles d’ici là, il les présente comme les mêmes titres, avec ce que les utilisateurs en ont fait.',
    linksDropped: (count: number, figure: string) =>
      count <= 1 ? 'l’association manuelle' : `les ${figure} associations manuelles`,
    episodesDropped: (count: number, figure: string) =>
      count <= 1
        ? 'les épisodes indiqués pour le titre d’un disque'
        : `les épisodes indiqués pour ${figure} titres de disques`,
    changeKindDrops: (parts: string[]) =>
      `Ce qui était fait pour son contenu actuel est supprimé : ${parts.join(' et ')}.`,
    useArtwork: 'Utiliser ses illustrations',
    useArtworkHelp:
      'Les affiches, fonds et logos à côté des fichiers, comme poster.jpg ou Film (2001)-fanart.jpg, s’affichent à la place de ceux des addons de métadonnées pour leurs titres, et les images des épisodes à la place de leurs vignettes. Désactivé, ceux des addons s’affichent.',
    readExtras: 'Lire les bonus et les thèmes',
    readExtrasHelp:
      'Les bonus, bandes-annonces locales et musiques de thème à côté des titres, nommés comme pour Jellyfin, comme Extras/Making of.mkv, Film (2001)-trailer.mkv ou theme.mp3, s’affichent dans les bonus de leurs titres, se lisent avec le bouton Bande-annonce ou en fond de leurs pages. Désactivé, la prochaine analyse les ignore.',
    add: 'Ajouter le dossier',
    added: (name: string) => `${name} a été ajouté. Sa première analyse a commencé.`,
    edit: 'Modifier le dossier',
    save: 'Enregistrer',
    cancel: 'Annuler',
    saved: 'Dossier enregistré.',
    movedHint:
      'Un nouveau chemin oublie les fichiers trouvés dans l’ancien, associations manuelles comprises.',
    movedShareHint:
      'Une nouvelle adresse oublie les fichiers trouvés dans l’ancienne, associations manuelles comprises.',
    scan: 'Analyser maintenant',
    scanLabel: (name: string) => `Analyser ${name} maintenant`,
    scanStarted: (name: string) => `${name} : l’analyse a commencé.`,
    scanning: 'Analyse en cours',
    unreadable: 'Illisible',
    figuresTitle: 'Fichiers',
    files: 'Fichiers vidéo',
    matched: 'Associés à un titre',
    unmatched: 'Non associés',
    extras: 'Bonus',
    tracks: 'Morceaux',
    albums: 'Albums',
    artists: 'Artistes',
    gained: 'Avec un gain',
    measured: 'Mesurés',
    leftOut: 'Laissés de côté',
    videos: 'Vidéos',
    datedBy: {
      nfo: 'Datées par un fichier NFO',
      created: 'Datées par l’enregistrement',
      file: 'Datées par le fichier',
    },
    matchedBy: (parts: string[]) => `Associés : ${parts.join(', ')}.`,
    matchMethods: {
      name: (count: string) => `${count} par un identifiant dans le nom`,
      nfo: (count: string) => `${count} par un fichier NFO`,
      search: (count: string) => `${count} par une recherche`,
      link: (count: string) => `${count} à la main`,
    },
    holds: 'Contenu',
    lastScan: 'Dernière analyse',
    lastAttempt: 'Dernière tentative',
    never: 'Pas encore',
    errorTitle: 'Polyfin ne peut pas lire ce dossier',
    errorTitleShare: 'Polyfin ne peut pas lire ce partage',
    errors: {
      missing:
        'Ce chemin n’existe pas là où tourne Polyfin : vérifiez que le dossier, ou le disque ou le partage où il se trouve, est bien là, et sous Docker qu’il est monté dans le conteneur.',
      unreadable:
        'Le dossier existe mais Polyfin ne peut pas le lister : donnez au compte sous lequel il tourne, l’utilisateur 65532 sous Docker, le droit de le lire et d’ouvrir ses dossiers.',
      not_folder: 'Ce chemin est un fichier, pas un dossier.',
    } as Record<string, string>,
    shareErrors: {
      missing: 'Le partage, ou le dossier qu’il contient, n’existe pas : vérifiez l’adresse.',
      unreadable:
        'L’utilisateur du partage ne peut pas lister ce dossier : vérifiez ses droits sur le serveur.',
      not_folder: 'Cette adresse est un fichier, pas un dossier.',
      unreachable:
        'Polyfin ne peut pas joindre le partage : vérifiez que le serveur est allumé, ainsi que le serveur et le port de l’adresse.',
      refused:
        'Le partage refuse l’utilisateur ou le mot de passe : vérifiez-les, et saisissez-les à nouveau.',
      host_key_changed:
        'Le serveur SFTP présente une autre clé d’hôte que celle enregistrée à la première connexion, et Polyfin la refuse. Si ce changement est attendu, acceptez la nouvelle clé sous Modifier le dossier.',
    } as Record<string, string>,
    keptNote:
      'Les fichiers trouvés auparavant restent dans la bibliothèque jusqu’à ce que le dossier soit à nouveau lisible.',
    unmatchedTitle: 'Fichiers non associés',
    unmatchedHelp:
      'Aucun titre n’a été trouvé pour ces fichiers. Renommez-les avec le titre et l’année, ou ajoutez un fichier NFO donnant l’identifiant du titre, puis lancez une analyse ; ou associez-les à l’identifiant IMDb présent dans l’adresse IMDb du titre (tt suivi de chiffres). Une association survit aux analyses ; dans un dossier de séries, elle couvre toute la série, et pour un disque Blu-ray ou un DVD, tout le disque, ou l’un des films d’un disque qui en contient plusieurs. Les titres du disque d’une série prennent leur épisode comme le nom d’un fichier le donne, par exemple S01E05, ou S01E05-E06 pour deux ; les épisodes proposés pour un disque suivent ceux des disques précédents de la saison.',
    noUnmatched: 'Chaque fichier est associé à un titre.',
    leftOutTitle: 'Morceaux laissés de côté',
    leftOutHelp:
      'Ni les tags ni les noms de ces fichiers ne donnent un titre ou un album. Ajoutez-leur des tags, ou placez-les dans un dossier Artiste/Album, puis lancez une analyse.',
    noLeftOut: 'Chaque morceau est dans un album.',
    shownOf: (shown: string, total: string) => `${shown} affichés sur ${total}.`,
    reasons: {
      unreadable_name: 'Son nom ne donne aucun titre, ou aucun numéro d’épisode',
      disc_title: 'Un autre film du disque : associez-le à son titre',
      disc_episode: 'Un titre du disque d’une série : indiquez son épisode',
      lone_extra: 'Un bonus sans son titre à côté : associez-le pour en faire un titre',
      not_found: 'Les addons de métadonnées ne connaissent aucun titre de ce nom',
      ambiguous: 'Plusieurs titres portent ce nom : ajoutez-y l’année, ou associez-le',
      other_year: 'Les titres de ce nom sont d’autres années',
      search_failed: 'La recherche auprès des addons de métadonnées a échoué',
      no_title: 'Ses tags et son nom ne donnent aucun titre',
      no_album: 'Ses tags et ses dossiers ne donnent aucun album',
    } as Record<string, string>,
    read: (title: string) => `Lu comme « ${title} »`,
    showFolder: (unit: string) => `Dossier de la série : ${unit}`,
    discTitle: (disc: string, title: number) => `${disc} · titre ${title}`,
    disc: {
      bluray: (unit: string) => `Disque Blu-ray : ${unit}`,
      dvd: (unit: string) => `DVD : ${unit}`,
      bluray_image: (unit: string) => `Image Blu-ray : ${unit}`,
      dvd_image: (unit: string) => `Image DVD : ${unit}`,
    },
    minutes: (minutes: number) => `${minutes} min`,
    episodeLabel: (title: string) => `Épisode de ${title}`,
    episodePlaceholder: 'S01E01',
    giveEpisode: 'Enregistrer',
    episodesGiven: (count: number) =>
      count === 1 ? 'Le titre est numéroté.' : `${count} titres sont numérotés.`,
    suggestion: (disc: string, first: string, last: string) =>
      first === last
        ? `Proposé pour ${disc} : ${first}`
        : `Proposés pour ${disc} : ${first} à ${last}`,
    confirmSuggestion: 'Utiliser ces épisodes',
    imdbId: 'Identifiant IMDb',
    imdbPlaceholder: 'tt0000000',
    link: 'Associer',
    linkLabel: (path: string) => `Identifiant IMDb pour ${path}`,
    linked: (unit: string, id: string) => `${unit} est associé à ${id}.`,
    findTitle: 'Trouver le titre',
    searchLabel: (path: string) => `Recherche du titre de ${path}`,
    searchQuery: 'Titre',
    searchYear: 'Année',
    search: 'Rechercher',
    searching: 'Recherche dans les addons de métadonnées…',
    noTitles:
      'Les addons de métadonnées n’ont trouvé aucun titre de ce nom avec un identifiant IMDb. Modifiez la recherche, ou saisissez l’identifiant.',
    titlesFound: (query: string) => `Titres trouvés pour «\u202F${query}\u202F»`,
    titleKinds: { movie: 'Film', show: 'Série' },
    pickTitle: (name: string, details: string) => `Associer à ${name}, ${details}`,
    linksTitle: 'Associés à la main',
    unlink: 'Retirer l’association',
    unlinkLabel: (unit: string) => `Retirer l’association de ${unit}`,
    unlinked: (unit: string) =>
      `L’association de ${unit} a été retirée : il est à nouveau associé d’après son nom ou son fichier NFO.`,
    removeEpisodeLabel: (title: string) => `Retirer l’épisode de ${title}`,
    episodeRemoved: (title: string) =>
      `L’épisode de ${title} a été retiré : le nom de son disque le numérote, ou il est à nouveau proposé.`,
    openPage: 'Ouvrir la page du dossier',
  },
  mySources: {
    lede: 'Ajoutez vos propres sources Stremio, Eclipse et IPTV et choisissez quels catalogues apparaissent comme bibliothèques dans vos applications Jellyfin.',
    preferenceTitle: 'Les sources du serveur',
    useShared: 'Utiliser les sources du serveur',
    useSharedHelp:
      'Activé : vous profitez des sources et bibliothèques du serveur, suivies des vôtres. Désactivé : vous ne voyez que vos propres sources et bibliothèques.',
    preferenceSaved: 'Préférence enregistrée.',
    parentalControl:
      'Le contrôle parental s’applique à votre compte : vos applications Jellyfin n’affichent que les sources et bibliothèques du serveur, qui donnent les classifications sur lesquelles il s’appuie. Vos propres sources sont conservées mais pas utilisées.',
    personalAddonsOff:
      'Un administrateur a désactivé vos propres sources : vos applications Jellyfin n’affichent que les sources et bibliothèques du serveur. Vos propres sources sont conservées mais pas utilisées.',
    listTitle: 'Vos sources',
  },
  music: {
    content: { music: 'Musique', audiobook: 'Livres audio', podcast: 'Podcasts' },
    library: {
      music: 'Bibliothèque musicale',
      audiobook: 'Bibliothèque de livres',
      podcast: 'Bibliothèque musicale',
    },
    settings: 'Réglages',
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
    belowMin: (min: string) => `Saisissez ${min} ou plus.`,
    aboveMax: (max: string) => `Saisissez ${max} ou moins.`,
    offStep: (step: string) => `Utilisez un multiple de ${step}.`,
    resetDefaults: 'Remettre les valeurs par défaut',
    saved: 'Réglages enregistrés. L’addon les reçoit dès sa prochaine requête.',
  },
}

export default sources
