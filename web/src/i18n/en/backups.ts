/** Backup destinations (Settings › Backups) and restoring a backup (System › Schedule). */
const backups = {
  backups: {
    destinations: {
      title: 'Destinations',
      loading: 'Loading the destinations…',
      empty: 'No destination yet.',
      emptyHint:
        'Add S3-compatible storage, an SFTP server or a WebDAV server, to keep backups away from this server’s disks.',
      add: 'Add a destination',
      addTitle: 'Add a destination',
      editTitle: (name: string) => `Edit ${name}`,
      kind: 'Kind',
      kinds: { s3: 'S3-compatible storage', sftp: 'SFTP', webdav: 'WebDAV' },
      kindHelp: {
        s3: 'Amazon S3, Backblaze B2, Cloudflare R2, MinIO, Wasabi and other storage that speaks S3. Use a bucket of its own, or a prefix only Polyfin writes to.',
        sftp: 'A server Polyfin signs in to over SSH, such as a NAS. Its host key is saved at the first connection, and checked afterwards.',
        webdav: 'A folder of a WebDAV server, such as Nextcloud, ownCloud or a NAS.',
      },
      endpoint: 'Endpoint',
      endpointHelp:
        'The storage’s address, without the bucket, such as https://s3.eu-west-3.amazonaws.com.',
      bucket: 'Bucket',
      prefix: 'Folder prefix',
      prefixHelp: 'The folder within the bucket, such as polyfin. Empty for the bucket’s root.',
      region: 'Region',
      regionHelp: 'Empty for us-east-1, which most storages accept. Cloudflare R2 takes auto.',
      accessKey: 'Access key',
      secretKey: 'Secret key',
      host: 'Host',
      hostHelp: 'The server’s name or IP address, without sftp://.',
      port: 'Port',
      user: 'User',
      password: 'Password',
      sftpPasswordHelp: 'Leave empty when a private key signs in.',
      privateKey: 'Private key',
      privateKeyHelp:
        'An OpenSSH or PEM private key without a passphrase, whose public key the server accepts. Leave empty to sign in with the password.',
      privateKeySaved: 'A key is saved: paste another one to replace it.',
      privateKeyRemove: 'Remove the saved key',
      sftpFolder: 'Folder',
      sftpFolderHelp:
        'Where backups go, such as /volume1/backups/polyfin. Empty for the user’s home.',
      webdavAddress: 'Folder address',
      webdavAddressHelp:
        'The address of an existing folder, such as https://cloud.example.org/remote.php/dav/files/sam/polyfin/.',
      webdavPasswordHelp: 'An app password, when the server offers them.',
      kept: 'Backups to keep',
      keptHelp: (min: number, max: number, byDefault: number) =>
        `After each upload, Polyfin deletes its oldest backups there past this number. Other files are never touched. From ${min} to ${max}; ${byDefault} by default.`,
      hostKey: 'Host key',
      hostKeyNone: 'Saved at the first connection.',
      hostKeyForget: 'Forget',
      hostKeyForgetHelp:
        'Forget the saved key when the server’s key changed on purpose: the key it offers at the next connection is saved.',
      hostKeyForgotten: 'The key the server offers at the next connection will be saved.',
      create: 'Add',
      creating: 'Adding…',
      save: 'Save',
      saving: 'Saving…',
      created: (name: string) => `${name} added.`,
      saved: (name: string) => `${name} saved.`,
      test: 'Test',
      testing: 'Testing…',
      testHelp: 'Writes a small file, reads it back and deletes it.',
      testOk: (name: string) => `${name} works.`,
      testFailed: (name: string) => `${name} does not work.`,
      edit: 'Edit',
      delete: 'Delete',
      deleteTitle: (name: string) => `Delete ${name}?`,
      deleteBody: 'Backups are no longer sent there. Those already there stay.',
      deleted: (name: string) => `${name} deleted.`,
      keeps: (count: number) => (count === 1 ? 'keeps 1 backup' : `keeps ${count} backups`),
      status: {
        waiting: 'No upload yet',
        working: 'Working',
        failed: 'Failed',
        stale: 'Out of date',
        unreadable: 'Key unreadable',
      },
      lastUpload: 'Last upload',
      newest: 'Newest backup there',
      none: 'None yet',
      unreadable:
        'Its password or key cannot be decrypted with POLYFIN_SECRET_KEY: enter it again.',
      staleHelp: 'The newest backup there is more than two days old.',
    },
    restore: {
      button: 'Restore',
      title: 'Restore a backup',
      intro:
        'The database comes back as it was when the backup was made. Choose a backup of the folder or of a destination, or send a backup file.',
      loading: 'Listing the backups…',
      folder: 'Backups folder',
      none: 'No backup here.',
      beforeRestore: 'before a restore',
      encrypted: 'encrypted',
      choose: 'Restore this backup',
      download: 'Download',
      downloadDecrypted: 'Download decrypted',
      downloadTitle: (file: string) => `Download ${file} decrypted`,
      downloadBody:
        'Type the passphrase this backup was encrypted with. The dump is saved unencrypted on this computer: keep it as safe as the server.',
      passphrase: 'Backup passphrase',
      downloading: 'Checking the passphrase…',
      upload: 'Send a backup file',
      uploadHelp:
        'A .dump file of Polyfin’s database, made by Polyfin or with pg_dump --format=custom, or a .dump.enc file Polyfin encrypted.',
      unlockTitle: (file: string) => `Open ${file}`,
      unlockBody:
        'This backup is encrypted. Type the passphrase it was made with: Polyfin decrypts it, checks every part of it, then tells what restoring it does.',
      unlock: 'Open',
      unlocking: 'Decrypting and checking…',
      checking: 'Checking the backup…',
      unavailable: {
        pg_restore_missing:
          'pg_restore was not found on the server: install PostgreSQL 18’s client tools, which the Docker image includes.',
      },
      confirmTitle: (file: string) => `Restore ${file}?`,
      made: (date: string) => `Made ${date}.`,
      from: (name: string) => `From ${name}.`,
      uploaded: 'Sent from this computer.',
      steps: [
        'Polyfin backs the current database up first, in the backups folder, as a before-restore backup it never deletes.',
        'Apps get an error while the restore runs, and playbacks, conversions and recordings in progress stop.',
        'Polyfin then starts again on the restored database: everything is as it was when the backup was made, users, settings and watch history included.',
        'Apps and administrators signed in since then must sign in again, you included, and stream links handed out before stop working.',
      ],
      keyTitle: 'Some stored keys cannot be read',
      keyBody:
        'This backup holds keys and passwords that the current POLYFIN_SECRET_KEY cannot decrypt. Set the key that was set when the backup was made, then restore again; or restore anyway, and enter these again afterwards:',
      kinds: {
        settings: (n: number) => (n === 1 ? '1 key of the settings' : `${n} keys of the settings`),
        tracking: (n: number) => (n === 1 ? '1 tracking connection' : `${n} tracking connections`),
        apps: (n: number) => (n === 1 ? '1 app connection' : `${n} app connections`),
        notifications: (n: number) =>
          n === 1 ? '1 notification target' : `${n} notification targets`,
        shares: (n: number) => (n === 1 ? '1 share password' : `${n} share passwords`),
        destinations: (n: number) =>
          n === 1 ? '1 backup destination' : `${n} backup destinations`,
        certificates: (n: number) =>
          n === 1
            ? '1 key of the Let’s Encrypt certificate'
            : `${n} keys of the Let’s Encrypt certificate`,
        twoStep: (n: number) =>
          n === 1 ? '1 key of a two-step sign-in' : `${n} keys of two-step sign-ins`,
      },
      serverName: (name: string) => `Type the server’s name, ${name}, to confirm`,
      confirm: 'Restore',
      back: 'Other backups',
      anyway: 'Restore anyway',
      starting: 'Backing up the current database…',
      restoringTitle: 'Restoring',
      restoringBody:
        'Polyfin is restoring the backup, then starts again. This page reloads once it answers.',
      lastRestore: 'Last restore',
      lastRestoreOk: (file: string, by: string) => `${file}, by ${by}`,
      lastRestoreSetup: (file: string) => `${file}, from the setup page`,
      lastRestoreFailed: (file: string) =>
        `${file} could not be restored: the database is as it was.`,
    },
  },
}

export default backups
