import type en from '../en/system'

const system: typeof en = {
  apiKeys: {
    title: 'Claves API',
    description:
      'Las claves permiten que herramientas y aplicaciones, como gestores de solicitudes o scripts, usen la API de Jellyfin de este servidor con permisos de administrador. Cualquier persona que tenga una clave tiene estos permisos: mantenga las claves en privado y revoque las que ya no use.',
    listTitle: 'Claves',
    empty: 'Todavía no hay ninguna clave API.',
    created: 'Creada',
    lastUsed: 'Último uso',
    createTitle: 'Crear una clave',
    app: 'Nombre de la aplicación',
    appHint: 'Quién usará esta clave, para reconocerla más adelante. De 1 a 64 caracteres.',
    create: 'Crear clave',
    creating: 'Creando…',
    newKeyTitle: (app: string) => `Clave para ${app}`,
    newKeyNotice: 'Copie esta clave ahora: no se volverá a mostrar.',
    copy: 'Copiar',
    copied: 'Copiada.',
    copyFailed: 'No se pudo copiar la clave. Selecciónela y cópiela manualmente.',
    done: 'Ya la he copiado',
    revoke: 'Revocar',
    revoking: 'Revocando…',
    revokeConfirm: (app: string) =>
      `¿Revocar la clave para ${app}? Las herramientas y aplicaciones que la usen dejarán de funcionar. Esta acción no se puede deshacer.`,
    revoked: (app: string) => `Se ha revocado la clave para ${app}.`,
    emptyHint:
      'Cree una a continuación para cada herramienta o aplicación, de modo que cada una pueda revocarse por separado.',
    revokeTitle: '¿Revocar esta clave?',
    revokeLabel: (app: string) => `Revocar la clave para ${app}`,
  },
  system: {
    schedule: {
      title: 'Programación',
      description:
        'Las tareas del servidor, las próximas grabaciones y las próximas actualizaciones de Televisión en vivo.',
      tasksTitle: 'Tareas',
      task: 'Tarea',
      lastRun: 'Última ejecución',
      nextRun: 'Próxima ejecución',
      notRunYet: 'No se ha ejecutado desde que se inició el servidor',
      byHandOnly: 'Solo ejecución manual',
      every: (span: string) => `Intervalo: ${span}`,
      daily: (time: string) => `Todos los días a las ${time}, hora del servidor`,
      took: (span: string) => `duró ${span}`,
      results: {
        Completed: 'Completada',
        Failed: 'Fallida',
        Cancelled: 'Detenida',
      } as Record<string, string>,
      running: 'En ejecución',
      cancelling: 'Deteniendo',
      runNow: 'Ejecutar ahora',
      runLabel: (name: string) => `Ejecutar ${name} ahora`,
      stop: 'Detener',
      stopLabel: (name: string) => `Detener ${name}`,
      started: (name: string) => `Se ha iniciado ${name}.`,
      autoRefresh: 'Se actualiza cada 5 segundos.',
      recordingsTitle: 'Próximas grabaciones',
      recordingsOff: 'La grabación está desactivada. Actívela en Ajustes › Grabaciones.',
      noTimers: 'No hay ninguna grabación programada.',
      recordingNow: 'Grabando',
      failed: 'Fallida',
      series: 'Serie',
      scheduledBy: (name: string) => `Programada por ${name}`,
      unknownChannel: 'No se encontró el canal',
      tunersShort: 'No hay suficientes sintonizadores',
      tunersShortHelp:
        'Hay más grabaciones simultáneas en los canales de este sintonizador que sintonizadores disponibles: una de ellas podría fallar.',
      refreshesTitle: 'Actualizaciones de Televisión en vivo',
      refreshesHelp: (hours: number) =>
        `Las listas de canales IPTV y las guías de programación, tanto las del servidor como las de los usuarios, se vuelven a descargar cada ${hours} h (Ajustes › Televisión en vivo) mediante la tarea «Actualizar guías de Televisión en vivo».`,
      noRefreshes: 'No hay ninguna fuente IPTV ni guía de programación.',
      channelList: 'Lista de canales',
      iptvLists: 'Listas de canales, películas y series',
      guide: 'Guía de programación',
      due: 'En la próxima comprobación',
      lastDownload: 'Última descarga',
      nextDownload: 'Próxima descarga',
      notYet: 'Todavía no',
      noTasks: 'No hay tareas.',
      noTasksHint: 'El servidor muestra aquí sus tareas una vez iniciado.',
      recordingsHint:
        'Las grabaciones programadas desde una aplicación o desde la guía aparecen aquí.',
      refreshesHint:
        'Añada una fuente IPTV en Contenido › Fuentes, o una guía de programación a un catálogo de TV, y sus descargas aparecerán aquí.',
      backupsTitle: 'Copias de seguridad de la base de datos',
      backUpNow: 'Hacer copia de seguridad ahora',
    },
    health: {
      title: 'Estado del sistema',
      noIptv: 'No hay ninguna fuente IPTV.',
      owner: { server: 'Servidor', user: (name: string) => `Extensiones de ${name}` },
      description:
        'Cómo funcionan el servidor y sus fuentes, según lo que Polyfin ya registra: esta página no les envía ninguna solicitud.',
      problemsTitle: 'Requiere atención',
      allGood: 'No se ha encontrado ningún problema.',
      update: (version: string) => `Polyfin ${version} está disponible`,
      updateRunning: (version: string) => `Este servidor ejecuta la versión ${version}.`,
      releaseNotes: 'Notas de la versión',
      checkedAt: (time: string) => `Comprobado a las ${time}. Se actualiza cada 10 segundos.`,
      ok: 'Correcto',
      warning: 'Advertencia',
      error: 'Error',
      problems: {
        database: 'La base de datos no responde.',
        disk: (folder: string, free: string) =>
          `Queda poco espacio para la carpeta ${folder}: ${free} libres.`,
        addon: (name: string, failure: string) => `${name}: ${failure}`,
        iptv: (name: string) => `${name}: ha fallado la última descarga de la lista de canales.`,
        tuner: (name: string) => `${name}: el sintonizador no ha podido reproducir un canal.`,
        tunerUnreachable: (name: string) => `${name}: el sintonizador no responde.`,
        folder: (name: string) =>
          `${name}: Polyfin no puede leer esta carpeta local. Compruebe que está montada y que el usuario 65532 puede leerla.`,
        share: (name: string) =>
          `${name}: Polyfin no puede leer este recurso compartido de red. Compruebe su dirección y que su usuario tiene permiso de lectura.`,
        shareUnreachable: (name: string) =>
          `${name}: Polyfin no puede acceder a este recurso compartido de red.`,
        shareRefused: (name: string) =>
          `${name}: el recurso compartido de red rechaza el usuario o la contraseña de Polyfin.`,
        shareHostKey: (name: string) =>
          `${name}: el servidor SFTP presenta una clave de host distinta de la guardada, y Polyfin la rechaza hasta que se acepte.`,
        folderWatchLimit: (name: string) =>
          `${name}: Polyfin no puede vigilar los cambios de esta carpeta local: se ha alcanzado el límite de carpetas vigiladas del sistema. Se sigue escaneando según su programación. Aumente fs.inotify.max_user_watches en el host.`,
        folderInstanceLimit: (name: string) =>
          `${name}: Polyfin no puede vigilar los cambios de esta carpeta local: se ha alcanzado el límite de observadores o de archivos abiertos del sistema. Se sigue escaneando según su programación. Aumente fs.inotify.max_user_instances en el host, o el límite de archivos abiertos del contenedor.`,
        folderOpenFilesLimit: (name: string) =>
          `${name}: Polyfin no puede vigilar los cambios de esta carpeta local: se ha alcanzado el límite de archivos abiertos del sistema, ya que macOS vigila cada carpeta y archivo mediante uno. Se sigue escaneando según su programación. Aumente kern.maxfilesperproc y kern.maxfiles con sysctl.`,
        folderUnwatched: (name: string) =>
          `${name}: Polyfin no puede vigilar los cambios de esta carpeta local. Se sigue escaneando según su programación.`,
        shareUnwatchedUnreachable: (name: string) =>
          `${name}: Polyfin no puede vigilar los cambios de este recurso compartido de red: no se puede acceder a él. Se escanea según su programación, y de nuevo en cuanto responda.`,
        shareUnwatchedRefused: (name: string) =>
          `${name}: Polyfin no puede vigilar los cambios de este recurso compartido de red: rechaza el usuario o la contraseña de Polyfin. Compruébelos e introdúzcalos de nuevo.`,
        shareUnwatched: (name: string) =>
          `${name}: Polyfin no puede vigilar los cambios de este recurso compartido de red. Se sigue escaneando según su programación.`,
        folderNoSearch: (name: string) =>
          `${name}: ninguna extensión del servidor describe ni busca los títulos de sus archivos, así que siguen sin asociar. Añada una en Contenido › Fuentes: las extensiones propias de los usuarios, en Mis fuentes, no cuentan.`,
        folderSearch: (name: string, addon: string, failure: string) =>
          `${name}: no se pudieron buscar los títulos de sus archivos${addon === '' ? '' : failure === '' ? ` (${addon})` : ` (${addon}: ${failure})`}. Siguen sin asociar hasta que una búsqueda salga bien, lo que Polyfin vuelve a intentar por sí solo, al cabo de unos minutos y luego con menos frecuencia.`,
        searchLate: (name: string) =>
          `${name}: su búsqueda no respondió a tiempo. Las búsquedas lo dejan fuera unos minutos y luego vuelven a consultarlo.`,
        guide: (name: string) =>
          `${name}: ha fallado la última obtención de la guía de programación.`,
        paused: (host: string) =>
          `Miniaturas en pausa para ${host}: ha pedido reducir la velocidad.`,
        unknownProxy: (address: string) =>
          `${address} reenvía solicitudes (X-Forwarded-For) pero no es un proxy conocido, así que todos sus clientes cuentan como ${address}. Si es su proxy inverso, añádalo a «Proxies conocidos» en Ajustes › Seguridad.`,
        discoveryPort: (port: number) =>
          `Polyfin no puede escuchar en el puerto UDP ${port}, así que las aplicaciones de Jellyfin de la red local no lo encuentran por sí solas. Probablemente otro programa lo está usando, como un servidor Jellyfin en el mismo host. Al guardar los ajustes se vuelve a intentar.`,
        torrentRoute: (why: string) =>
          `Las transmisiones torrent no se reproducen: ${why}. No se envía nada por otra vía; compruebe la ruta en Ajustes › Torrents.`,
        torrentWhy: {
          interface_missing: (name: string) => `no se encuentra la interfaz de red ${name}`,
          interface_down: (name: string) => `la interfaz de red ${name} está desactivada`,
          interface_no_address: (name: string) => `la interfaz de red ${name} no tiene dirección`,
          interface_unsupported: () => 'las interfaces de red solo funcionan en Linux',
          proxy_unreachable: (name: string) => `el proxy SOCKS5 ${name} no responde`,
          proxy_refused: (name: string) =>
            `el proxy SOCKS5 ${name} ha rechazado el usuario y la contraseña`,
          wireguard_endpoint: (name: string) => `no se encuentra el servidor WireGuard ${name}`,
          wireguard_down: (name: string) => `el túnel WireGuard hacia ${name} no puede iniciarse`,
          wireguard_handshake: (name: string) =>
            `el servidor WireGuard ${name} no responde a la negociación: no se puede contactar o una clave de la configuración es incorrecta`,
          no_route: () => 'no hay ninguna ruta definida',
        },
        torrentUdp: (name: string) =>
          `El proxy SOCKS5 ${name} no ofrece UDP ASSOCIATE: los torrents se reproducen por TCP, sin trackers UDP ni DHT.`,
        torrentStart: (why: string) =>
          `Un torrent no se inició a tiempo: ${why}. Compruebe que la ruta llega a Internet; el registro indica la respuesta de cada tracker en el nivel de depuración.`,
        torrentStartWhy: {
          no_udp:
            'no se encontró ningún par y la ruta no transporta UDP, que necesitan la DHT y los trackers UDP',
          refused:
            'sus trackers rechazaron el anuncio o no respondieron, y la DHT no encontró ningún par',
          no_peers: 'sus trackers y la DHT no encontraron ningún par',
          silent: 'se encontraron pares, pero ninguno lo envió',
        },
        torrentStartPeers: (found: number, why: string) =>
          `${found} ${found === 1 ? 'par encontrado' : 'pares encontrados'}: ${why}`,
        torrentPeerWhy: {
          connected: (n: number) => `${n} ${n === 1 ? 'conectado' : 'conectados'}`,
          timedOut: (n: number) => `${n} sin respuesta a tiempo`,
          refused: (n: number) => `${n} ${n === 1 ? 'rechazó' : 'rechazaron'} la conexión`,
          unreachable: (n: number) => `${n} ${n === 1 ? 'inaccesible' : 'inaccesibles'}`,
          proxy: (n: number) => `${n} ${n === 1 ? 'rechazado' : 'rechazados'} por el proxy`,
          closed: (n: number) => `${n} ${n === 1 ? 'cerró' : 'cerraron'} la conexión en el saludo`,
          left: (n: number) =>
            `${n} ${n === 1 ? 'conectado y luego desconectado' : 'conectados y luego desconectados'}`,
          other: (n: number) => `${n} ${n === 1 ? 'falló' : 'fallaron'} por otro motivo`,
          untried: (n: number) => `${n} aún sin contactar`,
        },
        dlnaPort: (port: number) =>
          `Polyfin no puede escuchar en el puerto UDP ${port}, así que los dispositivos DLNA de la red local no lo encuentran por sí solos. Probablemente otro programa lo está usando, como otro servidor DLNA en el mismo host. Al guardar los ajustes se vuelve a intentar.`,
        discoveryAddress: (address: string) =>
          `Las aplicaciones de Jellyfin que buscan servidores en la red local reciben ${address}, una dirección del contenedor de Polyfin a la que no pueden acceder. Defina «Dirección en la red local» en Ajustes › General.`,
        proxyNameUnresolved: (name: string) =>
          `El proxy conocido ${name} no se resuelve en ninguna dirección, así que no se confía en que ninguna solicitud proceda de él. Compruebe su nombre en Ajustes › Seguridad y que Polyfin comparte una red con él.`,
        task: (name: string) => `Ha fallado la tarea «${name}».`,
        httpsPort: (port: number) =>
          `Polyfin no puede escuchar en el puerto TCP ${port}, así que no se sirve HTTPS. HTTP sigue funcionando. Probablemente otro programa lo está usando, o es inferior a 1024. Elija otro puerto HTTPS en Ajustes › Seguridad.`,
        httpsFileMissing: (file: string) =>
          `El archivo de certificado HTTPS ${file} no existe. Se sigue usando el último certificado leído.`,
        httpsFilesMismatch:
          'La clave privada HTTPS no corresponde al certificado. Se sigue usando el último certificado leído.',
        httpsFileUnreadable: (file: string) =>
          `Polyfin no puede leer el archivo de certificado HTTPS ${file}, o no contiene ningún certificado ni clave PEM que pueda leer. Se sigue usando el último certificado leído.`,
        httpsRenewal: (error: string) =>
          `Let’s Encrypt no ha emitido el certificado HTTPS: ${error}`,
        httpsExpires: (date: string) => `El certificado HTTPS caduca el ${date}.`,
        httpsExpired: (date: string) => `El certificado HTTPS caducó el ${date}.`,
        subtitleOcrLanguage: (name: string, file: string) =>
          `Los subtítulos en imagen en ${name} no se convierten a texto: Tesseract no tiene datos para este idioma. Añada el archivo ${file} a la carpeta que indica POLYFIN_TESSDATA_DIR.`,
        metadataLanguages: (name: string) =>
          `${name}: todas las extensiones de metadatos del servidor tienen un idioma, así que describen los títulos en su idioma para todos los usuarios, sea cual sea su idioma de los metadatos. Deje «Idioma de sus descripciones» en «Ninguno» en esta extensión o en otra extensión de metadatos más.`,
        themeCopyMissing: (missing: number, files: number) =>
          `A la copia del tema del reproductor web le faltan ${missing} de sus ${files} archivos en la carpeta de datos de Polyfin, como tras restaurar una copia de seguridad en otro sistema: mientras tanto, los navegadores los cargan desde sus direcciones originales, lo que requiere internet. Actualice la copia en Ajustes › Reproductor web, o elija otro tema.`,
        nvidiaDriver: (version: string) =>
          `La tarjeta gráfica NVIDIA no convierte el vídeo: su controlador es anterior a la versión ${version}, que FFmpeg necesita para NVENC. Actualice el controlador para usar la tarjeta.`,
        backupFailed: 'Ha fallado la última copia de seguridad de la base de datos.',
        backupStale: 'La última copia de seguridad de la base de datos tiene más de dos días.',
        backupDestinationFailed: (name: string) =>
          `${name}: ha fallado la última subida de las copias de seguridad de la base de datos.`,
        backupDestinationStale: (name: string) =>
          `${name}: la copia de seguridad de la base de datos más reciente que hay ahí tiene más de dos días.`,
        conversionsFull:
          'Las conversiones de vídeo están al límite: las nuevas esperan o se reproducen de otro modo.',
        unreadableSecrets: (names: string) =>
          `Algunas claves guardadas no se pueden descifrar con POLYFIN_SECRET_KEY y se consideran no definidas: ${names}. Vuelva a definir la clave con la que se cifraron, o introdúzcalas de nuevo.`,
        plaintextSecrets:
          'Las claves y los tokens se guardan sin cifrar. Defina POLYFIN_SECRET_KEY para cifrarlos.',
        appStopped: (app: string, user: string) =>
          `${app} ha rechazado el inicio de sesión que Polyfin guardaba para ${user}: no se importa nada hasta que ${user} vuelva a iniciar sesión en «Mi cuenta», con «Mantener la sesión iniciada» marcado.`,
        trackingStopped: (service: string, user: string) =>
          `${service} ha rechazado el token o la clave que Polyfin guardaba para ${user}: no se le envía ni se importa nada hasta que ${user} vuelva a pulsar «Conectar» en Mi cuenta › Seguimiento.`,
      },
      addonsTitle: 'Extensiones',
      addonsHelp:
        'Según las solicitudes hechas para las aplicaciones desde que se inició el servidor. «Comprobar» pide su manifiesto a una extensión una vez, como máximo una vez por minuto.',
      noAddons: 'El servidor no tiene ninguna extensión.',
      addonStatus: {
        ok: 'Responde',
        failing: 'Con errores',
        unknown: 'Ninguna solicitud desde el inicio',
        off: 'Desactivada',
      },
      failures: {
        unreachable: 'no se pudo contactar',
        timeout: 'no respondió a tiempo',
        private_network: 'está en una red local',
        rate_limited: 'pidió reducir el ritmo (demasiadas solicitudes)',
        server_error: 'respondió con un error del servidor',
        http_error: 'rechazó la solicitud',
        invalid_response: 'envió una respuesta demasiado grande',
      } as Record<string, string>,
      lastSuccess: 'Última respuesta',
      lastFailure: 'Último error',
      responseTime: 'Tiempo de respuesta',
      requests: (count: number, failures: number) =>
        `${count} ${count === 1 ? 'solicitud' : 'solicitudes'}, ${failures} ${failures === 1 ? 'fallida' : 'fallidas'}`,
      check: 'Comprobar',
      checkLabel: (name: string) => `Comprobar ${name} ahora`,
      checking: 'Comprobando…',
      iptvTitle: 'Fuentes IPTV',
      guidesTitle: 'Guías de programación',
      sharesTitle: 'Recursos compartidos de red',
      shareKinds: {
        smb: 'SMB',
        webdav: 'WebDAV',
        s3: 'S3',
        sftp: 'SFTP',
        rclone: 'rclone',
      } as Record<string, string>,
      shareStatus: {
        notified: 'Notificado',
        polled: 'Sondeado',
        unwatched: 'No vigilado',
        scans: 'Solo en los escaneos',
      },
      sharePolledEvery: (share: string, minutes: number) =>
        `${share === 's3' ? 'Los buckets S3 no avisan' : share === 'sftp' ? 'Los servidores SFTP no avisan' : 'Los remotos de rclone no avisan'} de los cambios: las carpetas de la carpeta se vuelven a listar ${minutes === 1 ? 'cada minuto' : `cada ${minutes} minutos`}, según indican sus ajustes.`,
      shareScansOnly:
        'Sus ajustes indican buscar cambios solo en sus escaneos: la carpeta se lee según su programación y cuando se escanea manualmente.',
      shareNotified:
        'El servidor avisa a Polyfin de cada cambio: el recurso compartido se vuelve a escanear unos segundos después. Sus carpetas también se listan cada 10 minutos, por si algún cambio no se notifica.',
      sharePartial:
        'El servidor avisa a Polyfin de los cambios hechos a través de la red, pero se le pasó uno en una carpeta del recurso compartido, como los hechos en el propio disco del servidor: las carpetas del recurso compartido también se vuelven a listar cada 2 minutos.',
      sharePolled: {
        webdav:
          'Los servidores WebDAV no avisan de los cambios: las carpetas del recurso compartido se vuelven a listar cada 2 minutos.',
        unsupported:
          'Este servidor SMB no avisa de los cambios: las carpetas del recurso compartido se vuelven a listar cada 2 minutos.',
        silent:
          'Este servidor SMB no avisó de un cambio que se detectó al listar: las carpetas del recurso compartido se vuelven a listar cada 2 minutos.',
        failed:
          'Polyfin ha perdido las notificaciones del servidor y volverá a solicitarlas en breve. Mientras tanto, las carpetas del recurso compartido se vuelven a listar cada 2 minutos.',
        starting:
          'Polyfin está pidiendo al servidor que avise de los cambios. Mientras tanto, las carpetas del recurso compartido se vuelven a listar cada 2 minutos.',
      } as Record<string, string>,
      shareUnwatched: {
        unreachable:
          'No se puede contactar con el recurso compartido. Se escanea según su programación y de nuevo en cuanto responda.',
        refused:
          'El recurso compartido rechaza el usuario o la contraseña de Polyfin. Compruébelos e introdúzcalos de nuevo.',
        host_key_changed:
          'El servidor SFTP presenta una clave de host distinta de la guardada. Acéptela en «Editar la carpeta» si el cambio es esperado.',
        failed:
          'Polyfin no puede vigilar este recurso compartido. Se sigue escaneando según su programación.',
      } as Record<string, string>,
      sharesOff:
        'La vigilancia de cambios está desactivada en los ajustes: los recursos compartidos solo se escanean según su programación.',
      transcoderTitle: 'Transcodificador',
      gpu: 'GPU',
      noGpu: 'Ninguna: el vídeo se convierte en la CPU',
      toneMapping: 'HDR a SDR en la GPU',
      cpuEncoders: 'Codificadores de CPU',
      conversions: 'Conversiones de vídeo',
      noLimit: 'sin límite',
      remuxes: 'Remuxados',
      maxHeight: 'Conversión hasta',
      transcodingOff: 'La conversión está desactivada en los ajustes.',
      databaseTitle: 'Base de datos',
      reachable: 'Responde',
      unreachable: 'No responde',
      size: 'Tamaño',
      cacheTitle: 'Caché de fuentes',
      cacheHelp:
        'Partes de los archivos en reproducción, guardadas en disco hasta el tamaño fijado en Ajustes › Reproducción. Es normal que la caché esté llena: las partes más antiguas dejan sitio.',
      cacheUse: (used: string, limit: string) => `${used} de ${limit}`,
      imagesTitle: 'Caché de imágenes',
      imagesHelp:
        'Las imágenes que se reenvían a las aplicaciones, sus copias redimensionadas y las imágenes que dibuja Polyfin, guardadas en disco hasta su límite, primero se van las usadas hace más tiempo. Vaciarla hace que cada imagen se descargue, redimensione o dibuje de nuevo en su siguiente petición. Las miniaturas para avanzar, las imágenes subidas, las imágenes de las carpetas locales y el tema del reproductor web no forman parte de ella.',
      imagesFiles: 'Imágenes en disco',
      imagesMemory: 'En memoria',
      clearImages: 'Vaciar la caché de imágenes',
      imagesCleared: (images: string, bytes: string) =>
        `Caché de imágenes vaciada: ${images} imágenes, ${bytes} liberados en disco.`,
      sourcesOpen: 'Archivos abiertos',
      torrentPieces: 'De ellos, piezas de torrents',
      torrentsTitle: 'Torrents',
      torrentsHelp:
        'Los torrents que la reproducción está leyendo ahora, solo por su ruta. Un torrent se detiene cuando termina su reproducción: sin más pares, sin más compartición.',
      torrentRoute: {
        interface: 'Interfaz de red',
        proxy: 'Proxy SOCKS5',
        wireguard: 'Túnel WireGuard',
        '': 'Sin ruta',
      },
      torrentAddresses: 'Direcciones del túnel',
      torrentHandshake: 'Última negociación',
      torrentNoHandshake: 'Ninguna todavía',
      torrentsPlaying: 'Torrents en reproducción',
      torrentsPlayingOf: (playing: number, atOnce: number) => `${playing} de ${atOnce}`,
      torrentPeers: 'Pares',
      torrentUTPPeers: 'Pares por uTP',
      torrentsShared: 'Compartido desde el inicio',
      disksTitle: 'Espacio en disco',
      folders: {
        cache: 'caché',
        recordings: 'grabaciones',
        backups: 'copias de seguridad',
      } as Record<string, string>,
      folderTitle: {
        cache: 'Carpeta de caché',
        recordings: 'Carpeta de grabaciones',
        backups: 'Carpeta de copias de seguridad',
      } as Record<string, string>,
      free: (free: string) => `${free} libres`,
      notMeasured: 'No se puede medir aquí.',
      thumbnailsTitle: 'Miniaturas (trickplay)',
      thumbnailsOff:
        'Las miniaturas y las imágenes de capítulos están desactivadas en Ajustes › Miniaturas (trickplay).',
      waiting: (count: number, length: number) => `${count} de ${length} en espera`,
      working: 'Creando imágenes',
      idle: 'Inactivo',
      pausedHosts: 'Fuentes en pausa',
      noPausedHosts: 'No hay ninguna fuente en pausa.',
      until: (time: string) => `hasta las ${time}`,
      ratingsTitle: 'Valoraciones de MDBList',
      ratingsHelp:
        'Las solicitudes que MDBList permite cada día a la clave de valoraciones del servidor, y las que quedan hasta su reinicio diario. Los títulos cuyas valoraciones no se conservan se consultan por lotes, en segundo plano.',
      requestsLeft: 'Solicitudes restantes hoy',
      requestsLeftOf: (left: string, limit: string) => `${left} de ${limit}`,
      notToldYet: 'Se sabrá en la próxima consulta a MDBList',
      resets: 'Reinicio diario',
      ratingsHeld: 'Nueva consulta',
      heldReasons: {
        limit: 'MDBList limita las solicitudes',
        refused: 'MDBList rechazó la clave',
        failed: 'MDBList no respondió',
      } as Record<string, string>,
      notHeld: 'Ahora',
      titlesWaiting: 'Títulos en espera',
      titlesKept: 'Títulos con valoraciones conservadas',
      backupTitle: 'Copias de seguridad',
      backupOff:
        'Las copias de seguridad están desactivadas. Actívelas en Ajustes › Copias de seguridad.',
      backup: {
        result: 'Última ejecución',
        none: 'Aún no hay ninguna copia de seguridad',
        succeeded: 'Correcta',
        failed: 'Fallida',
        made: 'Última copia de seguridad realizada',
        file: 'Archivo',
        size: 'Tamaño',
        next: 'Próxima copia de seguridad',
        runHint:
          'Ejecute «Hacer copia de seguridad de la base de datos» en la página Programación para hacerla ahora.',
      },
      processTitle: 'Polyfin',
      version: 'Versión',
      uptime: 'En ejecución desde hace',
      memory: 'Memoria',
      heap: 'En uso',
      goroutines: 'Goroutines',
      goroutinesDownload: 'Descargar las goroutines',
      goroutinesHelp:
        'Cuando una solicitud nunca recibe respuesta, descarga lo que está haciendo el servidor y envíalo con tu informe. No contiene direcciones ni secretos.',
      secretsTitle: 'Claves almacenadas',
      secretsHelp:
        'Las claves del servidor, los tokens y claves de seguimiento de los usuarios, y las contraseñas de los recursos compartidos de red, cifrados en la base de datos con POLYFIN_SECRET_KEY (openssl rand -base64 32).',
      encryption: 'Cifrado',
      encrypted: 'Activado',
      notEncrypted: 'Desactivado: POLYFIN_SECRET_KEY no está definida',
      storedPlain: 'Almacenadas sin cifrar',
      unreadable: 'No se pueden descifrar',
      noneUnreadable: 'Ninguna',
      proxiesTitle: 'Proxies conocidos',
      proxiesHelp:
        'Los nombres de host indicados en Proxies conocidos, resueltos en cada cambio de los ajustes, cada minuto y en cuanto una dirección desconocida reenvía solicitudes. Cada dirección a la que se resuelve un nombre se considera de confianza como proxy.',
      resolvesToNothing: 'No se resuelve a ninguna dirección',
      secretNames: {
        publicMetaDbKey: 'Clave de PublicMetaDB',
        theIntroDbKey: 'Clave de TheIntroDB',
        mdblistRatingsKey: 'Clave API de MDBList para valoraciones',
        tmdbKey: 'Clave API de TMDB',
        traktClientSecret: 'Secreto de cliente de Trakt',
        lastFmSecret: 'Secreto compartido de Last.fm',
        aniListClientSecret: 'Secreto de cliente de AniList',
        myAnimeListClientSecret: 'Secreto de cliente de MyAnimeList',
        smtpPassword: 'Contraseña SMTP',
        torrentProxyPassword: 'Contraseña del proxy SOCKS5 de los torrents',
        torrentWireGuard: 'Configuración de WireGuard de los torrents',
        metricsToken: 'Token de métricas',
        updateToolToken: 'Token de la herramienta de actualización',
      },
      connectionOf: (service: string, user: string) => `conexión de ${service} de ${user}`,
      targetOf: (target: string, user: string) =>
        `destino de notificaciones «${target}» de ${user}`,
      serverTargetOf: (target: string) => `destino de notificaciones del servidor «${target}»`,
      sharePasswordOf: (folder: string) => `contraseña o clave del recurso compartido «${folder}»`,
      destinationSecretOf: (name: string) =>
        `contraseña o clave del destino de las copias de seguridad «${name}»`,
      certificateKeys: 'claves del certificado de Let’s Encrypt',
      twoStepOf: (user: string) => `clave del inicio de sesión en dos pasos de ${user}`,
      noAddonsHint: 'Añada una extensión de Stremio en Contenido › Fuentes.',
      noIptvHint: 'Añada una cuenta IPTV en Contenido › Fuentes.',
      noGuides: 'No hay ninguna guía de programación.',
      noGuidesHint:
        'Añada una guía de programación a un catálogo de TV en Contenido › Televisión en vivo.',
      serverTitle: 'Servidor',
    },
    logs: {
      title: 'Registros',
      description:
        'Las últimas líneas del registro del servidor, guardadas en memoria. De las direcciones solo se conserva el host, y las contraseñas, tokens y claves se ocultan.',
      follow: 'Seguir',
      pause: 'Pausar',
      level: 'Nivel',
      levels: {
        all: 'Todos',
        info: 'Información y superior',
        warn: 'Advertencias y errores',
        error: 'Solo errores',
      },
      search: 'Buscar',
      download: 'Descargar',
      clear: 'Limpiar la vista',
      empty: 'Aún no hay ninguna línea.',
      noMatch: 'Ninguna línea coincide.',
      shown: (shown: number, total: number) => `Se muestran ${shown} de ${total} líneas.`,
      following: 'Siguiendo: las nuevas líneas aparecen cada 2 segundos.',
      paused: 'En pausa: las líneas escritas mientras tanto aparecerán al volver a seguir.',
      detailedHint:
        'Las líneas de depuración aparecen al activar «Registro detallado», en Ajustes › Diagnóstico.',
      region: 'Registro del servidor',
      emptyHint: 'Las líneas aparecen aquí a medida que el servidor las escribe.',
      noMatchHint: 'Elija otro nivel o modifique la búsqueda.',
      filters: 'Filtrar el registro',
    },
  },
}

export default system
