import type en from '../en/home'

/** The GPU methods, as the server names them. */
const gpuMethodNames: Record<string, string> = {
  cuda: 'NVIDIA',
  vaapi: 'VAAPI',
  videotoolbox: 'VideoToolbox',
  qsv: 'QSV',
  amf: 'AMF',
}

const home: typeof en = {
  quickConnect: {
    title: 'Quick Connect',
    description:
      'Inicie sesión en una aplicación de televisión o de móvil sin escribir su contraseña: elija Quick Connect en una aplicación de Jellyfin, o abra la aplicación de Silo en un televisor, y, a continuación, introduzca el código que se muestra.',
    code: 'Código de Quick Connect',
    codeHint: 'Los códigos caducan a los 10 minutos.',
    lookingUp: 'Buscando el dispositivo…',
    requestTitle: 'Dispositivo pendiente de aprobación',
    device: 'Dispositivo',
    app: 'Aplicación',
    requested: 'Solicitado',
    approve: 'Aprobar e iniciar sesión en este dispositivo',
    approving: 'Aprobando…',
    approved: (device: string, user: string) => `${device} ha iniciado sesión como ${user}.`,
    another: 'Aprobar otro dispositivo',
    steps: [
      'En el dispositivo, elija Quick Connect en una aplicación de Jellyfin, o abra la aplicación de Silo en un televisor.',
      'Escriba aquí el código que se muestra: 6 dígitos en Jellyfin, 8 en Silo.',
      'Compruebe el dispositivo y, a continuación, apruébelo.',
    ],
    stepsLabel: 'Cómo funciona',
  },
  notFound: {
    title: 'Página no encontrada',
    description: 'Esta página no existe.',
    home: 'Volver a la página de inicio',
  },
  dashboard: {
    overview: {
      greeting: (hour: number, name: string) =>
        `${hour >= 18 || hour < 5 ? 'Buenas noches' : hour >= 12 ? 'Buenas tardes' : 'Buenos días'}, ${name}`,
      summary: (playing: number, problems: number) =>
        `${playing === 0 ? 'No se está reproduciendo nada' : playing === 1 ? 'Una reproducción en curso' : `${playing} reproducciones en curso`}${
          problems === 0
            ? ' y nada que revisar. Todo funciona con normalidad.'
            : problems === 1
              ? ', y una cosa que revisar.'
              : `, y ${problems} cosas que revisar.`
        }`,
      description:
        'Lo que se está reproduciendo en el servidor y lo que ha ocurrido recientemente.',
      conversions: 'Conversiones de vídeo',
      conversionsOf: (count: number, limit: number) =>
        limit > 0 ? `${count} de ${limit}` : `${count}, sin límite`,
      bandwidth: 'Ancho de banda',
      bandwidthSent: (rate: string) => `${rate} enviados a las aplicaciones`,
      bandwidthReceived: (rate: string) => `${rate} recibidos de las fuentes`,
      uptime: 'En funcionamiento desde hace',
      problems: 'Para revisar',
      problemsEmpty: 'Nada que revisar',
      problemsEmptyHelp:
        'Polyfin supervisa sus discos, fuentes, guías, copias de seguridad y tareas. Todo lo que requiera su atención aparecerá aquí.',
      problemError: 'Problema',
      problemWarning: 'Por comprobar',
      update: (version: string, running: string) =>
        `Polyfin ${version} está disponible. Este servidor ejecuta la versión ${running}.`,
      releaseNotes: 'Notas de la versión',
      open: 'Abrir',
      openHealth: 'Abrir Estado del sistema',
      serverTitle: 'Estado del servidor',
      version: 'Versión',
      database: 'Base de datos',
      databaseSize: (size: string) => `${size} en disco`,
      cache: 'Caché',
      cacheOf: (limit: string) => `de ${limit}`,
      cacheNoLimit: 'sin límite',
      cacheOff: 'Desactivada',
      cacheUnit: 'Caché en uso',
      graphics: 'Tarjeta gráfica',
      graphicsNone: 'Ninguna',
      graphicsCpu: 'Las conversiones usan el procesador',
      toneMapping: 'mapeo de tonos',
    },
    server: {
      restart: 'Reiniciar',
      updateNow: 'Actualizar ahora',
      install: (version: string) => `Instalar Polyfin ${version}`,
      downloading: (percent: number) => `Descargando: ${percent} %`,
      confirmWith: 'Confirmar con',
      withPassword: 'Contraseña',
      withCode: 'Código de dos pasos',
      password: 'Su contraseña',
      code: 'Código de su aplicación, o un código de recuperación',
      identityHelp: 'Cada una de estas acciones lo vuelve a pedir.',
      restartTitle: 'Reiniciar Polyfin',
      restartText:
        'Polyfin detiene lo que está haciendo, reproducciones y grabaciones incluidas, y termina. Las aplicaciones se vuelven a conectar en cuanto responde.',
      restartBy: {
        desktop: 'Polyfin Desktop lo vuelve a iniciar por sí mismo.',
        confirmed:
          'La política de reinicio de su contenedor, o su gestor de servicios, lo vuelve a iniciar, como se confirmó en los ajustes generales.',
      },
      unknownTitle: 'Polyfin podría no volver',
      unknown: {
        container:
          'Polyfin no ve la política de reinicio de su contenedor. Sin una que lo vuelva a iniciar, como unless-stopped o always, Polyfin queda detenido hasta que se inicie en el host.',
        other:
          'Polyfin no sabe si un gestor de servicios lo vuelve a iniciar. Sin él, Polyfin queda detenido hasta que se inicie en el host.',
      },
      unknownHelp:
        'Cuando esté configurado, active «Polyfin se vuelve a iniciar cuando se detiene» en los ajustes generales, y este aviso desaparece.',
      restartConfirm: 'Reiniciar',
      restartAnyway: 'Reiniciar de todos modos',
      restarting: 'Reiniciando…',
      waitingTitle: 'Polyfin se está reiniciando',
      waiting: 'Esperando a que Polyfin vuelva a responder…',
      waitingLong: (seconds: number) =>
        `Polyfin no responde desde hace ${seconds} segundos. Si nada lo vuelve a iniciar, inícielo en el host.`,
      installTitle: (version: string) => `Instalar Polyfin ${version}`,
      installText:
        'Polyfin Desktop descarga la nueva versión y la comprueba, luego detiene Polyfin, la instala y la inicia, como Instalar Polyfin en su menú. Las reproducciones y grabaciones se detienen mientras tanto.',
      installConfirm: 'Instalar',
      installing: 'Pidiendo…',
      installWaitingTitle: (version: string) => `Instalando Polyfin ${version}`,
      installStopping:
        'Polyfin se detiene para instalar la nueva versión. Esperando a que vuelva a responder…',
      installFailed:
        'Polyfin Desktop no pudo instalar la actualización: la carpeta de registros dice por qué. Polyfin sigue como estaba.',
      updateTitle: 'Actualizar ahora',
      updateText:
        'Polyfin pide a la herramienta de actualización de los ajustes generales que actualice ahora. Lo que haga después depende de ella: si reemplaza el contenedor de Polyfin, Polyfin se detiene un momento.',
      updateConfirm: 'Enviar la petición',
      sending: 'Enviando…',
      answer: (status: number) => `La herramienta de actualización respondió HTTP ${status}.`,
      answerEmpty: 'Su respuesta no contiene texto.',
      watching:
        'Si la herramienta reemplaza el contenedor de Polyfin, esta página se recarga en cuanto Polyfin vuelva a responder.',
      gone: 'Polyfin se detuvo antes de que la herramienta respondiera: probablemente lo está reemplazando. Esperando a que Polyfin vuelva a responder…',
      tooSoon: 'Esto se pidió hace menos de un minuto. Inténtelo de nuevo en un momento.',
      close: 'Cerrar',
    },
    live: {
      title: 'Reproduciendo ahora',
      empty: 'No se está reproduciendo nada en este momento.',
      emptyHelp:
        'Las reproducciones desde aplicaciones de Jellyfin aparecen aquí en pocos segundos.',
      autoRefresh: 'Se actualiza cada 3 segundos.',
      paused: 'En pausa',
      unknownTitle: 'Título no encontrado',
      movie: (year: number) => (year > 0 ? `Película, ${year}` : 'Película'),
      liveChannel: 'En vivo',
      progress: (title: string) => `Posición de reproducción de ${title}`,
      remaining: (time: string) => `Quedan ${time}`,
      startedAt: (time: string) => `Iniciado ${time}`,
      appOn: (app: string, device: string) => (app ? `${app} en ${device}` : device),
      sharedLink: 'Enlace compartido',
      downloadOn: (app: string, device: string) =>
        app ? `Descarga, ${app} en ${device}` : 'Descarga',
      downloading: 'Descargando',
      downloadProgress: (title: string) => `Progreso de la descarga de ${title}`,
      downloadNotControllable:
        'Una descarga convertida para verla sin conexión. Se detiene cuando la aplicación la cancela.',
      howItPlays: 'Cómo se reproduce',
      details: 'Detalles',
      delivery: {
        directPlay: 'Reproducción directa',
        remux: 'Remux',
        conversion: 'Conversión',
        stream: 'Transmisión',
      } as Record<string, string>,
      deliveryHelp: {
        directPlay: 'La aplicación lee el archivo tal cual.',
        remux: 'Polyfin reempaqueta el archivo para la aplicación, sin volver a codificarlo.',
        conversion: 'Polyfin vuelve a codificar lo que la aplicación no puede reproducir.',
        stream:
          'Polyfin transmite el archivo a la aplicación. Los detalles vuelven a mostrarse en cuanto la aplicación pide más.',
      } as Record<string, string>,
      why: 'Motivo',
      source: 'Fuente',
      sent: 'Enviado',
      encoder: 'Codificador',
      gpu: (method: string) => `GPU (${gpuMethodNames[method] ?? method})`,
      gpuMethod: (method: string) => gpuMethodNames[method] ?? method,
      cpu: 'CPU',
      copied: 'Copiado',
      toneMapped: 'HDR a SDR',
      burned: 'Subtítulos incrustados',
      qualityGroup: 'Grupo de calidad',
      original: 'Original',
      sizeLimit: 'Menor que la fuente',
      qualityPlayed: 'Calidad reproducida',
      switchedVersion: 'Versión de relevo',
      bandwidth: {
        sent: (rate: string) => `${rate} a la aplicación`,
        received: (rate: string) => `${rate} desde la fuente`,
        redirected: 'La aplicación lee la fuente directamente',
      },
      origins: {
        addon: 'Extensión',
        torrent: 'Torrent',
        localFolder: 'Carpeta local',
        networkShare: 'Recurso compartido de red',
        iptv: 'IPTV',
      } as Record<string, string>,
      peers: (count: number) => (count === 1 ? '1 par' : `${count} pares`),
      qualityAmong: (played: string, offered: string) => `${played}, entre ${offered}`,
      sizeLimits: {
        steppedDown: (height: string) => `La aplicación bajó de ${height}`,
        gpu: (height: string) => `${height} como máximo: convertido por la tarjeta gráfica`,
        processor: (height: string) => `${height} como máximo: convertido por el procesador`,
        toneMapping: (height: string) => `${height} como máximo: HDR convertido por el procesador`,
        qualityGroup: (height: string) => `Grupo de calidad: ${height}`,
        serverMax: (height: string) => `Calidad máxima del vídeo convertido: ${height}`,
        app: (height: string) => `Calidad permitida por la aplicación: ${height}`,
        appBitrate: (rate: string) => `Tasa de bits permitida por la aplicación: ${rate}`,
        userBitrate: (rate: string) => `Calidad máxima del usuario: ${rate}`,
      },
      unknown: 'Desconocido',
      stop: 'Detener',
      stopFor: (user: string) => `Detener la reproducción de ${user}`,
      stopTitle: '¿Detener esta reproducción?',
      stopConfirm: (user: string, title: string) =>
        `¿Detener «${title}» en el dispositivo de ${user}? La aplicación deja de reproducir de inmediato.`,
      stopped: 'Se ha pedido a la aplicación que se detenga.',
      message: 'Enviar un mensaje',
      messageTo: (user: string) => `Enviar un mensaje a ${user}`,
      messageLabel: (device: string) => `Mensaje que se mostrará en ${device}`,
      messageHint: 'Se muestra en el dispositivo durante 10 segundos. 500 caracteres como máximo.',
      send: 'Enviar',
      messageSent: 'Mensaje enviado.',
      notControllable: 'Esta aplicación no admite el control remoto.',
      sharedNotControllable:
        'Un invitado que ve el contenido mediante un enlace para compartir. Para detenerlo, revoque el enlace en la página del usuario.',
    },
    reasons: {
      ContainerNotSupported: 'Formato de archivo no compatible',
      VideoCodecNotSupported: 'Códec de vídeo no compatible',
      AudioCodecNotSupported: 'Códec de audio no compatible',
      SubtitleCodecNotSupported: 'Formato de subtítulos no compatible',
      AudioIsExternal: 'Audio en un archivo aparte',
      SecondaryAudioNotSupported: 'Segunda pista de audio no compatible',
      VideoProfileNotSupported: 'Perfil de vídeo no compatible',
      VideoLevelNotSupported: 'Nivel de vídeo no compatible',
      VideoResolutionNotSupported: 'Resolución demasiado alta',
      VideoBitDepthNotSupported: 'Profundidad de bits de vídeo no compatible',
      VideoFramerateNotSupported: 'Velocidad de fotogramas no compatible',
      RefFramesNotSupported: 'Demasiados fotogramas de referencia',
      AnamorphicVideoNotSupported: 'Vídeo anamórfico no compatible',
      InterlacedVideoNotSupported: 'Vídeo entrelazado no compatible',
      VideoCodecTagNotSupported: 'Etiqueta de códec de vídeo no compatible',
      VideoBitrateNotSupported: 'Tasa de bits de vídeo demasiado alta',
      AudioChannelsNotSupported: 'Demasiados canales de audio',
      AudioProfileNotSupported: 'Perfil de audio no compatible',
      AudioSampleRateNotSupported: 'Frecuencia de muestreo de audio no compatible',
      AudioBitDepthNotSupported: 'Profundidad de bits de audio no compatible',
      AudioBitrateNotSupported: 'Tasa de bits de audio demasiado alta',
      ContainerBitrateExceedsLimit: 'Por encima del límite de tasa de bits',
      UnknownVideoStreamInfo: 'Vídeo sin describir',
      UnknownAudioStreamInfo: 'Audio sin describir',
      DirectPlayError: 'Error en la reproducción directa',
      VideoRangeTypeNotSupported: 'HDR no compatible',
      StreamCountExceedsLimit: 'Demasiadas pistas',
    } as Record<string, string>,
    activity: {
      title: 'Actividad reciente',
      openLog: 'Abrir el registro',
      empty: 'Aún no hay actividad.',
      emptyHelp:
        'Los inicios de sesión, las reproducciones y los cambios en el servidor aparecen aquí a medida que se producen.',
      warning: 'Advertencia',
      error: 'Error',
      filter: 'Mostrar',
      filters: {
        all: 'Todo',
        problems: 'Problemas',
        signIns: 'Inicios de sesión',
        playback: 'Reproducción',
        users: 'Usuarios',
        server: 'Servidor y extensiones',
        recordings: 'Grabaciones',
      },
      search: 'Buscar en los eventos mostrados',
      listLabel: 'Últimos eventos',
      noMatch: 'Ningún evento coincide.',
      noMatchHelp: 'Elija otro filtro o borre la búsqueda.',
      count: (shown: number, total: number) => `${shown} de ${total} eventos.`,
      autoRefresh: 'Se actualiza cada 15 segundos.',
    },
  },
}

export default home
