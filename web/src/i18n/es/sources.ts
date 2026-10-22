import type en from '../en/sources'

const sources: typeof en = {
  sources: {
    title: 'Fuentes',
    lede: 'Extensiones de Stremio, extensiones de Eclipse, fuentes IPTV y carpetas locales en una sola lista. Las fuentes del servidor sirven a todos los usuarios; las fuentes propias de un miembro solo le sirven a él.',
    add: 'Añadir una fuente',
    listLabel: 'Fuentes',
    listNote: 'Las fuentes propias de los miembros solo sirven a su propietario.',
    loading: 'Cargando las fuentes',
    kindFilter: 'Filtrar por tipo',
    all: 'Todas',
    kinds: { stremio: 'Stremio', eclipse: 'Eclipse', iptv: 'IPTV', local: 'Carpetas' },
    owner: 'Propietario',
    everyOwner: 'Todos los propietarios',
    server: 'El servidor',
    search: 'Buscar en las fuentes',
    searchPlaceholder: 'Buscar por nombre',
    clearFilters: 'Borrar los filtros',
    emptyTitle: 'Todavía no hay fuentes',
    emptyShared:
      'Añada una extensión de Stremio, una extensión de Eclipse, una fuente IPTV o una carpeta local: servirá a todos los usuarios de este servidor.',
    emptyMine:
      'Añada una extensión de Stremio, una extensión de Eclipse o una fuente IPTV: solo le servirá a usted, después de las del servidor.',
    noMatchTitle: 'Ninguna fuente coincide',
    noMatch: 'Cambie el tipo, el propietario o la búsqueda.',
    tag: {
      stremio: 'Stremio',
      eclipse: 'Eclipse',
      m3u: 'M3U',
      xtream: 'Xtream',
      hdhomerun: 'HDHomeRun',
      local: 'Carpeta',
    },
    kindName: {
      stremio: 'Extensión de Stremio',
      eclipse: 'Extensión de Eclipse',
      m3u: 'Lista de reproducción M3U',
      xtream: 'Cuenta de Xtream Codes',
      hdhomerun: 'Sintonizador HDHomeRun',
      local: 'Carpeta local',
    },
    ownedBy: (name: string) => `Fuente de ${name}`,
    yours: 'Su propia fuente',
    active: 'Activa',
    off: 'Desactivada',
    failed: 'Error en la descarga',
    channelsShown: (count: string) => `${count} canales mostrados`,
    notDownloaded: 'Todavía no se ha descargado',
    moreActions: (name: string) => `Más acciones para ${name}`,
    openPage: 'Abrir su página',
    enabled: 'Activada',
    enabledHelp:
      'Desactivar una fuente oculta sus bibliotecas en las aplicaciones de Jellyfin sin perderlas.',
    order: 'Orden',
    position: (position: number, count: number) => `${position} de ${count}`,
    moveUp: (name: string) => `Subir ${name}`,
    moveDown: (name: string) => `Bajar ${name}`,
    moved: (name: string, position: number, count: number) =>
      `${name} se ha movido a la posición ${position} de ${count}.`,
    description: 'Descripción',
    details: 'Detalles',
    manifestUrl: 'Dirección del manifiesto',
    address: 'Dirección',
    version: 'Versión',
    provides: 'Proporciona',
    types: 'Tipos',
    catalogs: 'Catálogos',
    catalogCount: (count: number) => (count === 1 ? '1 catálogo' : `${count} catálogos`),
    lastRefresh: 'Última actualización',
    channels: 'Canales',
    notImported: 'No importados',
    shownOf: (shown: string, total: string) => `${shown} mostrados de ${total}`,
    vod: 'Películas y series',
    vodShort: (movies: string, series: string) =>
      `${movies} películas y ${series} series importadas`,
    lastFetch: 'Última descarga',
    nextFetch: 'Próxima descarga',
    never: 'Nunca',
    figures: {
      entries: 'Entradas en la fuente',
      shown: 'Canales mostrados',
      mapped: 'Con guía',
      unmapped: 'Sin guía',
    },
    openSource: 'Abrir la fuente',
    iptvHint:
      'Qué importar, las categorías, los canales, los números y las guías se configuran en la página de la fuente.',
    refresh: 'Actualizar',
    refreshLabel: (name: string) => `Actualizar ${name}`,
    refreshed: (name: string) => `${name}: se ha vuelto a descargar el manifiesto.`,
    refreshedList: (name: string) => `${name}: se ha vuelto a descargar la lista.`,
    replace: 'Reemplazar la dirección',
    newManifestUrl: 'Nueva dirección del manifiesto',
    replaceHint:
      'Úselo después de volver a configurar la extensión en su página de configuración. Sus bibliotecas se conservan.',
    replaceSubmit: 'Reemplazar',
    replaced: 'Se ha reemplazado la dirección del manifiesto.',
    language: 'Idioma de sus descripciones',
    languageHelp:
      'Déjelo en Ninguno en la única extensión de metadatos del servidor, sea cual sea el idioma que se haya fijado en su página de configuración: una extensión con un idioma solo describe los títulos para los usuarios que han elegido ese idioma, salvo que ninguna extensión dejada en Ninguno los describa. Para que los usuarios lean los títulos en otros idiomas, añada la misma extensión de metadatos una vez más por cada idioma, configurada en ese idioma en su página de configuración, e indique aquí el idioma solo en esas copias adicionales: los usuarios que elijan ese idioma de los metadatos verán sus nombres, sinopsis e imágenes. Una extensión con un idioma y sin biblioteca propia solo describe títulos.',
    languageNone: 'Ninguno',
    languageSaved: (name: string) => `${name}: se ha guardado el idioma de sus descripciones.`,
    edit: 'Editar la cuenta',
    saved: 'Fuente guardada.',
    settings: 'Ajustes',
    remove: 'Eliminar',
    removeTitle: (name: string) => `¿Eliminar ${name}?`,
    removeShared:
      'Sus bibliotecas desaparecerán de las aplicaciones de Jellyfin de todos los usuarios. Esta acción no se puede deshacer.',
    removeMine:
      'Sus bibliotecas desaparecerán de sus aplicaciones de Jellyfin. Esta acción no se puede deshacer.',
    removed: (name: string) => `Se ha eliminado ${name}.`,
    readOnly: (name: string) =>
      `Esta es una fuente propia de ${name}. Solo su propietario puede cambiarla, desde su página Mis fuentes.`,
    back: 'Fuentes',
    backMine: 'Mis fuentes',
    notFoundTitle: 'No se ha encontrado la fuente',
    notFound: 'Esta fuente no existe o no es una que usted pueda gestionar.',
  },
  sourceAdd: {
    title: 'Añadir una fuente',
    kind: 'Tipo de fuente',
    kinds: {
      stremio: 'Extensión de Stremio',
      music: 'Extensión de Eclipse',
      iptv: 'Fuente IPTV',
      tuner: 'Sintonizador HDHomeRun',
      local: 'Carpeta',
    },
    stremioHelp:
      'Una extensión de Stremio aporta catálogos, metadatos y transmisiones. Pegue su enlace de instalación.',
    musicHelp:
      'Una extensión de música de Eclipse aporta música, audiolibros o podcasts. Pegue la dirección de su manifiesto.',
    manifestUrl: 'Dirección del manifiesto',
    manifestUrlPlaceholder: 'https://…/manifest.json',
    manifestUrlHint:
      'Copie el enlace de instalación desde la página de configuración de la extensión. Suele contener sus ajustes o claves: Polyfin lo mantiene privado y solo muestra una versión abreviada.',
    install: 'Instalar',
    installed: (name: string) => `Se ha instalado ${name}.`,
    iptvHelp:
      'Importe directamente los canales de una lista de reproducción M3U o de una cuenta de Xtream Codes. Aparecen en Televisión en vivo como un catálogo de televisión, que puede ordenar en Bibliotecas. La lista se vuelve a descargar con el intervalo definido en Ajustes › Televisión en vivo.',
    steps: 'Pasos',
    stepAccount: 'Cuenta',
    stepImport: 'Qué importar',
    name: 'Nombre',
    account: 'Cuenta',
    m3u: 'Lista de reproducción M3U',
    xtream: 'Cuenta de Xtream Codes',
    playlistUrl: 'Dirección de la lista de reproducción',
    playlistUrlHint:
      'La dirección M3U que le da su proveedor. Puede contener sus credenciales: nunca se muestra completa.',
    server: 'Dirección del servidor',
    serverHint: 'Por ejemplo https://servidor:8080, tal como se la da su proveedor.',
    username: 'Nombre de usuario',
    password: 'Contraseña',
    keepHint: 'Déjelo vacío para conservar el valor actual.',
    passwordKeepHint: 'Déjelo vacío para conservar la contraseña actual.',
    providerGuide: 'Usar la guía de programación del proveedor',
    providerGuideHelp:
      'Los servidores de Xtream Codes publican una guía para la cuenta: completa la programación de los canales.',
    guideUrl: 'Dirección de la guía de programación (XMLTV, opcional)',
    guideUrlHint:
      'Archivo XMLTV, comprimido o sin comprimir. También puede añadirlo o cambiarlo más adelante en Bibliotecas.',
    next: 'Siguiente: elegir qué importar',
    back: 'Volver a la cuenta',
    import: 'Importar',
    added: (name: string, parts: string[]) => `Se ha añadido ${name}: ${parts.join(', ')}.`,
    channels: (count: string) => `${count} canales`,
    movies: (count: string) => `${count} películas`,
    series: (count: string) => `${count} series`,
    tunerHelp:
      'Un sintonizador HDHomeRun de su red aporta los canales que recibe por antena o cable. Aparecen en Televisión en vivo como los de una fuente IPTV, y el sintonizador reproduce a la vez tantos canales como sintonizadores tiene.',
    findTuners: 'Buscar sintonizadores',
    findTunersHelp: 'Pregunta a la red local qué sintonizadores hay; tarda unos segundos.',
    finding: 'Consultando la red local…',
    foundTitle: 'Sintonizadores en la red',
    tunerCount: (count: number, formatted: string) =>
      count === 1 ? '1 sintonizador' : `${formatted} sintonizadores`,
    addTuner: 'Añadir',
    addTunerLabel: (name: string) => `Añadir ${name}`,
    tunerAdded: 'Añadido',
    openTuner: (name: string) => `Abrir ${name}`,
    noTuners: 'Ningún sintonizador ha respondido.',
    noTunersHelp:
      'En Docker, Polyfin solo detecta sintonizadores con red de host (network_mode: host). También puede añadir un sintonizador por su dirección.',
    byAddress: 'Añadir por dirección',
    tunerAddress: 'Dirección del sintonizador',
    tunerAddressHint: 'Su dirección IP, como 192.168.1.20, o su dirección http://.',
    tunerAddressInvalid:
      'Introduzca la dirección IP del sintonizador, como 192.168.1.20, o su dirección http://.',
    tunerName: 'Nombre (opcional)',
    tunerNameHint: 'Si se deja vacío, el nombre propio del sintonizador.',
    favoritesOnly: 'Solo los canales favoritos del sintonizador',
    favoritesOnlyHelp:
      'Conserva los canales marcados como favoritos en el sintonizador, en su aplicación o su página web, y omite los demás.',
  },
  localFolders: {
    help: {
      container:
        'Una carpeta montada en el contenedor de Polyfin; basta con que sea de solo lectura. Sus vídeos se asocian a títulos por su nombre y se reproducen como versiones de esos títulos, junto a las transmisiones de las extensiones; una carpeta de música se convierte en una biblioteca de música. Polyfin se ejecuta como el usuario 65532, que debe poder leerla.',
      computer:
        'Una carpeta del ordenador en el que se ejecuta Polyfin, o de un disco o recurso compartido al que tenga acceso; basta con que sea de solo lectura. Sus vídeos se asocian a títulos por su nombre y se reproducen como versiones de esos títulos, junto a las transmisiones de las extensiones; una carpeta de música se convierte en una biblioteca de música. La cuenta con la que se ejecuta Polyfin debe poder leerla.',
    },
    location: 'Ubicación',
    locations: {
      local: 'Carpeta local',
      smb: 'Recurso compartido SMB',
      webdav: 'Carpeta WebDAV',
      s3: 'Bucket S3',
      sftp: 'Carpeta SFTP',
      rclone: 'Remoto de rclone',
    },
    shareHelp: {
      smb: 'Una carpeta compartida por SMB (uso compartido de archivos de Windows) desde un NAS u otro ordenador, que Polyfin lee directamente, sin montarla. Sus vídeos se asocian a títulos por su nombre y se reproducen como versiones de esos títulos, junto a las transmisiones de las extensiones; una carpeta de música se convierte en una biblioteca de música.',
      webdav:
        'Una carpeta que un servidor comparte por WebDAV, que Polyfin lee directamente, sin montarla. Sus vídeos se asocian a títulos por su nombre y se reproducen como versiones de esos títulos, junto a las transmisiones de las extensiones; una carpeta de música se convierte en una biblioteca de música.',
      s3: 'Un prefijo de un bucket de almacenamiento compatible con S3, que Polyfin lee directamente, sin montarlo. Sus vídeos se asocian a títulos por su nombre y se reproducen como versiones de esos títulos, junto a las transmisiones de las extensiones, leídos a través de la caché de archivos en lectura; una carpeta de música se convierte en una biblioteca de música.',
      sftp: 'Una carpeta de un servidor accesible por SSH, que Polyfin lee directamente, sin montarla. Sus vídeos se asocian a títulos por su nombre y se reproducen como versiones de esos títulos, junto a las transmisiones de las extensiones, leídos a través de la caché de archivos en lectura; una carpeta de música se convierte en una biblioteca de música.',
      rclone:
        'Una carpeta de uno de los almacenamientos a los que accede rclone, leída a través de una instancia de rclone que ejecuta rclone rcd --rc-serve, como un contenedor junto a Polyfin. Sus vídeos se asocian a títulos por su nombre y se reproducen como versiones de esos títulos, junto a las transmisiones de las extensiones, leídos a través de la caché de archivos en lectura; una carpeta de música se convierte en una biblioteca de música.',
    },
    address: 'Dirección',
    addressHint: {
      smb: 'smb:// seguido del servidor, el recurso compartido y, si procede, la carpeta dentro de él. Añada :puerto tras el servidor para usar un puerto distinto del 445.',
      webdav:
        'La dirección WebDAV de la carpeta. Es preferible https://, ya que http:// envía la contraseña sin cifrar.',
      s3: 's3:// seguido del bucket y, si procede, el prefijo dentro de él. Sus objetos se muestran como carpetas separadas por /.',
      sftp: 'sftp:// seguido del servidor y la ruta completa de la carpeta en él. Añada :puerto tras el servidor para usar un puerto distinto del 22.',
      rclone: '',
    },
    addressPlaceholder: {
      smb: 'smb://nas.local/media/peliculas',
      webdav: 'https://nas.local/dav/peliculas',
      s3: 's3://media/peliculas',
      sftp: 'sftp://nas.local/srv/media/peliculas',
      rclone: '',
    },
    user: 'Usuario',
    userHint: {
      smb: 'Vacío para un recurso compartido abierto a invitados. DOMINIO\\usuario para una cuenta de dominio.',
      webdav: 'Vacío para una carpeta abierta a todos.',
      s3: 'Ambos vacíos para un bucket que cualquiera puede leer.',
      sftp: 'La cuenta con la que Polyfin inicia sesión.',
      rclone: 'El usuario del control remoto de rclone (--rc-user), vacío si no hay ninguno.',
    },
    password: 'Contraseña',
    passwordHint: 'Polyfin no vuelve a mostrarla nunca.',
    sftpPasswordHint:
      'Vacía si se inicia sesión con la clave privada. Polyfin no vuelve a mostrarla nunca.',
    accessKey: 'Clave de acceso',
    secretKey: 'Clave secreta',
    endpoint: 'Endpoint',
    endpointHint:
      'La dirección del servicio S3 del almacenamiento. Polyfin indica el bucket en la ruta, como acepta todo almacenamiento compatible con S3, salvo en los endpoints propios de Amazon.',
    region: 'Región',
    regionHint:
      'Vacía para us-east-1, que acepta la mayoría de los almacenamientos compatibles con S3.',
    privateKey: 'Clave privada',
    privateKeyHint:
      'Una clave privada OpenSSH o PEM, en lugar de la contraseña o junto a ella. Polyfin no vuelve a mostrarla nunca.',
    privateKeySaved: 'Hay una clave guardada. Pegue otra para sustituirla.',
    privateKeyNotSent:
      'La clave guardada no se envía a otro servidor: péguela de nuevo o se eliminará al guardar.',
    secretNotSent:
      'El valor guardado no se envía a otro servidor: escríbalo de nuevo o se eliminará al guardar.',
    privateKeyRemove: 'Eliminar la clave guardada',
    passphrase: 'Frase de contraseña',
    passphraseHint: 'Solo para una clave privada que la tenga.',
    hostKey: 'Clave de host',
    hostKeyHint:
      'Se guarda en la primera conexión. Polyfin rechaza el servidor si presenta otra distinta.',
    hostKeyNone: 'Se guarda en la primera conexión',
    hostKeyOffered: (fingerprint: string) =>
      `El servidor presenta ahora otra clave de host, ${fingerprint}, y Polyfin la rechaza. Acéptela solo si el servidor se ha reinstalado o su clave se ha cambiado a propósito.`,
    acceptHostKey: 'Aceptar la nueva clave',
    hostKeyAccepted: 'Se ha aceptado la nueva clave de host. Polyfin vuelve a escanear la carpeta.',
    rcloneInstance: 'Dirección de rclone',
    rcloneInstanceHint:
      'Donde responde rclone rcd --rc-serve, accesible por http://, como http://rclone:5572 para un contenedor junto a Polyfin.',
    rcloneRemote: 'Remoto',
    rcloneRemoteHint: 'Uno de los remotos de la configuración de rclone.',
    listRemotes: 'Mostrar los remotos',
    remotesListed: (count: number, figure: string) =>
      count === 1 ? 'rclone tiene un remoto.' : `rclone tiene ${figure} remotos.`,
    noRemotes: 'rclone no tiene ningún remoto en su configuración.',
    remotesFailed: {
      unreachable: 'rclone no responde en esta dirección.',
      refused: 'rclone rechaza este usuario o contraseña.',
    } as Record<string, string>,
    rcloneFolder: 'Ruta en el remoto',
    rcloneFolderHint: 'La carpeta del remoto que se leerá; vacío para su raíz.',
    pollMinutes: 'Buscar cambios cada (minutos, 0 = solo en los escaneos)',
    pollMinutesHint: {
      s3: 'Las carpetas del bucket se vuelven a listar con esta frecuencia para encontrar los archivos añadidos y eliminados. Los proveedores pueden cobrar cada listado. 30 de forma predeterminada.',
      sftp: 'La carpeta y sus subcarpetas se vuelven a listar con esta frecuencia para encontrar los archivos añadidos y eliminados. 2 de forma predeterminada.',
      rclone:
        'Las carpetas del remoto se vuelven a listar con esta frecuencia para encontrar los archivos añadidos y eliminados. 2 de forma predeterminada.',
    },
    name: 'Nombre',
    nameHint: 'El nombre de su biblioteca en las aplicaciones de Jellyfin.',
    path: { container: 'Ruta en el contenedor', computer: 'Ruta' },
    pathHint: {
      container:
        'Dónde está montada la carpeta en el contenedor de Polyfin, no su ruta en el host.',
      linux: 'La ruta completa de la carpeta en el ordenador donde se ejecuta Polyfin.',
      macos:
        'La ruta completa de la carpeta en el Mac donde se ejecuta Polyfin; las carpetas de un disco externo están en /Volumes.',
      windows:
        'La ruta completa de la carpeta en el PC donde se ejecuta Polyfin: en una unidad, como D:\\Películas, o en un recurso compartido, como \\\\nas\\media\\Películas.',
    },
    pathPlaceholder: {
      container: '/media/peliculas',
      linux: '/srv/media/peliculas',
      macos: '/Users/yo/Movies',
      windows: 'D:\\Películas',
    },
    kind: 'Contiene',
    kinds: { movies: 'Películas', shows: 'Series', music: 'Música', homevideos: 'Vídeos caseros' },
    kindHelp: {
      movies:
        'Un archivo por película, solo o en su propia carpeta, con el nombre de la película y su año.',
      shows:
        'Una carpeta por serie, con la temporada y el episodio en el nombre de cada archivo, como S01E02.',
      music:
        'Archivos de audio leídos por sus etiquetas o, si no, por sus nombres, como Artista/Álbum/01 Título.flac, agrupados en álbumes y artistas.',
      homevideos:
        'Cada vídeo es un título independiente, con el nombre de su archivo, o de su carpeta cuando está solo en ella, y fechado por su archivo NFO, su grabación o el archivo. No se consulta ninguna extensión de metadatos.',
    },
    kindNouns: {
      movies: 'películas',
      shows: 'series',
      music: 'música',
      homevideos: 'vídeos caseros',
    },
    changeKind: 'Cambiar',
    changeKindTitle: (name: string) => `¿Cambiar el contenido de ${name}?`,
    changeKindScan: (from: string, to: string) =>
      `Contiene ${from}; contendrá ${to}. La carpeta y su biblioteca conservan sus identificadores, su dirección y sus ajustes, y Polyfin la vuelve a escanear como ${to}, en segundo plano.`,
    changeKindFollows:
      'Lo que los usuarios hicieron con cada vídeo (su estado de visto, su punto de reanudación y su favorito) pasa a su nuevo título: las marcas de visto y los favoritos se copian, y el título anterior los conserva; el punto de reanudación se mueve con el archivo, y el título anterior lo pierde.',
    changeKindNothing:
      'No se traslada nada: las canciones y los vídeos son otros archivos. Lo que los usuarios hicieron con sus títulos se queda con ellos y deja de mostrarse.',
    changeKindBack:
      'Sus vídeos caseros se recuerdan durante 30 días: si vuelve a contener vídeos caseros en ese plazo, los muestra como los mismos títulos, con lo que los usuarios hicieron con ellos.',
    linksDropped: (count: number, figure: string) =>
      count === 1 ? 'la vinculación manual' : `las ${figure} vinculaciones manuales`,
    episodesDropped: (count: number, figure: string) =>
      count === 1
        ? 'los episodios asignados al título de un disco'
        : `los episodios asignados a ${figure} títulos de discos`,
    changeKindDrops: (parts: string[]) =>
      `Se eliminan, ya que se crearon para su contenido actual: ${parts.join(' y ')}.`,
    useArtwork: 'Usar sus ilustraciones',
    useArtworkHelp:
      'Los pósteres, imágenes de fondo y logotipos junto a los archivos, como poster.jpg o Película (2001)-fanart.jpg, se muestran en lugar de los de las extensiones de metadatos para sus títulos, y las imágenes de los episodios en lugar de sus fotogramas. Desactivado, se muestran los de las extensiones.',
    readExtras: 'Leer extras y temas',
    readExtrasHelp:
      'Los extras, tráilers locales y músicas de tema junto a los títulos, con los nombres que usa Jellyfin, como Extras/Making of.mkv, Película (2001)-trailer.mkv o theme.mp3, se muestran en los contenidos especiales de sus títulos, se reproducen con el botón Tráiler o suenan de fondo en sus páginas. Desactivado, el próximo escaneo los omite.',
    add: 'Añadir la carpeta',
    added: (name: string) => `Se ha añadido ${name}. Su primer escaneo ha comenzado.`,
    edit: 'Editar la carpeta',
    save: 'Guardar',
    cancel: 'Cancelar',
    saved: 'Carpeta guardada.',
    movedHint:
      'Una ruta nueva olvida los archivos encontrados en la anterior, vinculaciones incluidas.',
    movedShareHint:
      'Una dirección nueva olvida los archivos encontrados en la anterior, vinculaciones incluidas.',
    scan: 'Escanear ahora',
    scanLabel: (name: string) => `Escanear ${name} ahora`,
    scanStarted: (name: string) => `${name}: el escaneo ha comenzado.`,
    scanning: 'Escaneando',
    unreadable: 'No se puede leer',
    figuresTitle: 'Archivos',
    files: 'Archivos de vídeo',
    matched: 'Asociados a un título',
    unmatched: 'Sin asociar',
    extras: 'Extras',
    tracks: 'Pistas',
    albums: 'Álbumes',
    artists: 'Artistas',
    gained: 'Con ganancia',
    measured: 'Medidos',
    leftOut: 'Omitidos',
    videos: 'Vídeos',
    datedBy: {
      nfo: 'Fechados por un archivo NFO',
      created: 'Fechados por la grabación',
      file: 'Fechados por el archivo',
    },
    matchedBy: (parts: string[]) => `Asociados: ${parts.join(', ')}.`,
    matchMethods: {
      name: (count: string) => `${count} por un identificador en el nombre`,
      nfo: (count: string) => `${count} por un archivo NFO`,
      search: (count: string) => `${count} por una búsqueda`,
      link: (count: string) => `${count} a mano`,
    },
    holds: 'Contenido',
    lastScan: 'Último escaneo',
    lastAttempt: 'Último intento',
    never: 'Todavía no',
    errorTitle: 'Polyfin no puede leer esta carpeta',
    errorTitleShare: 'Polyfin no puede leer este recurso compartido',
    errors: {
      missing:
        'La ruta no existe donde se ejecuta Polyfin: compruebe que la carpeta, o el disco o recurso compartido en el que está, existe y, en Docker, que está montada en el contenedor.',
      unreadable:
        'La carpeta existe, pero Polyfin no puede listarla: conceda a la cuenta con la que se ejecuta (el usuario 65532 en Docker) permiso para leerla y abrir sus carpetas.',
      not_folder: 'Esta ruta es un archivo, no una carpeta.',
    } as Record<string, string>,
    shareErrors: {
      missing:
        'El recurso compartido, o la carpeta dentro de él, no existe: compruebe la dirección.',
      unreadable:
        'Es posible que el usuario del recurso compartido no pueda listar esta carpeta: compruebe sus permisos en el servidor.',
      not_folder: 'Esta dirección es un archivo, no una carpeta.',
      unreachable:
        'Polyfin no puede conectar con el recurso compartido: compruebe que el servidor está encendido, así como el servidor y el puerto de la dirección.',
      refused:
        'El recurso compartido rechaza el usuario o la contraseña: compruébelos e introdúzcalos de nuevo.',
      host_key_changed:
        'El servidor SFTP presenta una clave de host distinta de la guardada en la primera conexión, y Polyfin la rechaza. Si el cambio es esperado, acepte la nueva clave en «Editar la carpeta».',
    } as Record<string, string>,
    keptNote:
      'Los archivos encontrados anteriormente permanecen en la biblioteca hasta que la carpeta se pueda volver a leer.',
    unmatchedTitle: 'Archivos sin asociar',
    unmatchedHelp:
      'No se encontró ningún título para estos archivos. Cámbieles el nombre con el título y el año, o añada un archivo NFO con el identificador del título, y después escanee; o asócielos al identificador de IMDb que aparece en la dirección de IMDb del título (tt seguido de dígitos). Una asociación se mantiene tras los nuevos escaneos; en una carpeta de series, abarca toda la serie y, para un disco Blu-ray o un DVD, todo el disco, o una película de un disco que contenga varias. Los títulos del disco de una serie toman su episodio tal como lo indica el nombre de un archivo, por ejemplo S01E05, o S01E05-E06 para dos; los episodios sugeridos para un disco siguen a los de los discos anteriores de la temporada.',
    noUnmatched: 'Todos los archivos están asociados a un título.',
    leftOutTitle: 'Pistas excluidas',
    leftOutHelp:
      'Ni las etiquetas ni los nombres de estos archivos indican un título o un álbum. Etiquételos o muévalos a una carpeta Artista/Álbum, y después escanee.',
    noLeftOut: 'Todas las pistas están en un álbum.',
    shownOf: (shown: string, total: string) => `Se muestran ${shown} de ${total}.`,
    reasons: {
      unreadable_name: 'Su nombre no indica ningún título o ningún número de episodio',
      disc_title: 'Otra película del disco: asóciela a su título',
      disc_episode: 'Un título del disco de una serie: indique su episodio',
      lone_extra: 'Un extra sin su título al lado: asócielo para convertirlo en un título',
      not_found: 'Las extensiones de metadatos no conocen ningún título con este nombre',
      no_search: 'Ninguna extensión del servidor describe ni busca títulos de este tipo',
      ambiguous: 'Varios títulos tienen este nombre: añádale el año o asócielo',
      other_year: 'Los títulos con este nombre son de otros años',
      search_failed: 'No se pudo buscar en las extensiones de metadatos',
      no_title: 'Sus etiquetas y su nombre no indican ningún título',
      no_album: 'Sus etiquetas y carpetas no indican ningún álbum',
    } as Record<string, string>,
    searchFailedBy: (addon: string, failure: string) =>
      failure === ''
        ? `La búsqueda de ${addon} ha fallado`
        : `La búsqueda ha fallado: ${addon} ${failure}`,
    searchFailures: {
      unreachable: 'no se pudo contactar',
      timeout: 'no respondió a tiempo',
      private_network: 'está en una red local',
      rate_limited: 'pidió reducir el ritmo (demasiadas solicitudes)',
      server_error: 'respondió con un error del servidor',
      http_error: 'rechazó la solicitud',
      invalid_response: 'envió una respuesta que Polyfin no puede leer',
    } as Record<string, string>,
    noSearchTitle: (count: number, figure: string) =>
      count === 1
        ? 'Ninguna extensión puede buscar el título de 1 archivo'
        : `Ninguna extensión puede buscar los títulos de ${figure} archivos`,
    noSearchHelp:
      'La asociación necesita una extensión del servidor, en Contenido › Fuentes, que describa estos títulos y tenga un catálogo de búsqueda. Las extensiones propias de los usuarios, en Mis fuentes, no cuentan. Cuando añada una, escanee la carpeta.',
    searchFailedTitle: (count: number, figure: string) =>
      count === 1
        ? 'No se pudo buscar el título de 1 archivo'
        : `No se pudieron buscar los títulos de ${figure} archivos`,
    searchRetry: (at: string) =>
      `Polyfin vuelve a buscar por sí solo, la próxima vez el ${at}, luego con menos frecuencia mientras la búsqueda falle, y en cada escaneo.`,
    searchHelp:
      'La asociación necesita una extensión del servidor que describa estos títulos y tenga un catálogo de búsqueda. Si sus búsquedas siguen fallando, compruébela en Sistema › Estado del sistema, o asocie los archivos a mano más abajo.',
    read: (title: string) => `Leído como «${title}»`,
    showFolder: (unit: string) => `Carpeta de la serie: ${unit}`,
    discTitle: (disc: string, title: number) => `${disc} · título ${title}`,
    disc: {
      bluray: (unit: string) => `Disco Blu-ray: ${unit}`,
      dvd: (unit: string) => `DVD: ${unit}`,
      bluray_image: (unit: string) => `Imagen de Blu-ray: ${unit}`,
      dvd_image: (unit: string) => `Imagen de DVD: ${unit}`,
    },
    minutes: (minutes: number) => `${minutes} min`,
    episodeLabel: (title: string) => `Episodio de ${title}`,
    episodePlaceholder: 'S01E01',
    giveEpisode: 'Guardar',
    episodesGiven: (count: number) =>
      count === 1 ? 'El título está numerado.' : `${count} títulos están numerados.`,
    suggestion: (disc: string, first: string, last: string) =>
      first === last
        ? `Sugerido para ${disc}: ${first}`
        : `Sugeridos para ${disc}: del ${first} al ${last}`,
    confirmSuggestion: 'Usar estos episodios',
    imdbId: 'Identificador de IMDb',
    imdbPlaceholder: 'tt0000000',
    link: 'Asociar',
    linkLabel: (path: string) => `Identificador de IMDb para ${path}`,
    linked: (unit: string, id: string) => `${unit} está asociado a ${id}.`,
    findTitle: 'Buscar el título',
    searchLabel: (path: string) => `Búsqueda de título para ${path}`,
    searchQuery: 'Título',
    searchYear: 'Año',
    search: 'Buscar',
    searching: 'Buscando en las extensiones de metadatos…',
    noTitles:
      'Las extensiones de metadatos no encontraron ningún título con este nombre que tenga identificador de IMDb. Cambie la búsqueda o escriba el identificador.',
    titlesFound: (query: string) => `Títulos encontrados para «${query}»`,
    titleKinds: { movie: 'Película', show: 'Serie' },
    pickTitle: (name: string, details: string) => `Vincular a ${name}, ${details}`,
    linksTitle: 'Vinculados manualmente',
    unlink: 'Eliminar el vínculo',
    unlinkLabel: (unit: string) => `Eliminar el vínculo de ${unit}`,
    unlinked: (unit: string) =>
      `Se ha eliminado el vínculo de ${unit}: vuelve a identificarse por su nombre o su archivo NFO.`,
    removeEpisodeLabel: (title: string) => `Eliminar el episodio de ${title}`,
    episodeRemoved: (title: string) =>
      `Se ha eliminado el episodio de ${title}: el nombre de su disco lo numera, o vuelve a sugerirse.`,
    openPage: 'Abrir la página de la carpeta',
  },
  mySources: {
    lede: 'Añada sus propias fuentes de Stremio, Eclipse e IPTV y elija cuáles de sus catálogos aparecen como bibliotecas en sus aplicaciones de Jellyfin.',
    preferenceTitle: 'Las fuentes del servidor',
    useShared: 'Usar las fuentes del servidor',
    useSharedHelp:
      'Activado: recibe las fuentes y bibliotecas del servidor, seguidas de las suyas. Desactivado: solo recibe sus propias fuentes y bibliotecas.',
    preferenceSaved: 'Preferencia guardada.',
    parentalControl:
      'El control parental se aplica a su cuenta: sus aplicaciones de Jellyfin solo muestran las fuentes y bibliotecas del servidor, ya que proporcionan las clasificaciones en las que se basa. Sus propias fuentes se conservan, pero no se usan.',
    personalAddonsOff:
      'Un administrador ha desactivado sus propias fuentes: sus aplicaciones de Jellyfin solo muestran las fuentes y bibliotecas del servidor. Sus propias fuentes se conservan, pero no se usan.',
    listTitle: 'Sus fuentes',
  },
  music: {
    content: { music: 'Música', audiobook: 'Audiolibros', podcast: 'Podcasts' } as Record<
      string,
      string
    >,
    library: {
      music: 'Biblioteca de música',
      audiobook: 'Biblioteca de libros',
      podcast: 'Biblioteca de música',
    } as Record<string, string>,
    settings: 'Ajustes',
    settingsTitle: 'Ajustes de la extensión',
    settingsHelp:
      'Los declara la extensión y se envían con cada solicitud que Polyfin le hace. Pueden contener los datos de su cuenta: solo usted, o los administradores en el caso de las extensiones del servidor, puede verlos.',
    noSettings: 'Esta extensión no tiene ajustes.',
    defaultValue: (value: string) => `Predeterminado: ${value}`,
    defaultEmpty: 'Predeterminado: vacío',
    on: 'Activado',
    off: 'Desactivado',
    range: (min: string, max: string) => `De ${min} a ${max}.`,
    atLeast: (min: string) => `Como mínimo ${min}.`,
    atMost: (max: string) => `Como máximo ${max}.`,
    stepOf: (step: string) => `En incrementos de ${step}.`,
    maxLength: (count: number) => `${count} caracteres como máximo.`,
    tooLong: (count: number) => `Use ${count} caracteres como máximo.`,
    belowMin: (min: string) => `Introduzca ${min} o más.`,
    aboveMax: (max: string) => `Introduzca ${max} o menos.`,
    offStep: (step: string) => `Use un múltiplo de ${step}.`,
    resetDefaults: 'Usar los valores predeterminados',
    saved: 'Configuración guardada. La extensión la recibe a partir de su próxima solicitud.',
  },
}

export default sources
