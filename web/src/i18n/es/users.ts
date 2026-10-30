import type en from '../en/users'

const users: typeof en = {
  users: {
    title: 'Usuarios',
    description: 'Cree y gestione las cuentas autorizadas para usar este servidor.',
    listTitle: 'Cuentas',
    empty: 'Todavía no hay ningún usuario.',
    administrator: 'Administrador',
    hidden: 'Oculto en la pantalla de inicio de sesión',
    disabled: 'Desactivado',
    you: 'Usted',
    createTitle: 'Crear un usuario',
    name: 'Nombre',
    password: 'Contraseña',
    isAdministrator: 'Administrador',
    isAdministratorHelp:
      'Los administradores pueden gestionar los usuarios y los ajustes del servidor.',
    showOnSignIn: 'Mostrar en la pantalla de inicio de sesión',
    showOnSignInHelp:
      'Los usuarios visibles aparecen en la pantalla de inicio de sesión de las aplicaciones de Jellyfin. Los usuarios ocultos escriben su nombre, como en Jellyfin de forma predeterminada.',
    create: 'Crear usuario',
    creating: 'Creando…',
    created: (name: string) => `Se ha creado ${name}.`,
    edit: 'Editar',
    close: 'Cerrar',
    rename: 'Cambiar nombre',
    renamed: 'Se ha cambiado el nombre.',
    resetPassword: 'Restablecer contraseña',
    newPassword: 'Nueva contraseña',
    resetPasswordHelp:
      'Al cambiar la contraseña se cierra la sesión en todos los dispositivos Jellyfin de este usuario.',
    passwordReset: 'Se ha restablecido la contraseña.',
    accessTitle: 'Acceso',
    isDisabled: 'Desactivado',
    isDisabledHelp:
      'Un usuario desactivado no puede iniciar sesión y se cierra la sesión en todos sus dispositivos.',
    providerOnly: 'Solo el proveedor de identidad',
    providerOnlyHelp:
      'El usuario inicia sesión con el proveedor de identidad configurado en Ajustes › Seguridad, nunca con una contraseña: la contraseña se rechaza, y una cuenta creada así no necesita ninguna. Las aplicaciones de Jellyfin que no pueden abrir el proveedor inician sesión con Quick Connect.',
    ended: 'Finalizada',
    endsOn: (date: string) => `Finaliza el ${date}`,
    endedOn: (date: string) => `Finalizó el ${date}`,
    endTitle: 'Finaliza el',
    endHelp:
      'A partir de esta fecha, en la zona horaria del servidor, la cuenta se rechaza igual que una cuenta desactivada: no puede iniciar sesión y sus dispositivos se rechazan. Todo lo que tiene se conserva, y una fecha posterior, o «Nunca», la restablece.',
    endAdministratorHelp:
      'La cuenta de un administrador nunca finaliza: convertir a un usuario en administrador borra su fecha de fin.',
    endDate: 'Fecha de fin',
    saveEnd: 'Guardar fecha',
    endNever: 'Nunca',
    canTranscode: 'Puede usar la conversión (transcodificación)',
    canTranscodeHelp:
      'Permite que el servidor vuelva a codificar el vídeo y el audio cuando la aplicación de este usuario no puede reproducir un archivo tal cual. Sin ello, los archivos se reproducen tal cual o simplemente reempaquetados.',
    canDownload: 'Puede descargar',
    canDownloadHelp:
      'Permite a este usuario guardar títulos en las aplicaciones de Jellyfin para verlos sin conexión.',
    canPlayTorrents: 'Puede reproducir transmisiones torrent',
    canPlayTorrentsHelp:
      'Ofrece a este usuario la lista de torrents de las extensiones, cuando Ajustes › Torrents los reproduce. Desactivado para todos, administradores incluidos, hasta que se conceda.',
    noTranscoding: 'Sin conversión',
    noDownloads: 'Sin descargas',
    turnOffDownloads: 'Desactivar las descargas para todos',
    turnOffDownloadsConfirm:
      'Todos los usuarios y todos los grupos pierden el permiso para descargar, también en las aplicaciones de Jellyfin con sesión ya iniciada. Puede volver a concederlo a un usuario o a un grupo en su página, en Acceso.',
    turningOffDownloads: 'Desactivando…',
    downloadsTurnedOff: (count: number) =>
      count === 0
        ? 'Nadie podía descargar.'
        : count === 1
          ? '1 usuario ya no puede descargar.'
          : `${count} usuarios ya no pueden descargar.`,
    lastAdminHelp: 'El último administrador activo no se puede borrar, degradar ni desactivar.',
    updated: 'Cambios guardados.',
    metadataLanguageTitle: 'Idioma de los metadatos',
    metadataLanguageHelp:
      'El idioma de los nombres, las sinopsis y las imágenes de los títulos en las aplicaciones de este usuario, cuando una de las extensiones del servidor los describe en ese idioma; los títulos que ninguna extensión describe en él conservan la descripción del servidor. Los géneros, las clasificaciones y las personas son los mismos para todos, de modo que el control parental y los géneros bloqueados juzgan a todos los usuarios por igual. Los nombres que crea el propio Polyfin lo siguen cuando Polyfin lo habla: inglés, francés, alemán, español, italiano o portugués.',
    metadataLanguageNone:
      'Ninguna extensión del servidor tiene idioma todavía. Asigne un idioma a una en su página, en Contenido › Fuentes.',
    parentalTitle: 'Control parental',
    maxRating: 'Clasificación máxima',
    maxRatingHelp:
      'Los títulos con una clasificación superior a este límite se ocultan a este usuario.',
    noLimit: 'Sin límite',
    blockUnratedMovies: 'Bloquear películas sin clasificar',
    blockUnratedShows: 'Bloquear series sin clasificar',
    ratingLimit: (rating: string) => `Hasta ${rating}`,
    saveParental: 'Guardar control parental',
    devicesTitle: 'Dispositivos',
    delete: 'Borrar usuario',
    deleteConfirm: (name: string) =>
      `¿Borrar ${name}? Se cerrará la sesión en sus dispositivos y esta acción no se puede deshacer.`,
    deleting: 'Borrando…',
    deleted: (name: string) => `Se ha borrado ${name}.`,
    canAddAddons: 'Puede añadir sus propias extensiones',
    canAddAddonsHelp:
      'Permite a este usuario añadir sus propias extensiones de Stremio en su página Mis fuentes. Sin ello, sus extensiones se conservan pero no se usan.',
    noPersonalAddons: 'Sin extensiones propias',
    blockedUntil: (time: string) => `Bloqueado hasta las ${time}`,
    blockedHelp:
      'Se han introducido demasiadas contraseñas incorrectas para esta cuenta: no puede iniciar sesión hasta entonces, ni siquiera con la contraseña correcta.',
    unblock: 'Desbloquear',
    unblocking: 'Desbloqueando…',
    unblocked: 'La cuenta está desbloqueada.',
    twoStepOn: 'El inicio de sesión en dos pasos está activado.',
    twoStepHelp:
      'Si este usuario ha perdido el teléfono con su aplicación de autenticación y no le quedan códigos de recuperación, desactívelo: el usuario inicia sesión solo con su contraseña y después lo vuelve a configurar desde Mi cuenta.',
    turnOffTwoStep: 'Desactivar el inicio de sesión en dos pasos',
    turningOffTwoStep: 'Desactivando…',
    turnOffTwoStepTitle: (name: string) =>
      `¿Desactivar el inicio de sesión en dos pasos de ${name}?`,
    turnOffTwoStepConfirm: (name: string) =>
      `${name} iniciará sesión solo con su contraseña. El registro de actividad indicará que usted lo ha desactivado.`,
    twoStepTurnedOff: 'El inicio de sesión en dos pasos está desactivado para este usuario.',
    playbackAccessTitle: 'Reproducción y acceso',
    maxPlaybacks: 'Reproducciones simultáneas (0 = sin límite)',
    maxPlaybacksHelp:
      'Cuántos dispositivos de este usuario pueden reproducir al mismo tiempo. Puede seguir iniciando sesión en más dispositivos.',
    maxBitrate: 'Calidad máxima',
    maxBitrateHelp:
      'Por encima de ella, el vídeo se convierte a menor calidad si este usuario puede usar la conversión; si no, se reproduce una versión más ligera.',
    bitrateNoLimit: 'Sin límite',
    bitrate4k: '40 Mbit/s (4K)',
    bitrate1080High: '20 Mbit/s (1080p, alta calidad)',
    bitrate1080: '10 Mbit/s (1080p)',
    bitrate720: '4 Mbit/s (720p)',
    bitrate480: '2 Mbit/s (480p)',
    bitrateOther: (mbits: string) => `${mbits} Mbit/s`,
    liveTv: 'Televisión en vivo',
    liveTvHelp: 'Muestra los canales de televisión en vivo a este usuario.',
    syncPlay: 'Ver juntos',
    syncPlayHelp:
      'Reproducir el mismo título de forma sincronizada con otros, en las aplicaciones de Jellyfin que lo permiten.',
    syncPlayCreateAndJoin: 'Crear grupos y unirse',
    syncPlayJoin: 'Solo unirse',
    syncPlayNone: 'No',
    remoteControl: 'Puede controlar las aplicaciones de otros usuarios',
    remoteControlHelp:
      'Permite a este usuario reproducir, pausar o enviar mensajes a las aplicaciones de Jellyfin de otros usuarios. Todos pueden controlar sus propias aplicaciones.',
    liveTvManagement: 'Puede grabar televisión en vivo',
    liveTvManagementHelp: 'Programar y borrar grabaciones.',
    savePlaybackAccess: 'Guardar reproducción y acceso',
    visibleLibrariesTitle: 'Bibliotecas visibles',
    visibleLibrariesHelp:
      'Las bibliotecas del servidor que muestran las aplicaciones de este usuario. Las bibliotecas añadidas más adelante se muestran. Esto solo elige las bibliotecas que se muestran: los títulos siguen siendo accesibles desde otras bibliotecas, la búsqueda o enlaces. Para bloquear títulos, use el control parental o los géneros bloqueados.',
    noServerLibraries: 'El servidor aún no tiene ninguna biblioteca.',
    saveVisibleLibraries: 'Guardar bibliotecas visibles',
    blockedGenresTitle: 'Géneros bloqueados',
    blockedGenresHelp:
      'Los títulos de cualquiera de estos géneros se ocultan a este usuario en todas partes, como los títulos que superan su límite de control parental.',
    genre: 'Género',
    genreHint: 'Elija un género de las bibliotecas del servidor o escriba uno.',
    addGenre: 'Añadir',
    removeGenre: (genre: string) => `Eliminar ${genre}`,
    noBlockedGenres: 'No hay ningún género bloqueado.',
    saveBlockedGenres: 'Guardar géneros bloqueados',
    allowedHoursTitle: 'Horario permitido',
    allowedHoursHelp:
      'Si se define un horario, este usuario solo puede iniciar sesión y usar el servidor dentro de él, según la zona horaria del servidor. Fuera de ese horario, se rechazan sus aplicaciones.',
    noAllowedHours: 'Sin horario definido: este usuario puede usar el servidor a cualquier hora.',
    day: 'Día',
    from: 'Desde',
    to: 'Hasta',
    addHours: 'Añadir horario',
    removeHours: 'Eliminar',
    hoursOrder: 'Cada hora de fin debe ser posterior a su hora de inicio.',
    saveAllowedHours: 'Guardar horario permitido',
    days: {
      Sunday: 'Domingo',
      Monday: 'Lunes',
      Tuesday: 'Martes',
      Wednesday: 'Miércoles',
      Thursday: 'Jueves',
      Friday: 'Viernes',
      Saturday: 'Sábado',
      Everyday: 'Todos los días',
      Weekday: 'Entre semana (de lunes a viernes)',
      Weekend: 'Fines de semana',
    },
    canManageCollections: 'Puede gestionar colecciones',
    canManageCollectionsHelp:
      'Permite a este usuario crear colecciones de títulos en las aplicaciones de Jellyfin, añadirles títulos o quitárselos, y borrarlas. Todos los usuarios ven las colecciones, cada uno solo con los títulos que puede ver.',
    resetPinRequested: 'Restablecimiento de contraseña solicitado: PIN',
    resetPinValidUntil: (time: string) => `, válido hasta las ${time}`,
    resetPinHelp:
      'Entregue este PIN al usuario: inicia sesión con él y se convierte en su nueva contraseña.',
    canManageSubtitles: 'Puede gestionar subtítulos',
    canManageSubtitlesHelp:
      'Permite a este usuario añadir archivos de subtítulos a los títulos desde sus aplicaciones de Jellyfin y buscar allí los subtítulos de las extensiones.',
    canShare: 'Puede compartir títulos',
    canShareHelp:
      'Permite a este usuario dar acceso a una película o un episodio a alguien sin cuenta, mediante un enlace que caduca, desde Mi cuenta o la página de un título en el reproductor web, mientras Ajustes › Seguridad › Permitir enlaces para compartir esté activado. Al desactivarlo, se revocan sus enlaces.',
    shareLinksTitle: 'Enlaces compartidos',
    shareLinksHelp:
      'Los enlaces que este usuario ha creado para compartir una película o un episodio. «Revocar» detiene uno al instante.',
    qualityGroup: 'Grupo de calidad',
    qualityGroupHelp:
      'La resolución más alta que se ofrece a este usuario. Las versiones superiores se descartan mientras haya una que se ajuste; si ninguna se ajusta, se convierten a menor resolución, o se rechazan si este usuario no puede usar la conversión. La televisión en directo también se convierte a menor resolución. Solo las versiones que se ajustan se descargan tal cual; con «Descargas convertidas» activado en Ajustes › Conversión (transcodificación), las aplicaciones que lo solicitan descargan las demás convertidas a menor resolución.',
    qualityGroupOriginal: 'Original (sin límite)',
    member: 'Miembro',
    lastSignInLabel: 'Último inicio de sesión:',
    active: 'Activo',
    pinRequested: 'PIN solicitado',
    emptyHelp:
      'Cree una cuenta para cada persona que vea contenido aquí y, a continuación, inicie sesión con ella desde una aplicación de Jellyfin.',
    devicesEmptyHelp: 'Las aplicaciones aparecen aquí cuando inician sesión con esta cuenta.',
    notFound: 'Usuario no encontrado',
    notFoundHelp:
      'Este usuario ya no existe o la dirección es incorrecta. Seleccione un usuario de la lista.',
    backToUsers: 'Volver a usuarios',
    sectionsLabel: 'Secciones del usuario',
    profileTitle: 'Nombre y contraseña',
    accessHelp: 'Cada cambio se guarda al instante.',
    noServerLibrariesHelp:
      'Añada una biblioteca en Contenido › Bibliotecas y, a continuación, elija aquí cuáles ve este usuario.',
    jellyfinImport: {
      open: 'Importar desde otro servidor',
      title: 'Importar desde otro servidor',
      description:
        'Traiga las cuentas y sus datos de visualización desde un servidor Jellyfin, Emby o Plex: nombres de cuenta y condición de administrador y, para cada usuario, las películas y episodios reproducidos con sus fechas, los puntos de reanudación y los favoritos, identificados por IMDb, TMDB o TVDB. A los datos de Polyfin solo se les añade información, y no se escribe nada en el otro servidor. Los servidores no facilitan las contraseñas: usted establece una para cada cuenta nueva, o conserva la que usa el usuario para iniciar sesión.',
      serverKind: 'Servidor',
      servers: {
        jellyfin: 'Jellyfin',
        emby: 'Emby',
        plex: 'Plex',
      },
      serverKindHelp: {
        jellyfin: 'Jellyfin, o un servidor compatible con su API.',
        emby: 'Emby, con sus propias claves API y usuarios.',
        plex: 'Plex Media Server, leído con el token de su propietario.',
      },
      connectTitle: (server: string) => `Conectar con ${server}`,
      connectHelp:
        'La dirección, la clave y las contraseñas solo se usan para esta importación: Polyfin no las guarda.',
      address: 'Dirección del servidor',
      addressHelp: (server: string) =>
        `La dirección con la que ${server} se abre en un navegador. Sirve una dirección de la red local.`,
      plexAddressHelp:
        'La dirección del propio Plex Media Server, normalmente en el puerto 32400. Sirve una dirección de la red local.',
      connectWith: 'Conectar con',
      withApiKey: 'Una clave API',
      withAccount: 'Una cuenta de usuario',
      apiKey: 'Clave API',
      apiKeyHelp:
        'Cree una en el panel de control de Jellyfin, en Claves API: lee los datos de visualización de todos los usuarios.',
      embyApiKeyHelp:
        'Cree una en los ajustes de Emby, en Claves API: lee los datos de visualización de todos los usuarios. No se acepta la clave propia de un usuario: Emby no indica a quién pertenece.',
      plexToken: 'Token de Plex',
      plexTokenHelp:
        'El token del propietario del servidor, que Polyfin solo usa para leer el servidor. Para encontrarlo, abra Plex en un navegador con la sesión iniciada como propietario. Abra el menú de cualquier película, elija Get Info y luego View XML: la dirección de la página que se abre termina en X-Plex-Token= seguido del token. Copie lo que viene a continuación.',
      accountName: 'Nombre de usuario',
      accountNameHelp: (server: string) =>
        `La cuenta de un administrador muestra todos los usuarios. Polyfin inicia sesión como lo hacen las aplicaciones de ${server} y después cierra la sesión.`,
      accountPassword: 'Contraseña',
      accountPasswordHelp: 'Déjela en blanco si la cuenta no tiene.',
      connect: 'Conectar',
      connecting: 'Conectando…',
      chooseTitle: 'Elegir a quién importar',
      chooseHelp: (server: string, product: string, version: string) =>
        `${server}, ${product} ${version}. Para cada usuario, elija la cuenta de Polyfin en la que importar, o cree una.`,
      changeServer: 'Cambiar de servidor',
      noUsers: (product: string) => `Este servidor ${product} no tiene ningún usuario.`,
      lastActivityLabel: 'Última actividad:',
      importAs: 'Importar en',
      skip: 'No importar',
      newUser: 'Nuevo usuario',
      watchData: 'Importar datos de visualización',
      watchDataHelp: 'Títulos reproducidos con sus fechas, puntos de reanudación y favoritos.',
      plexOwnerWatchDataHelp:
        'Títulos reproducidos con sus fechas y número de reproducciones, y puntos de reanudación.',
      plexWatchDataHelp:
        'Los títulos que esta cuenta reprodujo hasta el final en este servidor, según el historial de reproducción de Plex, con sus fechas y número de reproducciones. Los títulos que marcó como vistos o sincronizó desde su cuenta de Plex, y sus puntos de reanudación, se quedan en Plex.',
      watchDataNeeded: 'Sin ellos, no se importa nada para este usuario.',
      plexNotice:
        'Con el token del propietario, Plex permite a Polyfin leer los títulos reproducidos y los puntos de reanudación del propietario. De las demás cuentas, solo da su historial de reproducción en este servidor: los títulos que reprodujeron hasta el final aquí. Los títulos que marcaron como vistos sin reproducirlos, los sincronizados desde su cuenta de Plex y sus puntos de reanudación no figuran en él y se quedan en Plex. Plex no tiene favoritos que importar.',
      ownerNotice: (owner: string | null, signedIn: boolean) =>
        `${signedIn ? `Con la sesión iniciada como ${owner ?? 'un usuario'}, Polyfin solo lee los datos de visualización de ese usuario.` : `Esta clave es ${owner === null ? 'de un usuario' : `de ${owner}`}: solo lee los datos de visualización de ese usuario.`} Para importar los de otro usuario, escriba la contraseña de su cuenta en el servidor: Polyfin inicia sesión como ese usuario para leerlos. Sin ella, su cuenta se puede crear igualmente.`,
      ownerOnlyNotice: (owner: string | null, signedIn: boolean) =>
        `${signedIn ? `Con la sesión iniciada como ${owner ?? 'este usuario'}` : `Esta clave pertenece a ${owner ?? 'un usuario'}`}, que no es administrador de este servidor: Polyfin solo importa esta cuenta. Para ver los demás usuarios del servidor, conéctese con la cuenta de un administrador o con una clave API.`,
      serverPassword: (server: string) => `Contraseña en ${server}`,
      serverPasswordHelp: (user: string) =>
        `Polyfin inicia sesión como ${user} para leer sus datos de visualización y después cierra la sesión. Déjela en blanco si la cuenta no tiene.`,
      keepPassword: 'Conservar esta contraseña en Polyfin',
      keepPasswordHelp:
        'La cuenta nueva inicia sesión en Polyfin con la misma contraseña, si tiene al menos 8 caracteres.',
      nothingToImport: 'Elija un usuario que crear o datos de visualización que importar.',
      start: 'Iniciar importación',
      starting: 'Iniciando…',
      started: (created: number, importing: boolean) =>
        created === 0
          ? 'La importación ha comenzado.'
          : `${created === 1 ? '1 usuario creado.' : `${created} usuarios creados.`}${importing ? ' La importación ha comenzado.' : ''}`,
      currentTitle: 'Importación en curso',
      lastTitle: 'Última importación',
      startedLabel: 'Iniciada',
      endedLabel: 'Finalizada',
      startedBy: (name: string, own: boolean) =>
        own ? `Historial de reproducción de ${name}, desde Mi cuenta` : `Iniciada por ${name}`,
      runningHelp:
        'Puede salir de esta página: la importación continúa. Si Polyfin se reinicia mientras tanto, la importación se detiene: lo importado se conserva, y volver a importar no añade nada por duplicado.',
      stop: 'Detener importación',
      stopping: 'Deteniendo…',
      states: {
        running: 'En curso',
        done: 'Finalizada',
        stopped: 'Detenida',
        failed: 'Fallida',
      },
      problems: {
        jellyfin_unreachable: (server: string) =>
          `No se pudo contactar con ${server} durante la importación. Se conserva lo que ya se había importado.`,
        jellyfin_key_refused: (server: string) =>
          `${server} rechazó la clave API durante la importación: es posible que se haya revocado. Se conserva lo que ya se había importado.`,
        not_jellyfin: (server: string) =>
          `El servidor dejó de responder como ${server} durante la importación. Se conserva lo que ya se había importado.`,
        internal: () =>
          'Polyfin encontró un error durante la importación: el registro del servidor ofrece más detalles. Se conserva lo que ya se había importado.',
        jellyfin_user_forbidden: () =>
          'El servidor no permite que esta clave lea los datos de este usuario. Impórtelos con una clave API del panel de control del servidor o con la propia clave del usuario.',
      },
      errors: {
        emby: {
          invalid_jellyfin_address:
            'Introduzca la dirección del servidor Emby, por ejemplo http://192.168.1.10:8096.',
          jellyfin_key_refused:
            'Emby rechazó esta clave API. Cree una en sus ajustes, en API Keys, y péguela de nuevo.',
          jellyfin_sign_in_refused: 'Emby rechazó este nombre o esta contraseña.',
          jellyfin_sign_in_forbidden:
            'Emby no permite que esta cuenta inicie sesión: puede que esté desactivada o fuera de su horario permitido.',
          jellyfin_password_refused:
            'Emby rechazó esta contraseña. Déjela vacía si la cuenta no tiene ninguna.',
          jellyfin_unreachable:
            'Nada respondió en esta dirección. Compruébela y verifique que Emby esté en ejecución.',
          not_jellyfin:
            'Un servidor respondió en esta dirección, pero no es Emby. Compruebe la dirección.',
        },
        plex: {
          invalid_jellyfin_address:
            'Introduzca la dirección de Plex Media Server, por ejemplo http://192.168.1.10:32400.',
          jellyfin_key_refused: 'Plex rechazó este token. Copie de nuevo el token del propietario.',
          jellyfin_key_limited:
            'Este token no puede listar las cuentas del servidor: no es el del propietario. Use el token del propietario.',
          jellyfin_unreachable:
            'Nada respondió en esta dirección. Compruébela y verifique que Plex Media Server esté en ejecución.',
          not_jellyfin:
            'Un servidor respondió en esta dirección, pero no es Plex Media Server. Compruebe la dirección.',
        },
      } as Record<'emby' | 'plex', Partial<Record<string, string>>>,
      userStates: {
        waiting: 'En espera',
        reading: 'Leyendo',
        saving: 'Guardando',
        done: 'Finalizado',
        failed: 'Fallido',
      },
      notImported: 'No importado',
      userMapping: (jellyfinName: string, userName: string) => `${jellyfinName} → ${userName}`,
      read: (count: number) => (count === 1 ? '1 elemento leído' : `${count} elementos leídos`),
      counts: (played: number, resumed: number, favorites: number) =>
        `${played === 1 ? '1 marcado como reproducido' : `${played} marcados como reproducidos`}, ${resumed === 1 ? '1 punto de reanudación' : `${resumed} puntos de reanudación`}, ${favorites === 1 ? '1 favorito' : `${favorites} favoritos`}`,
      upToDate: (count: number) => `${count} ya en Polyfin`,
      ratings: (count: number) => (count === 1 ? '1 valorado' : `${count} valorados`),
      watchlist: (count: number) =>
        count === 1 ? '1 añadido a «Para ver»' : `${count} añadidos a «Para ver»`,
      undated: (count: number) =>
        count === 1
          ? '1 punto de reanudación sin fecha, no añadido'
          : `${count} puntos de reanudación sin fecha, no añadidos`,
      nothingAdded: {
        up_to_date: () => 'Todo lo leído ya estaba en Polyfin: no había nada que añadir.',
        nothing_played: (server: string) =>
          `${server} no muestra nada reproducido, en curso ni favorito para este usuario.`,
        no_sections: () =>
          'Este servidor Plex no tiene ninguna biblioteca de películas o series: no había nada que leer.',
        plex_owner_nothing_watched: () =>
          'El propietario no tiene nada reproducido ni en curso en las bibliotecas de películas y series de este servidor.',
        plex_nothing_watched: () =>
          'Esta cuenta no tiene nada reproducido, en curso ni valorado en las bibliotecas de películas y series de este servidor.',
        plex_no_history: () =>
          'Plex no muestra ninguna reproducción de esta cuenta en este servidor. Su historial de reproducción solo contiene los títulos reproducidos hasta el final en este servidor: los títulos marcados como vistos sin reproducirlos, o sincronizados desde la cuenta de Plex, no figuran en él. Para traerlo todo, puntos de reanudación incluidos, la persona inicia sesión con Plex en Mi cuenta › Importar desde otro servidor.',
      },
      unmatched: (count: number) =>
        count === 1 ? '1 título no encontrado' : `${count} títulos no encontrados`,
      unmatchedTitle: (name: string) => `Títulos no encontrados para ${name}`,
      unmatchedHelp: 'Estos títulos no se importaron: el motivo indica por qué.',
      titleColumn: 'Título',
      kindColumn: 'Tipo',
      yearColumn: 'Año',
      reasonColumn: 'Motivo',
      kinds: {
        movie: 'Película',
        episode: 'Episodio',
        series: 'Serie',
      },
      reasons: {
        no_identifier: 'Sin identificador de IMDb, TMDB o TVDB',
        not_found: 'No está en Polyfin',
        not_in_library: 'Ya no está en las bibliotecas de Plex',
        other_episode_order: 'Numerado según otro orden de episodios de TheTVDB',
      },
      more: (count: number) => (count === 1 ? 'Y 1 más.' : `Y ${count} más.`),
    },
    appImport: {
      open: 'Importar desde una aplicación',
      title: 'Importar desde una aplicación',
      description:
        'Traiga los datos de visionado de un usuario desde su cuenta en una aplicación basada en extensiones: las películas y episodios que ha reproducido con sus fechas, sus puntos de reanudación y la biblioteca de la aplicación como favoritos, identificados mediante IMDb, TMDB o Kitsu. A los datos de Polyfin solo se les añade información, y no se escribe nada en la aplicación. Necesita los datos de inicio de sesión del usuario en la aplicación; los usuarios también pueden importarlos ellos mismos, en «Mi cuenta».',
      user: 'En la cuenta de',
      userHelp:
        'El usuario de Polyfin a cuyos datos de visionado y extensiones se añade la importación.',
    },
    letterboxdImport: {
      open: 'Importar desde Letterboxd',
      title: 'Importar desde Letterboxd',
      description:
        'Traiga las películas de un usuario desde su archivo de exportación de Letterboxd: las películas que ha visto con sus fechas y número de reproducciones, su lista de seguimiento, sus valoraciones y, si lo desea, las películas que le han gustado como favoritos. A los datos de Polyfin solo se les añade información. Los usuarios también pueden importar los suyos, en «Mi cuenta».',
      user: 'En la cuenta de',
      userHelp: 'El usuario de Polyfin a cuyos datos de visionado se añade la importación.',
    },
  },
  invites: {
    title: 'Enlaces de invitación',
    create: 'Crear un enlace de invitación',
    empty: 'Todavía no hay ningún enlace de invitación.',
    emptyHelp:
      'Un enlace de invitación permite a las personas a las que se lo envíe crear su propia cuenta, con los ajustes que elija.',
    maxUses: 'Cuentas que puede crear',
    maxUsesHelp: 'De 1 a 100. 1 de forma predeterminada.',
    expires: 'Caduca',
    after: (days: number) => (days === 1 ? 'Después de 1 día' : `Después de ${days} días`),
    never: 'Nunca',
    accountsEnd: 'Fin de las cuentas',
    accountsEndHelp:
      'Cada cuenta que crea el enlace finaliza este número de días después de su creación.',
    accountsEndAfter: (days: number) =>
      days === 1
        ? 'Las cuentas finalizan después de 1 día'
        : `Las cuentas finalizan después de ${days} días`,
    model: 'Ajustes de',
    modelHelp:
      'Las cuentas nuevas copian los permisos, las bibliotecas visibles, los géneros bloqueados, las horas permitidas, el control parental, los límites de reproducción y acceso y el grupo de calidad de este usuario. Nunca son administradores.',
    newUser: 'Un usuario nuevo, como con «Crear un usuario»',
    submit: 'Crear enlace',
    creating: 'Creando…',
    createdTitle: 'Enlace de invitación creado',
    createdNotice: 'Copie este enlace ahora: solo se muestra esta vez.',
    copy: 'Copiar enlace',
    copied: 'Enlace copiado.',
    copyFailed: 'No se pudo copiar el enlace. Selecciónelo y cópielo manualmente.',
    done: 'Hecho',
    uses: (uses: number, maxUses: number) =>
      maxUses === 1 ? `${uses} de 1 cuenta creada` : `${uses} de ${maxUses} cuentas creadas`,
    settingsOf: (name: string) => `Ajustes de ${name}`,
    newUserSettings: 'Ajustes de usuario nuevo',
    expiresLabel: 'Caduca',
    expiredLabel: 'Caducado',
    neverExpires: 'No caduca nunca',
    createdBy: (name: string) => `Creado por ${name}`,
    createdByDeleted: 'Creado por un usuario borrado',
    states: {
      active: 'Activo',
      used_up: 'Agotado',
      expired: 'Caducado',
      revoked: 'Revocado',
    },
    revoke: 'Revocar',
    revokeTitle: '¿Revocar este enlace de invitación?',
    revokeConfirm:
      'El enlace deja de funcionar de inmediato. Se conservan las cuentas ya creadas con él.',
    revoking: 'Revocando…',
    revoked: 'Se ha revocado el enlace de invitación.',
  },
  groups: {
    tabsLabel: 'Usuarios y grupos',
    title: 'Grupos',
    description:
      'Un grupo reúne los límites de la página de un usuario. Sus miembros lo siguen: un cambio en el grupo les llega a todos a la vez.',
    listTitle: 'Grupos',
    empty: 'Aún no hay ningún grupo.',
    emptyHelp:
      'Cree un grupo para definir a la vez los permisos, los límites de reproducción, el control parental, las bibliotecas, los géneros y los horarios de varias cuentas.',
    create: 'Crear un grupo',
    name: 'Nombre',
    nameTaken: 'Este nombre ya lo usa otro grupo.',
    nameRule: 'De 1 a 64 caracteres, distinto del nombre de los demás grupos.',
    copyFrom: 'Ajustes de',
    copyFromHelp:
      'El grupo empieza con los permisos, los límites de reproducción y de acceso, el grupo de calidad, el control parental, las bibliotecas visibles, los géneros bloqueados y los horarios permitidos de este usuario.',
    submit: 'Crear grupo',
    creating: 'Creando…',
    created: (name: string) => `Se ha creado el grupo ${name}.`,
    memberCount: (count: number) =>
      count === 0 ? 'Ningún miembro' : count === 1 ? '1 miembro' : `${count} miembros`,
    notFound: 'No se ha encontrado el grupo',
    notFoundHelp: 'Es posible que se haya borrado.',
    backToGroups: 'Volver a los grupos',
    sectionsLabel: 'Secciones del grupo',
    pageHelp:
      'Cada sección se guarda por separado y llega de inmediato a los miembros que siguen el grupo.',
    rename: 'Cambiar nombre',
    renamed: 'Se ha cambiado el nombre del grupo.',
    updated: 'Se ha actualizado el grupo.',
    accessHelp:
      'Los permisos que los miembros reciben del grupo. Su rol, la pantalla de inicio de sesión, «Desactivado» y la fecha de fin se mantienen en su propia página.',
    delete: 'Borrar grupo',
    deleteTitle: (name: string) => `¿Borrar el grupo ${name}?`,
    deleteConfirm:
      'Sus miembros conservan los valores del grupo como ajustes propios, que luego se pueden cambiar en sus páginas.',
    deleting: 'Borrando…',
    deleted: (name: string) => `Se ha borrado el grupo ${name}.`,
    membersTitle: 'Miembros',
    membersHelp:
      'Un miembro que deja de seguir el grupo conserva sus valores como ajustes propios, marcados como «Ajustes propios».',
    noMembers: 'Aún no hay ningún miembro.',
    noMembersHelp:
      'Añada miembros aquí o elija el grupo en la sección Acceso de la página de un usuario.',
    ownSettings: 'Ajustes propios',
    followsHelp: 'Sigue el grupo',
    ownSettingsHelp: 'Conserva sus propios ajustes',
    addMembers: 'Añadir miembros',
    addMembersTitle: (name: string) => `Añadir miembros a ${name}`,
    addMembersHelp:
      'Los usuarios elegidos siguen el grupo: adoptan sus valores en lugar de los suyos. Los administradores no pueden unirse a un grupo.',
    noCandidates: 'No hay nadie que añadir.',
    noCandidatesHelp: 'Todos los usuarios que no son administradores ya están en este grupo.',
    movesFrom: (name: string) => `Sale del grupo ${name}`,
    add: 'Añadir',
    adding: 'Añadiendo…',
    membersAdded: (count: number) =>
      count === 1 ? 'Se ha añadido 1 miembro.' : `Se han añadido ${count} miembros.`,
    group: 'Grupo',
    noGroup: 'Ningún grupo',
    groupHelp:
      'Un miembro sigue el grupo: se aplican sus valores y esta página los muestra en modo de solo lectura. Al salir, el usuario los conserva como propios.',
    administratorNoGroup:
      'Los administradores no pueden unirse a un grupo, ya que sus permisos son distintos.',
    administratorLeaves: (name: string) =>
      `Convertir a este usuario en administrador lo saca del grupo ${name}.`,
    newAccountGroupHelp: 'Las cuentas creadas se unen a este grupo y lo siguen.',
    newUserGroupHelp: 'El usuario se une a este grupo y lo sigue.',
    followGroup: 'Seguir el grupo',
    followGroupHelp:
      'Si se desactiva, el usuario conserva los valores actuales del grupo como propios y se pueden cambiar en esta página.',
    setByGroup: (name: string) => `Definido por el grupo ${name}.`,
    openGroup: 'Abrir el grupo',
    joinTitle: '¿Unirse al grupo?',
    joinConfirm: (user: string, group: string) =>
      `${user} se une a ${group} y lo sigue: los valores del grupo sustituyen a los suyos en Acceso, Reproducción y acceso, Control parental, Bibliotecas visibles, Géneros bloqueados y Horarios permitidos.`,
    join: 'Unirse',
    followAgainTitle: '¿Volver a seguir el grupo?',
    followAgainConfirm: (user: string, group: string) =>
      `${user} recupera los valores de ${group}: se sustituyen sus ajustes propios en las secciones del grupo.`,
    followAgain: 'Seguir el grupo',
    groupBadge: (name: string) => `Grupo ${name}`,
    groupOwnBadge: (name: string) => `Grupo ${name}, ajustes propios`,
    filter: 'Mostrar los usuarios de',
    allGroups: 'Todos los grupos',
    noneShown: 'No hay ningún usuario aquí.',
  },
  resetDefaults: {
    button: 'Restablecer los valores predeterminados',
    title: (name: string) => `¿Restablecer los ajustes de ${name}?`,
    groupTitle: (name: string) => `¿Restablecer los ajustes del grupo ${name}?`,
    userHelp: (name: string) =>
      `Los permisos de Acceso, Reproducción y acceso con el Grupo de calidad, Control parental, Bibliotecas visibles, Géneros bloqueados y Horario permitido vuelven a los ajustes de un usuario nuevo, los que da «Crear un usuario»; las aplicaciones de ${name} ya conectadas los tienen al instante. Su nombre y contraseña, Administrador, Mostrar en la pantalla de inicio de sesión, Desactivado, Finaliza el, su grupo, sus propios addons, conexiones, cuentas de seguimiento y todo lo que ha visto se mantienen.`,
    ownSettingsHelp: (group: string) =>
      `Sigue en el grupo ${group} con sus propios ajustes, sin seguirlo.`,
    groupHelp:
      'Los permisos de Acceso, Reproducción y acceso con el Grupo de calidad, Control parental, Bibliotecas visibles, Géneros bloqueados y Horario permitido vuelven a los ajustes de un usuario nuevo. El nombre y los miembros del grupo se mantienen.',
    followers: (count: number) =>
      count === 0
        ? 'Ningún miembro sigue el grupo: el cambio no llega a nadie.'
        : count === 1
          ? '1 miembro sigue el grupo y recibe estos ajustes al instante.'
          : `${count} miembros siguen el grupo y reciben estos ajustes al instante.`,
    changesTitle: 'Qué cambia',
    loading: 'Buscando qué cambia…',
    noChange: 'No cambia nada: ya son los ajustes de un usuario nuevo.',
    confirm: 'Restablecer',
    resetting: 'Restableciendo…',
    done: (name: string) => `Se han restablecido los ajustes de ${name}.`,
    groupDone: (name: string) => `Se han restablecido los ajustes del grupo ${name}.`,
  },
}

export default users
