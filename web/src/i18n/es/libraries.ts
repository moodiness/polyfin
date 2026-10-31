import type en from '../en/libraries'

const libraries: typeof en = {
  libraries: {
    title: 'Bibliotecas',
    description:
      'Elija qué catálogos de las extensiones del servidor aparecen como bibliotecas en las aplicaciones de Jellyfin, para todos los usuarios.',
    shownTitle: 'Visibles en las aplicaciones de Jellyfin',
    shownHelp:
      'Las bibliotecas aparecen en este orden. Deje un nombre vacío para usar el nombre del catálogo. A las bibliotecas que comparten nombre se les añade su tipo en las aplicaciones, por ejemplo «Populares (Películas)».',
    shownHelpMine:
      'Las bibliotecas aparecen en este orden, después de las bibliotecas del servidor si las usa. Deje un nombre vacío para usar el nombre del catálogo. A las bibliotecas que comparten nombre se les añade su tipo en las aplicaciones, por ejemplo «Populares (Películas)».',
    defaultHelp:
      'Al instalar una extensión, sus catálogos de colecciones se convierten en bibliotecas: cada una muestra sus colecciones (por ejemplo, géneros o décadas), que se abren en sus películas y series. Una extensión sin colecciones aporta sus catálogos de películas, series y TV como bibliotecas, hasta 20. Esto es solo un punto de partida: active tantos catálogos como quiera. Cada biblioteca añade una fila a la pantalla de inicio de las aplicaciones de Jellyfin, excepto los catálogos de TV, cuyos canales aparecen en Televisión en vivo.',
    manyWarning:
      'Más de 20 bibliotecas pueden sobrecargar la pantalla de inicio de las aplicaciones de Jellyfin.',
    noAddons: 'Añada primero una fuente: sus catálogos aparecerán aquí.',
    shownEmpty:
      'Ninguna biblioteca: las aplicaciones de Jellyfin no muestran nada de estas extensiones.',
    name: 'Nombre en las aplicaciones',
    appName: (name: string) => `Se muestra en las aplicaciones como «${name}».`,
    liveTv: 'Sus canales aparecen en Televisión en vivo en las aplicaciones de Jellyfin.',
    iptvVod: 'De una fuente IPTV.',
    iptvVodLink: 'Sus opciones de importación',
    addonOff: 'Extensión desactivada',
    missing: 'Ya no está disponible',
    hideInMenus: 'Ocultar en la barra superior del reproductor web',
    hideInMenusHelp:
      'Conserva su fila en la pantalla de inicio. El menú lateral del reproductor web también la omite; las demás aplicaciones de Jellyfin la siguen mostrando. Se aplica la próxima vez que se cargue el reproductor web.',
    hiddenInMenus: 'Oculta en la barra superior',
    remove: 'Eliminar',
    removeLabel: (name: string) => `Eliminar ${name}`,
    moreActions: (name: string) => `Más acciones para ${name}`,
    userOrders: {
      help: 'Un usuario que ordenó las bibliotecas en Ajustes › Inicio del reproductor web conserva ese orden, hasta «Aplicar este orden a todos los usuarios» o su propio «Restablecer mis preferencias».',
      apply: 'Aplicar este orden a todos los usuarios',
      saveFirst: 'Guarde primero la lista: se aplica el orden guardado.',
      title: '¿Aplicar este orden a todos los usuarios?',
      counting: 'Contando los usuarios con un orden propio…',
      none: 'Todos los usuarios ya siguen este orden: ninguno ordenó las bibliotecas en Ajustes › Inicio del reproductor web.',
      some: (count: number) =>
        `${count === 1 ? '1 usuario ordenó' : `${count} usuarios ordenaron`} las bibliotecas en Ajustes › Inicio del reproductor web. Su orden se olvida, y sus aplicaciones muestran las bibliotecas en este orden, el guardado; el reproductor web, en un minuto como mucho. Sus demás ajustes se mantienen.`,
      applied: (count: number) =>
        count === 0
          ? 'Todos los usuarios siguen este orden.'
          : count === 1
            ? 'Este orden se aplica ahora a todos: se olvidó el orden propio de 1 usuario.'
            : `Este orden se aplica ahora a todos: se olvidó el orden propio de ${count} usuarios.`,
    },
    refresh: {
      start: 'Actualizar las descripciones',
      stop: 'Detener la actualización',
      queued: 'Esperando para actualizar sus descripciones',
      running: (done: string, titles: string) =>
        `Actualizando las descripciones: ${done} de ${titles}`,
      done: (done: string, failed: number, figure: string) =>
        failed === 0
          ? `Descripciones actualizadas: ${done}`
          : `Descripciones actualizadas: ${done}, ${figure} sin describir`,
      stopped: (done: string, titles: string) => `Actualización detenida en ${done} de ${titles}`,
      failed: 'No se pudieron actualizar sus descripciones',
      started: (name: string) => `${name}: sus descripciones se vuelven a leer, 30 por minuto.`,
      progress: (name: string) => `Actualización de ${name}`,
    },
    removedLive: (name: string) => `${name} se ha eliminado de las bibliotecas.`,
    dragHandle: (name: string) => `Arrastrar ${name}`,
    availableTitle: 'Catálogos disponibles',
    availableHelp:
      'Al añadir un catálogo, se muestra como biblioteca al final de la lista anterior.',
    filter: 'Filtrar',
    filterPlaceholder: 'Nombre del catálogo o de la extensión',
    type: 'Tipo',
    allTypes: 'Todos los tipos',
    add: 'Añadir',
    addLabel: (name: string) => `Añadir ${name}`,
    addedLive: (name: string, position: number) =>
      `${name} se ha añadido como biblioteca ${position}.`,
    notBrowsable: 'No disponible',
    notBrowsableHelp: 'Necesita una búsqueda u otro valor, por lo que no puede ser una biblioteca.',
    availableEmpty: 'Ya se muestran todos los catálogos.',
    noMatch: 'Ningún catálogo coincide con este filtro.',
    reset: 'Restablecer',
    saved: 'Bibliotecas guardadas.',
    guideAfterSave: 'Guarde las bibliotecas para añadir una guía de programación a este catálogo.',
    guideFetched: 'Última obtención',
    guideNever: 'Aún no se ha obtenido.',
    loading: 'Cargando bibliotecas…',
    listLabel: 'Bibliotecas, en el orden en que las muestran las aplicaciones',
    columnCatalog: 'Catálogo',
    shownEmptyTitle: 'Aún no hay bibliotecas',
    shownEmptyHow:
      'Añada catálogos desde la lista de abajo: cada uno se convierte en una biblioteca.',
    noAddonsTitle: 'Aún no hay catálogos',
    addSource: 'Añadir una fuente',
    defaultTitle: 'Cómo se eligen las primeras bibliotecas',
    availableLabel: (addon: string) => `Catálogos de ${addon}`,
    music: {
      library: {
        music: 'Biblioteca de música',
        audiobook: 'Biblioteca de libros',
        podcast: 'Biblioteca de música',
      } as Record<string, string>,
      help: {
        music: 'Sus elementos aparecen en la vista de música de las aplicaciones de Jellyfin.',
        audiobook:
          'Sus audiolibros aparecen en una biblioteca de libros de las aplicaciones de Jellyfin.',
        podcast: 'Sus episodios aparecen en la vista de música de las aplicaciones de Jellyfin.',
      } as Record<string, string>,
    },
    image: {
      label: 'Imagen en las aplicaciones',
      edit: (name: string, state: string) => `Imagen de ${name}: ${state}`,
      choices: {
        none: 'Ninguna',
        automatic: 'Automática',
        generated: 'Generada',
        custom: 'Personalizada',
      },
      help: {
        none: 'Las aplicaciones muestran esta biblioteca sin imagen, como antes.',
        automatic:
          'Polyfin la toma del catálogo: la imagen de fondo de uno de sus primeros títulos o, si no, un póster.',
        generated:
          'Polyfin la compone con los pósteres de los primeros títulos del catálogo, o de las primeras colecciones de una biblioteca de colecciones, junto con el nombre de la biblioteca, una vez que los ha leído.',
        custom:
          'Una imagen que usted sube, o que Polyfin descarga una sola vez desde una dirección. Las imágenes anchas en 16:9 son las que mejor encajan en los mosaicos de las bibliotecas. JPEG, PNG o WebP, 10 MB como máximo.',
      },
      parental: 'Los usuarios con control parental o con géneros bloqueados no la ven.',
      notFound: {
        automatic:
          'Por ahora no se ha encontrado nada en este catálogo, así que las aplicaciones no muestran ninguna imagen.',
        generated:
          'Aún no se ha leído ningún póster de este catálogo, así que las aplicaciones no muestran ninguna imagen. Polyfin la compone en cuanto se abre la biblioteca en una aplicación.',
      },
      noImage: 'Sin imagen',
      upload: 'Subir una imagen',
      uploadAnother: 'Subir otra imagen',
      address: 'O una dirección de imagen',
      addressHelp: 'Polyfin la descarga una sola vez y guarda una copia.',
      useAddress: 'Usar esta dirección',
      remove: 'Eliminar la imagen',
      removeTitle: (name: string) => `¿Eliminar la imagen de ${name}?`,
      removeToNone:
        'Se borrará la imagen subida. Las aplicaciones mostrarán esta biblioteca sin imagen.',
      removeToAutomatic:
        'Se borrará la imagen subida. Las aplicaciones mostrarán en su lugar la imagen encontrada en el catálogo.',
      removeToGenerated:
        'Se borrará la imagen subida. Las aplicaciones mostrarán en su lugar la imagen que compone Polyfin.',
      removeConfirm: 'Eliminar',
      saved: 'Imagen guardada.',
      removed: 'Imagen eliminada.',
      atOnce: 'Los cambios de imagen se guardan al instante.',
      afterSave: 'Guarde primero las bibliotecas y, después, elija la imagen de esta biblioteca.',
      close: (name: string) => `Cerrar la imagen de ${name}`,
    },
    narrow: {
      title: 'Género y máximo',
      edit: (name: string) => `Género y máximo de ${name}`,
      close: (name: string) => `Cerrar el género y el máximo de ${name}`,
      staged: 'Se guarda con el botón «Guardar», como los nombres.',
      genre: 'Género',
      allGenres: 'Todos los géneros',
      genreHelp: 'Muestra solo los títulos de este género.',
      maxItems: 'Máximo de títulos',
      noMax: 'Sin máximo',
      maxHelp: (max: string, where: string) =>
        `Muestra como máximo este número de títulos en la biblioteca, y otros tantos en cada una de sus colecciones, en lugar del límite del catálogo en ${where}. De 1 a ${max}. Vacío: se aplica ese límite.`,
      maxHelpMine: (max: string) =>
        `Muestra como máximo este número de títulos en la biblioteca, y otros tantos en cada una de sus colecciones, en lugar del límite del servidor. De 1 a ${max}. Vacío: se aplica el límite del servidor.`,
      maxRange: (max: string) => `Introduzca un número entero de 1 a ${max}, o déjelo vacío.`,
      maxBadge: (count: string) => `${count} máx.`,
    },
  },
  smartLists: {
    title: 'Listas inteligentes',
    help: 'Listas creadas a partir de reglas sobre los títulos de los catálogos del servidor, como «Películas francesas de los años 90 aún no vistas». Todos los usuarios las ven, dentro de las bibliotecas que pueden ver.',
    helpMine:
      'Listas creadas a partir de reglas sobre los títulos de sus catálogos, y de los del servidor mientras los utilice. Solo usted las ve.',
    add: 'Añadir una lista inteligente',
    emptyTitle: 'Aún no hay ninguna lista inteligente',
    emptyHow:
      'Una lista inteligente conserva los títulos de los catálogos elegidos que cumplen sus reglas, y sigue esos catálogos a medida que cambian.',
    loading: 'Cargando listas inteligentes…',
    shownAsLibrary: 'Biblioteca',
    shownAsCollection: 'Colección',
    catalogCount: (count: number) => (count === 1 ? '1 catálogo' : `${count} catálogos`),
    ruleCount: (count: number) => (count === 1 ? '1 regla' : `${count} reglas`),
    maxBadge: (count: string) => `${count} máx.`,
    newTitle: 'Nueva lista inteligente',
    back: 'Bibliotecas',
    backMine: 'Mis fuentes',
    lede: 'Elija los catálogos de los que toma sus títulos y las reglas que deben cumplir. La vista previa muestra sus títulos tal y como están ahora, antes de guardar.',
    notFound: 'Esta lista inteligente no existe.',
    name: 'Nombre',
    nameHelp: 'El nombre que muestran las aplicaciones de Jellyfin.',
    shownAs: 'Mostrar como',
    shownAsChoices: { library: 'Una biblioteca', collection: 'Una colección' },
    shownAsHelp: {
      library:
        'Una biblioteca propia en las aplicaciones de Jellyfin, con su fila en la pantalla de inicio: una biblioteca de películas o de series cuando todos sus catálogos son del mismo tipo.',
      collection:
        'Una colección en la vista Colecciones, junto a las creadas por los usuarios. Las aplicaciones no pueden añadir ni eliminar sus títulos.',
    },
    catalogs: 'Catálogos',
    catalogsHelp:
      'Sus títulos proceden de estos catálogos, cada uno leído hasta su máximo: el de la biblioteca cuando el catálogo es una de sus bibliotecas; si no, Títulos leídos por catálogo de películas y series.',
    catalogFilter: 'Filtrar catálogos',
    catalogFilterPlaceholder: 'Nombre del catálogo o de la extensión',
    serverBadge: 'Servidor',
    noCatalogs: 'Aún no hay ningún catálogo del que tomar títulos: añada primero una fuente.',
    noCatalogMatch: 'Ningún catálogo coincide con este filtro.',
    rules: 'Reglas',
    rulesHelp: 'Sin reglas, la lista conserva todos los títulos de sus catálogos.',
    exclude: 'Exclusiones',
    excludeHelp:
      'Los títulos que cumplen estas reglas quedan fuera, digan lo que digan las reglas de arriba. Sin exclusiones, ninguno queda fuera.',
    excludeMatch: 'Dejar fuera los títulos que cumplen',
    match: 'Los títulos cumplen',
    matchChoices: { all: 'Todas las reglas', any: 'Alguna de las reglas' },
    groupMatch: 'Este grupo cumple',
    group: 'Grupo',
    addRule: 'Añadir una regla',
    addGroup: 'Añadir un grupo',
    removeRule: 'Eliminar esta regla',
    removeGroup: 'Eliminar este grupo',
    field: 'Se fija en',
    fields: {
      genre: 'Género',
      year: 'Año',
      rating: 'Valoración de IMDb',
      language: 'Idioma',
      type: 'Película o serie',
      rank: 'Posición en su catálogo',
      state: 'Estado de visualización',
      released: 'Fecha de estreno',
      votes: 'Votos de IMDb',
      runtime: 'Duración',
      contentRating: 'Clasificación por edad',
      person: 'Actor o director',
      studio: 'Estudio o cadena',
    } as Record<string, string>,
    negation: 'Cumple o no',
    is: 'es',
    isNot: 'no es',
    genre: 'Género',
    from: 'Desde',
    to: 'Hasta',
    language: 'Idioma',
    rankMax: 'Entre sus primeros',
    minutes: 'minutos',
    atMost: 'Como mucho',
    releasedWhen: 'Cuándo',
    releasedWithin: { last: 'En los últimos', next: 'En los próximos' } as Record<string, string>,
    days: 'días',
    namePlaceholder: { person: 'Su nombre', studio: 'Su nombre' } as Record<string, string>,
    types: { movie: 'Una película', series: 'Una serie' } as Record<string, string>,
    states: {
      played: 'Reproducido',
      unplayed: 'No reproducido',
      started: 'Empezado, sin terminar',
      favorite: 'Favorito',
    } as Record<string, string>,
    stateHelp:
      'Las reglas de reproducido, empezado y favorito siguen lo que hizo cada usuario: cada uno ve su propia lista.',
    describedHelp:
      'El idioma de un título, y su duración, su clasificación, sus personas y su fecha de estreno cuando su catálogo no las da, llegan con su descripción, que Polyfin lee en segundo plano: un título aún sin describir queda fuera hasta que lo esté. Un título al que le falta lo que mira una regla no cumple ni la regla ni su contrario.',
    votesHelp:
      'Los votos de IMDb vienen de MDBList, con la clave API de MDBList para valoraciones en Ajustes › Contenido, consultados en segundo plano. Sin esa clave, ningún título tiene votos.',
    studioHelp:
      'Los estudios y las cadenas vienen de TMDB, con la clave API de TMDB en Ajustes › Contenido, consultados en segundo plano y guardados: la lista crece a medida que llegan. Sin esa clave, solo cuentan los estudios que un administrador puso en los metadatos de un título.',
    sort: 'Ordenar',
    sorts: {
      catalog: 'Como los listan sus catálogos, uno de cada uno por turnos',
      premiere: 'Fecha de estreno, más reciente primero',
      latestEpisode: 'Último episodio emitido, más reciente primero',
      seasonPremiere: 'Último estreno de temporada, más reciente primero',
      rating: 'Valoración de IMDb, más alta primero',
      name: 'Nombre',
      random: 'Al azar, sorteado de nuevo cada día',
    } as Record<string, string>,
    episodeSortHelp:
      'Las series van según su último episodio regular emitido, o el estreno de su última temporada, según su descripción, sin especiales ni episodios por venir; las películas según su fecha de estreno. Una serie cuya descripción aún no se ha leído va al final hasta que se lea.',
    maxItems: 'Máximo de títulos',
    maxHelp: (max: string) => `De 1 a ${max}. Vacío: sin máximo.`,
    noMax: 'Sin máximo',
    create: 'Crear la lista',
    save: 'Guardar',
    created: 'Lista inteligente creada.',
    saved: 'Lista inteligente guardada.',
    delete: 'Borrar',
    deleteTitle: (name: string) => `¿Borrar ${name}?`,
    deleteBody:
      'Las aplicaciones de Jellyfin dejarán de mostrarla. Sus títulos permanecen en sus catálogos.',
    deleted: 'Lista inteligente borrada.',
    preview: 'Vista previa',
    previewHelp:
      'Los títulos que la lista le muestra ahora. Cada usuario la ve con sus propios títulos vistos.',
    refresh: 'Actualizar',
    listed: (count: string) => (count === '1' ? '1 título' : `${count} títulos`),
    firstOf: (shown: string, listed: string) => `Los primeros ${shown} de ${listed}.`,
    unknown: (count: string) =>
      count === '1'
        ? '1 título aún espera lo que miran sus reglas: su descripción, sus votos o sus estudios. La lista crece a medida que llegan.'
        : `${count} títulos aún esperan lo que miran sus reglas: su descripción, sus votos o sus estudios. La lista crece a medida que llegan.`,
    incomplete:
      'Algunos catálogos no respondieron a tiempo: la lista muestra lo leído hasta ahora y lee el resto.',
    previewEmpty: 'Por ahora, ningún título cumple estas reglas.',
    previewNeedsCatalog: 'Elija al menos un catálogo para ver la lista.',
    previewLabel: 'Títulos de la lista',
    columns: {
      title: 'Título',
      type: 'Tipo',
      year: 'Año',
      rating: 'IMDb',
      genres: 'Géneros',
      languages: 'Idioma',
    },
  },
  /** Episode orders: the order a series shows its seasons and episodes in, for every user. */
  episodeOrders: {
    title: 'Órdenes de episodios',
    help: 'Una serie muestra sus temporadas y episodios como los numera su complemento, en orden de emisión. Muestre una en otro orden de TMDB, como DVD, absoluto o arco argumental, para todos los usuarios, como hace el orden de visualización del editor de metadatos de jellyfin-web. Sus episodios conservan su propia temporada y número para la reproducción, el historial y el seguimiento.',
    loading: 'Cargando los órdenes de episodios…',
    emptyTitle: 'Todas las series se muestran en orden de emisión',
    emptyHow: 'Busque una serie abajo para mostrarla en otro orden.',
    search: 'Buscar una serie',
    searchPlaceholder: 'Nombre de la serie',
    searchButton: 'Buscar',
    searching: 'Buscando…',
    results: 'Series encontradas',
    noSeries: 'Ninguna serie coincide con este nombre.',
    editTitle: (name: string) => `Orden de episodios de ${name}`,
    order: 'Orden',
    orderHelp:
      'Los grupos de episodios que TMDB da para esta serie. Las partes de un grupo pasan a ser sus temporadas.',
    aired: 'Orden de emisión',
    types: {
      originalAirDate: 'Fecha de emisión original',
      absolute: 'Absoluto',
      dvd: 'DVD',
      digital: 'Digital',
      storyArc: 'Arco argumental',
      production: 'Producción',
      tv: 'TV',
    },
    counts: (seasons: number, episodes: number) =>
      `${seasons === 1 ? '1 temporada' : `${seasons} temporadas`}, ${episodes === 1 ? '1 episodio' : `${episodes} episodios`}`,
    lookup: {
      no_key: 'Añada una clave de TMDB en Ajustes › Contenido para ver los órdenes de esta serie.',
      unknown: 'TMDB no conoce esta serie: se muestra en orden de emisión.',
      failed:
        'No se pudo contactar con TMDB para ver los órdenes de esta serie. Inténtelo de nuevo en un momento.',
    },
    specials: 'Especiales',
    specialsHelp:
      'Dónde aparece la temporada 0 entre las temporadas. Los especiales ocultos se siguen reproduciendo desde «Seguir viendo» o un enlace.',
    specialsFirst: 'Primero',
    specialsLast: 'Al final',
    specialsHidden: 'Ocultos',
    specialsSummary: { last: 'Especiales al final', hidden: 'Especiales ocultos' },
    saved: (name: string) => `${name}: orden de episodios guardado.`,
  },
}

export default libraries
