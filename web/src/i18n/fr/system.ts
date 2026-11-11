import type en from '../en/system'

const system: typeof en = {
  apiKeys: {
    title: 'Clés d’API',
    description:
      'Les clés permettent à des outils et applications, comme des gestionnaires de demandes ou des scripts, d’utiliser l’API Jellyfin de ce serveur avec les droits d’administrateur. Toute personne qui a une clé a ces droits\u202f: gardez vos clés secrètes et révoquez celles que vous n’utilisez plus.',
    listTitle: 'Clés',
    empty: 'Aucune clé d’API pour l’instant.',
    created: 'Créée',
    lastUsed: 'Dernière utilisation',
    createTitle: 'Créer une clé',
    app: 'Nom de l’application',
    appHint: 'Qui va utiliser cette clé, pour la reconnaître plus tard. De 1 à 64 caractères.',
    create: 'Créer la clé',
    creating: 'Création…',
    newKeyTitle: (app: string) => `Clé pour ${app}`,
    newKeyNotice: 'Copiez cette clé maintenant\u202f: elle ne sera plus affichée.',
    copy: 'Copier',
    copied: 'Copiée.',
    copyFailed: 'Impossible de copier la clé. Sélectionnez-la et copiez-la vous-même.',
    done: 'Je l’ai copiée',
    revoke: 'Révoquer',
    revoking: 'Révocation…',
    revokeConfirm: (app: string) =>
      `Révoquer la clé pour ${app}\u202f? Les outils et applications qui l’utilisent ne fonctionneront plus. Impossible de revenir en arrière.`,
    revoked: (app: string) => `La clé pour ${app} a été révoquée.`,
    emptyHint:
      'Créez-en une ci-dessous pour chaque outil ou application, pour pouvoir les révoquer une à une.',
    revokeTitle: 'Révoquer cette clé\u202f?',
    revokeLabel: (app: string) => `Révoquer la clé pour ${app}`,
  },
  system: {
    schedule: {
      title: 'Planning',
      description:
        'Les tâches du serveur, les enregistrements à venir et les prochaines mises à jour de la TV en direct.',
      tasksTitle: 'Tâches',
      lastRun: 'Dernière exécution',
      nextRun: 'Prochaine exécution',
      notRunYet: 'Pas encore lancée',
      byHandOnly: 'Lancée à la main seulement',
      took: (span: string) => `en ${span}`,
      results: {
        Completed: 'Terminée',
        Failed: 'Échec',
        Cancelled: 'Arrêtée',
      },
      running: 'En cours',
      cancelling: 'Arrêt en cours',
      runNow: 'Lancer',
      runLabel: (name: string) => `Lancer ${name} maintenant`,
      stop: 'Arrêter',
      stopLabel: (name: string) => `Arrêter ${name}`,
      started: (name: string) => `${name} est lancée.`,
      triggersButton: 'Déclencheurs',
      triggersLabel: (name: string) => `Modifier les déclencheurs de ${name}`,
      triggers: {
        title: (name: string) => `Déclencheurs\u202f: ${name}`,
        serverTime: 'Les heures sont celles du fuseau horaire du serveur.',
        none: 'Aucun déclencheur\u202f: cette tâche ne se lance qu’à la main.',
        keepOne: 'Cette tâche a besoin d’au moins un déclencheur.',
        locked: {
          settings: 'La planification de cette tâche est fixe\u202f: elle suit ses paramètres.',
          recordings:
            'La planification de cette tâche est fixe\u202f: les enregistrements de séries en dépendent.',
          notifications:
            'La planification de cette tâche est fixe\u202f: le résumé part de lui-même, comme le règle Paramètres › Notifications.',
          byHand: 'La planification de cette tâche est fixe\u202f: elle ne se lance qu’à la main.',
        },
        daily: (time: string) => `Tous les jours à ${time}`,
        weekly: (day: string, _index: number, time: string) => `Chaque ${day} à ${time}`,
        interval: (count: number, unit: 'minute' | 'hour' | 'day') =>
          unit === 'day'
            ? count === 1
              ? 'Tous les jours'
              : `Tous les ${count} jours`
            : count === 1
              ? `Toutes les ${unit === 'hour' ? 'heures' : 'minutes'}`
              : `Toutes les ${count} ${unit === 'hour' ? 'heures' : 'minutes'}`,
        startup: 'Au démarrage',
        stopsAfter: (span: string) => `Arrêtée au bout de ${span}`,
        remove: 'Retirer',
        removeLabel: (trigger: string) => `Retirer le déclencheur «\u202f${trigger}\u202f»`,
        add: 'Ajouter un déclencheur',
        type: 'Quand',
        types: {
          daily: 'Tous les jours',
          weekly: 'Chaque semaine',
          interval: 'À intervalle régulier',
          startup: 'Au démarrage',
        },
        time: 'Heure',
        day: 'Jour',
        intervalLabel: 'Intervalle',
        duplicate: 'Ce déclencheur est déjà dans la liste.',
        full: (max: number) => `Une tâche a au plus ${max} déclencheurs.`,
        reset: 'Rétablir les déclencheurs par défaut',
        saved: (name: string) => `Déclencheurs enregistrés\u202f: ${name}`,
        resetDone: (name: string) => `Déclencheurs par défaut rétablis\u202f: ${name}`,
      },
      autoRefresh: 'Mise à jour toutes les 5 secondes.',
      recordingsTitle: 'Enregistrements à venir',
      recordingsOff:
        'L’enregistrement est désactivé. Activez-le dans Paramètres › Enregistrements.',
      noTimers: 'Aucun enregistrement n’est prévu.',
      recordingNow: 'En cours',
      failed: 'Échec',
      series: 'Série',
      scheduledBy: (name: string) => `Prévu par ${name}`,
      unknownChannel: 'Chaîne introuvable',
      refreshesTitle: 'Mises à jour de la TV en direct',
      refreshesHelp: (hours: number) =>
        `Les listes de chaînes IPTV et les guides des programmes, ceux du serveur comme ceux des utilisateurs, sont téléchargés à nouveau toutes les ${hours} h (Paramètres › TV en direct), par la tâche «\u202fActualiser les guides TV\u202f».`,
      noRefreshes: 'Aucune source IPTV ni guide des programmes.',
      channelList: 'Liste de chaînes',
      iptvLists: 'Listes de chaînes, films et séries',
      guide: 'Guide des programmes',
      due: 'À la prochaine vérification',
      lastDownload: 'Dernier téléchargement',
      nextDownload: 'Prochain téléchargement',
      notYet: 'Pas encore',
      noTasks: 'Aucune tâche.',
      noTasksHint: 'Le serveur liste ici ses tâches une fois démarré.',
      recordingsHint:
        'Les enregistrements programmés depuis une application ou depuis le guide apparaissent ici.',
      refreshesHint:
        'Ajoutez une source IPTV dans Contenu › Sources, ou un guide des programmes à un catalogue TV, et leurs téléchargements apparaissent ici.',
      backupsTitle: 'Sauvegardes de la base de données',
      backUpNow: 'Sauvegarder maintenant',
    },
    health: {
      title: 'Santé',
      noIptv: 'Aucune source IPTV.',
      owner: { server: 'Serveur', user: (name: string) => `Addons de ${name}` },
      description:
        'L’état du serveur et de ses sources, d’après ce que Polyfin enregistre déjà\u202f: cette page ne leur envoie aucune requête.',
      problemsTitle: 'À regarder',
      allGood: 'Aucun problème.',
      update: (version: string) => `Polyfin ${version} est disponible`,
      updateRunning: (version: string) => `Ce serveur est en version ${version}.`,
      updateKind: 'Mise à jour',
      releaseNotes: 'Notes de version',
      checkedAt: (time: string) => `Vérifié à ${time}. Mise à jour toutes les 10 secondes.`,
      ok: 'OK',
      problemAction: {
        details: 'Voir le détail',
        settings: 'Ouvrir les paramètres',
        sources: 'Ouvrir Sources',
        source: 'Ouvrir la source',
        user: 'Ouvrir l’utilisateur',
        schedule: 'Ouvrir la planification',
        open: 'Ouvrir',
      },
      checksTitle: 'Vérifications',
      moreTitle: 'Aussi sur cette page',
      checks: {
        summary: (total: number, parts: string[]) =>
          `${total === 1 ? '1 vérification' : `${total} vérifications`}${parts.length > 0 ? ` · ${parts.join(', ')}` : ''}`,
        fineCount: (count: number) => `${count} en ordre`,
        failingCount: (count: number) => `${count} en échec`,
        quietCount: (count: number) => `${count} désactivées ou en attente`,
        none: 'Aucun',
        off: 'Désactivé',
        ready: 'Prêt',
        full: 'Complet',
        fine: 'En ordre',
        encrypted: 'Chiffrées',
        notEncrypted: 'Non chiffrées',
        keySet: 'Clé secrète définie',
        keyNotSet: 'Aucune clé secrète',
        processor: 'Processeur seul',
        databaseSize: 'Taille sur le disque',
        details: 'Détail',
        openSettings: 'Paramètres › Sauvegardes',
        noKey: 'Aucune clé TMDB',
        held: 'En pause',
        unknown: 'Le serveur ne dit rien de cette partie pour l’instant.',
        addons: (count: number) => (count === 1 ? '1 addon' : `${count} addons`),
        iptv: (count: number) => (count === 1 ? '1 source IPTV' : `${count} sources IPTV`),
        guides: (count: number) =>
          count === 1 ? '1 guide des programmes' : `${count} guides des programmes`,
        moreFolders: (folder: string, more: number) => `${folder} et ${more} autres`,
        versionUptime: (version: string, span: string) => `${version} · lancé depuis ${span}`,
      },
      problems: {
        database: 'La base de données ne répond pas.',
        disk: (folder: string, free: string) =>
          `Il reste peu de place pour le dossier ${folder}\u202f: ${free} libres.`,
        addon: (name: string, failure: string) => `${name}\u202f: ${failure}`,
        addonTimingOut: (name: string, until: string) =>
          `${name} ne répond pas à temps quand on lui demande des versions\u202f: les requêtes de Polyfin en arrière-plan vers son serveur sont suspendues jusqu’à ${until}. Les lectures l’interrogent toujours. S’il rassemble les versions d’autres addons, désactivez-y celui qui ne répond plus, ou raccourcissez le temps qu’il leur accorde\u202f; sinon, désactivez-le.`,
        iptv: (name: string) => `${name}\u202f: le dernier téléchargement de la liste a échoué.`,
        tuner: (name: string) => `${name}\u202f: le tuner n’a pas pu lire une chaîne.`,
        tunerUnreachable: (name: string) => `${name}\u202f: le tuner ne répond pas.`,
        folder: (name: string) =>
          `${name}\u202f: Polyfin ne peut pas lire ce dossier local. Vérifiez qu’il est monté et lisible par l’utilisateur 65532.`,
        share: (name: string) =>
          `${name}\u202f: Polyfin ne peut pas lire ce partage réseau. Vérifiez son adresse, et que son utilisateur peut le lire.`,
        shareUnreachable: (name: string) =>
          `${name}\u202f: Polyfin ne peut pas joindre ce partage réseau.`,
        shareRefused: (name: string) =>
          `${name}\u202f: le partage réseau refuse l’utilisateur ou le mot de passe de Polyfin.`,
        shareHostKey: (name: string) =>
          `${name}\u202f: le serveur SFTP présente une autre clé d’hôte que celle enregistrée, et Polyfin la refuse tant qu’elle n’est pas acceptée.`,
        folderWatchLimit: (name: string) =>
          `${name}\u202f: Polyfin ne peut pas surveiller les changements de ce dossier local\u202f: la limite de dossiers surveillés du système est atteinte. Il est toujours analysé selon sa planification. Augmentez fs.inotify.max_user_watches sur l’hôte.`,
        folderInstanceLimit: (name: string) =>
          `${name}\u202f: Polyfin ne peut pas surveiller les changements de ce dossier local\u202f: la limite d’observateurs ou de fichiers ouverts du système est atteinte. Il est toujours analysé selon sa planification. Augmentez fs.inotify.max_user_instances sur l’hôte, ou la limite de fichiers ouverts du conteneur.`,
        folderOpenFilesLimit: (name: string) =>
          `${name}\u202f: Polyfin ne peut pas surveiller les changements de ce dossier local\u202f: la limite de fichiers ouverts du système est atteinte, macOS surveillant chaque dossier et fichier par l’un d’eux. Il est toujours analysé selon sa planification. Augmentez kern.maxfilesperproc et kern.maxfiles avec sysctl.`,
        folderUnwatched: (name: string) =>
          `${name}\u202f: Polyfin ne peut pas surveiller les changements de ce dossier local. Il est toujours analysé selon sa planification.`,
        shareUnwatchedUnreachable: (name: string) =>
          `${name}\u202f: Polyfin ne peut pas surveiller les changements de ce partage réseau\u202f: il est injoignable. Il est analysé selon sa planification, et de nouveau dès qu’il répond.`,
        shareUnwatchedRefused: (name: string) =>
          `${name}\u202f: Polyfin ne peut pas surveiller les changements de ce partage réseau\u202f: il refuse l’utilisateur ou le mot de passe de Polyfin. Vérifiez-les, et saisissez-les à nouveau.`,
        shareUnwatched: (name: string) =>
          `${name}\u202f: Polyfin ne peut pas surveiller les changements de ce partage réseau. Il est toujours analysé selon sa planification.`,
        folderNoSearch: (name: string) =>
          `${name}\u202f: aucun addon du serveur ne décrit ni ne recherche les titres de ses fichiers, qui restent donc non associés. Ajoutez-en un dans Contenu › Sources\u202f: les addons propres aux utilisateurs, dans Mes sources, ne comptent pas.`,
        folderSearch: (name: string, addon: string, failure: string) =>
          `${name}\u202f: les titres de ses fichiers n’ont pas pu être recherchés${addon === '' ? '' : failure === '' ? ` (${addon})` : ` (${addon}\u202f: ${failure})`}. Ils restent non associés jusqu’à ce qu’une recherche aboutisse, ce que Polyfin réessaie de lui-même, après quelques minutes, puis moins souvent.`,
        searchLate: (name: string) =>
          `${name}\u202f: sa recherche n’a pas répondu à temps. Les recherches le laissent de côté quelques minutes, puis l’interrogent de nouveau.`,
        smartListEmpty: (name: string, owner: string | null) =>
          owner === null
            ? `La liste intelligente ${name} ne liste rien\u202f: les catalogues dont elle tirait ses titres ont disparu. Choisissez-lui d’autres catalogues.`
            : `La liste intelligente ${name} de ${owner} ne liste rien\u202f: les catalogues dont elle tirait ses titres ont disparu. Il faut lui en choisir d’autres.`,
        virtualChannelSource: (name: string) =>
          `La chaîne ${name} diffuse une source qui n’existe plus. Choisissez-lui une autre source.`,
        guide: (name: string) => `${name}\u202f: la dernière récupération du guide a échoué.`,
        paused: (host: string) =>
          `Miniatures en pause pour ${host}\u202f: la source a demandé de ralentir.`,
        hostErrorFile: (host: string, since: string) =>
          `L’hôte ${host} répond par un fichier d’erreur au lieu des vidéos, depuis ${since}.`,
        hostErrorFileHint:
          'Cela signifie souvent que le compte auprès de ce service demande votre attention (un changement de mot de passe, un abonnement, une limite atteinte), ou que le service est en panne\u202f: vérifiez-le sur son site.',
        unknownProxy: (address: string) =>
          `${address} transmet des requêtes (X-Forwarded-For) sans être un proxy connu\u202f: tous ses clients comptent donc comme ${address}. Si c’est votre proxy inverse, ajoutez-le aux Proxys connus dans Paramètres › Sécurité.`,
        discoveryPort: (port: number) =>
          `Polyfin ne peut pas écouter sur le port UDP ${port}\u202f: les applications Jellyfin du réseau local ne le trouvent donc pas d’elles-mêmes. Un autre programme l’utilise sans doute, comme un serveur Jellyfin sur le même hôte. Enregistrer les paramètres réessaie.`,
        torrentRoute: (why: string) =>
          `Les flux torrent ne sont pas lus\u202f: ${why}. Rien n’est envoyé par un autre chemin\u202f; vérifiez la route dans Paramètres › Torrents.`,
        torrentRouteRiding: (why: string, seconds: number) =>
          `${why}. Les torrents en lecture continuent jusqu’à ${seconds} s\u202f; les nouveaux ne peuvent pas démarrer. Rien n’est envoyé par un autre chemin\u202f; vérifiez la route dans Paramètres › Torrents.`,
        torrentWhy: {
          interface_missing: (name: string) => `l’interface réseau ${name} est absente`,
          interface_down: (name: string) => `l’interface réseau ${name} est désactivée`,
          interface_no_address: (name: string) => `l’interface réseau ${name} n’a pas d’adresse`,
          interface_unsupported: () => 'une interface réseau ne fonctionne que sous Linux',
          proxy_unreachable: (name: string) => `le proxy SOCKS5 ${name} ne répond pas`,
          proxy_refused: (name: string) =>
            `le proxy SOCKS5 ${name} a refusé l’utilisateur et le mot de passe`,
          wireguard_endpoint: (name: string) => `le serveur WireGuard ${name} est introuvable`,
          wireguard_down: (name: string) => `le tunnel WireGuard vers ${name} ne peut pas démarrer`,
          wireguard_handshake: (name: string) =>
            `le serveur WireGuard ${name} ne répond pas à la négociation\u202f: il est injoignable, ou une clé de la configuration est erronée`,
          no_route: () => 'aucune route n’est définie',
        },
        torrentUdp: (name: string) =>
          `Le proxy SOCKS5 ${name} ne propose pas UDP ASSOCIATE\u202f: les torrents sont lus en TCP, sans trackers UDP ni DHT.`,
        torrentStart: (why: string) =>
          `Un torrent n’a pas démarré à temps\u202f: ${why}. Vérifiez que la route atteint Internet\u202f; le journal donne la réponse de chaque tracker au niveau débogage.`,
        torrentStartWhy: {
          no_udp:
            'aucun pair n’a été trouvé, et la route ne transporte pas l’UDP, dont ont besoin le DHT et les trackers UDP',
          refused:
            'ses trackers ont refusé l’annonce ou n’ont pas répondu, et le DHT n’a trouvé aucun pair',
          no_peers: 'ses trackers et le DHT n’ont trouvé aucun pair',
          no_trackers: 'l’addon n’a indiqué aucun tracker, et le DHT n’a trouvé aucun pair',
          silent: 'des pairs ont été trouvés, mais aucun ne l’a envoyé',
        },
        torrentStopped: (at: string, why: string) =>
          `Un torrent s’est arrêté pendant la lecture à ${at}\u202f: ${why}. Rien n’a été envoyé par un autre chemin. Vérifiez que la route reste joignable\u202f; la prochaine lecture relance le torrent.`,
        torrentStoppedWhy: {
          interface_missing: (name: string) => `l’interface réseau ${name} a disparu`,
          interface_down: (name: string) => `l’interface réseau ${name} s’est désactivée`,
          interface_no_address: (name: string) => `l’interface réseau ${name} a perdu son adresse`,
          proxy_unreachable: (name: string) => `le proxy SOCKS5 ${name} n’a pas répondu`,
          proxy_refused: (name: string) =>
            `le proxy SOCKS5 ${name} a refusé l’utilisateur et le mot de passe`,
          wireguard_endpoint: (name: string) =>
            `le serveur WireGuard ${name} est devenu introuvable`,
          wireguard_down: (name: string) => `le tunnel WireGuard vers ${name} s’est arrêté`,
          wireguard_handshake: (name: string) =>
            `le serveur WireGuard ${name} n’a pas répondu à la négociation`,
          route: () => 'sa route n’a pas répondu',
        },
        torrentStartPeers: (found: number, why: string) =>
          `${found} ${found > 1 ? 'pairs trouvés' : 'pair trouvé'}\u202f: ${why}`,
        torrentPeerWhy: {
          connected: (n: number) => `${n} ${n > 1 ? 'connectés' : 'connecté'}`,
          timedOut: (n: number) => `${n} sans réponse à temps`,
          refused: (n: number) => `${n} ${n > 1 ? 'ont refusé' : 'a refusé'} la connexion`,
          unreachable: (n: number) => `${n} ${n > 1 ? 'injoignables' : 'injoignable'}`,
          proxy: (n: number) => `${n} ${n > 1 ? 'refusés' : 'refusé'} par le proxy`,
          closed: (n: number) =>
            `${n} ${n > 1 ? 'ont fermé' : 'a fermé'} la connexion à la négociation`,
          left: (n: number) => `${n} ${n > 1 ? 'connectés, puis partis' : 'connecté, puis parti'}`,
          other: (n: number) => `${n} en échec pour une autre raison`,
          untried: (n: number) => `${n} pas encore ${n > 1 ? 'contactés' : 'contacté'}`,
        },
        dlnaPort: (port: number) =>
          `Polyfin ne peut pas écouter sur le port UDP ${port}\u202f: les appareils DLNA du réseau local ne le trouvent donc pas d’eux-mêmes. Un autre programme l’utilise sans doute, comme un autre serveur DLNA sur le même hôte. Enregistrer les paramètres réessaie.`,
        discoveryAddress: (address: string) =>
          `Les applications Jellyfin qui cherchent des serveurs sur le réseau local reçoivent ${address}, une adresse du conteneur de Polyfin qu’elles ne peuvent pas joindre. Renseignez Adresse sur le réseau local dans Paramètres › Général.`,
        proxyNameUnresolved: (name: string) =>
          `Le proxy connu ${name} ne correspond à aucune adresse\u202f: aucune requête n’est donc reconnue comme venant de lui. Vérifiez son nom dans Paramètres › Sécurité, et que Polyfin partage un réseau avec lui.`,
        task: (name: string) => `La tâche «\u202f${name}\u202f» a échoué.`,
        httpsPort: (port: number) =>
          `Polyfin ne peut pas écouter sur le port TCP ${port}\u202f: HTTPS n’est donc pas servi. HTTP fonctionne toujours. Un autre programme l’utilise sans doute, ou il est inférieur à 1024. Choisissez un autre port HTTPS dans Paramètres › Sécurité.`,
        httpsFileMissing: (file: string) =>
          `Le fichier de certificat HTTPS ${file} n’existe pas. Le dernier certificat lu reste utilisé.`,
        httpsFilesMismatch:
          'La clé privée HTTPS ne correspond pas au certificat. Le dernier certificat lu reste utilisé.',
        httpsFileUnreadable: (file: string) =>
          `Polyfin ne peut pas lire le fichier de certificat HTTPS ${file}, ou il ne contient aucun certificat ou clé PEM qu’il sait lire. Le dernier certificat lu reste utilisé.`,
        httpsRenewal: (error: string) =>
          `Let’s Encrypt n’a pas délivré le certificat HTTPS\u202f: ${error}`,
        httpsExpires: (date: string) => `Le certificat HTTPS expire le ${date}.`,
        httpsExpired: (date: string) => `Le certificat HTTPS a expiré le ${date}.`,
        subtitleOcrLanguage: (name: string, file: string) =>
          `Les sous-titres en images en ${name} ne sont pas lus en texte\u202f: Tesseract n’a pas de données pour cette langue. Ajoutez le fichier ${file} au dossier que nomme POLYFIN_TESSDATA_DIR.`,
        subtitleOcrDownloadFailed: (error: string) =>
          `Le dernier téléchargement de ses données a échoué\u202f: ${error}`,
        metadataLanguages: (name: string) =>
          `${name}\u202f: chaque addon de métadonnées du serveur a une langue\u202f: ils décrivent donc les titres dans leur langue pour tous les utilisateurs, quelle que soit leur langue des métadonnées. Laissez Langue de ses descriptions sur Aucune pour cet addon, ou pour un addon de métadonnées de plus.`,
        desktopRolledBack: (version: string, previous: string, error: string) =>
          `Polyfin Desktop est revenu à la version ${previous}, car la version ${version} n’a pas démarré\u202f: ${error}. Cette version n’est plus proposée\u202f; la suivante le sera.`,
        collationVersion: (name: string, recorded: string, actual: string) =>
          `La version de collation de la base de données ${name} a changé\u202f: PostgreSQL a enregistré ${recorded}, et la bibliothèque de collation du système fournit maintenant ${actual}. Ses index de texte peuvent ne plus être dans l’ordre, ce qui peut masquer des lignes ou les dupliquer.`,
        collationRepair:
          'Pour les réparer, arrêtez Polyfin, puis, connecté en tant que propriétaire de la base, exécutez ces commandes l’une après l’autre\u202f:',
        collationCopy: 'Copier les commandes',
        collationCopied: 'Les commandes sont copiées.',
        collationCopyFailed:
          'Les commandes n’ont pas pu être copiées\u202f: sélectionnez-les et copiez-les à la main.',
        collationDocs: 'Comment réparer',
        themeCopyMissing: (missing: number, files: number) =>
          `La copie du thème du lecteur web n’a plus ${missing} de ses ${files} fichiers dans le dossier de données de Polyfin, comme après la restauration d’une sauvegarde sur un autre système\u202f: les navigateurs les chargent en attendant depuis leurs adresses d’origine, ce qui demande internet. Mettez la copie à jour dans Paramètres › Lecteur web, ou choisissez un autre thème.`,
        folderUnwritable: (name: string, path: string) =>
          `${name} (${path})\u202f: Polyfin ne peut pas écrire dans ce dossier, et garde ces fichiers dans le cache de son dossier de données en attendant. Rendez le dossier accessible en écriture ou choisissez-en un autre, puis redémarrez Polyfin.`,
        nvidiaDriver: (version: string) =>
          `La carte graphique NVIDIA ne convertit pas la vidéo\u202f: son pilote est antérieur à la version ${version}, dont FFmpeg a besoin pour NVENC. Mettez à jour le pilote pour utiliser la carte.`,
        nvidiaOlderFFmpeg: (failure: 'download' | 'encode', error: string) =>
          failure === 'download'
            ? `La carte graphique NVIDIA ne convertit pas la vidéo\u202f: son pilote est antérieur à la version 610, et le FFmpeg pour ces pilotes n’a pas pu être téléchargé\u202f: ${error}. Polyfin réessaie à son prochain démarrage, ou quand les réglages de conversion sont enregistrés.`
            : `La carte graphique NVIDIA ne convertit pas la vidéo, même avec le FFmpeg pour les pilotes antérieurs à la version 610\u202f: ${error}`,
        backupFailed: 'La dernière sauvegarde de la base de données a échoué.',
        backupStale:
          'La dernière sauvegarde de la base de données est en retard\u202f: elle est plus ancienne que ne le permet sa planification.',
        backupDestinationFailed: (name: string) =>
          `${name}\u202f: le dernier envoi des sauvegardes de la base de données a échoué.`,
        backupDestinationStale: (name: string) =>
          `${name}\u202f: la sauvegarde de la base de données la plus récente qui s’y trouve date de plus de deux jours.`,
        conversionsFull:
          'Les conversions vidéo sont à leur limite\u202f: les nouvelles attendent ou sont lues autrement.',
        unreadableSecrets: (names: string) =>
          `Des clés enregistrées ne peuvent pas être déchiffrées avec POLYFIN_SECRET_KEY et comptent comme absentes\u202f: ${names}. Redonnez la clé avec laquelle elles ont été chiffrées, ou saisissez-les de nouveau.`,
        plaintextSecrets:
          'Les clés et jetons sont enregistrés sans chiffrement. Définissez POLYFIN_SECRET_KEY pour les chiffrer.',
        appStopped: (app: string, user: string) =>
          `${app} a refusé la connexion que Polyfin gardait pour ${user}\u202f: plus rien n’est importé tant que ${user} ne s’y reconnecte pas sous Mon compte, avec «\u202fRester connecté\u202f» coché.`,
        trackingStopped: (service: string, user: string) =>
          `${service} a refusé le jeton ou la clé que Polyfin gardait pour ${user}\u202f: plus rien ne lui est envoyé ni n’en est importé tant que ${user} n’appuie pas sur Reconnecter sous Mon compte › Suivi.`,
      },
      addonsTitle: 'Addons',
      addonsHelp:
        'D’après les requêtes faites pour les applis depuis le démarrage du serveur. «\u202fVérifier\u202f» demande une fois son manifeste à l’addon, au plus une fois par minute.',
      noAddons: 'Le serveur n’a aucun addon.',
      addonStatus: {
        ok: 'Répond',
        failing: 'En échec',
        unknown: 'Aucune requête depuis le démarrage',
        off: 'Désactivé',
      },
      failures: {
        unreachable: 'injoignable',
        timeout: 'n’a pas répondu à temps',
        private_network: 'est sur un réseau local',
        rate_limited: 'a demandé de ralentir (trop de requêtes)',
        server_error: 'a répondu par une erreur serveur',
        http_error: 'a refusé la requête',
        invalid_response: 'a envoyé une réponse trop grande',
      },
      lastSuccess: 'Dernière réponse',
      lastFailure: 'Dernier échec',
      responseTime: 'Temps de réponse',
      requestsTerm: 'Requêtes',
      requests: (count: number, failures: number) =>
        `${count} ${count === 1 ? 'requête' : 'requêtes'}, ${failures} en échec`,
      check: 'Vérifier',
      checkLabel: (name: string) => `Vérifier ${name} maintenant`,
      checking: 'Vérification…',
      downloadLanguage: 'Télécharger',
      downloadLanguageLabel: (name: string) => `Télécharger les données de Tesseract en ${name}`,
      downloadingLanguage: 'Téléchargement…',
      iptvTitle: 'Sources IPTV',
      guidesTitle: 'Guides des programmes',
      sharesTitle: 'Partages réseau',
      shareKinds: { smb: 'SMB', webdav: 'WebDAV', s3: 'S3', sftp: 'SFTP', rclone: 'rclone' },
      shareStatus: {
        notified: 'Notifié',
        polled: 'Scruté',
        unwatched: 'Non surveillé',
        scans: 'Aux analyses seulement',
      },
      sharePolledEvery: (share: string, minutes: number) =>
        `${share === 's3' ? 'Les buckets S3 ne signalent' : share === 'sftp' ? 'Les serveurs SFTP ne signalent' : 'Les remotes rclone ne signalent'} pas les changements\u202f: les dossiers du dossier sont listés de nouveau ${minutes === 1 ? 'chaque minute' : `toutes les ${minutes} minutes`}, comme le demandent ses réglages.`,
      shareScansOnly:
        'Ses réglages demandent de ne chercher les changements qu’aux analyses\u202f: le dossier est lu selon sa planification, et quand il est analysé à la main.',
      shareNotified:
        'Le serveur signale chaque changement à Polyfin\u202f: le partage est analysé à nouveau quelques secondes après. Ses dossiers sont aussi listés toutes les 10 minutes, au cas où un changement ne serait pas signalé.',
      sharePartial:
        'Le serveur signale à Polyfin les changements faits par le réseau, mais en a manqué un dans un dossier du partage, comme ceux faits sur son propre disque\u202f: les dossiers du partage sont aussi listés de nouveau toutes les 2 minutes.',
      sharePolled: {
        webdav:
          'Les serveurs WebDAV ne signalent pas les changements\u202f: les dossiers du partage sont listés de nouveau toutes les 2 minutes.',
        unsupported:
          'Ce serveur SMB ne signale pas les changements\u202f: les dossiers du partage sont listés de nouveau toutes les 2 minutes.',
        silent:
          'Ce serveur SMB n’a pas signalé un changement trouvé en listant le partage\u202f: ses dossiers sont listés de nouveau toutes les 2 minutes.',
        failed:
          'Polyfin ne reçoit plus les notifications du serveur et les redemande sous peu. En attendant, les dossiers du partage sont listés de nouveau toutes les 2 minutes.',
        starting:
          'Polyfin demande au serveur de lui signaler les changements. En attendant, les dossiers du partage sont listés de nouveau toutes les 2 minutes.',
      },
      shareUnwatched: {
        unreachable:
          'Le partage est injoignable. Il est analysé selon sa planification, et de nouveau dès qu’il répond.',
        refused:
          'Le partage refuse l’utilisateur ou le mot de passe de Polyfin. Vérifiez-les, et saisissez-les à nouveau.',
        host_key_changed:
          'Le serveur SFTP présente une autre clé d’hôte que celle enregistrée. Acceptez-la sous Modifier le dossier si ce changement est attendu.',
        failed:
          'Polyfin ne peut pas surveiller ce partage. Il est toujours analysé selon sa planification.',
      },
      sharesOff:
        'La surveillance des changements est désactivée dans les paramètres\u202f: les partages ne sont analysés que selon leur planification.',
      transcoderTitle: 'Transcodage',
      gpu: 'GPU',
      noGpu: 'Aucun\u202f: la vidéo est convertie par le processeur',
      toneMapping: 'HDR vers SDR sur le GPU',
      cpuEncoders: 'Encodeurs processeur',
      ffmpeg: 'FFmpeg',
      ffmpegShipped: 'Celui que Polyfin fournit',
      ffmpegOlderDrivers: 'Pour les pilotes NVIDIA antérieurs à 610',
      ffmpegOlderDownloading: 'Téléchargement de celui des pilotes NVIDIA antérieurs à 610',
      conversions: 'Conversions vidéo',
      noLimit: 'sans limite',
      remuxes: 'Remux',
      maxHeight: 'Conversion jusqu’à',
      transcodingOff: 'La conversion est désactivée dans les paramètres.',
      databaseTitle: 'Base de données',
      reachable: 'Répond',
      unreachable: 'Ne répond pas',
      size: 'Taille',
      cacheTitle: 'Cache des sources',
      cacheHelp:
        'Des morceaux des fichiers lus, gardés sur le disque dans la limite réglée dans Paramètres › Lecture, d’un démarrage à l’autre. Un cache plein est normal\u202f: les morceaux lus il y a le plus longtemps laissent la place, ceux des titres relus en dernier.',
      cacheUse: (used: string, limit: string) => `${used} sur ${limit}`,
      imagesTitle: 'Cache des images',
      imagesHelp:
        'Les illustrations relayées aux applications, leurs copies redimensionnées et les images que Polyfin dessine, gardées sur le disque jusqu’à sa limite, les moins récemment utilisées partant les premières. Le vider fait télécharger, redimensionner ou dessiner de nouveau chaque image à sa prochaine demande. Les vignettes de défilement, les images envoyées, les illustrations des dossiers locaux et le thème du lecteur web n’en font pas partie.',
      imagesFiles: 'Images sur le disque',
      imagesMemory: 'En mémoire',
      clearImages: 'Vider le cache des images',
      imagesCleared: (images: string, bytes: string) =>
        `Cache des images vidé\u202f: ${images} images, ${bytes} libérés sur le disque.`,
      sourcesOpen: 'Fichiers ouverts',
      torrentPieces: 'Dont morceaux de torrents',
      sourcesKept: 'Fichiers gardés',
      hitRatio: 'Lu depuis le disque',
      hitRatioValue: (percent: string, read: string) =>
        `${percent} des ${read} lus depuis le démarrage`,
      keptFor: (days: number) =>
        days > 0
          ? `Gardés ${days} jours après leur dernière lecture, redémarrages compris`
          : 'Vidé à chaque démarrage',
      torrentsTitle: 'Torrents',
      torrentsHelp:
        'Les torrents que la lecture lit en ce moment, par leur seule route. Un torrent s’arrête à la fin de sa lecture\u202f: plus de pairs, plus de partage.',
      torrentRoute: {
        interface: 'Interface réseau',
        proxy: 'Proxy SOCKS5',
        wireguard: 'Tunnel WireGuard',
        '': 'Aucune route',
      },
      torrentAddresses: 'Adresses du tunnel',
      torrentHandshake: 'Dernière négociation',
      torrentNoHandshake: 'Aucune pour l’instant',
      torrentsPlaying: 'Torrents en lecture',
      torrentsPlayingOf: (playing: number, atOnce: number) => `${playing} sur ${atOnce}`,
      torrentPeers: 'Pairs',
      torrentUTPPeers: 'Pairs joints en uTP',
      torrentsShared: 'Partagé depuis le démarrage',
      torrentOutageAt: (at: string) => `Coupure à ${at}`,
      torrentOutage: {
        interface: (span: string) =>
          `l’interface réseau n’a pas répondu pendant ${span}\u202f; la lecture a continué`,
        proxy: (span: string) =>
          `le proxy n’a pas répondu pendant ${span}\u202f; la lecture a continué`,
        wireguard: (span: string) =>
          `le serveur WireGuard n’a pas répondu pendant ${span}\u202f; la lecture a continué`,
        '': (span: string) =>
          `la route n’a pas répondu pendant ${span}\u202f; la lecture a continué`,
      },
      disksTitle: 'Espace disque',
      folders: {
        cache: 'du cache',
        conversion: 'de conversion',
        reading: 'des fichiers en cours de lecture',
        recordings: 'des enregistrements',
        backups: 'des sauvegardes',
      },
      folderTitle: {
        cache: 'Dossier du cache',
        conversion: 'Dossier de conversion',
        reading: 'Dossier des fichiers en cours de lecture',
        recordings: 'Dossier des enregistrements',
        backups: 'Dossier des sauvegardes',
      },
      free: (free: string) => `${free} libres`,
      notMeasured: 'Impossible à mesurer ici.',
      thumbnailsTitle: 'Vignettes (trickplay)',
      thumbnailsOff:
        'Les miniatures et images de chapitres sont désactivées dans Paramètres › Vignettes (trickplay).',
      waiting: (count: number, length: number) => `${count} sur ${length} en attente`,
      working: 'Création d’images en cours',
      idle: 'Au repos',
      pausedHosts: 'Sources en pause',
      noPausedHosts: 'Aucune source en pause.',
      until: (time: string) => `jusqu’à ${time}`,
      ratingsTitle: 'Notes depuis MDBList',
      ratingsHelp:
        'Les demandes que MDBList accorde chaque jour à la clé des notes du serveur, et celles qui restent jusqu’à sa remise à zéro quotidienne. Les titres dont les notes ne sont pas gardées sont demandés par lots, en arrière-plan.',
      requestsLeft: 'Demandes restantes aujourd’hui',
      requestsLeftOf: (left: string, limit: string) => `${left} sur ${limit}`,
      notToldYet: 'Connues à la prochaine demande à MDBList',
      resets: 'Remise à zéro quotidienne',
      ratingsHeld: 'Nouvelle demande',
      heldReasons: {
        limit: 'MDBList limite les demandes',
        refused: 'MDBList a refusé la clé',
        failed: 'MDBList n’a pas répondu',
      } as Record<string, string>,
      notHeld: 'Dès maintenant',
      titlesWaiting: 'Titres en attente',
      titlesKept: 'Titres dont les notes sont gardées',
      artworkTitle: 'Illustrations',
      artworkHelp:
        'Les téléchargements d’illustrations depuis les serveurs d’images des addons qui ont échoué dans la dernière heure. Une image en échec est redemandée plus tard, d’autant plus tard qu’elle échoue souvent\u202f; un serveur qui demande d’attendre n’est plus sollicité jusque-là.',
      artworkFailed: 'Échecs dans la dernière heure',
      artworkKinds: {
        not_found: 'Introuvables',
        rate_limited: 'Demandes de ralentir',
        timeout: 'Délais dépassés',
        server_error: 'Erreurs du serveur',
        other: 'Autres échecs',
      } as Record<string, string>,
      artworkHosts: 'Serveurs en échec',
      artworkNoFailures: 'Aucun téléchargement d’illustration n’a échoué dans la dernière heure.',
      artworkWaiting: 'Serveurs attendus',
      artworkNoWaiting: 'Aucun serveur n’est attendu.',
      artworkWaitingUntil: (time: string) => `plus sollicité jusqu’à ${time}`,
      artworkSparedUntil: (time: string) => `moins sollicité à la fois jusqu’à ${time}`,
      peopleTitle: 'Personnes',
      peopleHelp:
        'Photos et biographies des personnes depuis TMDB. Une personne est cherchée dès que la distribution d’un titre l’affiche sans photo, ou que sa page s’ouvre.',
      peopleNoKey:
        'Aucune clé d’API TMDB n’est enregistrée dans Paramètres › Contenu\u202f: les personnes et les distributions n’affichent que ce que donnent les addons. Une clé d’un compte TMDB gratuit apporte\u202f:',
      peopleNoKeyBrings: [
        'Les photos et biographies des personnes',
        'La distribution et l’équipe de chaque titre depuis TMDB, dans l’ordre du générique, avec photos',
        'Des pages de personnes listant chaque titre des bibliothèques où elles sont créditées',
        'Les titres pour lesquels TMDB connaît le mieux chaque personne, sur sa page, au-delà des bibliothèques',
        'Les affiches des films et séries dont l’addon n’en donne aucune',
        'Les coffrets dans les applications de Silo',
        'Les images de TMDB dans «\u202fModifier les images\u202f»',
      ],
      peopleNoKeyLink: 'Ajouter la clé dans Paramètres › Contenu',
      tmdbKey: 'Clé d’API TMDB',
      tmdbKeySaved: 'Enregistrée',
      peopleFound: 'Trouvées sur TMDB',
      peopleNotFound: 'Introuvables',
      peopleFailing: 'Recherches en échec',
      peopleWaiting: 'Recherches en attente',
      tmdbLimited: 'Dernière limite de TMDB',
      tmdbNeverLimited: 'Aucune depuis le démarrage du serveur',
      tmdbLimitedAt: (time: string, wait: string) =>
        wait ? `${time}, attente demandée de ${wait}` : time,
      backupTitle: 'Sauvegardes',
      backupOff: 'Les sauvegardes sont désactivées. Activez-les dans Paramètres › Sauvegardes.',
      backup: {
        result: 'Dernière exécution',
        none: 'Aucune sauvegarde pour l’instant',
        succeeded: 'Réussie',
        failed: 'Échec',
        made: 'Dernière sauvegarde faite',
        file: 'Fichier',
        size: 'Taille',
        next: 'Prochaine sauvegarde',
        runHint:
          'Lancez «\u202fSauvegarder la base de données\u202f» sur la page Planning pour sauvegarder tout de suite.',
      },
      processTitle: 'Polyfin',
      version: 'Version',
      uptime: 'En marche depuis',
      memory: 'Mémoire',
      heap: 'Utilisée',
      goroutines: 'Goroutines',
      goroutinesDownload: 'Télécharger les goroutines',
      goroutinesHelp:
        'Quand une requête ne reçoit jamais de réponse, téléchargez ce que fait le serveur et joignez-le à votre signalement. Il ne contient ni adresse ni secret.',
      lastCrash: 'Dernier plantage',
      noCrash: 'Aucun',
      lastCrashHelp:
        'Le début de la trace de l’erreur qui a arrêté Polyfin la dernière fois, lue au redémarrage. Les adresses ne gardent que leur hôte, et les jetons et clés sont masqués.',
      secretsTitle: 'Clés enregistrées',
      secretsHelp:
        'Les clés du serveur, les jetons et clés de suivi des utilisateurs, et les mots de passe des partages réseau, chiffrés dans la base de données avec POLYFIN_SECRET_KEY (openssl rand -base64 32).',
      encryption: 'Chiffrement',
      encrypted: 'Activé',
      notEncrypted: 'Désactivé\u202f: POLYFIN_SECRET_KEY n’est pas définie',
      storedPlain: 'Enregistrées sans chiffrement',
      unreadable: 'Impossibles à déchiffrer',
      noneUnreadable: 'Aucune',
      proxiesTitle: 'Proxys connus',
      proxiesHelp:
        'Les noms d’hôte listés dans Proxys connus, résolus à chaque changement des paramètres, chaque minute, et dès qu’une adresse inconnue transmet des requêtes. Chaque adresse à laquelle un nom correspond est reconnue comme proxy.',
      resolvesToNothing: 'Ne correspond à aucune adresse',
      secretNames: {
        publicMetaDbKey: 'Clé PublicMetaDB',
        theIntroDbKey: 'Clé TheIntroDB',
        mdblistRatingsKey: 'Clé d’API MDBList des notes',
        tmdbKey: 'Clé d’API TMDB',
        traktClientSecret: 'Client secret Trakt',
        lastFmSecret: 'Secret partagé Last.fm',
        aniListClientSecret: 'Client secret AniList',
        myAnimeListClientSecret: 'Client secret MyAnimeList',
        torrentProxyPassword: 'Mot de passe du proxy SOCKS5 des torrents',
        torrentWireGuard: 'Configuration WireGuard des torrents',
        smtpPassword: 'Mot de passe SMTP',
        backupPassphrase: 'Phrase de passe des sauvegardes',
        metricsToken: 'Jeton des métriques',
        updateToolToken: 'Jeton de l’outil de mise à jour',
      },
      connectionOf: (service: string, user: string) => `Connexion ${service} de ${user}`,
      targetOf: (target: string, user: string) =>
        `Cible de notification «\u202f${target}\u202f» de ${user}`,
      serverTargetOf: (target: string) =>
        `Cible de notification du serveur «\u202f${target}\u202f»`,
      sharePasswordOf: (folder: string) => `Mot de passe ou clé du partage «\u202f${folder}\u202f»`,
      destinationSecretOf: (name: string) =>
        `Mot de passe ou clé de la destination des sauvegardes «\u202f${name}\u202f»`,
      certificateKeys: 'Clés du certificat Let’s Encrypt',
      twoStepOf: (user: string) => `Clé de la connexion en deux étapes de ${user}`,
      noAddonsHint: 'Ajoutez un addon Stremio dans Contenu › Sources.',
      noIptvHint: 'Ajoutez un compte IPTV dans Contenu › Sources.',
      noGuides: 'Aucun guide des programmes.',
      noGuidesHint:
        'Ajoutez un guide des programmes à un catalogue TV dans Contenu › TV en direct.',
    },
    logs: {
      title: 'Journal',
      description:
        'Les dernières lignes du journal du serveur, gardées en mémoire. Les adresses ne gardent que leur hôte, et les mots de passe, jetons et clés sont masqués.',
      follow: 'Suivre',
      pause: 'Pause',
      level: 'Niveau',
      levels: {
        all: 'Tout',
        info: 'Infos et plus',
        warn: 'Avertissements et erreurs',
        error: 'Erreurs seulement',
      },
      search: 'Chercher',
      download: 'Télécharger',
      clear: 'Vider l’affichage',
      empty: 'Aucune ligne pour l’instant.',
      noMatch: 'Aucune ligne ne correspond.',
      shown: (shown: number, total: number) => `${shown} lignes affichées sur ${total}.`,
      following: 'Suivi en direct\u202f: les nouvelles lignes arrivent toutes les 2 secondes.',
      paused: 'En pause\u202f: les lignes écrites entre-temps s’afficheront à la reprise.',
      detailedHint:
        'Les lignes de débogage apparaissent avec le «\u202fJournal détaillé\u202f», dans Paramètres › Diagnostic.',
      region: 'Journal du serveur',
      emptyHint: 'Les lignes apparaissent ici à mesure que le serveur les écrit.',
      noMatchHint: 'Choisissez un autre niveau ou modifiez la recherche.',
      filters: 'Filtrer le journal',
    },
  },
}

export default system
