import type en from '../en/notifications'

const notifications: typeof en = {
  notifications: {
    title: 'Notifications',
    loading: 'Chargement des cibles…',
    empty: 'Aucune cible pour l’instant.',
    emptyHint: {
      server:
        'Ajoutez une cible, comme une adresse e-mail, une discussion Telegram ou un sujet ntfy, pour être prévenu des nouveaux épisodes, des enregistrements et des problèmes que Système › Santé détecte.',
      own: 'Ajoutez une cible, comme une adresse e-mail, une discussion Telegram ou un sujet ntfy, pour être prévenu quand un nouvel épisode d’une série que vous suivez sort, ou quand vos enregistrements se terminent.',
    },
    serverTargets: 'Cibles du serveur',
    serverTargetsHelp:
      'Elles reçoivent les événements de tous les utilisateurs\u202f: un message par nouvel épisode, quel que soit qui suit la série, chaque enregistrement et chaque lecture. Chaque utilisateur peut ajouter ses propres cibles dans Mon compte › Notifications.',
    add: 'Ajouter une cible',
    addTitle: 'Ajouter une cible',
    editTitle: (name: string) => `Modifier ${name}`,
    kind: 'Type',
    kinds: {
      webhook: 'Webhook',
      jellyfin: 'Format webhook Jellyfin',
      discord: 'Discord',
      ntfy: 'ntfy',
      email: 'E-mail',
      telegram: 'Telegram',
      gotify: 'Gotify',
      pushover: 'Pushover',
    },
    kindHelp: {
      webhook:
        'Polyfin envoie chaque événement à cette adresse en JSON, décrit dans la documentation pour les développeurs d’applis.',
      jellyfin:
        'Polyfin envoie chaque événement à cette adresse comme le fait le plugin webhook de Jellyfin, pour que les outils faits pour les webhooks de Jellyfin fonctionnent avec Polyfin\u202f: domotique, suivi de visionnage, gestionnaires de demandes et scripts.',
      discord: 'Polyfin envoie chaque événement à un salon Discord, en message avec un lien.',
      ntfy: 'Polyfin publie chaque événement dans un sujet ntfy, que l’appli ntfy affiche sur votre téléphone.',
      email:
        'Polyfin envoie chaque événement par e-mail à cette adresse, en texte brut et en HTML.',
      telegram:
        'Polyfin envoie chaque événement à une discussion Telegram par votre bot, avec un lien.',
      gotify: 'Polyfin envoie chaque événement à une application de votre serveur Gotify.',
      pushover:
        'Polyfin envoie chaque événement à vos appareils Pushover, par l’une de vos applications.',
    },
    emailUnavailable: {
      server:
        'L’e-mail demande un serveur SMTP\u202f: enregistrez-en un, avec une adresse d’expéditeur, dans E-mail plus haut, puis ajoutez la cible.',
      own: 'L’e-mail demande un serveur SMTP, qu’un administrateur règle dans Paramètres › Notifications.',
    },
    name: 'Nom',
    nameHelp: 'Affiché ici seulement, de 1 à 64 caractères.',
    webhookAddress: 'Adresse du webhook',
    discordAddress: 'Adresse du webhook Discord',
    discordSwitched:
      'C’est un webhook Discord\u202f: la cible devient une cible Discord, qui envoie le message propre à Discord.',
    discordAsWebhook:
      'C’est un webhook Discord\u202f: Polyfin lui envoie le message propre à Discord, comme à une cible Discord.',
    addressHelp: {
      webhook:
        'Elle n’est plus jamais affichée, seul son hôte l’est. Elle est chiffrée si le serveur a une POLYFIN_SECRET_KEY, et enregistrée telle quelle sinon.',
      discord:
        'Dans Discord\u202f: les paramètres du salon, Intégrations, Webhooks, Copier l’URL du webhook. Elle n’est plus jamais affichée. Elle est chiffrée si le serveur a une POLYFIN_SECRET_KEY, et enregistrée telle quelle sinon.',
    },
    savedAddress: (host: string) => `Enregistrée, sur ${host}.`,
    ntfyServer: 'Serveur ntfy',
    ntfyServerHelp: 'Laissez vide pour https://ntfy.sh.',
    topic: 'Sujet',
    topicHelp:
      'Le sujet auquel vous vous abonnez dans l’appli ntfy. Sur un serveur public, quiconque connaît un sujet peut le lire\u202f: choisissez-en un difficile à deviner.',
    token: 'Jeton d’accès',
    tokenHelp:
      'Pour un sujet qui demande une connexion. Il n’est plus jamais affiché. Il est chiffré si le serveur a une POLYFIN_SECRET_KEY, et enregistré tel quel sinon.',
    emailAddress: 'Adresse e-mail',
    emailAddressHelp: 'Où vont les messages.',
    chat: 'Discussion',
    chatHelp:
      'Le numéro de la discussion, comme -1001234567890, ou le nom d’une chaîne publique, comme @news_example. Ajoutez d’abord le bot à la discussion.',
    botToken: 'Jeton du bot',
    botTokenHelp:
      'BotFather le donne à la création du bot. Il n’est plus jamais affiché. Il est chiffré si le serveur a une POLYFIN_SECRET_KEY, et enregistré tel quel sinon.',
    gotifyServer: 'Serveur Gotify',
    gotifyServerHelp: 'Son adresse, comme https://gotify.example.org.',
    appToken: 'Jeton de l’application',
    appTokenHelp: {
      gotify:
        'Dans Gotify\u202f: Apps, Create Application, puis copiez son jeton. Il n’est plus jamais affiché. Il est chiffré si le serveur a une POLYFIN_SECRET_KEY, et enregistré tel quel sinon.',
      pushover:
        'Le jeton d’API d’une application créée sur Pushover. Il n’est plus jamais affiché. Il est chiffré si le serveur a une POLYFIN_SECRET_KEY, et enregistré tel quel sinon.',
    },
    userKey: 'Clé d’utilisateur',
    userKeyHelp:
      'Affichée sur le tableau de bord de Pushover une fois connecté, ou une clé de groupe. Elle n’est plus jamais affichée. Elle est chiffrée si le serveur a une POLYFIN_SECRET_KEY, et enregistrée telle quelle sinon.',
    events: 'Événements',
    eventsHelp: {
      server: 'Ce qui lui est signalé, pour tous les utilisateurs.',
      own: 'Ce qui lui est signalé, vous concernant.',
    },
    eventNames: {
      server_crashed: 'Plantage du serveur',
      playback_failed: 'Échec d’une lecture',
      new_episode: 'Nouveaux épisodes',
      recording_finished: 'Enregistrement terminé',
      recording_failed: 'Échec d’un enregistrement',
      health_problem: 'Problème détecté',
      health_solved: 'Problème résolu',
      user_joined: 'Nouvel utilisateur',
      new_version: 'Nouvelle version',
      playback_started: 'Lecture commencée',
      playback_paused: 'Lecture en pause',
      playback_resumed: 'Lecture reprise',
      playback_stopped: 'Lecture arrêtée',
      weekly_summary: 'Résumé de la semaine',
      monthly_summary: 'Bilan du mois',
      yearly_summary: 'Bilan de l’année',
      connection_stopped: 'Connexion arrêtée',
      account_blocked: 'Compte bloqué',
      new_device: 'Connexion depuis un nouvel appareil',
      share_played: 'Lien partagé lu',
      programme_reminder: 'Début d’une émission',
      title_available: 'Titre à regarder disponible',
      titles_suggested: 'Rangée d’accueil suggérée',
    },
    eventHelp: {
      server_crashed:
        'Polyfin s’est arrêté sur une erreur puis a redémarré\u202f: signalé au redémarrage, avec le début de la trace de l’erreur, que Système › Santé affiche aussi.',
      playback_failed:
        'Une lecture n’a pas pu commencer ou s’est interrompue\u202f: aucune version ne se lit sur l’appli, une limite l’a refusée, ou son flux a échoué. Une fois par heure par utilisateur et par titre, avec l’appli et la raison.',
      new_episode:
        'Un épisode d’une série regardée ou mise en favori est sorti\u202f: une fois par épisode, jamais pour ceux déjà sortis.',
      recording_finished:
        'Un enregistrement de TV en direct est terminé, entier ou avec une partie manquante.',
      recording_failed: 'Un enregistrement de TV en direct n’a rien enregistré.',
      health_problem: 'Système › Santé a détecté un problème, lors de deux vérifications de suite.',
      health_solved: 'Un problème détecté par Système › Santé a disparu.',
      user_joined: 'Quelqu’un a créé son compte grâce à un lien d’invitation.',
      new_version:
        'Une nouvelle version de Polyfin est sortie, trouvée par la vérification quotidienne\u202f: une fois par version, avec ses notes de version.',
      playback_started:
        'Une vidéo ou un morceau commence dans une appli Jellyfin, avec qui le lit, sur quel appareil, et s’il est converti.',
      playback_paused:
        'Une lecture est mise en pause. La position en cours de lecture n’est jamais envoyée.',
      playback_resumed: 'Une lecture en pause reprend.',
      playback_stopped:
        'Une lecture s’arrête, ou son appli a cessé de la signaler depuis cinq minutes.',
      weekly_summary:
        'Une fois par semaine, au jour et à l’heure choisis pour le serveur\u202f: les heures regardées, les titres et les nouveaux épisodes de la semaine. Les cibles du serveur et des administrateurs reçoivent la semaine du serveur, avec les utilisateurs arrivés, les fichiers ajoutés aux dossiers locaux et les problèmes détectés\u202f; les autres reçoivent la leur. Rien n’est envoyé pour une semaine sans rien à dire.',
      monthly_summary:
        'Le 1er de chaque mois, à l’heure du résumé de la semaine\u202f: le contenu du résumé de la semaine sur le mois écoulé. Rien n’est envoyé pour un mois sans rien à dire.',
      yearly_summary:
        'Le 1er janvier, à l’heure du résumé de la semaine\u202f: les heures regardées, les dix films et les dix séries les plus regardés et le mois le plus regardé de l’année écoulée. Les cibles du serveur et des administrateurs reçoivent l’année du serveur, avec les utilisateurs arrivés et les fichiers ajoutés aux dossiers locaux\u202f; les autres reçoivent la leur. Rien n’est envoyé pour une année sans rien à dire.',
      connection_stopped:
        'Une connexion à une appli ou un service de suivi a refusé ce que Polyfin gardait, et s’est arrêté\u202f: une fois par arrêt, avec ce qu’il faut faire pour le reconnecter.',
      account_blocked:
        'Des mots de passe erronés ont bloqué un compte pour un moment\u202f: une fois par blocage, avec jusqu’à quand, l’adresse et l’appli de la dernière tentative. Les cibles du serveur et des administrateurs apprennent ceux de tous les comptes, les autres ceux de leur compte.',
      new_device:
        'Votre compte s’est connecté depuis une appli Jellyfin ou le client web sur un appareil qu’il n’avait jamais utilisé, avec l’appli, l’appareil et l’adresse. Le premier appareil d’un compte n’est pas signalé.',
      share_played:
        'Quelqu’un sans compte a lancé un film ou un épisode grâce à un lien de partage\u202f: au plus une fois par heure et par lien, avec le titre et qui l’a partagé. Les cibles du serveur apprennent ceux de tous les liens, les autres ceux de leurs liens.',
      programme_reminder:
        'Une émission de TV en direct dont vous avez demandé le rappel dans le lecteur web commence bientôt, avec l’avance choisie dans «\u202fMe prévenir avant\u202f», et un lien vers sa chaîne.',
      title_available:
        'Des versions ont été trouvées pour un titre de votre liste de lecture «\u202fÀ regarder\u202f» pour lequel les vérifications précédentes n’en avaient trouvé aucune\u202f: une fois par titre. Une version trouvée n’a pas encore été essayée.',
      titles_suggested:
        'Une rangée d’accueil qu’un administrateur a faite pour vous commence à s’afficher sur votre page d’accueil, et il a demandé que vous en soyez prévenu\u202f: une fois par rangée, avec son mot.',
    },
    enabled: 'Envoyer les messages à cette cible',
    create: 'Ajouter',
    creating: 'Ajout…',
    save: 'Enregistrer',
    saving: 'Enregistrement…',
    created: (name: string) => `${name} ajoutée.`,
    saved: (name: string) => `${name} enregistrée.`,
    test: 'Envoyer un test',
    testing: 'Envoi…',
    testDelivered: (name: string) => `${name} a reçu le message de test.`,
    testFailed: (name: string) => `${name} n’a pas accepté le message de test.`,
    testAnswer: (answer: string) => `Sa réponse\u202f: ${answer}`,
    edit: 'Modifier',
    delete: 'Supprimer',
    deleteTitle: (name: string) => `Supprimer ${name}\u202f?`,
    deleteBody: 'Les messages ne lui sont plus envoyés, et ceux en attente sont abandonnés.',
    deleted: (name: string) => `${name} supprimée.`,
    status: {
      working: 'Fonctionne',
      waiting: 'Rien envoyé',
      off: 'Désactivée',
      refused: 'Refusée',
      rejected: 'Message refusé',
      unreachable: 'Injoignable',
      unreadable: 'À saisir de nouveau',
    },
    answer: (code: string, email: boolean) => (email ? `SMTP ${code}` : `HTTP ${code}`),
    problem: {
      refused: (answer: string) =>
        `La cible a refusé Polyfin (${answer})\u202f: vérifiez son adresse, son jeton ou sa clé.`,
      refusedEmail: (answer: string) =>
        `Le serveur SMTP a refusé Polyfin (${answer})\u202f: vérifiez son utilisateur et son mot de passe, l’adresse de l’expéditeur et cette adresse.`,
      rejected: (answer: string) => `La cible a refusé le dernier message (${answer}).`,
      unreachable:
        'Les derniers messages n’ont pas pu être remis\u202f: la cible n’a pas répondu, ou a répondu par des erreurs. Chaque message est retenté pendant une heure.',
      unreadable:
        'Son adresse, son jeton ou sa clé ne peut pas être déchiffré avec POLYFIN_SECRET_KEY\u202f: rien ne lui est envoyé tant qu’il n’est pas saisi de nouveau.',
    },
    perMinute: 'Messages par minute, au plus',
    perMinuteHelp:
      'Les messages suivants attendent leur tour, dans l’ordre. Vide pour aucune limite\u202f; de 1 à 60.',
    groupMinutes: 'Grouper les lectures et nouveaux épisodes, en minutes',
    groupMinutesHelp:
      'Les lectures, les liens partagés lus et les nouveaux épisodes attendent ce temps, puis partent en un seul message. Tout le reste part toujours aussitôt. Vide pour tout envoyer aussitôt\u202f; de 1 à 1 440.',
    capped: (n: number) => `Au plus ${n} par minute`,
    grouped: (n: number) => `Groupés toutes les ${n} min`,
    deliveries: {
      open: 'Derniers envois',
      title: (name: string) => `Derniers envois à ${name}`,
      help: 'Les 100 derniers messages, du plus récent au plus ancien, gardés 30 jours.',
      loading: 'Chargement des envois…',
      empty: 'Aucun message envoyé.',
      when: 'Quand',
      event: 'Message',
      outcome: 'Résultat',
      test: 'Message de test',
      grouped: (n: number) => `${n} événements en un message`,
      attempts: (n: number) => `${n} tentatives`,
      outcomes: {
        delivered: 'Remis',
        refused: 'Refusé',
        rejected: 'Message refusé',
        failed: 'Non remis',
        dropped: 'Abandonné',
        unreadable: 'Indéchiffrable',
      },
    },
    preview: {
      title: 'Aperçu',
      help: 'Voir un résumé tel qu’une cible e-mail le recevrait maintenant, celui du serveur ou d’un utilisateur. Rien n’est envoyé.',
      summary: 'Résumé',
      summaries: {
        weekly_summary: 'Résumé de la semaine',
        monthly_summary: 'Bilan du mois',
        yearly_summary: 'Bilan de l’année',
      },
      of: 'De',
      server: 'Le serveur',
      show: 'Aperçu',
      loading: 'Préparation de l’aperçu…',
      empty: 'Rien à dire pour cette période\u202f: ce résumé ne serait pas envoyé.',
      frame: 'L’e-mail du résumé',
    },
    lastSent: 'Dernier message ',
    nothingSent: 'Aucun message envoyé',
    noEvents: 'Aucun événement choisi',
    jellyfin: {
      headers: 'En-têtes',
      headersHelp:
        'Envoyés avec chaque message, comme une clé d’API que l’outil demande. Leurs valeurs ne sont plus jamais affichées, et sont conservées comme l’adresse\u202f; une valeur enregistrée laissée vide est gardée.',
      headerName: 'Nom',
      headerValue: 'Valeur',
      headerSaved: 'Enregistrée',
      addHeader: 'Ajouter un en-tête',
      removeHeader: 'Retirer',
      users: 'Utilisateurs',
      usersHelp:
        'Seuls les événements des utilisateurs cochés sont envoyés\u202f; ceux qui ne concernent aucun utilisateur, comme les nouveaux épisodes, le sont toujours. N’en cochez aucun pour envoyer ceux de tous.',
      usersLoading: 'Chargement des utilisateurs…',
      forUsers: (n: number) => (n === 1 ? 'Pour 1 utilisateur' : `Pour ${n} utilisateurs`),
      eventsHelp:
        'Les types de notification du plugin webhook de Jellyfin. Chaque message contient les champs du plugin, comme ItemId, Provider_imdb ou PlaybackPositionTicks.',
      eventNames: {
        ItemAdded: 'Élément ajouté',
        PlaybackStart: 'Début de lecture',
        PlaybackProgress: 'Progression de lecture',
        PlaybackStop: 'Fin de lecture',
        UserDataSaved: 'Données utilisateur enregistrées',
        UserLockedOut: 'Utilisateur bloqué',
      },
      eventHelp: {
        ItemAdded:
          'Un nouvel épisode d’une série suivie est sorti, ou un titre d’une playlist «\u202fÀ regarder\u202f» a des versions. Les cibles du serveur apprennent chaque épisode une fois.',
        PlaybackStart: 'Une vidéo ou un morceau commence dans une appli Jellyfin.',
        PlaybackProgress:
          'Où en est une lecture\u202f: au plus toutes les 30 secondes par lecture, et aussitôt quand elle est mise en pause ou reprend.',
        PlaybackStop: 'Une lecture s’arrête, en disant si elle est allée jusqu’au bout.',
        UserDataSaved:
          'Un titre est marqué vu ou non vu, favori ou noté, ou une lecture enregistre sa position\u202f: les positions au plus toutes les 30 secondes par titre.',
        UserLockedOut: 'Des mots de passe erronés ont bloqué un compte.',
      },
    },
  },
}

export default notifications
