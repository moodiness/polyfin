import type en from '../en/settings'
import type { RangeText } from '../en/settings'

const settings: typeof en = {
  settings: {
    tracking: {
      description:
        'Chaque utilisateur peut connecter ses propres comptes Trakt, Simkl, MDBList et PublicMetaDB depuis sa page Mon compte, et Polyfin indique à ces services ce qu’il regarde ; Last.fm et ListenBrainz reçoivent les morceaux qu’il écoute. Trakt, Simkl et Last.fm ont d’abord besoin d’une application de ce serveur, à configurer ici. MDBList, PublicMetaDB et ListenBrainz n’ont besoin de rien.',
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
    },
    title: 'Paramètres',
    description: 'Options qui s’appliquent à l’ensemble du serveur.',
    serverName: 'Nom du serveur',
    serverNameHelp: (r: RangeText) =>
      `De ${r.min} à ${r.max} caractères, affiché dans les applications Jellyfin.`,
    quickConnect: 'Autoriser Quick Connect',
    quickConnectHelp:
      'Permet aux applications TV et mobiles de se connecter avec un code à 6 chiffres autorisé ici.',
    language: 'Langue des noms générés',
    languageHelp:
      'Polyfin nomme lui-même certains éléments dans les applications Jellyfin : les saisons (« Saison 1 », « Épisodes spéciaux »), les épisodes sans titre et le type ajouté aux bibliothèques de même nom (« Populaires (Films) »).',
    legacyAuthorization: 'Autoriser l’authentification héritée',
    legacyAuthorizationHelp:
      'Accepte les anciens en-têtes X-Emby-*, le paramètre api_key et le schéma Emby pour les applications qui en ont encore besoin. Désactivé par défaut, comme dans Jellyfin 12.2.',
    legacyWarningTitle: 'Avertissement de sécurité',
    legacyWarning:
      'Les méthodes héritées peuvent transmettre les identifiants dans les URL, qui se retrouvent alors dans les journaux, l’historique du navigateur et les proxys. N’activez cette option que si une de vos applications ne parvient pas à se connecter autrement.',
    prepareAhead: 'Préparer la lecture à l’avance',
    prepareAheadHelp:
      'Polyfin lit le fichier dès que la page d’un titre s’ouvre, et prépare l’épisode suivant vers la fin de celui en cours, pour que la lecture démarre tout de suite. Il liste aussi à l’avance les versions des titres de « Continuer de regarder » et « À suivre », pour que leur page les affiche aussitôt. En contrepartie, vos sources reçoivent un peu plus de demandes, y compris pour les titres ouverts mais pas regardés.',
    transcoding: 'Conversion (transcodage)',
    transcodingHelp:
      'Convertit l’image et le son pour les applications qui ne savent pas lire un fichier tel quel. Si cette option est désactivée, les fichiers sont lus tels quels ou simplement présentés autrement, sans toucher à l’image ni au son. Un titre qu’une application ne peut pas lire ainsi ne démarrera pas sur cette application.',
    analysisTimeout: 'Temps max pour analyser une version',
    analysisTimeoutHelp: (r: RangeText) =>
      `Avant une première lecture, Polyfin analyse le fichier ou la chaîne pour savoir comment le lire. Si la source ne répond pas dans ce délai, en secondes, Polyfin abandonne cette version et passe à la suivante. Un nombre plus petit passe plus vite à la suivante, mais peut abandonner des sources lentes qui auraient marché. Une chaîne est analysée 8 secondes au plus. De ${r.min} à ${r.max} secondes ; ${r.default} par défaut.`,
    versionAttempts: 'Versions essayées quand une ne marche pas',
    versionAttemptsHelp: (r: RangeText) =>
      `Quand une application lit un titre sans choisir de version, Polyfin essaie les versions dans l’ordre jusqu’à en trouver une qui marche, sans en analyser plus que ce nombre. Les versions déjà analysées sont aussi essayées, car elles ne coûtent rien. Un nombre plus grand trouve plus souvent une version qui marche, mais un titre qui ne se lit pas met plus longtemps à le dire. De ${r.min} à ${r.max} ; ${r.default} par défaut.`,
    preferDirectPlay: 'Préférer les versions lues sans conversion',
    preferDirectPlayHelp:
      'Quand une application lit un titre sans choisir de version, Polyfin prend la première version que l’application lit telle quelle ou simplement présentée autrement, plutôt que la première qui se lit tout court. La toute première lecture d’un titre peut être un peu plus lente, car plus de versions peuvent être analysées ; rien ne change une fois qu’elles sont connues.',
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
    publicAddress: 'Adresse publique',
    publicAddressHelp:
      'L’adresse à laquelle on ouvre Polyfin, comme https://media.example.org. Les liens des notifications commencent par elle ; vide, les messages n’ont pas de lien.',
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
    maxConversions: 'Conversions vidéo en même temps (0 = pas de limite)',
    maxConversionsHelp: (r: RangeText) =>
      `Convertir l’image est le travail le plus lourd du serveur. Quand ce nombre de lectures avec image convertie est atteint (sous-titres incrustés dans l’image compris), une nouvelle lecture prend une version qui n’a pas besoin de conversion, ou ne démarre pas. Les lectures en cours ne sont jamais coupées. La TV en direct garde aussi ses propres limites : 4 chaînes par utilisateur et 16 pour le serveur. De ${r.min} à ${r.max} ; ${r.default} par défaut.`,
    maxConversionHeight: 'Qualité max des vidéos converties',
    maxConversionHeightHelp:
      'Les vidéos converties sont réduites à cette hauteur au plus, sans être déformées, pour passer mieux sur une connexion lente. Les fichiers lus tels quels ou simplement présentés autrement gardent leur qualité. La carte graphique convertit jusqu’en 4K ; le processeur s’arrête à 1080p, et quand il convertit du HDR en SDR, à « Qualité max du HDR converti par le processeur » (Conversion › HDR).',
    conversionHeightOriginal: 'Originale',
    conversionHeight: (height: number) => `${height}p`,
    conversion: {
      description:
        'Quand une application ne sait pas lire un fichier tel quel, Polyfin le convertit avec FFmpeg. Ces réglages disent comment, pour les films, les séries et la TV en direct. Les valeurs par défaut conviennent à la plupart des serveurs.',
      groups: {
        general: 'Général',
        gpu: 'Carte graphique',
        video: 'Image',
        hdr: 'HDR',
        interlaced: 'Vidéo entrelacée',
        audio: 'Son',
        performance: 'Performances',
      },
      hardwareAcceleration: 'Carte graphique pour convertir',
      hardwareAccelerationHelp:
        'Une carte graphique (GPU) convertit la vidéo bien plus vite que le processeur. Un changement s’applique dès l’enregistrement, sans redémarrer.',
      vaapiDevice: 'Carte graphique pour VAAPI',
      vaapiDeviceHelp:
        'Le nœud de rendu sur lequel VAAPI convertit la vidéo, quand plusieurs cartes graphiques le pourraient. Chacune à son tour par défaut, la première qui fonctionne.',
      vaapiDeviceEach: 'Chacune à son tour',
      vaapiDeviceMissing: (path: string) => `${path} (introuvable)`,
      hardware: {
        auto: 'Automatique : NVIDIA, sinon AMD ou Intel',
        nvenc: 'NVIDIA (NVENC)',
        vaapi: 'AMD ou Intel (VAAPI)',
        none: 'Aucune : le processeur seul',
      },
      detected: 'Trouvé sur ce serveur',
      noGpu: 'Aucune carte graphique n’est utilisée : le processeur convertit la vidéo.',
      gpu: 'Carte graphique',
      gpuNames: { cuda: 'NVIDIA', vaapi: 'AMD ou Intel (VAAPI)' },
      encoders: 'Encodeurs de la carte',
      gpuToneMapping: 'HDR vers SDR sur la carte',
      processorToneMappingHint:
        'Le HDR est alors converti par le processeur, jusqu’à « Qualité max du HDR converti par le processeur ».',
      qualityFactor: 'Accepte un niveau de qualité',
      processor: 'Encodeurs du processeur',
      processorToneMapping: 'HDR vers SDR sur le processeur',
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
        'Plus lent donne une meilleure image pour la même taille, mais demande plus de puissance. Si les vidéos converties saccadent, choisissez plus rapide. Automatique garde le choix de Polyfin : très rapide sur le processeur, moyen sur les cartes NVIDIA, celui du pilote sur les cartes AMD et Intel.',
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
      toneMapping: 'Convertir le HDR en SDR (tone mapping)',
      toneMappingHelp:
        'Garde les bonnes couleurs et la bonne luminosité des vidéos HDR sur les écrans qui n’affichent que le SDR. Si cette option est désactivée, les vidéos HDR converties paraissent ternes, et les vidéos Dolby Vision sans couche HDR10 ne sont pas converties.',
      toneMappingUnavailable:
        'Ni la carte graphique ni FFmpeg ne savent le faire sur ce serveur : les vidéos HDR ne sont converties que si cette option est désactivée.',
      toneMappingAlgorithm: 'Méthode de conversion HDR',
      toneMappingAlgorithmHelp:
        'La façon de ramener les parties claires au SDR. Automatique utilise BT.2390 sur les cartes NVIDIA et Hable sur le processeur. Les cartes Intel ont leur propre méthode et ignorent ce choix.',
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
        'Les cartes NVIDIA, et les cartes Intel par VAAPI, convertissent elles-mêmes le HDR en SDR, jusqu’en 4K. Désactivez cette option si leurs couleurs semblent fausses : le processeur s’en charge alors, en qualité moindre. Les cartes Intel ne prennent que les vidéos HDR10 qui portent les informations de leur écran de mastering ; le processeur convertit les autres, et le Dolby Vision sans couche HDR10 n’est converti que sur les cartes NVIDIA. Les cartes AMD ne le font jamais : leur pilote Linux a échoué ou s’est bloqué à chaque façon essayée par Polyfin.',
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
        `Automatique donne 192 kb/s en stéréo, et 64 kb/s par canal au-delà. 0, ou de ${r.min} à ${r.max} ; ${r.default} par défaut.`,
      encodingThreads: 'Threads du processeur par conversion (0 = automatique)',
      encodingThreadsHelp: (r: RangeText) =>
        `Limite la part du processeur qu’une conversion utilise, pour laisser de la place au reste. De ${r.min} à ${r.max} ; 0 laisse FFmpeg choisir.`,
      aheadSeconds: 'Secondes préparées à l’avance',
      aheadSecondsHelp: (r: RangeText) =>
        `Polyfin prépare une vidéo au plus ce nombre de secondes au-delà de la partie demandée par l’application, puis attend. Plus aide avec les sources lentes et garde leur connexion occupée, mais utilise plus de puissance et de disque quand on arrête de regarder tôt. De ${r.min} à ${r.max} ; ${r.default} par défaut.`,
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
      'Facultatif. Une clé d’API PublicMetaDB ajoute une troisième source de repères pour passer l’intro et le générique.',
    theIntroDbKey: 'Clé TheIntroDB (facultative)',
    theIntroDbKeyHelp:
      'Relève la limite quotidienne de TheIntroDB et inclut vos propres contributions. La lecture fonctionne sans elle.',
    segmentSources: {
      label: 'Sources de repères',
      help: 'Pour chaque type de passage (intro, résumé, générique, aperçu), la première source activée de cette liste qui le connaît l’emporte ; les suivantes complètent. Les sources désactivées ne sont jamais consultées.',
      turnOn: (name: string) => `Consulter ${name}`,
      needsKey: 'Clé requise',
    },
    similarTitles: 'Titres similaires',
    similarTitlesHelp:
      'La page d’un titre montre des titres proches, trouvés dans les catalogues des addons. Si cette option est désactivée, la liste est vide et les addons reçoivent moins de demandes.',
    lyrics: 'Paroles des chansons depuis LRCLIB',
    lyricsHelp:
      'Les applications affichent les paroles des chansons, synchronisées ligne par ligne quand LRCLIB, une base de paroles libre, les connaît. Polyfin recherche une chanson une seule fois, par son artiste, son titre, son album et sa durée, quand sa lecture commence ou que sa fiche ou ses paroles s’ouvrent. Si cette option est désactivée, les chansons n’ont pas de paroles et LRCLIB n’est jamais consulté.',
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
      'Lit toutes les collections des bibliothèques de collections du serveur à cette heure, l’une après l’autre, pour qu’elles s’ouvrent tout de suite. Cela demande beaucoup de pages aux addons : laissez-le désactivé pour un addon que vous partagez avec d’autres.',
    localScanHours: 'Analyser les dossiers locaux toutes les (heures, 0 = jamais)',
    localScanHoursHelp: (r: RangeText) =>
      `À quelle fréquence chaque dossier local est analysé à nouveau pour trouver les fichiers ajoutés, modifiés ou supprimés. Les dossiers sont aussi analysés au démarrage et à la demande. 0 désactive cette planification. De ${r.min} à ${r.max} ; ${r.default} par défaut.`,
    personalAddons: 'Autoriser les addons personnels des utilisateurs',
    personalAddonsHelp:
      'Les utilisateurs peuvent ajouter leurs propres addons Stremio, en plus de ceux du serveur. Sinon, leurs addons sont conservés mais pas utilisés, et leurs applications Jellyfin n’affichent que les addons du serveur.',
    serverImports: 'Les utilisateurs peuvent importer leur historique depuis un autre serveur',
    serverImportsHelp:
      'Sous Mon compte, chaque utilisateur peut importer ses films et épisodes vus, ses points de reprise et ses favoris depuis un serveur Jellyfin ou Emby, en s’y connectant avec son propre nom et son mot de passe. Sinon, cette section est masquée.',
    loginAttempts: 'Bloquer un compte après ce nombre de mots de passe faux (0 = jamais)',
    loginAttemptsHelp: (r: RangeText) =>
      `Après ce nombre de mots de passe faux à la suite, le compte ne peut plus se connecter pendant 15 minutes, même avec le bon mot de passe. Un administrateur peut le débloquer plus tôt depuis la page Utilisateurs. 0, ou de ${r.min} à ${r.max}.`,
    inactiveDeviceDays: 'Déconnecter les appareils inutilisés depuis (jours, 0 = jamais)',
    inactiveDeviceDaysHelp: (r: RangeText) =>
      `Les applications Jellyfin qui n’ont pas servi depuis ce nombre de jours sont déconnectées et doivent se reconnecter. La vérification a lieu toutes les heures. La connexion à cette page d’administration n’est pas concernée. De ${r.min} à ${r.max}.`,
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
    recordingsTitle: 'Enregistrements',
    recordingsFolder: (folder: string) => `Les enregistrements sont gardés dans ${folder}.`,
    recording: 'Enregistrer la TV en direct',
    recordingHelp:
      'Permet aux utilisateurs de programmer des enregistrements de la TV en direct, écrits dans le dossier ci-dessous.',
    recordingsFolderLabel: 'Dossier des enregistrements',
    recordingsFolderHelp:
      'Un dossier du conteneur de Polyfin, celui indiqué s’il est vide, dans son volume de données. Pour garder les enregistrements sur un autre disque, montez-le à cet endroit, ou ailleurs et indiquez ici son chemin. Polyfin doit pouvoir y écrire. Le changer ne déplace pas les enregistrements déjà faits.',
    recordingPrePadding: 'Commencer les enregistrements avant (minutes)',
    recordingPrePaddingHelp: (r: RangeText) =>
      `Combien de minutes avant l’émission un enregistrement commence. De ${r.min} à ${r.max} ; ${r.default} par défaut.`,
    recordingPostPadding: 'Continuer les enregistrements après (minutes)',
    recordingPostPaddingHelp: (r: RangeText) =>
      `Combien de minutes après l’émission un enregistrement continue. De ${r.min} à ${r.max} ; ${r.default} par défaut.`,
    recordingRetentionDays: 'Garder les enregistrements (jours, 0 = toujours)',
    recordingRetentionDaysHelp: (r: RangeText) =>
      `Les enregistrements plus anciens sont supprimés. La vérification a lieu chaque jour. De ${r.min} à ${r.max}.`,
    liveTvRefreshHours: 'Actualiser les listes et les guides de TV toutes les (heures)',
    liveTvRefreshHoursHelp: (r: RangeText) =>
      `À quelle fréquence les listes de chaînes IPTV et les guides des programmes XMLTV sont téléchargés à nouveau. Un téléchargement qui a échoué est réessayé plus tôt : après 5 minutes, 15 minutes, puis toutes les heures. De ${r.min} à ${r.max} ; ${r.default} par défaut.`,
    backupsFolder: (folder: string) =>
      `Les sauvegardes de la base de données sont enregistrées dans ${folder}.`,
    backups: 'Sauvegarder la base de données chaque jour',
    backupsHelp:
      'Écrit chaque jour une copie de la base de données dans le dossier ci-dessous, à l’heure choisie, et supprime les plus anciennes au-delà du nombre conservé.',
    backupFolderLabel: 'Dossier des sauvegardes',
    backupFolderHelp:
      'Un dossier du conteneur de Polyfin, celui indiqué s’il est vide, dans son volume de données. Les sauvegardes sont plus sûres sur un autre disque que celui de la base : montez-en un à cet endroit, ou ailleurs et indiquez ici son chemin. Polyfin doit pouvoir y écrire.',
    backupHour: 'Sauvegarder chaque jour à',
    backupHourHelp: (hour: string) => `Dans le fuseau horaire du serveur. ${hour} par défaut.`,
    backupsKept: 'Sauvegardes à conserver',
    backupsKeptHelp: (r: RangeText) =>
      `Après chaque sauvegarde, Polyfin supprime ses plus anciennes sauvegardes au-delà de ce nombre. Les autres fichiers du dossier ne sont jamais touchés. De ${r.min} à ${r.max} ; ${r.default} par défaut.`,
    lastBackup: 'Dernière sauvegarde',
    webPlayerHelp:
      'Ce que le lecteur web (jellyfin-web, sur /web/) affiche en plus de ses propres pages. Le script s’applique au prochain chargement d’une page du lecteur web ; le lecteur web garde le CSS et le message de connexion jusqu’à une minute.',
    openWebPlayer: 'Ouvrir le lecteur web',
    customCss: 'CSS personnalisé',
    customCssHelp:
      'Appliqué à toutes les pages du lecteur web, pour tous les utilisateurs, comme le CSS personnalisé de Jellyfin. Chacun peut le désactiver dans Paramètres › Affichage. Par défaut, il contient la feuille de style du thème LumaaGlaass, qui va avec le JavaScript personnalisé par défaut : videz les deux pour garder l’apparence d’origine du lecteur web.',
    customJs: 'JavaScript personnalisé',
    customJsHelp:
      'Chargé par toutes les pages du lecteur web, après ses propres scripts. Vide, rien n’est chargé. Par défaut, il charge le script du thème LumaaGlaass, qui va avec le CSS personnalisé par défaut.',
    customJsWarningTitle: 'Ne collez que du code de confiance',
    customJsWarning:
      'Ce script s’exécute dans le navigateur de chaque utilisateur qui ouvre le lecteur web de ce serveur, avec son compte. Il peut lire ce qu’il voit et agir à sa place.',
    loginDisclaimer: 'Message de connexion',
    loginDisclaimerHelp:
      'Affiché sous le formulaire de connexion du lecteur web. Texte, Markdown ou HTML ; le lecteur web retire le HTML dangereux.',
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
      general: 'Le nom du serveur, sa langue et la connexion des applications.',
      playback: 'Comment Polyfin choisit et prépare les versions qu’il lit.',
      content: 'Passer l’intro, titres similaires, paroles des chansons et seuils de lecture.',
      catalogs: 'Ce que Polyfin lit des catalogues des addons, et à quelle fréquence.',
      security: 'Addons et importations des utilisateurs, comptes bloqués et appareils inutilisés.',
      liveTv:
        'La fréquence à laquelle les listes de chaînes IPTV et les guides sont retéléchargés.',
      diagnostics: 'Le journal détaillé et les variables d’environnement utilisées.',
      notifications:
        'Où Polyfin signale les nouveaux épisodes, les enregistrements, les problèmes de santé et les nouveaux utilisateurs, pour tous les utilisateurs.',
    },
    groups: {
      skip: 'Passer l’intro et le générique',
      titlePages: 'Fiche des titres',
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
