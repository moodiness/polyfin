import type en from '../en/statistics'

const statistics: typeof en = {
  statistics: {
    title: 'Estadísticas',
    description:
      'Cómo se usa el servidor: horas vistas, los títulos, aplicaciones y dispositivos, y cómo llegan los vídeos a las aplicaciones. No se cuentan las canciones ni los audiolibros.',
    period: 'Periodo',
    periods: { '7d': '7 días', '30d': '30 días', year: 'Año', all: 'Todo' },
    user: 'Usuario',
    allUsers: 'Todos los usuarios',
    exportCsv: 'Exportar CSV',
    historyOff: {
      server:
        'El historial de reproducción está desactivado: no se cuentan las nuevas reproducciones.',
      own: 'El historial de reproducción está desactivado en este servidor: no se cuentan las nuevas reproducciones.',
    },
    openSettings: 'Abrir Ajustes › Reproducción',
    empty: 'No hay reproducciones en este periodo',
    emptyHint:
      'Los vídeos reproducidos durante al menos 10 segundos en una aplicación de Jellyfin aparecen aquí cuando se detienen.',
    figures: {
      played: 'Horas vistas',
      plays: 'Reproducciones',
      users: 'Usuarios',
      sent: 'Datos enviados',
    },
    perDay: 'Horas vistas por día',
    perMonth: 'Horas vistas por mes',
    perUser: 'Horas vistas por usuario',
    movies: 'Películas más reproducidas',
    series: 'Series más reproducidas',
    channels: 'Canales más reproducidos',
    apps: 'Aplicaciones',
    devices: 'Dispositivos',
    methods: 'Cómo se reproducen los vídeos',
    methodNames: {
      direct_play: 'Reproducción directa',
      direct_stream: 'Remux',
      conversion: 'Conversión',
      '': 'Desconocido',
    },
    hours: 'Horas de más actividad',
    hoursHelp: 'Horas vistas en cada hora de la semana, en su zona horaria.',
    hoursCell: (day: string, hour: string, time: string) => `${day} ${hour}: ${time}`,
    plays: (count: number) => (count === 1 ? '1 reproducción' : `${count} reproducciones`),
    users: (count: number) => (count === 1 ? '1 usuario' : `${count} usuarios`),
    share: (percent: string) => `${percent} de las reproducciones`,
    none: 'Ninguno en este periodo.',
    unknownApp: 'Aplicación desconocida',
    unknownDevice: 'Dispositivo desconocido',
    bar: (label: string, time: string) => `${label}: ${time}`,
    history: 'Reproducciones recientes',
    historyEmpty: 'No hay reproducciones en este periodo.',
    columns: {
      started: 'Inicio',
      user: 'Usuario',
      title: 'Título',
      device: 'Aplicación y dispositivo',
      played: 'Tiempo visto',
      sent: 'Datos enviados',
      method: 'Cómo se reprodujo',
    },
    appOn: (app: string, device: string) => (app && device ? `${app} en ${device}` : app || device),
    shared: 'Compartido',
    sharedHelp: 'Reproducido por un invitado mediante un enlace para compartir',
    sharedLink: 'Enlace compartido',
    webBrowser: 'Navegador web',
    showMore: 'Mostrar más',
    loadingMore: 'Cargando…',
  },
}

export default statistics
