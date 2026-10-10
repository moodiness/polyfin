import type en from '../en/common'

const common: typeof en = {
  documentTitle: 'Administration Polyfin',
  header: {
    productName: 'Polyfin',
  },
  language: {
    label: 'Langue de l’interface',
    en: { short: 'EN', name: 'English' },
    fr: { short: 'FR', name: 'Français' },
  },
  common: {
    enterNumber: 'Saisissez un nombre.',
    loading: 'Chargement…',
    save: 'Enregistrer',
    saving: 'Enregistrement…',
    cancel: 'Annuler',
    retry: 'Réessayer',
    never: 'Jamais',
    colon: '\u202F:',
    passwordRule: 'Au moins 8 caractères.',
    nameRule: "De 1 à 64 caractères : lettres, chiffres, espaces et - _ ' . @ +",
    passwordMismatch: 'Les deux mots de passe ne correspondent pas.',
    moveUp: (name: string) => `Monter ${name}`,
    moveDown: (name: string) => `Descendre ${name}`,
    moved: (name: string, position: number, count: number) =>
      `${name} est maintenant en position ${position} sur ${count}.`,
  },
  errors: {
    generic: 'Une erreur est survenue. Veuillez réessayer.',
    network: 'Impossible de joindre le serveur. Vérifiez que Polyfin est bien lancé.',
    unauthenticated: 'Votre session a expiré. Veuillez vous reconnecter.',
    forbidden: 'Vous n’êtes pas autorisé à effectuer cette action.',
    cross_origin: 'La requête a été refusée, car elle ne provient pas de cette page.',
    invalid_setup_code:
      'Ce code d’installation n’est pas valide. Copiez-le à nouveau depuis le journal du serveur.',
    invalid_name:
      "Le nom doit comporter de 1 à 64 caractères : lettres, chiffres, espaces et - _ ' . @ +",
    invalid_password: 'Le mot de passe doit comporter au moins 8 caractères.',
    setup_complete: 'L’installation est déjà terminée. Connectez-vous.',
    too_many_attempts: 'Trop de tentatives. Patientez quelques minutes avant de réessayer.',
    invalid_credentials: 'Nom ou mot de passe incorrect.',
    account_disabled: 'Ce compte est désactivé. Demandez à un administrateur de le réactiver.',
    wrong_password: 'Votre mot de passe actuel est incorrect.',
    not_found: 'Cet élément n’existe plus. La liste a été actualisée.',
    not_set:
      'Aucune valeur enregistrée : elle a été supprimée, ou ne peut pas être déchiffrée avec POLYFIN_SECRET_KEY. Rechargez la page.',
    not_revealable: 'Cette connexion n’a pas de clé à afficher.',
    unknown_code: 'Aucun appareil n’attend avec ce code. Il a peut-être expiré ou été mal saisi.',
    quick_connect_disabled: 'Quick Connect est désactivé sur ce serveur.',
    name_taken: 'Ce nom est déjà utilisé par un autre utilisateur.',
    last_administrator:
      'C’est le dernier administrateur actif : il ne peut être ni supprimé, ni rétrogradé, ni désactivé.',
    invalid_server_name: 'Le nom du serveur doit comporter de 1 à 64 caractères.',
    invalid_language: 'Choisissez la langue du serveur dans la liste.',
    invalid_catalog_limit:
      'Le nombre de titres lus par catalogue doit être un nombre entier de 100 à 20 000.',
    invalid_channel_limit:
      'Le nombre de chaînes lues par catalogue de TV en direct doit être un nombre entier de 100 à 50 000.',
    invalid_played_percent:
      'Le pourcentage « Marqué comme vu après » doit être un nombre entier de 50 à 100.',
    invalid_resume_percent:
      'Le pourcentage « Point de reprise gardé après » doit être un nombre entier de 0 à 50.',
    resume_not_below_played:
      'Le pourcentage « Point de reprise gardé après » doit être plus petit que « Marqué comme vu après ».',
    invalid_version_list_minutes:
      'La durée de conservation des listes de versions doit être un nombre entier de minutes de 1 à 360.',
    invalid_catalog_refresh_minutes:
      'Le délai de rafraîchissement des catalogues doit être un nombre entier de minutes de 1 à 1 440.',
    invalid_analysis_timeout:
      'Le temps pour analyser une version doit être un nombre entier de secondes de 5 à 120.',
    invalid_version_attempts:
      'Le nombre de versions essayées doit être un nombre entier de 1 à 10.',
    invalid_max_conversions:
      'Le nombre de conversions vidéo en même temps doit être un nombre entier de 0 à 32.',
    invalid_max_conversion_height: 'Choisissez la qualité max des vidéos converties dans la liste.',
    invalid_encoder_preset: 'Choisissez une vitesse d’encodage dans la liste.',
    invalid_video_quality: 'La qualité vidéo doit être 0, ou un nombre entier de 1 à 51.',
    invalid_hardware_acceleration: 'Choisissez une carte graphique dans la liste.',
    invalid_hardware_decoding_codecs:
      'Cette liste de formats lus par la carte graphique n’est pas prise en charge. Rechargez la page.',
    invalid_tone_mapping_algorithm: 'Choisissez une méthode de conversion HDR dans la liste.',
    invalid_tone_mapping_peak:
      'La luminosité max doit être 0, ou un nombre entier de nits de 100 à 10 000.',
    invalid_tone_mapping_desat: 'La désaturation des zones claires doit être un nombre de 0 à 10.',
    invalid_deinterlace_method: 'Choisissez une méthode de désentrelacement dans la liste.',
    invalid_downmix_algorithm: 'Choisissez un mixage stéréo dans la liste.',
    invalid_downmix_boost: 'Le volume du mixage stéréo doit être un nombre de 0,5 à 3.',
    invalid_max_audio_channels: 'Choisissez le nombre max de canaux audio dans la liste.',
    invalid_audio_bitrate_per_channel:
      'Le débit audio par canal doit être 0, ou un nombre entier de kb/s de 32 à 320.',
    invalid_encoding_threads:
      'Le nombre de threads du processeur doit être un nombre entier de 0 à 64.',
    invalid_ahead_seconds:
      'Le nombre de secondes préparées à l’avance doit être un nombre entier de 30 à 600.',
    parental_control:
      'Le contrôle parental s’applique à ce compte : il garde les addons du serveur, qui donnent les classifications sur lesquelles il s’appuie.',
    invalid_parental_control:
      'Ce réglage du contrôle parental n’est pas pris en charge. Rechargez la page.',
    invalid_max_playbacks:
      'Le nombre de lectures en même temps doit être un nombre entier de 0 à 20.',
    invalid_max_bitrate: 'Choisissez la qualité maximale dans la liste.',
    invalid_sync_play: 'Choisissez une option de « Regarder ensemble » dans la liste.',
    invalid_manifest_url:
      'Saisissez l’adresse du manifeste d’un addon, commençant par https://, http:// ou stremio:// et se terminant par /manifest.json.',
    addon_exists: 'Cet addon est déjà installé ici.',
    addon_unreachable:
      'Impossible de joindre l’addon. Vérifiez l’adresse et que l’addon est en ligne, puis réessayez.',
    invalid_manifest: 'Cette adresse n’a pas renvoyé de manifeste d’addon Stremio.',
    private_network:
      'Cet addon se trouve à une adresse du réseau local. Seuls les administrateurs peuvent installer ce type d’addon.',
    invalid_order: 'La liste des addons a changé entre-temps. Elle a été actualisée : réessayez.',
    invalid_library:
      'Une bibliothèque fait référence à un catalogue qui n’existe plus ou ne peut pas être parcouru. Retirez les bibliothèques signalées comme plus disponibles, puis enregistrez à nouveau.',
    invalid_library_name: 'Le nom d’une bibliothèque doit comporter de 1 à 64 caractères.',
    invalid_library_genre:
      'Le genre d’une bibliothèque n’est pas proposé par son catalogue. Choisissez-en un autre dans la liste, puis enregistrez à nouveau.',
    invalid_library_max_items:
      'Le nombre maximum de titres d’une bibliothèque doit être un nombre entier de 1 à 20 000, ou rester vide.',
    invalid_image: 'Choisissez une image JPEG, PNG ou WebP de 10 Mo au plus.',
    invalid_image_url: 'Saisissez une adresse d’image commençant par https:// ou http://.',
    image_unreachable:
      'Aucune image n’a pu être téléchargée depuis cette adresse. Vérifiez qu’elle ouvre bien une image, puis réessayez.',
    image_private_network:
      'Cette image est sur une adresse du réseau local. Seuls les administrateurs peuvent utiliser de telles adresses.',
    invalid_login_attempts:
      'Le nombre de mots de passe faux avant blocage doit être 0, ou un nombre entier de 3 à 20.',
    invalid_inactive_device_days:
      'Le nombre de jours avant de déconnecter les appareils inutilisés doit être un nombre entier de 0 à 365.',
    personal_addons_disabled:
      'Un administrateur a désactivé vos propres addons. Ils sont conservés, mais vous ne pouvez pas en ajouter, ni les actualiser, ni les activer.',
    invalid_hidden_libraries: 'La liste des bibliothèques a changé entre-temps. Rechargez la page.',
    invalid_blocked_genres:
      'Un genre doit comporter de 1 à 100 caractères, et 100 genres au plus peuvent être bloqués.',
    invalid_access_schedules:
      'Les horaires vont de 00:00 à 24:00, et chaque fin doit venir après son début (50 lignes au plus).',
    outside_allowed_hours:
      'Ce compte ne peut pas être utilisé à cette heure-ci. Réessayez pendant ses horaires autorisés.',
    invalid_guide_url:
      'Saisissez une adresse de guide commençant par https:// ou http://, de 4 096 caractères au plus.',
    invalid_app:
      'Le nom de l’application doit comporter de 1 à 64 caractères, sans caractères spéciaux.',
    invalid_trickplay_interval:
      'Le temps entre deux miniatures doit être un nombre entier de secondes de 5 à 60.',
    invalid_trickplay_width: 'Choisissez la largeur des miniatures dans la liste.',
    invalid_thumbnail_storage_gb:
      'La place pour les images doit être un nombre entier de Go de 1 à 50.',
    invalid_recording_padding:
      'Les minutes avant et après un enregistrement doivent être un nombre entier de 0 à 60.',
    invalid_recording_retention_days:
      'Les jours de conservation des enregistrements doivent être un nombre entier de 0 à 3 650.',
    invalid_quality_group: 'Choisissez le groupe de qualité dans la liste.',
    invalid_live_tv_refresh_hours:
      'L’intervalle d’actualisation doit être un nombre entier d’heures de 1 à 168.',
    invalid_local_scan_hours:
      'L’intervalle d’analyse doit être un nombre entier d’heures de 0 à 168.',
    invalid_custom_css: 'Le CSS personnalisé ne doit pas dépasser 2 Mo.',
    invalid_custom_js: 'Le JavaScript personnalisé ne doit pas dépasser 2 Mo.',
    invalid_login_disclaimer: 'Le message de connexion ne doit pas dépasser 8 Ko.',
    invalid_publicmetadb_key:
      'PublicMetaDB n’a pas accepté cette clé. Copiez-la de nouveau depuis votre compte PublicMetaDB. Rien n’a été enregistré.',
    publicmetadb_unreachable:
      'PublicMetaDB n’a pas pu être joint pour vérifier la clé. Rien n’a été enregistré : réessayez plus tard.',
    invalid_theintrodb_key:
      'TheIntroDB n’a pas accepté cette clé. Copiez-la de nouveau depuis votre compte TheIntroDB. Rien n’a été enregistré.',
    theintrodb_unreachable:
      'TheIntroDB n’a pas pu être joint pour vérifier la clé, ou limite les demandes pour le moment. Rien n’a été enregistré : réessayez plus tard.',
    invalid_segment_order:
      'L’ordre des sources de repères doit citer chaque source une fois. Rechargez la page.',
    invalid_trakt_app:
      'Le client ID et le client secret Trakt doivent faire au plus 256 caractères, sans espaces ni caractères spéciaux. Copiez-les de nouveau depuis votre application Trakt.',
    invalid_simkl_app:
      'Le client ID Simkl doit faire au plus 256 caractères, sans espaces ni caractères spéciaux. Copiez-le de nouveau depuis votre application Simkl.',
    invalid_backup_hour: 'Choisissez l’heure des sauvegardes dans la liste.',
    invalid_backups_kept:
      'Le nombre de sauvegardes conservées doit être un nombre entier de 1 à 90.',
    invalid_cache_size: 'L’espace disque doit être un nombre entier de Go, de 1 à 2000.',
    invalid_vaapi_device: 'Choisissez la carte graphique dans la liste.',
    invalid_recordings_folder:
      'Le dossier des enregistrements doit être un chemin absolu vers un dossier où Polyfin peut écrire.',
    invalid_backup_folder:
      'Le dossier des sauvegardes doit être un chemin absolu vers un dossier où Polyfin peut écrire.',
    invalid_collection_read_hour:
      'Choisissez dans la liste l’heure de lecture des collections, ou Jamais.',
    invalid_remuxdb_url:
      'L’adresse de RemuxDB doit être une adresse web commençant par http:// ou https://.',
    invalid_public_address:
      'L’adresse publique doit être une adresse web commençant par http:// ou https://, sans paramètres.',
    invalid_smtp_host:
      'Saisissez le nom d’hôte du serveur SMTP, comme smtp.example.org, sans espaces.',
    invalid_smtp_port: 'Le port SMTP doit être un nombre entier de 1 à 65535.',
    invalid_smtp_security: 'Choisissez dans la liste le chiffrement de la connexion SMTP.',
    invalid_smtp_account:
      'L’utilisateur et le mot de passe SMTP font au plus 256 caractères, sur une ligne.',
    invalid_smtp_sender:
      'Saisissez l’adresse de l’expéditeur seule, comme polyfin@example.org, et un nom de 128 caractères au plus.',
    invalid_target_kind: 'Choisissez un type de cible dans la liste.',
    invalid_target_name: 'Le nom d’une cible doit faire de 1 à 64 caractères.',
    invalid_target_address: 'Saisissez une adresse web commençant par https:// ou http://.',
    private_target_address:
      'Cette adresse est sur un réseau local : seules les cibles d’un administrateur peuvent y accéder.',
    invalid_email_address: 'Saisissez une adresse e-mail, comme sam@example.org.',
    email_unavailable:
      'L’e-mail demande un serveur SMTP : un administrateur le règle dans Paramètres › Notifications.',
    invalid_topic: 'Un sujet ntfy fait de 1 à 64 lettres, chiffres, tirets et tirets bas.',
    invalid_chat:
      'Saisissez le numéro de la discussion, comme -1001234567890, ou le nom d’une chaîne publique, comme @news_example.',
    invalid_token:
      'Saisissez le jeton : 256 caractères au plus, sans espaces. Celui d’un bot Telegram ressemble à 123456:ABC-DEF.',
    invalid_user_key: 'Une clé d’utilisateur Pushover ne contient que des lettres et des chiffres.',
    invalid_events: 'Un des événements choisis n’est pas disponible pour cette cible.',
    too_many_targets: 'Il y a déjà 20 cibles. Supprimez-en une pour en ajouter une autre.',
    target_unreadable:
      'L’adresse ou le jeton de cette cible ne peut pas être déchiffré avec POLYFIN_SECRET_KEY. Saisissez-le de nouveau.',
    invalid_source_name: 'Le nom d’une source doit faire de 1 à 64 caractères.',
    invalid_source_address:
      'Saisissez une adresse commençant par https:// ou http://, et pour un compte Xtream Codes un identifiant et un mot de passe.',
    invalid_folder_name: 'Le nom d’un dossier doit faire de 1 à 64 caractères.',
    invalid_folder_path:
      'Saisissez le chemin complet du dossier dans le conteneur de Polyfin, commençant par /, autre que / lui-même.',
    invalid_share_address:
      'Saisissez une adresse comme smb://serveur/partage/dossier ou https://serveur/dossier, sans utilisateur ni mot de passe : ils ont leurs propres champs.',
    invalid_share_user:
      'Un utilisateur fait au plus 256 caractères, un mot de passe au plus 1 024.',
    invalid_folder_kind: 'Choisissez si le dossier contient des films ou des séries.',
    invalid_imdb_id: 'Un identifiant IMDb est tt suivi de chiffres, comme tt0063350.',
    invalid_channel_list:
      'Cette adresse n’a pas renvoyé de liste de chaînes. Vérifiez-la, et que le compte est toujours valable.',
    channel_list_too_large:
      'Cette liste de chaînes est trop grande (100 Mo ou 100 000 chaînes au plus).',
    iptv_login_refused: 'Le serveur IPTV a refusé cet identifiant ou ce mot de passe.',
    invalid_message: 'Écrivez un message de 1 à 500 caractères.',
    not_controllable:
      'Cette appli n’accepte pas le contrôle à distance : impossible de l’arrêter ou de lui envoyer un message d’ici.',
    remote_control_refused:
      'Votre compte ne peut pas contrôler les applis des autres utilisateurs. Un autre administrateur peut vous donner ce droit sur la page Utilisateurs.',
    too_soon: 'Cet addon a été vérifié il y a moins d’une minute. Réessayez dans un instant.',
    invalid_limit: 'Cette liste ne peut pas afficher autant d’éléments.',
    addon_kind_changed:
      'Cette adresse mène maintenant à un autre type d’addon (de musique au lieu de vidéo, ou l’inverse). Installez-le plutôt comme un nouvel addon.',
    invalid_addon_settings:
      'L’addon a refusé ces réglages : une valeur ne fait pas partie de ses choix, est trop longue ou hors limites. Vérifiez les champs et réessayez.',
    invalid_options: 'Ces options d’import ne sont pas acceptées. Rechargez la page et réessayez.',
    invalid_category_name: 'Le nom d’une catégorie doit comporter de 1 à 64 caractères.',
    category_not_custom: 'Seules vos propres catégories peuvent être supprimées.',
    invalid_channel_name: 'Le nom d’une chaîne doit comporter de 1 à 100 caractères.',
    invalid_logo:
      'Saisissez une adresse de logo commençant par https:// ou http://, de 4 096 caractères au plus.',
    invalid_description: 'La description doit comporter 2 000 caractères au plus.',
    invalid_category: 'Cette catégorie n’existe plus. Rechargez la page.',
    invalid_number: 'Saisissez un nombre entier de 1 à 99 999, ou laissez le numéro vide.',
    invalid_move: 'Cette chaîne ne peut bouger que dans sa catégorie. Rechargez la page.',
    invalid_bulk: 'Choisissez des chaînes, ou un mot-clé d’au moins 2 caractères.',
    invalid_streams: 'Les flux ont changé entre-temps. Rechargez la chaîne.',
    invalid_stream_url:
      'Saisissez une adresse de flux commençant par https:// ou http://, de 4 096 caractères au plus.',
    stream_not_custom: 'Seuls les flux que vous avez ajoutés peuvent être retirés.',
    too_many_guides: 'Un catalogue prend 10 guides au plus.',
    invalid_guide: 'Ce guide n’existe plus. Rechargez la page.',
    invalid_mode: 'Ce mode de correspondance n’est pas pris en charge. Rechargez la page.',
    invalid_mapping:
      'Cette chaîne de guide n’est plus dans les guides du catalogue. Cherchez à nouveau.',
    invalid_jellyfin_address:
      'Saisissez l’adresse du serveur Jellyfin, par exemple http://192.168.1.10:8096.',
    jellyfin_key_refused:
      'Jellyfin a refusé cette clé d’API. Créez-en une dans son tableau de bord, sous Clés API, puis collez-la à nouveau.',
    jellyfin_key_limited:
      'Cette clé ou ce compte ne peut pas lister les utilisateurs du serveur. Utilisez une clé d’API de son tableau de bord, ou le compte d’un administrateur.',
    jellyfin_key_owner_only:
      'Cette clé ne lit que les données de visionnage de son propriétaire. Saisissez le mot de passe des autres utilisateurs sur le serveur, ou décochez « Importer les données de visionnage » pour eux.',
    jellyfin_sign_in_refused: 'Jellyfin a refusé ce nom ou ce mot de passe.',
    jellyfin_sign_in_forbidden:
      'Jellyfin ne laisse pas ce compte se connecter : il est peut-être désactivé, ou hors de ses horaires autorisés.',
    jellyfin_password_refused:
      'Jellyfin a refusé ce mot de passe. Laissez vide si le compte n’en a pas.',
    jellyfin_other_user:
      'Ce mot de passe a ouvert le compte d’un autre utilisateur sur le serveur. Connectez-vous plutôt avec une clé d’API de son tableau de bord.',
    jellyfin_unreachable:
      'Rien n’a répondu à cette adresse. Vérifiez-la, et que Jellyfin est bien lancé.',
    not_jellyfin:
      'Un serveur a répondu à cette adresse, mais ce n’est pas Jellyfin. Vérifiez l’adresse.',
    jellyfin_import_running:
      'Une importation depuis Jellyfin est déjà en cours. Attendez qu’elle se termine, ou arrêtez-la.',
    unknown_jellyfin_user:
      'Un utilisateur Jellyfin n’est plus sur ce serveur. Reconnectez-vous pour relire ses utilisateurs.',
  },
  status: {
    title: 'État du serveur',
    description: 'L’essentiel sur le fonctionnement de ce serveur Polyfin.',
    version: 'Version',
    serverId: 'Identifiant du serveur',
    database: 'Base de données',
    databaseReady: 'Prête',
    databaseUnavailable: 'Indisponible',
    loading: 'Chargement de l’état du serveur…',
    errorTitle: 'Serveur injoignable',
    errorBody:
      'L’API d’administration ne répond pas. Vérifiez que Polyfin est bien lancé, puis réessayez.',
    staleWarning:
      'La dernière actualisation a échoué. Les informations ci-dessous ne sont peut-être plus à jour.',
    retry: 'Réessayer',
    retrying: 'Nouvelle tentative…',
    autoRefresh: 'Actualisation automatique toutes les 10 secondes.',
    updatedAt: (time: string) => `Dernière mise à jour à ${time}`,
  },
  devices: {
    empty: 'Aucun appareil connecté.',
    lastActivity: 'Dernière activité',
    address: 'Adresse',
    signOut: 'Déconnecter',
    signOutConfirm: (device: string) =>
      `Déconnecter « ${device} » ? L’application devra se reconnecter.`,
    signedOut: (device: string) => `« ${device} » a été déconnecté.`,
  },
  stremioTypes: {
    movie: 'Films',
    series: 'Séries',
    channel: 'Chaînes',
    tv: 'TV',
    track: 'Titres',
    album: 'Albums',
    artist: 'Artistes',
    playlist: 'Playlists',
  },
  stremioResources: {
    catalog: 'Catalogues',
    meta: 'Fiches',
    stream: 'Flux',
    subtitles: 'Sous-titres',
    addon_catalog: 'Catalogues d’addons',
    search: 'Recherche',
    isrc: 'Correspondance des titres',
    resolve: 'Liens de lecture',
    settings: 'Réglages',
  },
  forbidden: {
    title: 'Accès refusé',
    description: 'Seuls les administrateurs peuvent ouvrir cette page.',
    home: 'Retour à l’accueil',
  },
  time: {
    justNow: 'à l’instant',
  },
  ui: {
    on: 'Activé',
    off: 'Désactivé',
    show: 'Afficher',
    hide: 'Masquer',
    close: 'Fermer',
    dismiss: 'Fermer le message',
    notifications: 'Notifications',
    unsaved: 'Modifications non enregistrées',
    allSaved: 'Tout est enregistré',
    discard: 'Annuler les modifications',
    searchList: 'Chercher dans la liste',
    noMatch: 'Rien ne correspond.',
    secret: {
      saved: 'Valeur enregistrée',
      notSet: 'Aucune valeur',
      removing: 'Supprimée à l’enregistrement',
      pastePlaceholder: 'Collez la valeur ici',
      replacePlaceholder: 'Collez la nouvelle valeur',
      replace: 'Remplacer',
      cancelReplace: 'Garder la valeur enregistrée',
      remove: 'Supprimer',
      keep: 'Garder la valeur',
      removeHint: 'Enregistrez pour la supprimer, ou choisissez «\u202FGarder la valeur\u202F».',
      show: 'Afficher la clé',
      hide: 'Masquer la clé',
      savedHidden: 'Clé enregistrée, masquée',
    },
  },
}

export default common
