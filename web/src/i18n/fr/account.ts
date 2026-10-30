import type en from '../en/account'

const account: typeof en = {
  account: {
    title: 'Mon compte',
    signedInAs: 'Connecté avec le compte ',
    sectionsLabel: 'Sections du compte',
    sections: {
      statistics: 'Statistiques',
      tracking: 'Suivi',
      resume: 'Continuer de regarder',
      hidden: 'Titres masqués',
      homeRows: 'Rangées d’accueil',
      metadataLanguage: 'Langue des métadonnées',
      sharedLinks: 'Liens partagés',
      reminders: 'Rappels',
      import: 'Importer depuis un autre serveur',
      appImport: 'Importer depuis une appli',
      notifications: 'Notifications',
      devices: 'Appareils',
      password: 'Mot de passe',
      twoStep: 'Connexion en deux étapes',
    },
    metadataLanguage: {
      help: 'La langue des noms, des résumés et des images des titres dans vos applis, quand l’un des addons du serveur les décrit dans cette langue ; les titres qu’aucun addon n’y décrit gardent la description du serveur. Les genres, les classifications et les personnes restent les mêmes pour tous. Les noms que Polyfin crée lui-même, comme les saisons, la suivent quand Polyfin la parle : anglais, français, allemand, espagnol, italien ou portugais.',
      label: 'Langue des métadonnées',
      server: 'Celle du serveur',
      noLanguage:
        'Aucun addon du serveur n’a encore de langue : les titres sont décrits dans celle du serveur. Un administrateur donne une langue aux addons dans Contenu › Sources.',
      saved: 'Langue des métadonnées enregistrée.',
    },
    homeRows: {
      help: 'Les rangées que l’administrateur ajoute pour vous à la page d’accueil du lecteur web. Désactivez-en une pour la retirer de votre page d’accueil, et de vos bibliothèques dans les autres applis quand elle en est aussi une. Les sections propres au lecteur web restent dans ses Paramètres › Accueil.',
      empty: 'Aucune rangée d’accueil',
      emptyHelp:
        'L’administrateur n’a ajouté aucune rangée pour vous à la page d’accueil du lecteur web.',
      shown: (name: string) => `Afficher ${name} sur votre page d’accueil`,
      on: (name: string) => `${name} est de retour sur votre page d’accueil.`,
      off: (name: string) => `${name} est retirée de votre page d’accueil.`,
    },
    watching: {
      resumeHelp:
        'Les titres en cours de votre liste « Continuer de regarder », pour les applis qui ne savent pas en retirer un. Retirer enlève un titre sans changer s’il est vu ou non. Un import depuis un service de suivi ou une appli ne le ramène qu’avec un point de reprise plus récent.',
      hiddenHelp:
        'Les titres que vous avez masqués de vos bibliothèques, rangées d’accueil, recherches et titres similaires. Ils s’ouvrent toujours depuis un lien. Dans le lecteur web, le menu ⋮ d’un titre le masque.',
      remove: 'Retirer',
      removeLabel: (name: string) => `Retirer ${name} de « Continuer de regarder »`,
      removed: (name: string) => `${name} est retiré de « Continuer de regarder ».`,
      resumeEmpty: 'Rien en cours',
      resumeEmptyHelp:
        'Les films et les épisodes que vous commencez s’affichent ici jusqu’à ce que vous les finissiez.',
      progress: (position: string, runtime: string) => `${position} sur ${runtime}`,
      showAgain: 'Afficher de nouveau',
      showLabel: (name: string) => `Afficher de nouveau ${name}`,
      shown: (name: string) => `${name} est de retour dans vos listes.`,
      hiddenEmpty: 'Aucun titre masqué',
      hiddenEmptyHelp: 'Masquez un titre depuis son menu ⋮ dans le lecteur web.',
      hiddenOn: 'Masqué',
      kinds: {
        movie: 'Film',
        series: 'Série',
        episode: 'Épisode',
        video: 'Vidéo personnelle',
        replay: 'Replay',
      },
    },
    reminders: {
      help: 'Les émissions de TV en direct dont vous avez demandé le rappel depuis leur page dans le lecteur web. Chaque rappel arrive sur vos cibles qui ont choisi « Début d’une émission », avec l’avance réglée sous Notifications.',
      empty: 'Aucun rappel',
      emptyHelp:
        'Sur la page d’une émission à venir dans le lecteur web, « Me le rappeler » en ajoute un.',
      everyAiring: (time: string) => `Chaque diffusion vers ${time}`,
      next: 'Prochaine ',
      noAiring: 'Aucune diffusion dans le guide',
      delete: 'Supprimer',
      deleteLabel: (name: string) => `Supprimer le rappel de ${name}`,
      deleted: (name: string) => `Le rappel de ${name} est supprimé.`,
      settingsTitle: 'Rappels',
      before: 'Me prévenir avant',
      beforeHelp: 'Combien de temps avant le début d’une émission son rappel arrive.',
      minutes: (minutes: number) =>
        minutes === 1 ? '1 minute' : minutes === 60 ? '1 heure' : `${minutes} minutes`,
      onApps: 'Afficher aussi les rappels dans mes applis',
      onAppsHelp:
        'Chaque rappel s’affiche aussi dans vos applis Jellyfin qui acceptent les messages, comme le lecteur web tant qu’il est ouvert, à la manière d’« Envoyer un message ».',
      noTarget:
        'Aucune de vos cibles n’a choisi « Début d’une émission », et vos applis sont désactivées : vos rappels ne vous arriveraient nulle part.',
      saved: 'Réglages des rappels enregistrés.',
    },
    importEvery: (minutes: number) =>
      minutes < 60
        ? `toutes les ${minutes} minutes`
        : minutes === 60
          ? 'toutes les heures'
          : `toutes les ${minutes / 60} heures`,
    serverImport: {
      description:
        'Reprenez vos films et épisodes vus, vos points de reprise et vos favoris depuis un serveur Jellyfin, Emby ou Plex que vous utilisiez, ou vos films depuis votre export Letterboxd. Ils vont dans votre compte seulement, et complètent seulement ce que Polyfin a déjà. Rien n’est écrit sur l’autre serveur.',
      serverKind: 'Serveur',
      address: 'Adresse du serveur',
      addressHelp:
        'L’adresse à laquelle le serveur s’ouvre dans un navigateur, par exemple http://192.168.1.10:8096.',
      name: 'Votre nom d’utilisateur sur ce serveur',
      password: 'Votre mot de passe sur ce serveur',
      passwordHelp:
        'Il sert seulement à cette importation, et n’est jamais gardé. Polyfin se connecte avec votre compte, comme les applis du serveur, puis se déconnecte. Laissez vide si votre compte n’en a pas.',
      start: 'Importer mon historique',
      starting: 'Lancement…',
      started: 'L’importation a commencé. Vous pouvez quitter cette page : elle continue.',
      lastTitle: 'Votre dernière importation',
      from: (server: string) => `Depuis ${server}`,
    },
    plexImport: {
      help: 'Connectez-vous à Plex sur la page de Plex, puis choisissez le serveur. Polyfin y lit, en votre nom, les films et épisodes que vous avez lus, marqués comme vus ou synchronisés, vos points de reprise et vos notes, et votre Watchlist de Plex. Polyfin ne voit jamais votre mot de passe Plex : ce que Plex lui donne sert seulement à cette importation, puis est déconnecté.',
      signIn: 'Se connecter avec Plex',
      signingIn: 'Demande à Plex…',
      approve:
        'Connectez-vous à Plex sur sa page, qui s’ouvre dans un nouvel onglet, puis revenez ici.',
      open: 'Ouvrir la page de connexion de Plex',
      waiting: 'En attente de votre connexion à Plex…',
      signedInAs: (name: string) => `Connecté à Plex en tant que ${name}.`,
      profile: 'Profil Plex Home',
      profileHelp:
        'Le profil dont les données de visionnage sont reprises. Un profil géré n’a pas de connexion à lui : choisissez-le ici.',
      managed: (name: string) => `${name} (géré)`,
      pin: 'Code PIN du profil',
      pinHelp: 'Le code PIN que ce profil demande sur Plex.',
      useProfile: 'Utiliser ce profil',
      switching: 'Changement…',
      server: 'Serveur',
      serverHelp:
        'Les serveurs Plex auxquels ce profil a accès. Polyfin joint celui choisi aux adresses que Plex donne pour lui.',
      owned: (name: string) => `${name} (le vôtre)`,
      noServers: 'Ce profil n’a accès à aucun serveur Plex.',
      watchlist: 'Watchlist vers ma liste « À regarder »',
      watchlistHelp:
        'Les films et séries de votre Watchlist de Plex vont dans votre liste « À regarder » de Polyfin.',
      cancel: 'Annuler',
    },
    letterboxd: {
      option: 'Letterboxd (fichier d’export)',
      help: 'Dans Letterboxd, ouvrez Settings › Import & Export et choisissez Export your data : Letterboxd donne un fichier ZIP. Choisissez-le ici tel quel. Polyfin le lit en mémoire, et ne le garde jamais.',
      watchedHelp:
        'Les films que vous avez vus sont marqués vus, à la dernière date que donne votre journal, autant de fois qu’il les note. Chaque film est cherché par son nom et son année dans les addons de métadonnées du serveur, et retenu quand ils connaissent exactement un film de ce nom et de cette année.',
      noSearch:
        'Aucun addon de métadonnées de ce serveur ne cherche de films : aucun film de l’export ne peut être trouvé. Demandez à un administrateur d’en ajouter un.',
      file: 'Fichier d’export',
      fileHelp: 'Le fichier ZIP que donne Letterboxd, de 20 Mo au plus.',
      choose: 'Choisir le fichier',
      chooseAnother: 'Choisir un autre fichier',
      tooLarge: 'Ce fichier dépasse 20 Mo : choisissez le fichier ZIP que donne Letterboxd.',
      watchlist: 'Watchlist vers ma liste « À regarder »',
      watchlistHelp:
        'Les films de votre watchlist Letterboxd vont dans « À regarder » de vos applis Jellyfin, jusqu’à ce que vous les commenciez.',
      ratings: 'Notes',
      ratingsHelp:
        'Vos étoiles deviennent votre note sur 10 : 3,5 étoiles donnent 7. Une note que vous avez déjà dans Polyfin reste.',
      likes: 'Films aimés en favoris',
      likesHelp: 'Les films que vous avez aimés sur Letterboxd deviennent vos favoris.',
      forUser: {
        watchedHelp:
          'Les films vus sont marqués vus, à la dernière date que donne son journal, autant de fois qu’il les note. Chaque film est cherché par son nom et son année dans les addons de métadonnées du serveur, et retenu quand ils connaissent exactement un film de ce nom et de cette année.',
        watchlist: 'Watchlist vers sa liste « À regarder »',
        watchlistHelp:
          'Les films de sa watchlist Letterboxd vont dans « À regarder » de ses applis Jellyfin, jusqu’à ce qu’ils soient commencés.',
        ratingsHelp:
          'Ses étoiles deviennent sa note sur 10 : 3,5 étoiles donnent 7. Une note déjà donnée dans Polyfin reste.',
        likesHelp: 'Les films aimés sur Letterboxd deviennent ses favoris.',
      },
      start: 'Importer depuis Letterboxd',
      starting: 'Envoi…',
      started: 'L’importation a commencé. Vous pouvez quitter cette page : elle continue.',
      currentTitle: 'Importation en cours',
      lastTitle: 'Dernière importation',
      ownLastTitle: 'Votre dernière importation',
      from: 'Depuis un export Letterboxd',
      startedLabel: 'Commencée',
      endedLabel: 'Terminée',
      startedBy: (name: string) => `Lancée par ${name}`,
      states: { running: 'En cours', done: 'Terminée', failed: 'Échouée' },
      problem:
        'Polyfin a rencontré une erreur pendant l’importation : le journal du serveur en dit plus. Ce qui a été importé est conservé.',
      runningHelp:
        'Vous pouvez quitter cette page : l’importation continue. Si Polyfin redémarre entre-temps, elle s’arrête : ce qu’elle a importé reste, et importer à nouveau n’ajoute rien deux fois.',
      searching: (searched: number, movies: number) =>
        `Recherche des films : ${searched} sur ${movies}.`,
      searchingLabel: 'Films cherchés',
      summary: (movies: number, added: string) =>
        `${movies <= 1 ? `${movies} film` : `${movies} films`} : ${added}.`,
      played: (count: number) => (count <= 1 ? `${count} marqué vu` : `${count} marqués vus`),
      addedToWatchlist: (count: number) =>
        count <= 1 ? `${count} ajouté à « À regarder »` : `${count} ajoutés à « À regarder »`,
      rated: (count: number) => (count <= 1 ? `${count} noté` : `${count} notés`),
      favorites: (count: number) =>
        count <= 1 ? `${count} mis en favori` : `${count} mis en favoris`,
      unmatched: (count: number) =>
        count <= 1 ? `${count} film non trouvé` : `${count} films non trouvés`,
      unmatchedTitle: 'Films non trouvés',
      unmatchedHelp:
        'Ces films n’ont pas été importés. Chacun est cherché par son nom et son année, et retenu seulement quand les addons de métadonnées du serveur connaissent exactement un film de ce nom et de cette année. Ouvrez-le sur Letterboxd pour les vérifier.',
      movieColumn: 'Film',
      reasonColumn: 'Raison',
      onLetterboxd: 'Sur Letterboxd',
      reasons: {
        not_found: 'Non trouvé',
        ambiguous: 'Plusieurs films de ce nom et de cette année',
        search_failed: 'Échec de la recherche',
      },
    },
    appImport: {
      description:
        'Reprenez ce que vous avez regardé dans une appli basée sur des addons : les films et épisodes vus avec leurs dates, vos points de reprise, et la bibliothèque de l’appli, qui va dans « À regarder » de vos applis Jellyfin, reconnus par IMDb, TMDB ou Kitsu. Ils complètent seulement ce que Polyfin a déjà, et rien n’est écrit dans l’appli.',
      app: 'Appli',
      apps: { nuvio: 'Nuvio', stremio: 'Stremio' },
      backend: 'Serveur Nuvio',
      backendHelp:
        'Laissez vide pour celui de Nuvio. Un serveur auto-hébergé fonctionne aussi, à son adresse, y compris sur le réseau local.',
      email: 'E-mail',
      password: 'Mot de passe',
      passwordHelp:
        'Il sert seulement à lire le compte, et n’est jamais gardé. Polyfin se connecte comme les applis Nuvio, lit le compte, puis se déconnecte, sauf si vous restez connecté.',
      signIn: 'Se connecter à Nuvio',
      signingIn: 'Connexion…',
      chooseTitle: 'Choisir quoi importer',
      chooseHelp: {
        nuvio:
          'Nuvio garde un historique par profil : choisissez celui à importer. Polyfin garde ce qu’il a lu quelques minutes seulement.',
        stremio:
          'Stremio garde un historique par profil, ou un seul pour tout le compte sans profils : choisissez celui à importer. Polyfin garde ce qu’il a lu quelques minutes seulement.',
      },
      stayConnected: 'Rester connecté',
      stayConnectedHelp: {
        nuvio: (every: string) =>
          `Polyfin garde la session, chiffrée, jamais votre mot de passe, et importe de nouveau le profil choisi ${every}. Il ne fait que lire le compte. Vous pouvez vous déconnecter à tout moment.`,
        stremio: (every: string) =>
          `Polyfin garde la clé d’authentification, chiffrée, et pour un profil à code PIN la session de ce profil, jamais le code, et importe de nouveau le profil choisi ${every}. Il ne fait que lire le compte. Vous pouvez vous déconnecter à tout moment. Les conditions de Stremio interdisent d’extraire ses données par des moyens automatisés : lire votre compte ${every} rend cette zone grise plus marquée qu’une importation ponctuelle.`,
      },
      staysConnected: (every: string) =>
        `Vous restez connecté : ce profil est importé de nouveau ${every}, de la même façon. Les addons ne sont installés que maintenant.`,
      wholeAccount: 'Tout le compte',
      profile: 'Profil',
      profileNumber: (id: string) => `Profil ${id}`,
      profileCounts: (played: number, resumes: number, library: number) =>
        `${played} vus, ${resumes <= 1 ? `${resumes} point de reprise` : `${resumes} points de reprise`}, ${library} dans la bibliothèque.`,
      lockedProfile: (name: string) => `${name} (code PIN)`,
      pin: 'Code PIN',
      pinHelp:
        'Ce profil demande son code PIN. Polyfin ne l’envoie qu’à Stremio, pour ouvrir le profil, et ne le garde jamais.',
      unlock: 'Ouvrir ce profil',
      unlocking: 'Ouverture…',
      addons: 'Addons',
      addonsHelp:
        'Chaque addon coché devient un addon propre au compte, sous Mes sources. Son adresse peut contenir des réglages et des clés : Polyfin ne la montre jamais en entier.',
      addonsOff:
        'Les addons personnels sont désactivés pour ce compte : les addons de l’appli ne sont pas proposés.',
      noAddons: 'Ce profil n’a aucun addon.',
      back: 'Se reconnecter',
      start: 'Importer ce profil',
      starting: 'Lancement…',
      started: 'L’importation a commencé. Vous pouvez quitter cette page : elle continue.',
      currentTitle: 'Importation en cours',
      lastTitle: 'Dernière importation',
      ownLastTitle: 'Votre dernière importation',
      from: (app: string, profile: string) =>
        profile === '' ? `Depuis ${app}` : `Depuis ${app}, profil ${profile}`,
      startedLabel: 'Commencée',
      endedLabel: 'Terminée',
      startedBy: (name: string) => `Lancée par ${name}`,
      states: { running: 'En cours', done: 'Terminée', failed: 'Échouée' },
      problem:
        'Polyfin a rencontré une erreur pendant l’importation : le journal du serveur en dit plus. Ce qui a été importé est conservé.',
      runningHelp:
        'Vous pouvez quitter cette page : l’importation continue. Si Polyfin redémarre entre-temps, elle s’arrête : ce qu’elle a importé reste, et importer à nouveau n’ajoute rien deux fois.',
      read: (count: number) => (count <= 1 ? `${count} titre lu.` : `${count} titres lus.`),
      counts: (read: number, played: number, resumed: number, watchlist: number) =>
        `${read <= 1 ? `${read} titre lu` : `${read} titres lus`} : ${played <= 1 ? `${played} marqué vu` : `${played} marqués vus`}, ${resumed <= 1 ? `${resumed} point de reprise` : `${resumed} points de reprise`}, ${watchlist <= 1 ? `${watchlist} ajouté` : `${watchlist} ajoutés`} à « À regarder ».`,
      unmatched: (count: number) =>
        count <= 1 ? `${count} titre introuvable` : `${count} titres introuvables`,
      unmatchedTitle: 'Titres introuvables',
      unmatchedHelp:
        'Ces titres n’ont pas été importés : l’appli les nomme par un identifiant que Polyfin ne reconnaît pas, ou aucun titre de Polyfin ne l’a. Pour une série, l’appli peut aussi désigner les épisodes vus par leur place dans une liste d’épisodes que les addons de Polyfin donnent autrement.',
      titleColumn: 'Titre',
      kindColumn: 'Type',
      reasonColumn: 'Raison',
      reasons: {
        other_identifier: 'Pas un identifiant IMDb, TMDB ou Kitsu',
        not_found: 'Absent de Polyfin',
        episode_list:
          'Ses épisodes ne correspondent pas à la liste de Polyfin : aucun n’a été marqué vu',
      },
      addonInstalled: 'ajouté',
      addonWaiting: 'en attente',
      addonProblems: {
        not_allowed: 'non ajouté : les addons personnels ont été désactivés',
        exists: 'déjà installé',
        invalid_address: 'non ajouté : ce n’est pas l’adresse d’un addon',
        private_network:
          'non ajouté : sur un réseau local, que seule l’importation d’un administrateur atteint',
        unreachable: 'non ajouté : l’addon n’a pas répondu',
        invalid_manifest: 'non ajouté : ce n’est pas un addon',
        internal: 'non ajouté : Polyfin a rencontré une erreur',
      },
      stremio: {
        way: 'Se connecter avec',
        ways: {
          link: 'Un code de liaison',
          password: 'E-mail et mot de passe',
          key: 'Une clé d’authentification',
        },
        linkHelp:
          'Polyfin demande un code à Stremio, comme les applis TV de Stremio. Vous l’approuvez dans un navigateur connecté à Stremio : cela fonctionne quelle que soit la façon dont votre compte se connecte, Facebook et Apple compris, et Polyfin ne voit jamais votre mot de passe.',
        getCode: 'Obtenir un code de liaison',
        gettingCode: 'Demande à Stremio…',
        codeLabel: 'Code de liaison',
        approve:
          'Ouvrez le lien dans un navigateur connecté à Stremio, vérifiez qu’il affiche ce code, et approuvez-le.',
        openLink: 'Ouvrir le lien',
        waiting:
          'En attente de votre approbation. Le code dure 5 minutes ; une fois approuvé, Polyfin lit le compte, puis se déconnecte, sauf si vous restez connecté.',
        cancel: 'Annuler',
        email: 'E-mail',
        password: 'Mot de passe',
        passwordHelp:
          'Il sert seulement à lire le compte, et n’est jamais gardé. Polyfin se connecte comme les applis Stremio, lit le compte, puis se déconnecte, sauf si vous restez connecté. Un compte créé avec Facebook ou Apple n’a pas de mot de passe : utilisez un code de liaison.',
        signIn: 'Se connecter à Stremio',
        signingIn: 'Connexion…',
        key: 'Clé d’authentification',
        keyHelp:
          'La clé que l’appli web de Stremio garde une fois connectée : dans la console de votre navigateur, sur cette appli, JSON.parse(localStorage.getItem("profile")).auth.key la donne. Elle sert à lire le compte, et n’est gardée que si vous restez connecté. Polyfin ne la déconnecte jamais, ce qui déconnecterait votre navigateur.',
        readKey: 'Lire le compte',
        reading: 'Lecture…',
      },
      connections: {
        title: 'Applis connectées',
        help: (every: string) =>
          `Importées ${every}, comme lors de la première importation : le même profil, sans leurs addons. Polyfin ne fait que les lire.`,
        connectedAs: 'Connecté avec le compte ',
        connected: 'Connecté',
        connectedOn: (date: string) => ` depuis le ${date}`,
        connectedBy: (name: string) => ` par ${name}`,
        status: {
          connected: 'Connecté',
          importing: 'Importation',
          paused: 'En pause',
          stopped: 'À reconnecter',
        },
        never: 'Pas encore importé.',
        lastImport: 'Dernière importation ',
        colon: ' : ',
        nextImport: 'Prochaine importation ',
        paused:
          'Le serveur ne permet plus aux utilisateurs d’importer leur historique : cette connexion est en pause jusqu’à ce qu’il le permette.',
        stopped: (app: string) =>
          `${app} a refusé la connexion que Polyfin gardait : rien n’est importé tant que vous ne vous reconnectez pas ci-dessous, avec « Rester connecté » coché. Se déconnecter l’oublie.`,
        problems: {
          unreachable: (app: string) =>
            `${app} n’a pas répondu lors de la dernière importation. La prochaine réessaie.`,
          rate_limited: (app: string) =>
            `${app} a demandé à Polyfin d’attendre lors de la dernière importation. La prochaine réessaie.`,
          unexpected: (app: string) =>
            `${app} a répondu à la dernière importation d’une manière que cette version de Polyfin ne lit pas. La prochaine réessaie.`,
          profile_missing: (app: string) =>
            `Le profil importé n’est plus dans le compte ${app}. Reconnectez-vous pour en choisir un autre.`,
          profile_locked: (app: string) =>
            `Le profil importé demande désormais un code PIN, ou un autre, dans ${app}. Reconnectez-vous pour le choisir avec son code.`,
          sign_in_refused: (app: string) => `${app} a refusé la connexion que Polyfin gardait.`,
          internal: () =>
            'Polyfin a rencontré une erreur pendant la dernière importation : le journal du serveur en dit plus. La prochaine réessaie.',
        },
        importNow: 'Importer maintenant',
        importing: 'Importation…',
        disconnect: 'Déconnecter',
        disconnectTitle: (app: string) => `Déconnecter ${app} ?`,
        disconnectBody: (app: string) =>
          `Polyfin arrête d’importer ce compte ${app} et oublie ce qu’il gardait pour le lire, après avoir fermé la session qu’il avait ouverte. Une clé d’authentification que vous avez collée est seulement oubliée. Ce qui a été importé reste.`,
        disconnected: (app: string) => `${app} déconnecté.`,
      },
    },
    sharedLinksHelp:
      'Partagez un film ou un épisode avec quelqu’un qui n’a pas de compte : il le regarde dans son navigateur grâce à un lien qui expire, et compte parmi vos lectures simultanées.',
    statisticsHelp:
      'Ce que vous avez regardé sur ce serveur : heures, titres, applis et appareils, et vos lectures récentes.',
    notificationsHelp:
      'Où Polyfin vous signale les nouveaux épisodes des séries que vous suivez, vos enregistrements et vos lectures, vos connexions qui s’arrêtent, les blocages de votre compte et les nouveaux appareils où il se connecte, vous rappelle les émissions que vous avez choisies, et vous envoie le résumé de votre semaine et le bilan de votre mois et de votre année.',
    devicesHelp:
      'Les applications connectées à votre compte. Déconnecter un appareil lui demande votre mot de passe à la prochaine ouverture.',
    passwordHelp: 'Changer de mot de passe déconnecte tous vos appareils Jellyfin.',
    currentPassword: 'Mot de passe actuel',
    newPassword: 'Nouveau mot de passe',
    confirmPassword: 'Confirmer le nouveau mot de passe',
    changePassword: 'Changer le mot de passe',
    changingPassword: 'Modification…',
    passwordChanged: 'Votre mot de passe a été modifié.',
    twoStep: {
      help: 'Après votre mot de passe, cette appli d’administration demande un code d’une application d’authentification sur votre téléphone. Les applis Jellyfin ne peuvent pas en demander : une fois activée, elles se connectent avec Quick Connect, que vous approuvez ici, et votre mot de passe seul ne les connecte plus. Les applis et navigateurs déjà connectés le restent.',
      off: 'Désactivée',
      on: 'Activée',
      offText: 'Votre mot de passe seul vous connecte.',
      onText:
        'Vous vous connectez avec votre mot de passe, puis un code de votre application d’authentification.',
      required:
        'Les administrateurs doivent utiliser la connexion en deux étapes sur ce serveur. Activez-la maintenant, ou elle vous sera demandée à votre prochaine connexion.',
      requiredOn:
        'Les administrateurs doivent utiliser la connexion en deux étapes sur ce serveur : elle reste activée.',
      unreadable:
        'POLYFIN_SECRET_KEY ne peut plus lire la clé de votre application d’authentification, comme après une restauration sous une autre clé : seuls vos codes de secours vous connectent. Désactivez la connexion en deux étapes avec votre mot de passe, puis réactivez-la.',
      turnOn: 'Activer',
      starting: 'Préparation…',
      scan: 'Scannez ce code QR avec votre application d’authentification, ou saisissez la clé à la main. Polyfin a créé le code lui-même : la clé ne part nulle part ailleurs.',
      qrLabel: 'Code QR de la clé, pour votre application d’authentification',
      key: 'Clé',
      openInApp: 'Ouvrir dans une application d’authentification de cet appareil',
      code: 'Code de l’application',
      codeHelp:
        'Le code à 6 chiffres que l’application affiche pour Polyfin. Il change toutes les 30 secondes.',
      confirm: 'Activer',
      confirming: 'Vérification…',
      turnedOn: 'La connexion en deux étapes est activée.',
      recoveryTitle: 'Vos codes de secours',
      recoveryHelp:
        'Chaque code vous connecte une fois, à la place d’un code de votre application, si vous perdez votre téléphone. Gardez-les en lieu sûr, par exemple dans un gestionnaire de mots de passe : ils ne sont affichés que maintenant.',
      copy: 'Copier les codes',
      copied: 'Les codes sont copiés.',
      copyFailed: 'Les codes n’ont pas pu être copiés : sélectionnez-les et copiez-les à la main.',
      saved: 'Je les ai enregistrés',
      codesLeft: (n: number) =>
        n === 1 ? '1 code de secours restant' : `${n} codes de secours restants`,
      newCodes: 'Nouveaux codes de secours',
      newCodesTitle: 'Créer de nouveaux codes de secours ?',
      newCodesText: 'Les nouveaux codes annulent les précédents.',
      making: 'Création…',
      turnOff: 'Désactiver',
      turnOffTitle: 'Désactiver la connexion en deux étapes ?',
      turnOffText: 'Votre mot de passe seul vous connecte alors, ici et dans les applis Jellyfin.',
      turningOff: 'Désactivation…',
      turnedOff: 'La connexion en deux étapes est désactivée.',
      confirmWith: 'Confirmer avec',
      withCode: 'Un code',
      withPassword: 'Mon mot de passe',
      codeOrRecovery: 'Code de l’application, ou code de secours',
      password: 'Mot de passe',
    },
    tracking: {
      title: 'Suivi',
      description:
        'Connectez les services qui gardent la trace de ce que vous regardez et écoutez. Polyfin indique à chaque service connecté les films et les épisodes que vous regardez, ou les morceaux que vous écoutez, dans vos applications Jellyfin.',
      loading: 'Chargement des services de suivi…',
      status: {
        connected: 'Connecté',
        notConnected: 'Non connecté',
        waiting: 'En attente du code',
        waitingSignIn: 'En attente de vous',
        reconnect: 'À reconnecter',
        unreachable: 'Nouvelles tentatives',
        unavailable: 'Non configuré',
        appRefused: 'Application refusée',
      },
      unavailable: (name: string) =>
        `${name} n’est pas encore configuré sur ce serveur : un administrateur doit d’abord ajouter l’application ${name} dans les paramètres.`,
      setUpApp: (name: string) => `Configurer l’application ${name}`,
      codeIntro: (name: string) =>
        `Connectez votre compte ${name} : Polyfin vous donne un code à saisir sur le site de ${name}.`,
      keyIntro: (name: string) => `Connectez votre compte ${name} avec votre clé d’API ${name}.`,
      signInIntro: (name: string) =>
        `Connectez votre compte ${name} : connectez-vous sur le site de ${name} et autorisez Polyfin à y envoyer les morceaux que vous écoutez.`,
      tokenIntro: (name: string) =>
        `Connectez votre compte ${name} avec votre jeton d’utilisateur ${name}, pour y envoyer les morceaux que vous écoutez.`,
      keyLabel: (name: string) => `Clé d’API ${name}`,
      tokenLabel: (name: string) => `Jeton d’utilisateur ${name}`,
      savedKeyHelp:
        'Vous seul pouvez l’afficher. Pour utiliser une autre clé, déconnectez-vous, puis connectez-vous de nouveau.',
      keyHelp: {
        mdblist: 'Elle se trouve sur mdblist.com, dans Preferences, sous API key.',
        publicmetadb: 'Elle se trouve dans votre compte PublicMetaDB.',
        listenbrainz: 'Il se trouve sur listenbrainz.org, dans vos paramètres, sous User token.',
      },
      connect: 'Connecter',
      connecting: 'Connexion…',
      checking: 'Vérification de la clé…',
      reconnect: 'Reconnecter',
      newCode: 'Obtenir un nouveau code',
      signInAgain: 'Recommencer',
      enterCode: (site: string) => `Allez sur ${site}, connectez-vous et saisissez ce code :`,
      codeLabel: 'Code à saisir',
      openSite: (site: string) => `Ouvrir ${site}`,
      waiting: 'Cette page se met à jour toute seule une fois le code saisi.',
      expires: (when: string) => `Le code expire ${when}.`,
      codeEnded:
        'Le code a expiré ou a été refusé avant d’être saisi. Obtenez un nouveau code pour réessayer.',
      signInStep: (site: string) =>
        `Allez sur ${site}, connectez-vous si on vous le demande et autorisez Polyfin à utiliser votre compte.`,
      signInWaiting: 'Cette page se met à jour toute seule une fois l’autorisation donnée.',
      signInExpires: (when: string) => `Cette demande expire ${when}.`,
      signInEnded:
        'Polyfin n’a pas été autorisé à temps, ou a été refusé. Recommencez pour réessayer.',
      pinIntro: (name: string) =>
        `Connectez vos listes ${name} : autorisez Polyfin sur le site de ${name}, puis collez ici le code qu’il vous montre.`,
      redirectIntro: (name: string) =>
        `Connectez votre liste ${name} : autorisez Polyfin sur le site de ${name}, qui vous ramène ici.`,
      pinStep: (site: string) =>
        `Allez sur ${site}, connectez-vous si on vous le demande, autorisez Polyfin, puis copiez ici le code affiché.`,
      redirectStep: (site: string) =>
        `Allez sur ${site}, connectez-vous si on vous le demande et autorisez Polyfin : vous revenez ici une fois l’autorisation donnée.`,
      codeField: (name: string) => `Code ${name}`,
      codeFieldHelp: (name: string) => `Le code que ${name} affiche une fois Polyfin autorisé.`,
      addressField: 'Adresse ou code',
      addressFieldHelp: (name: string) =>
        `Si votre navigateur n’a pas pu ouvrir la page où ${name} l’a envoyé, copiez ici l’adresse de cette page depuis la barre d’adresse.`,
      pasteEnded:
        'Cette demande a expiré avant que Polyfin soit autorisé. Recommencez pour réessayer.',
      invalidCode: (name: string) =>
        `${name} n’a pas accepté ce code, ou il correspond à une demande précédente. Recommencez pour en obtenir un nouveau.`,
      tokenExpires: (date: string) => `, jusqu’au ${date}`,
      returning: (name: string) => `Connexion de votre compte ${name}…`,
      connectedAs: 'Connecté avec le compte ',
      connectedWithKey: 'Connecté avec votre clé d’API',
      connected: 'Connecté',
      connectedOn: (date: string) => ` le ${date}`,
      lastSent: ', dernier envoi ',
      nothingSent: ', rien d’envoyé pour l’instant',
      connectedToast: (name: string) => `${name} est connecté.`,
      disconnectedToast: (name: string) => `${name} est déconnecté.`,
      problemReconnect: (name: string) =>
        `${name} n’accepte plus cette connexion, donc plus rien ne lui est envoyé. Reconnectez-vous pour reprendre l’envoi de ce que vous regardez.`,
      problemUnreachable: (name: string) =>
        `${name} n’a pas pu être joint ces derniers temps. Polyfin réessaie, et envoie ce que vous avez regardé dès qu’il répond.`,
      disconnect: 'Déconnecter',
      disconnectTitle: (name: string) => `Déconnecter ${name} ?`,
      disconnectBody: (name: string) =>
        `Polyfin cesse de lui indiquer ce que vous regardez. Ce qu’il a déjà reste sur ${name}.`,
      invalidKey: (name: string) =>
        `${name} n’a pas accepté cette clé. Copiez-la de nouveau depuis votre compte ${name}.`,
      invalidToken: (name: string) =>
        `${name} n’a pas accepté ce jeton. Copiez-le de nouveau depuis vos paramètres ${name}.`,
      serviceUnreachable: (name: string) =>
        `${name} n’a pas pu être joint. Réessayez dans quelques minutes.`,
      notAvailable: (name: string) =>
        `${name} n’est pas configuré sur ce serveur : un administrateur doit d’abord ajouter l’application ${name}.`,
      appRefused: (name: string) =>
        `${name} a refusé l’application de ce serveur. Un administrateur doit vérifier ses identifiants dans Paramètres › Suivi.`,
      history: {
        toggle: (name: string) => `Importer mon historique ${name}`,
        toggleHelp: (name: string, every: string) =>
          `Marque comme vu dans Polyfin ce que vous avez regardé sur ${name}, et reprend là où vous vous êtes arrêté, pour que Reprendre et À suivre correspondent. Importé ${every} ; rien n’est renvoyé à ${name}.`,
        listsToggle: (name: string) => `Importer mes listes ${name}`,
        listsToggleHelp: (name: string, every: string) =>
          `Marque comme vus dans Polyfin les épisodes que vos listes ${name} comptent comme vus : jusqu’à la progression de chaque entrée pour Watching, On hold et Dropped, tous pour Completed. Importé ${every} ; l’importation ne fait qu’ajouter.`,
        listCounts: (played: number, unmapped: number) =>
          `${played <= 1 ? `${played} titre marqué vu` : `${played} titres marqués vus`}, ${unmapped <= 1 ? `${unmapped} titre introuvable` : `${unmapped} titres introuvables`} dans Polyfin.`,
        importNow: 'Importer maintenant',
        importing: 'Importation…',
        importingStatus: 'Importation de votre historique…',
        never: 'Pas encore importé.',
        lastImport: 'Dernière importation',
        colon: ' : ',
        nextImport: 'Prochaine importation',
        counts: (played: number, resumed: number, unmapped: number) =>
          `${played <= 1 ? `${played} titre marqué vu` : `${played} titres marqués vus`}, ${resumed <= 1 ? `${resumed} point de reprise` : `${resumed} points de reprise`}, ${unmapped <= 1 ? `${unmapped} titre introuvable` : `${unmapped} titres introuvables`} dans Polyfin.`,
        problem: {
          reconnect: (name: string) =>
            `${name} a refusé la connexion lors de la dernière importation. Reconnectez-vous pour importer votre historique.`,
          unreachable: (name: string) =>
            `${name} n’a pas pu être lu entièrement lors de la dernière importation. Ce qui a été lu a été importé ; la prochaine importation réessaie.`,
          rate_limited: (name: string) =>
            `${name} a demandé à Polyfin d’attendre lors de la dernière importation. Ce qui a été lu a été importé ; la prochaine importation réessaie.`,
        },
        watchlist: {
          lists: {
            trakt: 'watchlist',
            simkl: 'liste Plan to Watch',
            anilist: 'liste Plan to Watch',
            myanimelist: 'liste Plan to Watch',
          },
          toggle: (name: string, list: string) => `Importer ma ${list} ${name}`,
          toggleHelp: (name: string, list: string) =>
            `Place les films et séries de votre ${list} ${name} dans « À regarder » de vos applis Jellyfin, à chaque importation. Un titre en sort dès que vous le commencez, ou dès que ${name} ne le liste plus. Rien n’est renvoyé à ${name}.`,
          counts: (added: number, unmapped: number) =>
            `${added <= 1 ? `${added} ajouté` : `${added} ajoutés`} à « À regarder », ${unmapped <= 1 ? `${unmapped} titre introuvable` : `${unmapped} titres introuvables`} dans Polyfin.`,
          added: (added: number) =>
            `${added <= 1 ? `${added} ajouté` : `${added} ajoutés`} à « À regarder ».`,
        },
        ratings: {
          toggle: 'Synchroniser mes notes',
          toggleHelp: (name: string) =>
            `Envoie à ${name} les notes que vous donnez aux films et aux séries, et donne à vos titres sans note vos notes ${name} à chaque importation. Une série que ${name} découpe en plusieurs entrées est ignorée.`,
          counts: (rated: number, skipped: number) =>
            `${rated <= 1 ? `${rated} titre noté` : `${rated} titres notés`}, ${skipped <= 1 ? `${skipped} note ignorée` : `${skipped} notes ignorées`}.`,
        },
      },
    },
  },
  shareLinks: {
    share: 'Partager un titre',
    off: 'Vous ne pouvez pas créer de lien de partage pour le moment : un administrateur les a désactivés pour ce serveur ou pour votre compte. Les liens encore actifs ne sont pas lus en attendant.',
    empty: 'Aucun lien partagé',
    emptyHelp:
      '« Partager un titre » donne un film ou un épisode grâce à un lien. Dans le lecteur web, la page d’un film ou d’un épisode propose aussi « Partager ».',
    userEmpty: 'Cet utilisateur n’a aucun lien partagé.',
    expiresLabel: 'Expire',
    expiredLabel: 'Expiré',
    revokedLabel: 'Révoqué',
    plays: (count: number) => (count <= 1 ? `${count} lecture` : `${count} lectures`),
    viewers: (count: number) =>
      count === 1 ? '1 spectateur à la fois' : `${count} spectateurs à la fois`,
    states: { active: 'Actif', expired: 'Expiré', revoked: 'Révoqué' },
    revoke: 'Révoquer',
    revokeLabel: (title: string) => `Révoquer le lien vers ${title}`,
    revokeTitle: 'Révoquer ce lien partagé ?',
    revokeConfirm:
      'Le lien cesse aussitôt de fonctionner, et la personne qui regarde grâce à lui est arrêtée en quelques secondes.',
    revoking: 'Révocation…',
    revoked: 'Le lien partagé est révoqué.',
    search: 'Chercher dans vos bibliothèques',
    searchHelp: 'Un film, ou une série pour choisir l’un de ses épisodes.',
    searchButton: 'Chercher',
    searching: 'Recherche…',
    noTitles: 'Aucun film ni aucune série trouvé.',
    titlesFound: (term: string) => `Titres trouvés pour ${term}`,
    pickTitle: (name: string, details: string) => `Partager ${name}, ${details}`,
    episodesOf: (series: string) => `Épisodes de ${series}`,
    noEpisodes: 'Cette série n’a aucun épisode que vous pouvez partager.',
    back: 'Retour aux résultats',
    chosen: 'Titre',
    change: 'Changer',
    expires: 'Expire après',
    expiresHelp: 'Personne ne peut plus ouvrir le lien ensuite. Vous pouvez le révoquer avant.',
    after: (hours: number) =>
      hours === 1 ? '1 heure' : hours === 24 ? '1 jour' : `${hours / 24} jours`,
    viewersLabel: 'Spectateurs à la fois',
    viewersHelp:
      'Combien de personnes peuvent regarder grâce au lien en même temps. Chacune compte comme l’une de vos lectures simultanées.',
    submit: 'Créer le lien',
    creating: 'Création…',
    createdTitle: 'Lien créé',
    createdNotice: (title: string) =>
      `Toute personne qui a ce lien peut regarder ${title} dans un navigateur, sans compte, jusqu’à son expiration. Copiez-le maintenant : il n’est affiché que cette fois.`,
    copy: 'Copier le lien',
    copied: 'Lien copié.',
    copyFailed: 'Le lien n’a pas pu être copié. Sélectionnez-le et copiez-le à la main.',
    done: 'Terminé',
  },
  resetPreferences: {
    button: 'Rétablir mes préférences',
    title: 'Rétablir vos préférences ?',
    help: 'Ce que vous avez choisi vous-même reprend les valeurs par défaut du serveur :',
    metadataLanguage: 'Langue des métadonnées : Celle du serveur',
    homeRows: 'Rangées d’accueil : toutes les rangées s’affichent de nouveau',
    appPreferences:
      'Les préférences que les applications Jellyfin ont enregistrées dans votre compte : langues et mode de l’audio et des sous-titres, préférences de l’écran d’accueil et d’affichage',
    kept: 'Ce que vous avez regardé, vos favoris, vos points de reprise, vos connexions et vos imports restent.',
    confirm: 'Rétablir',
    resetting: 'Rétablissement…',
    done: 'Vos préférences ont été rétablies.',
  },
}

export default account
