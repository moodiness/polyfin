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
    } as Record<string, string>,
    negation: 'Cumple o no',
    is: 'es',
    isNot: 'no es',
    genre: 'Género',
    from: 'Desde',
    to: 'Hasta',
    language: 'Idioma',
    rankMax: 'Entre sus primeros',
    types: { movie: 'Una película', series: 'Una serie' } as Record<string, string>,
    states: {
      played: 'Reproducido',
      unplayed: 'No reproducido',
      started: 'Empezado, sin terminar',
      favorite: 'Favorito',
    } as Record<string, string>,
    stateHelp:
      'Las reglas de reproducido, empezado y favorito siguen lo que hizo cada usuario: cada uno ve su propia lista.',
    languageHelp:
      'El idioma viene con la descripción de un título, que Polyfin lee en segundo plano: un título aún sin describir queda fuera hasta que lo esté.',
    sort: 'Ordenar',
    sorts: {
      catalog: 'Como los listan sus catálogos, uno de cada uno por turnos',
      premiere: 'Fecha de estreno, más reciente primero',
      rating: 'Valoración de IMDb, más alta primero',
      name: 'Nombre',
      random: 'Al azar, sorteado de nuevo cada día',
    } as Record<string, string>,
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
        ? '1 título aún espera su descripción, que indica su idioma: la lista crece a medida que llegan las descripciones.'
        : `${count} títulos aún esperan su descripción, que indica su idioma: la lista crece a medida que llegan las descripciones.`,
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
}

export default libraries
