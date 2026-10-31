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
    de: { short: 'DE', name: 'Deutsch' },
    es: { short: 'ES', name: 'Español' },
    it: { short: 'IT', name: 'Italiano' },
    'pt-BR': { short: 'PT', name: 'Português (Brasil)' },
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
    invalid_code:
      'Ce code n’est pas valide. Saisissez le code à 6 chiffres que votre application affiche maintenant, ou l’un de vos codes de secours.',
    two_step_expired:
      'La connexion a pris trop de temps. Saisissez de nouveau votre nom et votre mot de passe.',
    two_step_on: 'La connexion en deux étapes est déjà activée.',
    two_step_off: 'La connexion en deux étapes est désactivée pour ce compte.',
    two_step_not_started:
      'Recommencez : activez la connexion en deux étapes pour obtenir une nouvelle clé.',
    two_step_required:
      'Les administrateurs doivent utiliser la connexion en deux étapes sur ce serveur : elle ne peut pas être désactivée.',
    two_step_own: 'Désactivez votre propre connexion en deux étapes depuis Mon compte.',
    not_found: 'Cet élément n’existe plus. La liste a été actualisée.',
    not_set:
      'Aucune valeur enregistrée : elle a été supprimée, ou ne peut pas être déchiffrée avec POLYFIN_SECRET_KEY. Rechargez la page.',
    not_revealable: 'Cette connexion n’a pas de clé à afficher.',
    unknown_code: 'Aucun appareil n’attend avec ce code. Il a peut-être expiré ou été mal saisi.',
    quick_connect_disabled: 'Quick Connect est désactivé sur ce serveur.',
    name_taken: 'Ce nom est déjà utilisé par un autre utilisateur.',
    invalid_invite_uses:
      'Les comptes qu’un lien d’invitation peut créer doivent être un nombre entier de 1 à 100.',
    invalid_invite_expiry: 'Choisissez l’expiration du lien d’invitation dans la liste.',
    invalid_invite_model: 'Cet utilisateur n’existe plus. Choisissez-en un autre.',
    invite_unknown: 'Ce lien d’invitation ne fonctionne pas.',
    invite_used_up: 'Ce lien d’invitation est épuisé.',
    invite_expired: 'Ce lien d’invitation a expiré.',
    invite_revoked: 'Ce lien d’invitation a été révoqué.',
    share_links_off:
      'Vous ne pouvez pas créer de lien de partage : un administrateur les a désactivés pour ce serveur ou pour votre compte.',
    not_shareable: 'Seul un film ou un épisode que vous pouvez voir peut être partagé.',
    invalid_share_expiry: 'Choisissez l’expiration du lien dans la liste.',
    invalid_share_viewers:
      'Le nombre de spectateurs à la fois doit être un nombre entier de 1 à 5.',
    last_administrator:
      'C’est le dernier administrateur actif : il ne peut être ni supprimé, ni rétrogradé, ni désactivé.',
    admin_cannot_join_group:
      'Les administrateurs ne peuvent pas rejoindre un groupe, car leurs autorisations diffèrent. Rechargez la page pour voir qui l’est à présent.',
    invalid_group: 'Ce groupe n’existe plus. Rechargez la page et choisissez-en un autre.',
    follows_group:
      'Cet utilisateur suit son groupe : modifiez le groupe, ou désactivez d’abord « Suivre le groupe ».',
    invalid_copy_from: 'Cet utilisateur n’existe plus. Choisissez-en un autre.',
    invalid_server_name: 'Le nom du serveur doit comporter de 1 à 64 caractères.',
    invalid_language: 'Choisissez une langue dans la liste.',
    invalid_used_by: 'Un groupe ou un utilisateur coché n’existe plus. Rechargez la page.',
    invalid_audio_region: 'Choisissez une région dans la liste.',
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
    invalid_version_order:
      'Choisissez au plus six critères pour trier les versions, un de chaque sorte. Rechargez la page.',
    invalid_version_codec_order:
      'L’ordre des codecs vidéo doit citer chaque codec une fois. Rechargez la page.',
    invalid_version_source_order:
      'L’ordre des sources doit citer des sources du serveur, chacune une fois. Rechargez la page.',
    invalid_hide_versions_larger:
      'La taille au-delà de laquelle les versions sont masquées doit être 0, ou un nombre entier de Go de 1 à 1 000.',
    invalid_hidden_version_codecs:
      'Cette liste de codecs masqués n’est pas prise en charge. Rechargez la page.',
    invalid_hidden_version_words:
      'Saisissez au plus 50 mots masquant des versions, un par ligne, de 100 caractères au plus chacun.',
    invalid_hidden_releases:
      'Cette liste de genres de sorties masqués n’est pas prise en charge. Rechargez la page.',
    invalid_version_edition_order:
      'L’ordre des éditions doit citer chaque édition une fois. Rechargez la page.',
    invalid_version_mb_per_minute:
      'La taille par minute doit être un nombre entier de Mo de 0 à 1 000.',
    invalid_version_label_rules:
      'Saisissez au plus 40 règles de libellés, chacune d’au plus 200 caractères, une expression régulière valide là où elle est cochée, et un score de −100 à 100 autre que 0 sauf pour masquer.',
    invalid_max_conversions:
      'Le nombre de conversions vidéo en même temps doit être un nombre entier de 0 à 32.',
    invalid_max_conversion_height: 'Choisissez la qualité max des vidéos converties dans la liste.',
    invalid_subtitle_ocr:
      'Choisissez dans la liste quand les sous-titres en images sont lus en texte.',
    invalid_subtitle_ocr_tracks:
      'Le nombre de pistes de sous-titres en images lues en même temps doit être un nombre entier de 1 à 4.',
    invalid_default_language: 'Choisissez une langue par défaut dans la liste.',
    invalid_encoder_preset: 'Choisissez une vitesse d’encodage dans la liste.',
    invalid_video_quality: 'La qualité vidéo doit être 0, ou un nombre entier de 1 à 51.',
    invalid_hardware_acceleration: 'Choisissez une carte graphique dans la liste.',
    invalid_hardware_decoding_codecs:
      'Cette liste de formats lus par la carte graphique n’est pas prise en charge. Rechargez la page.',
    invalid_adaptive_quality: 'Choisissez quand proposer plusieurs qualités dans la liste.',
    invalid_tone_mapping_algorithm: 'Choisissez une méthode de conversion HDR dans la liste.',
    invalid_tone_mapping_peak:
      'La luminosité max doit être 0, ou un nombre entier de nits de 100 à 10 000.',
    invalid_tone_mapping_desat: 'La désaturation des zones claires doit être un nombre de 0 à 10.',
    invalid_processor_tone_mapping_height:
      'Choisissez la qualité max du HDR converti par le processeur dans la liste.',
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
    invalid_smart_list_name: 'Le nom d’une liste intelligente doit comporter de 1 à 64 caractères.',
    invalid_smart_list_shown_as:
      'Choisissez si la liste s’affiche comme une bibliothèque ou comme une collection.',
    invalid_smart_list_catalogs: 'Choisissez de 1 à 50 catalogues dont la liste tire ses titres.',
    invalid_smart_list_rules:
      'Une règle est incomplète : donnez une valeur à chacune, et au moins une borne à chaque année ou note.',
    invalid_smart_list_sort: 'Choisissez comment la liste est triée.',
    invalid_smart_list_max_items:
      'Le nombre maximum de titres d’une liste intelligente doit être un nombre entier de 1 à 20 000, ou rester vide.',
    too_many_smart_lists:
      'Il y a déjà 100 listes intelligentes ici. Supprimez-en une pour en créer une autre.',
    invalid_own_channel_name: 'Le nom d’une chaîne doit comporter de 1 à 64 caractères.',
    invalid_channel_number:
      'Le numéro d’une chaîne doit être un nombre entier de 1 à 9 999, ou rester vide.',
    invalid_channel_source:
      'Choisissez une collection, une liste intelligente ou une série, ou au moins un titre, qui existe encore.',
    channel_without_titles:
      'Les titres choisis ne contiennent aucun film ni épisode à diffuser, ou plus de 5 000. Choisissez-en d’autres.',
    invalid_channel_blocks:
      'Les tranches horaires doivent finir après leur début, dans la journée, sans se chevaucher, 12 au plus.',
    too_many_channels:
      'Il y a déjà 100 chaînes de vos titres. Supprimez-en une pour en créer une autre.',
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
    invalid_https_mode:
      'Servir HTTPS doit valoir Désactivé, Fichiers de certificat ou Let’s Encrypt.',
    invalid_https_port: 'Le port HTTPS doit être un nombre entier de 1 à 65535.',
    invalid_https_certificate:
      'Le certificat doit être un chemin absolu dans le conteneur, comme /certs/fullchain.pem.',
    invalid_https_private_key:
      'La clé privée doit être un chemin absolu dans le conteneur, comme /certs/privkey.pem.',
    https_certificate_missing: 'Aucun fichier de certificat à ce chemin dans le conteneur.',
    https_private_key_missing: 'Aucun fichier de clé privée à ce chemin dans le conteneur.',
    https_certificate_unreadable:
      'Polyfin ne peut pas lire ce fichier de certificat, ou il ne contient aucun certificat PEM. Vérifiez que l’utilisateur 65532 peut le lire.',
    https_private_key_unreadable:
      'Polyfin ne peut pas lire ce fichier de clé privée, ou il ne contient aucune clé RSA ou ECDSA en PEM. Vérifiez que l’utilisateur 65532 peut le lire.',
    https_files_mismatch: 'La clé privée ne correspond pas au certificat.',
    invalid_acme_hosts:
      'Les noms d’hôte doivent être des noms publics comme media.example.org, un par ligne, sans joker ni adresse IP, 100 au plus, et au moins un pour Let’s Encrypt.',
    invalid_acme_email: 'L’e-mail doit être une adresse e-mail, ou rester vide.',
    acme_terms_required: 'Acceptez les conditions d’utilisation de Let’s Encrypt pour l’utiliser.',
    invalid_acme_challenge: 'Le défi doit être HTTP-01 ou TLS-ALPN-01.',
    invalid_known_proxies:
      'Les proxys connus doivent être des adresses IP, des réseaux comme 172.18.0.0/16 ou des noms d’hôte comme traefik, un par ligne, 64 au plus.',
    personal_addons_disabled:
      'Un administrateur a désactivé vos propres addons. Ils sont conservés, mais vous ne pouvez pas en ajouter, ni les actualiser, ni les activer.',
    invalid_hidden_libraries: 'La liste des bibliothèques a changé entre-temps. Rechargez la page.',
    invalid_blocked_genres:
      'Un genre doit comporter de 1 à 100 caractères, et 100 genres au plus peuvent être bloqués.',
    invalid_live_tv_channels:
      'Le choix des chaînes n’est pas valide : « Toutes », « Seulement celles-ci » ou « Toutes sauf celles-ci », avec au plus 10 000 groupes et 10 000 chaînes.',
    invalid_access_schedules:
      'Les horaires vont de 00:00 à 24:00, et chaque fin doit venir après son début (50 lignes au plus).',
    outside_allowed_hours:
      'Ce compte ne peut pas être utilisé à cette heure-ci. Réessayez pendant ses horaires autorisés.',
    outside_local_network:
      'Ce compte ne peut être utilisé que depuis le réseau local. Connectez-vous depuis celui-ci, ou demandez à un administrateur.',
    remote_access_self:
      'Vous êtes connecté depuis l’extérieur du réseau local : désactiver vos propres connexions depuis l’extérieur vous bloquerait. Faites-le depuis le réseau local.',
    invalid_guide_url:
      'Saisissez une adresse de guide commençant par https:// ou http://, de 4 096 caractères au plus, ou le chemin absolu d’un fichier du serveur.',
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
    invalid_convert_recordings_hours:
      'Choisissez dans les listes deux heures différentes entre lesquelles convertir les enregistrements.',
    invalid_quality_group: 'Choisissez le groupe de qualité dans la liste.',
    invalid_live_tv_refresh_hours:
      'L’intervalle d’actualisation doit être un nombre entier d’heures de 1 à 168.',
    invalid_local_scan_hours:
      'L’intervalle d’analyse doit être un nombre entier d’heures de 0 à 168.',
    invalid_watchlist_check_hours:
      'L’intervalle de vérification des titres à regarder doit être un nombre entier d’heures de 0 à 168.',
    invalid_custom_css: 'Le CSS personnalisé ne doit pas dépasser 2 Mo.',
    invalid_custom_js: 'Le JavaScript personnalisé ne doit pas dépasser 2 Mo.',
    invalid_web_theme: 'Choisissez un thème dans la liste.',
    web_theme_locked: 'Votre administrateur a choisi le thème de votre lecteur web.',
    theme_copy_running: 'Une copie du thème est déjà en cours. Réessayez dans un instant.',
    theme_copy_nothing: 'Les textes personnalisés enregistrés ne chargent rien d’ailleurs.',
    theme_copy_failed: 'Aucun fichier n’a pu être enregistré, donc aucune copie n’a été faite.',
    custom_texts_changed: 'Les textes personnalisés ont changé pendant la copie. Réessayez.',
    invalid_login_disclaimer: 'Le message de connexion ne doit pas dépasser 8 Ko.',
    invalid_publicmetadb_key:
      'PublicMetaDB n’a pas accepté cette clé. Copiez-la de nouveau depuis votre compte PublicMetaDB. Rien n’a été enregistré.',
    publicmetadb_unreachable:
      'PublicMetaDB n’a pas pu être joint pour vérifier la clé. Rien n’a été enregistré : réessayez plus tard.',
    invalid_theintrodb_key:
      'TheIntroDB n’a pas accepté cette clé. Copiez-la de nouveau depuis votre compte TheIntroDB. Rien n’a été enregistré.',
    theintrodb_unreachable:
      'TheIntroDB n’a pas pu être joint pour vérifier la clé, ou limite les demandes pour le moment. Rien n’a été enregistré : réessayez plus tard.',
    invalid_mdblist_ratings_key:
      'MDBList n’a pas accepté cette clé. Copiez-la de nouveau depuis votre compte MDBList. Rien n’a été enregistré.',
    mdblist_unreachable:
      'MDBList n’a pas pu être joint pour vérifier la clé. Rien n’a été enregistré : réessayez plus tard.',
    invalid_tmdb_key:
      'TMDB n’a pas accepté cette clé. Copiez de nouveau la clé d’API ou le jeton d’accès en lecture depuis votre compte TMDB. Rien n’a été enregistré.',
    unknown_episode_group:
      'TMDB ne donne plus cet ordre pour cette série. Rouvrez-la et choisissez-en un autre.',
    episode_groups_unreachable:
      'TMDB n’a pas pu être joint pour lister les ordres de cette série. Rien n’a été enregistré : réessayez plus tard.',
    no_tmdb_key:
      'Ajoutez d’abord une clé TMDB dans Paramètres › Contenu : les ordres viennent de TMDB.',
    tmdb_unreachable:
      'TMDB n’a pas pu être joint pour vérifier la clé. Rien n’a été enregistré : réessayez plus tard.',
    invalid_critic_rating_shows: 'Choisissez dans la liste le score qu’affiche la note critique.',
    invalid_poster_badge_size: 'Choisissez dans la liste la taille des badges.',
    invalid_poster_template:
      'Le modèle d’adresse des affiches doit être une adresse web commençant par http:// ou https://, nommant {imdb_id} ou {tmdb_id}, sans espace, nom d’utilisateur ni fragment.',
    invalid_measure_loudness: 'Choisissez dans la liste les chansons dont le volume est mesuré.',
    invalid_segment_order:
      'L’ordre des sources de repères doit citer chaque source une fois. Rechargez la page.',
    invalid_trakt_app:
      'Le client ID et le client secret Trakt doivent faire au plus 256 caractères, sans espaces ni caractères spéciaux. Copiez-les de nouveau depuis votre application Trakt.',
    invalid_simkl_app:
      'Le client ID Simkl doit faire au plus 256 caractères, sans espaces ni caractères spéciaux. Copiez-le de nouveau depuis votre application Simkl.',
    invalid_backup_hour: 'Choisissez l’heure des sauvegardes dans la liste.',
    invalid_backups_kept:
      'Le nombre de sauvegardes conservées doit être un nombre entier de 1 à 90.',
    invalid_torrent_route:
      'Les flux torrent demandent une seule route : un nom d’interface réseau (Linux seulement), un proxy SOCKS5 sous la forme hôte:port, ou une configuration WireGuard.',
    invalid_wireguard_config: 'La configuration WireGuard n’est pas valide.',
    invalid_torrent_limit: 'Les limites doivent être des nombres entiers de Mbit/s, de 0 à 10000.',
    invalid_torrent_trackers:
      'Saisissez 30 trackers au plus, chacun une URL d’annonce HTTP, HTTPS ou UDP, UDP avec son port, de 500 caractères au plus.',
    invalid_cache_size: 'L’espace disque doit être un nombre entier de Go, de 1 à 2000.',
    invalid_vaapi_device: 'Choisissez la carte graphique dans la liste.',
    invalid_recordings_folder:
      'Le dossier des enregistrements doit être un chemin absolu vers un dossier où Polyfin peut écrire.',
    invalid_backup_folder:
      'Le dossier des sauvegardes doit être un chemin absolu vers un dossier où Polyfin peut écrire.',
    invalid_conversion_folder:
      'Le dossier de conversion doit être un chemin absolu vers un dossier où Polyfin peut écrire.',
    invalid_reading_folder:
      'Le dossier des fichiers en cours de lecture doit être un chemin absolu vers un dossier où Polyfin peut écrire.',
    invalid_cache_folder:
      'Le dossier du cache doit être un chemin absolu vers un dossier où Polyfin peut écrire.',
    too_many_destinations:
      'Il y a déjà 10 destinations des sauvegardes : supprimez-en une d’abord.',
    too_many_home_rows: 'Il y a déjà 20 rangées d’accueil : supprimez-en une d’abord.',
    invalid_home_row:
      'Donnez à la rangée un nom de 100 caractères au plus, et choisissez son catalogue ou sa collection.',
    invalid_destination_kind: 'Choisissez le type de destination dans la liste.',
    invalid_destination_address:
      'Saisissez l’adresse demandée : une adresse http:// ou https:// sans chemin pour un stockage S3, un nom d’hôte ou une adresse IP pour un serveur SFTP, l’adresse d’un dossier pour un serveur WebDAV.',
    invalid_bucket:
      'Saisissez le nom du compartiment : 3 à 63 minuscules, chiffres, points, traits d’union ou tirets bas.',
    invalid_destination_folder:
      'Le dossier ne doit pas remonter avec « .. », ni contenir de caractères de contrôle, et fait au plus 512 octets.',
    invalid_region:
      'La région est faite de minuscules, de chiffres et de traits d’union, comme eu-west-3.',
    invalid_port: 'Le port est un nombre entier de 1 à 65535.',
    invalid_destination_user: 'Saisissez l’utilisateur, ou la clé d’accès pour un stockage S3.',
    invalid_destination_secret:
      'Saisissez le mot de passe, la clé secrète pour un stockage S3, ou pour un serveur SFTP une clé privée.',
    invalid_private_key: 'La clé privée doit être une clé OpenSSH ou PEM sans phrase de passe.',
    backups_off: 'Les sauvegardes sont désactivées : activez-les d’abord ci-dessus.',
    pg_restore_missing:
      'pg_restore est introuvable sur le serveur : installez les outils clients de PostgreSQL 18, que l’image Docker contient.',
    backup_not_found: 'Cette sauvegarde n’est plus là : choisissez-en une autre.',
    not_a_backup: 'Ce fichier n’est pas une sauvegarde de la base de données de Polyfin.',
    newer_backup:
      'Cette sauvegarde a été faite par un Polyfin plus récent : mettez d’abord Polyfin à jour.',
    changed_backup:
      'Cette sauvegarde a été faite par un Polyfin dont les migrations de la base de données diffèrent de celles de celui-ci.',
    restore_expired: 'La sauvegarde choisie n’est plus prête : choisissez-la à nouveau.',
    unreadable_secrets:
      'Cette sauvegarde contient des clés que POLYFIN_SECRET_KEY ne peut pas déchiffrer : confirmez avec Restaurer quand même.',
    restoring: 'Une restauration est déjà en cours.',
    wrong_server_name: 'Saisissez le nom du serveur exactement comme il est affiché.',
    too_large: 'Le fichier est trop volumineux.',
    destination_failed: 'La destination n’a pas pu être lue.',
    restore_failed: 'La restauration n’a pas pu commencer : rien n’a été modifié.',
    invalid_backup_passphrase:
      'La phrase de passe des sauvegardes fait au moins 12 caractères, sur une ligne, et 256 octets au plus.',
    passphrase_required:
      'Cette sauvegarde est chiffrée : ouvrez-la d’abord avec sa phrase de passe.',
    wrong_passphrase: 'Cette phrase de passe n’ouvre pas la sauvegarde.',
    damaged_backup:
      'Cette sauvegarde chiffrée est endommagée : une partie a été modifiée ou manque. Rien n’a été restauré.',
    not_encrypted: 'Cette sauvegarde n’est pas chiffrée.',
    unlock_failed: 'La sauvegarde n’a pas pu être déchiffrée.',
    invalid_collection_read_hour:
      'Choisissez dans la liste l’heure de lecture des collections, ou Jamais.',
    invalid_collection_library_apps:
      'Choisissez dans la liste les applis qui présentent les bibliothèques de collections par collection.',
    invalid_remuxdb_url:
      'L’adresse de RemuxDB doit être une adresse web commençant par http:// ou https://.',
    invalid_update_tool_url:
      'L’adresse de l’outil de mise à jour doit être une adresse web commençant par http:// ou https://, sans nom d’utilisateur ni fragment.',
    invalid_update_tool_token:
      'Le jeton de l’outil de mise à jour ne contient pas d’espace, et 1024 caractères au plus.',
    invalid_provider:
      'Vérifiez le fournisseur d’identité : son émetteur est une adresse web commençant par http:// ou https://, sans requête ni fragment ; l’ID client, le secret et les claims ne contiennent pas d’espace ; un claim nomme l’utilisateur.',
    restart_unconfirmed:
      'Polyfin ne sait pas si quelque chose le relance : confirmez d’abord l’avertissement.',
    no_update: 'Polyfin Desktop ne propose pas cette mise à jour pour le moment.',
    not_desktop: 'Polyfin ne tourne pas dans Polyfin Desktop.',
    no_update_tool: 'Aucun outil de mise à jour n’est réglé dans Paramètres › Général.',
    update_tool_timeout:
      'L’outil de mise à jour n’a pas répondu en 50 secondes : il est peut-être encore en train de mettre à jour.',
    update_tool_unreachable: 'L’outil de mise à jour n’a pas pu être joint.',
    invalid_public_address:
      'L’adresse publique doit être une adresse web commençant par http:// ou https://, sans paramètres.',
    invalid_local_address:
      'L’adresse sur le réseau local doit être une adresse web commençant par http:// ou https://, comme http://192.168.1.10:8096, sans paramètres.',
    invalid_dlna_name: 'Le nom sur le réseau ne peut pas dépasser 256 octets.',
    invalid_dlna_user: 'Cet utilisateur n’existe plus. Choisissez-en un autre, ou « Personne ».',
    invalid_dlna_profile: 'Ce n’est pas un profil DLNA au format XML du plugin DLNA de Jellyfin.',
    invalid_smtp_host:
      'Saisissez le nom d’hôte du serveur SMTP, comme smtp.example.org, sans espaces.',
    invalid_smtp_port: 'Le port SMTP doit être un nombre entier de 1 à 65535.',
    invalid_smtp_security: 'Choisissez dans la liste le chiffrement de la connexion SMTP.',
    invalid_smtp_account:
      'L’utilisateur et le mot de passe SMTP font au plus 256 caractères, sur une ligne.',
    invalid_smtp_sender:
      'Saisissez l’adresse de l’expéditeur seule, comme polyfin@example.org, et un nom de 128 caractères au plus.',
    invalid_weekly_summary_day: 'Choisissez dans la liste le jour du résumé de la semaine.',
    invalid_weekly_summary_hour: 'Choisissez dans la liste l’heure du résumé de la semaine.',
    invalid_import_every_minutes:
      'Choisissez dans la liste tous les combien les comptes connectés sont importés.',
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
    invalid_reminder_settings: 'Choisissez 1, 5, 10, 15, 30 ou 60 minutes.',
    too_many_targets: 'Il y a déjà 20 cibles. Supprimez-en une pour en ajouter une autre.',
    target_unreadable:
      'L’adresse ou le jeton de cette cible ne peut pas être déchiffré avec POLYFIN_SECRET_KEY. Saisissez-le de nouveau.',
    invalid_source_name: 'Le nom d’une source doit faire de 1 à 64 caractères.',
    invalid_source_address:
      'Saisissez une adresse commençant par https:// ou http://, ou pour une playlist le chemin absolu d’un fichier du serveur, et pour un compte Xtream Codes un identifiant et un mot de passe.',
    file_missing:
      'Il n’y a pas de fichier à ce chemin sur le serveur. Avec Docker, donnez son chemin dans le conteneur, dans un volume monté.',
    file_unreadable:
      'Polyfin ne peut pas lire ce fichier : vérifiez que c’est un fichier, pas un dossier, et que Polyfin a le droit de le lire.',
    local_file_refused: 'Seuls les administrateurs peuvent utiliser un fichier du serveur.',
    invalid_user_agent:
      'Un agent utilisateur fait au plus 512 caractères, sans saut de ligne ni caractère de contrôle.',
    tuner_added: 'Ce tuner est déjà une source du serveur.',
    invalid_folder_name: 'Le nom d’un dossier doit faire de 1 à 64 caractères.',
    invalid_folder_path:
      'Saisissez le chemin complet du dossier là où tourne Polyfin, autre que la racine du système : dans son conteneur, commençant par / ; sous Windows, avec son lecteur, comme D:\\Films, ou son partage, comme \\\\nas\\media\\Films.',
    invalid_share_address:
      'Saisissez une adresse comme smb://serveur/partage/dossier, https://serveur/dossier, s3://bucket/préfixe, sftp://serveur/dossier ou rclone://serveur:port/dossier, sans utilisateur ni mot de passe : ils ont leurs propres champs.',
    invalid_share_endpoint:
      'Saisissez le point de terminaison sous forme d’adresse http:// ou https://, comme https://s3.example.org, et une région d’au plus 64 caractères sans espace.',
    invalid_share_key:
      'Collez une clé privée OpenSSH ou PEM entière, et la phrase de passe qui l’ouvre si elle en a une.',
    invalid_rclone_remote:
      'Saisissez le nom de l’un des remotes de rclone, sans deux-points ni barre oblique, d’au plus 128 caractères.',
    invalid_poll_minutes:
      'Saisissez un nombre de minutes de 0 à 1 440, 0 pour ne chercher les changements qu’aux analyses.',
    invalid_share_user:
      'Un utilisateur fait au plus 256 caractères, un mot de passe au plus 1 024.',
    invalid_folder_kind: 'Choisissez si le dossier contient des films ou des séries.',
    invalid_imdb_id: 'Un identifiant IMDb est tt suivi de chiffres, comme tt0063350.',
    invalid_episode:
      'Écrivez l’épisode comme le nom d’un fichier le donne, par exemple S01E05, ou S01E05-E06 pour deux.',
    title_search_failed:
      'La recherche dans les addons de métadonnées a échoué. Réessayez dans un instant.',
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
      'Une importation est déjà en cours. Attendez qu’elle se termine, ou arrêtez-la.',
    unknown_jellyfin_user:
      'Un utilisateur Jellyfin n’est plus sur ce serveur. Reconnectez-vous pour relire ses utilisateurs.',
    emby_user_key:
      'Cette clé est celle d’un utilisateur, et Emby ne dit pas à qui elle appartient. Créez une clé d’API dans les paramètres d’Emby, ou connectez-vous avec un compte utilisateur.',
    server_import_running:
      'Une autre importation est en cours sur ce serveur ou dans ce compte. Réessayez dans quelques minutes.',
    server_imports_disabled:
      'L’importation de votre historique depuis un autre serveur ou une appli est désactivée sur ce serveur.',
    letterboxd_export_invalid:
      'Ce fichier n’est pas un export Letterboxd : choisissez le fichier ZIP que donne Letterboxd, tel quel. Il contient watched.csv ou diary.csv.',
    letterboxd_export_too_large:
      'Ce fichier dépasse 20 Mo ou compte plus de 50 000 films : choisissez le fichier ZIP que donne Letterboxd.',
    invalid_nuvio_address:
      'Saisissez l’adresse du serveur Nuvio, par exemple https://api.nuvio.tv, ou laissez vide pour celui de Nuvio.',
    nuvio_sign_in_refused: 'Nuvio a refusé cet e-mail ou ce mot de passe.',
    nuvio_unreachable:
      'Le serveur Nuvio n’a pas répondu. Vérifiez son adresse, et réessayez plus tard.',
    not_nuvio:
      'Quelque chose a répondu à cette adresse, mais pas un serveur Nuvio. Vérifiez l’adresse, ou laissez vide pour celui de Nuvio.',
    nuvio_rate_limited:
      'Nuvio demande d’attendre avant de se reconnecter. Réessayez dans quelques minutes.',
    nuvio_backup_unsupported:
      'Nuvio a donné les données du compte sous une forme que cette version de Polyfin ne lit pas. Polyfin lit la version 1 de la sauvegarde de compte de Nuvio.',
    stremio_sign_in_refused: 'Stremio a refusé cet e-mail ou ce mot de passe.',
    stremio_pin_refused: 'Stremio a refusé ce code PIN.',
    stremio_key_refused:
      'Stremio a refusé cette clé d’authentification. Recopiez-la depuis l’appli web de Stremio, connectée à votre compte.',
    stremio_link_expired: 'Le code de liaison a expiré, ou il a déjà servi. Demandez-en un autre.',
    stremio_unreachable: 'Stremio n’a pas répondu. Réessayez plus tard.',
    stremio_unexpected:
      'Stremio a répondu d’une manière que cette version de Polyfin ne lit pas. Réessayez plus tard.',
    app_import_expired:
      'Ce qui a été lu du compte n’est plus gardé, après quelques minutes. Reconnectez-vous.',
    app_import_running: 'Une importation dans ce compte est en cours. Attendez qu’elle se termine.',
    plex_sign_in_ended:
      'La connexion à Plex a pris fin : elle a expiré, a été remplacée, ou a servi à une importation. Reconnectez-vous avec Plex.',
    plex_sign_in_pending:
      'Plex ne vous a pas encore connecté. Terminez la connexion sur la page de Plex.',
    plex_unknown_profile: 'Ce Plex Home n’a pas ce profil. Reconnectez-vous avec Plex.',
    plex_profile_pin_refused: 'Plex a refusé ce code PIN. Saisissez à nouveau celui du profil.',
    plex_profile_forbidden: 'Plex ne laisse pas ce compte passer à ce profil.',
    plex_unknown_server: 'Votre compte Plex n’a pas accès à ce serveur. Choisissez-en un autre.',
    plex_not_owner:
      'Ce compte Plex n’est pas propriétaire du serveur. Connectez-vous avec le compte Plex du propriétaire.',
    plex_server_unreachable:
      'Aucune des adresses que Plex donne pour ce serveur n’a répondu comme lui. Vérifiez qu’il fonctionne, et que Polyfin peut le joindre.',
    plex_tv_failed: 'plex.tv n’a pas répondu comme prévu. Réessayez plus tard.',
    plex_tv_unreachable: 'plex.tv est injoignable. Vérifiez la connexion de Polyfin à Internet.',
    app_not_connected: 'Cette appli n’est plus connectée au compte.',
    app_connection_stopped:
      'L’appli a refusé la connexion que Polyfin gardait. Reconnectez-vous, avec « Rester connecté » coché.',
    invalid_ends_on: 'Choisissez la date de fin du compte, ou « Jamais ».',
    administrator_cannot_end:
      'Le compte d’un administrateur ne prend jamais fin. Faites-en d’abord un membre pour lui donner une date de fin.',
    invalid_invite_account_end: 'Choisissez dans la liste quand les comptes prennent fin.',
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
    other: 'Autres vidéos',
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
