import type en from '../en/backups'

const backups: typeof en = {
  backups: {
    destinations: {
      title: 'Destinations',
      loading: 'Chargement des destinations…',
      empty: 'Aucune destination pour l’instant.',
      emptyHint:
        'Ajoutez un stockage compatible S3, un serveur SFTP ou un serveur WebDAV, pour garder les sauvegardes loin des disques de ce serveur.',
      add: 'Ajouter une destination',
      addTitle: 'Ajouter une destination',
      editTitle: (name: string) => `Modifier ${name}`,
      kind: 'Type',
      kinds: { s3: 'Stockage compatible S3', sftp: 'SFTP', webdav: 'WebDAV' },
      kindHelp: {
        s3: 'Amazon S3, Backblaze B2, Cloudflare R2, MinIO, Wasabi et les autres stockages qui parlent S3. Utilisez un compartiment dédié, ou un préfixe où seul Polyfin écrit.',
        sftp: 'Un serveur auquel Polyfin se connecte en SSH, comme un NAS. Sa clé d’hôte est enregistrée à la première connexion, puis vérifiée.',
        webdav: 'Un dossier d’un serveur WebDAV, comme Nextcloud, ownCloud ou un NAS.',
      },
      endpoint: 'Point de terminaison',
      endpointHelp:
        'L’adresse du stockage, sans le compartiment, comme https://s3.eu-west-3.amazonaws.com.',
      bucket: 'Compartiment',
      prefix: 'Préfixe de dossier',
      prefixHelp:
        'Le dossier dans le compartiment, comme polyfin. Vide pour la racine du compartiment.',
      region: 'Région',
      regionHelp:
        'Vide pour us-east-1, que la plupart des stockages acceptent. Cloudflare R2 prend auto.',
      accessKey: 'Clé d’accès',
      secretKey: 'Clé secrète',
      host: 'Hôte',
      hostHelp: 'Le nom ou l’adresse IP du serveur, sans sftp://.',
      port: 'Port',
      user: 'Utilisateur',
      password: 'Mot de passe',
      sftpPasswordHelp: 'Laissez vide quand une clé privée sert à se connecter.',
      privateKey: 'Clé privée',
      privateKeyHelp:
        'Une clé privée OpenSSH ou PEM sans phrase de passe, dont le serveur accepte la clé publique. Laissez vide pour se connecter avec le mot de passe.',
      privateKeySaved: 'Une clé est enregistrée : collez-en une autre pour la remplacer.',
      privateKeyRemove: 'Supprimer la clé enregistrée',
      sftpFolder: 'Dossier',
      sftpFolderHelp:
        'Où vont les sauvegardes, comme /volume1/backups/polyfin. Vide pour le dossier personnel de l’utilisateur.',
      webdavAddress: 'Adresse du dossier',
      webdavAddressHelp:
        'L’adresse d’un dossier existant, comme https://cloud.example.org/remote.php/dav/files/sam/polyfin/.',
      webdavPasswordHelp: 'Un mot de passe d’application, quand le serveur en propose.',
      kept: 'Sauvegardes à conserver',
      keptHelp: (min: number, max: number, byDefault: number) =>
        `Après chaque envoi, Polyfin supprime ses plus anciennes sauvegardes au-delà de ce nombre. Les autres fichiers ne sont jamais touchés. De ${min} à ${max} ; ${byDefault} par défaut.`,
      hostKey: 'Clé d’hôte',
      hostKeyNone: 'Enregistrée à la première connexion.',
      hostKeyForget: 'Oublier',
      hostKeyForgetHelp:
        'Oubliez la clé enregistrée quand la clé du serveur a changé volontairement : celle qu’il présente à la connexion suivante est enregistrée.',
      hostKeyForgotten:
        'La clé que le serveur présentera à la connexion suivante sera enregistrée.',
      create: 'Ajouter',
      creating: 'Ajout…',
      save: 'Enregistrer',
      saving: 'Enregistrement…',
      created: (name: string) => `${name} ajoutée.`,
      saved: (name: string) => `${name} enregistrée.`,
      test: 'Tester',
      testing: 'Test…',
      testHelp: 'Écrit un petit fichier, le relit et le supprime.',
      testOk: (name: string) => `${name} fonctionne.`,
      testFailed: (name: string) => `${name} ne fonctionne pas.`,
      edit: 'Modifier',
      delete: 'Supprimer',
      deleteTitle: (name: string) => `Supprimer ${name} ?`,
      deleteBody: 'Les sauvegardes n’y sont plus envoyées. Celles qui s’y trouvent restent.',
      deleted: (name: string) => `${name} supprimée.`,
      keeps: (count: number) =>
        count === 1 ? 'conserve 1 sauvegarde' : `conserve ${count} sauvegardes`,
      status: {
        waiting: 'Aucun envoi',
        working: 'Fonctionne',
        failed: 'Échec',
        stale: 'Pas à jour',
        unreadable: 'Clé illisible',
      },
      lastUpload: 'Dernier envoi',
      newest: 'Sauvegarde la plus récente',
      none: 'Aucune pour l’instant',
      unreadable:
        'Son mot de passe ou sa clé ne peut pas être déchiffré avec POLYFIN_SECRET_KEY : saisissez-le à nouveau.',
      staleHelp: 'La sauvegarde la plus récente qui s’y trouve date de plus de deux jours.',
    },
    restore: {
      button: 'Restaurer',
      title: 'Restaurer une sauvegarde',
      intro:
        'La base de données revient telle qu’elle était quand la sauvegarde a été faite. Choisissez une sauvegarde du dossier ou d’une destination, ou envoyez un fichier .dump.',
      loading: 'Liste des sauvegardes…',
      folder: 'Dossier des sauvegardes',
      none: 'Aucune sauvegarde ici.',
      beforeRestore: 'avant une restauration',
      choose: 'Restaurer cette sauvegarde',
      upload: 'Envoyer un fichier .dump',
      uploadHelp:
        'Une sauvegarde de la base de données de Polyfin, faite par Polyfin ou avec pg_dump --format=custom.',
      checking: 'Vérification de la sauvegarde…',
      unavailable: {
        pg_restore_missing:
          'pg_restore est introuvable sur le serveur : installez les outils clients de PostgreSQL 18, que l’image Docker contient.',
      },
      confirmTitle: (file: string) => `Restaurer ${file} ?`,
      made: (date: string) => `Faite le ${date}.`,
      from: (name: string) => `Depuis ${name}.`,
      uploaded: 'Envoyée depuis cet ordinateur.',
      steps: [
        'Polyfin sauvegarde d’abord la base de données actuelle, dans le dossier des sauvegardes, en une sauvegarde d’avant restauration qu’il ne supprime jamais.',
        'Les applis reçoivent une erreur pendant la restauration, et les lectures, conversions et enregistrements en cours s’arrêtent.',
        'Polyfin redémarre ensuite sur la base restaurée : tout est comme au moment de la sauvegarde, utilisateurs, paramètres et historique compris.',
        'Les applis et les administrateurs connectés depuis doivent se reconnecter, vous compris, et les liens de lecture déjà donnés ne fonctionnent plus.',
      ],
      keyTitle: 'Des clés enregistrées sont illisibles',
      keyBody:
        'Cette sauvegarde contient des clés et des mots de passe que la POLYFIN_SECRET_KEY actuelle ne peut pas déchiffrer. Définissez la clé en place quand la sauvegarde a été faite, puis restaurez à nouveau ; ou restaurez quand même, et saisissez-les à nouveau ensuite :',
      kinds: {
        settings: (n: number) => (n === 1 ? '1 clé des paramètres' : `${n} clés des paramètres`),
        tracking: (n: number) => (n === 1 ? '1 connexion de suivi' : `${n} connexions de suivi`),
        apps: (n: number) => (n === 1 ? '1 connexion d’appli' : `${n} connexions d’applis`),
        notifications: (n: number) =>
          n === 1 ? '1 cible de notification' : `${n} cibles de notification`,
        shares: (n: number) =>
          n === 1 ? '1 mot de passe de partage' : `${n} mots de passe de partage`,
        destinations: (n: number) =>
          n === 1 ? '1 destination des sauvegardes' : `${n} destinations des sauvegardes`,
        certificates: (n: number) =>
          n === 1 ? '1 clé du certificat Let’s Encrypt' : `${n} clés du certificat Let’s Encrypt`,
      },
      serverName: (name: string) => `Saisissez le nom du serveur, ${name}, pour confirmer`,
      confirm: 'Restaurer',
      back: 'Autres sauvegardes',
      anyway: 'Restaurer quand même',
      starting: 'Sauvegarde de la base actuelle…',
      restoringTitle: 'Restauration',
      restoringBody:
        'Polyfin restaure la sauvegarde, puis redémarre. Cette page se recharge dès qu’il répond.',
      lastRestore: 'Dernière restauration',
      lastRestoreOk: (file: string, by: string) => `${file}, par ${by}`,
      lastRestoreFailed: (file: string) =>
        `${file} n’a pas pu être restaurée : la base de données est restée telle quelle.`,
    },
  },
}

export default backups
