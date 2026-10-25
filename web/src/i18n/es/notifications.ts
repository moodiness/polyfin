import type en from '../en/notifications'

const notifications: typeof en = {
  notifications: {
    title: 'Notificaciones',
    loading: 'Cargando los destinos…',
    empty: 'Aún no hay ningún destino.',
    emptyHint: {
      server:
        'Añada un destino, como una dirección de correo electrónico, un chat de Telegram o un tema de ntfy, para recibir avisos de nuevos episodios, grabaciones y problemas del estado del sistema.',
      own: 'Añada un destino, como una dirección de correo electrónico, un chat de Telegram o un tema de ntfy, para recibir un aviso cuando salga un nuevo episodio de una serie que sigue o cuando terminen sus grabaciones.',
    },
    serverTargets: 'Destinos del servidor',
    serverTargetsHelp:
      'Reciben los eventos de todos los usuarios: un mensaje por cada nuevo episodio, sea quien sea quien siga la serie, cada grabación y cada reproducción. Cada usuario puede añadir sus propios destinos en Mi cuenta › Notificaciones.',
    add: 'Añadir un destino',
    addTitle: 'Añadir un destino',
    editTitle: (name: string) => `Editar ${name}`,
    kind: 'Tipo',
    kinds: {
      webhook: 'Webhook',
      discord: 'Discord',
      ntfy: 'ntfy',
      email: 'Correo electrónico',
      telegram: 'Telegram',
      gotify: 'Gotify',
      pushover: 'Pushover',
    },
    kindHelp: {
      webhook:
        'Polyfin envía cada evento a esta dirección en formato JSON, descrito en la documentación para desarrolladores de aplicaciones.',
      discord: 'Polyfin publica cada evento en un canal de Discord, como un mensaje con un enlace.',
      ntfy: 'Polyfin publica cada evento en un tema de ntfy, que la aplicación ntfy muestra en su teléfono.',
      email:
        'Polyfin envía cada evento por correo electrónico a esta dirección, en texto sin formato y HTML.',
      telegram:
        'Polyfin envía cada evento a un chat de Telegram a través de su bot, con un enlace.',
      gotify: 'Polyfin envía cada evento a una aplicación de su servidor Gotify.',
      pushover:
        'Polyfin envía cada evento a sus dispositivos Pushover, a través de una de sus aplicaciones.',
    },
    emailUnavailable: {
      server:
        'El correo electrónico necesita un servidor SMTP: guarde uno, con una dirección de remitente, en Correo electrónico más arriba y, después, añada el destino.',
      own: 'El correo electrónico necesita un servidor SMTP, que un administrador configura en Ajustes › Notificaciones.',
    },
    name: 'Nombre',
    nameHelp: 'Solo se muestra aquí, de 1 a 64 caracteres.',
    webhookAddress: 'Dirección del webhook',
    discordAddress: 'Dirección del webhook de Discord',
    discordSwitched:
      'Es un webhook de Discord: el destino pasa a ser un destino de Discord, que envía el mensaje propio de Discord.',
    discordAsWebhook:
      'Es un webhook de Discord: Polyfin le envía el mensaje propio de Discord, como a un destino de Discord.',
    addressHelp: {
      webhook: 'Se guarda cifrada y no se vuelve a mostrar: solo se muestra su host.',
      discord:
        'En Discord: ajustes del canal, Integraciones, Webhooks, Copiar URL de webhook. Se guarda cifrada y no se vuelve a mostrar.',
    },
    savedAddress: (host: string) => `Guardada, en ${host}.`,
    ntfyServer: 'Servidor ntfy',
    ntfyServerHelp: 'Déjelo vacío para usar https://ntfy.sh.',
    topic: 'Tema',
    topicHelp:
      'El tema al que se suscribe en la aplicación ntfy. Cualquiera que conozca un tema de un servidor público puede leerlo: elija uno difícil de adivinar.',
    token: 'Token de acceso',
    tokenHelp:
      'Para un tema que requiere iniciar sesión. Se guarda cifrado y no se vuelve a mostrar.',
    emailAddress: 'Dirección de correo electrónico',
    emailAddressHelp: 'Adonde se envían los mensajes.',
    chat: 'Chat',
    chatHelp:
      'El número del chat, como -1001234567890, o el nombre de un canal público, como @news_example. Añada antes el bot al chat.',
    botToken: 'Token del bot',
    botTokenHelp:
      'BotFather lo proporciona al crear el bot. Se guarda cifrado y no se vuelve a mostrar.',
    gotifyServer: 'Servidor Gotify',
    gotifyServerHelp: 'Su dirección, como https://gotify.example.org.',
    appToken: 'Token de la aplicación',
    appTokenHelp: {
      gotify:
        'En Gotify: Apps, Create Application y, después, copie su token. Se guarda cifrado y no se vuelve a mostrar.',
      pushover:
        'El token de API de una aplicación creada en Pushover. Se guarda cifrado y no se vuelve a mostrar.',
    },
    userKey: 'Clave de usuario',
    userKeyHelp:
      'Se muestra en el panel de Pushover tras iniciar sesión, o una clave de grupo. Se guarda cifrada y no se vuelve a mostrar.',
    events: 'Eventos',
    eventsHelp: {
      server: 'De qué se le avisa, para todos los usuarios.',
      own: 'De qué se le avisa, en relación con usted.',
    },
    eventNames: {
      new_episode: 'Nuevos episodios',
      recording_finished: 'Grabación finalizada',
      recording_failed: 'Grabación fallida',
      health_problem: 'Problema del sistema detectado',
      health_solved: 'Problema del sistema resuelto',
      user_joined: 'Nuevo usuario',
      new_version: 'Nueva versión',
      playback_started: 'Reproducción iniciada',
      playback_paused: 'Reproducción en pausa',
      playback_resumed: 'Reproducción reanudada',
      playback_stopped: 'Reproducción detenida',
      weekly_summary: 'Resumen semanal',
      monthly_summary: 'Resumen del mes',
      yearly_summary: 'Resumen del año',
      connection_stopped: 'Conexión detenida',
      account_blocked: 'Cuenta bloqueada',
      new_device: 'Inicio de sesión en un dispositivo nuevo',
      share_played: 'Enlace compartido reproducido',
      programme_reminder: 'Programa a punto de empezar',
    },
    eventHelp: {
      new_episode:
        'Se ha publicado un episodio de una serie vista o marcada como favorita: una vez por episodio, nunca para los ya publicados.',
      recording_finished:
        'Ha terminado una grabación de Televisión en vivo, completa o con una parte ausente.',
      recording_failed: 'Una grabación de Televisión en vivo no ha grabado nada.',
      health_problem:
        'Sistema › Estado del sistema ha detectado un problema en dos comprobaciones seguidas.',
      health_solved: 'Ha desaparecido un problema detectado por Sistema › Estado del sistema.',
      user_joined: 'Alguien ha creado su cuenta mediante un enlace de invitación.',
      new_version:
        'Se ha publicado una nueva versión de Polyfin, detectada por la comprobación diaria: una vez por versión, con sus notas de la versión.',
      playback_started:
        'Empieza a reproducirse un vídeo o una canción en una aplicación de Jellyfin, con quién lo reproduce, en qué dispositivo y si se está transcodificando.',
      playback_paused:
        'Se pausa una reproducción. Nunca se envían las posiciones durante la reproducción.',
      playback_resumed: 'Se reanuda una reproducción en pausa.',
      playback_stopped:
        'Se detiene una reproducción, o su aplicación ha dejado de notificarla durante cinco minutos.',
      weekly_summary:
        'Una vez por semana, el día y a la hora fijados en el servidor: las horas vistas, los títulos y los nuevos episodios de la semana. Los destinos del servidor y de los administradores reciben la semana del servidor, con los usuarios que se han unido, los archivos añadidos a las carpetas locales y los problemas detectados; los demás reciben la suya. No se envía nada si en la semana no hay nada que contar.',
      monthly_summary:
        'El día 1 de cada mes, a la hora del resumen semanal: el contenido del resumen semanal referido al mes que ha terminado. No se envía nada si en el mes no hay nada que contar.',
      yearly_summary:
        'El 1 de enero, a la hora del resumen semanal: las horas vistas, las diez películas y las diez series más reproducidas y el mes con más horas del año que ha terminado. Los destinos del servidor y de los administradores reciben el año del servidor, con los usuarios que se han unido y los archivos añadidos a las carpetas locales; los demás reciben el suyo. No se envía nada si en el año no hay nada que contar.',
      connection_stopped:
        'Una conexión con una aplicación o un servicio de seguimiento ha rechazado lo que Polyfin guardaba y se ha detenido: una vez por detención, con lo que hay que hacer para volver a conectarla.',
      account_blocked:
        'Varias contraseñas incorrectas han bloqueado una cuenta durante un tiempo: una vez por bloqueo, con la hora hasta la que dura y la dirección y la aplicación del último intento. Los destinos del servidor y de los administradores reciben los de todas las cuentas; los demás, los de la suya.',
      new_device:
        'Su cuenta ha iniciado sesión desde una aplicación de Jellyfin o el cliente web en un dispositivo que nunca había usado, con la aplicación, el dispositivo y la dirección. No se avisa del primer dispositivo de una cuenta.',
      share_played:
        'Alguien sin cuenta ha iniciado una película o un episodio mediante un enlace para compartir: como máximo una vez por hora y por enlace, con el título y quién lo compartió. Los destinos del servidor reciben los de todos los enlaces; los demás, los de los suyos.',
      programme_reminder:
        'Un programa de Televisión en vivo para el que ha creado un recordatorio en el reproductor web empieza pronto, con la antelación indicada en «Recordarme antes», y un enlace a su canal.',
    },
    enabled: 'Enviar mensajes a este destino',
    create: 'Añadir',
    creating: 'Añadiendo…',
    save: 'Guardar',
    saving: 'Guardando…',
    created: (name: string) => `${name} añadido.`,
    saved: (name: string) => `${name} guardado.`,
    test: 'Enviar una prueba',
    testing: 'Enviando…',
    testDelivered: (name: string) => `${name} ha recibido el mensaje de prueba.`,
    testFailed: (name: string) => `${name} no ha aceptado el mensaje de prueba.`,
    testAnswer: (answer: string) => `Respondió: ${answer}`,
    edit: 'Editar',
    delete: 'Borrar',
    deleteTitle: (name: string) => `¿Borrar ${name}?`,
    deleteBody: 'Se dejarán de enviar mensajes a este destino y se descartarán los pendientes.',
    deleted: (name: string) => `${name} borrado.`,
    status: {
      working: 'Funciona',
      waiting: 'Aún no se ha enviado nada',
      off: 'Desactivado',
      refused: 'Rechazado',
      rejected: 'Mensaje rechazado',
      unreachable: 'No accesible',
      unreadable: 'Vuelva a introducirlo',
    },
    answer: (code: string, email: boolean) => (email ? `SMTP ${code}` : `HTTP ${code}`),
    problem: {
      refused: (answer: string) =>
        `El destino ha rechazado a Polyfin (${answer}): compruebe su dirección, token o clave.`,
      refusedEmail: (answer: string) =>
        `El servidor SMTP ha rechazado a Polyfin (${answer}): compruebe su usuario y contraseña, la dirección del remitente y esta dirección.`,
      rejected: (answer: string) => `El destino ha rechazado el último mensaje (${answer}).`,
      unreachable:
        'No se han podido entregar los mensajes recientes: el destino no ha respondido o ha respondido con errores. Cada mensaje se reintenta durante una hora.',
      unreadable:
        'Su dirección, token o clave no se puede descifrar con POLYFIN_SECRET_KEY: no se le enviará nada hasta que se vuelva a introducir.',
    },
    lastSent: 'Último mensaje ',
    nothingSent: 'Aún no se ha enviado ningún mensaje',
    noEvents: 'No se ha elegido ningún evento',
  },
}

export default notifications
