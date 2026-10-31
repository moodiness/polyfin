import type en from '../en/home'

/** The GPU methods, as the server names them. */
const gpuMethodNames: Record<string, string> = {
  cuda: 'NVIDIA',
  vaapi: 'VAAPI',
  videotoolbox: 'VideoToolbox',
  qsv: 'QSV',
  amf: 'AMF',
}

const home: typeof en = {
  quickConnect: {
    title: 'Quick Connect',
    description:
      'Connectez une application TV ou mobile sans saisir votre mot de passe : choisissez Quick Connect dans une application Jellyfin, ou ouvrez l’application de Silo sur une TV, puis saisissez le code qu’elle affiche.',
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
      `${device} est maintenant connecté avec le compte ${user}.`,
    another: 'Autoriser un autre appareil',
    steps: [
      'Sur l’appareil, choisissez Quick Connect dans une application Jellyfin, ou ouvrez l’application de Silo sur une TV.',
      'Saisissez ici le code qu’elle affiche : 6 chiffres pour Jellyfin, 8 pour Silo.',
      'Vérifiez l’appareil, puis autorisez-le.',
    ],
    stepsLabel: 'Comment ça marche',
  },
  notFound: {
    title: 'Page introuvable',
    description: 'Cette page n’existe pas.',
    home: 'Retour à l’accueil',
  },
  dashboard: {
    overview: {
      greeting: (hour: number, name: string) =>
        `${hour >= 18 || hour < 5 ? 'Bonsoir' : 'Bonjour'}, ${name}`,
      summary: (playing: number, problems: number) =>
        `${playing === 0 ? 'Rien en lecture' : playing === 1 ? 'Une lecture en cours' : `${playing} lectures en cours`}${
          problems === 0
            ? ' et rien à regarder. Tout tourne normalement.'
            : problems === 1
              ? ', et un point à regarder.'
              : `, et ${problems} points à regarder.`
        }`,
      description: 'Ce qui est en lecture sur le serveur, et ce qui s’est passé récemment.',
      conversions: 'Conversions vidéo',
      conversionsOf: (count: number, limit: number) =>
        limit > 0 ? `${count} sur ${limit}` : `${count}, sans limite`,
      bandwidth: 'Bande passante',
      bandwidthSent: (rate: string) => `${rate} envoyés aux applis`,
      bandwidthReceived: (rate: string) => `${rate} reçus des sources`,
      uptime: 'En marche depuis',
      problems: 'À regarder',
      problemsEmpty: 'Rien à regarder',
      problemsEmptyHelp:
        'Polyfin surveille ses disques, sources, guides, sauvegardes et tâches. Ce qui demande votre attention apparaît ici.',
      problemError: 'Problème',
      problemWarning: 'À vérifier',
      update: (version: string, running: string) =>
        `Polyfin ${version} est disponible. Ce serveur est en version ${running}.`,
      releaseNotes: 'Notes de version',
      open: 'Ouvrir',
      openHealth: 'Ouvrir Santé',
      serverTitle: 'État du serveur',
      version: 'Version',
      database: 'Base de données',
      databaseSize: (size: string) => `${size} sur le disque`,
      cache: 'Cache',
      cacheOf: (limit: string) => `sur ${limit}`,
      cacheNoLimit: 'sans limite',
      cacheOff: 'Désactivé',
      cacheUnit: 'Cache utilisé',
      graphics: 'Carte graphique',
      graphicsNone: 'Aucune',
      graphicsCpu: 'Les conversions passent par le processeur',
      toneMapping: 'tone mapping',
    },
    server: {
      restart: 'Redémarrer',
      updateNow: 'Mettre à jour maintenant',
      install: (version: string) => `Installer Polyfin ${version}`,
      downloading: (percent: number) => `Téléchargement : ${percent} %`,
      confirmWith: 'Confirmer avec',
      withPassword: 'Mot de passe',
      withCode: 'Code en deux étapes',
      password: 'Votre mot de passe',
      code: 'Code de votre appli, ou un code de secours',
      identityHelp: 'Chacune de ces actions le redemande.',
      restartTitle: 'Redémarrer Polyfin',
      restartText:
        'Polyfin arrête ce qu’il fait, lectures et enregistrements compris, et se termine. Les applis se reconnectent dès qu’il répond.',
      restartBy: {
        desktop: 'Polyfin Desktop le relance de lui-même.',
        confirmed:
          'La politique de redémarrage de son conteneur, ou son gestionnaire de services, le relance, comme confirmé dans Paramètres › Général.',
      },
      unknownTitle: 'Polyfin pourrait ne pas revenir',
      unknown: {
        container:
          'Polyfin ne voit pas la politique de redémarrage de son conteneur. Sans une politique qui le relance, comme unless-stopped ou always, Polyfin reste arrêté jusqu’à ce qu’on le démarre sur l’hôte.',
        other:
          'Polyfin ne sait pas si un gestionnaire de services le relance. Sans lui, Polyfin reste arrêté jusqu’à ce qu’on le démarre sur l’hôte.',
      },
      unknownHelp:
        'Une fois que c’est en place, activez « Polyfin est redémarré quand il s’arrête » dans Paramètres › Général, et cet avertissement disparaît.',
      restartConfirm: 'Redémarrer',
      restartAnyway: 'Redémarrer quand même',
      restarting: 'Redémarrage…',
      waitingTitle: 'Polyfin redémarre',
      waiting: 'En attente de la réponse de Polyfin…',
      waitingLong: (seconds: number) =>
        `Polyfin ne répond plus depuis ${seconds} secondes. Si rien ne le relance, démarrez-le sur l’hôte.`,
      installTitle: (version: string) => `Installer Polyfin ${version}`,
      installText:
        'Polyfin Desktop télécharge la nouvelle version et la vérifie, puis arrête Polyfin, l’installe et la démarre, comme « Installer Polyfin » dans son menu. Les lectures et les enregistrements s’arrêtent pendant ce temps.',
      installConfirm: 'Installer',
      installing: 'Demande en cours…',
      installWaitingTitle: (version: string) => `Installation de Polyfin ${version}`,
      installStopping:
        'Polyfin s’arrête pour installer la nouvelle version. En attente de sa réponse…',
      installFailed:
        'Polyfin Desktop n’a pas pu installer la mise à jour : le dossier des journaux dit pourquoi. Polyfin continue comme avant.',
      updateTitle: 'Mettre à jour maintenant',
      updateText:
        'Polyfin demande à l’outil de mise à jour réglé dans Paramètres › Général de mettre à jour maintenant. La suite dépend de l’outil : s’il remplace le conteneur de Polyfin, Polyfin s’arrête un moment.',
      updateConfirm: 'Envoyer la demande',
      sending: 'Envoi…',
      answer: (status: number) => `L’outil de mise à jour a répondu HTTP ${status}.`,
      answerEmpty: 'Sa réponse ne contient pas de texte.',
      watching:
        'Si l’outil remplace le conteneur de Polyfin, cette page se recharge dès que Polyfin répond de nouveau.',
      gone: 'Polyfin s’est arrêté avant la réponse de l’outil : l’outil est sans doute en train de le remplacer. En attente de la réponse de Polyfin…',
      tooSoon: 'Cette action a été demandée il y a moins d’une minute. Réessayez dans un instant.',
      close: 'Fermer',
    },
    live: {
      title: 'Lecture en cours',
      empty: 'Rien n’est en lecture pour le moment.',
      emptyHelp: 'Les lectures depuis les applis Jellyfin apparaissent ici en quelques secondes.',
      autoRefresh: 'Mise à jour toutes les 3 secondes.',
      paused: 'En pause',
      unknownTitle: 'Titre introuvable',
      movie: (year: number) => (year > 0 ? `Film, ${year}` : 'Film'),
      liveChannel: 'Direct',
      progress: (title: string) => `Position de lecture de ${title}`,
      remaining: (time: string) => `reste ${time}`,
      startedAt: (time: string) => `Commencé ${time}`,
      appOn: (app: string, device: string) => (app ? `${app} sur ${device}` : device),
      sharedLink: 'Lien partagé',
      /** A converted download, in place of the app and device. */
      downloadOn: (app: string, device: string) =>
        app ? `Téléchargement, ${app} sur ${device}` : 'Téléchargement',
      downloading: 'Téléchargement',
      downloadProgress: (title: string) => `Avancement du téléchargement de ${title}`,
      downloadNotControllable:
        'Un téléchargement converti pour être vu hors ligne. Il s’arrête si l’appli l’interrompt.',
      howItPlays: 'Comment c’est lu',
      details: 'Détails',
      delivery: {
        directPlay: 'Lecture directe',
        remux: 'Remux',
        conversion: 'Conversion',
        stream: 'Diffusion',
      },
      deliveryHelp: {
        directPlay: 'L’appli lit le fichier tel quel.',
        remux: 'Polyfin réemballe le fichier pour l’appli, sans le réencoder.',
        conversion: 'Polyfin réencode ce que l’appli ne sait pas lire.',
        stream:
          'Polyfin diffuse le fichier à l’appli. Le détail revient dès que l’appli demande la suite.',
      },
      why: 'Pourquoi',
      source: 'Source',
      sent: 'Envoyé',
      encoder: 'Encodeur',
      gpu: (method: string) => `GPU (${gpuMethodNames[method] ?? method})`,
      gpuMethod: (method: string) => gpuMethodNames[method] ?? method,
      cpu: 'Processeur',
      copied: 'Copié',
      toneMapped: 'HDR vers SDR',
      burned: 'Sous-titres incrustés',
      qualityGroup: 'Groupe de qualité',
      original: 'Originale',
      sizeLimit: 'Plus petit que la source',
      /** Shown when the app may switch between several qualities of converted video. */
      qualityPlayed: 'Qualité lue',
      switchedVersion: 'Version de relais',
      bandwidth: {
        sent: (rate: string) => `${rate} vers l’appli`,
        received: (rate: string) => `${rate} depuis la source`,
        redirected: 'L’appli lit la source elle-même',
      },
      origins: {
        addon: 'Addon',
        torrent: 'Torrent',
        localFolder: 'Dossier local',
        networkShare: 'Partage réseau',
        iptv: 'IPTV',
      } as Record<string, string>,
      peers: (count: number) => (count === 1 ? '1 pair' : `${count} pairs`),
      qualityAmong: (played: string, offered: string) => `${played}, parmi ${offered}`,
      sizeLimits: {
        steppedDown: (height: string) => `L’appli est passée sous ${height}`,
        gpu: (height: string) => `${height} au plus : converti par la carte graphique`,
        processor: (height: string) => `${height} au plus : converti par le processeur`,
        toneMapping: (height: string) => `${height} au plus : HDR converti par le processeur`,
        gpuToneMapping: (height: string) =>
          `${height} au plus : la carte graphique ne convertit le HDR à temps que jusque-là`,
        qualityGroup: (height: string) => `Groupe de qualité : ${height}`,
        serverMax: (height: string) => `Qualité max des vidéos converties : ${height}`,
        app: (height: string) => `Qualité permise par l’appli : ${height}`,
        appBitrate: (rate: string) => `Débit permis par l’appli : ${rate}`,
        userBitrate: (rate: string) => `Qualité maximale de l’utilisateur : ${rate}`,
      },
      unknown: 'Inconnu',
      stop: 'Arrêter',
      stopFor: (user: string) => `Arrêter la lecture de ${user}`,
      stopTitle: 'Arrêter cette lecture ?',
      stopConfirm: (user: string, title: string) =>
        `Arrêter « ${title} » sur l’appareil de ${user} ? La lecture s’arrête tout de suite.`,
      stopped: 'L’appli a reçu l’ordre d’arrêter.',
      message: 'Envoyer un message',
      messageTo: (user: string) => `Envoyer un message à ${user}`,
      messageLabel: (device: string) => `Message à afficher sur ${device}`,
      messageHint: 'Affiché 10 secondes sur l’appareil. 500 caractères au plus.',
      send: 'Envoyer',
      messageSent: 'Message envoyé.',
      notControllable: 'Cette appli n’accepte pas le contrôle à distance.',
      sharedNotControllable:
        'Un invité qui regarde grâce à un lien de partage. Révoquer le lien, sur la page de l’utilisateur, l’arrête.',
    },
    reasons: {
      ContainerNotSupported: 'Format de fichier non pris en charge',
      VideoCodecNotSupported: 'Codec vidéo non pris en charge',
      AudioCodecNotSupported: 'Codec audio non pris en charge',
      SubtitleCodecNotSupported: 'Format de sous-titres non pris en charge',
      AudioIsExternal: 'Audio dans un fichier à part',
      SecondaryAudioNotSupported: 'Deuxième piste audio non prise en charge',
      VideoProfileNotSupported: 'Profil vidéo non pris en charge',
      VideoLevelNotSupported: 'Niveau vidéo non pris en charge',
      VideoResolutionNotSupported: 'Résolution trop élevée',
      VideoBitDepthNotSupported: 'Profondeur de couleur non prise en charge',
      VideoFramerateNotSupported: 'Fréquence d’images non prise en charge',
      RefFramesNotSupported: 'Trop d’images de référence',
      AnamorphicVideoNotSupported: 'Vidéo anamorphique non prise en charge',
      InterlacedVideoNotSupported: 'Vidéo entrelacée non prise en charge',
      VideoCodecTagNotSupported: 'Étiquette de codec vidéo non prise en charge',
      VideoBitrateNotSupported: 'Débit vidéo trop élevé',
      AudioChannelsNotSupported: 'Trop de canaux audio',
      AudioProfileNotSupported: 'Profil audio non pris en charge',
      AudioSampleRateNotSupported: 'Fréquence audio non prise en charge',
      AudioBitDepthNotSupported: 'Résolution audio non prise en charge',
      AudioBitrateNotSupported: 'Débit audio trop élevé',
      ContainerBitrateExceedsLimit: 'Au-dessus de la limite de débit',
      UnknownVideoStreamInfo: 'Vidéo non décrite',
      UnknownAudioStreamInfo: 'Audio non décrit',
      DirectPlayError: 'Échec de la lecture directe',
      VideoRangeTypeNotSupported: 'HDR non pris en charge',
      StreamCountExceedsLimit: 'Trop de pistes',
    },
    activity: {
      title: 'Activité récente',
      openLog: 'Ouvrir le journal',
      empty: 'Aucune activité pour l’instant.',
      emptyHelp:
        'Les connexions, les lectures et les changements sur le serveur apparaissent ici au fil de l’eau.',
      warning: 'Avertissement',
      error: 'Erreur',
      filter: 'Afficher',
      filters: {
        all: 'Tout',
        problems: 'Problèmes',
        signIns: 'Connexions',
        playback: 'Lectures',
        users: 'Utilisateurs',
        server: 'Serveur et addons',
        recordings: 'Enregistrements',
      },
      search: 'Chercher dans les événements affichés',
      listLabel: 'Derniers événements',
      noMatch: 'Aucun événement ne correspond.',
      noMatchHelp: 'Choisissez un autre filtre ou videz la recherche.',
      count: (shown: number, total: number) => `${shown} événements sur ${total}.`,
      autoRefresh: 'Mise à jour toutes les 15 secondes.',
    },
  },
}

export default home
