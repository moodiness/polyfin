import type en from '../en/account'

const account: typeof en = {
  account: {
    title: 'Mi cuenta',
    signedInAs: 'Sesión iniciada como ',
    sectionsLabel: 'Secciones de la cuenta',
    sections: {
      statistics: 'Estadísticas',
      tracking: 'Seguimiento',
      resume: 'Seguir viendo',
      hidden: 'Títulos ocultos',
      homeRows: 'Filas de inicio',
      metadataLanguage: 'Idioma de los metadatos',
      sharedLinks: 'Enlaces compartidos',
      reminders: 'Recordatorios',
      import: 'Importar desde otro servidor',
      appImport: 'Importar desde una aplicación',
      notifications: 'Notificaciones',
      devices: 'Dispositivos',
      password: 'Contraseña',
      twoStep: 'Inicio de sesión en dos pasos',
    },
    metadataLanguage: {
      help: 'El idioma de los nombres, las sinopsis y las imágenes de los títulos en sus aplicaciones, cuando alguna de las extensiones del servidor los describe en ese idioma; los títulos que ninguna extensión describe en él conservan la descripción del servidor. Los géneros, las clasificaciones y las personas son iguales para todos. Los nombres que crea el propio Polyfin, como las temporadas, lo siguen cuando Polyfin lo habla: inglés, francés, alemán, español, italiano o portugués.',
      label: 'Idioma de los metadatos',
      server: 'El del servidor',
      noLanguage:
        'Ninguna extensión del servidor tiene idioma todavía, así que los títulos se describen en el del servidor. Un administrador asigna un idioma a las extensiones en Contenido › Fuentes.',
      saved: 'Idioma de los metadatos guardado.',
    },
    homeRows: {
      help: 'Las filas que el administrador añade para usted a la página de inicio del reproductor web. Desactive una para quitarla de su página de inicio, y de sus bibliotecas en otras aplicaciones cuando también sea una allí. Las secciones propias del reproductor web siguen en sus Ajustes › Inicio.',
      empty: 'Ninguna fila de inicio',
      emptyHelp:
        'El administrador no ha añadido ninguna fila para usted a la página de inicio del reproductor web.',
      shown: (name: string) => `Mostrar ${name} en su página de inicio`,
      on: (name: string) => `${name} vuelve a estar en su página de inicio.`,
      off: (name: string) => `${name} se ha quitado de su página de inicio.`,
    },
    watching: {
      resumeHelp:
        'Los títulos en curso de su lista «Seguir viendo», para las aplicaciones que no permiten quitar uno. «Eliminar» quita un título sin cambiar si está reproducido o no. Una importación desde un servicio de seguimiento o una aplicación solo lo recupera con un punto de reanudación más reciente.',
      hiddenHelp:
        'Los títulos que ha ocultado de sus bibliotecas, filas de inicio, búsquedas y títulos similares. Siguen abriéndose desde un enlace. En el reproductor web, el menú ⋮ de un título lo oculta.',
      remove: 'Eliminar',
      removeLabel: (name: string) => `Eliminar ${name} de «Seguir viendo»`,
      removed: (name: string) => `${name} se ha eliminado de «Seguir viendo».`,
      resumeEmpty: 'Nada en curso',
      resumeEmptyHelp:
        'Las películas y los episodios que empiece se muestran aquí hasta que los termine.',
      progress: (position: string, runtime: string) => `${position} de ${runtime}`,
      showAgain: 'Volver a mostrar',
      showLabel: (name: string) => `Volver a mostrar ${name}`,
      shown: (name: string) => `${name} vuelve a estar en sus listas.`,
      hiddenEmpty: 'Ningún título oculto',
      hiddenEmptyHelp: 'Oculte un título desde su menú ⋮ en el reproductor web.',
      hiddenOn: 'Oculto',
      kinds: {
        movie: 'Película',
        series: 'Serie',
        episode: 'Episodio',
        video: 'Vídeo casero',
        replay: 'Replay',
      } as Record<string, string>,
    },
    reminders: {
      help: 'Los programas de Televisión en vivo de los que pidió un recordatorio desde su página en el reproductor web. Cada recordatorio llega a sus destinos que eligieron «Inicio de un programa», con la antelación configurada en Notificaciones.',
      empty: 'Ningún recordatorio',
      emptyHelp:
        'En la página de un próximo programa en el reproductor web, «Recordármelo» crea uno.',
      everyAiring: (time: string) => `Cada emisión hacia las ${time}`,
      next: 'Próxima ',
      noAiring: 'Ninguna emisión en la guía',
      delete: 'Borrar',
      deleteLabel: (name: string) => `Borrar el recordatorio de ${name}`,
      deleted: (name: string) => `Se ha borrado el recordatorio de ${name}.`,
      settingsTitle: 'Recordatorios',
      before: 'Recordarme con antelación de',
      beforeHelp: 'Cuánto tiempo antes del inicio de un programa llega su recordatorio.',
      minutes: (minutes: number) =>
        minutes === 1 ? '1 minuto' : minutes === 60 ? '1 hora' : `${minutes} minutos`,
      onApps: 'Mostrar también los recordatorios en mis aplicaciones',
      onAppsHelp:
        'Cada recordatorio también se muestra en sus aplicaciones de Jellyfin que aceptan mensajes, como el reproductor web mientras está abierto, igual que «Enviar un mensaje».',
      noTarget:
        'Ninguno de sus destinos eligió «Inicio de un programa» y sus aplicaciones están desactivadas: sus recordatorios no le llegarían a ningún sitio.',
      saved: 'Configuración de recordatorios guardada.',
    },
    importEvery: (minutes: number) =>
      minutes < 60
        ? `cada ${minutes} minutos`
        : minutes === 60
          ? 'cada hora'
          : `cada ${minutes / 60} horas`,
    serverImport: {
      description:
        'Traiga sus películas y episodios vistos, puntos de reanudación y favoritos desde un servidor Jellyfin o Emby que usaba antes, o sus películas desde su exportación de Letterboxd. Se añaden solo a su cuenta, y solo completan lo que Polyfin ya tiene. No se escribe nada en el otro servidor.',
      serverKind: 'Servidor',
      address: 'Dirección del servidor',
      addressHelp:
        'La dirección con la que se abre el servidor en un navegador, por ejemplo http://192.168.1.10:8096.',
      name: 'Su nombre de usuario en ese servidor',
      password: 'Su contraseña en ese servidor',
      passwordHelp:
        'Se usa solo para esta importación y nunca se guarda. Polyfin inicia sesión con su cuenta, como hacen las aplicaciones del servidor, y después cierra sesión. Déjela vacía si su cuenta no tiene contraseña.',
      start: 'Importar mi historial de reproducción',
      starting: 'Iniciando…',
      started: 'La importación ha comenzado. Puede salir de esta página: continuará.',
      lastTitle: 'Su última importación',
      from: (server: string) => `Desde ${server}`,
    },
    letterboxd: {
      option: 'Letterboxd (archivo de exportación)',
      help: 'En Letterboxd, abra Settings › Import & Export y elija Export your data: Letterboxd le da un archivo ZIP. Selecciónelo aquí tal cual. Polyfin lo lee en memoria y nunca lo guarda.',
      watchedHelp:
        'Las películas que ha visto se marcan como vistas, en la fecha más reciente que indica su diario, tantas veces como las registre. Cada película se busca por su nombre y año en las extensiones de metadatos del servidor, y se conserva cuando estas conocen exactamente una película con ese nombre y año.',
      noSearch:
        'Ninguna extensión de metadatos de este servidor busca películas: no se puede encontrar ninguna película de la exportación. Pida a un administrador que añada una.',
      file: 'Archivo de exportación',
      fileHelp: 'El archivo ZIP que da Letterboxd, de 20 MB como máximo.',
      choose: 'Elegir el archivo',
      chooseAnother: 'Elegir otro archivo',
      tooLarge: 'Este archivo supera los 20 MB: elija el archivo ZIP que da Letterboxd.',
      watchlist: 'Watchlist a mi lista «Para ver»',
      watchlistHelp:
        'Las películas de su watchlist de Letterboxd se añaden a su lista «Para ver» en las aplicaciones de Jellyfin, hasta que las empiece.',
      ratings: 'Valoraciones',
      ratingsHelp:
        'Sus estrellas se convierten en su valoración sobre 10: 3,5 estrellas son un 7. Las valoraciones que ya tenga en Polyfin se mantienen.',
      likes: 'Películas que le gustan como favoritos',
      likesHelp: 'Las películas que le gustaron en Letterboxd pasan a ser sus favoritos.',
      forUser: {
        watchedHelp:
          'Las películas que ha visto el usuario se marcan como vistas, en la fecha más reciente que indica su diario, tantas veces como las registre. Cada película se busca por su nombre y año en las extensiones de metadatos del servidor, y se conserva cuando estas conocen exactamente una película con ese nombre y año.',
        watchlist: 'Watchlist a su lista «Para ver»',
        watchlistHelp:
          'Las películas de su watchlist de Letterboxd se añaden a su lista «Para ver» en las aplicaciones de Jellyfin, hasta que el usuario las empiece.',
        ratingsHelp:
          'Sus estrellas se convierten en su valoración sobre 10: 3,5 estrellas son un 7. Las valoraciones que el usuario ya tenga en Polyfin se mantienen.',
        likesHelp: 'Las películas que le gustaron en Letterboxd pasan a ser sus favoritos.',
      },
      start: 'Importar desde Letterboxd',
      starting: 'Enviando…',
      started: 'La importación ha comenzado. Puede salir de esta página: continuará.',
      currentTitle: 'Importación en curso',
      lastTitle: 'Última importación',
      ownLastTitle: 'Su última importación',
      from: 'Desde una exportación de Letterboxd',
      startedLabel: 'Iniciada',
      endedLabel: 'Finalizada',
      startedBy: (name: string) => `Iniciada por ${name}`,
      states: { running: 'En curso', done: 'Completada', failed: 'Fallida' },
      problem:
        'Polyfin encontró un error durante la importación: el registro del servidor ofrece más detalles. Lo que se importó se conserva.',
      runningHelp:
        'Puede salir de esta página: la importación continúa. Si Polyfin se reinicia mientras tanto, la importación se detiene: lo importado se conserva, y volver a importar no duplica nada.',
      searching: (searched: number, movies: number) =>
        `Buscando las películas: ${searched} de ${movies}.`,
      searchingLabel: 'Películas buscadas',
      summary: (movies: number, added: string) =>
        `${movies === 1 ? '1 película' : `${movies} películas`}: ${added}.`,
      played: (count: number) =>
        count === 1 ? '1 marcada como vista' : `${count} marcadas como vistas`,
      addedToWatchlist: (count: number) =>
        count === 1 ? '1 añadida a «Para ver»' : `${count} añadidas a «Para ver»`,
      rated: (count: number) => (count === 1 ? '1 valorada' : `${count} valoradas`),
      favorites: (count: number) =>
        count === 1 ? '1 añadida a favoritos' : `${count} añadidas a favoritos`,
      unmatched: (count: number) =>
        count === 1 ? '1 película no encontrada' : `${count} películas no encontradas`,
      unmatchedTitle: 'Películas no encontradas',
      unmatchedHelp:
        'Estas películas no se importaron. Cada una se busca por su nombre y año, y solo se conserva cuando las extensiones de metadatos del servidor conocen exactamente una película con ese nombre y año. Ábralas en Letterboxd para comprobarlas.',
      movieColumn: 'Película',
      reasonColumn: 'Motivo',
      onLetterboxd: 'En Letterboxd',
      reasons: {
        not_found: 'No encontrada',
        ambiguous: 'Varias películas con ese nombre y año',
        search_failed: 'Error en la búsqueda',
      },
    },
    appImport: {
      description:
        'Traiga lo que vio en una aplicación basada en extensiones: las películas y episodios reproducidos con sus fechas, sus puntos de reanudación y la biblioteca de la aplicación, que pasa a su lista «Para ver» en las aplicaciones de Jellyfin, identificados por IMDb, TMDB o Kitsu. Solo se añaden a lo que Polyfin ya tiene, y no se escribe nada en la aplicación.',
      app: 'Aplicación',
      apps: { nuvio: 'Nuvio', stremio: 'Stremio' },
      backend: 'Servidor de Nuvio',
      backendHelp:
        'Déjelo vacío para usar el de Nuvio. También funciona un servidor autoalojado, en su dirección, incluso en la red local.',
      email: 'Correo electrónico',
      password: 'Contraseña',
      passwordHelp:
        'Solo se usa para leer la cuenta y nunca se guarda. Polyfin inicia sesión como lo hacen las aplicaciones de Nuvio, lee la cuenta y después cierra la sesión, salvo que permanezca conectado.',
      signIn: 'Iniciar sesión en Nuvio',
      signingIn: 'Iniciando sesión…',
      chooseTitle: 'Elegir qué importar',
      chooseHelp: {
        nuvio:
          'Nuvio guarda un historial por perfil: elija el que desea importar. Polyfin conserva lo que ha leído solo unos minutos.',
        stremio:
          'Stremio guarda un único historial para toda la cuenta. Polyfin conserva lo que ha leído solo unos minutos.',
      },
      stayConnected: 'Permanecer conectado',
      stayConnectedHelp: {
        nuvio: (every: string) =>
          `Polyfin conserva la sesión, cifrada, nunca su contraseña, y vuelve a importar el perfil elegido ${every}. Solo lee la cuenta. Puede desconectarse en cualquier momento.`,
        stremio: (every: string) =>
          `Polyfin conserva la clave de autenticación, cifrada, y vuelve a importar la cuenta ${every}. Solo lee la cuenta. Puede desconectarse en cualquier momento. Las condiciones de Stremio prohíben extraer sus datos por medios automatizados: leer su cuenta ${every} acentúa esa zona gris más que una importación puntual.`,
      },
      staysConnected: (every: string) =>
        `Permanece conectado: este perfil se vuelve a importar ${every}, de la misma forma. Las extensiones solo se instalan ahora.`,
      wholeAccount: 'Toda la cuenta',
      profile: 'Perfil',
      profileNumber: (id: string) => `Perfil ${id}`,
      profileCounts: (played: number, resumes: number, library: number) =>
        `${played} vistos, ${resumes === 1 ? '1 punto de reanudación' : `${resumes} puntos de reanudación`}, ${library} en la biblioteca.`,
      addons: 'Extensiones',
      addonsHelp:
        'Cada extensión marcada pasa a ser una extensión propia de la cuenta, en «Mis fuentes». Su dirección puede contener ajustes y claves: Polyfin nunca la muestra completa.',
      addonsOff:
        'Las extensiones propias están desactivadas para esta cuenta: no se ofrecen las extensiones de la aplicación.',
      noAddons: 'Este perfil no tiene ninguna extensión.',
      back: 'Volver a iniciar sesión',
      start: 'Importar este perfil',
      starting: 'Iniciando…',
      started: 'La importación ha comenzado. Puede salir de esta página: continuará.',
      currentTitle: 'Importación en curso',
      lastTitle: 'Última importación',
      ownLastTitle: 'Su última importación',
      from: (app: string, profile: string) =>
        profile === '' ? `Desde ${app}` : `Desde ${app}, perfil ${profile}`,
      startedLabel: 'Iniciada',
      endedLabel: 'Finalizada',
      startedBy: (name: string) => `Iniciada por ${name}`,
      states: { running: 'En curso', done: 'Completada', failed: 'Fallida' },
      problem:
        'Polyfin ha encontrado un error durante la importación: el registro del servidor ofrece más detalles. Lo importado se conserva.',
      runningHelp:
        'Puede salir de esta página: la importación continúa. Si Polyfin se reinicia mientras tanto, la importación se detiene: lo importado se conserva, y volver a importar no añade nada dos veces.',
      read: (count: number) => (count === 1 ? '1 título leído.' : `${count} títulos leídos.`),
      counts: (read: number, played: number, resumed: number, watchlist: number) =>
        `${read === 1 ? '1 título leído' : `${read} títulos leídos`}: ${played === 1 ? '1 marcado como reproducido' : `${played} marcados como reproducidos`}, ${resumed === 1 ? '1 punto de reanudación' : `${resumed} puntos de reanudación`}, ${watchlist === 1 ? '1 añadido' : `${watchlist} añadidos`} a la lista «Para ver».`,
      unmatched: (count: number) =>
        count === 1 ? '1 título no encontrado' : `${count} títulos no encontrados`,
      unmatchedTitle: 'Títulos no encontrados',
      unmatchedHelp:
        'Estos títulos no se han importado: la aplicación los identifica con un identificador que Polyfin no reconoce, o ningún título de Polyfin lo tiene. En el caso de una serie, la aplicación también puede indicar los episodios vistos por su posición en una lista de episodios que las extensiones de Polyfin ordenan de otra forma.',
      titleColumn: 'Título',
      kindColumn: 'Tipo',
      reasonColumn: 'Motivo',
      reasons: {
        other_identifier: 'No es un identificador de IMDb, TMDB o Kitsu',
        not_found: 'No está en Polyfin',
        episode_list:
          'Sus episodios no coinciden con la lista de Polyfin: no se ha marcado ninguno como reproducido',
      },
      addonInstalled: 'añadida',
      addonWaiting: 'en espera',
      addonProblems: {
        not_allowed: 'no añadida: las extensiones propias estaban desactivadas',
        exists: 'ya instalada',
        invalid_address: 'no añadida: no es la dirección de una extensión',
        private_network:
          'no añadida: está en una red local, a la que solo llega la importación de un administrador',
        unreachable: 'no añadida: la extensión no respondió',
        invalid_manifest: 'no añadida: no es una extensión',
        internal: 'no añadida: Polyfin ha encontrado un error',
      },
      stremio: {
        way: 'Iniciar sesión con',
        ways: {
          link: 'Un código de vinculación',
          password: 'Correo electrónico y contraseña',
          key: 'Una clave de autenticación',
        },
        linkHelp:
          'Polyfin pide un código a Stremio, como hacen las aplicaciones de TV de Stremio. Usted lo aprueba en un navegador con la sesión de Stremio iniciada: funciona sea cual sea la forma en que inicia sesión su cuenta, incluidas Facebook y Apple, y Polyfin nunca ve su contraseña.',
        getCode: 'Obtener un código de vinculación',
        gettingCode: 'Solicitando a Stremio…',
        codeLabel: 'Código de vinculación',
        approve:
          'Abra el enlace en un navegador con la sesión de Stremio iniciada, compruebe que muestra este código y apruébelo.',
        openLink: 'Abrir el enlace',
        waiting:
          'Esperando su aprobación. El código dura 5 minutos; una vez aprobado, Polyfin lee la cuenta y después cierra la sesión, salvo que permanezca conectado.',
        cancel: 'Cancelar',
        email: 'Correo electrónico',
        password: 'Contraseña',
        passwordHelp:
          'Solo se usa para leer la cuenta y nunca se guarda. Polyfin inicia sesión como lo hacen las aplicaciones de Stremio, lee la cuenta y después cierra la sesión, salvo que permanezca conectado. Una cuenta creada con Facebook o Apple no tiene contraseña: use un código de vinculación.',
        signIn: 'Iniciar sesión en Stremio',
        signingIn: 'Iniciando sesión…',
        key: 'Clave de autenticación',
        keyHelp:
          'La clave que guarda la aplicación web de Stremio al iniciar sesión: en la consola de su navegador, en esa aplicación, JSON.parse(localStorage.getItem("profile")).auth.key la devuelve. Se usa para leer la cuenta y solo se guarda si permanece conectado. Polyfin nunca cierra su sesión, ya que eso cerraría la sesión de su navegador.',
        readKey: 'Leer la cuenta',
        reading: 'Leyendo…',
      },
      connections: {
        title: 'Aplicaciones conectadas',
        help: (every: string) =>
          `Se importan ${every}, igual que en la primera importación: el mismo perfil, sin sus extensiones. Polyfin solo las lee.`,
        connectedAs: 'Conectado como ',
        connected: 'Conectado',
        connectedOn: (date: string) => ` desde el ${date}`,
        connectedBy: (name: string) => ` por ${name}`,
        status: {
          connected: 'Conectado',
          importing: 'Importando',
          paused: 'En pausa',
          stopped: 'Volver a iniciar sesión',
        },
        never: 'Aún no se ha importado.',
        lastImport: 'Última importación ',
        colon: ': ',
        nextImport: 'Próxima importación ',
        paused:
          'El servidor ya no permite a los usuarios importar su historial de reproducción: esta conexión está en pausa hasta que vuelva a permitirlo.',
        stopped: (app: string) =>
          `${app} rechazó la sesión que Polyfin guardaba: no se importará nada hasta que vuelva a iniciar sesión más abajo, con «Permanecer conectado» marcado. Al desconectar, se olvida.`,
        problems: {
          unreachable: (app: string) =>
            `${app} no respondió en la última importación. La próxima lo volverá a intentar.`,
          rate_limited: (app: string) =>
            `${app} pidió a Polyfin que esperase en la última importación. La próxima lo volverá a intentar.`,
          unexpected: (app: string) =>
            `${app} respondió a la última importación de una forma que esta versión de Polyfin no sabe leer. La próxima lo volverá a intentar.`,
          profile_missing: (app: string) =>
            `El perfil importado ya no está en la cuenta de ${app}. Vuelva a iniciar sesión para elegir otro.`,
          sign_in_refused: (app: string) => `${app} rechazó la sesión que Polyfin guardaba.`,
          internal: () =>
            'Polyfin encontró un error durante la última importación: el registro del servidor ofrece más detalles. La próxima lo volverá a intentar.',
        },
        importNow: 'Importar ahora',
        importing: 'Importando…',
        disconnect: 'Desconectar',
        disconnectTitle: (app: string) => `¿Desconectar ${app}?`,
        disconnectBody: (app: string) =>
          `Polyfin dejará de importar esta cuenta de ${app} y olvidará lo que guardaba para leerla, tras cerrar la sesión que abrió. Si pegó una clave de autenticación, solo se olvidará. Lo que ya se importó se conserva.`,
        disconnected: (app: string) => `${app} desconectado.`,
      },
    },
    sharedLinksHelp:
      'Comparta una película o un episodio con alguien que no tiene cuenta: lo ve en su navegador mediante un enlace que caduca, y cuenta entre sus reproducciones simultáneas.',
    statisticsHelp:
      'Lo que ha visto en este servidor: horas, títulos, aplicaciones y dispositivos, y sus reproducciones recientes.',
    notificationsHelp:
      'Dónde Polyfin le avisa de los nuevos episodios de las series que sigue, de sus grabaciones y reproducciones, de sus conexiones que se interrumpen, de los bloqueos de su cuenta y de sus inicios de sesión en dispositivos nuevos, le recuerda los programas que eligió, y le envía su resumen semanal y su resumen del mes y del año.',
    devicesHelp:
      'Las aplicaciones con sesión iniciada en su cuenta. Un dispositivo cuya sesión cierre le pedirá la contraseña la próxima vez que se abra.',
    passwordHelp: 'Cambiar la contraseña cierra la sesión en todos sus dispositivos Jellyfin.',
    currentPassword: 'Contraseña actual',
    newPassword: 'Nueva contraseña',
    confirmPassword: 'Confirmar nueva contraseña',
    changePassword: 'Cambiar contraseña',
    changingPassword: 'Cambiando…',
    passwordChanged: 'Se ha cambiado la contraseña.',
    twoStep: {
      help: 'Después de la contraseña, esta aplicación de administración pide un código de una aplicación de autenticación de su teléfono. Las aplicaciones de Jellyfin no pueden pedirlo: una vez activado, inician sesión con Quick Connect, que usted aprueba aquí, y su contraseña por sí sola ya no les permite iniciar sesión. Las aplicaciones y navegadores que ya tienen la sesión iniciada la mantienen.',
      off: 'Desactivado',
      on: 'Activado',
      offText: 'Inicia sesión solo con su contraseña.',
      onText:
        'Inicia sesión con su contraseña y, después, con un código de su aplicación de autenticación.',
      required:
        'Los administradores deben usar el inicio de sesión en dos pasos en este servidor. Actívelo ahora, o se le pedirá en su próximo inicio de sesión.',
      requiredOn:
        'Los administradores deben usar el inicio de sesión en dos pasos en este servidor: permanece activado.',
      unreadable:
        'POLYFIN_SECRET_KEY ya no puede leer la clave de su aplicación de autenticación, como ocurre tras una restauración con otra clave: solo sus códigos de recuperación le permiten iniciar sesión. Desactive el inicio de sesión en dos pasos con su contraseña y vuelva a activarlo.',
      turnOn: 'Activar',
      starting: 'Preparando…',
      scan: 'Escanee este código QR con su aplicación de autenticación, o introduzca la clave a mano. Polyfin ha generado el código por sí mismo: la clave no se envía a ningún otro sitio.',
      qrLabel: 'Código QR de la clave, para su aplicación de autenticación',
      key: 'Clave',
      openInApp: 'Abrir en una aplicación de autenticación de este dispositivo',
      code: 'Código de la aplicación',
      codeHelp:
        'El código de 6 dígitos que la aplicación muestra para Polyfin. Cambia cada 30 segundos.',
      confirm: 'Activar',
      confirming: 'Comprobando…',
      turnedOn: 'El inicio de sesión en dos pasos está activado.',
      recoveryTitle: 'Sus códigos de recuperación',
      recoveryHelp:
        'Cada código le permite iniciar sesión una vez, en lugar de un código de su aplicación, por si pierde el teléfono. Guárdelos en un lugar seguro, como un gestor de contraseñas: solo se muestran ahora.',
      copy: 'Copiar los códigos',
      copied: 'Códigos copiados.',
      copyFailed: 'No se pudieron copiar los códigos: selecciónelos y cópielos a mano.',
      saved: 'Ya los he guardado',
      codesLeft: (n: number) =>
        n === 1 ? 'Queda 1 código de recuperación' : `Quedan ${n} códigos de recuperación`,
      newCodes: 'Nuevos códigos de recuperación',
      newCodesTitle: '¿Generar nuevos códigos de recuperación?',
      newCodesText: 'Los nuevos códigos anulan los anteriores.',
      making: 'Generando…',
      turnOff: 'Desactivar',
      turnOffTitle: '¿Desactivar el inicio de sesión en dos pasos?',
      turnOffText:
        'Entonces iniciará sesión solo con su contraseña, aquí y en las aplicaciones de Jellyfin.',
      turningOff: 'Desactivando…',
      turnedOff: 'El inicio de sesión en dos pasos está desactivado.',
      confirmWith: 'Confirmar con',
      withCode: 'Un código',
      withPassword: 'Mi contraseña',
      codeOrRecovery: 'Código de la aplicación o código de recuperación',
      password: 'Contraseña',
    },
    tracking: {
      title: 'Seguimiento',
      description:
        'Conecte los servicios que registran lo que ve y escucha. Polyfin informa a cada servicio conectado de las películas y los episodios que ve, o de las canciones que reproduce, en sus aplicaciones de Jellyfin.',
      loading: 'Cargando servicios de seguimiento…',
      status: {
        connected: 'Conectado',
        notConnected: 'No conectado',
        waiting: 'Esperando el código',
        waitingSignIn: 'Esperando su autorización',
        reconnect: 'Hay que volver a conectar',
        unreachable: 'Reintentando',
        unavailable: 'No configurado',
        appRefused: 'Aplicación rechazada',
      },
      unavailable: (name: string) =>
        `${name} aún no está configurado en este servidor: primero, un administrador debe añadir la aplicación de ${name} en los ajustes.`,
      setUpApp: (name: string) => `Configurar la aplicación de ${name}`,
      codeIntro: (name: string) =>
        `Conecte su cuenta de ${name}: Polyfin le da un código que debe introducir en el sitio de ${name}.`,
      keyIntro: (name: string) => `Conecte su cuenta de ${name} con su clave API de ${name}.`,
      signInIntro: (name: string) =>
        `Conecte su cuenta de ${name}: inicie sesión en el sitio de ${name} y permita que Polyfin registre (scrobble) las canciones que reproduce.`,
      tokenIntro: (name: string) =>
        `Conecte su cuenta de ${name} con su token de usuario de ${name} para que se registren las canciones que reproduce.`,
      keyLabel: (name: string) => `Clave API de ${name}`,
      tokenLabel: (name: string) => `Token de usuario de ${name}`,
      savedKeyHelp:
        'Solo usted puede mostrarla. Para usar otra clave, desconecte y vuelva a conectar.',
      keyHelp: {
        mdblist: 'La encontrará en mdblist.com, en Preferences, en el apartado API key.',
        publicmetadb: 'La encontrará en su cuenta de PublicMetaDB.',
        listenbrainz:
          'Lo encontrará en listenbrainz.org, en sus ajustes, en el apartado User token.',
      },
      connect: 'Conectar',
      connecting: 'Conectando…',
      checking: 'Comprobando la clave…',
      reconnect: 'Volver a conectar',
      newCode: 'Obtener un código nuevo',
      signInAgain: 'Volver a empezar',
      enterCode: (site: string) => `Vaya a ${site}, inicie sesión e introduzca este código:`,
      codeLabel: 'Código que debe introducir',
      openSite: (site: string) => `Abrir ${site}`,
      waiting: 'Esta página se actualiza sola en cuanto lo haya introducido.',
      expires: (when: string) => `El código caduca ${when}.`,
      codeEnded:
        'El código caducó o se rechazó antes de introducirlo. Obtenga un código nuevo para volver a intentarlo.',
      signInStep: (site: string) =>
        `Vaya a ${site}, inicie sesión si se le solicita y permita que Polyfin use su cuenta.`,
      signInWaiting: 'Esta página se actualiza sola en cuanto lo haya permitido.',
      signInExpires: (when: string) => `Esta solicitud caduca ${when}.`,
      signInEnded:
        'Polyfin no se autorizó a tiempo o se rechazó. Vuelva a empezar para intentarlo de nuevo.',
      pinIntro: (name: string) =>
        `Conecte sus listas de ${name}: autorice a Polyfin en el sitio de ${name} y, después, pegue aquí el código que le muestre.`,
      redirectIntro: (name: string) =>
        `Conecte su lista de ${name}: autorice a Polyfin en el sitio de ${name}, que le devolverá aquí.`,
      pinStep: (site: string) =>
        `Vaya a ${site}, inicie sesión si se le solicita, autorice a Polyfin y, después, copie aquí el código que le muestre.`,
      redirectStep: (site: string) =>
        `Vaya a ${site}, inicie sesión si se le solicita y autorice a Polyfin: volverá aquí al terminar.`,
      codeField: (name: string) => `Código de ${name}`,
      codeFieldHelp: (name: string) => `El código que muestra ${name} cuando autoriza a Polyfin.`,
      addressField: 'Dirección o código',
      addressFieldHelp: (name: string) =>
        `Si su navegador no pudo abrir la página a la que le envió ${name}, copie aquí la dirección de esa página desde la barra de direcciones.`,
      pasteEnded:
        'Esta solicitud caducó antes de autorizar a Polyfin. Vuelva a empezar para intentarlo de nuevo.',
      invalidCode: (name: string) =>
        `${name} no aceptó este código, o corresponde a una solicitud anterior. Vuelva a empezar para obtener uno nuevo.`,
      tokenExpires: (date: string) => `, hasta el ${date}`,
      returning: (name: string) => `Conectando su cuenta de ${name}…`,
      connectedAs: 'Conectado como ',
      connectedWithKey: 'Conectado con su clave API',
      connected: 'Conectado',
      connectedOn: (date: string) => ` el ${date}`,
      lastSent: ', último envío ',
      nothingSent: ', aún no se ha enviado nada',
      connectedToast: (name: string) => `${name} conectado.`,
      disconnectedToast: (name: string) => `${name} desconectado.`,
      problemReconnect: (name: string) =>
        `${name} ya no acepta esta conexión, así que no se le envía nada. Vuelva a conectar para empezar a enviar lo que ve.`,
      problemUnreachable: (name: string) =>
        `No se ha podido contactar con ${name} últimamente. Polyfin sigue intentándolo y enviará lo que ha visto en cuanto responda.`,
      disconnect: 'Desconectar',
      disconnectTitle: (name: string) => `¿Desconectar ${name}?`,
      disconnectBody: (name: string) =>
        `Polyfin dejará de informarle de lo que ve. Lo que ya tiene se conserva en ${name}.`,
      invalidKey: (name: string) =>
        `${name} no aceptó esta clave. Vuelva a copiarla desde su cuenta de ${name}.`,
      invalidToken: (name: string) =>
        `${name} no aceptó este token. Vuelva a copiarlo desde sus ajustes de ${name}.`,
      serviceUnreachable: (name: string) =>
        `No se pudo contactar con ${name}. Inténtelo de nuevo en unos minutos.`,
      notAvailable: (name: string) =>
        `${name} no está configurado en este servidor: un administrador debe añadir primero la aplicación de ${name}.`,
      appRefused: (name: string) =>
        `${name} ha rechazado la aplicación de este servidor. Un administrador debe comprobar sus credenciales en Ajustes › Seguimiento.`,
      history: {
        toggle: (name: string) => `Importar mi historial de ${name}`,
        toggleHelp: (name: string, every: string) =>
          `Marca como visto en Polyfin lo que ha visto en ${name} y continúa donde lo dejó, para que «Seguir viendo» y «A continuación» coincidan. Se importa ${every}; no se envía nada a ${name}.`,
        listsToggle: (name: string) => `Importar mis listas de ${name}`,
        listsToggleHelp: (name: string, every: string) =>
          `Marca como vistos en Polyfin los episodios que sus listas de ${name} cuentan como vistos: hasta el progreso de cada entrada para Watching, On hold y Dropped, y todos para Completed. Se importa ${every}; solo añade.`,
        listCounts: (played: number, unmapped: number) =>
          `${played === 1 ? '1 título marcado como visto' : `${played} títulos marcados como vistos`}, ${unmapped === 1 ? '1 título no encontrado' : `${unmapped} títulos no encontrados`} en Polyfin.`,
        importNow: 'Importar ahora',
        importing: 'Importando…',
        importingStatus: 'Importando su historial…',
        never: 'Aún no se ha importado.',
        lastImport: 'Última importación',
        colon: ': ',
        nextImport: 'Próxima importación',
        counts: (played: number, resumed: number, unmapped: number) =>
          `${played === 1 ? '1 título marcado como visto' : `${played} títulos marcados como vistos`}, ${resumed === 1 ? '1 punto de reanudación' : `${resumed} puntos de reanudación`}, ${unmapped === 1 ? '1 título no encontrado' : `${unmapped} títulos no encontrados`} en Polyfin.`,
        problem: {
          reconnect: (name: string) =>
            `${name} rechazó la conexión durante la última importación. Vuelva a conectarse para importar su historial.`,
          unreachable: (name: string) =>
            `No se pudo leer ${name} por completo durante la última importación. Se importó lo que se pudo leer; la próxima importación lo volverá a intentar.`,
          rate_limited: (name: string) =>
            `${name} pidió a Polyfin que esperase durante la última importación. Se importó lo que se pudo leer; la próxima importación lo volverá a intentar.`,
        },
        watchlist: {
          lists: {
            trakt: 'watchlist',
            simkl: 'lista Plan to Watch',
            anilist: 'lista Plan to Watch',
            myanimelist: 'lista Plan to Watch',
          },
          toggle: (name: string, list: string) => `Importar mi ${list} de ${name}`,
          toggleHelp: (name: string, list: string) =>
            `Coloca las películas y series de su ${list} de ${name} en la vista «Para ver» de sus aplicaciones de Jellyfin, en cada importación. Un título sale de ella en cuanto lo empieza o en cuanto ${name} deja de incluirlo. No se envía nada a ${name}.`,
          counts: (added: number, unmapped: number) =>
            `${added === 1 ? '1 añadido' : `${added} añadidos`} a «Para ver», ${unmapped === 1 ? '1 título no encontrado' : `${unmapped} títulos no encontrados`} en Polyfin.`,
          added: (added: number) =>
            `${added === 1 ? '1 añadido' : `${added} añadidos`} a «Para ver».`,
        },
        ratings: {
          toggle: 'Sincronizar mis valoraciones',
          toggleHelp: (name: string) =>
            `Envía a ${name} como puntuaciones las valoraciones que da a películas y series, y asigna sus puntuaciones de ${name} a los títulos que no ha valorado, en cada importación. Se omite una serie que ${name} divide en varias entradas.`,
          counts: (rated: number, skipped: number) =>
            `${rated === 1 ? '1 título valorado' : `${rated} títulos valorados`}, ${skipped === 1 ? '1 valoración omitida' : `${skipped} valoraciones omitidas`}.`,
        },
      },
    },
  },
  shareLinks: {
    share: 'Compartir un título',
    off: 'No puede crear enlaces para compartir en este momento: un administrador los ha desactivado para este servidor o para su cuenta. Mientras tanto, los enlaces aún activos no se reproducen.',
    empty: 'No hay enlaces compartidos',
    emptyHelp:
      '«Compartir un título» ofrece una película o un episodio mediante un enlace. En el reproductor web, la página de una película o de un episodio también incluye «Compartir».',
    userEmpty: 'Este usuario no tiene enlaces compartidos.',
    expiresLabel: 'Caduca',
    expiredLabel: 'Caducado',
    revokedLabel: 'Revocado',
    plays: (count: number) => (count === 1 ? '1 reproducción' : `${count} reproducciones`),
    viewers: (count: number) =>
      count === 1 ? '1 espectador a la vez' : `${count} espectadores a la vez`,
    states: { active: 'Activo', expired: 'Caducado', revoked: 'Revocado' },
    revoke: 'Revocar',
    revokeLabel: (title: string) => `Revocar el enlace a ${title}`,
    revokeTitle: '¿Revocar este enlace compartido?',
    revokeConfirm:
      'El enlace deja de funcionar de inmediato, y quien esté viendo a través de él se detendrá en unos segundos.',
    revoking: 'Revocando…',
    revoked: 'Se ha revocado el enlace compartido.',
    search: 'Buscar en sus bibliotecas',
    searchHelp: 'Una película, o una serie para elegir uno de sus episodios.',
    searchButton: 'Buscar',
    searching: 'Buscando…',
    noTitles: 'No se ha encontrado ninguna película ni serie.',
    titlesFound: (term: string) => `Títulos encontrados para ${term}`,
    pickTitle: (name: string, details: string) => `Compartir ${name}, ${details}`,
    episodesOf: (series: string) => `Episodios de ${series}`,
    noEpisodes: 'Esta serie no tiene ningún episodio que pueda compartir.',
    back: 'Volver a los resultados',
    chosen: 'Título',
    change: 'Cambiar',
    expires: 'Caduca después de',
    expiresHelp: 'Después, nadie podrá abrir el enlace. Puede revocarlo antes.',
    after: (hours: number) =>
      hours === 1 ? '1 hora' : hours === 24 ? '1 día' : `${hours / 24} días`,
    viewersLabel: 'Espectadores a la vez',
    viewersHelp:
      'Cuántas personas pueden ver a través del enlace al mismo tiempo. Cada una cuenta como una de sus reproducciones simultáneas.',
    submit: 'Crear enlace',
    creating: 'Creando…',
    createdTitle: 'Enlace creado',
    createdNotice: (title: string) =>
      `Cualquier persona con este enlace puede ver ${title} en un navegador, sin cuenta, hasta que caduque. Cópielo ahora: solo se muestra esta vez.`,
    copy: 'Copiar el enlace',
    copied: 'Enlace copiado.',
    copyFailed: 'No se pudo copiar el enlace. Selecciónelo y cópielo manualmente.',
    done: 'Hecho',
  },
}

export default account
