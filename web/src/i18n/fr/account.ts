import type en from '../en/account'

const account: typeof en = {
  account: {
    title: 'Mon compte',
    signedInAs: 'Connecté en tant que ',
    sectionsLabel: 'Sections du compte',
    sections: {
      statistics: 'Statistiques',
      tracking: 'Suivi',
      import: 'Importer depuis un autre serveur',
      appImport: 'Importer depuis une appli',
      notifications: 'Notifications',
      devices: 'Appareils',
      password: 'Mot de passe',
    },
    serverImport: {
      description:
        'Reprenez vos films et épisodes vus, vos points de reprise et vos favoris depuis un serveur Jellyfin ou Emby que vous utilisiez. Ils vont dans votre compte seulement, reconnus par IMDb, TMDB ou TVDB, et complètent seulement ce que Polyfin a déjà. Rien n’est écrit sur l’autre serveur.',
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
    appImport: {
      description:
        'Reprenez ce que vous avez regardé dans une appli basée sur des addons : les films et épisodes vus avec leurs dates, vos points de reprise, et la bibliothèque de l’appli en favoris, reconnus par IMDb, TMDB ou Kitsu. Ils complètent seulement ce que Polyfin a déjà, et rien n’est écrit dans l’appli.',
      app: 'Appli',
      apps: { nuvio: 'Nuvio', stremio: 'Stremio' },
      backend: 'Serveur Nuvio',
      backendHelp:
        'Laissez vide pour celui de Nuvio. Un serveur auto-hébergé fonctionne aussi, à son adresse, y compris sur le réseau local.',
      email: 'E-mail',
      password: 'Mot de passe',
      passwordHelp:
        'Il sert seulement à lire le compte, et n’est jamais gardé. Polyfin se connecte comme les applis Nuvio, lit le compte, puis se déconnecte.',
      signIn: 'Se connecter à Nuvio',
      signingIn: 'Connexion…',
      chooseTitle: 'Choisir quoi importer',
      chooseHelp: {
        nuvio:
          'Nuvio garde un historique par profil : choisissez celui à importer. Polyfin garde ce qu’il a lu quelques minutes seulement.',
        stremio:
          'Stremio garde un seul historique pour tout le compte. Polyfin garde ce qu’il a lu quelques minutes seulement.',
      },
      wholeAccount: 'Tout le compte',
      profile: 'Profil',
      profileNumber: (id: string) => `Profil ${id}`,
      profileCounts: (played: number, resumes: number, library: number) =>
        `${played} vus, ${resumes <= 1 ? `${resumes} point de reprise` : `${resumes} points de reprise`}, ${library} dans la bibliothèque.`,
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
      counts: (read: number, played: number, resumed: number, favorites: number) =>
        `${read <= 1 ? `${read} titre lu` : `${read} titres lus`} : ${played} marqués vus, ${resumed <= 1 ? `${resumed} point de reprise` : `${resumed} points de reprise`}, ${favorites <= 1 ? `${favorites} favori` : `${favorites} favoris`}.`,
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
          'En attente de votre approbation. Le code dure 5 minutes ; une fois approuvé, Polyfin lit le compte, puis se déconnecte.',
        cancel: 'Annuler',
        email: 'E-mail',
        password: 'Mot de passe',
        passwordHelp:
          'Il sert seulement à lire le compte, et n’est jamais gardé. Polyfin se connecte comme les applis Stremio, lit le compte, puis se déconnecte. Un compte créé avec Facebook ou Apple n’a pas de mot de passe : utilisez un code de liaison.',
        signIn: 'Se connecter à Stremio',
        signingIn: 'Connexion…',
        key: 'Clé d’authentification',
        keyHelp:
          'La clé que l’appli web de Stremio garde une fois connectée : dans la console de votre navigateur, sur cette appli, JSON.parse(localStorage.getItem("profile")).auth.key la donne. Elle sert seulement à lire le compte, et n’est jamais gardée. Polyfin ne la déconnecte pas, ce qui déconnecterait votre navigateur.',
        readKey: 'Lire le compte',
        reading: 'Lecture…',
      },
    },
    statisticsHelp:
      'Ce que vous avez regardé sur ce serveur : heures, titres, applis et appareils, et vos lectures récentes.',
    notificationsHelp:
      'Où Polyfin vous prévient quand un nouvel épisode d’une série que vous suivez sort, quand vos enregistrements se terminent, ou quand vos lectures commencent et s’arrêtent, et vous envoie le résumé de votre semaine.',
    devicesHelp:
      'Les applications connectées à votre compte. Déconnecter un appareil lui demande votre mot de passe à la prochaine ouverture.',
    passwordHelp: 'Changer de mot de passe déconnecte tous vos appareils Jellyfin.',
    currentPassword: 'Mot de passe actuel',
    newPassword: 'Nouveau mot de passe',
    confirmPassword: 'Confirmer le nouveau mot de passe',
    changePassword: 'Changer le mot de passe',
    changingPassword: 'Modification…',
    passwordChanged: 'Votre mot de passe a été modifié.',
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
      connectedAs: 'Connecté en tant que ',
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
        toggleHelp: (name: string) =>
          `Marque comme vu dans Polyfin ce que vous avez regardé sur ${name}, et reprend là où vous vous êtes arrêté, pour que Reprendre et À suivre correspondent. Importé toutes les 6 heures ; rien n’est renvoyé à ${name}.`,
        importNow: 'Importer maintenant',
        importing: 'Importation…',
        importingStatus: 'Importation de votre historique…',
        never: 'Pas encore importé.',
        lastImport: 'Dernière importation',
        colon: ' : ',
        counts: (played: number, resumed: number, unmapped: number) =>
          `${played <= 1 ? `${played} titre marqué vu` : `${played} titres marqués vus`}, ${resumed <= 1 ? `${resumed} point de reprise` : `${resumed} points de reprise`}, ${unmapped <= 1 ? `${unmapped} titre introuvable` : `${unmapped} titres introuvables`} dans Polyfin.`,
        problem: {
          reconnect: (name: string) =>
            `${name} a refusé la connexion lors de la dernière importation. Reconnectez-vous pour importer votre historique.`,
          unreachable: (name: string) =>
            `${name} n’a pas pu être lu entièrement lors de la dernière importation. Ce qui a été lu a été importé ; Polyfin réessaie dans 6 heures.`,
          rate_limited: (name: string) =>
            `${name} a demandé à Polyfin d’attendre lors de la dernière importation. Ce qui a été lu a été importé ; Polyfin réessaie dans 6 heures.`,
        },
      },
    },
  },
}

export default account
