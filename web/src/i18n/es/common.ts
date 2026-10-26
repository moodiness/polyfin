import type en from '../en/common'

const common: typeof en = {
  documentTitle: 'Administración de Polyfin',
  header: {
    productName: 'Polyfin',
  },
  language: {
    label: 'Idioma de la interfaz',
    en: { short: 'EN', name: 'English' },
    fr: { short: 'FR', name: 'Français' },
    de: { short: 'DE', name: 'Deutsch' },
    es: { short: 'ES', name: 'Español' },
    it: { short: 'IT', name: 'Italiano' },
    'pt-BR': { short: 'PT', name: 'Português (Brasil)' },
  },
  common: {
    enterNumber: 'Introduzca un número.',
    loading: 'Cargando…',
    save: 'Guardar',
    saving: 'Guardando…',
    cancel: 'Cancelar',
    retry: 'Reintentar',
    never: 'Nunca',
    colon: ':',
    passwordRule: 'Al menos 8 caracteres.',
    nameRule: "De 1 a 64 caracteres: letras, dígitos, espacios y - _ ' . @ +",
    passwordMismatch: 'Las dos contraseñas no coinciden.',
    moveUp: (name: string) => `Subir ${name}`,
    moveDown: (name: string) => `Bajar ${name}`,
    moved: (name: string, position: number, count: number) =>
      `${name} se ha movido a la posición ${position} de ${count}.`,
  },
  errors: {
    generic: 'Se ha producido un error. Inténtelo de nuevo.',
    network: 'No se pudo conectar con el servidor. Compruebe que Polyfin se está ejecutando.',
    unauthenticated: 'Su sesión ha caducado. Vuelva a iniciar sesión.',
    forbidden: 'No tiene permiso para realizar esta acción.',
    cross_origin: 'Se ha rechazado la solicitud porque no procedía de esta página.',
    invalid_setup_code:
      'Este código de configuración no es válido. Cópielo de nuevo del registro del servidor.',
    invalid_name:
      "Los nombres deben tener entre 1 y 64 caracteres: letras, dígitos, espacios y - _ ' . @ +",
    invalid_password: 'Las contraseñas deben tener al menos 8 caracteres.',
    setup_complete: 'La configuración ya se ha completado. Inicie sesión.',
    too_many_attempts: 'Demasiados intentos. Espere unos minutos antes de volver a intentarlo.',
    invalid_credentials: 'Nombre o contraseña incorrectos.',
    account_disabled: 'Esta cuenta está desactivada. Pida a un administrador que la active.',
    wrong_password: 'Su contraseña actual es incorrecta.',
    invalid_code:
      'Este código no es válido. Introduzca el código de 6 dígitos que muestra ahora su aplicación, o uno de sus códigos de recuperación.',
    two_step_expired:
      'El inicio de sesión ha tardado demasiado. Vuelva a introducir su nombre y contraseña.',
    two_step_on: 'El inicio de sesión en dos pasos ya está activado.',
    two_step_off: 'El inicio de sesión en dos pasos está desactivado para esta cuenta.',
    two_step_not_started:
      'Empiece de nuevo: active el inicio de sesión en dos pasos para obtener una nueva clave.',
    two_step_required:
      'Los administradores deben usar el inicio de sesión en dos pasos en este servidor: no se puede desactivar.',
    two_step_own: 'Desactive su propio inicio de sesión en dos pasos desde «Mi cuenta».',
    not_found: 'Este elemento ya no existe. Se ha actualizado la lista.',
    not_set:
      'No hay nada guardado: se eliminó o no se puede descifrar con POLYFIN_SECRET_KEY. Recargue la página.',
    not_revealable: 'Esta conexión no contiene ninguna clave que mostrar.',
    unknown_code:
      'Ningún dispositivo está esperando con este código. Puede que haya caducado o se haya escrito mal.',
    quick_connect_disabled: 'Quick Connect está desactivado en este servidor.',
    name_taken: 'Este nombre ya lo usa otro usuario.',
    invalid_invite_uses:
      'Las cuentas que puede crear un enlace de invitación deben ser un número entero entre 1 y 100.',
    invalid_invite_expiry: 'Elija en la lista cuándo caduca el enlace de invitación.',
    invalid_invite_model: 'Este usuario ya no existe. Elija otro.',
    invite_unknown: 'Este enlace de invitación no funciona.',
    invite_used_up: 'Este enlace de invitación ya se ha agotado.',
    invite_expired: 'Este enlace de invitación ha caducado.',
    invite_revoked: 'Este enlace de invitación se ha revocado.',
    share_links_off:
      'No puede crear enlaces para compartir: un administrador los ha desactivado para este servidor o para su cuenta.',
    not_shareable: 'Solo se puede compartir una película o un episodio que usted pueda ver.',
    invalid_share_expiry: 'Elija en la lista cuándo caduca el enlace.',
    invalid_share_viewers: 'Los espectadores simultáneos deben ser un número entero entre 1 y 5.',
    last_administrator:
      'Este es el último administrador activado: no se puede borrar, degradar ni desactivar.',
    admin_cannot_join_group:
      'Los administradores no pueden unirse a un grupo, ya que sus permisos son distintos. Recargue la página para ver quién lo es ahora.',
    invalid_group: 'Este grupo ya no existe. Recargue la página y elija otro.',
    follows_group:
      'Este usuario sigue a su grupo: cambie el grupo o desactive antes «Seguir al grupo».',
    invalid_copy_from: 'Este usuario ya no existe. Elija otro.',
    invalid_server_name: 'El nombre del servidor debe tener entre 1 y 64 caracteres.',
    invalid_language: 'Elija un idioma de la lista.',
    invalid_catalog_limit:
      'Los títulos leídos por catálogo deben ser un número entero entre 100 y 20.000.',
    invalid_channel_limit:
      'Los canales leídos por catálogo de Televisión en vivo deben ser un número entero entre 100 y 50.000.',
    invalid_played_percent: 'El umbral de visto debe ser un número entero entre 50 y 100.',
    invalid_resume_percent: 'El umbral para continuar debe ser un número entero entre 0 y 50.',
    resume_not_below_played: 'El umbral para continuar debe ser inferior al umbral de visto.',
    invalid_version_list_minutes:
      'El tiempo que se conservan las listas de versiones debe ser un número entero de minutos entre 1 y 360.',
    invalid_catalog_refresh_minutes:
      'El tiempo de actualización del catálogo debe ser un número entero de minutos entre 1 y 1440.',
    invalid_analysis_timeout:
      'El tiempo para analizar una versión debe ser un número entero de segundos entre 5 y 120.',
    invalid_version_attempts: 'Las versiones probadas deben ser un número entero entre 1 y 10.',
    invalid_version_order:
      'Elija como máximo cuatro criterios para ordenar las versiones, uno de cada tipo. Recargue la página.',
    invalid_version_codec_order:
      'El orden de los códecs de vídeo debe incluir cada códec una sola vez. Recargue la página.',
    invalid_version_source_order:
      'El orden de las fuentes debe incluir fuentes del servidor, cada una una sola vez. Recargue la página.',
    invalid_hide_versions_larger:
      'El tamaño a partir del cual se ocultan las versiones debe ser 0 o un número entero de GB entre 1 y 1000.',
    invalid_hidden_version_codecs:
      'Esta lista de códecs ocultos no es compatible. Recargue la página.',
    invalid_hidden_version_words:
      'Introduzca como máximo 50 palabras que oculten versiones, una por línea, de 100 caracteres como máximo cada una.',
    invalid_max_conversions:
      'Las conversiones de vídeo simultáneas deben ser un número entero entre 0 y 32.',
    invalid_max_conversion_height: 'Elija de la lista la calidad máxima del vídeo convertido.',
    invalid_subtitle_ocr:
      'Elija de la lista cuándo se convierten en texto los subtítulos en imagen.',
    invalid_subtitle_ocr_tracks:
      'Las pistas de subtítulos en imagen leídas a la vez deben ser un número entero entre 1 y 4.',
    invalid_encoder_preset: 'Elija una velocidad de codificación de la lista.',
    invalid_video_quality: 'La calidad de vídeo debe ser 0 o un número entero entre 1 y 51.',
    invalid_hardware_acceleration: 'Elija una tarjeta gráfica de la lista.',
    invalid_hardware_decoding_codecs:
      'Esta lista de formatos leídos en la tarjeta gráfica no es compatible. Recargue la página.',
    invalid_adaptive_quality: 'Elija de la lista cuándo ofrecer varias calidades.',
    invalid_tone_mapping_algorithm: 'Elija un método de mapeo de tonos de la lista.',
    invalid_tone_mapping_peak:
      'El brillo máximo debe ser 0 o un número entero de nits entre 100 y 10.000.',
    invalid_tone_mapping_desat: 'La desaturación de las luces debe ser un número entre 0 y 10.',
    invalid_processor_tone_mapping_height:
      'Elija de la lista la calidad máxima del HDR convertido por el procesador.',
    invalid_deinterlace_method: 'Elija de la lista un método de desentrelazado.',
    invalid_downmix_algorithm: 'Elija de la lista una mezcla estéreo.',
    invalid_downmix_boost: 'El volumen al mezclar a estéreo debe ser un número entre 0,5 y 3.',
    invalid_max_audio_channels: 'Elija de la lista el número máximo de canales de audio.',
    invalid_audio_bitrate_per_channel:
      'La tasa de bits de audio por canal debe ser 0 o un número entero de kb/s entre 32 y 320.',
    invalid_encoding_threads: 'Los hilos del procesador deben ser un número entero entre 0 y 64.',
    invalid_ahead_seconds:
      'Los segundos preparados por adelantado deben ser un número entero entre 30 y 600.',
    parental_control:
      'El control parental se aplica a esta cuenta: conserva las extensiones del servidor, que proporcionan las clasificaciones en las que se basa.',
    invalid_parental_control:
      'Este ajuste de control parental no es compatible. Vuelva a cargar la página.',
    invalid_max_playbacks:
      'Las reproducciones simultáneas deben ser un número entero entre 0 y 20.',
    invalid_max_bitrate: 'Elija de la lista la calidad máxima.',
    invalid_sync_play: 'Elija de la lista una opción de «Ver juntos».',
    invalid_manifest_url:
      'Introduzca la dirección del manifiesto de una extensión que empiece por https://, http:// o stremio:// y termine en /manifest.json.',
    addon_exists: 'Esta extensión ya está instalada aquí.',
    addon_unreachable:
      'No se pudo conectar con la extensión. Compruebe la dirección y que la extensión esté en línea, y vuelva a intentarlo.',
    invalid_manifest: 'Esta dirección no ha devuelto un manifiesto de extensión de Stremio.',
    private_network:
      'Esta extensión está en una dirección de red local. Solo los administradores pueden instalar este tipo de extensiones.',
    invalid_order:
      'La lista de extensiones ha cambiado mientras tanto. Se ha actualizado: vuelva a intentarlo.',
    invalid_library:
      'Una biblioteca hace referencia a un catálogo que ya no existe o no se puede explorar. Elimine las bibliotecas marcadas como no disponibles y vuelva a guardar.',
    invalid_library_name: 'Los nombres de biblioteca deben tener entre 1 y 64 caracteres.',
    invalid_library_genre:
      'El catálogo de una biblioteca no ofrece su género. Elija otro de la lista y vuelva a guardar.',
    invalid_library_max_items:
      'El número máximo de títulos de una biblioteca debe ser un número entero entre 1 y 20.000, o quedar vacío.',
    invalid_smart_list_name:
      'El nombre de una lista inteligente debe tener entre 1 y 64 caracteres.',
    invalid_smart_list_shown_as: 'Elija si la lista se muestra como biblioteca o como colección.',
    invalid_smart_list_catalogs:
      'Elija entre 1 y 50 catálogos de los que la lista obtenga sus títulos.',
    invalid_smart_list_rules:
      'Una regla está incompleta: asigne un valor a cada una y al menos un límite a cada año o valoración.',
    invalid_smart_list_sort: 'Elija cómo se ordena la lista.',
    invalid_smart_list_max_items:
      'El número máximo de títulos de una lista inteligente debe ser un número entero entre 1 y 20.000, o quedar vacío.',
    too_many_smart_lists: 'Ya hay 100 listas inteligentes aquí. Borre una para crear otra.',
    invalid_image: 'Elija una imagen JPEG, PNG o WebP de 10 MB como máximo.',
    invalid_image_url: 'Introduzca una dirección de imagen que empiece por https:// o http://.',
    image_unreachable:
      'No se pudo descargar ninguna imagen desde esta dirección. Compruebe que abre una imagen y vuelva a intentarlo.',
    image_private_network:
      'Esta imagen está en una dirección de red local. Solo los administradores pueden usar este tipo de direcciones.',
    invalid_login_attempts:
      'Las contraseñas incorrectas antes de bloquear una cuenta deben ser 0 o un número entero entre 3 y 20.',
    invalid_inactive_device_days:
      'Los días antes de cerrar la sesión de los dispositivos sin usar deben ser un número entero entre 0 y 365.',
    invalid_https_mode:
      'Servir HTTPS debe ser Desactivado, Archivos de certificado o Let’s Encrypt.',
    invalid_https_port: 'El puerto HTTPS debe ser un número entero entre 1 y 65535.',
    invalid_https_certificate:
      'El certificado debe ser una ruta absoluta en el contenedor, como /certs/fullchain.pem.',
    invalid_https_private_key:
      'La clave privada debe ser una ruta absoluta en el contenedor, como /certs/privkey.pem.',
    https_certificate_missing: 'No hay ningún archivo de certificado en esta ruta del contenedor.',
    https_private_key_missing:
      'No hay ningún archivo de clave privada en esta ruta del contenedor.',
    https_certificate_unreadable:
      'Polyfin no puede leer este archivo de certificado o no contiene ningún certificado PEM. Compruebe que el usuario 65532 pueda leerlo.',
    https_private_key_unreadable:
      'Polyfin no puede leer este archivo de clave privada o no contiene ninguna clave RSA o ECDSA en PEM. Compruebe que el usuario 65532 pueda leerlo.',
    https_files_mismatch: 'La clave privada no corresponde al certificado.',
    invalid_acme_hosts:
      'Los nombres de host deben ser nombres públicos como media.example.org, uno por línea, sin comodines ni direcciones IP, 100 como máximo y al menos uno para Let’s Encrypt.',
    invalid_acme_email:
      'El correo electrónico debe ser una dirección de correo válida o quedar vacío.',
    acme_terms_required: 'Acepte las condiciones de servicio de Let’s Encrypt para usarlo.',
    invalid_acme_challenge: 'El desafío debe ser HTTP-01 o TLS-ALPN-01.',
    invalid_known_proxies:
      'Los proxies conocidos deben ser direcciones IP, redes como 172.18.0.0/16 o nombres de host como traefik, uno por línea, 64 como máximo.',
    personal_addons_disabled:
      'Un administrador ha desactivado sus extensiones propias. Se conservan, pero no puede añadir, actualizar ni activar ninguna.',
    invalid_hidden_libraries:
      'La lista de bibliotecas ha cambiado mientras tanto. Vuelva a cargar la página.',
    invalid_blocked_genres:
      'Los géneros deben tener entre 1 y 100 caracteres, y se pueden bloquear 100 como máximo.',
    invalid_access_schedules:
      'Las horas van de 00:00 a 24:00, y cada fin debe ser posterior a su inicio (50 filas como máximo).',
    outside_allowed_hours:
      'Esta cuenta no se puede usar en este momento. Vuelva a intentarlo durante su horario permitido.',
    invalid_guide_url:
      'Introduzca una dirección de guía que empiece por https:// o http://, de 4096 caracteres como máximo.',
    invalid_app:
      'El nombre de la aplicación debe tener entre 1 y 64 caracteres, sin caracteres especiales.',
    invalid_trickplay_interval:
      'El tiempo entre dos miniaturas debe ser un número entero de segundos entre 5 y 60.',
    invalid_trickplay_width: 'Elija de la lista el ancho de las miniaturas.',
    invalid_thumbnail_storage_gb:
      'El espacio para imágenes debe ser un número entero de GB entre 1 y 50.',
    invalid_recording_padding:
      'Los minutos antes y después de una grabación deben ser números enteros entre 0 y 60.',
    invalid_recording_retention_days:
      'Los días de conservación de las grabaciones deben ser un número entero de 0 a 3650.',
    invalid_convert_recordings_hours:
      'Elija en las listas dos horas distintas entre las que convertir las grabaciones.',
    invalid_quality_group: 'Elija el grupo de calidad en la lista.',
    invalid_live_tv_refresh_hours:
      'El intervalo de actualización debe ser un número entero de horas de 1 a 168.',
    invalid_local_scan_hours:
      'El intervalo de escaneo debe ser un número entero de horas de 0 a 168.',
    invalid_custom_css: 'El CSS personalizado no debe superar los 2 MB.',
    invalid_custom_js: 'El JavaScript personalizado no debe superar los 2 MB.',
    invalid_web_theme: 'Elija LumaaGlaass, Ninguno o Personalizado.',
    theme_copy_running:
      'Ya se está haciendo una copia del tema. Vuelva a intentarlo en un momento.',
    theme_copy_nothing: 'Los textos personalizados guardados no cargan nada desde otro sitio.',
    theme_copy_failed: 'No se pudo guardar ningún archivo, así que no se hizo ninguna copia.',
    custom_texts_changed:
      'Los textos personalizados cambiaron mientras se hacía la copia. Vuelva a intentarlo.',
    invalid_login_disclaimer: 'El mensaje de inicio de sesión no debe superar los 8 KB.',
    invalid_publicmetadb_key:
      'PublicMetaDB no ha aceptado esta clave. Cópiela de nuevo desde su cuenta de PublicMetaDB. No se ha guardado nada.',
    publicmetadb_unreachable:
      'No se pudo contactar con PublicMetaDB para comprobar la clave. No se ha guardado nada: inténtelo de nuevo más tarde.',
    invalid_theintrodb_key:
      'TheIntroDB no ha aceptado esta clave. Cópiela de nuevo desde su cuenta de TheIntroDB. No se ha guardado nada.',
    theintrodb_unreachable:
      'No se pudo contactar con TheIntroDB para comprobar la clave, o está limitando las solicitudes por ahora. No se ha guardado nada: inténtelo de nuevo más tarde.',
    invalid_mdblist_ratings_key:
      'MDBList no ha aceptado esta clave. Cópiela de nuevo desde su cuenta de MDBList. No se ha guardado nada.',
    mdblist_unreachable:
      'No se pudo contactar con MDBList para comprobar la clave. No se ha guardado nada: inténtelo de nuevo más tarde.',
    invalid_tmdb_key:
      'TMDB no ha aceptado esta clave. Copie de nuevo la clave API o el token de acceso de lectura desde su cuenta de TMDB. No se ha guardado nada.',
    tmdb_unreachable:
      'No se pudo contactar con TMDB para comprobar la clave. No se ha guardado nada: inténtelo de nuevo más tarde.',
    invalid_critic_rating_shows:
      'Elija en la lista qué puntuación muestra la valoración de la crítica.',
    invalid_measure_loudness: 'Elija en la lista en qué canciones se mide el volumen.',
    invalid_segment_order:
      'El orden de las fuentes de marcadores de salto debe incluir cada fuente una sola vez. Recargue la página.',
    invalid_trakt_app:
      'El ID de cliente y el secreto de Trakt deben tener como máximo 256 caracteres, sin espacios ni caracteres especiales. Cópielos de nuevo desde su aplicación de Trakt.',
    invalid_simkl_app:
      'El ID de cliente de Simkl debe tener como máximo 256 caracteres, sin espacios ni caracteres especiales. Cópielo de nuevo desde su aplicación de Simkl.',
    invalid_backup_hour: 'Elija en la lista la hora de las copias de seguridad.',
    invalid_backups_kept:
      'El número de copias de seguridad conservadas debe ser un número entero de 1 a 90.',
    invalid_torrent_route:
      'Las transmisiones torrent necesitan una sola ruta: un nombre de interfaz de red (solo Linux), un proxy SOCKS5 con el formato host:puerto o una configuración de WireGuard.',
    invalid_wireguard_config: 'La configuración de WireGuard no es válida.',
    invalid_torrent_limit: 'Los límites deben ser números enteros de Mbps de 0 a 10.000.',
    invalid_cache_size: 'El espacio en disco debe ser un número entero de GB de 1 a 2000.',
    invalid_vaapi_device: 'Elija la tarjeta gráfica en la lista.',
    invalid_recordings_folder:
      'La carpeta de grabaciones debe ser una ruta absoluta a una carpeta en la que Polyfin pueda escribir.',
    invalid_backup_folder:
      'La carpeta de copias de seguridad debe ser una ruta absoluta a una carpeta en la que Polyfin pueda escribir.',
    invalid_conversion_folder:
      'La carpeta de conversión debe ser una ruta absoluta a una carpeta en la que Polyfin pueda escribir.',
    invalid_reading_folder:
      'La carpeta de archivos en lectura debe ser una ruta absoluta a una carpeta en la que Polyfin pueda escribir.',
    invalid_cache_folder:
      'La carpeta de caché debe ser una ruta absoluta a una carpeta en la que Polyfin pueda escribir.',
    too_many_destinations: 'Ya hay 10 destinos de las copias de seguridad: borre uno primero.',
    too_many_home_rows: 'Ya hay 20 filas de inicio: borre una primero.',
    invalid_home_row:
      'Asigne a la fila un nombre de 100 caracteres como máximo y elija su catálogo o su colección.',
    invalid_destination_kind: 'Elija el tipo de destino en la lista.',
    invalid_destination_address:
      'Introduzca la dirección solicitada: una dirección http:// o https:// sin ruta para un almacenamiento S3, un nombre de host o una dirección IP para un servidor SFTP, la dirección de una carpeta para un servidor WebDAV.',
    invalid_bucket:
      'Introduzca el nombre del bucket: de 3 a 63 letras minúsculas, dígitos, puntos, guiones o guiones bajos.',
    invalid_destination_folder:
      'La carpeta no debe subir de nivel con «..» ni contener caracteres de control, y tiene como máximo 512 bytes.',
    invalid_region: 'La región se compone de letras minúsculas, dígitos y guiones, como eu-west-3.',
    invalid_port: 'El puerto es un número entero de 1 a 65535.',
    invalid_destination_user:
      'Introduzca el usuario, o la clave de acceso para un almacenamiento S3.',
    invalid_destination_secret:
      'Introduzca la contraseña, la clave secreta para un almacenamiento S3 o, para un servidor SFTP, una clave privada.',
    invalid_private_key:
      'La clave privada debe ser una clave privada OpenSSH o PEM sin frase de contraseña.',
    backups_off: 'Las copias de seguridad están desactivadas: actívelas primero más arriba.',
    pg_restore_missing:
      'No se encontró pg_restore en el servidor: instale las herramientas de cliente de PostgreSQL 18, que incluye la imagen de Docker.',
    backup_not_found: 'Esta copia de seguridad ya no existe: elija otra.',
    not_a_backup: 'Este archivo no es una copia de seguridad de la base de datos de Polyfin.',
    newer_backup:
      'Esta copia de seguridad la creó una versión más reciente de Polyfin: actualice Polyfin primero.',
    changed_backup:
      'Esta copia de seguridad la creó un Polyfin cuyas migraciones de la base de datos difieren de las de este.',
    restore_expired: 'La copia de seguridad elegida ya no está lista: vuelva a elegirla.',
    unreadable_secrets:
      'Esta copia de seguridad contiene claves que POLYFIN_SECRET_KEY no puede descifrar: confirme con «Restaurar de todos modos».',
    restoring: 'Ya hay una restauración en curso.',
    wrong_server_name: 'Escriba el nombre del servidor exactamente como se muestra.',
    too_large: 'El archivo es demasiado grande.',
    destination_failed: 'No se pudo leer el destino.',
    restore_failed: 'No se pudo iniciar la restauración: no se ha cambiado nada.',
    invalid_collection_read_hour:
      'Elija en la lista la hora a la que se leen las colecciones, o «Nunca».',
    invalid_collection_library_apps:
      'Elija en la lista las aplicaciones que muestran las bibliotecas de colecciones por colección.',
    invalid_remuxdb_url:
      'La dirección de RemuxDB debe ser una dirección web que empiece por http:// o https://.',
    invalid_update_tool_url:
      'La dirección de la herramienta de actualización debe ser una dirección web que empiece por http:// o https://, sin nombre de usuario ni fragmento.',
    invalid_update_tool_token:
      'El token de la herramienta de actualización no lleva espacios y tiene 1024 caracteres como máximo.',
    restart_unconfirmed: 'Polyfin no sabe si algo lo vuelve a iniciar: confirme antes el aviso.',
    no_update: 'Polyfin Desktop no ofrece esa actualización ahora.',
    not_desktop: 'Polyfin no se ejecuta en Polyfin Desktop.',
    no_update_tool: 'No hay ninguna herramienta de actualización en los ajustes generales.',
    update_tool_timeout:
      'La herramienta de actualización no respondió en 50 segundos: puede que siga actualizando.',
    update_tool_unreachable: 'No se pudo contactar con la herramienta de actualización.',
    invalid_public_address:
      'La dirección pública debe ser una dirección web que empiece por http:// o https://, sin parámetros de consulta.',
    invalid_local_address:
      'La dirección en la red local debe ser una dirección web que empiece por http:// o https://, como http://192.168.1.10:8096, sin parámetros de consulta.',
    invalid_dlna_name: 'El nombre en la red puede tener como máximo 256 bytes.',
    invalid_dlna_user: 'Ese usuario ya no existe. Elija otro, o «Nadie».',
    invalid_dlna_profile:
      'Esto no es un perfil DLNA en el formato XML del complemento DLNA de Jellyfin.',
    invalid_smtp_host:
      'Introduzca el nombre de host del servidor SMTP, como smtp.example.org, sin espacios.',
    invalid_smtp_port: 'El puerto SMTP debe ser un número entero de 1 a 65535.',
    invalid_smtp_security: 'Elija en la lista cómo se cifra la conexión SMTP.',
    invalid_smtp_account:
      'El usuario y la contraseña SMTP tienen como máximo 256 caracteres, en una sola línea.',
    invalid_smtp_sender:
      'Introduzca solo la dirección del remitente, como polyfin@example.org, y un nombre de hasta 128 caracteres.',
    invalid_weekly_summary_day: 'Elija en la lista el día del resumen semanal.',
    invalid_weekly_summary_hour: 'Elija en la lista la hora del resumen semanal.',
    invalid_import_every_minutes:
      'Elija en la lista cada cuánto se importan las cuentas conectadas.',
    invalid_target_kind: 'Elija un tipo de destino de la lista.',
    invalid_target_name: 'El nombre de un destino debe tener de 1 a 64 caracteres.',
    invalid_target_address: 'Introduzca una dirección web que empiece por https:// o http://.',
    private_target_address:
      'Esta dirección está en una red local: solo los destinos de un administrador pueden acceder a ella.',
    invalid_email_address: 'Introduzca una dirección de correo electrónico, como sam@example.org.',
    email_unavailable:
      'El correo electrónico necesita un servidor SMTP: un administrador lo configura en Ajustes › Notificaciones.',
    invalid_topic: 'Un tema de ntfy tiene de 1 a 64 letras, dígitos, guiones y guiones bajos.',
    invalid_chat:
      'Introduzca el número del chat, como -1001234567890, o el nombre de un canal público, como @news_example.',
    invalid_token:
      'Introduzca el token: hasta 256 caracteres, sin espacios. El de un bot de Telegram tiene este aspecto: 123456:ABC-DEF.',
    invalid_user_key: 'Una clave de usuario de Pushover solo contiene letras y dígitos.',
    invalid_events: 'Uno de los eventos elegidos no está disponible para este destino.',
    invalid_reminder_settings: 'Elija 1, 5, 10, 15, 30 o 60 minutos.',
    too_many_targets: 'Ya hay 20 destinos. Borre uno para añadir otro.',
    target_unreadable:
      'La dirección o el token de este destino no se pueden descifrar con POLYFIN_SECRET_KEY. Introdúzcalo de nuevo.',
    invalid_source_name: 'El nombre de una fuente debe tener de 1 a 64 caracteres.',
    invalid_source_address:
      'Introduzca una dirección que empiece por https:// o http://, y, para una cuenta de Xtream Codes, un nombre de usuario y una contraseña.',
    tuner_added: 'Este sintonizador ya es una fuente del servidor.',
    invalid_folder_name: 'El nombre de una carpeta debe tener de 1 a 64 caracteres.',
    invalid_folder_path:
      'Introduzca la ruta completa de la carpeta donde se ejecuta Polyfin, distinta de la raíz del sistema: en su contenedor, empezando por /; en Windows, con su unidad, como D:\\Peliculas, o su recurso compartido, como \\\\nas\\media\\Peliculas.',
    invalid_share_address:
      'Introduzca una dirección como smb://servidor/recurso/carpeta, https://servidor/carpeta, s3://bucket/prefijo, sftp://servidor/carpeta o rclone://servidor:puerto/carpeta, sin usuario ni contraseña: tienen sus propios campos.',
    invalid_share_user: 'Los usuarios tienen como máximo 256 caracteres, y las contraseñas, 1024.',
    invalid_share_endpoint:
      'Introduzca el endpoint como una dirección http:// o https://, como https://s3.example.org, y una región de como máximo 64 caracteres sin espacios.',
    invalid_share_key:
      'Pegue una clave privada OpenSSH o PEM completa, y la frase de contraseña que la abre si tiene una.',
    invalid_rclone_remote:
      'Introduzca el nombre de uno de los remotos de rclone, sin dos puntos ni barra, de como máximo 128 caracteres.',
    invalid_poll_minutes:
      'Introduzca un número de minutos de 0 a 1440; 0 para buscar cambios solo en los escaneos.',
    invalid_folder_kind: 'Elija si la carpeta contiene películas o series.',
    invalid_imdb_id: 'Un identificador de IMDb es tt seguido de dígitos, como tt0063350.',
    invalid_episode:
      'Escriba el episodio como aparece en el nombre de un archivo, como S01E05, o S01E05-E06 para dos.',
    title_search_failed:
      'No se pudo buscar en las extensiones de metadatos. Vuelva a intentarlo en un momento.',
    invalid_channel_list:
      'Esta dirección no devolvió una lista de canales. Compruébela, y que la cuenta siga siendo válida.',
    channel_list_too_large:
      'Esta lista de canales es demasiado grande (100 MB o 100.000 canales como máximo).',
    iptv_login_refused: 'El servidor IPTV rechazó este nombre de usuario o contraseña.',
    invalid_message: 'Escriba un mensaje de 1 a 500 caracteres.',
    not_controllable:
      'Esta aplicación no acepta control remoto: no se puede detener ni enviarle un mensaje desde aquí.',
    remote_control_refused:
      'Su cuenta no puede controlar las aplicaciones de otros usuarios. Otro administrador puede darle este permiso en la página Usuarios.',
    too_soon:
      'Esta extensión se comprobó hace menos de un minuto. Vuelva a intentarlo en un momento.',
    invalid_limit: 'Esta lista no puede mostrar tantas entradas.',
    addon_kind_changed:
      'Esta dirección lleva ahora a otro tipo de extensión (una extensión de música en lugar de una de vídeo, o al revés). Instálela como una extensión nueva.',
    invalid_addon_settings:
      'La extensión rechazó estos ajustes: un valor no es una de sus opciones, es demasiado largo o está fuera de rango. Revise los campos y vuelva a intentarlo.',
    invalid_options:
      'Estas opciones de importación no se aceptan. Recargue la página y vuelva a intentarlo.',
    invalid_category_name: 'El nombre de una categoría debe tener de 1 a 64 caracteres.',
    category_not_custom: 'Solo se pueden borrar sus propias categorías.',
    invalid_channel_name: 'El nombre de un canal debe tener de 1 a 100 caracteres.',
    invalid_logo:
      'Introduzca una dirección de logotipo que empiece por https:// o http://, de como máximo 4096 caracteres.',
    invalid_description: 'Las descripciones deben tener como máximo 2000 caracteres.',
    invalid_category: 'Esta categoría ya no existe. Recargue la página.',
    invalid_number: 'Introduzca un número entero de 1 a 99.999, o deje el número vacío.',
    invalid_move: 'Este canal solo se puede mover dentro de su categoría. Recargue la página.',
    invalid_bulk: 'Elija canales, o una palabra clave de al menos 2 caracteres.',
    invalid_streams: 'Las transmisiones cambiaron mientras tanto. Recargue el canal.',
    invalid_stream_url:
      'Introduzca una dirección de transmisión que empiece por https:// o http://, de como máximo 4096 caracteres.',
    stream_not_custom: 'Solo se pueden eliminar las transmisiones que usted añadió.',
    too_many_guides: 'Un catálogo admite como máximo 10 guías.',
    invalid_guide: 'Esta guía ya no existe. Recargue la página.',
    invalid_mode: 'Este modo de asignación no es compatible. Recargue la página.',
    invalid_mapping: 'Este canal de guía ya no está en las guías del catálogo. Vuelva a buscar.',
    invalid_jellyfin_address:
      'Introduzca la dirección del servidor Jellyfin, por ejemplo http://192.168.1.10:8096.',
    jellyfin_key_refused:
      'Jellyfin ha rechazado esta clave API. Cree una en su panel de control, en Claves API, y péguela de nuevo.',
    jellyfin_key_limited:
      'Esta clave o cuenta no puede listar los usuarios del servidor. Use una clave API de su panel de control o la cuenta de un administrador.',
    jellyfin_key_owner_only:
      'Esta clave solo lee los datos de visionado de su propietario. Escriba las contraseñas de los demás usuarios del servidor o desmarque «Importar datos de visionado» para ellos.',
    jellyfin_sign_in_refused: 'Jellyfin ha rechazado este nombre o contraseña.',
    jellyfin_sign_in_forbidden:
      'Jellyfin no permite iniciar sesión con esta cuenta: puede estar desactivada o fuera de su horario permitido.',
    jellyfin_password_refused:
      'Jellyfin ha rechazado esta contraseña. Déjela vacía si la cuenta no tiene ninguna.',
    jellyfin_other_user:
      'Esta contraseña ha abierto la cuenta de otro usuario del servidor. Conéctese en su lugar con una clave API de su panel de control.',
    jellyfin_unreachable:
      'Nada ha respondido en esta dirección. Compruébela y verifique que Jellyfin está en ejecución.',
    not_jellyfin:
      'Un servidor ha respondido en esta dirección, pero no es Jellyfin. Compruebe la dirección.',
    jellyfin_import_running: 'Ya hay una importación en curso. Espere a que termine o deténgala.',
    unknown_jellyfin_user:
      'Un usuario de Jellyfin ya no está en ese servidor. Vuelva a conectarse para leer sus usuarios.',
    emby_user_key:
      'Esta clave pertenece a un usuario y Emby no indica a quién. Cree una clave API en los ajustes de Emby o conéctese con una cuenta de usuario.',
    server_import_running:
      'Hay otra importación en curso en este servidor o en esta cuenta. Inténtelo de nuevo en unos minutos.',
    server_imports_disabled:
      'La importación del historial de visionado desde otro servidor o aplicación está desactivada en este servidor.',
    letterboxd_export_invalid:
      'Este archivo no es una exportación de Letterboxd: elija el archivo ZIP que proporciona Letterboxd, tal cual. Contiene watched.csv o diary.csv.',
    letterboxd_export_too_large:
      'Este archivo supera los 20 MB o incluye más de 50.000 películas: elija el archivo ZIP que proporciona Letterboxd.',
    invalid_nuvio_address:
      'Introduzca la dirección del backend de Nuvio, por ejemplo https://api.nuvio.tv, o déjela vacía para usar el de Nuvio.',
    nuvio_sign_in_refused: 'Nuvio ha rechazado este correo electrónico o contraseña.',
    nuvio_unreachable:
      'El backend de Nuvio no ha respondido. Compruebe su dirección e inténtelo de nuevo más tarde.',
    not_nuvio:
      'Algo ha respondido en esta dirección, pero no es un backend de Nuvio. Compruebe la dirección o déjela vacía para usar el de Nuvio.',
    nuvio_rate_limited:
      'Nuvio pide esperar antes de volver a iniciar sesión. Inténtelo de nuevo en unos minutos.',
    nuvio_backup_unsupported:
      'Nuvio ha devuelto los datos de la cuenta en un formato que esta versión de Polyfin no lee. Polyfin lee la versión 1 de la copia de seguridad de cuenta de Nuvio.',
    stremio_sign_in_refused: 'Stremio ha rechazado este correo electrónico o contraseña.',
    stremio_key_refused:
      'Stremio ha rechazado esta clave de autenticación. Cópiela de nuevo desde la aplicación web de Stremio, con la sesión iniciada en su cuenta.',
    stremio_link_expired: 'El código de vinculación ha caducado o ya se ha usado. Solicite otro.',
    stremio_unreachable: 'Stremio no ha respondido. Inténtelo de nuevo más tarde.',
    stremio_unexpected:
      'Stremio ha respondido de una forma que esta versión de Polyfin no lee. Inténtelo de nuevo más tarde.',
    app_import_expired:
      'Lo que se leyó de la cuenta ya no se conserva tras unos minutos. Vuelva a iniciar sesión.',
    app_import_running: 'Hay una importación en curso en esta cuenta. Espere a que termine.',
    app_not_connected: 'Esta aplicación ya no está conectada a la cuenta.',
    app_connection_stopped:
      'La aplicación ha rechazado la sesión que Polyfin conservaba. Vuelva a iniciar sesión con «Mantener la sesión iniciada» marcado.',
    invalid_ends_on: 'Elija la fecha de fin de la cuenta, o «Nunca».',
    administrator_cannot_end:
      'La cuenta de un administrador nunca termina. Conviértalo primero en miembro para darle una fecha de fin.',
    invalid_invite_account_end: 'Elija en la lista cuándo terminan las cuentas.',
  },
  status: {
    title: 'Estado del servidor',
    description: 'Estado del sistema de este servidor Polyfin de un vistazo.',
    version: 'Versión',
    serverId: 'ID del servidor',
    database: 'Base de datos',
    databaseReady: 'Lista',
    databaseUnavailable: 'No disponible',
    loading: 'Cargando el estado del servidor…',
    errorTitle: 'No se puede acceder al servidor',
    errorBody:
      'La API de administración no ha respondido. Compruebe que Polyfin se está ejecutando y vuelva a intentarlo.',
    staleWarning:
      'La última actualización ha fallado. Es posible que los datos siguientes no estén al día.',
    retry: 'Reintentar',
    retrying: 'Reintentando…',
    autoRefresh: 'Se actualiza automáticamente cada 10 segundos.',
    updatedAt: (time: string) => `Última actualización a las ${time}`,
  },
  devices: {
    empty: 'No hay ningún dispositivo con la sesión iniciada.',
    lastActivity: 'Última actividad',
    address: 'Dirección',
    signOut: 'Cerrar sesión',
    signOutConfirm: (device: string) =>
      `¿Cerrar la sesión de «${device}»? La aplicación tendrá que volver a iniciar sesión.`,
    signedOut: (device: string) => `Se ha cerrado la sesión de «${device}».`,
  },
  stremioTypes: {
    movie: 'Películas',
    series: 'Series',
    channel: 'Canales',
    tv: 'TV',
    track: 'Pistas',
    album: 'Álbumes',
    artist: 'Artistas',
    playlist: 'Listas de reproducción',
    other: 'Otros vídeos',
  },
  stremioResources: {
    catalog: 'Catálogos',
    meta: 'Detalles',
    stream: 'Transmisiones',
    subtitles: 'Subtítulos',
    addon_catalog: 'Catálogos de extensiones',
    search: 'Búsqueda',
    isrc: 'Identificación de pistas',
    resolve: 'Enlaces de reproducción',
    settings: 'Ajustes',
  },
  forbidden: {
    title: 'Acceso denegado',
    description: 'Solo los administradores pueden abrir esta página.',
    home: 'Volver a la página de inicio',
  },
  time: {
    justNow: 'ahora mismo',
  },
  ui: {
    on: 'Activado',
    off: 'Desactivado',
    show: 'Mostrar',
    hide: 'Ocultar',
    close: 'Cerrar',
    dismiss: 'Descartar',
    notifications: 'Notificaciones',
    unsaved: 'Cambios sin guardar',
    allSaved: 'Todos los cambios guardados',
    discard: 'Descartar cambios',
    searchList: 'Buscar en la lista',
    noMatch: 'No hay coincidencias.',
    secret: {
      saved: 'Guardada',
      notSet: 'Sin definir',
      removing: 'Se eliminará al guardar',
      pastePlaceholder: 'Péguela aquí',
      replacePlaceholder: 'Pegue la nueva',
      replace: 'Reemplazar',
      cancelReplace: 'Conservar la guardada',
      remove: 'Eliminar',
      keep: 'Conservarla',
      removeHint: 'Guarde para eliminarla o seleccione «Conservarla».',
      show: 'Mostrar clave',
      hide: 'Ocultar clave',
      savedHidden: 'Clave guardada, oculta',
    },
  },
}

export default common
