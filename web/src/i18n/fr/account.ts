import type en from '../en/account'

const account: typeof en = {
  account: {
    title: 'Mon compte',
    signedInAs: 'Connecté en tant que ',
    sectionsLabel: 'Sections du compte',
    sections: {
      tracking: 'Suivi',
      import: 'Importer depuis un autre serveur',
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
    notificationsHelp:
      'Où Polyfin vous prévient quand un nouvel épisode d’une série que vous suivez sort, ou quand vos enregistrements se terminent.',
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
