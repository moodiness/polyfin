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
    canTranscode: 'Peut utiliser la conversion (transcodage)',
    canTranscodeHelp:
      'Le serveur peut convertir l’image et le son quand l’application de cet utilisateur ne sait pas lire un fichier tel quel. Sinon, l’image et le son ne sont jamais modifiés.',
    canDownload: 'Peut télécharger',
    canDownloadHelp:
      'Permet à cet utilisateur d’enregistrer des titres dans les applications Jellyfin pour les regarder hors connexion.',
    noTranscoding: 'Sans conversion',
    noDownloads: 'Sans téléchargement',
    turnOffDownloads: 'Désactiver les téléchargements pour tous',
    turnOffDownloadsConfirm:
      'Tous les utilisateurs perdent l’autorisation de télécharger, y compris dans les applications Jellyfin déjà connectées. Vous pouvez la rendre à un utilisateur depuis sa page, dans Accès.',
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
    qualityGroup: 'Groupe de qualité',
    qualityGroupHelp:
      'La meilleure définition proposée à cet utilisateur. Les versions de définition supérieure sont écartées tant qu’une autre convient ; si aucune ne convient, elles sont converties, ou refusées si cet utilisateur ne peut pas utiliser la conversion. La TV en direct est aussi convertie, et seules les versions qui conviennent peuvent être téléchargées.',
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
      open: 'Importer depuis Jellyfin',
      title: 'Importer depuis Jellyfin',
      description:
        'Reprenez les comptes d’un serveur Jellyfin et ce qu’ils ont regardé : noms des comptes et statut d’administrateur, puis pour chaque utilisateur les films et épisodes vus avec leurs dates, les points de reprise et les favoris, reconnus par IMDb, TMDB ou TVDB. Les données de Polyfin sont seulement complétées, et rien n’est écrit dans Jellyfin. Les mots de passe ne peuvent pas être importés : vous en choisissez un pour chaque nouveau compte.',
      connectTitle: 'Se connecter à Jellyfin',
      connectHelp:
        'L’adresse et la clé d’API servent seulement à cette importation : Polyfin ne les enregistre pas.',
      address: 'Adresse du serveur',
      addressHelp:
        'L’adresse à laquelle Jellyfin s’ouvre dans un navigateur. Une adresse du réseau local fonctionne.',
      apiKey: 'Clé d’API',
      apiKeyHelp: 'Créez-en une dans le tableau de bord de Jellyfin, sous Clés API.',
      connect: 'Se connecter',
      connecting: 'Connexion…',
      chooseTitle: 'Choisir qui importer',
      chooseHelp: (server: string, version: string) =>
        `${server}, Jellyfin ${version}. Pour chaque utilisateur, choisissez le compte Polyfin dans lequel importer, ou créez-en un.`,
      changeServer: 'Changer de serveur',
      noUsers: 'Ce serveur Jellyfin n’a aucun utilisateur.',
      lastActivityLabel: 'Dernière activité :',
      importAs: 'Importer dans',
      skip: 'Ne pas importer',
      newUser: 'Nouvel utilisateur',
      watchData: 'Importer les données de visionnage',
      watchDataHelp: 'Titres vus avec leurs dates, points de reprise et favoris.',
      watchDataNeeded: 'Sans elles, rien n’est importé pour cet utilisateur.',
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
        jellyfin_unreachable:
          'Jellyfin est devenu injoignable pendant l’importation. Ce qui a été importé est gardé.',
        jellyfin_key_refused:
          'Jellyfin a refusé la clé d’API pendant l’importation : elle a peut-être été révoquée. Ce qui a été importé est gardé.',
        not_jellyfin:
          'Le serveur a cessé de répondre comme Jellyfin pendant l’importation. Ce qui a été importé est gardé.',
        internal:
          'Polyfin a rencontré une erreur pendant l’importation : le journal du serveur donne le détail. Ce qui a été importé est gardé.',
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
      unmatched: (count: number) =>
        count <= 1 ? `${count} titre introuvable` : `${count} titres introuvables`,
      unmatchedTitle: (name: string) => `Titres introuvables pour ${name}`,
      unmatchedHelp:
        'Ces titres n’ont pas été importés : aucun titre de Polyfin ne leur correspond par IMDb, TMDB ou TVDB.',
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
      },
      more: (count: number) => (count <= 1 ? `Et ${count} autre.` : `Et ${count} autres.`),
    },
  },
}

export default users
