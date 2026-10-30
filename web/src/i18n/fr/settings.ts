import type en from '../en/settings'
import type { RangeText } from '../en/settings'

const settings: typeof en = {
  settings: {
    tracking: {
      description:
        'Chaque utilisateur peut connecter ses propres comptes Trakt, Simkl, MDBList, PublicMetaDB, AniList et MyAnimeList depuis sa page Mon compte, et Polyfin indique à ces services ce qu’il regarde ; Last.fm et ListenBrainz reçoivent les morceaux qu’il écoute. Trakt, Simkl, AniList, MyAnimeList et Last.fm ont d’abord besoin d’une application de ce serveur, à configurer ici. MDBList, PublicMetaDB et ListenBrainz n’ont besoin de rien.',
      traktSetup:
        'Créez une application sur trakt.tv, dans Settings, Your API Apps, New application. Donnez-lui le nom de votre choix, puis copiez ici son client ID et son client secret.',
      redirectUri: 'Adresse de redirection (redirect URI) à saisir dans l’application',
      traktClientId: 'Client ID Trakt',
      traktClientIdHelp:
        'Laissez vide pour désactiver Trakt. Les utilisateurs peuvent se connecter une fois le client ID et le client secret enregistrés.',
      traktClientSecret: 'Client secret Trakt',
      traktClientSecretHelp:
        'Affiché sur la page de l’application sur trakt.tv, sous le client ID.',
      simklSetup:
        'Créez une application sur simkl.com, dans Settings, Developer, Create new app, du type « TV, devices & command line » (l’AUTH V2 de Simkl ; elle n’a pas besoin d’adresse de redirection, et les client ID des anciennes applications AUTH V1 sont refusés). Donnez-lui le nom de votre choix, puis copiez ici son client ID.',
      simklClientId: 'Client ID Simkl',
      simklClientIdHelp:
        'Laissez vide pour désactiver Simkl. Aucun client secret n’est nécessaire.',
      lastFmSetup:
        'Créez un compte d’API sur last.fm, à l’adresse last.fm/api/account/create. Donnez-lui le nom de votre choix et laissez son adresse de rappel (callback URL) vide : les utilisateurs autorisent Polyfin sur Last.fm, et Polyfin demande lui-même leur session à Last.fm. Copiez ensuite ici sa clé d’API et son secret partagé.',
      lastFmApiKey: 'Clé d’API Last.fm',
      lastFmApiKeyHelp:
        'Laissez vide pour désactiver Last.fm. Les utilisateurs peuvent se connecter une fois la clé d’API et le secret partagé enregistrés.',
      lastFmSecret: 'Secret partagé Last.fm',
      lastFmSecretHelp: 'Affiché sur la page de votre compte d’API sur last.fm, sous la clé d’API.',
      aniListSetup:
        'Créez une application sur anilist.co, dans Settings, Developer, Create New Client. Donnez-lui le nom de votre choix, et comme adresse de redirection l’adresse ci-dessous, où AniList montre aux utilisateurs le code qu’ils collent dans Polyfin. Copiez ensuite ici son client ID et son client secret.',
      aniListClientId: 'Client ID AniList',
      aniListClientIdHelp:
        'Laissez vide pour désactiver AniList. Les utilisateurs peuvent se connecter une fois le client ID et le client secret enregistrés.',
      aniListClientSecret: 'Client secret AniList',
      aniListClientSecretHelp:
        'Affiché sur la page de l’application sur anilist.co, sous le client ID.',
      myAnimeListSetup:
        'Créez une application sur myanimelist.net, dans Account Settings, API, Create ID, du type « web ». Donnez-lui le nom de votre choix, et comme adresse de redirection l’adresse ci-dessous : MyAnimeList y renvoie le navigateur des utilisateurs une fois Polyfin autorisé. Seul leur navigateur doit pouvoir l’atteindre. Copiez ensuite ici son client ID et son client secret.',
      myAnimeListRedirectHelp:
        'L’adresse publique de ce serveur, dans Paramètres › Général, sinon l’adresse que vous utilisez maintenant.',
      myAnimeListClientId: 'Client ID MyAnimeList',
      myAnimeListClientIdHelp:
        'Laissez vide pour désactiver MyAnimeList. Les utilisateurs peuvent se connecter une fois le client ID et le client secret enregistrés.',
      myAnimeListClientSecret: 'Client secret MyAnimeList',
      myAnimeListClientSecretHelp:
        'Affiché sur la page de l’application sur myanimelist.net, sous le client ID.',
    },
    title: 'Paramètres',
    description: 'Options qui s’appliquent à l’ensemble du serveur.',
    serverName: 'Nom du serveur',
    serverNameHelp: (r: RangeText) =>
      `De ${r.min} à ${r.max} caractères, affiché dans les applications Jellyfin.`,
    quickConnect: 'Autoriser Quick Connect',
    quickConnectHelp:
      'Permet aux applications TV et mobiles de se connecter avec un code autorisé ici : 6 chiffres pour les applications Jellyfin, 8 pour les applications TV de Silo tant que les applications de Silo sont autorisées.',
    language: 'Langue des noms générés',
    languageHelp:
      'Polyfin nomme lui-même certains éléments dans les applications Jellyfin : les saisons (« Saison 1 », « Épisodes spéciaux »), les épisodes sans titre et le type ajouté aux bibliothèques de même nom (« Populaires (Films) »).',
    legacyAuthorization: 'Autoriser l’authentification héritée',
    legacyAuthorizationHelp:
      'Accepte les anciens en-têtes X-Emby-*, le paramètre api_key et le schéma Emby pour les applications qui en ont encore besoin. Désactivé par défaut, comme dans Jellyfin 12.4.',
    siloApps: 'Autoriser les applications de Silo',
    siloAppsHelp:
      'Permet aux applications de Silo pour iPhone, iPad, Apple TV, Mac et Android de se connecter et de lire, par l’API de Silo sous /api/v2, comme les applications Jellyfin, dans les limites de chaque utilisateur. Désactivé, cette API ne répond rien.',
    updateCheck: 'Rechercher les nouvelles versions',
    updateCheckHelp:
      'Une fois par jour, et peu après le démarrage, Polyfin demande à GitHub sa dernière version publiée, en n’envoyant que sa propre version. Une version plus récente s’affiche sur l’Accueil et dans Système › Santé, et est envoyée aux notifications « Nouvelle version ». Désactivé, Polyfin ne contacte jamais GitHub.',
    restartConfirmed: 'Polyfin est redémarré quand il s’arrête',
    restartConfirmedHelp:
      'Activez-le une fois que le conteneur de Polyfin a une politique de redémarrage qui le relance, comme unless-stopped, always ou on-failure, ou que son gestionnaire de services le fait. Polyfin ne peut pas voir la politique de redémarrage de son conteneur sans le socket Docker, qui ne lui est jamais donné : tant que c’est désactivé, Redémarrer sur l’Accueil avertit que Polyfin pourrait ne pas revenir.',
    updateToolUrl: 'Adresse de l’outil de mise à jour',
    updateToolUrlHelp:
      'L’API de mise à jour d’un outil que vous faites tourner à côté de Polyfin pour mettre à jour son conteneur, comme http://updater:8080/v1/update. « Mettre à jour maintenant », sur l’Accueil, lui envoie une requête POST sans corps. Vide, ce bouton est masqué.',
    updateToolToken: 'Jeton de l’outil de mise à jour',
    updateToolTokenHelp:
      'Envoyé dans un en-tête Authorization: Bearer avec chaque requête, si l’outil en demande un. Stocké chiffré, comme les autres secrets.',
    localDiscovery: 'Répondre à la découverte sur le réseau local',
    localDiscoveryHelp:
      'Les applications Jellyfin et Emby qui cherchent des serveurs sur le réseau local trouvent Polyfin et l’affichent, sans qu’il faille saisir son adresse. Elles le demandent sur le port UDP 7359, et Polyfin ne répond qu’aux adresses du réseau local. S’applique tout de suite, sans redémarrage. Avec Docker, il faut le réseau de l’hôte ou le port 7359/udp publié : voir la documentation d’installation.',
    localAddress: 'Adresse sur le réseau local',
    localAddressHelp:
      'L’adresse à laquelle les applications du réseau local joignent Polyfin, comme http://192.168.1.10:8096. Les réponses à la découverte la donnent, et les applications Jellyfin la lisent comme l’adresse locale du serveur. Vide, les réponses donnent l’adresse de l’interface réseau qui joint l’application : c’est la bonne, sauf si Polyfin tourne dans un réseau bridge de Docker. Saisissez alors l’adresse de l’hôte et le port de Polyfin.',
    torrents: {
      description:
        'Certains addons listent des torrents plutôt que des liens. Polyfin peut les lire : il ne rejoint l’essaim d’un torrent que pendant qu’une lecture le lit, et garde tout son trafic sur une seule route, une interface réseau comme celle d’un VPN, un proxy SOCKS5, ou un tunnel WireGuard que Polyfin fait tourner lui-même. Rien n’est jamais envoyé par un autre chemin. Les morceaux sont gardés dans l’Espace disque des fichiers en lecture, dans Paramètres › Lecture.',
      play: 'Lire les flux torrent',
      playHelp:
        'Désactivé par défaut. Lire un torrent le télécharge auprès d’autres pairs, et partage ce qui a été téléchargé tant que la limite d’envoi le permet : vous répondez de ce que ce serveur télécharge et partage. Les utilisateurs ont aussi besoin de Peut lire les flux torrent, sur leur page dans Utilisateurs. Demande une route.',
      route: 'Route',
      routeHelp:
        'Chaque socket des torrents, pour les pairs, les trackers et le DHT, ne passe que par là. Quand elle ne fonctionne pas, les versions torrent sont ignorées, y compris pendant une lecture, et Système › Santé dit pourquoi.',
      routeHelpNoInterface:
        'Chaque socket des torrents, pour les pairs, les trackers et le DHT, ne passe que par là. Ce système ne lie aucun socket à une interface réseau, ce que seul Linux fait, comme Polyfin Desktop sous Windows et macOS : un proxy SOCKS5 ou un tunnel WireGuard porte les torrents. Quand la route ne fonctionne pas, les versions torrent sont ignorées, y compris pendant une lecture, et Système › Santé dit pourquoi.',
      routes: {
        interface: 'Interface réseau',
        proxy: 'Proxy SOCKS5',
        wireguard: 'Tunnel WireGuard',
      },
      interface: 'Interface réseau des torrents',
      interfaceHelp:
        'Son nom, comme wg0. Chaque socket des torrents y est lié, et les noms des trackers sont résolus par elle. Linux seulement. Sous Docker, le conteneur doit voir l’interface : voir la documentation des torrents.',
      proxy: 'Proxy SOCKS5 des torrents',
      proxyHelp:
        'Son adresse et son port, comme 10.0.0.2:1080. Les pairs et les trackers HTTP passent par son CONNECT. Les trackers UDP, le DHT et uTP ont besoin de son UDP ASSOCIATE, et restent désactivés sans lui. À travers un proxy, aucun pair ne peut se connecter à Polyfin.',
      proxyUser: 'Utilisateur du proxy',
      proxyUserHelp: 'Vide quand le proxy n’en demande pas.',
      proxyPassword: 'Mot de passe du proxy',
      proxyPasswordHelp: 'Stocké chiffré quand POLYFIN_SECRET_KEY est définie.',
      wireGuard: 'Configuration WireGuard',
      wireGuardHelp:
        'Collez le .conf que votre fournisseur de VPN donne pour WireGuard. Polyfin fait tourner le tunnel lui-même, pour les seuls torrents, sans rien à configurer sur ce système. Les noms des trackers sont résolus à travers le tunnel par ses serveurs DNS, jamais autrement. Aucun pair ne peut se connecter à Polyfin. Stockée chiffrée quand POLYFIN_SECRET_KEY est définie, et jamais réaffichée.',
      wireGuardSaved: (endpoint: string, addresses: string) =>
        `Une configuration est enregistrée : serveur ${endpoint}, adresses ${addresses}.`,
      wireGuardDns: (servers: string) =>
        `Les noms des trackers sont résolus par ${servers}, à travers le tunnel.`,
      wireGuardNoDns:
        'Elle ne nomme aucun serveur DNS : les trackers désignés par un nom d’hôte ne sont pas joints, ceux désignés par une adresse le sont.',
      wireGuardReplace: 'Collez une autre configuration pour remplacer celle enregistrée.',
      wireGuardInvalid: (problem: { line: number; key: string; reason: string }) => {
        const where = problem.line > 0 ? `Ligne ${problem.line} : ` : ''
        const expected: Record<string, string> = {
          PrivateKey: 'une clé de 32 octets en base64',
          PublicKey: 'une clé de 32 octets en base64',
          PresharedKey: 'une clé de 32 octets en base64',
          Address: 'des adresses IP, comme 10.2.0.2/32, séparées par des virgules',
          DNS: 'des adresses IP séparées par des virgules',
          MTU: 'un nombre de 576 à 65535',
          ListenPort: 'un port de 0 à 65535',
          Endpoint: 'un nom d’hôte ou une adresse IP avec un port, comme vpn.example.com:51820',
          AllowedIPs:
            'des adresses IP avec la longueur de leur préfixe, comme 0.0.0.0/0, séparées par des virgules',
          PersistentKeepalive: 'off, ou un nombre de secondes de 0 à 65535',
        }
        switch (problem.reason) {
          case 'too_long':
            return 'La configuration WireGuard doit être un texte d’au plus 8192 octets.'
          case 'syntax':
            return `${where}chaque ligne doit être une [section], une Clé = Valeur ou un commentaire #.`
          case 'outside':
            return `${where}${problem.key} vient avant [Interface] ou [Peer].`
          case 'unknown':
            return `${where}${problem.key} ne fait pas partie d’une configuration WireGuard.`
          case 'repeated':
            return problem.key === 'Peer'
              ? `${where}une seule section [Peer] est prise en charge.`
              : `${where}${problem.key} est donné deux fois.`
          case 'missing':
            return `${where}${problem.key} manque dans la configuration WireGuard.`
          case 'no_ipv4':
            return `${where}Address doit contenir une adresse IPv4, qu’utilisent les trackers UDP et le DHT.`
          case 'not_all':
            return `${where}AllowedIPs doit contenir 0.0.0.0/0 : les pairs d’un torrent sont partout.`
          default:
            return `${where}${problem.key} doit être ${expected[problem.key] ?? 'valide'}.`
        }
      },
      downloadLimit: 'Limite de téléchargement (Mbit/s, 0 = aucune)',
      downloadLimitHelp: (r: RangeText) => `De 0 à ${r.max} ; ${r.default} par défaut.`,
      uploadLimit: 'Limite d’envoi (Mbit/s, 0 = aucun partage)',
      uploadLimitHelp: (r: RangeText) =>
        `Les pairs favorisent ceux qui partagent en retour : le démarrage peut être plus lent à 0. Le partage s’arrête à la fin de la lecture. De 0 à ${r.max} ; ${r.default} par défaut.`,
      atOnce: 'Torrents simultanés (0 = aucune limite)',
      atOnceHelp: (r: RangeText) =>
        `Combien de torrents peuvent tourner en même temps, pour tous les utilisateurs ensemble. À la limite, un titre lit une autre de ses versions qui n’est pas un torrent, ou est dit illisible pour le moment ; une place se libère quand un torrent s’arrête, 30 secondes après sa lecture, ou aussitôt quand le même appareil ou utilisateur lit un autre titre. De 0 à ${r.max} ; ${r.default} par défaut.`,
      trackers: 'Trackers ajoutés à chaque torrent',
      trackersHelp: (r: RangeText) =>
        `Des URL d’annonce, une par ligne, comme udp://tracker.example:1337/announce. Polyfin leur demande les pairs de chaque torrent, en plus des trackers que nomme son addon : un torrent que l’addon liste sans aucun n’est alors pas laissé au seul DHT. HTTP, HTTPS et UDP ; ${r.max} au plus. Un torrent privé ne demande qu’à ses propres trackers. Vide par défaut : Polyfin n’est livré avec aucune liste.`,
    },
    dlna: {
      title: 'DLNA',
      description:
        'Les téléviseurs, amplificateurs, consoles et lecteurs du réseau local qui n’ont pas d’application Jellyfin peuvent parcourir et lire les bibliothèques en DLNA. Un appareil ne peut pas se connecter : chacun montre les bibliothèques de l’utilisateur choisi pour lui, avec les limites de cet utilisateur.',
      share: 'Partager les bibliothèques en DLNA',
      shareHelp:
        'Annonce Polyfin aux appareils du réseau local sur le port UDP 1900, et ne répond qu’à eux. S’applique tout de suite, sans redémarrage. Dans Docker, il faut le réseau de l’hôte ou un réseau qui donne au conteneur sa propre adresse : voir la documentation DLNA.',
      name: 'Nom sur le réseau',
      nameHelp: 'Le nom sous lequel les appareils listent Polyfin. Vide, le nom du serveur.',
      newDevicesUser: 'Montrer aux nouveaux appareils les bibliothèques de',
      newDevicesUserHelp:
        'L’utilisateur dont un appareil vu pour la première fois montre les bibliothèques. Avec « Personne », un nouvel appareil voit un serveur vide jusqu’à ce que vous lui choisissiez un utilisateur ci-dessous.',
      noOne: 'Personne',
      markPlayed: 'Marquer comme vu ce que lisent les appareils DLNA',
      markPlayedHelp:
        'Les appareils ne donnent pas leur position : un titre compte comme vu une fois que l’appareil en a lu plus que « Marqué comme vu après (%) ». Les appareils lisent en avance : un titre peut donc être marqué trop tôt.',
      devices: 'Appareils',
      devicesEmpty: 'Aucun appareil n’a encore rien demandé.',
      device: 'Appareil',
      address: 'Adresse',
      model: 'Modèle',
      lastSeen: 'Vu pour la dernière fois',
      showsLibrariesOf: 'Montre les bibliothèques de',
      blocked: 'Bloqué',
      profile: 'Profil',
      automaticProfile: (name: string) => `Automatique (${name})`,
      forget: 'Oublier',
      profiles: 'Profils',
      profilesHelp:
        'Les profils suivent le format XML du plugin DLNA de Jellyfin. Un appareil reçoit le premier qui correspond à ce qu’il envoie, les profils ajoutés d’abord, sinon le profil générique.',
      builtin: 'Intégré',
      addProfile: 'Ajouter un profil',
      replaceProfile: 'Remplacer',
      removeProfile: 'Supprimer',
      profileXml: 'XML du profil',
      devicesHelp:
        'Les appareils du réseau local qui ont demandé quelque chose à Polyfin, le dernier vu en premier, chacun reconnu à son adresse. Les changements s’appliquent tout de suite.',
      devicesLabel: 'Appareils DLNA',
      automatic: 'Automatique',
      deviceSaved: (name: string) => `${name} enregistré.`,
      forgetTitle: (name: string) => `Oublier ${name} ?`,
      forgetBody:
        'Polyfin oublie l’utilisateur, le blocage et le profil choisis pour lui. S’il demande à nouveau quelque chose, c’est un nouvel appareil, qui montre les bibliothèques de l’utilisateur choisi pour les nouveaux appareils.',
      forgotten: (name: string) => `${name} oublié.`,
      profilesLabel: 'Profils DLNA',
      added: 'Ajouté',
      viewXml: 'Voir le XML',
      profileXmlHelp:
        'Collez le XML d’un profil fait pour le plugin DLNA de Jellyfin, ou ouvrez son fichier.',
      openFile: 'Ouvrir un fichier',
      add: 'Ajouter',
      adding: 'Ajout…',
      replacing: 'Remplacement…',
      profileAdded: (name: string) => `Profil ${name} ajouté.`,
      profileReplaced: (name: string) => `Profil ${name} remplacé.`,
      removeTitle: (name: string) => `Supprimer le profil ${name} ?`,
      removeBody:
        'Les appareils auxquels il est imposé reçoivent de nouveau un profil choisi d’après ce qu’ils envoient.',
      profileRemoved: (name: string) => `Profil ${name} supprimé.`,
    },
    metrics: 'Exposer les métriques',
    metricsHelp:
      'Sert les métriques de Polyfin à Prometheus à l’adresse /metrics : lectures, conversions, requêtes aux addons, imports, notifications, requêtes HTTP, analyses de dossiers, base de données, espace disque et processus. Elles ne nomment aucun utilisateur, titre, adresse ni appareil. Prometheus envoie le jeton des métriques avec chaque requête ; sans lui, /metrics répond 401, et tant que cette option est désactivée, 404.',
    metricsToken: 'Jeton des métriques',
    metricsTokenHelp:
      'Créé par Polyfin la première fois que les métriques sont activées. Prometheus l’envoie comme jeton Bearer : la documentation d’administration donne une configuration de collecte.',
    metricsTokenPending: 'Polyfin crée le jeton des métriques à l’enregistrement.',
    newMetricsToken: 'Créer un nouveau jeton',
    newMetricsTokenTitle: 'Créer un nouveau jeton des métriques ?',
    newMetricsTokenConfirm:
      'Le jeton actuel cesse aussitôt de fonctionner : Prometheus a besoin du nouveau pour lire les métriques.',
    newMetricsTokenMade: 'Nouveau jeton des métriques créé',
    legacyWarningTitle: 'Avertissement de sécurité',
    legacyWarning:
      'Les méthodes héritées peuvent transmettre les identifiants dans les URL, qui se retrouvent alors dans les journaux, l’historique du navigateur et les proxys. N’activez cette option que si une de vos applications ne parvient pas à se connecter autrement.',
    prepareAhead: 'Préparer la lecture à l’avance',
    prepareAheadHelp:
      'Polyfin lit le fichier dès que la page d’un titre s’ouvre, et prépare l’épisode suivant vers la fin de celui en cours, pour que la lecture démarre tout de suite. Il liste aussi à l’avance les versions des titres de « Continuer de regarder » et « À suivre », pour que leur page les affiche aussitôt. En contrepartie, vos sources reçoivent un peu plus de demandes, y compris pour les titres ouverts mais pas regardés.',
    transcoding: 'Conversion (transcodage)',
    transcodingHelp:
      'Convertit l’image et le son pour les applications qui ne savent pas lire un fichier tel quel. Si cette option est désactivée, les fichiers sont lus tels quels ou simplement présentés autrement, sans toucher à l’image ni au son. Un titre qu’une application ne peut pas lire ainsi ne démarrera pas sur cette application, comme un enregistrement converti en HEVC sur une application qui ne lit pas le HEVC.',
    analysisTimeout: 'Temps max pour analyser une version',
    analysisTimeoutHelp: (r: RangeText) =>
      `Avant une première lecture, Polyfin analyse le fichier ou la chaîne pour savoir comment le lire. Si la source ne répond pas dans ce délai, en secondes, Polyfin abandonne cette version et passe à la suivante. Un nombre plus petit passe plus vite à la suivante, mais peut abandonner des sources lentes qui auraient marché. Une chaîne est analysée 8 secondes au plus. De ${r.min} à ${r.max} secondes ; ${r.default} par défaut.`,
    versionAttempts: 'Versions essayées quand une ne marche pas',
    versionAttemptsHelp: (r: RangeText) =>
      `Quand une application lit un titre sans choisir de version, Polyfin essaie les versions dans l’ordre jusqu’à en trouver une qui marche, sans en analyser plus que ce nombre. Les versions déjà analysées sont aussi essayées, car elles ne coûtent rien. Un nombre plus grand trouve plus souvent une version qui marche, mais un titre qui ne se lit pas met plus longtemps à le dire. De ${r.min} à ${r.max} ; ${r.default} par défaut.`,
    preferDirectPlay: 'Préférer les versions lues sans conversion',
    preferDirectPlayHelp:
      'Quand une application lit un titre sans choisir de version, Polyfin prend la première version que l’application lit telle quelle ou simplement présentée autrement, plutôt que la première qui se lit tout court. La toute première lecture d’un titre peut être un peu plus lente, car plus de versions peuvent être analysées ; rien ne change une fois qu’elles sont connues.',
    nameHowVersionsPlay: 'Dire dans le nom des versions comment elles se lisent',
    nameHowVersionsPlayHelp:
      'Le nom des versions finit par la façon dont l’appli lit chacune : « (lecture directe) », « (flux direct) » quand elle est copiée sans conversion, « (converti : raison) » avec la demande de l’appli, HDR, la limite de qualité, les sous-titres, la vidéo ou l’audio pour raison, ou « (illisible ici) ». Polyfin le décide pour le profil d’appareil envoyé par l’appli à sa dernière lecture : les noms ne disent rien avant la première lecture d’un appareil ni après un redémarrage, et rien pour une version pas encore analysée ni décrite par RemuxDB. Les noms sont dans la langue des noms générés. Rien n’est lu pour cela.',
    versionOrderGroup: 'Ordre des versions',
    versionOrder: {
      label: 'Trier les versions par',
      help: 'Chaque critère regroupe les versions d’un film ou d’un épisode, et le suivant les trie dans chaque groupe ; les versions à égalité gardent l’ordre des addons. La première version est celle que lance « Lire », et les versions essayées quand elle ne marche pas viennent ensuite. Polyfin se fie à ce qu’il sait d’une version sans lire le fichier : son analyse une fois lue, sinon la description de RemuxDB, sinon les mots de ses libellés. Une version dont un critère ne sait rien passe après les autres. La langue audio préférée est celle de chaque utilisateur, réglée dans son appli, sinon la Langue audio par défaut : les versions connues dans cette langue d’abord, puis celles marquées MULTI ou DUAL, puis celles de langue inconnue. Dans Mon compte, un utilisateur peut aussi préférer une région audio, comme le français du Québec : parmi les versions dans sa langue, celles de cette région d’abord, puis celles de région inconnue. Tant que la page d’un titre est ouverte, les versions qu’elle affiche gardent leur place.',
      none: 'Aucun : les versions gardent l’ordre des addons.',
      add: 'Ajouter un critère',
      remove: (name: string) => `Retirer « ${name} »`,
      criterion: (position: number) => `Critère ${position}`,
      criteria: {
        language: 'Langue audio préférée',
        'resolution-high': 'Résolution, la plus haute d’abord',
        'resolution-low': 'Résolution, la plus basse d’abord',
        'size-large': 'Taille du fichier, le plus gros d’abord',
        'size-small': 'Taille du fichier, le plus petit d’abord',
        codec: 'Codec vidéo, dans l’ordre ci-dessous',
        'hdr-first': 'HDR et Dolby Vision d’abord',
        'hdr-last': 'HDR et Dolby Vision en dernier',
        source: 'Source, dans l’ordre ci-dessous',
      },
    },
    versionCodecOrder: {
      label: 'Ordre des codecs vidéo',
      help: 'L’ordre dans lequel le critère « Codec vidéo » range les versions.',
    },
    versionSourceOrder: {
      label: 'Ordre des sources',
      help: 'L’ordre dans lequel le critère « Source » range les versions : les addons et dossiers du serveur qui donnent des versions de films et d’épisodes. Une source ajoutée plus tard passe en dernier tant que vous ne la déplacez pas, et les addons propres aux utilisateurs viennent après les sources du serveur. Cela ne change que l’ordre des versions : les descriptions, les sous-titres et les résultats de recherche suivent l’ordre de Contenu › Sources.',
      none: 'Aucune source du serveur ne donne encore de versions de films ou d’épisodes.',
      dragHandle: (name: string) => `Déplacer ${name}`,
    },
    versionCodecs: {
      av1: 'AV1',
      hevc: 'HEVC (H.265)',
      vp9: 'VP9',
      h264: 'H.264 (AVC)',
      vc1: 'VC-1',
      mpeg4: 'MPEG-4 partie 2 (Xvid, DivX)',
      mpeg2video: 'MPEG-2',
    },
    hideVersionsLarger: 'Masquer les versions de plus de (Go, 0 = sans limite)',
    hideVersionsLargerHelp: (r: RangeText) =>
      `Les versions dont on sait qu’elles sont plus grosses ne sont listées nulle part : ni sur la page des titres, ni à la lecture, ni dans les téléchargements. Si cette règle et les suivantes masquaient toutes les versions d’un titre, aucune n’est masquée. Jusqu’à ${r.max} Go ; ${r.default} par défaut.`,
    hiddenVersionCodecs: 'Masquer ces codecs vidéo',
    hiddenVersionCodecsHelp:
      'Les versions dont on sait qu’elles sont dans ces codecs ne sont listées nulle part.',
    hiddenVersionWords: 'Masquer les versions dont les libellés contiennent',
    hiddenVersionWordsHelp: (r: RangeText) =>
      `Un mot ou une expression par ligne, cherché sans tenir compte des majuscules dans le nom, la description et le nom de fichier d’une version : pour les addons qui disent dans leurs libellés qu’un fichier n’est pas encore prêt à être lu. Jusqu’à ${r.max} lignes ; aucun par défaut.`,
    hiddenReleases: 'Masquer les sorties de mauvaise qualité',
    hiddenReleasesHelp:
      'Les versions que leur nom ou leur nom de fichier désigne comme l’un de ces genres ne sont listées nulle part. Une étiquette compte comme un mot entier, après les mots du titre lui-même, sans tenir compte des majuscules : un film nommé « Cam » n’est pas une copie caméra, ni un fichier .ts un telesync. Aucun par défaut.',
    releaseKinds: {
      cam: 'Copies caméra (CAM, HDCAM)',
      telesync: 'Telesyncs (TS, HDTS, TELESYNC)',
      telecine: 'Télécinés (TC, TELECINE)',
      screener: 'Screeners (SCR, DVDSCR)',
      early: 'Copies anticipées (R5, WORKPRINT)',
    },
    syncAddonSubtitles: 'Recaler les sous-titres des addons',
    syncAddonSubtitlesHelp:
      'Cale les sous-titres des addons sur la version lue, quand ils arrivent trop tôt, trop tard, ou défilent plus vite ou plus lentement qu’elle. À la première lecture d’une version, Polyfin repère en arrière-plan quand on y parle : grâce à un sous-titre texte du fichier, sinon dans trois passages de son audio, en lisant au plus 300 Mo du fichier d’un addon. Les versions torrent sont laissées de côté, car il faudrait les télécharger depuis leur essaim. Ensuite, un sous-titre qui colle nettement mieux décalé ou accéléré arrive recalé, son nom finissant par « (recalé) ».',
    remuxDb: 'Décrire les versions grâce à RemuxDB',
    remuxDbHelp:
      'Affiche les pistes audio, de sous-titres et vidéo d’une version avant sa première lecture, telles que RemuxDB les a trouvées dans le même fichier. Quand les détails d’un titre s’ouvrent, Polyfin interroge RemuxDB, une base communautaire, avec l’identifiant IMDb du titre, et retrouve les versions par le nom de leur fichier. La lecture analyse toujours chaque version.',
    remuxDbUrl: 'Adresse de RemuxDB',
    remuxDbUrlHelp: 'Le serveur RemuxDB que Polyfin interroge.',
    playbackHistoryGroup: 'Historique de lecture',
    playbackHistory: 'Garder un historique de lecture',
    playbackHistoryHelp:
      'Garde chaque vidéo lue, avec qui l’a lue, sur quelle appli et comment, pour Système › Statistiques et les statistiques de chacun dans Mon compte. Désactivé, plus rien n’est gardé ; ce qui l’est reste jusqu’à être trop ancien.',
    playbackHistoryDays: 'Jours de conservation de l’historique de lecture',
    playbackHistoryDaysHelp: (r: RangeText) =>
      `Les lectures plus anciennes sont supprimées. La vérification a lieu chaque jour. De ${r.min} à ${r.max}, ${r.default} par défaut.`,
    defaultLanguagesGroup: 'Langues par défaut',
    defaultAudioLanguage: 'Langue audio par défaut',
    defaultAudioLanguageHelp:
      'Pour les utilisateurs qui n’ont choisi aucune langue audio dans leur appli, Polyfin lit la piste audio dans cette langue quand une version en a une, et place d’abord les versions connues dans cette langue quand les versions sont triées par langue. Le choix d’un utilisateur l’emporte toujours, et son appli continue de l’afficher tel qu’il l’a laissé. Cette langue passe avant la piste que le fichier marque par défaut, même pour les utilisateurs dont l’appli lit la piste par défaut quelle que soit sa langue, un réglage activé tant qu’ils ne le désactivent pas ; un fichier sans elle lit sa piste marquée. Aucune par défaut.',
    defaultSubtitleLanguage: 'Langue des sous-titres par défaut',
    defaultSubtitleLanguageHelp:
      'Pour les utilisateurs qui n’ont choisi aucune langue de sous-titres dans leur appli, Polyfin affiche des sous-titres dans cette langue selon le mode de sous-titres de chacun. Le choix d’un utilisateur l’emporte toujours, et son appli continue de l’afficher tel qu’il l’a laissé. Aucune par défaut.',
    noDefaultLanguage: 'Aucune',
    publicAddress: 'Adresse publique',
    publicAddressHelp:
      'L’adresse à laquelle on ouvre Polyfin, comme https://media.example.org. Les liens des notifications commencent par elle ; vide, les messages n’ont pas de lien. Quand elle commence par https://, « Rediriger HTTP vers HTTPS », dans Paramètres › Sécurité, mène à son port, 443 quand elle n’en indique pas.',
    email: 'E-mail',
    emailHelp:
      'Les cibles e-mail passent par ce serveur SMTP. On peut les ajouter une fois un serveur et une adresse d’expéditeur enregistrés.',
    smtpHost: 'Serveur SMTP',
    smtpHostHelp:
      'Son nom d’hôte, comme smtp.example.org. Vide, les cibles e-mail ne peuvent pas être ajoutées.',
    smtpPort: 'Port SMTP',
    smtpPortHelp: 'En général 587 avec STARTTLS, et 465 avec TLS dès la connexion.',
    smtpSecurity: 'Chiffrement SMTP',
    smtpSecurityHelp:
      'Comment la connexion est chiffrée. Avec STARTTLS, rien n’est envoyé si le serveur ne le propose pas. Sans chiffrement, le mot de passe n’est envoyé qu’à cette machine.',
    smtpSecurities: { starttls: 'STARTTLS', tls: 'TLS dès la connexion', none: 'Aucun' },
    smtpUser: 'Utilisateur SMTP',
    smtpUserHelp: 'Vide pour envoyer sans se connecter.',
    smtpPassword: 'Mot de passe SMTP',
    smtpPasswordHelp: 'Il est gardé chiffré et n’est plus jamais affiché.',
    smtpFrom: 'Adresse de l’expéditeur',
    smtpFromHelp: 'L’adresse d’où viennent les messages, comme polyfin@example.org.',
    smtpFromName: 'Nom de l’expéditeur',
    smtpFromNameHelp: 'Affiché avec l’adresse. Vide, c’est le nom du serveur.',
    weeklySummary: 'Résumé de la semaine',
    weeklySummaryHelp:
      'Les cibles qui ont choisi « Résumé de la semaine » dans leurs Événements le reçoivent une fois par semaine : les cibles du serveur et des administrateurs la semaine du serveur, les autres leur propre semaine. Rien n’est envoyé pour une semaine sans rien à dire. Système › Planning l’envoie tout de suite.',
    weeklySummaryDay: 'Envoyer le résumé de la semaine le',
    weeklySummaryDayHelp: (day: string) => `${day} par défaut.`,
    weeklySummaryHour: 'Envoyer le résumé de la semaine à',
    weeklySummaryHourHelp: (hour: string) =>
      `Dans le fuseau horaire du serveur. Les bilans du mois et de l’année sont aussi envoyés à cette heure, le 1er. ${hour} par défaut.`,
    maxConversions: 'Conversions vidéo en même temps (0 = pas de limite)',
    maxConversionsHelp: (r: RangeText) =>
      `Convertir l’image est le travail le plus lourd du serveur. Quand ce nombre de lectures avec image convertie est atteint (sous-titres incrustés dans l’image compris), une nouvelle lecture prend une version qui n’a pas besoin de conversion, ou ne démarre pas. Les lectures en cours ne sont jamais coupées. La TV en direct garde aussi ses propres limites : 4 chaînes par utilisateur et 16 pour le serveur. De ${r.min} à ${r.max} ; ${r.default} par défaut.`,
    maxConversionHeight: 'Qualité max des vidéos converties',
    maxConversionHeightHelp:
      'Les vidéos converties sont réduites à cette hauteur au plus, sans être déformées, pour passer mieux sur une connexion lente. Les fichiers lus tels quels ou simplement présentés autrement gardent leur qualité. La carte graphique convertit jusqu’en 4K ; le processeur s’arrête à 1080p, et quand il convertit du HDR en SDR, à « Qualité max du HDR converti par le processeur », sous Conversion (transcodage) › HDR.',
    conversionHeightOriginal: 'Originale',
    conversionHeight: (height: number) => `${height}p`,
    convertedDownloads: 'Téléchargements convertis',
    convertedDownloadsHelp:
      'Les applis qui le demandent, comme Streamyfin, enregistrent un titre dans la qualité choisie par l’utilisateur : Polyfin envoie une copie plus légère, faite pendant que l’appli la télécharge et jamais gardée sur le serveur. Sinon, elles enregistrent le fichier tel quel. Chaque téléchargement compte dans « Conversions vidéo en même temps » quand il convertit, et comme une des « Lectures en même temps » de l’utilisateur. L’utilisateur doit pouvoir télécharger, et utiliser la conversion (transcodage) quand l’image ou le son sont convertis. Un téléchargement interrompu recommence du début. Désactivé par défaut.',
    conversion: {
      description:
        'Quand une application ne sait pas lire un fichier tel quel, Polyfin le convertit avec FFmpeg. Ces réglages disent comment, pour les films, les séries et la TV en direct. Les valeurs par défaut conviennent à la plupart des serveurs.',
      groups: {
        general: 'Général',
        subtitles: 'Sous-titres en images',
        gpu: 'Carte graphique',
        video: 'Image',
        hdr: 'HDR',
        interlaced: 'Vidéo entrelacée',
        audio: 'Son',
        performance: 'Performances',
      },
      subtitleOcr: 'Lire en texte les sous-titres en images',
      subtitleOcrHelp:
        'Les sous-titres PGS et de DVD (VobSub) sont des images, et beaucoup d’applications ne prennent que du texte : pour elles, Polyfin incruste la piste dans l’image, ce qui demande une conversion. Activé, Tesseract lit une fois chaque piste en images, en arrière-plan, pour en faire une copie en texte, listée comme une piste de plus marquée « (OCR) », qui se lit sans conversion. Tant que la copie n’est pas prête, la piste est incrustée comme avant. La piste en images reste : l’italique, les couleurs et les positions sont perdus, et des polices travaillées peuvent être mal lues. « À la lecture » lit les pistes d’une version la première fois qu’une application qui ne sait pas les afficher la lit ; « Aussi après les analyses des bibliothèques » lit aussi les pistes des nouveaux fichiers locaux. Désactivé par défaut.',
      subtitleOcrChoices: {
        off: 'Désactivé',
        played: 'À la lecture',
        scans: 'Aussi après les analyses des bibliothèques',
      },
      subtitleOcrTracks: 'Pistes de sous-titres en images lues en même temps',
      subtitleOcrTracksHelp: (r: RangeText) =>
        `Chaque piste est lue par un processus Tesseract, sur un cœur du processeur. De ${r.min} à ${r.max} ; ${r.default} par défaut.`,
      hardwareAcceleration: 'Carte graphique pour convertir',
      hardwareAccelerationHelp:
        'Une carte graphique (GPU) convertit la vidéo bien plus vite que le processeur. Chaque système propose les siennes : NVIDIA et VAAPI sous Linux, VideoToolbox sous macOS, NVIDIA, Intel et AMD sous Windows. Sans carte qui fonctionne, le processeur convertit. Un changement s’applique dès l’enregistrement, sans redémarrer.',
      vaapiDevice: 'Carte graphique pour VAAPI',
      vaapiDeviceHelp:
        'Le nœud de rendu sur lequel VAAPI convertit la vidéo, quand plusieurs cartes graphiques le pourraient. Chacune à son tour par défaut, la première qui fonctionne.',
      vaapiDeviceEach: 'Chacune à son tour',
      vaapiDeviceMissing: (path: string) => `${path} (introuvable)`,
      hardware: {
        auto: 'Automatique : la première qui fonctionne, dans l’ordre ci-dessous',
        nvenc: 'NVIDIA (NVENC)',
        vaapi: 'AMD ou Intel (VAAPI)',
        videotoolbox: 'VideoToolbox',
        qsv: 'Intel (QSV)',
        amf: 'AMD (AMF)',
        none: 'Aucune : le processeur seul',
      },
      detected: 'Trouvé sur ce serveur',
      noGpu: 'Aucune carte graphique n’est utilisée : le processeur convertit la vidéo.',
      gpu: 'Carte graphique',
      gpuNames: {
        cuda: 'NVIDIA',
        vaapi: 'AMD ou Intel (VAAPI)',
        videotoolbox: 'VideoToolbox',
        qsv: 'Intel (QSV)',
        amf: 'AMD (AMF)',
      } as Record<string, string>,
      encoders: 'Encodeurs de la carte',
      gpuToneMapping: 'HDR vers SDR sur la carte',
      processorToneMappingHint:
        'Le HDR est alors converti par le processeur, jusqu’à « Qualité max du HDR converti par le processeur ».',
      qualityFactor: 'Accepte un niveau de qualité',
      processor: 'Encodeurs du processeur',
      processorToneMapping: 'HDR vers SDR sur le processeur',
      hdrPath: {
        term: 'Titres HDR',
        gpu: 'Convertis en SDR sur la carte',
        gpuUpTo: (height: number) => `Convertis en SDR sur la carte, jusqu’en ${height}p`,
        processor: 'Convertis en SDR sur le processeur, encodés sur la carte',
        notTimed: 'La vitesse de la carte n’est pas encore mesurée, ou n’a pas pu l’être.',
        keepsUp: (gpu: string) => `La carte convertit le HDR à ${gpu} la vitesse de lecture.`,
        processorFaster: (gpu: string, processor: string) =>
          `La carte convertit le HDR à ${gpu} la vitesse de lecture, trop lentement ; le processeur suit, à ${processor}.`,
        lowerHeight: (gpu: string, low: string, processor: string | null) =>
          `La carte convertit le HDR en 1080p à ${gpu} la vitesse de lecture, trop lentement, et en 720p à ${low}${processor ? ` ; le processeur à ${processor}, trop lentement lui aussi` : ''}.`,
        noneKeepsUp: (gpu: string, low: string, processor: string | null) =>
          `Aucun ne suit la lecture : la carte convertit le HDR en 1080p à ${gpu} la vitesse de lecture et en 720p à ${low}${processor ? `, le processeur à ${processor}` : ''}. Les titres HDR peuvent s’interrompre pendant leur conversion.`,
        turnedOff: 'Le HDR vers SDR sur la carte est désactivé dans les paramètres.',
      },
      ffmpeg: 'FFmpeg',
      ffmpegShipped: 'Celui que Polyfin fournit',
      ffmpegOlderDrivers: 'Pour les pilotes NVIDIA antérieurs à 610',
      ffmpegOlderDownloading:
        'Téléchargement du FFmpeg pour les pilotes NVIDIA antérieurs à 610, dont NVENC a besoin ici : le processeur convertit la vidéo en attendant.',
      ffmpegOlderFailed: (error: string) =>
        `Le FFmpeg pour les pilotes NVIDIA antérieurs à 610, dont NVENC a besoin ici, n’a pas pu être téléchargé : ${error}`,
      ffmpegOlderUnusable: (error: string) =>
        `NVENC n’encode pas non plus avec le FFmpeg pour les pilotes NVIDIA antérieurs à 610 : ${error}`,
      yes: 'Oui',
      no: 'Non',
      none: 'Aucun',
      hardwareDecoding: 'Lire ces formats avec la carte graphique',
      hardwareDecodingHelp:
        'La carte lit (décode) elle-même ces formats, ce qui laisse le processeur libre. Décochez-en un si ses vidéos se convertissent mal ou pas du tout : le processeur le lira alors. Les autres formats sont tentés sur la carte. HEVC 10 bits a besoin de HEVC.',
      hardwareDecodingNoGpu:
        'Aucune carte graphique n’est utilisée : le processeur lit tous les formats.',
      codecs: {
        h264: 'H.264',
        hevc: 'HEVC',
        hevc_10bit: 'HEVC 10 bits',
        vp9: 'VP9',
        av1: 'AV1',
        mpeg2video: 'MPEG-2',
        vc1: 'VC-1',
      },
      encoderPreset: 'Vitesse d’encodage',
      encoderPresetHelp:
        'Plus lent donne une meilleure image pour la même taille, mais demande plus de puissance. Si les vidéos converties saccadent, choisissez plus rapide. Automatique garde le choix de Polyfin : très rapide sur le processeur, moyen sur les cartes NVIDIA, celui de la carte sur les autres.',
      presets: {
        auto: 'Automatique',
        veryslow: 'Très lent (meilleure image)',
        slower: 'Plus lent',
        slow: 'Lent',
        medium: 'Moyen',
        fast: 'Rapide',
        faster: 'Plus rapide',
        veryfast: 'Très rapide',
        superfast: 'Super rapide',
        ultrafast: 'Ultra rapide (travail le plus léger)',
      },
      h264Quality: 'Qualité H.264 (0 = selon le débit)',
      hevcQuality: 'Qualité HEVC (0 = selon le débit)',
      qualityHelp: (r: RangeText) =>
        `Un nombre plus petit donne une meilleure image, mais plus de données ; le débit reste le maximum. 0 vise seulement le débit. Jellyfin utilise 23 pour H.264 et 28 pour HEVC. 0, ou de ${r.min} à ${r.max} ; ${r.default} par défaut.`,
      qualityIgnored:
        'Cette carte graphique n’accepte pas de niveau de qualité : elle continue de viser le débit.',
      allowHevcEncoding: 'Autoriser la conversion en HEVC',
      allowHevcEncodingHelp:
        'HEVC demande moins de données que H.264 pour la même image, mais plus de puissance pour le créer. Si cette option est activée, les applications qui citent HEVC en premier le reçoivent. Sinon, seules les applications qui ne lisent pas H.264 reçoivent du HEVC.',
      noHevcEncoder: 'FFmpeg ne sait pas créer de HEVC sur ce serveur.',
      adaptiveQuality: 'Plusieurs qualités lors de la conversion',
      adaptiveQualityHelp:
        'Les vidéos converties proposent aussi jusqu’à deux qualités inférieures, comme 720p et 480p sous 1080p. Les applications qui savent changer de qualité y passent quand leur connexion ralentit, au lieu de s’arrêter pour charger. Une qualité que l’application quitte reste convertie 10 secondes, pour qu’y revenir ne reparte pas de zéro, et toutes comptent pour une seule conversion. Les vidéos copiées, la TV en direct et la musique gardent une seule qualité. Le « réseau local » est celui des codes PIN de réinitialisation du mot de passe, à travers les proxys connus. Désactivé par défaut.',
      adaptiveQualities: {
        off: 'Désactivé',
        remote: 'En dehors du réseau local',
        always: 'Toujours',
      },
      toneMapping: 'Convertir le HDR en SDR (tone mapping)',
      toneMappingHelp:
        'Garde les bonnes couleurs et la bonne luminosité des vidéos HDR sur les écrans qui n’affichent que le SDR. Si cette option est désactivée, les vidéos HDR converties paraissent ternes, et les vidéos Dolby Vision sans couche HDR10 ne sont pas converties.',
      toneMappingUnavailable:
        'Ni la carte graphique ni FFmpeg ne savent le faire sur ce serveur : les vidéos HDR ne sont converties que si cette option est désactivée.',
      toneMappingAlgorithm: 'Méthode de conversion HDR',
      toneMappingAlgorithmHelp:
        'La façon de ramener les parties claires au SDR. Automatique utilise BT.2390 sur les cartes NVIDIA et Hable sur le processeur. Les cartes Intel et VideoToolbox ont leur propre méthode et ignorent ce choix.',
      algorithms: {
        auto: 'Automatique',
        bt2390: 'BT.2390 (cartes NVIDIA seulement)',
        hable: 'Hable',
        reinhard: 'Reinhard',
        mobius: 'Möbius',
        clip: 'Écrêtage',
        linear: 'Linéaire',
      },
      gpuToneMappingSetting: 'Convertir le HDR sur la carte graphique',
      gpuToneMappingSettingHelp:
        'Les cartes NVIDIA, les cartes Intel (par VAAPI sous Linux, QSV sous Windows) et VideoToolbox sous macOS convertissent elles-mêmes le HDR en SDR, jusqu’en 4K. Désactivez cette option si leurs couleurs semblent fausses : le processeur s’en charge alors, en qualité moindre. Les cartes Intel ne prennent que les vidéos HDR10 qui portent les informations de leur écran de mastering, et VideoToolbox le HDR10 et le HLG ; le processeur convertit les autres, et le Dolby Vision sans couche HDR10 n’est converti que sur les cartes NVIDIA. Les cartes AMD ne le font jamais : leur pilote Linux a échoué ou s’est bloqué à chaque façon essayée par Polyfin, et le filtre AMF de FFmpeg sous Windows ne convertit pas le HDR.',
      gpuToneMappingUnavailable:
        'La carte graphique de ce serveur ne sait pas le faire : le processeur convertit le HDR en SDR.',
      processorToneMappingHeight: 'Qualité max du HDR converti par le processeur',
      processorToneMappingHeightHelp:
        'Convertir le HDR en SDR est un gros travail pour le processeur : environ quatre fois l’encodage lui-même en 1080p. Automatique mesure ce serveur au démarrage : 1080p s’il convertit au moins une fois et demie plus vite que la lecture, sinon 720p, utilisé aussi tant que la mesure n’est pas finie. Au-delà de 1080p, le choix ne compte que si une carte graphique encode la vidéo : seul, le processeur s’arrête à 1080p.',
      toneMappingHeightAuto: 'Automatique (mesuré au démarrage)',
      height4k: '4K',
      measuredHeight: (height: number) => `Mesuré : ${height}p`,
      upToHeight: (height: number) => `jusqu’en ${height}p`,
      toneMappingPeak: 'Luminosité max en nits (0 = celle de la vidéo)',
      toneMappingDesat: 'Désaturation des zones claires (0 = aucune)',
      toneMappingPeakHelp: (peak: RangeText, desat: RangeText) =>
        `Seulement quand le processeur convertit le HDR en SDR : une carte graphique qui le fait les ignore. La luminosité max remplace le niveau le plus clair annoncé par la vidéo : 0, ou de ${peak.min} à ${peak.max} ; ${peak.default} par défaut. La désaturation atténue la couleur des parties très claires : de ${desat.min} à ${desat.max} ; ${desat.default} par défaut.`,
      processorCannotToneMap:
        'FFmpeg ne sait pas convertir le HDR sur le processeur de ce serveur.',
      deinterlaceMethod: 'Méthode de désentrelacement',
      deinterlaceMethodHelp:
        'Les émissions de télé et les DVD sont souvent entrelacés, ce qui fait apparaître des lignes en peigne une fois converti. Yadif est rapide ; Bwdif est un peu plus net.',
      noBwdif: 'FFmpeg n’a pas Bwdif sur ce serveur.',
      deinterlacers: { yadif: 'Yadif', bwdif: 'Bwdif' },
      deinterlaceDoubleRate: 'Doubler le nombre d’images par seconde',
      deinterlaceDoubleRateHelp:
        'Crée une image à partir de chaque demi-image, pour des mouvements plus fluides dans le sport et les émissions de télé. Seulement pour les vidéos jusqu’à 30 images par seconde.',
      downmixAlgorithm: 'Mixage en stéréo',
      downmixAlgorithmHelp:
        'La façon de ramener le son multicanal sur deux enceintes. Dave750 et le mode nuit gardent les voix claires ; RFC 7845 et AC-4 suivent des normes.',
      downmixes: {
        None: 'Le mixage de FFmpeg',
        Dave750: 'Dave750',
        NightmodeDialogue: 'Mode nuit (voix plus claires)',
        Rfc7845: 'RFC 7845',
        Ac4: 'AC-4',
      },
      downmixBoost: 'Volume du mixage stéréo',
      downmixBoostHelp: (r: RangeText) =>
        `Un mixage en stéréo sonne souvent moins fort : le volume est multiplié par ce nombre. De ${r.min} à ${r.max} ; ${r.default} (aucun changement) par défaut. Jellyfin utilise 2.`,
      maxAudioChannels: 'Nombre max de canaux audio',
      maxAudioChannelsHelp:
        'Le son converti garde au plus ce nombre de canaux, même si l’application en accepte plus.',
      audioChannels: (channels: number) =>
        channels === 0
          ? 'Autant que l’application en accepte'
          : channels === 1
            ? 'Mono'
            : channels === 2
              ? 'Stéréo'
              : '5.1',
      audioBitratePerChannel: 'Débit audio par canal en kb/s (0 = automatique)',
      audioBitratePerChannelHelp: (r: RangeText) =>
        `Automatique donne les débits de Jellyfin : 128 kb/s en mono, 384 kb/s en stéréo et 640 kb/s en 5.1, moins sous une limite de débit basse. 0, ou de ${r.min} à ${r.max} ; ${r.default} par défaut.`,
      encodingThreads: 'Threads du processeur par conversion (0 = automatique)',
      encodingThreadsHelp: (r: RangeText) =>
        `Limite la part du processeur qu’une conversion utilise, pour laisser de la place au reste. De ${r.min} à ${r.max} ; 0 laisse FFmpeg choisir.`,
      aheadSeconds: 'Secondes préparées à l’avance',
      aheadSecondsHelp: (r: RangeText) =>
        `Polyfin prépare une vidéo au plus ce nombre de secondes au-delà de la partie demandée par l’application, puis attend. Plus aide avec les sources lentes et garde leur connexion occupée, mais utilise plus de puissance et de disque quand on arrête de regarder tôt. De ${r.min} à ${r.max} ; ${r.default} par défaut.`,
      keepConvertedVideo: 'Garder la vidéo convertie quand la piste audio change',
      keepConvertedVideoHelp:
        'Convertit l’image et le son à part : choisir une autre piste audio dans le lecteur ne prépare que le nouveau son, l’image déjà convertie continue, et la nouvelle piste démarre plus vite. L’image est gardée 30 secondes après l’arrêt du lecteur, et compte pendant ce temps pour une conversion.',
    },
    catalogsTitle: 'Catalogues',
    catalogLimit: 'Titres lus par catalogue de films et séries',
    catalogLimitHelp: (r: RangeText) =>
      `Certains catalogues sont presque sans fin : Polyfin arrête donc de lire un catalogue après ce nombre de titres. Un nombre plus élevé affiche plus de titres, mais les listes se chargent plus lentement et l’addon reçoit plus de demandes. De ${r.min} à ${r.max} ; ${r.default} par défaut.`,
    channelLimit: 'Chaînes lues par catalogue de TV en direct',
    channelLimitHelp: (r: RangeText) =>
      `Polyfin arrête de lire un catalogue de TV en direct après ce nombre de chaînes, et lit au plus ce nombre de programmes par jour pour le guide. Un nombre plus élevé affiche plus de contenu, mais se charge plus lentement et l’addon reçoit plus de demandes. De ${r.min} à ${r.max} ; ${r.default} par défaut.`,
    skipButtons: 'Boutons « Passer l’intro » et « Passer le générique »',
    skipButtonsHelp:
      'Les applications proposent de passer les intros, les résumés et les génériques, repérés dans des bases de données communautaires. Si cette option est désactivée, les applications n’affichent pas ces boutons et ces bases ne sont pas consultées.',
    publicMetaDbKey: 'Clé PublicMetaDB',
    publicMetaDbKeyHelp:
      'Facultatif. Une clé d’API PublicMetaDB ajoute PublicMetaDB aux sources de repères pour passer l’intro et le générique.',
    theIntroDbKey: 'Clé TheIntroDB (facultative)',
    theIntroDbKeyHelp:
      'Relève la limite quotidienne de TheIntroDB et inclut vos propres contributions. La lecture fonctionne sans elle.',
    segmentSources: {
      label: 'Sources de repères',
      help: 'Pour chaque type de passage (intro, résumé, générique, aperçu), la première source activée de cette liste qui le connaît l’emporte ; les suivantes complètent. Les sources désactivées ne sont jamais consultées.',
      turnOn: (name: string) => `Consulter ${name}`,
      needsKey: 'Clé requise',
    },
    audioIntros: 'Trouver les intros et les génériques dans l’audio des épisodes locaux',
    audioIntrosHelp:
      'Quand un épisode d’un dossier local est lu sans intro ou sans générique donnés par ses chapitres ou par les sources ci-dessus, Polyfin compare en arrière-plan l’audio des épisodes de sa saison, et donne à chacun le passage qu’ils partagent près de leur début comme intro, et celui qu’ils partagent près de leur fin comme générique. Les autres épisodes de la saison ont alors les leurs, et celui qui a été lu les a dès sa prochaine lecture. Nécessite les boutons ci-dessus.',
    similarTitles: 'Titres similaires',
    similarTitlesHelp:
      'La page d’un titre montre des titres proches, trouvés dans les catalogues des addons. Si cette option est désactivée, la liste est vide et les addons reçoivent moins de demandes.',
    generateImages: 'Créer une image pour les collections qui n’en ont pas',
    generateImagesHelp:
      'Une collection d’un addon sans affiche, et une liste intelligente affichée comme une collection, montrent les affiches de leurs premiers titres avec leur nom, dessinées par Polyfin une fois leurs titres lus. Les utilisateurs sous contrôle parental ou qui bloquent des genres ne les voient pas. Si cette option est désactivée, elles n’ont pas d’image. Les bibliothèques réglées sur une image générée la gardent.',
    tmdbKey: 'Clé d’API TMDB',
    tmdbKeyHelp:
      'Facultatif. Avec une clé d’API ou un jeton d’accès en lecture d’un compte TMDB, « Modifier les images » de jellyfin-web cherche aussi sur TMDB les affiches, fonds et logos d’un titre, les images d’un épisode, les affiches d’une saison et les photos d’une personne, que Polyfin relaie. Les personnes sont aussi cherchées une fois sur TMDB : celles sans photo reçoivent la leur, et leurs pages montrent leur biographie, leurs dates de naissance et de décès et leur lieu de naissance. Les fichiers d’un dossier local dont la recherche échoue auprès de tous les addons de métadonnées sont aussi cherchés sur TMDB. Sans clé, « Modifier les images » ne montre que les images des addons et TMDB n’est jamais consulté.',
    mdblistRatingsKey: 'Clé d’API MDBList des notes',
    mdblistRatingsKeyHelp:
      'Facultatif. Avec la clé d’un compte MDBList, les films et les séries affichent leur note IMDb, le score des critiques et celui du public donnés par MDBList, cherchés en arrière-plan et gardés pour tous les utilisateurs. Les clés MDBList des utilisateurs pour le suivi ne servent jamais à cela. Sans clé, MDBList n’est jamais consulté.',
    criticRatingShows: 'La note critique affiche',
    criticRatingShowsHelp:
      'Les applications Jellyfin n’ont qu’un champ pour une note critique, de 0 à 100 : il affiche le score des critiques ou celui du public. Sur la fiche d’un titre, le lecteur web affiche les deux, avec la note IMDb.',
    criticRatingChoices: { critics: 'Score des critiques', audience: 'Score du public' },
    posterBadges: 'Badges sur les affiches',
    posterBadgesHelp:
      'Dessine sur l’affiche d’un film ou d’une série sa note en étoiles, en nommant IMDb quand c’est la note IMDb de MDBList, les scores des critiques et du public tant qu’une clé d’API MDBList des notes est définie, et pour une série un nouvel épisode ou une nouvelle saison des 30 derniers jours. Toutes les applications les affichent, puisqu’ils font partie de l’affiche, avec des icônes et des chiffres seulement, pour toutes les langues. Activé, le lecteur web n’affiche pas les notes au survol. Désactivé, les applications reçoivent les affiches telles quelles.',
    posterBadgeSize: 'Taille des badges',
    posterBadgeSizeHelp:
      'La taille des badges selon la largeur de l’affiche. Sur les petites cartes, ils gardent une taille minimale, et moins d’entre eux y tiennent.',
    posterBadgeSizeChoices: { medium: 'Moyenne', small: 'Petite', large: 'Grande' },
    posterPreview: {
      title: 'Aperçu',
      help: 'Des titres de vos bibliothèques tels que les applications les affichent, à la taille choisie, avant de l’enregistrer, que les badges soient activés ou non.',
      original: 'Telle quelle',
      badged: 'Avec les badges',
      card: 'Sur une petite carte',
      another: 'Un autre titre',
      none: 'Aucun film ni aucune série de vos bibliothèques n’est encore connu : ouvrez d’abord une bibliothèque dans le lecteur web.',
      nothing: 'Ce titre n’a ni note ni nouvel épisode : son affiche ne montre aucun badge.',
    },
    posterTemplate: 'Modèle d’adresse des affiches',
    posterTemplateHelp:
      'Facultatif. L’adresse d’où sont téléchargées les affiches des films et des séries à la place de celles des addons, avec {imdb_id}, {tmdb_id}, {type}, movie ou series, et {tmdb_type}, movie ou tv, remplis. Un titre sans un identifiant que nomme l’adresse, ou dont l’affiche ne peut pas y être téléchargée, garde celle de son addon. Les affiches des dossiers locaux et celles téléversées passent toujours en premier.',
    lyrics: 'Paroles des chansons depuis LRCLIB',
    lyricsHelp:
      'Les applications affichent les paroles des chansons, synchronisées ligne par ligne quand LRCLIB, une base de paroles libre, les connaît. Polyfin recherche une chanson une seule fois, par son artiste, son titre, son album et sa durée, quand sa lecture commence ou que sa fiche ou ses paroles s’ouvrent. Si cette option est désactivée, les chansons n’ont pas de paroles et LRCLIB n’est jamais consulté.',
    evenVolume: 'Égaliser le volume entre les chansons',
    evenVolumeHelp:
      'Les applications montent ou baissent chaque chanson, ou chaque album, au même volume, d’après les balises ReplayGain des chansons locales ou leur volume mesuré. Les chansons elles-mêmes ne changent pas. Si cette option est désactivée, les applications ne reçoivent aucun gain et rien n’est mesuré.',
    measureLoudness: 'Mesurer le volume des chansons sans gain',
    measureLoudnessHelp:
      'FFmpeg mesure le volume des chansons sans balise ReplayGain, deux à la fois à la priorité la plus basse, une seule fois chacune : les chansons locales après une analyse, celles des addons après leur première lecture. Les chansons de plus de 15 minutes et les livres audio ne sont jamais mesurés.',
    measureLoudnessChoices: {
      off: 'Non',
      local: 'Chansons locales',
      all: 'Chansons locales et des addons',
    },
    playedPercent: 'Marqué comme vu après (%)',
    playedPercentHelp: (r: RangeText) =>
      `Un titre est marqué comme vu dès que la lecture dépasse cette part de sa durée. De ${r.min} à ${r.max} ; ${r.default} par défaut, comme dans Jellyfin.`,
    resumePercent: 'Point de reprise gardé après (%)',
    resumePercentHelp: (r: RangeText) =>
      `L’endroit où la lecture s’est arrêtée est gardé, pour reprendre de là, dès qu’il dépasse cette part de la durée du titre. Ce nombre doit être plus petit que celui de « Marqué comme vu après ». De ${r.min} à ${r.max} ; ${r.default} par défaut, comme dans Jellyfin. Un titre de moins de 5 minutes est marqué comme vu dès qu’il dépasse ce point.`,
    versionListMinutes: 'Garder les listes de versions pendant (minutes)',
    versionListMinutesHelp: (r: RangeText) =>
      `Combien de temps Polyfin utilise les versions et les sous-titres que les addons donnent pour un titre avant de leur redemander. Une liste plus ancienne s’affiche quand même tout de suite quand on rouvre le titre, le temps que Polyfin redemande. Les garder plus longtemps envoie moins de demandes à l’addon qui donne les versions, ce qui aide avec les fournisseurs qui refusent trop de demandes, mais les nouvelles versions apparaissent plus tard. De ${r.min} à ${r.max} ; ${r.default} par défaut.`,
    catalogRefreshMinutes: 'Rafraîchir les catalogues après (minutes)',
    catalogRefreshMinutesHelp: (r: RangeText) =>
      `L’âge qu’une page de catalogue lue auprès d’un addon, guide de la TV en direct compris, peut atteindre avant que Polyfin la relise. Les applis ne l’attendent jamais : une page plus ancienne s’affiche quand même tout de suite pendant que Polyfin la relit en arrière-plan, et elle est gardée un jour de plus. Une durée plus longue envoie moins de demandes aux addons, mais les nouveaux titres apparaissent plus tard. De ${r.min} à ${r.max} (un jour) ; ${r.default} par défaut.`,
    collectionReadHour: 'Lire toutes les collections chaque jour',
    collectionReadHourNever: 'Jamais',
    collectionReadHourHelp:
      'Lit toutes les collections des bibliothèques de collections du serveur à cette heure, l’une après l’autre, puis les catalogues des listes intelligentes du serveur, pour qu’elles s’ouvrent tout de suite. Cela demande beaucoup de pages aux addons : laissez-le désactivé pour un addon que vous partagez avec d’autres.',
    collectionLibraryApps: 'Applis qui présentent les bibliothèques de collections par collection',
    collectionLibraryAppsHelp:
      'Les bibliothèques de collections sont présentées aux applis comme des bibliothèques de contenu mixte, pour que jellyfin-web et Streamyfin leur donnent une rangée sur l’accueil. Les applis cochées apprennent qu’elles contiennent des collections, comme la vue Collections de Jellyfin : les applis qui listent une bibliothèque selon son type affichent alors ses collections, puis les titres d’une collection une fois ouverte, plutôt que les titres de toutes ses collections mélangés. jellyfin-web et Streamyfin écartent ces bibliothèques des rangées de leur accueil : ne les cochez donc pas. La liste montre les applis qui se sont connectées à ce serveur.',
    collectionLibraryAppsNone: 'Aucune appli ne s’est encore connectée à ce serveur.',
    localScanHours: 'Analyser les dossiers locaux toutes les (heures, 0 = jamais)',
    localScanHoursHelp: (r: RangeText) =>
      `À quelle fréquence chaque dossier local est analysé à nouveau pour trouver les fichiers ajoutés, modifiés ou supprimés. Les dossiers sont aussi analysés au démarrage, à la demande, et quand ceux qui sont surveillés changent. 0 désactive cette planification. De ${r.min} à ${r.max} ; ${r.default} par défaut.`,
    watchlistCheckHours:
      'Chercher des versions des titres à regarder toutes les (heures, 0 = jamais)',
    watchlistCheckHoursHelp: (r: RangeText) =>
      `À quelle fréquence chaque titre non commencé de la liste « À regarder » d’un utilisateur est vérifié à nouveau pour trouver des versions, pour les rangées d’accueil Disponibles et Toujours en attente et le message qui prévient l’utilisateur d’un titre disponible. Chaque heure, jusqu’à 20 titres de chaque utilisateur sont vérifiés, en interrogeant ses addons de flux. 0 désactive ces vérifications. De ${r.min} à ${r.max} ; ${r.default} par défaut.`,
    watchLocalFolders: 'Surveiller les changements des dossiers et des partages',
    watchLocalFoldersHelp: {
      container:
        'Analyse à nouveau un dossier du conteneur quelques secondes après que ses fichiers ont cessé de changer, pour que les fichiers ajoutés, renommés ou supprimés apparaissent en moins d’une minute.',
      computer:
        'Analyse à nouveau un dossier de l’ordinateur où tourne Polyfin quelques secondes après que ses fichiers ont cessé de changer, pour que les fichiers ajoutés, renommés ou supprimés apparaissent en moins d’une minute.',
    },
    watchSharesHelp:
      'Un partage SMB est analysé à nouveau quelques secondes après que son serveur a signalé un changement. Les dossiers WebDAV sont listés de nouveau toutes les 2 minutes, les buckets S3, dossiers SFTP et remotes rclone aussi souvent que chaque dossier le demande, en plusieurs fois pour un partage de nombreux dossiers, et analysés à nouveau quand une liste a changé. Système › Santé indique comment chaque partage est surveillé, et quels dossiers ou partages ne peuvent pas l’être, et pourquoi.',
    personalAddons: 'Autoriser les addons personnels des utilisateurs',
    personalAddonsHelp:
      'Les utilisateurs peuvent ajouter leurs propres addons Stremio, en plus de ceux du serveur. Sinon, leurs addons sont conservés mais pas utilisés, et leurs applications Jellyfin n’affichent que les addons du serveur.',
    serverImports:
      'Les utilisateurs peuvent importer leur historique depuis un autre serveur ou une appli',
    serverImportsHelp:
      'Sous Mon compte, chaque utilisateur peut importer ses films et épisodes vus, ses points de reprise et ses favoris depuis un serveur Jellyfin ou Emby, ou depuis son compte Stremio ou Nuvio, en s’y connectant avec ses propres identifiants. Sinon, ces sections sont masquées. Un administrateur peut toujours importer pour un utilisateur, sous Utilisateurs.',
    importEveryMinutes: 'Importer les comptes connectés toutes les',
    importEveryMinutesHelp:
      'Tous les combien Polyfin importe de nouveau les comptes Stremio et Nuvio auxquels les utilisateurs restent connectés, et les historiques qu’ils importent depuis les services de suivi. Un changement s’applique tout de suite : chaque prochaine importation a lieu ce délai après la dernière. Chaque importation relit tout le compte : un intervalle court envoie plus de requêtes à ces services et, pour Stremio, dont les conditions interdisent l’extraction automatisée, rend cette zone grise plus marquée. 6 heures par défaut.',
    shareLinks: 'Autoriser les liens de partage',
    shareLinksHelp:
      'Les utilisateurs qui ont « Peut partager des titres », sur leur page sous Utilisateurs, peuvent donner un film ou un épisode à quelqu’un qui n’a pas de compte, grâce à un lien qui expire après 1 heure à 30 jours. L’invité regarde dans un navigateur, en 1080p au plus et dans le groupe de qualité de l’utilisateur, et compte parmi les lectures simultanées de l’utilisateur. Sinon, aucun lien ne peut être créé et tous les liens sont refusés, sans être supprimés. Désactivé par défaut.',
    importEveryChoice: (minutes: number) => (minutes < 60 ? `${minutes} min` : `${minutes / 60} h`),
    loginAttempts: 'Bloquer un compte après ce nombre de mots de passe faux (0 = jamais)',
    loginAttemptsHelp: (r: RangeText) =>
      `Après ce nombre de mots de passe faux à la suite, le compte ne peut plus se connecter pendant 15 minutes, même avec le bon mot de passe. Un administrateur peut le débloquer plus tôt depuis la page Utilisateurs. 0, ou de ${r.min} à ${r.max}.`,
    twoStepAdministrators: 'Exiger la connexion en deux étapes des administrateurs',
    twoStepAdministratorsHelp:
      'Les administrateurs se connectent à cette appli d’administration avec un code d’une application d’authentification après leur mot de passe. Celui qui ne l’a pas configurée le fait juste après son mot de passe, avant d’obtenir une session. Son mot de passe seul ne le connecte alors plus à aucune appli Jellyfin : il utilise Quick Connect, approuvé ici. Les sessions et appareils déjà connectés le restent. Désactivé par défaut.',
    inactiveDeviceDays: 'Déconnecter les appareils inutilisés depuis (jours, 0 = jamais)',
    inactiveDeviceDaysHelp: (r: RangeText) =>
      `Les applications Jellyfin qui n’ont pas servi depuis ce nombre de jours sont déconnectées et doivent se reconnecter. La vérification a lieu toutes les heures. La connexion à cette page d’administration n’est pas concernée. De ${r.min} à ${r.max}.`,
    knownProxies: 'Proxys connus',
    knownProxiesHelp: (r: RangeText) =>
      `Les proxys inverses placés devant Polyfin, un par ligne : une adresse IP, un réseau comme 172.18.0.0/16, ou un nom d’hôte comme le nom d’un conteneur sur un réseau Docker (traefik), jusqu’à ${r.max}. Un nom est résolu chaque minute et chaque adresse à laquelle il correspond est reconnue : ne listez que des noms que votre propre réseau résout. Une requête de l’un d’eux vient du client que nomme son en-tête X-Forwarded-For ou X-Real-IP : c’est cette adresse qui limite les connexions échouées, qui s’affiche pour les appareils et qui indique le réseau local. Les en-têtes de transfert venant de toute autre adresse sont ignorés. Laissez vide si rien ne se trouve devant Polyfin.`,
    provider: {
      title: 'Fournisseur d’identité',
      intro:
        'Les utilisateurs peuvent se connecter à cette application d’administration et au lecteur web avec un fournisseur OpenID Connect, qui demande alors sa propre connexion en deux étapes : Polyfin ne redemande pas de code. Les applications Jellyfin qui ne peuvent pas ouvrir le fournisseur se connectent avec Quick Connect, approuvé depuis une session ouverte par lui. Enregistrez Polyfin chez le fournisseur comme client confidentiel, ou comme client public sans secret, avec l’URI de redirection ci-dessous.',
      redirectUri: 'URI de redirection',
      redirectUriHelp:
        'L’adresse où le fournisseur renvoie les navigateurs, construite à partir de l’adresse publique. Enregistrez-la chez le fournisseur telle quelle.',
      noPublicAddress:
        'Indiquez d’abord l’adresse publique dans Paramètres › Notifications : la connexion y commence et y finit, et le fournisseur y renvoie les navigateurs.',
      issuer: 'Émetteur',
      issuerHelp:
        'L’émetteur (issuer) du fournisseur, exactement comme le donne son document de découverte, comme https://id.example.org ou https://id.example.org/realms/home. Polyfin lit son /.well-known/openid-configuration. Vide désactive la connexion.',
      name: 'Nom du bouton',
      nameHelp:
        'Ce que disent les boutons de connexion après « Se connecter avec ». Vide affiche le nom d’hôte de l’émetteur.',
      clientId: 'ID client',
      clientIdHelp:
        'L’ID que le fournisseur a donné au client de Polyfin. Vide désactive la connexion.',
      clientSecret: 'Secret client',
      clientSecretHelp:
        'Le secret d’un client confidentiel ; laissez vide pour un client public. Il est conservé chiffré avec POLYFIN_SECRET_KEY et n’est plus jamais affiché.',
      userClaim: 'Claim qui nomme l’utilisateur',
      userClaimHelp:
        'Le claim du jeton d’identité comparé aux noms des comptes, sans tenir compte de la casse, la première fois que quelqu’un se connecte : preferred_username par défaut. L’e-mail vérifié vient ensuite. Les connexions suivantes retrouvent le même compte, quoi que dise alors le claim.',
      createAccounts: 'Créer les comptes',
      createAccountsHelp:
        'Une personne qu’aucun compte ne désigne en reçoit un, nommé d’après le claim, sinon l’e-mail, avec les permissions d’un nouveau compte, qui se connecte par le fournisseur uniquement. Désactivé, elle est refusée. Désactivé par défaut.',
      adminClaim: 'Claim des administrateurs',
      adminClaimHelp:
        'Le claim qui liste les groupes ou rôles d’une personne, comme groups. Avec une valeur ci-dessous, celui dont le claim la contient devient administrateur en se connectant. Polyfin demande le scope groups quand le claim est groups et que le fournisseur le propose.',
      adminValue: 'Valeur des administrateurs',
      adminValueHelp:
        'Le groupe ou rôle qui fait les administrateurs, comme polyfin-admins. Le quitter ensuite ne refait pas d’un administrateur un membre : changez-le sur la page Utilisateurs.',
    },
    https: {
      title: 'HTTPS',
      intro:
        'Polyfin peut servir HTTPS lui-même, sur un port à part, sans proxy inverse devant lui. HTTP continue de fonctionner sur son port quoi qu’il arrive à HTTPS : un port impossible à ouvrir, ou un certificat impossible à lire ou à renouveler, s’affiche dans Système › Santé. Les changements s’appliquent tout de suite, sans redémarrage.',
      mode: 'Servir HTTPS',
      modeHelp:
        'Fichiers de certificat : un certificat qu’obtient un autre outil, relu quand il change. Let’s Encrypt : un certificat que Polyfin obtient et renouvelle lui-même, pour des noms d’hôte publics qui pointent vers ce serveur. Désactivé par défaut.',
      modes: { off: 'Désactivé', files: 'Fichiers de certificat', letsencrypt: 'Let’s Encrypt' },
      port: 'Port HTTPS',
      portHelp: (r: RangeText) =>
        `Le port TCP sur lequel HTTPS écoute dans le conteneur : publiez-le, comme 8096 l’est pour HTTP. L’utilisateur du conteneur ne peut pas écouter sous 1024 : pour joindre Polyfin sur 443, redirigez 443 vers ce port sur le routeur ou dans les ports du conteneur, et réglez l’adresse publique, dans Paramètres › Notifications, sur https:// et le nom d’hôte, sans port. De ${r.min} à ${r.max} ; ${r.default} par défaut, comme Jellyfin.`,
      redirect: 'Rediriger HTTP vers HTTPS',
      redirectHelp:
        'Les requêtes venues d’internet en HTTP reçoivent une redirection permanente (308) vers la même adresse en HTTPS, sur le port par lequel internet joint HTTPS : le port de l’adresse publique, dans Paramètres › Notifications, quand elle commence par https://, 443 quand elle n’en indique pas, sinon le port HTTPS. Le réseau local, les vérifications de Let’s Encrypt, et les requêtes qu’un proxy connu a reçues en HTTPS restent en HTTP. Ne s’applique que tant que HTTPS est servi. Désactivé par défaut.',
      certificate: 'Certificat',
      certificateHelp:
        'Le chemin dans le conteneur du fichier PEM du certificat avec sa chaîne, comme /certs/fullchain.pem. L’enregistrement vérifie qu’il peut être lu. Polyfin le relit dans la minute qui suit un changement, et garde le dernier bon certificat tant qu’un changement ne peut pas être lu.',
      privateKey: 'Clé privée',
      privateKeyHelp:
        'Le chemin dans le conteneur de la clé privée RSA ou ECDSA du certificat, en fichier PEM, comme /certs/privkey.pem. L’enregistrement vérifie qu’elle correspond au certificat.',
      hosts: 'Noms d’hôte',
      hostsHelp: (r: RangeText) =>
        `Les noms publics qui pointent vers ce serveur, un par ligne, comme media.example.org, jusqu’à ${r.max}. Un seul certificat les contient tous. Les noms résolus seulement sur le réseau local, et les jokers, demandent le défi DNS-01, auquel Polyfin ne répond pas : utilisez pour eux des fichiers de certificat obtenus par un autre outil.`,
      email: 'E-mail',
      emailHelp: 'Facultatif. Let’s Encrypt peut écrire à cette adresse au sujet du compte.',
      terms: 'J’accepte les conditions d’utilisation de Let’s Encrypt',
      termsHelp:
        'Let’s Encrypt ne délivre de certificats qu’une fois ses conditions d’utilisation, sur letsencrypt.org/repository, acceptées.',
      challenge: 'Défi',
      challengeHelp:
        'La façon dont Let’s Encrypt vérifie que les noms pointent vers ce serveur. HTTP-01 : Polyfin répond sur son port HTTP, que le port 80 d’internet doit joindre. TLS-ALPN-01 : Polyfin répond sur son port HTTPS, que le port 443 d’internet doit joindre. Le certificat, sa clé et la clé du compte sont gardés dans la base de données, les clés chiffrées avec POLYFIN_SECRET_KEY, et le certificat est renouvelé quand il lui reste un tiers de sa durée.',
      challenges: {
        'http-01': 'HTTP-01 (port 80 vers le port HTTP)',
        'tls-alpn-01': 'TLS-ALPN-01 (port 443 vers le port HTTPS)',
      },
      useAddress: (address: string) => `Utiliser ${address}`,
      offeredAddress:
        'Les adresses auxquelles Let’s Encrypt sert HTTPS, dans Paramètres › Sécurité : sans port une fois que le routeur redirige 443 vers le port HTTPS. « Rediriger HTTP vers HTTPS » mène au port de l’adresse choisie.',
    },
    detailedLog: 'Journal détaillé (pour diagnostiquer un problème)',
    detailedLogHelp:
      'Polyfin écrit beaucoup plus de détails dans son journal, tout de suite et sans redémarrage. Désactivez cette option une fois le problème trouvé.',
    saved: 'Paramètres enregistrés.',
    thumbnailsHelp:
      'Des images tirées des titres eux-mêmes, après leur visionnage. Pour les faire, Polyfin lit de petits morceaux du fichier à la source, doucement, quand personne ne regarde depuis cette source : au plus 60 demandes par titre, une toutes les 3 secondes, et 120 par heure et par source, jamais le fichier entier. Une source qui demande de ralentir ne reçoit plus de demande d’images pendant 2 heures. Désactivé par défaut.',
    trickplay: 'Miniatures quand on avance dans un titre',
    trickplayHelp:
      'Les applications montrent une petite image du moment visé dans la barre de lecture. Polyfin les fait une fois le titre regardé, et pour l’épisode suivant quand la lecture est préparée à l’avance. Les films longs ont des miniatures plus espacées : quelques minutes plutôt que quelques secondes.',
    trickplayInterval: 'Une miniature toutes les (secondes, au moins)',
    trickplayIntervalHelp: (r: RangeText) =>
      `Tous les combien la barre de lecture change d’image. Polyfin lit au plus 60 images par titre : un titre de plus de 10 minutes environ a une image par pas, ses pas répartis sur toute sa durée : environ 1 minute pour un épisode d’une heure, 2 minutes pour un film de 2 heures. De ${r.min} à ${r.max} ; ${r.default} par défaut, comme Jellyfin.`,
    trickplayWidth: 'Largeur des miniatures',
    trickplayWidthHelp:
      'Des miniatures plus larges sont plus nettes sur un grand écran, mais prennent plus de place. Un changement fait de nouvelles miniatures à la prochaine lecture d’un titre.',
    pixels: (width: number) => `${width} pixels`,
    chapterImages: 'Images des chapitres',
    chapterImagesHelp:
      'Les applications montrent une image pour chaque chapitre d’un titre, dans sa liste de scènes. Polyfin prend l’image lue la plus proche du début de chaque chapitre, avec les mêmes lectures que les miniatures, une fois le titre regardé.',
    thumbnailStorage: 'Place pour les images (Go)',
    thumbnailStorageHelp: (r: RangeText) =>
      `Les miniatures et les images des chapitres sont gardées dans la base de données. Au-delà de cette taille, les images des titres regardés il y a le plus longtemps sont effacées ; elles sont refaites si le titre est relu. De ${r.min} à ${r.max} ; ${r.default} par défaut.`,
    cacheSize: 'Espace disque des fichiers en cours de lecture (Go)',
    cacheSizeHelp: (r: RangeText) =>
      `L’espace que Polyfin peut utiliser pour garder les parties des fichiers lus, afin que les retours en arrière et les reprises ne les téléchargent pas à nouveau. Les parties lues dans les 30 dernières secondes sont gardées même au-delà. De ${r.min} à ${r.max} Go ; ${r.default} par défaut.`,
    workFolders: {
      conversionFolder: 'Dossier de conversion',
      conversionFolderHelp:
        'Où sont écrits les segments des fichiers convertis ou présentés autrement pendant leur lecture : les écritures les plus intenses, auxquelles conviennent un disque rapide ou un dossier en mémoire. Il demande la place des segments de toutes les conversions en cours.',
      readingFolder: 'Dossier des fichiers en cours de lecture',
      readingFolderHelp:
        'Où sont gardées les parties des fichiers des addons et les morceaux des torrents en cours de lecture, dans l’espace disque ci-dessus : souvent plusieurs Go, auxquels convient un grand disque.',
      cacheFolder: 'Dossier du cache',
      cacheFolderHelp:
        'Où sont gardées les images relayées, jusqu’à 1 Go, d’un démarrage à l’autre, avec les guides et les sous-titres en images en cours de lecture.',
      where: {
        container:
          'Un dossier du conteneur de Polyfin, où est monté un disque ou un dossier en mémoire ; vide, le cache de son volume de données, indiqué. Polyfin doit pouvoir y écrire. Un nouveau dossier s’applique au prochain démarrage de Polyfin ; l’ancien est laissé tel quel.',
        computer:
          'Un dossier de l’ordinateur où tourne Polyfin ; vide, le cache de son dossier de données, indiqué. Polyfin doit pouvoir y écrire. Un nouveau dossier s’applique au prochain démarrage de Polyfin ; l’ancien est laissé tel quel.',
      },
    },
    recordingsTitle: 'Enregistrements',
    recordingsFolder: (folder: string) => `Les enregistrements sont gardés dans ${folder}.`,
    recording: 'Enregistrer la TV en direct',
    recordingHelp:
      'Permet aux utilisateurs de programmer des enregistrements de la TV en direct, écrits dans le dossier ci-dessous.',
    recordingsFolderLabel: 'Dossier des enregistrements',
    recordingsFolderHelp: {
      container:
        'Un dossier du conteneur de Polyfin, celui indiqué s’il est vide, dans son volume de données. Pour garder les enregistrements sur un autre disque, montez-le à cet endroit, ou ailleurs et indiquez ici son chemin. Polyfin doit pouvoir y écrire. Le changer ne déplace pas les enregistrements déjà faits.',
      computer:
        'Un dossier de l’ordinateur où tourne Polyfin, celui indiqué s’il est vide, dans son dossier de données. Pour garder les enregistrements sur un autre disque, indiquez ici un dossier de ce disque. Polyfin doit pouvoir y écrire. Le changer ne déplace pas les enregistrements déjà faits.',
    },
    recordingPrePadding: 'Commencer les enregistrements avant (minutes)',
    recordingPrePaddingHelp: (r: RangeText) =>
      `Combien de minutes avant l’émission un enregistrement commence. De ${r.min} à ${r.max} ; ${r.default} par défaut.`,
    recordingPostPadding: 'Continuer les enregistrements après (minutes)',
    recordingPostPaddingHelp: (r: RangeText) =>
      `Combien de minutes après l’émission un enregistrement continue. De ${r.min} à ${r.max} ; ${r.default} par défaut.`,
    recordingRetentionDays: 'Garder les enregistrements (jours, 0 = toujours)',
    recordingRetentionDaysHelp: (r: RangeText) =>
      `Les enregistrements plus anciens sont supprimés. La vérification a lieu chaque jour. De ${r.min} à ${r.max}.`,
    recordingCommercials: 'Trouver les publicités des enregistrements',
    recordingCommercialsHelp:
      'Quand un enregistrement se termine, Polyfin le lit en arrière-plan, un enregistrement à la fois, pour trouver les moments à la fois noirs et silencieux, et prend les courts passages entre eux pour des coupures publicitaires. Les applications proposent alors de les passer, selon ce que chaque utilisateur a choisi dans les réglages de lecture de son application, quand les boutons « Passer l’intro » et « Passer le générique » sont activés. Les enregistrements de plus de 4 heures ne sont pas lus.',
    convertRecordings: 'Convertir les enregistrements terminés en HEVC',
    convertRecordingsHelp:
      'Les enregistrements terminés sont convertis en HEVC, un à la fois, aux heures ci-dessous, pour prendre moins de place. L’image garde sa taille ; l’audio et les sous-titres sont copiés tels quels. La conversion utilise la « Carte graphique pour convertir », avec la « Qualité HEVC » et la « Méthode de désentrelacement » de Conversion (transcodage), et compte parmi les « Conversions vidéo en même temps » : elle attend tant que des lectures les occupent toutes. Elle ne commence qu’avec la place libre pour une copie aussi grande que l’enregistrement. La copie ne remplace l’enregistrement que si elle dure aussi longtemps, contient les mêmes pistes, se décode jusqu’au bout et est plus petite ; sinon elle est supprimée, et l’enregistrement gardé tel quel et pas réessayé. Un enregistrement en cours de lecture est remplacé une fois la lecture arrêtée ; son état vu, ses points de reprise et ses publicités restent. Les applications qui ne lisent pas le HEVC reçoivent l’enregistrement converti à la lecture : avec « Conversion (transcodage) » désactivée, elles ne peuvent pas le lire. Désactivé par défaut.',
    convertRecordingsBetween: 'Convertir entre',
    convertRecordingsAnd: 'et',
    convertRecordingsBetweenHelp: (from: string, until: string) =>
      `Dans le fuseau horaire du serveur. Une conversion encore en cours à la fin s’arrête, et reprend du début la nuit suivante. De ${from} à ${until} par défaut.`,
    convertRecordingsFrom: 'Début des conversions',
    convertRecordingsUntil: 'Fin des conversions',
    convertRecordingsOnProcessor: 'Utiliser le processeur sans carte graphique',
    convertRecordingsOnProcessorHelp:
      'Sans carte graphique qui encode le HEVC, les enregistrements sont convertis par le processeur avec x265, à sa priorité la plus basse. C’est lent, et le processeur reste occupé pendant les heures ci-dessus. Sans carte graphique et avec ce réglage désactivé, rien n’est converti. Désactivé par défaut.',
    liveTvRefreshHours: 'Actualiser les listes et les guides de TV toutes les (heures)',
    liveTvRefreshHoursHelp: (r: RangeText) =>
      `À quelle fréquence les listes de chaînes IPTV et les guides des programmes XMLTV sont téléchargés à nouveau. Un téléchargement qui a échoué est réessayé plus tôt : après 5 minutes, 15 minutes, puis toutes les heures. De ${r.min} à ${r.max} ; ${r.default} par défaut.`,
    backupsFolder: (folder: string) =>
      `Les sauvegardes de la base de données sont enregistrées dans ${folder}.`,
    backups: 'Sauvegarder la base de données chaque jour',
    backupsHelp:
      'Écrit chaque jour une copie de la base de données dans le dossier ci-dessous, à l’heure choisie, et supprime les plus anciennes au-delà du nombre conservé.',
    backupFolderLabel: 'Dossier des sauvegardes',
    backupFolderHelp: {
      container:
        'Un dossier du conteneur de Polyfin, celui indiqué s’il est vide, dans son volume de données. Les sauvegardes sont plus sûres sur un autre disque que celui de la base : montez-en un à cet endroit, ou ailleurs et indiquez ici son chemin. Polyfin doit pouvoir y écrire.',
      computer:
        'Un dossier de l’ordinateur où tourne Polyfin, celui indiqué s’il est vide, dans son dossier de données. Les sauvegardes sont plus sûres sur un autre disque que celui de la base : indiquez ici un dossier d’un tel disque. Polyfin doit pouvoir y écrire.',
    },
    backupHour: 'Sauvegarder chaque jour à',
    backupHourHelp: (hour: string) => `Dans le fuseau horaire du serveur. ${hour} par défaut.`,
    backupsKept: 'Sauvegardes à conserver',
    backupsKeptHelp: (r: RangeText) =>
      `Après chaque sauvegarde, Polyfin supprime ses plus anciennes sauvegardes au-delà de ce nombre. Les autres fichiers du dossier ne sont jamais touchés. De ${r.min} à ${r.max} ; ${r.default} par défaut.`,
    backupPassphrase: 'Phrase de passe des sauvegardes',
    backupPassphraseHelp: (min: number) =>
      `Chiffre chaque sauvegarde à mesure qu’elle est écrite, dans le dossier et sur chaque destination, qui ne peut donc pas la lire. Au moins ${min} caractères. Elle est gardée chiffrée et n’est plus jamais affichée.`,
    backupPassphraseNew:
      'Gardez cette phrase de passe ailleurs que sur ce serveur, par exemple dans un gestionnaire de mots de passe : une phrase de passe perdue, ce sont des sauvegardes perdues. Personne ne peut les restaurer sans elle, Polyfin compris. Vérifiez-la avec l’œil avant d’enregistrer. Les sauvegardes déjà faites restent telles quelles.',
    backupPassphraseRemoved:
      'Les sauvegardes faites à partir de maintenant ne sont pas chiffrées. Celles déjà faites demandent toujours cette phrase de passe pour être restaurées.',
    lastBackup: 'Dernière sauvegarde',
    backupDestinations: 'Destinations',
    backupDestinationsHelp:
      'Chaque sauvegarde est aussi envoyée à ces endroits, sous le même nom de fichier, une fois écrite dans le dossier : stockages compatibles S3, serveurs SFTP ou WebDAV. Un envoi qui échoue est retenté à la sauvegarde suivante.',
    webPlayerHelp:
      'Ce que le lecteur web (jellyfin-web, sur /web/) affiche en plus de ses propres pages. Le thème et le script s’appliquent au prochain chargement d’une page du lecteur web ; le lecteur web garde le message de connexion jusqu’à une minute.',
    openWebPlayer: 'Ouvrir le lecteur web',
    webTheme: 'Thème du serveur',
    webThemeHelp:
      'L’apparence du lecteur web pour les utilisateurs qui gardent celle du serveur ; chacun peut avoir la sienne, sur sa page dans Utilisateurs. LumaaGlaass est le thème fourni avec Polyfin. L’apparence de jellyfin-web n’a ni CSS ni script personnalisé, et chacun en choisit le thème d’affichage dans Paramètres › Affichage. Personnalisé utilise votre CSS et votre JavaScript ci-dessous, conservés pendant qu’un autre thème est choisi.',
    webThemeChoices: {
      lumaaglaass: 'LumaaGlaass',
      none: 'L’apparence de jellyfin-web',
      custom: 'Personnalisé',
    },
    webThemeReload: 'Après un changement, rechargez les pages ouvertes du lecteur web.',
    webThemeTexts: {
      lumaaglaass:
        'Ce que Polyfin sert pour LumaaGlaass, affiché ici sans pouvoir être modifié. Choisissez Personnalisé, ou « Modifier comme personnalisé », pour écrire le vôtre.',
      none: 'L’apparence de jellyfin-web ne sert ni CSS ni JavaScript personnalisé. Choisissez Personnalisé pour écrire le vôtre.',
    },
    editAsCustom: 'Modifier comme personnalisé',
    editAsCustomTitle: 'Remplacer vos textes personnalisés ?',
    editAsCustomConfirm:
      'Votre CSS et votre JavaScript personnalisés sont remplacés par ceux de LumaaGlaass, et le thème devient Personnalisé. Enregistrer la page garde le changement.',
    themeCopy: 'Garder une copie dans Polyfin',
    themeCopyHelp: (depth: number, file: string, total: string, files: number) =>
      `Enregistre les feuilles de style, scripts, polices et images que les textes personnalisés chargent d’ailleurs, comme un thème sur un CDN, dans le dossier de données de Polyfin, et fait charger ces fichiers depuis Polyfin : le thème fonctionne alors sans joindre son CDN. Suit @import, url() et les adresses que nomment les scripts, sur ${depth} niveaux, depuis des adresses d’internet uniquement, jusqu’à ${file} par fichier, et ${total} et ${files} fichiers en tout.`,
    themeCopySaved: (files: number, size: string, date: string) =>
      `Une copie de ${files === 1 ? 'un fichier' : `${files} fichiers`}, ${size}, enregistrée le ${date}.`,
    themeCopyFiles: 'Fichiers enregistrés',
    themeCopyUnsaved: (count: number) =>
      count === 1
        ? 'Une adresse n’a pas pu être enregistrée ; elle reste chargée d’ailleurs :'
        : `${count} adresses n’ont pas pu être enregistrées ; elles restent chargées d’ailleurs :`,
    themeCopyMissing: (count: number) =>
      count === 1
        ? 'Un fichier de la copie manque dans le dossier de données de Polyfin, comme après la restauration d’une sauvegarde sur un autre système. Les navigateurs le chargent en attendant depuis son adresse d’origine, ce qui demande internet. Mettez la copie à jour pour le télécharger de nouveau.'
        : `${count} fichiers de la copie manquent dans le dossier de données de Polyfin, comme après la restauration d’une sauvegarde sur un autre système. Les navigateurs les chargent en attendant depuis leurs adresses d’origine, ce qui demande internet. Mettez la copie à jour pour les télécharger de nouveau.`,
    themeCopyOutside: (count: number) =>
      count === 1
        ? 'Les textes personnalisés chargent une adresse d’ailleurs :'
        : `Les textes personnalisés chargent ${count} adresses d’ailleurs :`,
    themeCopyStillOutside: (count: number) =>
      count === 1
        ? 'Une adresse ajoutée depuis la copie est chargée d’ailleurs ; mettez la copie à jour pour l’enregistrer aussi :'
        : `${count} adresses ajoutées depuis la copie sont chargées d’ailleurs ; mettez la copie à jour pour les enregistrer aussi :`,
    themeCopyNothing: 'Les textes personnalisés ne chargent rien d’ailleurs.',
    themeCopyKeep: 'Garder une copie dans Polyfin',
    themeCopyUpdate: 'Mettre la copie à jour',
    themeCopyRemove: 'Supprimer la copie',
    themeCopyRemoveTitle: 'Supprimer la copie ?',
    themeCopyRemoveConfirm:
      'Ses fichiers sont effacés, et les textes personnalisés les chargent de nouveau depuis leurs adresses d’origine.',
    themeCopyKept:
      'Copie enregistrée. Rechargez les pages ouvertes du lecteur web dans une minute.',
    themeCopyRemoved:
      'Copie supprimée. Rechargez les pages ouvertes du lecteur web dans une minute.',
    themeCopyFailed: 'Aucun fichier n’a pu être enregistré, donc aucune copie n’a été faite :',
    themeCopySaveFirst:
      'Enregistrez d’abord vos changements : la copie est faite à partir des textes enregistrés.',
    themeCopyKinds: {
      stylesheet: 'feuille de style',
      script: 'script',
      font: 'police',
      image: 'image',
    },
    themeCopyReasons: {
      private_address: 'sur un réseau local, ou redirigée vers lui',
      not_http: 'redirigée vers une adresse ni http ni https',
      unavailable: 'indisponible',
      timeout: 'trop longue à répondre',
      wrong_type: 'pas un fichier du genre sous lequel elle est chargée',
      too_large: 'plus grande que la limite d’un fichier',
      total_too_large: 'au-delà de la limite de la copie entière',
      too_many_files: 'au-delà du nombre de fichiers permis',
      too_deep: 'nommée trop de niveaux plus bas',
      built_at_run_time:
        'complétée par un script quand il s’exécute, ce qu’aucune copie ne peut suivre',
    },
    customCss: 'CSS personnalisé',
    customCssHelp:
      'Appliqué à toutes les pages du lecteur web, pour tous les utilisateurs, comme le CSS personnalisé de Jellyfin. Chacun peut le désactiver dans Paramètres › Affichage.',
    customJs: 'JavaScript personnalisé',
    customJsHelp:
      'Chargé par toutes les pages du lecteur web, après ses propres scripts. Vide, rien n’est chargé.',
    customJsWarningTitle: 'Ne collez que du code de confiance',
    customJsWarning:
      'Ce script s’exécute dans le navigateur de chaque utilisateur qui ouvre le lecteur web de ce serveur, avec son compte. Il peut lire ce qu’il voit et agir à sa place.',
    loginDisclaimer: 'Message de connexion',
    loginDisclaimerHelp:
      'Affiché sous le formulaire de connexion du lecteur web. Texte, Markdown ou HTML ; le lecteur web retire le HTML dangereux.',
    addVersionsToOpenPage: 'Ajouter les versions à la page ouverte d’un titre',
    addVersionsToOpenPageHelp:
      'Ajoute les versions trouvées par les addons au menu des versions de la page d’un film ou d’un épisode pendant qu’elle est ouverte, en gardant la version choisie. Désactivé, une page ouverte avant qu’une version soit trouvée affiche quand même les premières dès qu’elles arrivent, et ses boutons de lecture attendent toujours une version ; les autres apparaissent à la prochaine ouverture de la page.',
    posterRatings: 'Afficher les notes sur les affiches',
    posterRatingsHelp:
      'Affiche la note en étoiles d’un film ou d’une série sur son affiche dans le lecteur web quand le pointeur ou le focus y arrive, avec les scores des critiques et du public tant qu’une clé d’API MDBList des notes est définie dans Paramètres › Contenu. Pour tous les utilisateurs. Tant que Badges sur les affiches est activé dans Paramètres › Contenu, les badges s’affichent à la place.',
    homeRows: 'Rangées d’accueil',
    homeRowsHelp:
      'Les rangées que le lecteur web ajoute à sa page d’accueil, à leur place, pour tous les utilisateurs ou pour ceux choisis. Chaque utilisateur peut désactiver une rangée dans Mon compte › Rangées d’accueil ; les sections propres au lecteur web restent dans ses Paramètres › Accueil. Chaque rangée laisse de côté ce que l’utilisateur ne peut pas voir : les titres au-dessus de son contrôle parental, les genres bloqués, les titres masqués et les bibliothèques qui lui sont masquées.',
    homeRow: {
      loading: 'Chargement des rangées d’accueil…',
      empty: 'Aucune rangée d’accueil',
      emptyHint: 'La page d’accueil du lecteur web n’affiche que ses propres sections.',
      add: 'Ajouter une rangée',
      addTitle: 'Ajouter une rangée d’accueil',
      editTitle: (name: string) => `Modifier ${name}`,
      edit: 'Modifier',
      delete: 'Supprimer',
      deleteTitle: (name: string) => `Supprimer ${name} ?`,
      deleteBody:
        'La rangée quitte la page d’accueil de chaque utilisateur, et sa bibliothèque quitte les autres applis.',
      deleted: (name: string) => `${name} est supprimée.`,
      saved: (name: string) => `${name} est enregistrée.`,
      create: 'Ajouter',
      creating: 'Ajout…',
      save: 'Enregistrer',
      saving: 'Enregistrement…',
      name: 'Nom',
      nameHelp: 'Le titre de la rangée sur la page d’accueil.',
      becauseNameHelp:
        'Chaque rangée porte ce nom, suivi du titre qu’elle suit : « Parce que vous avez regardé Film exemple ».',
      kind: 'Type',
      kinds: {
        catalog: 'Un catalogue',
        collection: 'Une collection',
        because: 'Parce que vous avez regardé',
        new: 'Nouveautés dans vos bibliothèques',
        available: 'Disponibles',
        waiting: 'Toujours en attente',
        trakt: 'Recommandés sur Trakt',
        picks: 'Sélection pour vous',
      },
      kindHelp: {
        catalog:
          'La première page d’un catalogue des addons du serveur, bibliothèque ou non, comme un catalogue des tendances. Les utilisateurs qui n’utilisent pas les addons du serveur, ou à qui sa bibliothèque est masquée, ne la voient pas.',
        collection:
          'Les titres d’une collection : une collection faite par les utilisateurs, une liste intelligente affichée comme collection, ou une collection d’un addon.',
        because:
          'Une rangée pour chacun des derniers films et séries vus par l’utilisateur, avec les titres similaires, comme les recommandations de films les donnent. Aucune tant que Titres similaires est désactivé.',
        new: 'Les titres arrivés dans les catalogues des bibliothèques de l’utilisateur ces 7 derniers jours, les plus récents d’abord. Polyfin les compte depuis la première fois qu’il a vu chaque titre dans chaque catalogue.',
        available:
          'Les titres non commencés de la liste « À regarder » de l’utilisateur pour lesquels la dernière vérification a trouvé des versions, les derniers trouvés d’abord. Une version trouvée n’a pas encore été essayée : sa lecture peut encore échouer.',
        waiting:
          'Les titres non commencés de la liste « À regarder » de l’utilisateur pour lesquels la dernière vérification n’a trouvé aucune version, ou qui ne sont pas encore sortis, les derniers ajoutés d’abord. Un titre pas encore vérifié n’est dans aucune des deux rangées.',
        trakt:
          'Les films et séries que Trakt recommande à l’utilisateur, lus avec sa connexion Trakt toutes les 12 heures, sans ceux de sa collection ou de sa Watchlist Trakt ni ceux qu’il a vus. Seuls les utilisateurs qui ont connecté Trakt dans Mon compte › Suivi la voient ; la première fois, elle s’affiche une fois que Trakt a répondu.',
        picks:
          'Les films et séries des bibliothèques de l’utilisateur, listes intelligentes comprises, que son profil de goûts classe en premier, sans ceux qu’il a vus ni ceux d’un genre qu’il n’aime pas. Tant que le profil ne le connaît pas assez, les premiers titres des bibliothèques, les plus populaires d’abord. Chaque utilisateur voit son profil dans Mon compte.',
      },
      defaultNames: {
        because: 'Parce que vous avez regardé',
        new: 'Nouveautés dans vos bibliothèques',
        available: 'Disponibles',
        waiting: 'Toujours en attente',
        trakt: 'Recommandés sur Trakt',
        picks: 'Sélection pour vous',
      },
      catalog: 'Catalogue',
      noCatalog: 'Les addons du serveur n’ont aucun catalogue à afficher.',
      collection: 'Collection',
      noCollection: 'Il n’y a encore aucune collection.',
      collectionSources: {
        users: 'Faite par les utilisateurs',
        smart: 'Liste intelligente',
        addon: 'Addon',
      },
      choose: 'Choisir…',
      unknownCatalog: 'Un catalogue qui n’est plus proposé',
      unknownCollection: 'Une collection qui n’est plus proposée',
      count: 'Rangées',
      countHelp: 'Combien des derniers films et séries vus ont une rangée, de 1 à 10.',
      place: 'Place',
      places: {
        top: 'En haut',
        resume: 'Après « Continuer de regarder »',
        nextup: 'Après « À suivre »',
        bottom: 'En bas',
      },
      placeHelp:
        'Là où le lecteur web l’affiche. Quand la page d’accueil d’un utilisateur n’affiche pas cette section, la rangée prend la place précédente.',
      shownTo: 'Affichée pour',
      everyone: 'Tous les utilisateurs',
      chosenUsers: 'Les utilisateurs choisis',
      users: 'Utilisateurs',
      noUser: 'Choisissez au moins un utilisateur.',
      shownToUsers: (count: number) => (count === 1 ? '1 utilisateur' : `${count} utilisateurs`),
      rowsCount: (count: number) => (count === 1 ? '1 rangée' : `${count} rangées`),
      library: 'Aussi une bibliothèque dans les autres applis',
      libraryHelp:
        'On ne peut pas donner aux autres applis un nouveau type de section d’accueil : la rangée devient aussi une bibliothèque, sans type, qu’elles affichent parmi les bibliothèques de l’utilisateur, avec une rangée de ses derniers titres sur leur accueil.',
      libraryBadge: 'Bibliothèque',
      moveLabel: 'Déplacer',
    },
    codeSize: (used: number, max: number) =>
      `${used} Ko sur ${max >= 1024 ? `${max / 1024} Mo` : `${max} Ko`}`,
    codeKeys: 'Tab insère des espaces ; appuyez sur Échap puis Tab pour quitter le champ.',
  },
  settingsPage: {
    searchHint: 'Par nom ou par description.',
    noMatch: 'Aucun paramètre ne correspond à cette recherche.',
    sections: {
      variables: 'Variables d’environnement',
    },
    variablesHelp:
      'En lecture seule. Elles sont définies sur le conteneur (environnement Docker, fichier compose ou modèle Unraid) et s’appliquent au démarrage de Polyfin : modifiez-les là, puis redémarrez le conteneur. Les variables qui réglaient d’autres options sont copiées une fois dans les paramètres, au premier démarrage de cette version, puis ne sont plus lues : modifiez celles-ci dans les paramètres. Les secrets sont masqués, et l’adresse de la base de données n’affiche que son hôte et son nom.',
    value: 'Valeur utilisée',
    defaultValue: 'Par défaut',
    setValue: 'Définie',
    hidden: 'Masquée',
    empty: 'Vide',
    notRead: 'Non lue par Polyfin',
    searchPlaceholder: 'Chercher un réglage',
    results: (count: number) => (count === 1 ? '1 réglage trouvé' : `${count} réglages trouvés`),
    noMatchHelp: 'Essayez un autre mot, comme le nom d’une variable ou d’un service.',
    variableHint: 'Variable d’environnement',
    ledes: {
      general:
        'Le nom du serveur, sa langue, la connexion des applications et leur découverte du serveur, et la recherche de nouvelles versions.',
      playback: 'Comment Polyfin choisit et prépare les versions qu’il lit.',
      content:
        'Passer l’intro, titres similaires, images générées, notes, paroles des chansons, volume égal entre les chansons et seuils de lecture.',
      catalogs: 'Ce que Polyfin lit des catalogues des addons, et à quelle fréquence.',
      security:
        'Addons et importations des utilisateurs, comptes bloqués, appareils inutilisés et proxys inverses.',
      liveTv:
        'La fréquence à laquelle les listes de chaînes IPTV et les guides sont retéléchargés.',
      diagnostics: 'Le journal détaillé et les variables d’environnement utilisées.',
      notifications:
        'Où Polyfin signale, pour tous les utilisateurs, les nouveaux épisodes, les enregistrements, les lectures et les connexions qui s’arrêtent, ainsi que les problèmes de santé, les nouveaux utilisateurs, les nouvelles versions et les comptes bloqués, et envoie le résumé de la semaine et le bilan du mois et de l’année.',
    },
    groups: {
      skip: 'Passer l’intro et le générique',
      titlePages: 'Fiche des titres',
      images: 'Images',
      ratings: 'Notes',
      posters: 'Affiches',
      music: 'Musique',
      thresholds: 'Seuils de lecture',
    },
    leaveTitle: 'Partir sans enregistrer ?',
    leaveBody:
      'Les modifications de cette section ne sont pas encore enregistrées. Elles seront perdues si vous partez.',
    leave: 'Partir sans enregistrer',
    stay: 'Rester',
    variablesEmpty: 'Aucune variable à afficher',
    variablesEmptyHelp: 'Définissez des variables POLYFIN_ sur le conteneur, puis redémarrez-le.',
  },
}

export default settings
