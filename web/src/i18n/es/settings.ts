import type en from '../en/settings'
import type { RangeText } from '../en/settings'

const settings: typeof en = {
  settings: {
    tracking: {
      description:
        'Cada usuario puede conectar sus propias cuentas de Trakt, Simkl, MDBList, PublicMetaDB, AniList y MyAnimeList desde su página Mi cuenta, y Polyfin informa a esos servicios de lo que ve; Last.fm y ListenBrainz reciben las canciones que reproduce. Trakt, Simkl, AniList, MyAnimeList y Last.fm necesitan antes una aplicación de este servidor, que se configura aquí. MDBList, PublicMetaDB y ListenBrainz no necesitan nada.',
      traktSetup:
        'Cree una aplicación en trakt.tv, en Settings, Your API Apps, New application. Póngale cualquier nombre y copie aquí su client ID y su client secret.',
      redirectUri: 'URI de redirección que debe introducirse en la aplicación',
      traktClientId: 'Client ID de Trakt',
      traktClientIdHelp:
        'Déjelo vacío para desactivar Trakt. Los usuarios podrán conectarse cuando se hayan guardado el client ID y el client secret.',
      traktClientSecret: 'Client secret de Trakt',
      traktClientSecretHelp:
        'Se muestra en la página de la aplicación en trakt.tv, debajo del client ID.',
      simklSetup:
        'Cree una aplicación en simkl.com, en Settings, Developer, Create new app, del tipo «TV, devices & command line» (el AUTH V2 de Simkl; no necesita URI de redirección, y se rechazan los client ID de las aplicaciones AUTH V1 antiguas). Póngale cualquier nombre y copie aquí su client ID.',
      simklClientId: 'Client ID de Simkl',
      simklClientIdHelp: 'Déjelo vacío para desactivar Simkl. No se necesita client secret.',
      lastFmSetup:
        'Cree una cuenta de API en last.fm, en last.fm/api/account/create. Póngale cualquier nombre y deje vacía su callback URL: los usuarios autorizan a Polyfin en Last.fm, y el propio Polyfin solicita su sesión a Last.fm. Después, copie aquí su clave API y su secreto compartido.',
      lastFmApiKey: 'Clave API de Last.fm',
      lastFmApiKeyHelp:
        'Déjelo vacío para desactivar Last.fm. Los usuarios podrán conectarse cuando se hayan guardado la clave API y el secreto compartido.',
      lastFmSecret: 'Secreto compartido de Last.fm',
      lastFmSecretHelp:
        'Se muestra en la página de su cuenta de API en last.fm, debajo de la clave API.',
      aniListSetup:
        'Cree una aplicación en anilist.co, en Settings, Developer, Create New Client. Póngale cualquier nombre y, como URL de redirección, la dirección que aparece abajo, donde AniList muestra a los usuarios el código que pegan en Polyfin. Después, copie aquí su client ID y su client secret.',
      aniListClientId: 'Client ID de AniList',
      aniListClientIdHelp:
        'Déjelo vacío para desactivar AniList. Los usuarios podrán conectarse cuando se hayan guardado el client ID y el client secret.',
      aniListClientSecret: 'Client secret de AniList',
      aniListClientSecretHelp:
        'Se muestra en la página de la aplicación en anilist.co, debajo del client ID.',
      myAnimeListSetup:
        'Cree una aplicación en myanimelist.net, en Account Settings, API, Create ID, del tipo «web». Póngale cualquier nombre y, como URL de redirección, la dirección que aparece abajo: MyAnimeList devuelve allí el navegador de los usuarios cuando autorizan a Polyfin. Solo sus navegadores necesitan poder acceder a ella. Después, copie aquí su client ID y su client secret.',
      myAnimeListRedirectHelp:
        'La dirección pública de este servidor, en Ajustes › General; si no hay, la dirección que utiliza ahora.',
      myAnimeListClientId: 'Client ID de MyAnimeList',
      myAnimeListClientIdHelp:
        'Déjelo vacío para desactivar MyAnimeList. Los usuarios podrán conectarse cuando se hayan guardado el client ID y el client secret.',
      myAnimeListClientSecret: 'Client secret de MyAnimeList',
      myAnimeListClientSecretHelp:
        'Se muestra en la página de la aplicación en myanimelist.net, debajo del client ID.',
    },
    title: 'Ajustes',
    description: 'Opciones que se aplican a todo el servidor.',
    serverName: 'Nombre del servidor',
    serverNameHelp: (r: RangeText) =>
      `De ${r.min} a ${r.max} caracteres; se muestra en las aplicaciones de Jellyfin.`,
    quickConnect: 'Permitir Quick Connect',
    quickConnectHelp:
      'Permite que las aplicaciones de televisión y móvil inicien sesión con un código aprobado aquí: 6 dígitos en las aplicaciones de Jellyfin, 8 en las aplicaciones de Silo para televisor mientras estén permitidas.',
    language: 'Idioma de los nombres generados',
    languageHelp:
      'Polyfin asigna él mismo algunos nombres en las aplicaciones de Jellyfin: las temporadas («Temporada 1», «Especiales»), los episodios sin título y el tipo que se añade a las bibliotecas que comparten nombre («Populares (Películas)»).',
    legacyAuthorization: 'Permitir la autorización heredada',
    legacyAuthorizationHelp:
      'Acepta las antiguas cabeceras X-Emby-*, el parámetro api_key y el esquema Emby para las aplicaciones que todavía los necesitan. Desactivado de forma predeterminada, como en Jellyfin 12.4.',
    siloApps: 'Permitir las aplicaciones de Silo',
    siloAppsHelp:
      'Permite que las aplicaciones de Silo para iPhone, iPad, Apple TV, Mac y Android inicien sesión y reproduzcan, mediante la API de Silo en /api/v2, como las aplicaciones de Jellyfin, dentro de los límites de cada usuario. Desactivado, esa API no responde.',
    updateCheck: 'Buscar nuevas versiones',
    updateCheckHelp:
      'Una vez al día, y poco después del arranque, Polyfin consulta a GitHub su última versión publicada, enviando solo su propia versión. Si hay una más reciente, se muestra en Inicio y en Sistema › Estado del sistema, y se envía a las notificaciones de «Nueva versión». Si está desactivado, Polyfin nunca contacta con GitHub.',
    restartConfirmed: 'Polyfin se vuelve a iniciar cuando se detiene',
    restartConfirmedHelp:
      'Actívelo cuando el contenedor de Polyfin tenga una política de reinicio que lo vuelva a iniciar, como unless-stopped, always u on-failure, o cuando lo haga su gestor de servicios. Polyfin no puede ver la política de reinicio de su contenedor sin el socket de Docker, que nunca recibe: mientras esté desactivado, Reiniciar en Inicio avisa de que Polyfin podría no volver.',
    updateToolUrl: 'Dirección de la herramienta de actualización',
    updateToolUrlHelp:
      'La API de actualización de una herramienta que ejecuta junto a Polyfin para actualizar su contenedor, como http://updater:8080/v1/update. Actualizar ahora, en Inicio, le envía una petición POST sin cuerpo. Vacía, el botón no se muestra.',
    updateToolToken: 'Token de la herramienta de actualización',
    updateToolTokenHelp:
      'Se envía como cabecera Authorization: Bearer con cada petición, si la herramienta lo pide. Se guarda cifrado, como los demás secretos.',
    localDiscovery: 'Responder a la detección en la red local',
    localDiscoveryHelp:
      'Las aplicaciones de Jellyfin y Emby que buscan servidores en la red local encuentran Polyfin y lo muestran, sin necesidad de escribir su dirección. Preguntan por el puerto UDP 7359, y Polyfin solo responde a direcciones de la red local. Se aplica al instante, sin reiniciar. En Docker, requiere la red del host o publicar el puerto 7359/udp: consulte la documentación de instalación.',
    localAddress: 'Dirección en la red local',
    localAddressHelp:
      'La dirección con la que las aplicaciones de la red local acceden a Polyfin, como http://192.168.1.10:8096. Las respuestas de detección la facilitan, y las aplicaciones de Jellyfin la toman como la dirección local del servidor. Si está vacía, las respuestas dan la dirección de la interfaz de red que llega a la aplicación, lo cual es correcto salvo que Polyfin se ejecute en una red bridge de Docker: en ese caso, introduzca la dirección del host y el puerto de Polyfin.',
    torrents: {
      description:
        'Algunas extensiones muestran torrents en lugar de enlaces. Polyfin puede reproducirlos: solo se une al enjambre de un torrent mientras una reproducción lo lee, y mantiene todo su tráfico en una única ruta, una interfaz de red como la de una VPN, un proxy SOCKS5 o un túnel WireGuard que el propio Polyfin ejecuta. Nunca se envía nada por otro camino. Las piezas se guardan dentro del Espacio en disco para archivos en lectura, en Ajustes › Reproducción.',
      play: 'Reproducir transmisiones torrent',
      playHelp:
        'Desactivado de forma predeterminada. Reproducir un torrent lo descarga de otros pares y comparte lo descargado mientras el límite de subida lo permita: usted es responsable de lo que este servidor descarga y comparte. Los usuarios también necesitan Puede reproducir transmisiones torrent, en su página de Usuarios. Requiere una ruta.',
      route: 'Ruta',
      routeHelp:
        'Todos los sockets de torrent, para pares, trackers y la DHT, salen únicamente por aquí. Cuando no está disponible, se omiten las versiones torrent, también durante una reproducción, y Sistema › Estado del sistema indica el motivo.',
      routeHelpNoInterface:
        'Todos los sockets de torrent, para pares, trackers y la DHT, salen únicamente por aquí. Este sistema no vincula sockets a una interfaz de red, algo que solo hace Linux, como ocurre en Polyfin Desktop para Windows y macOS: un proxy SOCKS5 o un túnel WireGuard lleva los torrents. Cuando no está disponible, se omiten las versiones torrent, también durante una reproducción, y Sistema › Estado del sistema indica el motivo.',
      routes: { interface: 'Interfaz de red', proxy: 'Proxy SOCKS5', wireguard: 'Túnel WireGuard' },
      interface: 'Interfaz de red para torrents',
      interfaceHelp:
        'Su nombre, como wg0. Todos los sockets de torrent se vinculan a ella, y los nombres de los trackers se resuelven a través de ella. Solo Linux. En Docker, el contenedor debe ver la interfaz: consulte la documentación de torrents.',
      proxy: 'Proxy SOCKS5 para torrents',
      proxyHelp:
        'Su dirección y puerto, como 10.0.0.2:1080. Los pares y los trackers HTTP pasan por su CONNECT. Los trackers UDP, la DHT y uTP necesitan su UDP ASSOCIATE y, sin él, permanecen desactivados. A través de un proxy, ningún par puede conectarse de entrada.',
      proxyUser: 'Usuario del proxy',
      proxyUserHelp: 'Vacío si el proxy no lo pide.',
      proxyPassword: 'Contraseña del proxy',
      proxyPasswordHelp: 'Se guarda cifrada cuando POLYFIN_SECRET_KEY está definida.',
      wireGuard: 'Configuración de WireGuard',
      wireGuardHelp:
        'Pegue el .conf que su proveedor de VPN da para WireGuard. Polyfin ejecuta el túnel por sí mismo, solo para los torrents, sin nada que configurar en este sistema. Los nombres de los trackers se resuelven a través del túnel con sus servidores DNS, nunca de otro modo. Ningún par puede conectarse desde fuera. Se guarda cifrada cuando POLYFIN_SECRET_KEY está definida, y no se vuelve a mostrar.',
      wireGuardSaved: (endpoint: string, addresses: string) =>
        `Hay una configuración guardada: servidor ${endpoint}, direcciones ${addresses}.`,
      wireGuardDns: (servers: string) =>
        `Los nombres de los trackers los resuelve ${servers}, a través del túnel.`,
      wireGuardNoDns:
        'No indica ningún servidor DNS: no se llega a los trackers designados por un nombre de host, sí a los designados por una dirección.',
      wireGuardReplace: 'Pegue otra configuración para sustituir la guardada.',
      wireGuardInvalid: (problem: { line: number; key: string; reason: string }) => {
        const where = problem.line > 0 ? `Línea ${problem.line}: ` : ''
        const expected: Record<string, string> = {
          PrivateKey: 'una clave de 32 bytes en base64',
          PublicKey: 'una clave de 32 bytes en base64',
          PresharedKey: 'una clave de 32 bytes en base64',
          Address: 'direcciones IP, como 10.2.0.2/32, separadas por comas',
          DNS: 'direcciones IP separadas por comas',
          MTU: 'un número de 576 a 65535',
          ListenPort: 'un puerto de 0 a 65535',
          Endpoint:
            'un nombre de host o una dirección IP con un puerto, como vpn.example.com:51820',
          AllowedIPs:
            'direcciones IP con la longitud de su prefijo, como 0.0.0.0/0, separadas por comas',
          PersistentKeepalive: 'off, o un número de segundos de 0 a 65535',
        }
        switch (problem.reason) {
          case 'too_long':
            return 'La configuración de WireGuard debe ser un texto de 8192 bytes como máximo.'
          case 'syntax':
            return `${where}cada línea debe ser una [sección], una Clave = Valor o un comentario #.`
          case 'outside':
            return `${where}${problem.key} aparece antes de [Interface] o [Peer].`
          case 'unknown':
            return `${where}${problem.key} no forma parte de una configuración de WireGuard.`
          case 'repeated':
            return problem.key === 'Peer'
              ? `${where}solo se admite una sección [Peer].`
              : `${where}${problem.key} aparece dos veces.`
          case 'missing':
            return `${where}falta ${problem.key} en la configuración de WireGuard.`
          case 'no_ipv4':
            return `${where}Address necesita una dirección IPv4, que usan los trackers UDP y la DHT.`
          case 'not_all':
            return `${where}AllowedIPs necesita 0.0.0.0/0: los pares de un torrent están en cualquier parte.`
          default:
            return `${where}${problem.key} debe ser ${expected[problem.key] ?? 'válido'}.`
        }
      },
      downloadLimit: 'Límite de descarga (Mbps, 0 = sin límite)',
      downloadLimitHelp: (r: RangeText) => `De 0 a ${r.max}; ${r.default} de forma predeterminada.`,
      uploadLimit: 'Límite de subida (Mbps, 0 = sin compartir)',
      uploadLimitHelp: (r: RangeText) =>
        `Los pares favorecen a quienes comparten, así que el inicio puede ser más lento con 0. Se deja de compartir cuando termina la reproducción. De 0 a ${r.max}; ${r.default} de forma predeterminada.`,
      atOnce: 'Torrents a la vez (0 = sin límite)',
      atOnceHelp: (r: RangeText) =>
        `Cuántos torrents pueden funcionar a la vez, para todos los usuarios juntos. En el límite, un título reproduce otra de sus versiones que no sea un torrent, o se indica que no se puede reproducir por ahora; un lugar se libera cuando un torrent se detiene, 30 segundos después de su reproducción, o enseguida cuando el mismo dispositivo o usuario reproduce otro título. De 0 a ${r.max}; ${r.default} de forma predeterminada.`,
      trackers: 'Trackers añadidos a cada torrent',
      trackersHelp: (r: RangeText) =>
        `URL de anuncio, una por línea, como udp://tracker.example:1337/announce. Polyfin les pide los pares de cada torrent, además de los trackers que indica su extensión: así, un torrent que la extensión lista sin ninguno no depende solo de la DHT. HTTP, HTTPS y UDP; ${r.max} como máximo. Un torrent privado solo pregunta a sus propios trackers. Vacío de forma predeterminada: Polyfin no incluye ninguna lista.`,
    },
    dlna: {
      title: 'DLNA',
      description:
        'Los televisores, receptores, consolas y reproductores de la red local que no tienen aplicación de Jellyfin pueden explorar y reproducir las bibliotecas mediante DLNA. Un dispositivo no puede iniciar sesión: cada uno muestra las bibliotecas del usuario elegido para él, con los límites de ese usuario.',
      share: 'Compartir bibliotecas mediante DLNA',
      shareHelp:
        'Anuncia Polyfin a los dispositivos de la red local en el puerto UDP 1900 y solo les responde a ellos. Se aplica de inmediato, sin reiniciar. En Docker, requiere la red del host o una red que dé al contenedor su propia dirección: consulte la documentación de DLNA.',
      name: 'Nombre en la red',
      nameHelp:
        'El nombre con el que los dispositivos muestran Polyfin. Si se deja vacío, el nombre del servidor.',
      newDevicesUser: 'Mostrar a los dispositivos nuevos las bibliotecas de',
      newDevicesUserHelp:
        'El usuario cuyas bibliotecas muestra un dispositivo detectado por primera vez. Con «Nadie», un dispositivo nuevo ve un servidor vacío hasta que se le elija un usuario más abajo.',
      noOne: 'Nadie',
      markPlayed: 'Marcar como visto lo que reproducen los dispositivos DLNA',
      markPlayedHelp:
        'Los dispositivos no informan de su posición: un título cuenta como visto cuando el dispositivo ha leído más allá de «Marcar como visto después de (%)». Los dispositivos leen por adelantado, así que un título puede marcarse antes de tiempo.',
      devices: 'Dispositivos',
      devicesEmpty: 'Ningún dispositivo ha solicitado nada todavía.',
      device: 'Dispositivo',
      address: 'Dirección',
      model: 'Modelo',
      lastSeen: 'Visto por última vez',
      showsLibrariesOf: 'Muestra las bibliotecas de',
      blocked: 'Bloqueado',
      profile: 'Perfil',
      automaticProfile: (name: string) => `Automático (${name})`,
      forget: 'Olvidar',
      profiles: 'Perfiles',
      profilesHelp:
        'Los perfiles usan el formato XML del complemento DLNA de Jellyfin. Un dispositivo recibe el primero que coincide con lo que envía, primero los perfiles añadidos y, si no, el genérico.',
      builtin: 'Integrado',
      addProfile: 'Añadir un perfil',
      replaceProfile: 'Reemplazar',
      removeProfile: 'Eliminar',
      profileXml: 'XML del perfil',
      devicesHelp:
        'Los dispositivos de la red local que han solicitado algo a Polyfin, del más reciente al más antiguo, cada uno identificado por su dirección. Los cambios se aplican de inmediato.',
      devicesLabel: 'Dispositivos DLNA',
      automatic: 'Automático',
      deviceSaved: (name: string) => `${name} guardado.`,
      forgetTitle: (name: string) => `¿Olvidar ${name}?`,
      forgetBody:
        'Polyfin olvida el usuario, el bloqueo y el perfil elegidos para él. Si vuelve a solicitar algo, será un dispositivo nuevo y mostrará las bibliotecas del usuario elegido para los dispositivos nuevos.',
      forgotten: (name: string) => `${name} olvidado.`,
      profilesLabel: 'Perfiles DLNA',
      added: 'Añadido',
      viewXml: 'Ver XML',
      profileXmlHelp:
        'Pegue el XML de un perfil creado para el complemento DLNA de Jellyfin o abra su archivo.',
      openFile: 'Abrir un archivo',
      add: 'Añadir',
      adding: 'Añadiendo…',
      replacing: 'Reemplazando…',
      profileAdded: (name: string) => `Perfil ${name} añadido.`,
      profileReplaced: (name: string) => `Perfil ${name} reemplazado.`,
      removeTitle: (name: string) => `¿Eliminar el perfil ${name}?`,
      removeBody:
        'A los dispositivos a los que se impone se les volverá a elegir el perfil según lo que envían.',
      profileRemoved: (name: string) => `Perfil ${name} eliminado.`,
    },
    metrics: 'Exponer métricas',
    metricsHelp:
      'Sirve las métricas de Polyfin a Prometheus en /metrics: reproducciones, conversiones, solicitudes a extensiones, importaciones, notificaciones, solicitudes HTTP, escaneos, la base de datos, el espacio en disco y el proceso. No incluyen ningún usuario, título, dirección ni dispositivo. Prometheus envía el token de métricas con cada solicitud; sin él, /metrics responde 401 y, mientras esta opción esté desactivada, 404.',
    metricsToken: 'Token de métricas',
    metricsTokenHelp:
      'Polyfin lo crea la primera vez que se activan las métricas. Prometheus lo envía como token bearer: consulte la documentación de administración para ver una configuración de recogida.',
    metricsTokenPending: 'Polyfin creará el token de métricas al guardar.',
    newMetricsToken: 'Crear un token nuevo',
    newMetricsTokenTitle: '¿Crear un token de métricas nuevo?',
    newMetricsTokenConfirm:
      'El token actual deja de funcionar de inmediato: Prometheus necesitará el nuevo para leer las métricas.',
    newMetricsTokenMade: 'Nuevo token de métricas creado',
    legacyWarningTitle: 'Advertencia de seguridad',
    legacyWarning:
      'Los métodos heredados pueden enviar credenciales en las URL, que acaban en los registros, el historial del navegador y los proxies. Active esta opción solo si una aplicación que usa no puede iniciar sesión de otro modo.',
    prepareAhead: 'Preparar la reproducción por adelantado',
    prepareAheadHelp:
      'Polyfin lee el archivo en cuanto se abre la página de un título y prepara el siguiente episodio cerca del final del actual, para que la reproducción empiece al instante. También obtiene por adelantado las versiones de los títulos de «Seguir viendo» y «A continuación», para que sus páginas las muestren de inmediato. Esto envía algunas solicitudes más a sus fuentes, también para títulos que se abren pero no se reproducen.',
    transcoding: 'Conversión (transcodificación)',
    transcodingHelp:
      'Vuelve a codificar el vídeo y el audio para las aplicaciones que no pueden reproducir un archivo tal cual. Si está desactivada, las aplicaciones reproducen los archivos tal cual o simplemente reempaquetados sin recodificar, y un título que una aplicación no pueda reproducir así no se iniciará en ella. Esto incluye las grabaciones convertidas a HEVC en aplicaciones que no reproducen HEVC.',
    analysisTimeout: 'Tiempo máximo para analizar una versión',
    analysisTimeoutHelp: (r: RangeText) =>
      `Antes de una primera reproducción, Polyfin analiza el archivo o el canal para saber cómo reproducirlo. Si la fuente no responde en este número de segundos, Polyfin descarta esa versión y pasa a la siguiente. Un número menor pasa antes a la siguiente, pero puede descartar fuentes lentas que habrían funcionado. Un canal se analiza durante 8 segundos como máximo. De ${r.min} a ${r.max} segundos; ${r.default} de forma predeterminada.`,
    versionAttempts: 'Versiones probadas cuando una no funciona',
    versionAttemptsHelp: (r: RangeText) =>
      `Cuando una aplicación reproduce un título sin elegir versión, Polyfin prueba las versiones en orden hasta que una funciona, pero no analiza más de este número. También se prueban las versiones ya analizadas, ya que no cuestan nada. Un número mayor encuentra con más frecuencia una versión que funcione, pero un título que no se reproduce tarda más en indicarlo. De ${r.min} a ${r.max}; ${r.default} de forma predeterminada.`,
    preferDirectPlay: 'Preferir versiones que la aplicación reproduce sin conversión',
    preferDirectPlayHelp:
      'Cuando una aplicación reproduce un título sin elegir versión, Polyfin escoge la primera versión que la aplicación reproduce tal cual o simplemente reempaquetada, en lugar de la primera que se pueda reproducir. La primera reproducción de un título puede ser algo más lenta, ya que pueden analizarse más versiones; no cambia nada una vez que se conocen.',
    nameHowVersionsPlay: 'Indicar en el nombre de las versiones cómo se reproducen',
    nameHowVersionsPlayHelp:
      'Los nombres de las versiones terminan indicando cómo las reproduce la aplicación: «(reproducción directa)», «(transmisión directa)» cuando se copia sin conversión, «(convertida: motivo)» con HDR, el límite de calidad, los subtítulos, el vídeo o el audio como motivo, o «(no se puede reproducir aquí)». Polyfin lo decide según el perfil de dispositivo que la aplicación envió en su última reproducción, así que los nombres no indican nada antes de la primera reproducción de un dispositivo ni tras un reinicio, ni para una versión que aún no se haya analizado ni descrito en RemuxDB. Los nombres están en el idioma de los nombres generados. No se lee nada para ello.',
    versionOrderGroup: 'Orden de las versiones',
    versionOrder: {
      label: 'Ordenar las versiones por',
      help: 'Cada criterio agrupa las versiones de una película o un episodio, y el siguiente las ordena dentro de cada grupo; las versiones empatadas conservan el orden de las extensiones. La primera versión es la que inicia «Reproducir», y a continuación van las versiones que se prueban cuando esta no funciona. Polyfin se basa en lo que sabe de una versión sin leer el archivo: su análisis una vez reproducida, si no, la descripción de RemuxDB, y si no, las palabras de sus etiquetas. Una versión de la que un criterio no sabe nada va detrás de las demás. El idioma de audio preferido es el de cada usuario, configurado en su aplicación, o si no el Idioma de audio predeterminado: primero van las versiones que se sabe que están en ese idioma, luego las etiquetadas como MULTI o DUAL y después las de idioma desconocido. Mientras la página de un título está abierta, las versiones que muestra conservan su posición.',
      none: 'Ninguno: las versiones conservan el orden de las extensiones.',
      add: 'Añadir un criterio',
      remove: (name: string) => `Eliminar «${name}»`,
      criterion: (position: number) => `Criterio ${position}`,
      criteria: {
        language: 'Idioma de audio preferido',
        'resolution-high': 'Resolución, la más alta primero',
        'resolution-low': 'Resolución, la más baja primero',
        'size-large': 'Tamaño de archivo, el mayor primero',
        'size-small': 'Tamaño de archivo, el menor primero',
        codec: 'Códec de vídeo, en el orden de abajo',
        'hdr-first': 'HDR y Dolby Vision primero',
        'hdr-last': 'HDR y Dolby Vision al final',
        source: 'Fuente, en el orden de abajo',
      },
    },
    versionCodecOrder: {
      label: 'Orden de los códecs de vídeo',
      help: 'El orden en el que el criterio «Códec de vídeo» coloca las versiones.',
    },
    versionSourceOrder: {
      label: 'Orden de las fuentes',
      help: 'El orden en el que el criterio «Fuente» coloca las versiones: las extensiones y carpetas del servidor que ofrecen versiones de películas y episodios. Una fuente añadida más tarde va al final hasta que la mueva, y las extensiones propias de los usuarios van detrás de las fuentes del servidor. Esto solo cambia el orden de las versiones: las descripciones, los subtítulos y los resultados de búsqueda siguen el orden de Contenido › Fuentes.',
      none: 'Ninguna fuente del servidor ofrece todavía versiones de películas o episodios.',
      dragHandle: (name: string) => `Arrastrar ${name}`,
    },
    versionCodecs: {
      av1: 'AV1',
      hevc: 'HEVC (H.265)',
      vp9: 'VP9',
      h264: 'H.264 (AVC)',
      vc1: 'VC-1',
      mpeg4: 'MPEG-4 parte 2 (Xvid, DivX)',
      mpeg2video: 'MPEG-2',
    },
    hideVersionsLarger: 'Ocultar versiones de más de (GB, 0 = sin límite)',
    hideVersionsLargerHelp: (r: RangeText) =>
      `Las versiones que se sabe que son más grandes no aparecen en ningún sitio: ni en las páginas de los títulos, ni en la reproducción, ni en las descargas. Si esta regla y las siguientes ocultaran todas las versiones de un título, no se oculta ninguna. Hasta ${r.max} GB; ${r.default} de forma predeterminada.`,
    hiddenVersionCodecs: 'Ocultar estos códecs de vídeo',
    hiddenVersionCodecsHelp:
      'Las versiones que se sabe que usan estos códecs no aparecen en ningún sitio.',
    hiddenVersionWords: 'Ocultar las versiones cuyas etiquetas contengan',
    hiddenVersionWordsHelp: (r: RangeText) =>
      `Una palabra o frase por línea, buscada sin distinguir mayúsculas y minúsculas en el nombre, la descripción y el nombre de archivo de una versión: para extensiones que indican en sus etiquetas que un archivo aún no está listo para reproducirse. Hasta ${r.max} líneas; ninguna de forma predeterminada.`,
    hiddenReleases: 'Ocultar las versiones de baja calidad',
    hiddenReleasesHelp:
      'Las versiones cuyo nombre o nombre de archivo las marca como uno de estos tipos no aparecen en ningún sitio. Una marca cuenta como palabra completa tras las palabras del propio título, sin distinguir mayúsculas y minúsculas: una película llamada Cam no es una copia de cámara, ni un archivo .ts un telesync. Ninguno de forma predeterminada.',
    releaseKinds: {
      cam: 'Copias de cámara (CAM, HDCAM)',
      telesync: 'Telesyncs (TS, HDTS, TELESYNC)',
      telecine: 'Telecines (TC, TELECINE)',
      screener: 'Screeners (SCR, DVDSCR)',
      early: 'Copias anticipadas (R5, WORKPRINT)',
    },
    syncAddonSubtitles: 'Sincronizar los subtítulos de las extensiones',
    syncAddonSubtitlesHelp:
      'Ajusta los subtítulos de las extensiones a la versión reproducida cuando van adelantados, retrasados, o más rápidos o más lentos que ella. En la primera reproducción de una versión, Polyfin detecta en segundo plano cuándo se habla: a partir de un subtítulo de texto incluido en el archivo o, si no, de tres fragmentos de su audio, leyendo como máximo 300 MB del archivo de una extensión. Las versiones torrent quedan fuera, ya que habría que descargarlas de su enjambre. A partir de entonces, un subtítulo que encaja claramente mejor desplazado o acelerado llega sincronizado, con su nombre terminado en «(sincronizado)».',
    remuxDb: 'Describir las versiones con RemuxDB',
    remuxDbHelp:
      'Muestra las pistas de audio, subtítulos y vídeo de una versión antes de su primera reproducción, tal como RemuxDB las encontró en el mismo archivo. Al abrir los detalles de un título, Polyfin consulta RemuxDB, una base de datos comunitaria, con el identificador IMDb del título, y encuentra las versiones por sus nombres de archivo. La reproducción sigue analizando cada versión.',
    remuxDbUrl: 'Dirección de RemuxDB',
    remuxDbUrlHelp: 'El servidor RemuxDB que consulta Polyfin.',
    playbackHistoryGroup: 'Historial de reproducción',
    playbackHistory: 'Guardar un historial de reproducción',
    playbackHistoryHelp:
      'Guarda cada vídeo reproducido, con quién lo reprodujo, en qué aplicación y cómo, para Sistema › Estadísticas y las estadísticas de cada usuario en Mi cuenta. Si se desactiva, no se guarda nada más; lo guardado se conserva hasta que sea demasiado antiguo.',
    playbackHistoryDays: 'Días de conservación del historial de reproducción',
    playbackHistoryDaysHelp: (r: RangeText) =>
      `Las reproducciones más antiguas se borran. Se comprueba cada día. De ${r.min} a ${r.max}, ${r.default} de forma predeterminada.`,
    defaultLanguagesGroup: 'Idiomas predeterminados',
    defaultAudioLanguage: 'Idioma de audio predeterminado',
    defaultAudioLanguageHelp:
      'Para los usuarios que no eligieron ningún idioma de audio en su aplicación, Polyfin reproduce la pista de audio en este idioma cuando una versión la tiene, y pone primero las versiones que se sabe que están en este idioma cuando las versiones se ordenan por idioma. La elección propia de un usuario siempre prevalece, y su aplicación la sigue mostrando como la dejó. Este idioma va antes que la pista que el archivo marca como predeterminada, incluso para los usuarios cuya aplicación reproduce la pista predeterminada sea cual sea su idioma, un ajuste activado mientras no lo desactiven; un archivo sin él reproduce su pista marcada. Ninguno de forma predeterminada.',
    defaultSubtitleLanguage: 'Idioma de subtítulos predeterminado',
    defaultSubtitleLanguageHelp:
      'Para los usuarios que no eligieron ningún idioma de subtítulos en su aplicación, Polyfin muestra subtítulos en este idioma según el modo de subtítulos de cada uno. La elección propia de un usuario siempre prevalece, y su aplicación la sigue mostrando como la dejó. Ninguno de forma predeterminada.',
    noDefaultLanguage: 'Ninguno',
    publicAddress: 'Dirección pública',
    publicAddressHelp:
      'La dirección en la que se abre Polyfin, como https://media.example.org. Los enlaces de las notificaciones empiezan por ella; si está vacía, los mensajes no llevan enlace. Cuando empieza por https://, «Redirigir HTTP a HTTPS», en Ajustes › Seguridad, lleva a su puerto, o al 443 si no indica ninguno.',
    email: 'Correo electrónico',
    emailHelp:
      'Los destinos de correo electrónico pasan por este servidor SMTP. Se pueden añadir una vez guardados un servidor y una dirección de remitente.',
    smtpHost: 'Servidor SMTP',
    smtpHostHelp:
      'Su nombre de host, como smtp.example.org. Si está vacío, no se pueden añadir destinos de correo electrónico.',
    smtpPort: 'Puerto SMTP',
    smtpPortHelp: 'Normalmente 587 con STARTTLS y 465 con TLS desde el inicio.',
    smtpSecurity: 'Cifrado SMTP',
    smtpSecurityHelp:
      'Cómo se cifra la conexión. Con STARTTLS, no se envía nada si el servidor no lo ofrece. Sin cifrado, la contraseña solo se envía a este equipo.',
    smtpSecurities: { starttls: 'STARTTLS', tls: 'TLS desde el inicio', none: 'Ninguno' },
    smtpUser: 'Usuario SMTP',
    smtpUserHelp: 'Déjelo vacío para enviar sin iniciar sesión.',
    smtpPassword: 'Contraseña SMTP',
    smtpPasswordHelp: 'Se guarda cifrada y no se vuelve a mostrar.',
    smtpFrom: 'Dirección del remitente',
    smtpFromHelp: 'La dirección desde la que llegan los mensajes, como polyfin@example.org.',
    smtpFromName: 'Nombre del remitente',
    smtpFromNameHelp:
      'Se muestra con la dirección. Si está vacío, se muestra el nombre del servidor.',
    weeklySummary: 'Resumen semanal',
    weeklySummaryHelp:
      'Los destinos que eligieron «Resumen semanal» en sus Eventos lo reciben una vez por semana: los destinos del servidor y de los administradores, según la semana del servidor; los demás, según su propia semana. No se envía nada en una semana sin nada que contar. Sistema › Programación lo envía ahora.',
    weeklySummaryDay: 'Enviar el resumen semanal el',
    weeklySummaryDayHelp: (day: string) => `${day} de forma predeterminada.`,
    weeklySummaryHour: 'Enviar el resumen semanal a las',
    weeklySummaryHourHelp: (hour: string) =>
      `En la zona horaria del servidor. El resumen del mes y el resumen del año también se envían a esta hora, el día 1. ${hour} de forma predeterminada.`,
    maxConversions: 'Conversiones de vídeo simultáneas (0 = sin límite)',
    maxConversionsHelp: (r: RangeText) =>
      `Convertir vídeo es el trabajo más pesado que realiza el servidor. Cuando se alcanza este número de reproducciones con el vídeo convertido (incluidos los subtítulos incrustados en la imagen), una nueva reproducción recibe una versión que no necesita conversión, o no se inicia. Las reproducciones en curso nunca se cortan. La televisión en directo también mantiene sus propios límites: 4 canales por usuario y 16 para el servidor. De ${r.min} a ${r.max}; ${r.default} de forma predeterminada.`,
    maxConversionHeight: 'Calidad máxima del vídeo convertido',
    maxConversionHeightHelp:
      'El vídeo convertido se reduce como máximo a esta altura, conservando sus proporciones, para que se reproduzca bien en una conexión más lenta. Los archivos reproducidos tal cual o simplemente reempaquetados conservan su calidad. Una tarjeta gráfica convierte hasta 4K; el procesador se detiene en 1080p y, cuando convierte HDR a SDR, en «Calidad máxima del HDR convertido por el procesador», en Conversión (transcodificación) › HDR.',
    conversionHeightOriginal: 'Original',
    conversionHeight: (height: number) => `${height}p`,
    convertedDownloads: 'Descargas convertidas',
    convertedDownloadsHelp:
      'Las aplicaciones que lo solicitan, como Streamyfin, guardan un título en la calidad que elige el usuario: Polyfin envía una copia más ligera, creada mientras la aplicación la descarga y que nunca se guarda en el servidor. Si no, guardan el archivo tal cual. Cada descarga cuenta en «Conversiones de vídeo simultáneas» mientras se convierte, y como una de las «Reproducciones simultáneas» del usuario. El usuario necesita «Puede descargar», y «Puede usar la conversión (transcodificación)» cuando se convierte el vídeo o el audio. Una descarga interrumpida vuelve a empezar desde el principio. Desactivado de forma predeterminada.',
    conversion: {
      description:
        'Cuando una aplicación no puede reproducir un archivo tal cual, Polyfin lo convierte con FFmpeg. Estos ajustes determinan cómo, tanto para películas y series como para la televisión en directo. Los valores predeterminados son adecuados para la mayoría de los servidores.',
      groups: {
        general: 'General',
        subtitles: 'Subtítulos en imagen',
        gpu: 'Tarjeta gráfica',
        video: 'Vídeo',
        hdr: 'HDR',
        interlaced: 'Vídeo entrelazado',
        audio: 'Audio',
        performance: 'Rendimiento',
      },
      subtitleOcr: 'Convertir en texto los subtítulos en imagen',
      subtitleOcrHelp:
        'Los subtítulos PGS y de DVD (VobSub) son imágenes, y muchas aplicaciones solo admiten texto: para ellas, Polyfin incrusta la pista en el vídeo, lo que requiere una conversión. Con esta opción activada, Tesseract lee una vez cada pista en imagen, en segundo plano, y crea una copia en texto que aparece como una pista más marcada «(OCR)», que se reproduce sin conversión. Hasta que la copia esté lista, la pista se incrusta como antes. La pista en imagen se conserva: se pierden las cursivas, los colores y las posiciones, y las fuentes estilizadas pueden leerse mal. «Al reproducir» lee las pistas de una versión la primera vez que la reproduce una aplicación que no puede mostrarlas; «También tras los escaneos de bibliotecas» lee además las pistas de los nuevos archivos locales. Desactivado de forma predeterminada.',
      subtitleOcrChoices: {
        off: 'Desactivado',
        played: 'Al reproducir',
        scans: 'También tras los escaneos de bibliotecas',
      },
      subtitleOcrTracks: 'Pistas de subtítulos en imagen leídas a la vez',
      subtitleOcrTracksHelp: (r: RangeText) =>
        `Cada pista la lee un proceso de Tesseract que usa un núcleo del procesador. De ${r.min} a ${r.max}; ${r.default} de forma predeterminada.`,
      hardwareAcceleration: 'Tarjeta gráfica para convertir',
      hardwareAccelerationHelp:
        'Una tarjeta gráfica (GPU) convierte el vídeo mucho más rápido que el procesador. Cada sistema ofrece las suyas: NVIDIA y VAAPI en Linux, VideoToolbox en macOS, NVIDIA, Intel y AMD en Windows. Sin una tarjeta que funcione, convierte el procesador. Los cambios se aplican al guardar, sin reiniciar.',
      vaapiDevice: 'Tarjeta gráfica para VAAPI',
      vaapiDeviceHelp:
        'El nodo de renderizado en el que VAAPI convierte el vídeo, cuando hay varias tarjetas gráficas que podrían hacerlo. De forma predeterminada, se prueba cada una por turno y se usa la primera que funcione.',
      vaapiDeviceEach: 'Cada una por turno',
      vaapiDeviceMissing: (path: string) => `${path} (no encontrado)`,
      hardware: {
        auto: 'Automática: la primera que funcione, en el orden siguiente',
        nvenc: 'NVIDIA (NVENC)',
        vaapi: 'AMD o Intel (VAAPI)',
        videotoolbox: 'VideoToolbox',
        qsv: 'Intel (QSV)',
        amf: 'AMD (AMF)',
        none: 'Ninguna: solo el procesador',
      },
      detected: 'Detectado en este servidor',
      noGpu: 'No se usa ninguna tarjeta gráfica: el procesador convierte el vídeo.',
      gpu: 'Tarjeta gráfica',
      gpuNames: {
        cuda: 'NVIDIA',
        vaapi: 'AMD o Intel (VAAPI)',
        videotoolbox: 'VideoToolbox',
        qsv: 'Intel (QSV)',
        amf: 'AMD (AMF)',
      } as Record<string, string>,
      encoders: 'Codificadores de la tarjeta',
      gpuToneMapping: 'HDR a SDR en la tarjeta',
      processorToneMappingHint:
        'En ese caso, el HDR lo convierte el procesador, hasta «Calidad máxima del HDR convertido por el procesador».',
      qualityFactor: 'Admite un valor de calidad',
      processor: 'Codificadores del procesador',
      processorToneMapping: 'HDR a SDR en el procesador',
      yes: 'Sí',
      no: 'No',
      none: 'Ninguno',
      hardwareDecoding: 'Leer estos formatos con la tarjeta gráfica',
      hardwareDecodingHelp:
        'La tarjeta lee (decodifica) estos formatos por sí misma, lo que deja libre el procesador. Desmarque uno si sus vídeos no se convierten o se ven mal: entonces lo leerá el procesador. Los demás formatos se intentan en la tarjeta. HEVC 10 bits requiere HEVC.',
      hardwareDecodingNoGpu:
        'No se usa ninguna tarjeta gráfica, así que el procesador lee todos los formatos.',
      codecs: {
        h264: 'H.264',
        hevc: 'HEVC',
        hevc_10bit: 'HEVC 10 bits',
        vp9: 'VP9',
        av1: 'AV1',
        mpeg2video: 'MPEG-2',
        vc1: 'VC-1',
      },
      encoderPreset: 'Velocidad de codificación',
      encoderPresetHelp:
        'Más lento ofrece mejor imagen con el mismo tamaño, pero requiere más potencia. Si los vídeos convertidos van a tirones, elija uno más rápido. Automático mantiene la elección de Polyfin: muy rápido en el procesador, medio en las tarjetas NVIDIA y el propio de la tarjeta en las demás.',
      presets: {
        auto: 'Automático',
        veryslow: 'Muy lento (mejor imagen)',
        slower: 'Más lento',
        slow: 'Lento',
        medium: 'Medio',
        fast: 'Rápido',
        faster: 'Más rápido',
        veryfast: 'Muy rápido',
        superfast: 'Superrápido',
        ultrafast: 'Ultrarrápido (menor carga)',
      },
      h264Quality: 'Calidad H.264 (0 = según tasa de bits)',
      hevcQuality: 'Calidad HEVC (0 = según tasa de bits)',
      qualityHelp: (r: RangeText) =>
        `Un número más bajo ofrece mejor imagen y más datos; la tasa de bits sigue marcando el máximo. 0 se ciñe solo a la tasa de bits. Jellyfin usa 23 para H.264 y 28 para HEVC. 0, o de ${r.min} a ${r.max}; ${r.default} por defecto.`,
      qualityIgnored:
        'Esta tarjeta gráfica no admite un número de calidad: sigue ciñéndose a la tasa de bits.',
      allowHevcEncoding: 'Permitir convertir a HEVC',
      allowHevcEncodingHelp:
        'HEVC necesita menos datos que H.264 para la misma imagen, pero requiere más potencia para generarlo. Si está activado, las aplicaciones que indican HEVC en primer lugar lo reciben. Si está desactivado, solo reciben HEVC las aplicaciones que no pueden reproducir H.264.',
      noHevcEncoder: 'FFmpeg no puede generar HEVC en este servidor.',
      adaptiveQuality: 'Varias calidades al convertir',
      adaptiveQualityHelp:
        'Los vídeos convertidos ofrecen también hasta dos calidades inferiores, como 720p y 480p por debajo de 1080p. Las aplicaciones que pueden cambiar de calidad bajan a ellas cuando su conexión se ralentiza, en lugar de detenerse a cargar. Una calidad que la aplicación abandona se sigue convirtiendo durante 10 segundos, para que volver a ella no empiece de cero, y todas cuentan como una sola conversión. Los vídeos copiados, la televisión en directo y la música mantienen una sola calidad. «Red local» significa lo mismo que para los PIN de restablecimiento de contraseña, a través de los proxies conocidos. Desactivado por defecto.',
      adaptiveQualities: { off: 'Desactivado', remote: 'Fuera de la red local', always: 'Siempre' },
      toneMapping: 'Convertir HDR a SDR (mapeo de tonos)',
      toneMappingHelp:
        'Mantiene correctos los colores y el brillo de los vídeos HDR en pantallas que solo muestran SDR. Si está desactivado, los vídeos HDR convertidos se ven apagados, y los vídeos Dolby Vision sin capa HDR10 no se convierten.',
      toneMappingUnavailable:
        'Ni la tarjeta gráfica ni FFmpeg pueden hacerlo en este servidor: los vídeos HDR solo se convierten si esta opción está desactivada.',
      toneMappingAlgorithm: 'Método de mapeo de tonos',
      toneMappingAlgorithmHelp:
        'Cómo se reducen las partes brillantes a SDR. Automático usa BT.2390 en tarjetas NVIDIA y Hable en el procesador. Las tarjetas Intel y VideoToolbox usan su propio método e ignoran esta opción.',
      algorithms: {
        auto: 'Automático',
        bt2390: 'BT.2390 (solo tarjetas NVIDIA)',
        hable: 'Hable',
        reinhard: 'Reinhard',
        mobius: 'Möbius',
        clip: 'Recorte',
        linear: 'Lineal',
      },
      gpuToneMappingSetting: 'Mapear tonos HDR en la tarjeta gráfica',
      gpuToneMappingSettingHelp:
        'Las tarjetas NVIDIA, las tarjetas Intel (mediante VAAPI en Linux y QSV en Windows) y VideoToolbox en macOS convierten HDR a SDR por sí mismas, hasta 4K. Desactive esta opción si sus colores no se ven bien: entonces lo hace el procesador, con menor calidad. Las tarjetas Intel solo aceptan vídeos HDR10 que incluyan la información de su pantalla de masterización, y VideoToolbox, HDR10 y HLG; el procesador convierte los demás, y Dolby Vision sin capa HDR10 solo se convierte en tarjetas NVIDIA. Las tarjetas AMD nunca lo hacen: su controlador de Linux falló o se bloqueó con cada método que probó Polyfin, y el filtro AMF de FFmpeg en Windows no mapea tonos.',
      gpuToneMappingUnavailable:
        'La tarjeta gráfica de este servidor no puede hacerlo: el procesador convierte HDR a SDR.',
      processorToneMappingHeight: 'Calidad máxima del HDR convertido por el procesador',
      processorToneMappingHeightHelp:
        'Convertir HDR a SDR supone mucho trabajo para el procesador: unas cuatro veces la propia codificación a 1080p. Automático mide este servidor al iniciarse: 1080p si convierte al menos una vez y media más rápido de lo que se reproduce el vídeo; si no, 720p, que también se usa hasta que termina la medición. Por encima de 1080p solo cuenta cuando una tarjeta gráfica codifica el vídeo: por sí solo, el procesador se detiene en 1080p.',
      toneMappingHeightAuto: 'Automático (medido al iniciar)',
      height4k: '4K',
      measuredHeight: (height: number) => `Medido: ${height}p`,
      upToHeight: (height: number) => `hasta ${height}p`,
      toneMappingPeak: 'Brillo máximo en nits (0 = el del vídeo)',
      toneMappingDesat: 'Desaturación de las luces altas (0 = desactivada)',
      toneMappingPeakHelp: (peak: RangeText, desat: RangeText) =>
        `Solo cuando el procesador convierte HDR a SDR: una tarjeta gráfica que lo haga los ignora. El brillo máximo sustituye al nivel más brillante que el vídeo indica alcanzar: 0, o de ${peak.min} a ${peak.max}; ${peak.default} por defecto. La desaturación atenúa el color de las partes muy brillantes: de ${desat.min} a ${desat.max}; ${desat.default} por defecto.`,
      processorCannotToneMap: 'FFmpeg no puede mapear tonos en el procesador de este servidor.',
      deinterlaceMethod: 'Método de desentrelazado',
      deinterlaceMethodHelp:
        'Las emisiones de televisión y los DVD suelen estar entrelazados, lo que muestra líneas en forma de peine una vez convertidos. Yadif es rápido; Bwdif es algo más nítido.',
      noBwdif: 'FFmpeg no dispone de Bwdif en este servidor.',
      deinterlacers: { yadif: 'Yadif', bwdif: 'Bwdif' },
      deinterlaceDoubleRate: 'Duplicar la frecuencia de fotogramas',
      deinterlaceDoubleRateHelp:
        'Crea una imagen a partir de cada medio fotograma, para un movimiento más fluido en deportes y programas de televisión. Solo para vídeos de hasta 30 fotogramas por segundo.',
      downmixAlgorithm: 'Mezclar a estéreo',
      downmixAlgorithmHelp:
        'Cómo se mezcla el sonido envolvente para dos altavoces. Dave750 y Modo nocturno mantienen las voces claras; RFC 7845 y AC-4 siguen estándares.',
      downmixes: {
        None: 'Mezcla propia de FFmpeg',
        Dave750: 'Dave750',
        NightmodeDialogue: 'Modo nocturno (voces más claras)',
        Rfc7845: 'RFC 7845',
        Ac4: 'AC-4',
      },
      downmixBoost: 'Volumen al mezclar a estéreo',
      downmixBoostHelp: (r: RangeText) =>
        `Las mezclas estéreo suelen sonar más bajas: el volumen se multiplica por este número. De ${r.min} a ${r.max}; ${r.default} (sin cambios) por defecto. Jellyfin usa 2.`,
      maxAudioChannels: 'Máximo de canales de audio',
      maxAudioChannelsHelp:
        'El sonido convertido conserva como máximo este número de canales, aunque la aplicación admita más.',
      audioChannels: (channels: number): string =>
        channels === 0
          ? 'Tantos como admita la aplicación'
          : channels === 1
            ? 'Mono'
            : channels === 2
              ? 'Estéreo'
              : '5.1',
      audioBitratePerChannel: 'Tasa de bits de audio por canal en kb/s (0 = automática)',
      audioBitratePerChannelHelp: (r: RangeText) =>
        `Automática ofrece las tasas de bits de Jellyfin: 128 kb/s en mono, 384 kb/s en estéreo y 640 kb/s en 5.1, menos con un límite de tasa de bits bajo. 0, o de ${r.min} a ${r.max}; ${r.default} por defecto.`,
      encodingThreads: 'Hilos del procesador por conversión (0 = automático)',
      encodingThreadsHelp: (r: RangeText) =>
        `Limita cuánto procesador usa una conversión, para dejar margen a otras tareas. De ${r.min} a ${r.max}; 0 deja que FFmpeg elija.`,
      aheadSeconds: 'Segundos preparados por adelantado',
      aheadSecondsHelp: (r: RangeText) =>
        `Polyfin prepara un vídeo como máximo este número de segundos más allá de la parte que pidió la aplicación y luego espera. Un valor mayor ayuda con fuentes lentas y mantiene ocupada su conexión, pero consume más potencia y espacio en disco cuando se deja de ver antes de tiempo. De ${r.min} a ${r.max}; ${r.default} por defecto.`,
      keepConvertedVideo: 'Conservar el vídeo convertido al cambiar la pista de audio',
      keepConvertedVideoHelp:
        'Convierte la imagen y el sonido por separado, de modo que elegir otra pista de audio en el reproductor solo prepara el nuevo sonido: la imagen ya convertida sigue reproduciéndose y la nueva pista empieza antes. La imagen se conserva 30 segundos después de que un reproductor la detenga, y mientras tanto cuenta como una conversión.',
    },
    catalogsTitle: 'Catálogos',
    catalogLimit: 'Títulos leídos por catálogo de películas y series',
    catalogLimitHelp: (r: RangeText) =>
      `Algunos catálogos son casi infinitos, así que Polyfin deja de leer un catálogo tras este número de títulos. Un número mayor muestra más títulos, pero las listas cargan más despacio y la extensión recibe más solicitudes. De ${r.min} a ${r.max}; ${r.default} por defecto.`,
    channelLimit: 'Canales leídos por catálogo de Televisión en vivo',
    channelLimitHelp: (r: RangeText) =>
      `Polyfin deja de leer un catálogo de Televisión en vivo tras este número de canales, y lee como máximo este número de programas por día para la guía. Un número mayor muestra más contenido, pero carga más despacio y la extensión recibe más solicitudes. De ${r.min} a ${r.max}; ${r.default} por defecto.`,
    skipButtons: 'Botones para omitir la introducción y los créditos',
    skipButtonsHelp:
      'Las aplicaciones ofrecen omitir introducciones, resúmenes y créditos, localizados en bases de datos comunitarias. Si se desactiva, las aplicaciones no muestran botones para omitir y no se consultan las bases de datos.',
    publicMetaDbKey: 'Clave de PublicMetaDB',
    publicMetaDbKeyHelp:
      'Opcional. Una clave API de PublicMetaDB añade PublicMetaDB como fuente de marcas para omitir.',
    theIntroDbKey: 'Clave de TheIntroDB (opcional)',
    theIntroDbKeyHelp:
      'Aumenta el límite diario de TheIntroDB e incluye sus propias aportaciones. La lectura funciona sin ella.',
    segmentSources: {
      label: 'Fuentes de marcas para omitir',
      help: 'Para cada tipo de fragmento (introducción, resumen, créditos, avance), prevalece la primera fuente activada de esta lista que lo tenga; las siguientes completan lo que falte. Las fuentes desactivadas nunca se consultan.',
      turnOn: (name: string) => `Consultar ${name}`,
      needsKey: 'Requiere una clave',
    },
    audioIntros: 'Detectar introducciones y créditos en el audio de los episodios locales',
    audioIntrosHelp:
      'Cuando se reproduce un episodio de una carpeta local sin introducción ni créditos procedentes de sus capítulos o de las fuentes anteriores, Polyfin compara en segundo plano el audio de los episodios de su temporada, y asigna a cada uno como introducción el fragmento que comparten cerca del principio, y como créditos el que comparten cerca del final. Los demás episodios de la temporada ya tienen los suyos, y el reproducido los tendrá a partir de su siguiente reproducción. Requiere los botones para omitir activados.',
    similarTitles: 'Títulos similares',
    similarTitlesHelp:
      'La página de un título muestra títulos parecidos, localizados en los catálogos de las extensiones. Si se desactiva, la lista queda vacía y las extensiones reciben menos solicitudes.',
    generateImages: 'Generar imágenes para las colecciones que no tienen',
    generateImagesHelp:
      'Una colección de una extensión sin póster, y una lista inteligente mostrada como colección, muestran los pósteres de sus primeros títulos con su nombre, generados por Polyfin una vez leídos sus títulos. Los usuarios con control parental o con géneros bloqueados no los ven. Si se desactiva, no muestran ninguna imagen. Las bibliotecas configuradas con una imagen generada la conservan.',
    tmdbKey: 'Clave API de TMDB',
    tmdbKeyHelp:
      'Opcional. Con una clave API o un token de acceso de lectura de una cuenta de TMDB, «Edit images» de jellyfin-web también busca en TMDB pósteres, imágenes de fondo y logotipos de un título, fotogramas de un episodio, pósteres de una temporada y fotos de una persona, que Polyfin transmite. Además, las personas se buscan una vez en TMDB: las que no tienen foto reciben la suya, y sus páginas muestran su biografía, sus fechas de nacimiento y de fallecimiento y su lugar de nacimiento. Los archivos de una carpeta local cuya búsqueda falla en todas las extensiones de metadatos también se buscan en TMDB. Sin clave, «Edit images» solo muestra las imágenes de las extensiones y nunca se consulta TMDB.',
    mdblistRatingsKey: 'Clave API de MDBList para valoraciones',
    mdblistRatingsKeyHelp:
      'Opcional. Con una clave de una cuenta de MDBList, las películas y series muestran su valoración de IMDb, la puntuación de la crítica y la del público según MDBList, obtenidas en segundo plano y guardadas para todos los usuarios. Las claves de MDBList propias de los usuarios para el seguimiento nunca se usan para esto. Sin clave, nunca se consulta MDBList.',
    criticRatingShows: 'La valoración de la crítica muestra',
    criticRatingShowsHelp:
      'Las aplicaciones de Jellyfin tienen un único campo para la valoración de la crítica, de 0 a 100: muestra la puntuación de la crítica o la del público. En la página de un título, el reproductor web muestra ambas, junto con la valoración de IMDb.',
    criticRatingChoices: {
      critics: 'Puntuación de la crítica',
      audience: 'Puntuación del público',
    },
    lyrics: 'Letras de canciones de LRCLIB',
    lyricsHelp:
      'Las aplicaciones muestran la letra de las canciones, sincronizada línea a línea cuando LRCLIB, una base de datos de letras gratuita, la tiene. Polyfin busca cada canción una sola vez, por su artista, título, álbum y duración, cuando empieza a reproducirse o se abren sus detalles o su letra. Si se desactiva, las canciones no tienen letra y nunca se consulta LRCLIB.',
    evenVolume: 'Igualar el volumen entre canciones',
    evenVolumeHelp:
      'Las aplicaciones suben o bajan cada canción, o cada álbum, a la misma sonoridad, según las etiquetas ReplayGain de las canciones locales o su sonoridad medida. Las canciones en sí no se modifican. Si se desactiva, las aplicaciones no reciben ninguna ganancia y no se mide nada.',
    measureLoudness: 'Medir la sonoridad de las canciones sin ganancia',
    measureLoudnessHelp:
      'FFmpeg mide la sonoridad de las canciones sin etiqueta ReplayGain, de dos en dos con la prioridad más baja, una sola vez cada una: las canciones locales tras un escaneo y las de extensiones tras su primera reproducción. Las canciones de más de 15 minutos y los audiolibros nunca se miden.',
    measureLoudnessChoices: {
      off: 'Desactivado',
      local: 'Canciones locales',
      all: 'Canciones locales y de extensiones',
    },
    playedPercent: 'Marcar como visto después de (%)',
    playedPercentHelp: (r: RangeText) =>
      `Un título se marca como visto cuando la reproducción supera este porcentaje de su duración. De ${r.min} a ${r.max}; ${r.default} por defecto, como en Jellyfin.`,
    resumePercent: 'Guardar el punto de reanudación después de (%)',
    resumePercentHelp: (r: RangeText) =>
      `El punto donde se detuvo la reproducción se guarda, para reanudar desde ahí, una vez superado este porcentaje de la duración de un título. Debe ser inferior al umbral de visto. De ${r.min} a ${r.max}; ${r.default} por defecto, como en Jellyfin. Un título de menos de 5 minutos se marca como visto en cuanto supera este punto.`,
    versionListMinutes: 'Conservar las listas de versiones durante (minutos)',
    versionListMinutesHelp: (r: RangeText) =>
      `Tiempo durante el que Polyfin usa las versiones y los subtítulos que las extensiones indican para un título antes de volver a consultarlas. Una lista más antigua se sigue mostrando al instante al volver a abrir el título, mientras Polyfin consulta. Conservar las listas más tiempo envía menos solicitudes a la extensión de transmisiones, lo que ayuda con los proveedores que rechazan demasiadas solicitudes, pero las nuevas versiones aparecen más tarde. De ${r.min} a ${r.max}; ${r.default} por defecto.`,
    catalogRefreshMinutes: 'Actualizar los catálogos después de (minutos)',
    catalogRefreshMinutesHelp: (r: RangeText) =>
      `Antigüedad máxima que puede alcanzar una página de catálogo leída de una extensión, incluida la guía de Televisión en vivo, antes de que Polyfin vuelva a leerla. Las aplicaciones nunca esperan: una página más antigua se sigue mostrando al instante mientras Polyfin la vuelve a leer en segundo plano, y se conserva un día más. Un tiempo mayor envía menos solicitudes a las extensiones, pero los nuevos títulos aparecen más tarde. De ${r.min} a ${r.max} (un día); ${r.default} por defecto.`,
    collectionReadHour: 'Leer todas las colecciones cada día',
    collectionReadHourNever: 'Nunca',
    collectionReadHourHelp:
      'Lee a esta hora todas las colecciones de las bibliotecas de colecciones del servidor, una tras otra, y después los catálogos de las listas inteligentes del servidor, para que se abran al instante. Pide muchas páginas a las extensiones: déjelo desactivado si comparte la extensión con otras personas.',
    collectionLibraryApps: 'Aplicaciones que muestran las bibliotecas de colecciones por colección',
    collectionLibraryAppsHelp:
      'Las bibliotecas de colecciones se presentan a las aplicaciones como bibliotecas de contenido mixto, para que jellyfin-web y Streamyfin les den una fila en la pantalla de inicio. A las aplicaciones marcadas se les indica que contienen colecciones, como hace la vista Colecciones de Jellyfin: las aplicaciones que muestran una biblioteca según su tipo muestran entonces sus colecciones, y los títulos de una colección al abrirla, en lugar de los títulos de todas sus colecciones juntos. jellyfin-web y Streamyfin excluyen esas bibliotecas de las filas de su pantalla de inicio, así que no las marque. La lista muestra las aplicaciones que han iniciado sesión en este servidor.',
    collectionLibraryAppsNone: 'Ninguna aplicación ha iniciado sesión en este servidor todavía.',
    localScanHours: 'Escanear las carpetas locales cada (horas, 0 = nunca)',
    localScanHoursHelp: (r: RangeText) =>
      `Frecuencia con la que se vuelve a escanear cada carpeta local en busca de archivos añadidos, modificados o eliminados. Las carpetas también se escanean al iniciar, cuando usted lo solicita y cuando cambian las vigiladas. 0 desactiva la programación. De ${r.min} a ${r.max}; ${r.default} por defecto.`,
    watchLocalFolders: 'Vigilar los cambios en carpetas y recursos compartidos',
    watchLocalFoldersHelp: {
      container:
        'Vuelve a escanear una carpeta del contenedor unos segundos después de que sus archivos dejen de cambiar, para que los archivos añadidos, renombrados o eliminados aparezcan en menos de un minuto.',
      computer:
        'Vuelve a escanear una carpeta del equipo en el que se ejecuta Polyfin unos segundos después de que sus archivos dejen de cambiar, para que los archivos añadidos, renombrados o eliminados aparezcan en menos de un minuto.',
    },
    watchSharesHelp:
      'Un recurso compartido SMB se vuelve a escanear unos segundos después de que su servidor notifique un cambio. Las carpetas WebDAV se vuelven a listar cada 2 minutos, y los buckets S3, las carpetas SFTP y los remotos de rclone con la frecuencia que pida cada carpeta (un recurso con muchas carpetas, en varias rondas), y se vuelven a escanear cuando un listado ha cambiado. Sistema › Estado del sistema indica cómo se vigila cada recurso compartido, y qué carpetas o recursos no se pueden vigilar y por qué.',
    personalAddons: 'Permitir extensiones propias de los usuarios',
    personalAddonsHelp:
      'Permite a los usuarios añadir sus propias extensiones de Stremio, además de las del servidor. Si se desactiva, sus extensiones se conservan pero no se usan, y sus aplicaciones de Jellyfin solo muestran las extensiones del servidor.',
    serverImports:
      'Los usuarios pueden importar su historial de reproducción desde otro servidor o aplicación',
    serverImportsHelp:
      'En Mi cuenta, cada usuario puede importar sus películas y episodios vistos, puntos de reanudación y favoritos desde un servidor Jellyfin o Emby, o desde su cuenta de Stremio o Nuvio, iniciando sesión allí con sus propias credenciales. Si se desactiva, esas secciones se ocultan. Un administrador puede seguir importando para un usuario, en Usuarios.',
    importEveryMinutes: 'Importar las cuentas conectadas cada',
    importEveryMinutesHelp:
      'Con qué frecuencia Polyfin vuelve a importar las cuentas de Stremio y Nuvio a las que los usuarios siguen conectados, y los historiales de reproducción que importan desde servicios de seguimiento. Un cambio se aplica de inmediato: cada siguiente importación se produce este tiempo después de la anterior. Cada importación vuelve a leer toda la cuenta, así que un intervalo corto envía más solicitudes a esos servicios y, en el caso de Stremio, cuyas condiciones prohíben la extracción automatizada, acentúa esa zona gris. 6 horas de forma predeterminada.',
    shareLinks: 'Permitir enlaces para compartir',
    shareLinksHelp:
      'Los usuarios con «Puede compartir títulos», en su página de Usuarios, pueden dar una película o un episodio a alguien sin cuenta mediante un enlace que caduca entre 1 hora y 30 días después. El invitado lo ve en un navegador, a 1080p como máximo y dentro del grupo de calidad del usuario, y cuenta entre las reproducciones simultáneas del usuario. Si está desactivado, no se puede crear ningún enlace y todos los enlaces se rechazan, sin borrarse. Desactivado de forma predeterminada.',
    importEveryChoice: (minutes: number) => (minutes < 60 ? `${minutes} min` : `${minutes / 60} h`),
    loginAttempts: 'Bloquear una cuenta tras este número de contraseñas incorrectas (0 = nunca)',
    loginAttemptsHelp: (r: RangeText) =>
      `Tras este número de contraseñas incorrectas seguidas, la cuenta no puede iniciar sesión durante 15 minutos, ni siquiera con la contraseña correcta. Un administrador puede desbloquearla antes en la página Usuarios. 0, o de ${r.min} a ${r.max}.`,
    twoStepAdministrators: 'Exigir el inicio de sesión en dos pasos a los administradores',
    twoStepAdministratorsHelp:
      'Los administradores inician sesión en esta aplicación de administración con un código de una aplicación de autenticación después de su contraseña. Quien no lo haya configurado lo hace justo después de su contraseña, antes de obtener una sesión. A partir de entonces, su contraseña sola no inicia sesión en ninguna aplicación de Jellyfin: debe usar Quick Connect, aprobado aquí. Las sesiones y los dispositivos ya conectados siguen conectados. Desactivado de forma predeterminada.',
    inactiveDeviceDays: 'Cerrar sesión en dispositivos sin usar durante (días, 0 = nunca)',
    inactiveDeviceDaysHelp: (r: RangeText) =>
      `Las aplicaciones de Jellyfin que no se usen durante este número de días cierran sesión y deben volver a iniciarla. Se comprueba cada hora. No afecta al inicio de sesión en esta página de administración. De ${r.min} a ${r.max}.`,
    knownProxies: 'Proxies conocidos',
    knownProxiesHelp: (r: RangeText) =>
      `Los proxies inversos situados delante de Polyfin, uno por línea: una dirección IP, una red como 172.18.0.0/16 o un nombre de host como el nombre de un contenedor en una red Docker (traefik), hasta ${r.max}. Un nombre se resuelve cada minuto y se confía en cada dirección a la que apunte, así que indique solo nombres que resuelva su propia red. Una solicitud de uno de ellos se atribuye al cliente que indique su cabecera X-Forwarded-For o X-Real-IP: esa dirección limita los inicios de sesión fallidos, aparece en los dispositivos e identifica la red local. Las cabeceras reenviadas desde cualquier otra dirección se ignoran. Déjelo vacío si no hay nada delante de Polyfin.`,
    https: {
      title: 'HTTPS',
      intro:
        'Polyfin puede servir HTTPS por sí mismo, en un puerto propio, sin un proxy inverso delante. HTTP sigue funcionando en su puerto pase lo que pase con HTTPS: un puerto que no se puede abrir, o un certificado que no se puede leer o renovar, aparece en Sistema › Estado del sistema. Los cambios se aplican de inmediato, sin reiniciar.',
      mode: 'Servir HTTPS',
      modeHelp:
        'Archivos de certificado: un certificado que obtiene otra herramienta, que se vuelve a leer cuando cambia. Let’s Encrypt: un certificado que Polyfin obtiene y renueva por sí mismo, para nombres de host públicos que apuntan a este servidor. Desactivado de forma predeterminada.',
      modes: { off: 'Desactivado', files: 'Archivos de certificado', letsencrypt: 'Let’s Encrypt' },
      port: 'Puerto HTTPS',
      portHelp: (r: RangeText) =>
        `El puerto TCP en el que HTTPS escucha dentro del contenedor: publíquelo, como se hace con 8096 para HTTP. El usuario del contenedor no puede escuchar por debajo de 1024: para acceder a Polyfin en el 443, redirija el 443 a este puerto en el router o en los puertos del contenedor, y establezca la dirección pública, en Ajustes › Notificaciones, en https:// y el nombre de host, sin puerto. De ${r.min} a ${r.max}; ${r.default} de forma predeterminada, como Jellyfin.`,
      redirect: 'Redirigir HTTP a HTTPS',
      redirectHelp:
        'Las solicitudes desde internet que llegan por HTTP reciben una redirección permanente (308) a la misma dirección por HTTPS, en el puerto por el que internet llega a HTTPS: el puerto de la dirección pública, en Ajustes › Notificaciones, si empieza por https://; el 443 si no indica ninguno; y, si no, el puerto HTTPS. La red local, las comprobaciones de Let’s Encrypt y las solicitudes que un proxy conocido recibió por HTTPS se quedan en HTTP. Solo se aplica mientras se sirve HTTPS. Desactivado de forma predeterminada.',
      certificate: 'Certificado',
      certificateHelp:
        'La ruta dentro del contenedor del archivo PEM del certificado con su cadena, como /certs/fullchain.pem. Al guardar se comprueba que se puede leer. Polyfin lo vuelve a leer en el minuto siguiente a un cambio y conserva el último certificado válido mientras no se pueda leer un cambio.',
      privateKey: 'Clave privada',
      privateKeyHelp:
        'La ruta dentro del contenedor de la clave privada RSA o ECDSA del certificado, como archivo PEM, por ejemplo /certs/privkey.pem. Al guardar se comprueba que corresponde al certificado.',
      hosts: 'Nombres de host',
      hostsHelp: (r: RangeText) =>
        `Los nombres públicos que apuntan a este servidor, uno por línea, como media.example.org, hasta ${r.max}. Un solo certificado los incluye todos. Los nombres que solo se resuelven en la red local, y los comodines, requieren el desafío DNS-01, al que Polyfin no responde: para ellos, use archivos de certificado de otra herramienta.`,
      email: 'Correo electrónico',
      emailHelp: 'Opcional. Let’s Encrypt puede escribir a esta dirección sobre la cuenta.',
      terms: 'Acepto las condiciones del servicio de Let’s Encrypt',
      termsHelp:
        'Let’s Encrypt solo emite certificados una vez aceptadas sus condiciones del servicio, en letsencrypt.org/repository.',
      challenge: 'Desafío',
      challengeHelp:
        'Cómo comprueba Let’s Encrypt que los nombres apuntan a este servidor. HTTP-01: Polyfin responde en su puerto HTTP, al que debe llegar el puerto 80 de internet. TLS-ALPN-01: Polyfin responde en su puerto HTTPS, al que debe llegar el puerto 443 de internet. El certificado, su clave y la clave de la cuenta se guardan en la base de datos, con las claves cifradas mediante POLYFIN_SECRET_KEY, y el certificado se renueva cuando le queda un tercio de su vigencia.',
      challenges: {
        'http-01': 'HTTP-01 (puerto 80 al puerto HTTP)',
        'tls-alpn-01': 'TLS-ALPN-01 (puerto 443 al puerto HTTPS)',
      },
      useAddress: (address: string) => `Usar ${address}`,
      offeredAddress:
        'Las direcciones en las que Let’s Encrypt sirve HTTPS, en Ajustes › Seguridad: sin puerto una vez que el router redirige el 443 al puerto HTTPS. «Redirigir HTTP a HTTPS» lleva al puerto de la dirección elegida.',
    },
    detailedLog: 'Registro detallado (para diagnosticar un problema)',
    detailedLogHelp:
      'Polyfin escribe mucho más en su registro, de inmediato y sin reiniciar. Desactívelo cuando encuentre el problema.',
    saved: 'Configuración guardada.',
    thumbnailsHelp:
      'Imágenes extraídas de los propios títulos, después de verlos. Para crearlas, Polyfin lee pequeñas partes del archivo desde la fuente, a un ritmo moderado, cuando nadie está reproduciendo desde esa fuente: como máximo 60 solicitudes por título, una cada 3 segundos, y 120 por hora y fuente, nunca el archivo completo. Una fuente que pide a Polyfin que vaya más despacio no recibe solicitudes de imágenes durante 2 horas. Desactivado de forma predeterminada.',
    trickplay: 'Miniaturas al desplazarse por un título',
    trickplayHelp:
      'Las aplicaciones muestran una pequeña imagen del momento al que se desplaza el usuario en la barra de reproducción. Polyfin las crea una vez visto un título, y para el siguiente episodio cuando la reproducción se prepara por adelantado. Las películas largas tienen miniaturas más espaciadas: cada pocos minutos en lugar de cada pocos segundos.',
    trickplayInterval: 'Una miniatura cada (segundos, como mínimo)',
    trickplayIntervalHelp: (r: RangeText) =>
      `Con qué frecuencia cambia la imagen de la barra de reproducción. Polyfin lee como máximo 60 imágenes por título, así que un título de más de unos 10 minutos tiene una imagen por intervalo, con los intervalos repartidos por igual a lo largo de su duración: aproximadamente 1 minuto para un episodio de 1 hora, 2 minutos para una película de 2 horas. De ${r.min} a ${r.max}; ${r.default} de forma predeterminada, como Jellyfin.`,
    trickplayWidth: 'Ancho de las miniaturas',
    trickplayWidthHelp:
      'Las miniaturas más anchas se ven más nítidas en pantallas grandes, pero ocupan más espacio. Al cambiarlo se crean nuevas miniaturas la próxima vez que se reproduzca un título.',
    pixels: (width: number) => `${width} píxeles`,
    chapterImages: 'Imágenes de capítulos',
    chapterImagesHelp:
      'Las aplicaciones muestran una imagen para cada capítulo de un título, en su lista de escenas. Polyfin usa la imagen leída más cercana al inicio de cada capítulo, de las mismas lecturas que las miniaturas, una vez visto el título.',
    thumbnailStorage: 'Espacio para imágenes (GB)',
    thumbnailStorageHelp: (r: RangeText) =>
      `Las miniaturas y las imágenes de capítulos se guardan en la base de datos. Por encima de este tamaño, se eliminan las imágenes de los títulos vistos hace más tiempo; se vuelven a crear si el título se reproduce de nuevo. De ${r.min} a ${r.max}; ${r.default} de forma predeterminada.`,
    sourceProxy: 'Proxy SOCKS5 para los flujos de los complementos',
    sourceProxyHelp:
      'Los complementos Stremio del servidor que actives en Fuentes leen sus flujos a través de este proxy, por ejemplo WARP, sin enviar por él catálogos, metadatos, IPTV ni archivos locales. Una URL como socks5h://warp:1080; vacío deja todos los complementos directos. Una reproducción en curso conserva el proxy con el que empezó.',
    sourceProxyDetect: 'Detectar WARP',
    sourceProxyDetecting: 'Detectando…',
    sourceProxyDetected: (url: string) => `WARP detectado: ${url}`,
    sourceProxyUse: 'Usar',
    sourceProxyNotFound:
      'No se encontró WARP en esta red; introduce su dirección SOCKS5 manualmente.',
    cacheSize: 'Espacio en disco para archivos en lectura (GB)',
    cacheSizeHelp: (r: RangeText) =>
      `Espacio que Polyfin puede usar para conservar las partes de los archivos que se están reproduciendo, para que los saltos y reinicios no vuelvan a descargarlas. Las partes leídas en los últimos 30 segundos se conservan incluso por encima de este límite. De ${r.min} a ${r.max} GB; ${r.default} de forma predeterminada.`,
    workFolders: {
      conversionFolder: 'Carpeta de conversión',
      conversionFolderHelp:
        'Donde se escriben los segmentos de los archivos convertidos o reempaquetados mientras se reproducen: las escrituras más intensas, para las que conviene un disco rápido o una carpeta en memoria. Necesita espacio para los segmentos de todas las conversiones en curso.',
      readingFolder: 'Carpeta de archivos en lectura',
      readingFolderHelp:
        'Donde se conservan las partes de los archivos de las extensiones y las piezas de los torrents que se están reproduciendo, dentro del espacio en disco de arriba: a menudo varios GB, para los que conviene un disco grande.',
      cacheFolder: 'Carpeta de caché',
      cacheFolderHelp:
        'Donde se conservan las imágenes retransmitidas, hasta 1 GB, de un inicio al siguiente, junto con las guías y los subtítulos en imagen que se están leyendo.',
      where: {
        container:
          'Una carpeta del contenedor de Polyfin, donde se monta un disco o una carpeta en memoria; vacía, la caché de su volumen de datos, que se muestra. Polyfin debe poder escribir en ella. Una carpeta nueva se aplica cuando Polyfin vuelve a iniciarse; la anterior se deja como está.',
        computer:
          'Una carpeta del equipo donde se ejecuta Polyfin; vacía, la caché de su carpeta de datos, que se muestra. Polyfin debe poder escribir en ella. Una carpeta nueva se aplica cuando Polyfin vuelve a iniciarse; la anterior se deja como está.',
      },
    },
    recordingsTitle: 'Grabaciones',
    recordingsFolder: (folder: string) => `Las grabaciones se guardan en ${folder}.`,
    recording: 'Grabar Televisión en vivo',
    recordingHelp:
      'Permite a los usuarios programar grabaciones de programas y series de televisión en directo, que se guardan en la carpeta siguiente.',
    recordingsFolderLabel: 'Carpeta de grabaciones',
    recordingsFolderHelp: {
      container:
        'Una carpeta del contenedor de Polyfin, la que se muestra si está vacío, en su volumen de datos. Para guardar las grabaciones en otro disco, móntelo ahí, o en otro lugar e introduzca aquí su ruta. Polyfin debe poder escribir en ella. Cambiarla no mueve las grabaciones ya hechas.',
      computer:
        'Una carpeta del ordenador en el que se ejecuta Polyfin, la que se muestra si está vacío, en su carpeta de datos. Para guardar las grabaciones en otro disco, introduzca aquí una carpeta de ese disco. Polyfin debe poder escribir en ella. Cambiarla no mueve las grabaciones ya hechas.',
    },
    recordingPrePadding: 'Empezar las grabaciones antes (minutos)',
    recordingPrePaddingHelp: (r: RangeText) =>
      `Cuántos minutos antes del programa empieza una grabación. De ${r.min} a ${r.max}; ${r.default} de forma predeterminada.`,
    recordingPostPadding: 'Seguir grabando tras el final (minutos)',
    recordingPostPaddingHelp: (r: RangeText) =>
      `Cuántos minutos después del programa continúa una grabación. De ${r.min} a ${r.max}; ${r.default} de forma predeterminada.`,
    recordingRetentionDays: 'Conservar las grabaciones durante (días, 0 = siempre)',
    recordingRetentionDaysHelp: (r: RangeText) =>
      `Las grabaciones más antiguas se borran. Se comprueba cada día. De ${r.min} a ${r.max}.`,
    recordingCommercials: 'Detectar anuncios en las grabaciones',
    recordingCommercialsHelp:
      'Cuando termina una grabación, Polyfin la lee en segundo plano, una grabación cada vez, en busca de los momentos que son a la vez negros y silenciosos, y toma los fragmentos cortos entre ellos como pausas publicitarias. Las aplicaciones ofrecen entonces saltarlos, según lo que haya elegido cada usuario en los ajustes de reproducción de su aplicación, mientras los botones de saltar la introducción y los créditos estén activados. Las grabaciones de más de 4 horas no se leen.',
    convertRecordings: 'Convertir las grabaciones terminadas a HEVC',
    convertRecordingsHelp:
      'Las grabaciones terminadas se convierten a HEVC, una cada vez, en el horario indicado abajo, para que ocupen menos espacio. La imagen conserva su tamaño; el audio y los subtítulos se copian tal cual. La conversión usa la «Tarjeta gráfica para convertir», con la «Calidad HEVC» y el «Método de desentrelazado» de Conversión (transcodificación), y cuenta como una de las «Conversiones de vídeo simultáneas»: espera mientras las reproducciones las ocupen todas. Solo empieza si hay espacio libre para una copia tan grande como la grabación. La copia sustituye a la grabación solo si dura lo mismo, contiene las mismas pistas, se decodifica hasta el final y es más pequeña; si no, se borra, y la grabación se conserva tal cual y no se vuelve a intentar. Una grabación que alguien está reproduciendo se sustituye cuando deja de reproducirla; se mantienen su estado de visto, sus puntos de reanudación y sus anuncios. Las aplicaciones que no reproducen HEVC reciben la grabación convertida al reproducirla: con «Conversión (transcodificación)» desactivada, no pueden reproducirla. Desactivado de forma predeterminada.',
    convertRecordingsBetween: 'Convertir entre las',
    convertRecordingsAnd: 'y las',
    convertRecordingsBetweenHelp: (from: string, until: string) =>
      `En la zona horaria del servidor. Una conversión que siga en curso al final se detiene y vuelve a empezar desde el principio la noche siguiente. De ${from} a ${until} de forma predeterminada.`,
    convertRecordingsFrom: 'Inicio de las conversiones',
    convertRecordingsUntil: 'Fin de las conversiones',
    convertRecordingsOnProcessor: 'Usar el procesador si no hay tarjeta gráfica',
    convertRecordingsOnProcessorHelp:
      'Sin una tarjeta gráfica que codifique HEVC, las grabaciones se convierten en el procesador con x265, con su prioridad más baja. Es lento y mantiene ocupado el procesador durante el horario indicado arriba. Sin tarjeta gráfica y con esta opción desactivada, no se convierte nada. Desactivado de forma predeterminada.',
    liveTvRefreshHours: 'Actualizar las listas y guías de Televisión en vivo cada (horas)',
    liveTvRefreshHoursHelp: (r: RangeText) =>
      `Con qué frecuencia se vuelven a descargar las listas de canales IPTV y las guías de programación XMLTV. Una descarga que ha fallado se reintenta antes: a los 5 minutos, a los 15 minutos y después cada hora. De ${r.min} a ${r.max}; ${r.default} de forma predeterminada.`,
    backupsFolder: (folder: string) =>
      `Las copias de seguridad de la base de datos se guardan en ${folder}.`,
    backups: 'Hacer una copia de seguridad de la base de datos cada día',
    backupsHelp:
      'Escribe cada día una copia de la base de datos en la carpeta indicada abajo, a la hora elegida, y borra las más antiguas que superen el número conservado.',
    backupFolderLabel: 'Carpeta de copias de seguridad',
    backupFolderHelp: {
      container:
        'Una carpeta del contenedor de Polyfin, la que se muestra si está vacío, en su volumen de datos. Las copias de seguridad están más seguras en un disco distinto del de la base de datos: monte uno ahí, o en otro lugar e introduzca aquí su ruta. Polyfin debe poder escribir en ella.',
      computer:
        'Una carpeta del ordenador en el que se ejecuta Polyfin, la que se muestra si está vacío, en su carpeta de datos. Las copias de seguridad están más seguras en un disco distinto del de la base de datos: introduzca aquí una carpeta de uno. Polyfin debe poder escribir en ella.',
    },
    backupHour: 'Hacer la copia de seguridad cada día a las',
    backupHourHelp: (hour: string) =>
      `En la zona horaria del servidor. ${hour} de forma predeterminada.`,
    backupsKept: 'Copias de seguridad que conservar',
    backupsKeptHelp: (r: RangeText) =>
      `Tras cada copia de seguridad, Polyfin borra sus copias más antiguas que superen este número. Los demás archivos de la carpeta nunca se tocan. De ${r.min} a ${r.max}; ${r.default} de forma predeterminada.`,
    lastBackup: 'Última copia de seguridad',
    backupDestinations: 'Destinos',
    backupDestinationsHelp:
      'Cada copia de seguridad se envía también a estos lugares, con el mismo nombre de archivo, una vez escrita en la carpeta: almacenamiento compatible con S3, servidores SFTP o WebDAV. Una subida fallida se reintenta en la siguiente copia de seguridad.',
    webPlayerHelp:
      'Lo que el reproductor web (jellyfin-web, en /web/) muestra además de sus propias páginas. El script se aplica la próxima vez que se cargue una página del reproductor web; el reproductor web conserva el CSS y el mensaje de inicio de sesión hasta un minuto.',
    openWebPlayer: 'Abrir el reproductor web',
    webTheme: 'Tema',
    webThemeHelp:
      'El aspecto del reproductor web. LumaaGlaass es el tema que incluye Polyfin. Ninguno mantiene el aspecto propio de jellyfin-web, cuyo tema de visualización elige cada usuario en Ajustes › Visualización. Personalizado usa su propio CSS y JavaScript de abajo, que se conservan mientras se elige otro tema.',
    webThemeChoices: { lumaaglaass: 'LumaaGlaass', none: 'Ninguno', custom: 'Personalizado' },
    webThemeReload:
      'El reproductor web conserva el CSS personalizado que leyó durante un minuto como máximo: tras un cambio, vuelva a cargar sus páginas abiertas un minuto después.',
    webThemeTexts: {
      lumaaglaass:
        'Lo que Polyfin sirve para LumaaGlaass, mostrado aquí sin poder editarse. Elija Personalizado, o «Editar como personalizado», para escribir el suyo.',
      none: 'Ninguno no sirve CSS ni JavaScript personalizado. Elija Personalizado para escribir el suyo.',
    },
    editAsCustom: 'Editar como personalizado',
    editAsCustomTitle: '¿Reemplazar sus textos personalizados?',
    editAsCustomConfirm:
      'Su CSS y JavaScript personalizados se reemplazan por los de LumaaGlaass, y el tema pasa a Personalizado. Guardar la página conserva el cambio.',
    themeCopy: 'Guardar una copia en Polyfin',
    themeCopyHelp: (depth: number, file: string, total: string, files: number) =>
      `Guarda las hojas de estilos, scripts, fuentes e imágenes que los textos personalizados cargan desde otro sitio, como un tema en un CDN, en la carpeta de datos de Polyfin, y hace que los textos los carguen desde Polyfin: el tema funciona entonces sin llegar a su CDN. Sigue @import, url() y las direcciones que nombran los scripts, ${depth} niveles hacia abajo, solo desde direcciones de internet, hasta ${file} por archivo, y ${total} y ${files} archivos en total.`,
    themeCopySaved: (files: number, size: string, date: string) =>
      `Una copia de ${files === 1 ? 'un archivo' : `${files} archivos`}, ${size}, guardada el ${date}.`,
    themeCopyFiles: 'Archivos guardados',
    themeCopyUnsaved: (count: number) =>
      count === 1
        ? 'No se pudo guardar una dirección; se sigue cargando desde otro sitio:'
        : `No se pudieron guardar ${count} direcciones; se siguen cargando desde otro sitio:`,
    themeCopyMissing: (count: number) =>
      count === 1
        ? 'Falta un archivo de la copia en la carpeta de datos de Polyfin, como tras restaurar una copia de seguridad en otro sistema. Mientras tanto, los navegadores lo cargan desde su dirección original, lo que requiere internet. Actualice la copia para descargarlo de nuevo.'
        : `Faltan ${count} archivos de la copia en la carpeta de datos de Polyfin, como tras restaurar una copia de seguridad en otro sistema. Mientras tanto, los navegadores los cargan desde sus direcciones originales, lo que requiere internet. Actualice la copia para descargarlos de nuevo.`,
    themeCopyOutside: (count: number) =>
      count === 1
        ? 'Los textos personalizados cargan una dirección desde otro sitio:'
        : `Los textos personalizados cargan ${count} direcciones desde otro sitio:`,
    themeCopyStillOutside: (count: number) =>
      count === 1
        ? 'Una dirección añadida después de la copia se carga desde otro sitio; actualice la copia para guardarla también:'
        : `${count} direcciones añadidas después de la copia se cargan desde otro sitio; actualice la copia para guardarlas también:`,
    themeCopyNothing: 'Los textos personalizados no cargan nada desde otro sitio.',
    themeCopyKeep: 'Guardar una copia en Polyfin',
    themeCopyUpdate: 'Actualizar la copia',
    themeCopyRemove: 'Quitar la copia',
    themeCopyRemoveTitle: '¿Quitar la copia?',
    themeCopyRemoveConfirm:
      'Sus archivos se borran, y los textos personalizados vuelven a cargarlos desde sus direcciones originales.',
    themeCopyKept:
      'Copia guardada. Vuelva a cargar las páginas abiertas del reproductor web dentro de un minuto.',
    themeCopyRemoved:
      'Copia quitada. Vuelva a cargar las páginas abiertas del reproductor web dentro de un minuto.',
    themeCopyFailed: 'No se pudo guardar ningún archivo, así que no se hizo ninguna copia:',
    themeCopySaveFirst:
      'Guarde primero sus cambios: la copia se hace a partir de los textos guardados.',
    themeCopyKinds: {
      stylesheet: 'hoja de estilos',
      script: 'script',
      font: 'fuente',
      image: 'imagen',
    },
    themeCopyReasons: {
      private_address: 'en una red local, o redirigida a ella',
      not_http: 'redirigida a una dirección que no es http ni https',
      unavailable: 'no disponible',
      timeout: 'tardó demasiado',
      wrong_type: 'no es un archivo del tipo con que se carga',
      too_large: 'mayor que el límite de un archivo',
      total_too_large: 'más allá del límite de toda la copia',
      too_many_files: 'más allá del número de archivos permitido',
      too_deep: 'nombrada demasiados niveles más abajo',
      built_at_run_time: 'la completa un script al ejecutarse, algo que ninguna copia puede seguir',
    },
    customCss: 'CSS personalizado',
    customCssHelp:
      'Se aplica a todas las páginas del reproductor web, para todos los usuarios, como el CSS personalizado de Jellyfin. Los usuarios pueden desactivarlo en Ajustes › Visualización.',
    customJs: 'JavaScript personalizado',
    customJsHelp:
      'Lo carga cada página del reproductor web, después de sus propios scripts. Vacío, no carga nada.',
    customJsWarningTitle: 'Pegue solo código de confianza',
    customJsWarning:
      'Este script se ejecuta en el navegador de cada usuario que abre el reproductor web de este servidor, con su cuenta. Puede leer lo que ve y actuar en su nombre.',
    loginDisclaimer: 'Mensaje de inicio de sesión',
    loginDisclaimerHelp:
      'Se muestra bajo el formulario de inicio de sesión del reproductor web. Texto sin formato, Markdown o HTML; el reproductor web elimina el HTML no seguro.',
    addVersionsToOpenPage: 'Añadir versiones a la página abierta de un título',
    addVersionsToOpenPageHelp:
      'Añade las versiones que encuentran las extensiones al menú de versiones de la página de una película o un episodio mientras está abierta, manteniendo la versión elegida. Si está desactivado, una página abierta antes de encontrarse ninguna versión sigue mostrando las primeras a medida que llegan, y sus botones de reproducción siguen esperando una versión; las demás aparecen al volver a abrir la página.',
    homeRows: 'Filas de inicio',
    homeRowsHelp:
      'Filas que el reproductor web añade a su página de inicio, en su lugar, para todos los usuarios o para los usuarios elegidos. Cada usuario puede desactivar una fila en Mi cuenta › Filas de inicio; las secciones propias del reproductor web siguen en sus Ajustes › Inicio. Cada fila omite lo que el usuario no puede ver: los títulos por encima de su control parental, los géneros bloqueados, los títulos ocultos y las bibliotecas que tiene ocultas.',
    homeRow: {
      loading: 'Cargando las filas de inicio…',
      empty: 'Ninguna fila de inicio',
      emptyHint: 'La página de inicio del reproductor web solo muestra sus propias secciones.',
      add: 'Añadir una fila',
      addTitle: 'Añadir una fila de inicio',
      editTitle: (name: string) => `Editar ${name}`,
      edit: 'Editar',
      delete: 'Borrar',
      deleteTitle: (name: string) => `¿Borrar ${name}?`,
      deleteBody:
        'La fila desaparece de la página de inicio de todos los usuarios, y su biblioteca desaparece de las demás aplicaciones.',
      deleted: (name: string) => `Se ha borrado ${name}.`,
      saved: (name: string) => `Se ha guardado ${name}.`,
      create: 'Añadir',
      creating: 'Añadiendo…',
      save: 'Guardar',
      saving: 'Guardando…',
      name: 'Nombre',
      nameHelp: 'El título de la fila en la página de inicio.',
      becauseNameHelp:
        'Cada fila lleva este nombre, seguido del título al que corresponde: «Porque ha visto Película de ejemplo».',
      kind: 'Tipo',
      kinds: {
        catalog: 'Un catálogo',
        collection: 'Una colección',
        because: 'Porque ha visto',
        new: 'Novedades en sus bibliotecas',
      },
      kindHelp: {
        catalog:
          'La primera página de un catálogo de las extensiones del servidor, sea o no una biblioteca, como uno de tendencias. Los usuarios que no usan las extensiones del servidor, o para quienes su biblioteca está oculta, no la ven.',
        collection:
          'Los títulos de una colección: una creada por los usuarios, una lista inteligente mostrada como colección o una colección de una extensión.',
        because:
          'Una fila por cada una de las últimas películas y series que reprodujo el usuario, con los títulos similares, tal como los ofrecen las recomendaciones de películas. Ninguna mientras «Títulos similares» esté desactivado.',
        new: 'Los títulos que llegaron a los catálogos de las bibliotecas del usuario en los últimos 7 días, de más reciente a más antiguo. Polyfin los cuenta desde la primera vez que vio cada título en cada catálogo.',
      },
      defaultNames: {
        because: 'Porque ha visto',
        new: 'Novedades en sus bibliotecas',
      },
      catalog: 'Catálogo',
      noCatalog: 'Las extensiones del servidor no tienen ningún catálogo que mostrar.',
      collection: 'Colección',
      noCollection: 'Todavía no hay ninguna colección.',
      collectionSources: {
        users: 'Creada por los usuarios',
        smart: 'Lista inteligente',
        addon: 'Extensión',
      },
      choose: 'Elegir…',
      unknownCatalog: 'Un catálogo que ya no está disponible',
      unknownCollection: 'Una colección que ya no está disponible',
      count: 'Filas',
      countHelp:
        'Cuántas de las últimas películas y series reproducidas tienen una fila, de 1 a 10.',
      place: 'Posición',
      places: {
        top: 'Arriba del todo',
        resume: 'Después de «Seguir viendo»',
        nextup: 'Después de «A continuación»',
        bottom: 'Abajo del todo',
      },
      placeHelp:
        'Dónde la muestra el reproductor web. Cuando la página de inicio de un usuario no muestra esa sección, la fila ocupa la posición anterior.',
      shownTo: 'Visible para',
      everyone: 'Todos los usuarios',
      chosenUsers: 'Usuarios elegidos',
      users: 'Usuarios',
      noUser: 'Elija al menos un usuario.',
      shownToUsers: (count: number) => (count === 1 ? '1 usuario' : `${count} usuarios`),
      rowsCount: (count: number) => (count === 1 ? '1 fila' : `${count} filas`),
      library: 'También una biblioteca en otras aplicaciones',
      libraryHelp:
        'No se puede dar a otras aplicaciones un nuevo tipo de sección de inicio: esto convierte la fila también en una biblioteca, sin tipo, que muestran entre las bibliotecas del usuario, con una fila de sus últimos títulos en su inicio.',
      libraryBadge: 'Biblioteca',
      moveLabel: 'Mover',
    },
    codeSize: (used: number, max: number) =>
      `${used} KB de ${max >= 1024 ? `${max / 1024} MB` : `${max} KB`}`,
    codeKeys: 'Tab inserta espacios; pulse Escape y luego Tab para salir del campo.',
  },
  settingsPage: {
    searchHint: 'Por nombre o descripción.',
    noMatch: 'Ningún ajuste coincide con esta búsqueda.',
    sections: {
      variables: 'Variables de entorno',
    },
    variablesHelp:
      'Solo lectura. Se definen en el contenedor (entorno de Docker, archivo compose o plantilla de Unraid) y se aplican al iniciarse Polyfin: modifíquelas allí y reinicie el contenedor. Las variables que antes definían otras opciones se copian una sola vez en los ajustes, en el primer inicio de esta versión, y ya no se leen: modifíquelas en los ajustes. Los secretos se ocultan, y la dirección de la base de datos solo muestra su host y el nombre de la base de datos.',
    value: 'Valor en vigor',
    defaultValue: 'Predeterminado',
    setValue: 'Definida',
    hidden: 'Oculta',
    empty: 'Vacía',
    notRead: 'Polyfin no la lee',
    searchPlaceholder: 'Buscar un ajuste',
    results: (count: number) =>
      count === 1 ? '1 ajuste encontrado' : `${count} ajustes encontrados`,
    noMatchHelp: 'Pruebe con otra palabra, como el nombre de una variable o de un servicio.',
    variableHint: 'Variable de entorno',
    ledes: {
      general:
        'El nombre del servidor, su idioma, cómo inician sesión las aplicaciones y cómo lo encuentran, y la búsqueda de nuevas versiones.',
      playback: 'Cómo elige y prepara Polyfin las versiones que reproduce.',
      content:
        'Botones para saltar la introducción, títulos similares, imágenes generadas, valoraciones, letras de canciones, volumen uniforme entre canciones y umbrales de reproducción.',
      catalogs: 'Cuánto lee Polyfin de los catálogos de las extensiones, y con qué frecuencia.',
      security:
        'Extensiones e importaciones propias de los usuarios, cuentas bloqueadas, dispositivos sin usar y proxies inversos.',
      liveTv: 'Con qué frecuencia se vuelven a descargar las listas de canales y las guías IPTV.',
      diagnostics: 'El registro detallado y las variables de entorno en vigor.',
      notifications:
        'Dónde avisa Polyfin, para cada usuario, de nuevos episodios, grabaciones, reproducciones y conexiones que se detienen, y de problemas del estado del sistema, nuevos usuarios, nuevas versiones y cuentas bloqueadas, y dónde envía el resumen semanal y los resúmenes del mes y del año.',
    },
    groups: {
      skip: 'Saltar introducción y créditos',
      titlePages: 'Páginas de títulos',
      images: 'Imágenes',
      ratings: 'Valoraciones',
      music: 'Música',
      thresholds: 'Umbrales de reproducción',
    },
    leaveTitle: '¿Salir sin guardar?',
    leaveBody:
      'Los cambios realizados en esta sección aún no se han guardado. Se perderán si sale.',
    leave: 'Salir sin guardar',
    stay: 'Quedarse',
    variablesEmpty: 'No hay variables que mostrar',
    variablesEmptyHelp: 'Defina variables POLYFIN_ en el contenedor y reinícielo.',
  },
}

export default settings
