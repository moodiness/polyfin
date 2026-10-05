import type en from '../en/settings'

const settings: typeof en = {
  settings: {
    secret: {
      saved: 'Valeur enregistrée',
      notSet: 'Aucune valeur',
      removing: 'Supprimée à l’enregistrement',
      pastePlaceholder: 'Collez la valeur ici',
      replacePlaceholder: 'Collez une nouvelle valeur pour remplacer celle enregistrée',
      remove: 'Supprimer',
      keep: 'Garder la valeur',
      removeHint: 'Enregistrez pour la supprimer, ou choisissez « Garder la valeur ».',
      show: 'Afficher la clé',
      hide: 'Masquer la clé',
      savedHidden: 'Clé enregistrée, masquée',
    },
    tracking: {
      description:
        'Chaque utilisateur peut connecter ses propres comptes Trakt, Simkl, MDBList et PublicMetaDB depuis sa page Mon compte, et Polyfin indique à ces services ce qu’il regarde. Trakt et Simkl ont d’abord besoin d’une application de ce serveur, à configurer ici. MDBList et PublicMetaDB n’ont besoin de rien.',
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
    },
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
        'Une carte graphique (GPU) convertit la vidéo bien plus vite que le processeur. La variable d’environnement POLYFIN_HWACCEL donne toujours la valeur par défaut ; un choix fait ici la remplace dès l’enregistrement, sans redémarrer.',
      hardwareDefault: (value: string) => `Par défaut, selon POLYFIN_HWACCEL (${value})`,
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
      qualityHelp:
        'Un nombre plus petit donne une meilleure image, mais plus de données ; le débit reste le maximum. 0 vise seulement le débit. Jellyfin utilise 23 pour H.264 et 28 pour HEVC. 0, ou de 1 à 51 ; 0 par défaut.',
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
        'La façon de ramener les parties claires au SDR. Automatique utilise BT.2390 sur la carte graphique et Hable sur le processeur.',
      algorithms: {
        auto: 'Automatique',
        bt2390: 'BT.2390 (carte graphique seulement)',
        hable: 'Hable',
        reinhard: 'Reinhard',
        mobius: 'Möbius',
        clip: 'Écrêtage',
        linear: 'Linéaire',
      },
      toneMappingPeak: 'Luminosité max en nits (0 = celle de la vidéo)',
      toneMappingDesat: 'Désaturation des zones claires (0 = aucune)',
      toneMappingPeakHelp:
        'Sur le processeur seulement. La luminosité max remplace le niveau le plus clair annoncé par la vidéo : 0, ou de 100 à 10 000. La désaturation atténue la couleur des parties très claires : de 0 à 10. Les deux valent 0 par défaut.',
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
      downmixBoostHelp:
        'Un mixage en stéréo sonne souvent moins fort : le volume est multiplié par ce nombre. De 0,5 à 3 ; 1 (aucun changement) par défaut. Jellyfin utilise 2.',
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
      audioBitratePerChannelHelp:
        'Automatique donne 192 kb/s en stéréo, et 64 kb/s par canal au-delà. 0, ou de 32 à 320 ; 0 par défaut.',
      encodingThreads: 'Threads du processeur par conversion (0 = automatique)',
      encodingThreadsHelp:
        'Limite la part du processeur qu’une conversion utilise, pour laisser de la place au reste. De 0 à 64 ; 0 laisse FFmpeg choisir.',
      aheadSegments: 'Segments préparés à l’avance',
      aheadSegmentsHelp:
        'Polyfin prépare une vidéo au plus ce nombre de segments, d’environ 6 secondes chacun, au-delà de la partie demandée par l’application, puis attend. Plus aide avec les sources lentes, mais utilise plus de puissance et de disque quand on arrête de regarder tôt. De 1 à 60 ; 10 par défaut.',
    },
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
    publicMetaDbKey: 'Clé PublicMetaDB',
    publicMetaDbKeyHelp:
      'Facultatif. Une clé d’API PublicMetaDB ajoute une troisième source de repères pour passer l’intro et le générique.',
    theIntroDbKey: 'Clé TheIntroDB (facultative)',
    theIntroDbKeyHelp:
      'Relève la limite quotidienne de TheIntroDB et inclut vos propres contributions. La lecture fonctionne sans elle.',
    segmentSources: {
      label: 'Ordre des sources de repères',
      help: 'Pour chaque type de passage (intro, résumé, générique, aperçu), la première source de cette liste qui le connaît l’emporte ; les suivantes complètent.',
      offHelp:
        'Les sources désactivées par POLYFIN_SEGMENTS ne sont jamais consultées, quelle que soit leur place.',
      on: 'Active',
      needsKey: 'Clé requise',
      off: 'Désactivée (POLYFIN_SEGMENTS)',
      reset: 'Rétablir l’ordre par défaut',
      resetDone: 'Ordre par défaut rétabli. Enregistrez pour l’appliquer.',
    },
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
    backupsFolder: (folder: string) =>
      `Les sauvegardes de la base de données sont enregistrées dans ${folder}.`,
    backupsOff:
      'Les sauvegardes sont désactivées. Indiquez dans POLYFIN_BACKUP_DIR un dossier du conteneur, monté depuis le serveur, puis redémarrez Polyfin pour les activer.',
    backupHour: 'Sauvegarder chaque jour à',
    backupHourHelp: 'Dans le fuseau horaire du serveur. 4:00 par défaut.',
    backupsKept: 'Sauvegardes à conserver',
    backupsKeptHelp:
      'Après chaque sauvegarde, Polyfin supprime ses plus anciennes sauvegardes au-delà de ce nombre. Les autres fichiers du dossier ne sont jamais touchés. De 1 à 90 ; 7 par défaut.',
    lastBackup: 'Dernière sauvegarde',
    webPlayerHelp:
      'Ce que le lecteur web (jellyfin-web, sur /web/) affiche en plus de ses propres pages. Le script s’applique au prochain chargement d’une page du lecteur web ; le lecteur web garde le CSS et le message de connexion jusqu’à une minute.',
    openWebPlayer: 'Ouvrir le lecteur web',
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
    codeSize: (used: number, max: number) => `${used} Ko sur ${max} Ko`,
    codeKeys: 'Tab insère des espaces ; appuyez sur Échap puis Tab pour quitter le champ.',
  },
  settingsPage: {
    search: 'Chercher un paramètre',
    searchHint: 'Par nom ou par description.',
    noMatch: 'Aucun paramètre ne correspond à cette recherche.',
    sectionsLabel: 'Sections',
    unsaved: 'Modifications non enregistrées',
    upToDate: 'Tout est enregistré',
    sections: {
      general: 'Général',
      playback: 'Lecture',
      conversion: 'Conversion',
      content: 'Contenu',
      catalogs: 'Catalogues',
      thumbnails: 'Miniatures',
      security: 'Utilisateurs et sécurité',
      tracking: 'Suivi',
      liveTv: 'TV en direct',
      recordings: 'Enregistrements',
      diagnostics: 'Diagnostic',
      webPlayer: 'Lecteur web',
      backups: 'Sauvegardes',
      variables: 'Variables d’environnement',
    },
    variablesHelp:
      'En lecture seule. Elles sont définies sur le conteneur (environnement Docker, fichier compose ou modèle Unraid) et s’appliquent au démarrage de Polyfin : modifiez-les là, puis redémarrez le conteneur. Les secrets sont masqués, et l’adresse de la base de données n’affiche que son hôte et son nom.',
    variable: 'Variable',
    value: 'Valeur utilisée',
    defaultValue: 'Par défaut',
    setValue: 'Définie',
    hidden: 'Masquée',
    empty: 'Vide',
    notRead: 'Non lue par Polyfin',
  },
}

export default settings
