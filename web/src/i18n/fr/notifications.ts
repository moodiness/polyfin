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
      'Elles reçoivent les événements de tous les utilisateurs : un message par nouvel épisode, quel que soit qui suit la série, chaque enregistrement et chaque lecture. Chaque utilisateur peut ajouter ses propres cibles dans Mon compte › Notifications.',
    add: 'Ajouter une cible',
    addTitle: 'Ajouter une cible',
    editTitle: (name: string) => `Modifier ${name}`,
    kind: 'Type',
    kinds: {
      webhook: 'Webhook',
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
        'L’e-mail demande un serveur SMTP : enregistrez-en un, avec une adresse d’expéditeur, dans E-mail plus haut, puis ajoutez la cible.',
      own: 'L’e-mail demande un serveur SMTP, qu’un administrateur règle dans Paramètres › Notifications.',
    },
    name: 'Nom',
    nameHelp: 'Affiché ici seulement, de 1 à 64 caractères.',
    webhookAddress: 'Adresse du webhook',
    discordAddress: 'Adresse du webhook Discord',
    addressHelp: {
      webhook: 'Elle est gardée chiffrée et n’est plus jamais affichée : seul son hôte l’est.',
      discord:
        'Dans Discord : les paramètres du salon, Intégrations, Webhooks, Copier l’URL du webhook. Elle est gardée chiffrée et n’est plus jamais affichée.',
    },
    savedAddress: (host: string) => `Enregistrée, sur ${host}.`,
    ntfyServer: 'Serveur ntfy',
    ntfyServerHelp: 'Laissez vide pour https://ntfy.sh.',
    topic: 'Sujet',
    topicHelp:
      'Le sujet auquel vous vous abonnez dans l’appli ntfy. Sur un serveur public, quiconque connaît un sujet peut le lire : choisissez-en un difficile à deviner.',
    token: 'Jeton d’accès',
    tokenHelp:
      'Pour un sujet qui demande une connexion. Il est gardé chiffré et n’est plus jamais affiché.',
    emailAddress: 'Adresse e-mail',
    emailAddressHelp: 'Où vont les messages.',
    chat: 'Discussion',
    chatHelp:
      'Le numéro de la discussion, comme -1001234567890, ou le nom d’une chaîne publique, comme @news_example. Ajoutez d’abord le bot à la discussion.',
    botToken: 'Jeton du bot',
    botTokenHelp:
      'BotFather le donne à la création du bot. Il est gardé chiffré et n’est plus jamais affiché.',
    gotifyServer: 'Serveur Gotify',
    gotifyServerHelp: 'Son adresse, comme https://gotify.example.org.',
    appToken: 'Jeton de l’application',
    appTokenHelp: {
      gotify:
        'Dans Gotify : Apps, Create Application, puis copiez son jeton. Il est gardé chiffré et n’est plus jamais affiché.',
      pushover:
        'Le jeton d’API d’une application créée sur Pushover. Il est gardé chiffré et n’est plus jamais affiché.',
    },
    userKey: 'Clé d’utilisateur',
    userKeyHelp:
      'Affichée sur le tableau de bord de Pushover une fois connecté, ou une clé de groupe. Elle est gardée chiffrée et n’est plus jamais affichée.',
    events: 'Événements',
    eventsHelp: {
      server: 'Ce qui lui est signalé, pour tous les utilisateurs.',
      own: 'Ce qui lui est signalé, vous concernant.',
    },
    eventNames: {
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
    },
    eventHelp: {
      new_episode:
        'Un épisode d’une série regardée ou mise en favori est sorti : une fois par épisode, jamais pour ceux déjà sortis.',
      recording_finished:
        'Un enregistrement de TV en direct est terminé, entier ou avec une partie manquante.',
      recording_failed: 'Un enregistrement de TV en direct n’a rien enregistré.',
      health_problem: 'Système › Santé a détecté un problème, lors de deux vérifications de suite.',
      health_solved: 'Un problème détecté par Système › Santé a disparu.',
      user_joined: 'Quelqu’un a créé son compte grâce à un lien d’invitation.',
      new_version:
        'Une nouvelle version de Polyfin est sortie, trouvée par la vérification quotidienne : une fois par version, avec ses notes de version.',
      playback_started:
        'Une vidéo ou un morceau commence dans une appli Jellyfin, avec qui le lit, sur quel appareil, et s’il est converti.',
      playback_paused:
        'Une lecture est mise en pause. La position en cours de lecture n’est jamais envoyée.',
      playback_resumed: 'Une lecture en pause reprend.',
      playback_stopped:
        'Une lecture s’arrête, ou son appli a cessé de la signaler depuis cinq minutes.',
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
    edit: 'Modifier',
    delete: 'Supprimer',
    deleteTitle: (name: string) => `Supprimer ${name} ?`,
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
        `La cible a refusé Polyfin (${answer}) : vérifiez son adresse, son jeton ou sa clé.`,
      refusedEmail: (answer: string) =>
        `Le serveur SMTP a refusé Polyfin (${answer}) : vérifiez son utilisateur et son mot de passe, l’adresse de l’expéditeur et cette adresse.`,
      rejected: (answer: string) => `La cible a refusé le dernier message (${answer}).`,
      unreachable:
        'Les derniers messages n’ont pas pu être remis : la cible n’a pas répondu, ou a répondu par des erreurs. Chaque message est retenté pendant une heure.',
      unreadable:
        'Son adresse, son jeton ou sa clé ne peut pas être déchiffré avec POLYFIN_SECRET_KEY : rien ne lui est envoyé tant qu’il n’est pas saisi de nouveau.',
    },
    lastSent: 'Dernier message ',
    nothingSent: 'Aucun message envoyé',
    noEvents: 'Aucun événement choisi',
  },
}

export default notifications
