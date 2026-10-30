import type en from '../en/backups'

const backups: typeof en = {
  backups: {
    destinations: {
      title: 'Destinos',
      loading: 'Cargando los destinos…',
      empty: 'Aún no hay ningún destino.',
      emptyHint:
        'Añada un almacenamiento compatible con S3, un servidor SFTP o un servidor WebDAV para guardar las copias de seguridad fuera de los discos de este servidor.',
      add: 'Añadir un destino',
      addTitle: 'Añadir un destino',
      editTitle: (name: string) => `Editar ${name}`,
      kind: 'Tipo',
      kinds: { s3: 'Almacenamiento compatible con S3', sftp: 'SFTP', webdav: 'WebDAV' },
      kindHelp: {
        s3: 'Amazon S3, Backblaze B2, Cloudflare R2, MinIO, Wasabi y otros almacenamientos compatibles con S3. Use un bucket exclusivo o un prefijo en el que solo escriba Polyfin.',
        sftp: 'Un servidor en el que Polyfin inicia sesión por SSH, como un NAS. Su clave de host se guarda en la primera conexión y se comprueba en las siguientes.',
        webdav: 'Una carpeta de un servidor WebDAV, como Nextcloud, ownCloud o un NAS.',
      },
      endpoint: 'Endpoint',
      endpointHelp:
        'La dirección del almacenamiento, sin el bucket, como https://s3.eu-west-3.amazonaws.com.',
      bucket: 'Bucket',
      prefix: 'Prefijo de carpeta',
      prefixHelp: 'La carpeta dentro del bucket, como polyfin. Vacío para la raíz del bucket.',
      region: 'Región',
      regionHelp:
        'Vacío para us-east-1, que acepta la mayoría de los almacenamientos. Cloudflare R2 usa auto.',
      accessKey: 'Clave de acceso',
      secretKey: 'Clave secreta',
      host: 'Host',
      hostHelp: 'El nombre o la dirección IP del servidor, sin sftp://.',
      port: 'Puerto',
      user: 'Usuario',
      password: 'Contraseña',
      sftpPasswordHelp: 'Déjelo vacío si se inicia sesión con una clave privada.',
      privateKey: 'Clave privada',
      privateKeyHelp:
        'Una clave privada OpenSSH o PEM sin frase de contraseña, cuya clave pública acepte el servidor. Déjelo vacío para iniciar sesión con la contraseña.',
      privateKeySaved: 'Hay una clave guardada: pegue otra para reemplazarla.',
      privateKeyRemove: 'Eliminar la clave guardada',
      sftpFolder: 'Carpeta',
      sftpFolderHelp:
        'Dónde se guardan las copias de seguridad, como /volume1/backups/polyfin. Vacío para la carpeta personal del usuario.',
      webdavAddress: 'Dirección de la carpeta',
      webdavAddressHelp:
        'La dirección de una carpeta existente, como https://cloud.example.org/remote.php/dav/files/sam/polyfin/.',
      webdavPasswordHelp: 'Una contraseña de aplicación, si el servidor las ofrece.',
      kept: 'Copias de seguridad que conservar',
      keptHelp: (min: number, max: number, byDefault: number) =>
        `Tras cada subida, Polyfin borra allí sus copias de seguridad más antiguas que superen este número. Los demás archivos nunca se tocan. De ${min} a ${max}; ${byDefault} de forma predeterminada.`,
      hostKey: 'Clave de host',
      hostKeyNone: 'Se guarda en la primera conexión.',
      hostKeyForget: 'Olvidar',
      hostKeyForgetHelp:
        'Olvide la clave guardada si la clave del servidor ha cambiado intencionadamente: se guardará la que ofrezca en la próxima conexión.',
      hostKeyForgotten: 'Se guardará la clave que ofrezca el servidor en la próxima conexión.',
      create: 'Añadir',
      creating: 'Añadiendo…',
      save: 'Guardar',
      saving: 'Guardando…',
      created: (name: string) => `${name} añadido.`,
      saved: (name: string) => `${name} guardado.`,
      test: 'Probar',
      testing: 'Probando…',
      testHelp: 'Escribe un archivo pequeño, lo vuelve a leer y lo borra.',
      testOk: (name: string) => `${name} funciona.`,
      testFailed: (name: string) => `${name} no funciona.`,
      edit: 'Editar',
      delete: 'Borrar',
      deleteTitle: (name: string) => `¿Borrar ${name}?`,
      deleteBody: 'Ya no se enviarán copias de seguridad allí. Las que ya están allí se conservan.',
      deleted: (name: string) => `${name} borrado.`,
      keeps: (count: number) =>
        count === 1 ? 'conserva 1 copia de seguridad' : `conserva ${count} copias de seguridad`,
      status: {
        waiting: 'Aún sin subidas',
        working: 'Funciona',
        failed: 'Error',
        stale: 'Desactualizado',
        unreadable: 'Clave ilegible',
      },
      lastUpload: 'Última subida',
      newest: 'Copia de seguridad más reciente allí',
      none: 'Ninguna todavía',
      unreadable:
        'Su contraseña o clave no se puede descifrar con POLYFIN_SECRET_KEY: introdúzcala de nuevo.',
      staleHelp: 'La copia de seguridad más reciente allí tiene más de dos días.',
    },
    restore: {
      button: 'Restaurar',
      title: 'Restaurar una copia de seguridad',
      intro:
        'La base de datos vuelve al estado en que estaba cuando se hizo la copia de seguridad. Elija una copia de seguridad de la carpeta o de un destino, o envíe un archivo de copia de seguridad.',
      loading: 'Obteniendo la lista de copias de seguridad…',
      folder: 'Carpeta de copias de seguridad',
      none: 'No hay ninguna copia de seguridad aquí.',
      beforeRestore: 'antes de una restauración',
      encrypted: 'cifrada',
      choose: 'Restaurar esta copia de seguridad',
      download: 'Descargar',
      downloadDecrypted: 'Descargar descifrada',
      downloadTitle: (file: string) => `Descargar ${file} descifrada`,
      downloadBody:
        'Escriba la frase de contraseña con la que se cifró esta copia de seguridad. El volcado se guarda sin cifrar en este equipo: guárdelo con el mismo cuidado que el servidor.',
      passphrase: 'Frase de contraseña de las copias de seguridad',
      downloading: 'Comprobando la frase de contraseña…',
      upload: 'Enviar un archivo de copia de seguridad',
      uploadHelp:
        'Un archivo .dump de la base de datos de Polyfin, hecho por Polyfin o con pg_dump --format=custom, o un archivo .dump.enc cifrado por Polyfin.',
      unlockTitle: (file: string) => `Abrir ${file}`,
      unlockBody:
        'Esta copia de seguridad está cifrada. Escriba la frase de contraseña con la que se hizo: Polyfin la descifra, comprueba cada parte y luego indica lo que hace su restauración.',
      unlock: 'Abrir',
      unlocking: 'Descifrando y comprobando…',
      checking: 'Comprobando la copia de seguridad…',
      unavailable: {
        pg_restore_missing:
          'No se ha encontrado pg_restore en el servidor: instale las herramientas cliente de PostgreSQL 18, que incluye la imagen de Docker.',
      },
      confirmTitle: (file: string) => `¿Restaurar ${file}?`,
      made: (date: string) => `Hecha el ${date}.`,
      from: (name: string) => `Desde ${name}.`,
      uploaded: 'Enviada desde este equipo.',
      steps: [
        'Primero, Polyfin hace una copia de seguridad de la base de datos actual en la carpeta de copias de seguridad, como copia previa a la restauración que nunca borra.',
        'Las aplicaciones reciben un error mientras dura la restauración, y se detienen las reproducciones, conversiones y grabaciones en curso.',
        'Después, Polyfin se reinicia con la base de datos restaurada: todo queda como estaba cuando se hizo la copia de seguridad, incluidos usuarios, ajustes e historial de reproducción.',
        'Las aplicaciones y los administradores que hayan iniciado sesión desde entonces deben volver a iniciarla, usted incluido, y los enlaces de transmisión entregados antes dejan de funcionar.',
      ],
      keyTitle: 'No se pueden leer algunas claves guardadas',
      keyBody:
        'Esta copia de seguridad contiene claves y contraseñas que la POLYFIN_SECRET_KEY actual no puede descifrar. Establezca la clave que estaba configurada cuando se hizo la copia de seguridad y vuelva a restaurar; o restaure de todos modos y vuelva a introducirlas después:',
      kinds: {
        settings: (n: number) =>
          n === 1 ? '1 clave de los ajustes' : `${n} claves de los ajustes`,
        tracking: (n: number) =>
          n === 1 ? '1 conexión de seguimiento' : `${n} conexiones de seguimiento`,
        apps: (n: number) =>
          n === 1 ? '1 conexión de aplicación' : `${n} conexiones de aplicaciones`,
        notifications: (n: number) =>
          n === 1 ? '1 destino de notificaciones' : `${n} destinos de notificaciones`,
        shares: (n: number) =>
          n === 1
            ? '1 contraseña de enlace para compartir'
            : `${n} contraseñas de enlaces para compartir`,
        destinations: (n: number) =>
          n === 1
            ? '1 destino de las copias de seguridad'
            : `${n} destinos de las copias de seguridad`,
        certificates: (n: number) =>
          n === 1
            ? '1 clave del certificado de Let’s Encrypt'
            : `${n} claves del certificado de Let’s Encrypt`,
        twoStep: (n: number) =>
          n === 1
            ? '1 clave de un inicio de sesión en dos pasos'
            : `${n} claves de inicios de sesión en dos pasos`,
      },
      serverName: (name: string) => `Escriba el nombre del servidor, ${name}, para confirmar`,
      confirm: 'Restaurar',
      back: 'Otras copias de seguridad',
      anyway: 'Restaurar de todos modos',
      starting: 'Haciendo una copia de seguridad de la base de datos actual…',
      restoringTitle: 'Restaurando',
      restoringBody:
        'Polyfin está restaurando la copia de seguridad y después se reiniciará. Esta página se recargará en cuanto responda.',
      lastRestore: 'Última restauración',
      lastRestoreOk: (file: string, by: string) => `${file}, por ${by}`,
      lastRestoreSetup: (file: string) => `${file}, desde la página de configuración`,
      lastRestoreFailed: (file: string) =>
        `No se pudo restaurar ${file}: la base de datos sigue como estaba.`,
    },
  },
}

export default backups
