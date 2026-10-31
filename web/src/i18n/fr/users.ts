import type en from '../en/users'

const users: typeof en = {
  users: {
    title: 'Utilisateurs',
    description: 'Créez et gérez les comptes autorisés à utiliser ce serveur.',
    listTitle: 'Comptes',
    empty: 'Aucun utilisateur pour le moment.',
    administrator: 'Administrateur',
    hidden: 'Masqué sur l’écran de connexion',
    disabled: 'Désactivé',
    you: 'Vous',
    createTitle: 'Créer un utilisateur',
    name: 'Nom',
    password: 'Mot de passe',
    isAdministrator: 'Administrateur',
    isAdministratorHelp:
      'Les administrateurs peuvent gérer les utilisateurs et les paramètres du serveur.',
    showOnSignIn: 'Afficher sur l’écran de connexion',
    showOnSignInHelp:
      'Les utilisateurs visibles apparaissent sur l’écran de connexion des applications Jellyfin. Les autres saisissent leur nom, comme par défaut dans Jellyfin.',
    create: 'Créer l’utilisateur',
    creating: 'Création…',
    created: (name: string) => `${name} a été créé.`,
    edit: 'Modifier',
    close: 'Fermer',
    rename: 'Renommer',
    renamed: 'Le nom a été modifié.',
    resetPassword: 'Réinitialiser le mot de passe',
    newPassword: 'Nouveau mot de passe',
    resetPasswordHelp:
      'Changer le mot de passe déconnecte tous les appareils Jellyfin de cet utilisateur.',
    passwordReset: 'Le mot de passe a été réinitialisé.',
    accessTitle: 'Accès',
    isDisabled: 'Désactivé',
    isDisabledHelp:
      'Un utilisateur désactivé ne peut plus se connecter, et tous ses appareils sont déconnectés.',
    providerOnly: 'Fournisseur d’identité uniquement',
    providerOnlyHelp:
      'L’utilisateur se connecte avec le fournisseur d’identité configuré dans Paramètres › Sécurité, jamais avec un mot de passe : le mot de passe est refusé, et un compte créé ainsi n’en a pas besoin. Les applications Jellyfin qui ne peuvent pas ouvrir le fournisseur se connectent avec Quick Connect.',
    ended: 'Terminé',
    endsOn: (date: string) => `Prend fin le ${date}`,
    endedOn: (date: string) => `A pris fin le ${date}`,
    endTitle: 'Prend fin le',
    endHelp:
      'À partir de cette date, dans le fuseau horaire du serveur, le compte est refusé comme un compte désactivé : il ne peut plus se connecter, et ses appareils sont refusés. Tout ce qu’il possède est gardé, et une date plus tardive, ou « Jamais », le rétablit.',
    endAdministratorHelp:
      'Le compte d’un administrateur ne prend jamais fin : faire d’un utilisateur un administrateur efface sa date de fin.',
    endDate: 'Date de fin',
    saveEnd: 'Enregistrer la date',
    endNever: 'Jamais',
    canTranscode: 'Peut utiliser la conversion (transcodage)',
    canTranscodeHelp:
      'Le serveur peut convertir l’image et le son quand l’application de cet utilisateur ne sait pas lire un fichier tel quel. Sinon, l’image et le son ne sont jamais modifiés.',
    canDownload: 'Peut télécharger',
    canDownloadHelp:
      'Permet à cet utilisateur d’enregistrer des titres dans les applications Jellyfin pour les regarder hors connexion.',
    canPlayTorrents: 'Peut lire les flux torrent',
    canPlayTorrentsHelp:
      'Propose à cet utilisateur les torrents que listent les addons, quand Paramètres › Torrents les lit. Désactivé pour tous, administrateurs compris, jusqu’à ce qu’on le donne.',
    noTranscoding: 'Sans conversion',
    noDownloads: 'Sans téléchargement',
    turnOffDownloads: 'Désactiver les téléchargements pour tous',
    turnOffDownloadsConfirm:
      'Tous les utilisateurs et tous les groupes perdent l’autorisation de télécharger, y compris dans les applications Jellyfin déjà connectées. Vous pouvez la rendre à un utilisateur ou à un groupe depuis sa page, dans Accès.',
    turningOffDownloads: 'Désactivation…',
    downloadsTurnedOff: (count: number) =>
      count === 0
        ? 'Personne ne pouvait télécharger.'
        : count === 1
          ? '1 utilisateur ne peut plus télécharger.'
          : `${count} utilisateurs ne peuvent plus télécharger.`,
    lastAdminHelp:
      'Le dernier administrateur actif ne peut être ni supprimé, ni rétrogradé, ni désactivé.',
    updated: 'Modifications enregistrées.',
    metadataLanguageTitle: 'Langue des métadonnées',
    metadataLanguageHelp:
      'La langue des noms, des résumés et des images des titres dans les applis de cet utilisateur, quand l’un des addons du serveur les décrit dans cette langue ; les titres qu’aucun addon n’y décrit gardent la description du serveur. Les genres, les classifications et les personnes restent les mêmes pour tous : le contrôle parental et les genres bloqués jugent chaque utilisateur de la même façon. Les noms que Polyfin crée lui-même la suivent quand Polyfin la parle : anglais, français, allemand, espagnol, italien ou portugais.',
    metadataLanguageNone:
      'Aucun addon du serveur n’a encore de langue. Donnez-en une à un addon sur sa page, dans Contenu › Sources.',
    webThemeTitle: 'Thème du lecteur web',
    webThemeHelp:
      'L’apparence du lecteur web pour cet utilisateur, sur chaque navigateur où il se connecte : le thème du serveur, LumaaGlaass, l’apparence de jellyfin-web, plus légère, pour les téléviseurs et les téléphones anciens, ou le thème personnalisé tant qu’il a un texte. Les navigateurs trop anciens pour LumaaGlaass prennent d’eux-mêmes l’apparence de jellyfin-web.',
    webThemeChange: 'Lui permettre de le changer',
    webThemeChangeHelp:
      'Dans Mon compte › Thème du lecteur web. Désactivé, le thème choisi ici reste le sien.',
    audioRegionTitle: 'Région audio',
    audioRegionHelp:
      'Le doublage que cet utilisateur préfère quand une langue est doublée à part pour une région, comme le français du Québec. Elle s’applique à la langue audio choisie dans ses applis, sinon celle du serveur, quand c’est la langue de la région, et la choisit quand aucune ne l’est : dans cette langue, les pistes audio de la région sont lues en premier, et ses versions passent en premier quand le serveur trie les versions par langue.',
    parentalTitle: 'Contrôle parental',
    maxRating: 'Classification maximale',
    maxRatingHelp: 'Les titres classés au-delà de cette limite sont masqués pour cet utilisateur.',
    noLimit: 'Aucune limite',
    blockUnratedMovies: 'Bloquer les films non classés',
    blockUnratedShows: 'Bloquer les séries non classées',
    ratingLimit: (rating: string) => `Jusqu’à ${rating}`,
    saveParental: 'Enregistrer le contrôle parental',
    devicesTitle: 'Appareils',
    delete: 'Supprimer l’utilisateur',
    deleteConfirm: (name: string) =>
      `Supprimer ${name} ? Ses appareils seront déconnectés et cette action est irréversible.`,
    deleting: 'Suppression…',
    deleted: (name: string) => `${name} a été supprimé.`,
    canAddAddons: 'Peut ajouter ses propres addons',
    canAddAddonsHelp:
      'Cet utilisateur peut ajouter ses propres addons Stremio depuis sa page Mes sources. Sinon, ses addons sont conservés mais pas utilisés.',
    noPersonalAddons: 'Sans addons personnels',
    blockedUntil: (time: string) => `Bloqué jusqu’à ${time}`,
    blockedHelp:
      'Trop de mots de passe faux ont été saisis pour ce compte : il ne peut pas se connecter avant cette heure, même avec le bon mot de passe.',
    unblock: 'Débloquer',
    unblocking: 'Déblocage…',
    unblocked: 'Le compte est débloqué.',
    twoStepOn: 'La connexion en deux étapes est activée.',
    twoStepHelp:
      'Si cet utilisateur a perdu le téléphone de son application d’authentification et n’a plus de code de secours, désactivez-la : il se connecte avec son mot de passe seul, puis la configure de nouveau depuis Mon compte.',
    turnOffTwoStep: 'Désactiver la connexion en deux étapes',
    turningOffTwoStep: 'Désactivation…',
    turnOffTwoStepTitle: (name: string) => `Désactiver la connexion en deux étapes de ${name} ?`,
    turnOffTwoStepConfirm: (name: string) =>
      `${name} se connecte alors avec son mot de passe seul. Le journal d’activité indique que vous l’avez désactivée.`,
    twoStepTurnedOff: 'La connexion en deux étapes est désactivée pour cet utilisateur.',
    playbackAccessTitle: 'Lecture et accès',
    maxPlaybacks: 'Lectures en même temps (0 = pas de limite)',
    maxPlaybacksHelp:
      'Le nombre d’appareils de cet utilisateur qui peuvent lire en même temps. Il peut toujours se connecter sur d’autres appareils.',
    maxBitrate: 'Qualité maximale',
    maxBitrateHelp:
      'Au-delà, la vidéo est convertie si cet utilisateur peut utiliser la conversion ; sinon, une version plus légère est lue.',
    bitrateNoLimit: 'Pas de limite',
    bitrate4k: '40 Mbit/s (4K)',
    bitrate1080High: '20 Mbit/s (1080p, haute qualité)',
    bitrate1080: '10 Mbit/s (1080p)',
    bitrate720: '4 Mbit/s (720p)',
    bitrate480: '2 Mbit/s (480p)',
    bitrateOther: (mbits: string) => `${mbits} Mbit/s`,
    liveTv: 'TV en direct',
    liveTvHelp: 'Montre les chaînes de TV en direct à cet utilisateur.',
    syncPlay: 'Regarder ensemble',
    syncPlayHelp:
      'Lire le même titre en même temps que d’autres, dans les applications Jellyfin qui le proposent.',
    syncPlayCreateAndJoin: 'Créer et rejoindre des groupes',
    syncPlayJoin: 'Rejoindre seulement',
    syncPlayNone: 'Non',
    remoteControl: 'Peut contrôler les applis des autres utilisateurs',
    remoteControlHelp:
      'Permet à cet utilisateur de lancer, mettre en pause ou envoyer des messages aux applications Jellyfin des autres utilisateurs. Chacun peut contrôler ses propres applications.',
    liveTvManagement: 'Peut enregistrer la TV en direct',
    liveTvManagementHelp: 'Programmer et supprimer des enregistrements.',
    savePlaybackAccess: 'Enregistrer la lecture et l’accès',
    visibleLibrariesTitle: 'Bibliothèques visibles',
    visibleLibrariesHelp:
      'Les bibliothèques du serveur que les applications de cet utilisateur affichent. Les bibliothèques ajoutées plus tard sont affichées. Ce choix ne bloque aucun titre : on peut toujours les trouver dans d’autres bibliothèques, par la recherche ou par un lien. Pour bloquer des titres, utilisez le contrôle parental ou les genres bloqués.',
    noServerLibraries: 'Le serveur n’a pas encore de bibliothèque.',
    saveVisibleLibraries: 'Enregistrer les bibliothèques visibles',
    blockedGenresTitle: 'Genres bloqués',
    blockedGenresHelp:
      'Les titres de l’un de ces genres sont masqués partout pour cet utilisateur, comme ceux qui dépassent la limite du contrôle parental.',
    genre: 'Genre',
    genreHint: 'Choisissez un genre des bibliothèques du serveur, ou tapez-en un.',
    addGenre: 'Ajouter',
    removeGenre: (genre: string) => `Retirer ${genre}`,
    noBlockedGenres: 'Aucun genre n’est bloqué.',
    saveBlockedGenres: 'Enregistrer les genres bloqués',
    liveTvChannelsTitle: 'Chaînes de TV en direct',
    liveTvChannelsHelp:
      'Les chaînes des sources de TV en direct du serveur que cet utilisateur voit, choisies par groupe et par chaîne. Les chaînes masquées n’apparaissent ni dans la liste des chaînes, ni dans le guide, les recherches, le Replay, les enregistrements, les programmations ou les rappels, et ne se lisent pas. Les sources propres de l’utilisateur ne sont pas limitées.',
    channelsMode: 'Chaînes',
    channelsAll: 'Toutes',
    channelsOnly: 'Seulement celles-ci',
    channelsExcept: 'Toutes sauf celles-ci',
    channelsOnlyHelp: 'Seuls les groupes et les chaînes cochés sont affichés.',
    channelsExceptHelp: 'Les groupes et les chaînes cochés sont masqués.',
    channelGroups: 'Groupes',
    filterChannelGroups: 'Filtrer les groupes',
    moreChannelGroups: (count: number) => `${count} de plus : filtrez pour les trouver.`,
    newChannelGroup: 'Nouveau',
    noChannelGroups: 'Les sources de TV en direct du serveur n’ont encore aucune chaîne.',
    singleChannels: 'Chaînes',
    searchChannels: 'Rechercher une chaîne par nom ou par numéro',
    tickedChannels: 'Chaînes cochées',
    noTickedChannels: 'Aucune chaîne cochée.',
    goneChannel: 'Chaîne qui n’est plus listée',
    removeChannel: (name: string) => `Retirer ${name}`,
    moreChannels: (count: number) => `${count} de plus : recherchez pour les trouver.`,
    hideNewChannelGroups: 'Masquer les chaînes des groupes ajoutés plus tard',
    hideNewChannelGroupsHelp:
      'Les groupes qu’une source ajoute après l’enregistrement sont masqués. Ils apparaissent ici cochés, marqués « Nouveau », jusqu’au prochain enregistrement.',
    saveLiveTvChannels: 'Enregistrer les chaînes de TV en direct',
    allowedHoursTitle: 'Horaires autorisés',
    allowedHoursHelp:
      'Si des horaires sont définis, cet utilisateur ne peut se connecter et utiliser le serveur que pendant ces horaires, à l’heure du serveur. En dehors, ses applications sont refusées.',
    noAllowedHours: 'Aucun horaire : cet utilisateur peut utiliser le serveur à toute heure.',
    day: 'Jour',
    from: 'De',
    to: 'À',
    addHours: 'Ajouter un horaire',
    removeHours: 'Retirer',
    hoursOrder: 'Chaque heure de fin doit venir après son heure de début.',
    saveAllowedHours: 'Enregistrer les horaires autorisés',
    days: {
      Sunday: 'Dimanche',
      Monday: 'Lundi',
      Tuesday: 'Mardi',
      Wednesday: 'Mercredi',
      Thursday: 'Jeudi',
      Friday: 'Vendredi',
      Saturday: 'Samedi',
      Everyday: 'Tous les jours',
      Weekday: 'En semaine (du lundi au vendredi)',
      Weekend: 'Le week-end',
    },
    canManageCollections: 'Peut gérer les collections',
    canManageCollectionsHelp:
      'Permet à cet utilisateur de créer des collections de titres dans les applications Jellyfin, d’y ajouter ou d’en retirer des titres, et de les supprimer. Tous les utilisateurs voient les collections, chacun avec seulement les titres qu’il a le droit de voir.',
    resetPinRequested: 'Réinitialisation du mot de passe demandée : code',
    resetPinValidUntil: (time: string) => `, valable jusqu’à ${time}`,
    resetPinHelp:
      'Donnez ce code à l’utilisateur : il se connecte avec, et ce code devient son nouveau mot de passe.',
    canManageSubtitles: 'Peut gérer les sous-titres',
    canManageSubtitlesHelp:
      'Permet à cet utilisateur d’ajouter des fichiers de sous-titres aux titres depuis ses applications Jellyfin, et d’y chercher les sous-titres des addons.',
    canShare: 'Peut partager des titres',
    canShareHelp:
      'Permet à cet utilisateur de donner un film ou un épisode à quelqu’un qui n’a pas de compte, grâce à un lien qui expire, depuis Mon compte ou la page d’un titre dans le lecteur web, tant que Paramètres › Sécurité › « Autoriser les liens de partage » est activé. Le désactiver révoque ses liens.',
    remoteAccess: 'Peut se connecter depuis l’extérieur du réseau local',
    remoteAccessHelp:
      'Permet à cet utilisateur de se connecter, de parcourir et de lire depuis n’importe où. Désactivé, il ne le peut que depuis le réseau local, même administrateur, tel que les Proxys connus de Paramètres › Sécurité le définissent. Les liens qu’il partage restent lisibles partout.',
    remoteAccessUnknownProxy:
      'La page Santé voit un proxy inverse absent des Proxys connus : tous ceux qui passent par lui semblent locaux, et cette limite ne s’applique donc pas à eux.',
    openKnownProxies: 'Ouvrir les Proxys connus',
    shareLinksTitle: 'Liens partagés',
    shareLinksHelp:
      'Les liens que cet utilisateur a créés pour partager un film ou un épisode. « Révoquer » en arrête un tout de suite.',
    qualityGroup: 'Groupe de qualité',
    qualityGroupHelp:
      'La meilleure définition proposée à cet utilisateur. Les versions de définition supérieure sont écartées tant qu’une autre convient ; si aucune ne convient, elles sont converties, ou refusées si cet utilisateur ne peut pas utiliser la conversion. La TV en direct est aussi convertie. Seules les versions qui conviennent sont téléchargées telles quelles ; avec « Téléchargements convertis » dans Paramètres › Conversion (transcodage), les applis qui le demandent téléchargent les autres converties.',
    qualityGroupOriginal: 'D’origine (sans limite)',
    member: 'Membre',
    lastSignInLabel: 'Dernière connexion :',
    active: 'Actif',
    pinRequested: 'PIN demandé',
    emptyHelp:
      'Créez un compte pour chaque personne qui regarde ici, puis connectez-vous avec depuis une application Jellyfin.',
    devicesEmptyHelp:
      'Les applications apparaissent ici dès qu’elles se connectent avec ce compte.',
    notFound: 'Utilisateur introuvable',
    notFoundHelp:
      'Cet utilisateur n’existe plus, ou l’adresse est fausse. Choisissez un utilisateur dans la liste.',
    backToUsers: 'Retour aux utilisateurs',
    sectionsLabel: 'Sections de l’utilisateur',
    profileTitle: 'Nom et mot de passe',
    accessHelp: 'Chaque changement est enregistré tout de suite.',
    noServerLibrariesHelp:
      'Ajoutez une bibliothèque dans Contenu › Bibliothèques, puis choisissez ici celles que cet utilisateur voit.',
    jellyfinImport: {
      open: 'Importer depuis un autre serveur',
      title: 'Importer depuis un autre serveur',
      description:
        'Reprenez les comptes d’un serveur Jellyfin, Emby ou Plex et ce qu’ils ont regardé : noms des comptes et statut d’administrateur, puis pour chaque utilisateur les films et épisodes vus avec leurs dates, les points de reprise et les favoris, reconnus par IMDb, TMDB ou TVDB, et si vous le souhaitez leurs notes, les collections du serveur et leurs playlists. Les données de Polyfin sont seulement complétées, et rien n’est écrit sur l’autre serveur. Les serveurs ne donnent pas les mots de passe : vous en choisissez un pour chaque nouveau compte, ou gardez celui avec lequel l’utilisateur se connecte.',
      serverKind: 'Serveur',
      servers: {
        jellyfin: 'Jellyfin',
        emby: 'Emby',
        plex: 'Plex',
      },
      serverKindHelp: {
        jellyfin: 'Jellyfin, ou un serveur qui parle son API.',
        emby: 'Emby, avec ses propres clés d’API et utilisateurs.',
        plex: 'Plex Media Server, lu avec le jeton de son propriétaire.',
      },
      connectTitle: (server: string) => `Se connecter à ${server}`,
      connectHelp:
        'L’adresse, la clé et les mots de passe servent seulement à cette importation : Polyfin ne les enregistre pas.',
      address: 'Adresse du serveur',
      addressHelp: (server: string) =>
        `L’adresse à laquelle ${server} s’ouvre dans un navigateur. Une adresse du réseau local fonctionne.`,
      plexAddressHelp:
        'L’adresse de Plex Media Server lui-même, en général sur le port 32400. Une adresse du réseau local fonctionne.',
      connectWith: 'Se connecter avec',
      withApiKey: 'Une clé d’API',
      withAccount: 'Un compte utilisateur',
      apiKey: 'Clé d’API',
      apiKeyHelp:
        'Créez-en une dans le tableau de bord de Jellyfin, sous Clés API : elle lit les données de visionnage de tous les utilisateurs.',
      embyApiKeyHelp:
        'Créez-en une dans les paramètres d’Emby, sous Clés API : elle lit les données de visionnage de tous les utilisateurs. La clé d’un utilisateur n’est pas acceptée : Emby ne dit pas à qui elle appartient.',
      plexToken: 'Jeton Plex',
      plexTokenHelp:
        'Le jeton du propriétaire du serveur, avec lequel Polyfin ne fait que lire le serveur. Pour le trouver, ouvrez Plex dans un navigateur, connecté avec le compte du propriétaire. Ouvrez le menu d’un film, choisissez Obtenir des infos, puis Afficher le XML : l’adresse de la page qui s’ouvre se termine par X-Plex-Token= suivi du jeton. Copiez ce qui suit.',
      accountName: 'Nom d’utilisateur',
      accountNameHelp: (server: string) =>
        `Le compte d’un administrateur liste tous les utilisateurs. Polyfin se connecte comme les applis de ${server}, puis se déconnecte.`,
      accountPassword: 'Mot de passe',
      accountPasswordHelp: 'Laissez vide si le compte n’en a pas.',
      connect: 'Se connecter',
      connecting: 'Connexion…',
      chooseTitle: 'Choisir qui importer',
      chooseHelp: (server: string, product: string, version: string) =>
        `${server}, ${product} ${version}. Pour chaque utilisateur, choisissez le compte Polyfin dans lequel importer, ou créez-en un.`,
      changeServer: 'Changer de serveur',
      noUsers: (product: string) => `Ce serveur ${product} n’a aucun utilisateur.`,
      lastActivityLabel: 'Dernière activité :',
      importAs: 'Importer dans',
      skip: 'Ne pas importer',
      newUser: 'Nouvel utilisateur',
      watchData: 'Importer les données de visionnage',
      watchDataHelp: 'Titres vus avec leurs dates, points de reprise et favoris.',
      plexOwnerWatchDataHelp:
        'Titres vus avec leurs dates et leur nombre de lectures, points de reprise et notes.',
      plexWatchDataHelp:
        'Les titres que ce compte a lus jusqu’au bout sur ce serveur, d’après l’historique de lecture de Plex, avec leurs dates et leur nombre de lectures. Les titres qu’il a marqués comme vus ou synchronisés depuis son compte Plex, et ses points de reprise, restent sur Plex.',
      watchDataNeeded: 'Sans elles, rien n’est importé pour cet utilisateur.',
      extras: {
        title: 'Reprendre aussi',
        ratings: 'Notes',
        ratingsHelp:
          'Les notes de chaque utilisateur, de 0 à 10, telles quelles. Un pouce levé sans note devient un favori. Une note que Polyfin a déjà est gardée.',
        collections: 'Collections',
        collectionsHelp:
          'Les collections du serveur, en collections Polyfin que chaque utilisateur voit, avec les titres trouvés. Une collection dont aucun titre n’est trouvé n’est pas créée.',
        playlists: 'Playlists',
        playlistsHelp:
          'Les playlists vidéo de chaque utilisateur, en playlists à lui, dans leur ordre, avec les titres trouvés.',
        plexHelp:
          'Plex ne donne les notes et les playlists que des comptes lus avec leur propre jeton : celui du propriétaire, et ceux des autres une fois que vous vous connectez à Plex en tant que propriétaire.',
      },
      plexNotice:
        'Avec le jeton du propriétaire, Plex laisse Polyfin lire les titres vus, les points de reprise et les notes du propriétaire. Des autres comptes, il ne donne que leur historique de lecture sur ce serveur : les titres qu’ils y ont lus jusqu’au bout. Les titres qu’ils ont marqués comme vus sans les lire, ceux synchronisés depuis leur compte Plex, leurs points de reprise et leurs notes n’y figurent pas. Pour lire les données de visionnage de chaque compte, connectez-vous avec Plex en tant que propriétaire, ci-dessus. Plex n’a pas de favoris à importer.',
      plexOwner: {
        title: 'Lire chaque compte avec Plex',
        help: 'Connectez-vous avec Plex en tant que propriétaire de ce serveur. Plex donne alors à Polyfin le jeton de chaque compte avec lequel le serveur est partagé, et Polyfin y lit ce que le compte a vu, marqué comme vu ou synchronisé, ses points de reprise et ses notes, pas seulement ses lectures ici. Les profils de votre Plex Home sont lus avec leur code PIN, quand ils en ont un.',
        signIn: 'Se connecter avec Plex en tant que propriétaire',
        checking: 'Plex est interrogé sur les comptes dont il donne le jeton…',
        signedInAs: (name: string) =>
          `Connecté à Plex en tant que ${name}, propriétaire du serveur.`,
        signOut: 'Se déconnecter de Plex',
        notice:
          'Polyfin lit chaque compte ci-dessous avec le jeton que Plex donne au propriétaire pour lui, comme les applications Plex de ce compte lisent ce serveur. Les jetons ne servent qu’à cet import : ils ne sont jamais conservés, et votre connexion à Plex est fermée à la fin de l’import.',
        all: 'Tout : les titres vus, marqués comme vus ou synchronisés, les points de reprise et les notes.',
        pin: 'Un profil Plex Home avec un code PIN : tout avec son code PIN, seulement ses lectures sur ce serveur sans lui.',
        plays:
          'Seulement ses lectures sur ce serveur, d’après l’historique de lecture de Plex : Plex ne donne pas de jeton pour ce compte.',
        everything: 'Tout',
        playsOnly: 'Lectures seulement',
        pinLabel: 'Code PIN du profil',
        pinHelp: (name: string) =>
          `Le code PIN que ${name} demande sur Plex. Laissé vide, seules ses lectures sont lues.`,
      },
      ownerNotice: (owner: string | null, signedIn: boolean) =>
        `${signedIn ? `Connecté avec le compte ${owner ?? 'd’un utilisateur'}, Polyfin ne lit que ses données de visionnage.` : `Cette clé appartient à ${owner ?? 'un utilisateur'} : elle ne lit que ses données de visionnage.`} Pour importer celles d’un autre utilisateur, saisissez le mot de passe de son compte sur le serveur : Polyfin se connecte en tant que lui pour les lire. Sans lui, son compte peut quand même être créé.`,
      ownerOnlyNotice: (owner: string | null, signedIn: boolean) =>
        `${signedIn ? `Connecté avec le compte ${owner ?? 'de cet utilisateur'}` : `Cette clé appartient à ${owner ?? 'un utilisateur'}`}, qui n’est pas administrateur de ce serveur : Polyfin n’importe que ce compte. Pour lister les autres utilisateurs du serveur, connectez-vous avec le compte d’un administrateur ou une clé d’API.`,
      serverPassword: (server: string) => `Mot de passe sur ${server}`,
      serverPasswordHelp: (user: string) =>
        `Polyfin se connecte avec le compte ${user} pour lire ses données de visionnage, puis se déconnecte. Laissez vide si le compte n’en a pas.`,
      keepPassword: 'Garder ce mot de passe dans Polyfin',
      keepPasswordHelp:
        'Le nouveau compte se connecte à Polyfin avec le même mot de passe, s’il a au moins 8 caractères.',
      nothingToImport:
        'Choisissez un utilisateur à créer, ou des données de visionnage à importer.',
      start: 'Lancer l’importation',
      starting: 'Lancement…',
      started: (created: number, importing: boolean) =>
        created === 0
          ? 'L’importation a commencé.'
          : `${created === 1 ? '1 utilisateur créé.' : `${created} utilisateurs créés.`}${importing ? ' L’importation a commencé.' : ''}`,
      currentTitle: 'Importation en cours',
      lastTitle: 'Dernière importation',
      startedLabel: 'Commencée',
      endedLabel: 'Terminée',
      startedBy: (name: string, own: boolean) =>
        own ? `Historique de ${name}, depuis Mon compte` : `Lancée par ${name}`,
      runningHelp:
        'Vous pouvez quitter cette page : l’importation continue. Si Polyfin redémarre entre-temps, l’importation s’arrête : ce qu’elle a importé reste, et importer à nouveau n’ajoute rien en double.',
      stop: 'Arrêter l’importation',
      stopping: 'Arrêt…',
      states: {
        running: 'En cours',
        done: 'Terminée',
        stopped: 'Arrêtée',
        failed: 'Échouée',
      },
      problems: {
        jellyfin_unreachable: (server: string) =>
          `${server} est devenu injoignable pendant l’importation. Ce qui a été importé est gardé.`,
        jellyfin_key_refused: (server: string) =>
          `${server} a refusé la clé d’API pendant l’importation : elle a peut-être été révoquée. Ce qui a été importé est gardé.`,
        not_jellyfin: (server: string) =>
          `Le serveur a cessé de répondre comme ${server} pendant l’importation. Ce qui a été importé est gardé.`,
        internal: () =>
          'Polyfin a rencontré une erreur pendant l’importation : le journal du serveur donne le détail. Ce qui a été importé est gardé.',
        jellyfin_user_forbidden: () =>
          'Le serveur ne laisse pas cette clé lire les données de cet utilisateur. Importez-les avec une clé d’API du tableau de bord du serveur, ou avec sa propre clé.',
      },
      errors: {
        emby: {
          invalid_jellyfin_address:
            'Saisissez l’adresse du serveur Emby, par exemple http://192.168.1.10:8096.',
          jellyfin_key_refused:
            'Emby a refusé cette clé d’API. Créez-en une dans ses paramètres, sous Clés API, puis collez-la à nouveau.',
          jellyfin_sign_in_refused: 'Emby a refusé ce nom ou ce mot de passe.',
          jellyfin_sign_in_forbidden:
            'Emby ne laisse pas ce compte se connecter : il est peut-être désactivé, ou hors de ses horaires autorisés.',
          jellyfin_password_refused:
            'Emby a refusé ce mot de passe. Laissez vide si le compte n’en a pas.',
          jellyfin_unreachable:
            'Rien n’a répondu à cette adresse. Vérifiez-la, et qu’Emby est bien lancé.',
          not_jellyfin:
            'Un serveur a répondu à cette adresse, mais ce n’est pas Emby. Vérifiez l’adresse.',
        },
        plex: {
          invalid_jellyfin_address:
            'Saisissez l’adresse de Plex Media Server, par exemple http://192.168.1.10:32400.',
          jellyfin_key_refused:
            'Plex a refusé ce jeton. Copiez à nouveau le jeton du propriétaire.',
          jellyfin_key_limited:
            'Ce jeton ne peut pas lister les comptes du serveur : ce n’est pas celui du propriétaire. Utilisez le jeton du propriétaire.',
          jellyfin_unreachable:
            'Rien n’a répondu à cette adresse. Vérifiez-la, et que Plex Media Server est bien lancé.',
          not_jellyfin:
            'Un serveur a répondu à cette adresse, mais ce n’est pas Plex Media Server. Vérifiez l’adresse.',
        },
      },
      userStates: {
        waiting: 'En attente',
        reading: 'Lecture',
        saving: 'Enregistrement',
        done: 'Terminé',
        failed: 'Échoué',
      },
      notImported: 'Non importé',
      userMapping: (jellyfinName: string, userName: string) => `${jellyfinName} → ${userName}`,
      read: (count: number) => (count <= 1 ? `${count} élément lu` : `${count} éléments lus`),
      counts: (played: number, resumed: number, favorites: number) =>
        `${played <= 1 ? `${played} marqué vu` : `${played} marqués vus`}, ${resumed <= 1 ? `${resumed} point de reprise` : `${resumed} points de reprise`}, ${favorites <= 1 ? `${favorites} favori` : `${favorites} favoris`}`,
      upToDate: (count: number) => `${count} déjà dans Polyfin`,
      ratings: (count: number) => (count <= 1 ? `${count} noté` : `${count} notés`),
      collections: (count: number) =>
        count <= 1 ? `${count} collection créée` : `${count} collections créées`,
      playlists: (count: number) =>
        count <= 1 ? `${count} playlist créée` : `${count} playlists créées`,
      listTitles: (count: number) =>
        count <= 1
          ? `${count} titre ajouté aux listes déjà importées`
          : `${count} titres ajoutés aux listes déjà importées`,
      skipped: {
        collection: (name: string) =>
          `Collection « ${name} » : aucun titre trouvé, elle n’a pas été créée.`,
        playlist: (name: string) =>
          `Playlist « ${name} » : aucun titre trouvé, elle n’a pas été créée.`,
      },
      watchlist: (count: number) =>
        count <= 1 ? `${count} ajouté à « À regarder »` : `${count} ajoutés à « À regarder »`,
      undated: (count: number) =>
        count <= 1
          ? `${count} point de reprise sans date, non ajouté`
          : `${count} points de reprise sans date, non ajoutés`,
      nothingAdded: {
        up_to_date: () =>
          'Tout ce qui a été lu était déjà dans Polyfin : il n’y avait rien à ajouter.',
        nothing_played: (server: string) =>
          `${server} ne liste rien de vu, en cours ni favori pour cet utilisateur.`,
        no_sections: () =>
          'Ce serveur Plex n’a aucune bibliothèque de films ou de séries : il n’y avait rien à lire.',
        plex_owner_nothing_watched: () =>
          'Le propriétaire n’a rien de vu ni en cours dans les bibliothèques de films et de séries de ce serveur.',
        plex_nothing_watched: () =>
          'Ce compte n’a rien de vu, en cours ni noté dans les bibliothèques de films et de séries de ce serveur.',
        plex_no_history: () =>
          'Plex ne liste aucune lecture de ce compte sur ce serveur. Son historique de lecture ne contient que les titres lus jusqu’au bout sur ce serveur : les titres marqués comme vus sans être lus, ou synchronisés depuis le compte Plex, n’y figurent pas. Pour tout reprendre, points de reprise compris, connectez-vous avec Plex en tant que propriétaire dans cet import, avec le code PIN d’un profil Plex Home qui en a un, ou la personne se connecte avec Plex dans Mon compte › Importer depuis un autre serveur.',
      },
      unmatched: (count: number) =>
        count <= 1 ? `${count} titre introuvable` : `${count} titres introuvables`,
      unmatchedTitle: (name: string) => `Titres introuvables pour ${name}`,
      unmatchedHelp: 'Ces titres n’ont pas été importés : la raison dit pourquoi.',
      titleColumn: 'Titre',
      kindColumn: 'Type',
      yearColumn: 'Année',
      reasonColumn: 'Raison',
      kinds: {
        movie: 'Film',
        episode: 'Épisode',
        series: 'Série',
      },
      reasons: {
        no_identifier: 'Aucun identifiant IMDb, TMDB ou TVDB',
        not_found: 'Absent de Polyfin',
        not_in_library: 'Plus dans les bibliothèques de Plex',
        other_episode_order: 'Numéroté selon un autre ordre des épisodes de TheTVDB',
      },
      more: (count: number) => (count <= 1 ? `Et ${count} autre.` : `Et ${count} autres.`),
    },
    appImport: {
      open: 'Importer depuis une appli',
      title: 'Importer depuis une appli',
      description:
        'Reprenez les données de visionnage d’un utilisateur depuis son compte sur une appli basée sur des addons : les films et épisodes vus avec leurs dates, ses points de reprise, et la bibliothèque de l’appli en favoris, reconnus par IMDb, TMDB ou Kitsu. Les données de Polyfin sont seulement complétées, et rien n’est écrit dans l’appli. Il vous faut les identifiants de l’utilisateur pour l’appli ; les utilisateurs peuvent aussi importer eux-mêmes, sous Mon compte.',
      user: 'Dans le compte de',
      userHelp:
        'L’utilisateur de Polyfin dont l’importation complète les données de visionnage et les addons.',
    },
    letterboxdImport: {
      open: 'Importer depuis Letterboxd',
      title: 'Importer depuis Letterboxd',
      description:
        'Reprenez les films d’un utilisateur depuis son fichier d’export Letterboxd : les films vus avec leurs dates et leur nombre de lectures, sa watchlist, ses notes, et les films aimés en favoris si vous le choisissez. Les données de Polyfin sont seulement complétées. Les utilisateurs peuvent aussi importer le leur, sous Mon compte.',
      user: 'Dans le compte de',
      userHelp: 'L’utilisateur de Polyfin dont l’importation complète les données de visionnage.',
    },
  },
  invites: {
    title: 'Liens d’invitation',
    create: 'Créer un lien d’invitation',
    empty: 'Aucun lien d’invitation pour le moment.',
    emptyHelp:
      'Un lien d’invitation permet aux personnes à qui vous l’envoyez de créer leur propre compte, avec les paramètres que vous choisissez.',
    maxUses: 'Comptes qu’il peut créer',
    maxUsesHelp: 'De 1 à 100. 1 par défaut.',
    expires: 'Expiration',
    after: (days: number) => (days === 1 ? 'Après 1 jour' : `Après ${days} jours`),
    never: 'Jamais',
    accountsEnd: 'Fin des comptes',
    accountsEndHelp:
      'Chaque compte que le lien crée prend fin ce nombre de jours après sa création.',
    accountsEndAfter: (days: number) =>
      days === 1
        ? 'Les comptes prennent fin après 1 jour'
        : `Les comptes prennent fin après ${days} jours`,
    model: 'Paramètres de',
    modelHelp:
      'Les nouveaux comptes reprennent les autorisations, les bibliothèques visibles, les genres bloqués, les heures autorisées, le contrôle parental, les limites de lecture et d’accès et le groupe de qualité de cet utilisateur. Ils ne sont jamais administrateurs.',
    newUser: 'Un nouvel utilisateur, comme avec Créer un utilisateur',
    submit: 'Créer le lien',
    creating: 'Création…',
    createdTitle: 'Lien d’invitation créé',
    createdNotice: 'Copiez ce lien maintenant : il n’est affiché que cette fois.',
    copy: 'Copier le lien',
    copied: 'Lien copié.',
    copyFailed: 'Le lien n’a pas pu être copié. Sélectionnez-le et copiez-le à la main.',
    done: 'Terminé',
    uses: (uses: number, maxUses: number) =>
      maxUses === 1
        ? `${uses} compte créé sur 1`
        : `${uses} ${uses <= 1 ? 'compte créé' : 'comptes créés'} sur ${maxUses}`,
    settingsOf: (name: string) => `Paramètres de ${name}`,
    newUserSettings: 'Paramètres d’un nouvel utilisateur',
    expiresLabel: 'Expire',
    expiredLabel: 'Expiré',
    neverExpires: 'N’expire jamais',
    createdBy: (name: string) => `Créé par ${name}`,
    createdByDeleted: 'Créé par un utilisateur supprimé',
    states: {
      active: 'Actif',
      used_up: 'Épuisé',
      expired: 'Expiré',
      revoked: 'Révoqué',
    },
    revoke: 'Révoquer',
    revokeTitle: 'Révoquer ce lien d’invitation ?',
    revokeConfirm:
      'Le lien cesse aussitôt de fonctionner. Les comptes déjà créés grâce à lui sont conservés.',
    revoking: 'Révocation…',
    revoked: 'Le lien d’invitation est révoqué.',
  },
  groups: {
    tabsLabel: 'Utilisateurs et groupes',
    title: 'Groupes',
    description:
      'Un groupe réunit les limites de la page d’un utilisateur. Ses membres le suivent : une modification du groupe les atteint tous aussitôt.',
    listTitle: 'Groupes',
    empty: 'Aucun groupe pour le moment.',
    emptyHelp:
      'Créez un groupe pour régler en une fois les autorisations, les limites de lecture, le contrôle parental, les bibliothèques, les genres et les horaires de plusieurs comptes.',
    create: 'Créer un groupe',
    name: 'Nom',
    nameTaken: 'Ce nom est déjà utilisé par un autre groupe.',
    nameRule: 'De 1 à 64 caractères, différent du nom des autres groupes.',
    copyFrom: 'Paramètres de',
    copyFromHelp:
      'Le groupe commence avec les autorisations, les limites de lecture et d’accès, le groupe de qualité, le contrôle parental, les bibliothèques visibles, les genres bloqués et les horaires autorisés de cet utilisateur.',
    submit: 'Créer le groupe',
    creating: 'Création…',
    created: (name: string) => `Le groupe ${name} a été créé.`,
    memberCount: (count: number) =>
      count === 0 ? 'Aucun membre' : count === 1 ? '1 membre' : `${count} membres`,
    notFound: 'Groupe introuvable',
    notFoundHelp: 'Il a peut-être été supprimé.',
    backToGroups: 'Retour aux groupes',
    sectionsLabel: 'Sections du groupe',
    pageHelp:
      'Chaque section s’enregistre à part et atteint aussitôt les membres qui suivent le groupe.',
    rename: 'Renommer',
    renamed: 'Le groupe a été renommé.',
    updated: 'Le groupe a été mis à jour.',
    accessHelp:
      'Les autorisations que les membres reçoivent du groupe. Leur rôle, l’écran de connexion, « Désactivé » et la date de fin restent sur leur propre page.',
    delete: 'Supprimer le groupe',
    deleteTitle: (name: string) => `Supprimer le groupe ${name} ?`,
    deleteConfirm:
      'Ses membres gardent les valeurs du groupe comme leurs propres réglages, que leur page peut ensuite modifier.',
    deleting: 'Suppression…',
    deleted: (name: string) => `Le groupe ${name} a été supprimé.`,
    membersTitle: 'Membres',
    membersHelp:
      'Un membre qui ne suit plus le groupe garde ses valeurs comme ses propres réglages, marqués « Réglages propres ».',
    noMembers: 'Aucun membre pour le moment.',
    noMembersHelp:
      'Ajoutez des membres ici, ou choisissez le groupe dans la section Accès de la page d’un utilisateur.',
    ownSettings: 'Réglages propres',
    followsHelp: 'Suit le groupe',
    ownSettingsHelp: 'Garde ses propres réglages',
    addMembers: 'Ajouter des membres',
    addMembersTitle: (name: string) => `Ajouter des membres à ${name}`,
    addMembersHelp:
      'Les utilisateurs choisis suivent le groupe : ils prennent ses valeurs à la place des leurs. Les administrateurs ne peuvent pas rejoindre un groupe.',
    noCandidates: 'Personne à ajouter.',
    noCandidatesHelp:
      'Tous les utilisateurs qui ne sont pas administrateurs sont déjà dans ce groupe.',
    movesFrom: (name: string) => `Quitte le groupe ${name}`,
    add: 'Ajouter',
    adding: 'Ajout…',
    membersAdded: (count: number) =>
      count === 1 ? '1 membre a été ajouté.' : `${count} membres ont été ajoutés.`,
    group: 'Groupe',
    noGroup: 'Aucun groupe',
    groupHelp:
      'Un membre suit le groupe : ses valeurs s’appliquent, et cette page les affiche en lecture seule. Le quitter les garde comme les réglages propres de l’utilisateur.',
    administratorNoGroup:
      'Les administrateurs ne peuvent pas rejoindre un groupe, car leurs autorisations diffèrent.',
    administratorLeaves: (name: string) =>
      `Rendre cet utilisateur administrateur le retire du groupe ${name}.`,
    newAccountGroupHelp: 'Les comptes créés rejoignent ce groupe et le suivent.',
    newUserGroupHelp: 'L’utilisateur rejoint ce groupe et le suit.',
    followGroup: 'Suivre le groupe',
    followGroupHelp:
      'Désactivé, l’utilisateur garde les valeurs actuelles du groupe comme les siennes, et cette page peut les modifier.',
    setByGroup: (name: string) => `Réglé par le groupe ${name}.`,
    openGroup: 'Ouvrir le groupe',
    joinTitle: 'Rejoindre le groupe ?',
    joinConfirm: (user: string, group: string) =>
      `${user} rejoint ${group} et le suit : les valeurs du groupe remplacent les siennes dans Accès, Lecture et accès, Contrôle parental, Bibliothèques visibles, Genres bloqués et Horaires autorisés.`,
    join: 'Rejoindre',
    followAgainTitle: 'Suivre de nouveau le groupe ?',
    followAgainConfirm: (user: string, group: string) =>
      `${user} reprend les valeurs de ${group} : ses propres réglages dans les sections du groupe sont remplacés.`,
    followAgain: 'Suivre le groupe',
    groupBadge: (name: string) => `Groupe ${name}`,
    groupOwnBadge: (name: string) => `Groupe ${name}, réglages propres`,
    filter: 'Afficher les utilisateurs de',
    allGroups: 'Tous les groupes',
    noneShown: 'Aucun utilisateur ici.',
  },
  resetDefaults: {
    button: 'Rétablir les réglages par défaut',
    title: (name: string) => `Rétablir les réglages par défaut de ${name} ?`,
    groupTitle: (name: string) => `Rétablir les réglages par défaut du groupe ${name} ?`,
    userHelp: (name: string) =>
      `Les autorisations d’Accès, Lecture et accès avec le Groupe de qualité, Contrôle parental, Bibliothèques visibles, Genres bloqués et Horaires autorisés reprennent les réglages d’un nouvel utilisateur, ceux que donne « Créer un utilisateur » ; les applications de ${name} déjà connectées les ont aussitôt. Son nom et son mot de passe, Administrateur, Afficher sur l’écran de connexion, Désactivé, Prend fin le, son groupe, ses propres addons, ses connexions, ses comptes de suivi et tout ce qu’il a regardé restent.`,
    ownSettingsHelp: (group: string) =>
      `Cet utilisateur reste dans le groupe ${group} avec ses propres réglages, sans le suivre.`,
    groupHelp:
      'Les autorisations d’Accès, Lecture et accès avec le Groupe de qualité, Contrôle parental, Bibliothèques visibles, Genres bloqués et Horaires autorisés reprennent les réglages d’un nouvel utilisateur. Le nom et les membres du groupe restent.',
    followers: (count: number) =>
      count === 0
        ? 'Aucun membre ne suit le groupe : le changement n’atteint personne.'
        : count === 1
          ? '1 membre suit le groupe et prend aussitôt ces réglages.'
          : `${count} membres suivent le groupe et prennent aussitôt ces réglages.`,
    changesTitle: 'Ce qui change',
    loading: 'Recherche de ce qui change…',
    noChange: 'Rien ne change : ce sont déjà les réglages d’un nouvel utilisateur.',
    confirm: 'Rétablir',
    resetting: 'Rétablissement…',
    done: (name: string) => `Les réglages par défaut de ${name} ont été rétablis.`,
    groupDone: (name: string) => `Les réglages par défaut du groupe ${name} ont été rétablis.`,
  },
}

export default users
