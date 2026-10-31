import type en from '../en/common'

const common: typeof en = {
  documentTitle: 'Polyfin-Verwaltung',
  header: {
    productName: 'Polyfin',
  },
  language: {
    label: 'Anzeigesprache',
    en: { short: 'EN', name: 'English' },
    fr: { short: 'FR', name: 'Français' },
    de: { short: 'DE', name: 'Deutsch' },
    es: { short: 'ES', name: 'Español' },
    it: { short: 'IT', name: 'Italiano' },
    'pt-BR': { short: 'PT', name: 'Português (Brasil)' },
  },
  common: {
    enterNumber: 'Gib eine Zahl ein.',
    loading: 'Wird geladen…',
    save: 'Speichern',
    saving: 'Wird gespeichert…',
    cancel: 'Abbrechen',
    retry: 'Erneut versuchen',
    never: 'Nie',
    colon: ':',
    passwordRule: 'Mindestens 8 Zeichen.',
    nameRule: "1 bis 64 Zeichen: Buchstaben, Ziffern, Leerzeichen und - _ ' . @ +",
    passwordMismatch: 'Die beiden Passwörter stimmen nicht überein.',
    moveUp: (name: string) => `${name} nach oben verschieben`,
    moveDown: (name: string) => `${name} nach unten verschieben`,
    moved: (name: string, position: number, count: number) =>
      `${name} an Position ${position} von ${count} verschoben.`,
  },
  errors: {
    generic: 'Etwas ist schiefgelaufen. Bitte versuche es erneut.',
    network: 'Der Server ist nicht erreichbar. Prüfe, ob Polyfin läuft.',
    unauthenticated: 'Deine Sitzung ist abgelaufen. Bitte melde dich erneut an.',
    forbidden: 'Du hast keine Berechtigung für diese Aktion.',
    cross_origin: 'Die Anfrage wurde abgelehnt, weil sie nicht von dieser Seite stammt.',
    invalid_setup_code:
      'Dieser Einrichtungscode ist ungültig. Kopiere ihn erneut aus dem Serverprotokoll.',
    invalid_name:
      "Namen müssen 1 bis 64 Zeichen lang sein: Buchstaben, Ziffern, Leerzeichen und - _ ' . @ +",
    invalid_password: 'Passwörter müssen mindestens 8 Zeichen lang sein.',
    setup_complete: 'Die Einrichtung ist bereits abgeschlossen. Melde dich stattdessen an.',
    too_many_attempts: 'Zu viele Versuche. Warte einige Minuten, bevor du es erneut versuchst.',
    invalid_credentials: 'Name oder Passwort ist falsch.',
    account_disabled: 'Dieses Konto ist deaktiviert. Bitte einen Administrator, es zu aktivieren.',
    wrong_password: 'Dein aktuelles Passwort ist falsch.',
    invalid_code:
      'Dieser Code ist ungültig. Gib den 6-stelligen Code ein, den deine App gerade anzeigt, oder einen deiner Wiederherstellungscodes.',
    two_step_expired:
      'Die Anmeldung hat zu lange gedauert. Gib deinen Namen und dein Passwort erneut ein.',
    two_step_on: 'Die zweistufige Anmeldung ist bereits aktiviert.',
    two_step_off: 'Die zweistufige Anmeldung ist für dieses Konto deaktiviert.',
    two_step_not_started:
      'Beginne von vorn: Aktiviere die zweistufige Anmeldung, um einen neuen Schlüssel zu erhalten.',
    two_step_required:
      'Administratoren müssen auf diesem Server die zweistufige Anmeldung verwenden: Sie kann nicht deaktiviert werden.',
    two_step_own: 'Deaktiviere deine eigene zweistufige Anmeldung unter „Mein Konto“.',
    not_found: 'Dieses Element existiert nicht mehr. Die Liste wurde aktualisiert.',
    not_set:
      'Nichts gespeichert: Der Wert wurde entfernt oder kann mit POLYFIN_SECRET_KEY nicht entschlüsselt werden. Lade die Seite neu.',
    not_revealable: 'Diese Verbindung enthält keinen Schlüssel, der angezeigt werden kann.',
    unknown_code:
      'Kein Gerät wartet mit diesem Code. Er ist möglicherweise abgelaufen oder wurde falsch eingegeben.',
    quick_connect_disabled: 'Quick Connect ist auf diesem Server deaktiviert.',
    name_taken: 'Dieser Name wird bereits von einem anderen Benutzer verwendet.',
    invalid_invite_uses:
      'Die Anzahl der Konten, die ein Einladungslink erstellen darf, muss eine ganze Zahl von 1 bis 100 sein.',
    invalid_invite_expiry: 'Wähle aus der Liste, wann der Einladungslink abläuft.',
    invalid_invite_model: 'Dieser Benutzer existiert nicht mehr. Wähle einen anderen.',
    invite_unknown: 'Dieser Einladungslink funktioniert nicht.',
    invite_used_up: 'Dieser Einladungslink ist aufgebraucht.',
    invite_expired: 'Dieser Einladungslink ist abgelaufen.',
    invite_revoked: 'Dieser Einladungslink wurde widerrufen.',
    share_links_off:
      'Du kannst keine Freigabelinks erstellen: Ein Administrator hat sie für diesen Server oder für dein Konto deaktiviert.',
    not_shareable: 'Nur ein Film oder eine Folge, die du sehen kannst, kann freigegeben werden.',
    invalid_share_expiry: 'Wähle aus der Liste, wann der Link abläuft.',
    invalid_share_viewers:
      'Die Anzahl gleichzeitiger Zuschauer muss eine ganze Zahl von 1 bis 5 sein.',
    last_administrator:
      'Dies ist der letzte aktive Administrator: Er kann nicht gelöscht, herabgestuft oder deaktiviert werden.',
    admin_cannot_join_group:
      'Administratoren können keiner Gruppe beitreten, da ihre Berechtigungen abweichen. Lade die Seite neu, um zu sehen, wer jetzt Administrator ist.',
    invalid_group: 'Diese Gruppe existiert nicht mehr. Lade die Seite neu und wähle eine andere.',
    follows_group:
      'Dieser Benutzer folgt seiner Gruppe: Ändere die Gruppe oder deaktiviere zuerst „Der Gruppe folgen“.',
    invalid_copy_from: 'Dieser Benutzer existiert nicht mehr. Wähle einen anderen.',
    invalid_server_name: 'Der Servername muss 1 bis 64 Zeichen lang sein.',
    invalid_language: 'Wähle eine Sprache aus der Liste.',
    invalid_audio_region: 'Wähle eine Region aus der Liste.',
    invalid_catalog_limit:
      'Die pro Katalog gelesenen Titel müssen eine ganze Zahl von 100 bis 20.000 sein.',
    invalid_channel_limit:
      'Die pro Live-TV-Katalog gelesenen Kanäle müssen eine ganze Zahl von 100 bis 50.000 sein.',
    invalid_played_percent:
      'Der Schwellenwert für „Gesehen“ muss eine ganze Zahl von 50 bis 100 sein.',
    invalid_resume_percent:
      'Der Schwellenwert für „Fortsetzen“ muss eine ganze Zahl von 0 bis 50 sein.',
    resume_not_below_played:
      'Der Schwellenwert für „Fortsetzen“ muss niedriger sein als der für „Gesehen“.',
    invalid_version_list_minutes:
      'Die Aufbewahrungsdauer der Versionslisten muss eine ganze Zahl von 1 bis 360 Minuten sein.',
    invalid_catalog_refresh_minutes:
      'Das Aktualisierungsintervall der Kataloge muss eine ganze Zahl von 1 bis 1.440 Minuten sein.',
    invalid_analysis_timeout:
      'Die Zeit zum Analysieren einer Version muss eine ganze Zahl von 5 bis 120 Sekunden sein.',
    invalid_version_attempts:
      'Die Anzahl der versuchten Versionen muss eine ganze Zahl von 1 bis 10 sein.',
    invalid_version_order:
      'Wähle höchstens vier Kriterien zum Sortieren der Versionen, je eines pro Art. Lade die Seite neu.',
    invalid_version_codec_order:
      'Die Reihenfolge der Videocodecs muss jeden Codec genau einmal enthalten. Lade die Seite neu.',
    invalid_version_source_order:
      'Die Reihenfolge der Quellen muss Quellen des Servers enthalten, jede genau einmal. Lade die Seite neu.',
    invalid_hide_versions_larger:
      'Die Größe, ab der Versionen ausgeblendet werden, muss 0 oder eine ganze Zahl von 1 bis 1.000 GB sein.',
    invalid_hidden_version_codecs:
      'Diese Liste ausgeblendeter Codecs wird nicht unterstützt. Lade die Seite neu.',
    invalid_hidden_version_words:
      'Gib höchstens 50 Wörter zum Ausblenden von Versionen ein, eines pro Zeile, jedes mit höchstens 100 Zeichen.',
    invalid_hidden_releases:
      'Diese Liste ausgeblendeter Release-Arten wird nicht unterstützt. Lade die Seite neu.',
    invalid_max_conversions:
      'Die Anzahl gleichzeitiger Videokonvertierungen muss eine ganze Zahl von 0 bis 32 sein.',
    invalid_max_conversion_height:
      'Wähle die maximale Qualität konvertierter Videos aus der Liste.',
    invalid_subtitle_ocr: 'Wähle aus der Liste, wann Bilduntertitel in Text umgewandelt werden.',
    invalid_subtitle_ocr_tracks:
      'Die Anzahl gleichzeitig gelesener Bilduntertitelspuren muss eine ganze Zahl von 1 bis 4 sein.',
    invalid_default_language: 'Wähle eine Standardsprache aus der Liste.',
    invalid_encoder_preset: 'Wähle eine Kodierungsgeschwindigkeit aus der Liste.',
    invalid_video_quality: 'Die Videoqualität muss 0 oder eine ganze Zahl von 1 bis 51 sein.',
    invalid_hardware_acceleration: 'Wähle eine Grafikkarte aus der Liste.',
    invalid_hardware_decoding_codecs:
      'Diese Liste der auf der Grafikkarte dekodierten Formate wird nicht unterstützt. Lade die Seite neu.',
    invalid_adaptive_quality: 'Wähle aus der Liste, wann mehrere Qualitäten angeboten werden.',
    invalid_tone_mapping_algorithm: 'Wähle eine Tone-Mapping-Methode aus der Liste.',
    invalid_tone_mapping_peak:
      'Die Spitzenhelligkeit muss 0 oder eine ganze Zahl von 100 bis 10.000 Nits sein.',
    invalid_tone_mapping_desat:
      'Die Entsättigung heller Bereiche muss eine Zahl von 0 bis 10 sein.',
    invalid_processor_tone_mapping_height:
      'Wähle die maximale Qualität für die HDR-Konvertierung durch den Prozessor aus der Liste.',
    invalid_deinterlace_method: 'Wähle eine Deinterlacing-Methode aus der Liste.',
    invalid_downmix_algorithm: 'Wähle eine Stereo-Abmischung aus der Liste.',
    invalid_downmix_boost:
      'Die Lautstärke beim Abmischen auf Stereo muss eine Zahl von 0,5 bis 3 sein.',
    invalid_max_audio_channels: 'Wähle die maximale Anzahl an Audiokanälen aus der Liste.',
    invalid_audio_bitrate_per_channel:
      'Die Audio-Bitrate pro Kanal muss 0 oder eine ganze Zahl in kb/s von 32 bis 320 sein.',
    invalid_encoding_threads: 'Die Prozessor-Threads müssen eine ganze Zahl von 0 bis 64 sein.',
    invalid_ahead_seconds:
      'Die im Voraus vorbereiteten Sekunden müssen eine ganze Zahl von 30 bis 600 sein.',
    parental_control:
      'Für dieses Konto gilt die Kindersicherung: Es behält die Add-ons des Servers, die die Altersfreigaben liefern, auf denen sie beruht.',
    invalid_parental_control:
      'Diese Einstellung der Kindersicherung wird nicht unterstützt. Lade die Seite neu.',
    invalid_max_playbacks:
      'Die gleichzeitigen Wiedergaben müssen eine ganze Zahl von 0 bis 20 sein.',
    invalid_max_bitrate: 'Wähle die maximale Qualität aus der Liste.',
    invalid_sync_play: 'Wähle eine Option für „Gemeinsam schauen“ aus der Liste.',
    invalid_manifest_url:
      'Gib die Manifest-Adresse eines Add-ons ein, die mit https://, http:// oder stremio:// beginnt und auf /manifest.json endet.',
    addon_exists: 'Dieses Add-on ist hier bereits installiert.',
    addon_unreachable:
      'Das Add-on konnte nicht erreicht werden. Prüfe die Adresse und ob das Add-on online ist, und versuche es dann erneut.',
    invalid_manifest: 'Diese Adresse hat kein Stremio-Add-on-Manifest zurückgegeben.',
    private_network:
      'Dieses Add-on liegt unter einer Adresse im lokalen Netzwerk. Nur Administratoren können solche Add-ons installieren.',
    invalid_order:
      'Die Add-on-Liste hat sich inzwischen geändert. Sie wurde aktualisiert: Versuche es erneut.',
    invalid_library:
      'Eine Bibliothek verweist auf einen Katalog, der nicht mehr existiert oder nicht durchsucht werden kann. Entferne die als nicht mehr verfügbar markierten Bibliotheken und speichere dann erneut.',
    invalid_library_name: 'Bibliotheksnamen müssen 1 bis 64 Zeichen lang sein.',
    invalid_library_genre:
      'Das Genre einer Bibliothek wird von ihrem Katalog nicht angeboten. Wähle ein anderes aus der Liste und speichere dann erneut.',
    invalid_library_max_items:
      'Die maximale Anzahl an Titeln einer Bibliothek muss eine ganze Zahl von 1 bis 20.000 sein oder leer bleiben.',
    invalid_smart_list_name: 'Der Name einer intelligenten Liste muss 1 bis 64 Zeichen lang sein.',
    invalid_smart_list_shown_as:
      'Wähle, ob die Liste als Bibliothek oder als Sammlung angezeigt wird.',
    invalid_smart_list_catalogs: 'Wähle 1 bis 50 Kataloge, aus denen die Liste ihre Titel bezieht.',
    invalid_smart_list_rules:
      'Eine Regel ist unvollständig: Gib jeder Regel ihren Wert und jedem Jahr bzw. jeder Bewertung mindestens eine Grenze.',
    invalid_smart_list_sort: 'Wähle, wie die Liste sortiert wird.',
    invalid_smart_list_max_items:
      'Die maximale Anzahl an Titeln einer intelligenten Liste muss eine ganze Zahl von 1 bis 20.000 sein oder leer bleiben.',
    too_many_smart_lists:
      'Hier gibt es bereits 100 intelligente Listen. Lösche eine, um eine neue zu erstellen.',
    invalid_image: 'Wähle ein JPEG-, PNG- oder WebP-Bild mit höchstens 10 MB.',
    invalid_image_url: 'Gib eine Bildadresse ein, die mit https:// oder http:// beginnt.',
    image_unreachable:
      'Von dieser Adresse konnte kein Bild heruntergeladen werden. Prüfe, ob sie ein Bild öffnet, und versuche es dann erneut.',
    image_private_network:
      'Dieses Bild liegt unter einer Adresse im lokalen Netzwerk. Nur Administratoren können solche Adressen verwenden.',
    invalid_login_attempts:
      'Die Anzahl falscher Passwörter, bevor ein Konto gesperrt wird, muss 0 oder eine ganze Zahl von 3 bis 20 sein.',
    invalid_inactive_device_days:
      'Die Tage, bevor ungenutzte Geräte abgemeldet werden, müssen eine ganze Zahl von 0 bis 365 sein.',
    invalid_https_mode:
      'HTTPS bereitstellen muss „Aus“, „Zertifikatsdateien“ oder „Let’s Encrypt“ sein.',
    invalid_https_port: 'Der HTTPS-Port muss eine ganze Zahl von 1 bis 65535 sein.',
    invalid_https_certificate:
      'Das Zertifikat muss ein absoluter Pfad im Container sein, z. B. /certs/fullchain.pem.',
    invalid_https_private_key:
      'Der private Schlüssel muss ein absoluter Pfad im Container sein, z. B. /certs/privkey.pem.',
    https_certificate_missing: 'Unter diesem Pfad im Container gibt es keine Zertifikatsdatei.',
    https_private_key_missing:
      'Unter diesem Pfad im Container gibt es keine Datei mit einem privaten Schlüssel.',
    https_certificate_unreadable:
      'Polyfin kann diese Zertifikatsdatei nicht lesen, oder sie enthält kein PEM-Zertifikat. Prüfe, ob Benutzer 65532 sie lesen darf.',
    https_private_key_unreadable:
      'Polyfin kann diese Schlüsseldatei nicht lesen, oder sie enthält keinen RSA- oder ECDSA-Schlüssel im PEM-Format. Prüfe, ob Benutzer 65532 sie lesen darf.',
    https_files_mismatch: 'Der private Schlüssel gehört nicht zum Zertifikat.',
    invalid_acme_hosts:
      'Hostnamen müssen öffentliche Namen wie media.example.org sein, einer pro Zeile, ohne Platzhalter oder IP-Adressen, höchstens 100 und mindestens einer für Let’s Encrypt.',
    invalid_acme_email: 'Die E-Mail muss eine E-Mail-Adresse sein oder leer bleiben.',
    acme_terms_required: 'Stimme den Nutzungsbedingungen von Let’s Encrypt zu, um es zu verwenden.',
    invalid_acme_challenge: 'Die Challenge muss HTTP-01 oder TLS-ALPN-01 sein.',
    invalid_known_proxies:
      'Bekannte Proxys müssen IP-Adressen, Netzwerke wie 172.18.0.0/16 oder Hostnamen wie traefik sein, einer pro Zeile, höchstens 64.',
    personal_addons_disabled:
      'Deine eigenen Add-ons wurden von einem Administrator deaktiviert. Sie bleiben erhalten, aber du kannst keine hinzufügen, aktualisieren oder aktivieren.',
    invalid_hidden_libraries:
      'Die Bibliotheksliste hat sich inzwischen geändert. Lade die Seite neu.',
    invalid_blocked_genres:
      'Genres müssen 1 bis 100 Zeichen lang sein, und höchstens 100 können gesperrt werden.',
    invalid_access_schedules:
      'Uhrzeiten reichen von 00:00 bis 24:00, und jedes Ende muss nach seinem Beginn liegen (höchstens 50 Zeilen).',
    outside_allowed_hours:
      'Dieses Konto kann zu dieser Zeit nicht verwendet werden. Versuche es während der erlaubten Zeiten erneut.',
    outside_local_network:
      'Dieses Konto kann nur im lokalen Netzwerk verwendet werden. Verbinde dich von dort oder frage einen Administrator.',
    remote_access_self:
      'Du bist von außerhalb des lokalen Netzwerks verbunden: Wenn du deine eigenen Verbindungen von außerhalb ausschaltest, sperrst du dich aus. Tu es aus dem lokalen Netzwerk.',
    invalid_guide_url:
      'Gib eine Adresse für das Fernsehprogramm ein, die mit https:// oder http:// beginnt und höchstens 4.096 Zeichen lang ist, oder den absoluten Pfad einer Datei auf dem Server.',
    invalid_app: 'Der App-Name muss 1 bis 64 Zeichen lang sein, ohne Sonderzeichen.',
    invalid_trickplay_interval:
      'Die Zeit zwischen zwei Vorschaubildern muss eine ganze Zahl von 5 bis 60 Sekunden sein.',
    invalid_trickplay_width: 'Wähle die Breite der Vorschaubilder aus der Liste.',
    invalid_thumbnail_storage_gb:
      'Der Speicherplatz für Bilder muss eine ganze Zahl von 1 bis 50 GB sein.',
    invalid_recording_padding:
      'Die Minuten vor und nach einer Aufnahme müssen ganze Zahlen von 0 bis 60 sein.',
    invalid_recording_retention_days:
      'Die Aufbewahrungsdauer für Aufnahmen muss eine ganze Zahl von 0 bis 3.650 Tagen sein.',
    invalid_convert_recordings_hours:
      'Wähle aus den Listen zwei verschiedene Uhrzeiten, zwischen denen Aufnahmen konvertiert werden.',
    invalid_quality_group: 'Wähle die Qualitätsgruppe aus der Liste.',
    invalid_live_tv_refresh_hours:
      'Das Aktualisierungsintervall muss eine ganze Zahl von 1 bis 168 Stunden sein.',
    invalid_local_scan_hours: 'Das Scan-Intervall muss eine ganze Zahl von 0 bis 168 Stunden sein.',
    invalid_watchlist_check_hours:
      'Das Prüfintervall der Merklisten muss eine ganze Zahl von 0 bis 168 Stunden sein.',
    invalid_custom_css: 'Das benutzerdefinierte CSS darf höchstens 2 MB groß sein.',
    invalid_custom_js: 'Das benutzerdefinierte JavaScript darf höchstens 2 MB groß sein.',
    invalid_web_theme: 'Wähle ein Theme aus der Liste.',
    web_theme_locked: 'Dein Administrator hat das Theme deines Web-Players gewählt.',
    theme_copy_running:
      'Eine Kopie des Themes wird bereits angelegt. Versuche es gleich noch einmal.',
    theme_copy_nothing: 'Die gespeicherten benutzerdefinierten Texte laden nichts von anderswo.',
    theme_copy_failed: 'Keine Datei konnte gespeichert werden, daher wurde keine Kopie angelegt.',
    custom_texts_changed:
      'Die benutzerdefinierten Texte haben sich während der Kopie geändert. Versuche es erneut.',
    invalid_login_disclaimer: 'Die Anmeldenachricht darf höchstens 8 KB groß sein.',
    invalid_publicmetadb_key:
      'PublicMetaDB hat diesen Schlüssel nicht akzeptiert. Kopiere ihn erneut aus deinem PublicMetaDB-Konto. Es wurde nichts gespeichert.',
    publicmetadb_unreachable:
      'PublicMetaDB war zur Prüfung des Schlüssels nicht erreichbar. Es wurde nichts gespeichert: Versuche es später erneut.',
    invalid_theintrodb_key:
      'TheIntroDB hat diesen Schlüssel nicht akzeptiert. Kopiere ihn erneut aus deinem TheIntroDB-Konto. Es wurde nichts gespeichert.',
    theintrodb_unreachable:
      'TheIntroDB war zur Prüfung des Schlüssels nicht erreichbar oder begrenzt derzeit die Anfragen. Es wurde nichts gespeichert: Versuche es später erneut.',
    invalid_mdblist_ratings_key:
      'MDBList hat diesen Schlüssel nicht akzeptiert. Kopiere ihn erneut aus deinem MDBList-Konto. Es wurde nichts gespeichert.',
    mdblist_unreachable:
      'MDBList war zur Prüfung des Schlüssels nicht erreichbar. Es wurde nichts gespeichert: Versuche es später erneut.',
    invalid_tmdb_key:
      'TMDB hat diesen Schlüssel nicht akzeptiert. Kopiere den API-Schlüssel oder das Lesezugriffstoken erneut aus deinem TMDB-Konto. Es wurde nichts gespeichert.',
    tmdb_unreachable:
      'TMDB war zur Prüfung des Schlüssels nicht erreichbar. Es wurde nichts gespeichert: Versuche es später erneut.',
    invalid_critic_rating_shows: 'Wähle aus der Liste, welchen Wert die Kritikerbewertung anzeigt.',
    invalid_measure_loudness:
      'Wähle aus der Liste, bei welchen Songs die Lautstärke gemessen wird.',
    invalid_segment_order:
      'Die Reihenfolge der Quellen für Überspringmarker muss jede Quelle genau einmal enthalten. Lade die Seite neu.',
    invalid_trakt_app:
      'Client-ID und Secret von Trakt dürfen höchstens 256 Zeichen lang sein, ohne Leerzeichen oder Sonderzeichen. Kopiere sie erneut aus deiner Trakt-App.',
    invalid_simkl_app:
      'Die Client-ID von Simkl darf höchstens 256 Zeichen lang sein, ohne Leerzeichen oder Sonderzeichen. Kopiere sie erneut aus deiner Simkl-App.',
    invalid_backup_hour: 'Wähle die Uhrzeit der Backups aus der Liste.',
    invalid_backups_kept:
      'Die Anzahl der aufbewahrten Backups muss eine ganze Zahl von 1 bis 90 sein.',
    invalid_torrent_route:
      'Torrent-Streams brauchen genau eine Route: einen Netzwerkschnittstellennamen (nur Linux), einen SOCKS5-Proxy als host:port oder eine WireGuard-Konfiguration.',
    invalid_wireguard_config: 'Die WireGuard-Konfiguration ist ungültig.',
    invalid_torrent_limit: 'Die Limits müssen ganze Zahlen von 0 bis 10.000 Mbit/s sein.',
    invalid_torrent_trackers:
      'Gib höchstens 30 Tracker ein, jeder eine HTTP-, HTTPS- oder UDP-Announce-URL, UDP mit Port, mit höchstens 500 Zeichen.',
    invalid_use_source_proxy:
      'Der Quell-Proxy kann nur für ein Stremio-Add-on des Servers gewählt werden.',
    invalid_source_proxy:
      'Der Proxy muss eine SOCKS5- oder HTTP-URL mit Host und Port sein, z. B. socks5h://warp:1080.',
    invalid_cache_size: 'Der Speicherplatz muss eine ganze Zahl von 1 bis 2.000 GB sein.',
    invalid_vaapi_device: 'Wähle die Grafikkarte aus der Liste.',
    invalid_recordings_folder:
      'Der Aufnahmeordner muss ein absoluter Pfad zu einem Ordner sein, in den Polyfin schreiben kann.',
    invalid_backup_folder:
      'Der Backup-Ordner muss ein absoluter Pfad zu einem Ordner sein, in den Polyfin schreiben kann.',
    invalid_conversion_folder:
      'Der Konvertierungsordner muss ein absoluter Pfad zu einem Ordner sein, in den Polyfin schreiben kann.',
    invalid_reading_folder:
      'Der Ordner für gelesene Dateien muss ein absoluter Pfad zu einem Ordner sein, in den Polyfin schreiben kann.',
    invalid_cache_folder:
      'Der Cache-Ordner muss ein absoluter Pfad zu einem Ordner sein, in den Polyfin schreiben kann.',
    too_many_destinations: 'Es gibt bereits 10 Backup-Ziele: Lösche zuerst eines.',
    too_many_home_rows: 'Es gibt bereits 20 Zeilen auf der Startseite: Lösche zuerst eine.',
    invalid_home_row:
      'Gib der Zeile einen Namen mit höchstens 100 Zeichen und wähle ihren Katalog oder ihre Sammlung.',
    invalid_destination_kind: 'Wähle die Art des Ziels aus der Liste.',
    invalid_destination_address:
      'Gib die Adresse wie verlangt ein: eine http://- oder https://-Adresse ohne Pfad für S3-Speicher, einen Hostnamen oder eine IP-Adresse für einen SFTP-Server, die Adresse eines Ordners für einen WebDAV-Server.',
    invalid_bucket:
      'Gib den Namen des Buckets ein: 3 bis 63 Kleinbuchstaben, Ziffern, Punkte, Bindestriche oder Unterstriche.',
    invalid_destination_folder:
      'Der Ordner darf nicht mit „..“ nach oben wechseln, keine Steuerzeichen enthalten und höchstens 512 Byte lang sein.',
    invalid_region:
      'Die Region besteht aus Kleinbuchstaben, Ziffern und Bindestrichen, etwa eu-west-3.',
    invalid_port: 'Der Port ist eine ganze Zahl von 1 bis 65535.',
    invalid_destination_user: 'Gib den Benutzer ein oder für S3-Speicher den Zugriffsschlüssel.',
    invalid_destination_secret:
      'Gib das Passwort ein, für S3-Speicher den geheimen Schlüssel oder für einen SFTP-Server einen privaten Schlüssel.',
    invalid_private_key:
      'Der private Schlüssel muss ein OpenSSH- oder PEM-Schlüssel ohne Passphrase sein.',
    backups_off: 'Backups sind ausgeschaltet: Schalte sie zuerst oben ein.',
    pg_restore_missing:
      'pg_restore wurde auf dem Server nicht gefunden: Installiere die Client-Tools von PostgreSQL 18, die im Docker-Image enthalten sind.',
    backup_not_found: 'Dieses Backup ist nicht mehr vorhanden: Wähle ein anderes.',
    not_a_backup: 'Diese Datei ist kein Backup der Polyfin-Datenbank.',
    newer_backup:
      'Dieses Backup wurde von einer neueren Polyfin-Version erstellt: Aktualisiere zuerst Polyfin.',
    changed_backup:
      'Dieses Backup wurde von einer Polyfin-Version erstellt, deren Datenbankmigrationen sich von denen dieser Version unterscheiden.',
    restore_expired: 'Das gewählte Backup ist nicht mehr bereit: Wähle es erneut.',
    unreadable_secrets:
      'Dieses Backup enthält Schlüssel, die POLYFIN_SECRET_KEY nicht entschlüsseln kann: Bestätige mit „Trotzdem wiederherstellen“.',
    restoring: 'Eine Wiederherstellung läuft bereits.',
    wrong_server_name: 'Gib den Namen des Servers genau wie angezeigt ein.',
    too_large: 'Die Datei ist zu groß.',
    destination_failed: 'Das Ziel konnte nicht gelesen werden.',
    restore_failed:
      'Die Wiederherstellung konnte nicht gestartet werden: Es wurde nichts geändert.',
    invalid_backup_passphrase:
      'Die Backup-Passphrase ist mindestens 12 Zeichen lang, steht in einer Zeile und hat höchstens 256 Byte.',
    passphrase_required: 'Dieses Backup ist verschlüsselt: Öffne es zuerst mit seiner Passphrase.',
    wrong_passphrase: 'Diese Passphrase öffnet das Backup nicht.',
    damaged_backup:
      'Dieses verschlüsselte Backup ist beschädigt: Ein Teil davon wurde verändert oder fehlt. Es wurde nichts wiederhergestellt.',
    not_encrypted: 'Dieses Backup ist nicht verschlüsselt.',
    unlock_failed: 'Das Backup konnte nicht entschlüsselt werden.',
    invalid_collection_read_hour:
      'Wähle aus der Liste die Uhrzeit, zu der Sammlungen eingelesen werden, oder „Nie“.',
    invalid_collection_library_apps:
      'Wähle aus der Liste die Apps, die Sammlungsbibliotheken nach Sammlung anzeigen.',
    invalid_remuxdb_url:
      'Die RemuxDB-Adresse muss eine Webadresse sein, die mit http:// oder https:// beginnt.',
    invalid_update_tool_url:
      'Die Adresse des Update-Werkzeugs muss eine Webadresse sein, die mit http:// oder https:// beginnt, ohne Benutzername und Fragment.',
    invalid_update_tool_token:
      'Das Token des Update-Werkzeugs enthält keine Leerzeichen und höchstens 1024 Zeichen.',
    invalid_provider:
      'Prüfe den Identitätsanbieter: Sein Aussteller ist eine Webadresse, die mit http:// oder https:// beginnt, ohne Abfrage und Fragment; Client-ID, Geheimnis und Claims enthalten keine Leerzeichen; ein Claim nennt den Benutzer.',
    restart_unconfirmed:
      'Polyfin weiß nicht, ob es wieder gestartet wird: Bestätige zuerst die Warnung.',
    no_update: 'Polyfin Desktop bietet dieses Update gerade nicht an.',
    not_desktop: 'Polyfin läuft nicht in Polyfin Desktop.',
    no_update_tool: 'In den allgemeinen Einstellungen ist kein Update-Werkzeug eingetragen.',
    update_tool_timeout:
      'Das Update-Werkzeug hat nicht innerhalb von 50 Sekunden geantwortet: Vielleicht aktualisiert es noch.',
    update_tool_unreachable: 'Das Update-Werkzeug war nicht erreichbar.',
    invalid_public_address:
      'Die öffentliche Adresse muss eine Webadresse sein, die mit http:// oder https:// beginnt, ohne Abfrageparameter.',
    invalid_local_address:
      'Die Adresse im lokalen Netzwerk muss eine Webadresse sein, die mit http:// oder https:// beginnt, etwa http://192.168.1.10:8096, ohne Abfrageparameter.',
    invalid_dlna_name: 'Der Name im Netzwerk darf höchstens 256 Byte lang sein.',
    invalid_dlna_user: 'Dieser Benutzer existiert nicht mehr. Wähle einen anderen oder „Niemand“.',
    invalid_dlna_profile: 'Dies ist kein DLNA-Profil im XML-Format des DLNA-Plugins von Jellyfin.',
    invalid_smtp_host:
      'Gib den Hostnamen des SMTP-Servers ein, etwa smtp.example.org, ohne Leerzeichen.',
    invalid_smtp_port: 'Der SMTP-Port muss eine ganze Zahl von 1 bis 65535 sein.',
    invalid_smtp_security: 'Wähle aus der Liste, wie die SMTP-Verbindung verschlüsselt wird.',
    invalid_smtp_account:
      'SMTP-Benutzer und -Passwort sind höchstens 256 Zeichen lang, in einer Zeile.',
    invalid_smtp_sender:
      'Gib nur die Absenderadresse ein, etwa polyfin@example.org, und einen Namen mit höchstens 128 Zeichen.',
    invalid_weekly_summary_day: 'Wähle den Tag der Wochenzusammenfassung aus der Liste.',
    invalid_weekly_summary_hour: 'Wähle die Uhrzeit der Wochenzusammenfassung aus der Liste.',
    invalid_import_every_minutes:
      'Wähle aus der Liste, wie oft verbundene Konten importiert werden.',
    invalid_target_kind: 'Wähle eine Zielart aus der Liste.',
    invalid_target_name: 'Zielnamen müssen 1 bis 64 Zeichen lang sein.',
    invalid_target_address: 'Gib eine Webadresse ein, die mit https:// oder http:// beginnt.',
    private_target_address:
      'Diese Adresse liegt in einem lokalen Netzwerk: Nur Ziele eines Administrators dürfen sie erreichen.',
    invalid_email_address: 'Gib eine E-Mail-Adresse ein, etwa sam@example.org.',
    email_unavailable:
      'E-Mail benötigt einen SMTP-Server: Ein Administrator richtet ihn unter Einstellungen › Benachrichtigungen ein.',
    invalid_topic:
      'Ein ntfy-Topic besteht aus 1 bis 64 Buchstaben, Ziffern, Bindestrichen und Unterstrichen.',
    invalid_chat:
      'Gib die Nummer des Chats ein, etwa -1001234567890, oder den Namen eines öffentlichen Kanals, etwa @news_example.',
    invalid_token:
      'Gib das Token ein: höchstens 256 Zeichen, ohne Leerzeichen. Das eines Telegram-Bots sieht aus wie 123456:ABC-DEF.',
    invalid_user_key: 'Ein Pushover-Benutzerschlüssel besteht nur aus Buchstaben und Ziffern.',
    invalid_events: 'Eines der gewählten Ereignisse ist für dieses Ziel nicht verfügbar.',
    invalid_reminder_settings: 'Wähle 1, 5, 10, 15, 30 oder 60 Minuten.',
    too_many_targets: 'Es gibt bereits 20 Ziele. Lösche eines, um ein weiteres hinzuzufügen.',
    target_unreadable:
      'Adresse oder Token dieses Ziels kann mit POLYFIN_SECRET_KEY nicht entschlüsselt werden. Gib es erneut ein.',
    invalid_source_name: 'Quellennamen müssen 1 bis 64 Zeichen lang sein.',
    invalid_source_address:
      'Gib eine Adresse ein, die mit https:// oder http:// beginnt, oder für eine Wiedergabeliste den absoluten Pfad einer Datei auf dem Server, und für ein Xtream-Codes-Konto einen Benutzernamen und ein Passwort.',
    file_missing:
      'Unter diesem Pfad gibt es auf dem Server keine Datei. Gib mit Docker ihren Pfad im Container an, in einem eingebundenen Volume.',
    file_unreadable:
      'Polyfin kann diese Datei nicht lesen: Prüfe, dass es eine Datei und kein Ordner ist und dass Polyfin sie lesen darf.',
    local_file_refused: 'Nur Administratoren können eine Datei des Servers verwenden.',
    invalid_user_agent:
      'Ein User-Agent hat höchstens 512 Zeichen, ohne Zeilenumbrüche oder Steuerzeichen.',
    tuner_added: 'Dieser Tuner ist bereits eine Quelle des Servers.',
    invalid_folder_name: 'Ordnernamen müssen 1 bis 64 Zeichen lang sein.',
    invalid_folder_path:
      'Gib den vollständigen Pfad des Ordners dort ein, wo Polyfin läuft, außer dem Stammverzeichnis des Systems: in seinem Container beginnend mit /; unter Windows mit Laufwerk, etwa D:\\Filme, oder Freigabe, etwa \\\\nas\\media\\Filme.',
    invalid_share_address:
      'Gib eine Adresse ein wie smb://server/freigabe/ordner, https://server/ordner, s3://bucket/praefix, sftp://server/ordner oder rclone://server:port/ordner, ohne Benutzer oder Passwort darin: Dafür gibt es eigene Felder.',
    invalid_share_user: 'Benutzer sind höchstens 256 Zeichen lang, Passwörter höchstens 1.024.',
    invalid_share_endpoint:
      'Gib den Endpunkt als http://- oder https://-Adresse ein, etwa https://s3.example.org, und eine Region mit höchstens 64 Zeichen ohne Leerzeichen.',
    invalid_share_key:
      'Füge einen vollständigen privaten OpenSSH- oder PEM-Schlüssel ein und, falls vorhanden, die Passphrase, die ihn öffnet.',
    invalid_rclone_remote:
      'Gib den Namen eines rclone-Remotes ein, ohne Doppelpunkt oder Schrägstrich, mit höchstens 128 Zeichen.',
    invalid_poll_minutes:
      'Gib eine Anzahl Minuten von 0 bis 1.440 ein, 0, um nur bei Scans nach Änderungen zu suchen.',
    invalid_folder_kind: 'Wähle, ob der Ordner Filme oder Serien enthält.',
    invalid_imdb_id: 'Eine IMDb-Kennung besteht aus tt gefolgt von Ziffern, etwa tt0063350.',
    invalid_episode:
      'Schreib die Folge so, wie ein Dateiname sie angibt, etwa S01E05, oder S01E05-E06 für zwei.',
    title_search_failed:
      'Die Metadaten-Add-ons konnten nicht durchsucht werden. Versuche es gleich noch einmal.',
    invalid_channel_list:
      'Diese Adresse hat keine Kanalliste geliefert. Prüfe sie und ob das Konto noch gültig ist.',
    channel_list_too_large: 'Diese Kanalliste ist zu groß (höchstens 100 MB oder 100.000 Kanäle).',
    iptv_login_refused: 'Der IPTV-Server hat diesen Benutzernamen oder dieses Passwort abgelehnt.',
    invalid_message: 'Schreib eine Nachricht mit 1 bis 500 Zeichen.',
    not_controllable:
      'Diese App unterstützt keine Fernsteuerung: Sie kann von hier weder gestoppt werden noch eine Nachricht erhalten.',
    remote_control_refused:
      'Dein Konto darf die Apps anderer Benutzer nicht steuern. Ein anderer Administrator kann ihm diese Berechtigung auf der Seite „Benutzer“ geben.',
    too_soon:
      'Dieses Add-on wurde vor weniger als einer Minute geprüft. Versuche es gleich noch einmal.',
    invalid_limit: 'Diese Liste kann nicht so viele Einträge anzeigen.',
    addon_kind_changed:
      'Diese Adresse führt jetzt zu einer anderen Art von Add-on (einem Musik-Add-on statt eines Video-Add-ons oder umgekehrt). Installiere es stattdessen als neues Add-on.',
    invalid_addon_settings:
      'Das Add-on hat diese Einstellungen abgelehnt: Ein Wert ist keine seiner Auswahlmöglichkeiten, zu lang oder außerhalb des zulässigen Bereichs. Prüfe die Felder und versuche es erneut.',
    invalid_options:
      'Diese Importoptionen werden nicht akzeptiert. Lade die Seite neu und versuche es erneut.',
    invalid_category_name: 'Kategorienamen müssen 1 bis 64 Zeichen lang sein.',
    category_not_custom: 'Nur deine eigenen Kategorien können gelöscht werden.',
    invalid_channel_name: 'Kanalnamen müssen 1 bis 100 Zeichen lang sein.',
    invalid_logo:
      'Gib eine Logo-Adresse ein, die mit https:// oder http:// beginnt, mit höchstens 4.096 Zeichen.',
    invalid_description: 'Beschreibungen dürfen höchstens 2.000 Zeichen lang sein.',
    invalid_category: 'Diese Kategorie existiert nicht mehr. Lade die Seite neu.',
    invalid_number: 'Gib eine ganze Zahl von 1 bis 99.999 ein oder lass die Nummer leer.',
    invalid_move:
      'Dieser Kanal kann nur innerhalb seiner Kategorie verschoben werden. Lade die Seite neu.',
    invalid_bulk: 'Wähle Kanäle oder ein Stichwort mit mindestens 2 Zeichen.',
    invalid_streams: 'Die Streams haben sich inzwischen geändert. Lade den Kanal neu.',
    invalid_stream_url:
      'Gib eine Stream-Adresse ein, die mit https:// oder http:// beginnt, mit höchstens 4.096 Zeichen.',
    stream_not_custom: 'Nur von dir hinzugefügte Streams können entfernt werden.',
    too_many_guides: 'Ein Katalog nimmt höchstens 10 Fernsehprogramme auf.',
    invalid_guide: 'Dieses Fernsehprogramm existiert nicht mehr. Lade die Seite neu.',
    invalid_mode: 'Dieser Zuordnungsmodus wird nicht unterstützt. Lade die Seite neu.',
    invalid_mapping:
      'Dieser Programmkanal ist nicht mehr in den Fernsehprogrammen des Katalogs. Suche erneut.',
    invalid_jellyfin_address:
      'Gib die Adresse des Jellyfin-Servers ein, zum Beispiel http://192.168.1.10:8096.',
    jellyfin_key_refused:
      'Jellyfin hat diesen API-Schlüssel abgelehnt. Erstelle einen in der Jellyfin-Übersicht unter „API-Schlüssel“ und füge ihn erneut ein.',
    jellyfin_key_limited:
      'Mit diesem Schlüssel oder Konto können die Benutzer des Servers nicht aufgelistet werden. Verwende einen API-Schlüssel aus der Übersicht des Servers oder das Konto eines Administrators.',
    jellyfin_key_owner_only:
      'Dieser Schlüssel liest nur die Wiedergabedaten seines Besitzers. Gib die Passwörter der anderen Benutzer auf dem Server ein oder entferne für sie das Häkchen bei „Wiedergabedaten importieren“.',
    jellyfin_sign_in_refused: 'Jellyfin hat diesen Namen oder dieses Passwort abgelehnt.',
    jellyfin_sign_in_forbidden:
      'Jellyfin lässt die Anmeldung mit diesem Konto nicht zu: Es ist möglicherweise deaktiviert oder außerhalb seiner erlaubten Zeiten.',
    jellyfin_password_refused:
      'Jellyfin hat dieses Passwort abgelehnt. Lass das Feld leer, wenn das Konto kein Passwort hat.',
    jellyfin_other_user:
      'Mit diesem Passwort wurde das Konto eines anderen Benutzers auf dem Server geöffnet. Verbinde dich stattdessen mit einem API-Schlüssel aus der Übersicht des Servers.',
    jellyfin_unreachable:
      'Unter dieser Adresse hat nichts geantwortet. Überprüfe die Adresse und ob Jellyfin läuft.',
    not_jellyfin:
      'Unter dieser Adresse hat ein Server geantwortet, aber es ist nicht Jellyfin. Überprüfe die Adresse.',
    jellyfin_import_running:
      'Es läuft bereits ein Import. Warte, bis er abgeschlossen ist, oder brich ihn ab.',
    unknown_jellyfin_user:
      'Ein Jellyfin-Benutzer ist nicht mehr auf diesem Server vorhanden. Verbinde dich erneut, um die Benutzer neu einzulesen.',
    emby_user_key:
      'Dieser Schlüssel gehört einem einzelnen Benutzer, und Emby verrät nicht, welchem. Erstelle einen API-Schlüssel in den Emby-Einstellungen oder verbinde dich mit einem Benutzerkonto.',
    server_import_running:
      'Auf diesem Server oder in dieses Konto läuft bereits ein anderer Import. Versuche es in ein paar Minuten erneut.',
    server_imports_disabled:
      'Der Import deines Wiedergabeverlaufs von einem anderen Server oder einer App ist auf diesem Server deaktiviert.',
    letterboxd_export_invalid:
      'Diese Datei ist kein Letterboxd-Export: Wähle die ZIP-Datei, die Letterboxd bereitstellt, unverändert aus. Sie enthält watched.csv oder diary.csv.',
    letterboxd_export_too_large:
      'Diese Datei ist größer als 20 MB oder enthält mehr als 50.000 Filme: Wähle die ZIP-Datei, die Letterboxd bereitstellt.',
    invalid_nuvio_address:
      'Gib die Adresse des Nuvio-Backends ein, zum Beispiel https://api.nuvio.tv, oder lass das Feld leer, um das Backend von Nuvio zu verwenden.',
    nuvio_sign_in_refused: 'Nuvio hat diese E-Mail-Adresse oder dieses Passwort abgelehnt.',
    nuvio_unreachable:
      'Das Nuvio-Backend hat nicht geantwortet. Überprüfe die Adresse und versuche es später erneut.',
    not_nuvio:
      'Unter dieser Adresse hat etwas geantwortet, aber kein Nuvio-Backend. Überprüfe die Adresse oder lass das Feld leer, um das Backend von Nuvio zu verwenden.',
    nuvio_rate_limited:
      'Nuvio bittet darum, vor einer erneuten Anmeldung zu warten. Versuche es in ein paar Minuten erneut.',
    nuvio_backup_unsupported:
      'Nuvio hat die Kontodaten in einem Format geliefert, das diese Version von Polyfin nicht lesen kann. Polyfin liest Version 1 des Nuvio-Konto-Backups.',
    stremio_sign_in_refused: 'Stremio hat diese E-Mail-Adresse oder dieses Passwort abgelehnt.',
    stremio_pin_refused: 'Stremio hat diese PIN abgelehnt.',
    stremio_key_refused:
      'Stremio hat diesen Authentifizierungsschlüssel abgelehnt. Kopiere ihn erneut aus der Stremio-Web-App, während du bei deinem Konto angemeldet bist.',
    stremio_link_expired:
      'Der Verknüpfungscode ist abgelaufen oder wurde bereits verwendet. Fordere einen neuen an.',
    stremio_unreachable: 'Stremio hat nicht geantwortet. Versuche es später erneut.',
    stremio_unexpected:
      'Stremio hat in einer Form geantwortet, die diese Version von Polyfin nicht lesen kann. Versuche es später erneut.',
    app_import_expired:
      'Die gelesenen Kontodaten werden nach ein paar Minuten nicht mehr aufbewahrt. Melde dich erneut an.',
    app_import_running: 'In dieses Konto läuft gerade ein Import. Warte, bis er abgeschlossen ist.',
    plex_sign_in_ended:
      'Die Anmeldung bei Plex ist beendet: Sie ist abgelaufen, wurde ersetzt oder von einem Import verwendet. Melde dich erneut mit Plex an.',
    plex_sign_in_pending:
      'Plex hat dich noch nicht angemeldet. Schließe die Anmeldung auf der Seite von Plex ab.',
    plex_unknown_profile:
      'Dieses Plex Home hat kein solches Profil. Melde dich erneut mit Plex an.',
    plex_profile_pin_refused: 'Plex hat diese PIN abgelehnt. Gib die PIN des Profils erneut ein.',
    plex_profile_forbidden: 'Plex lässt dieses Konto nicht zu diesem Profil wechseln.',
    plex_unknown_server: 'Dein Plex-Konto greift nicht auf diesen Server zu. Wähle einen anderen.',
    plex_not_owner:
      'Dieses Plex-Konto ist nicht der Besitzer des Servers. Melde dich mit dem Plex-Konto des Besitzers an.',
    plex_server_unreachable:
      'Keine der Adressen, die Plex für diesen Server angibt, hat als dieser Server geantwortet. Prüfe, ob er läuft und Polyfin ihn erreichen kann.',
    plex_tv_failed: 'plex.tv hat nicht wie erwartet geantwortet. Versuche es später erneut.',
    plex_tv_unreachable: 'plex.tv ist nicht erreichbar. Prüfe die Internetverbindung von Polyfin.',
    app_not_connected: 'Diese App ist nicht mehr mit dem Konto verbunden.',
    app_connection_stopped:
      'Die App hat die von Polyfin gespeicherte Anmeldung abgelehnt. Melde dich erneut an und setze dabei das Häkchen bei „Angemeldet bleiben“.',
    invalid_ends_on: 'Wähle das Datum, an dem das Konto endet, oder „Nie“.',
    administrator_cannot_end:
      'Das Konto eines Administrators endet nie. Mach die Person zuerst zu einem Mitglied, um ein Enddatum festzulegen.',
    invalid_invite_account_end: 'Wähle aus der Liste, wann die Konten enden.',
  },
  status: {
    title: 'Serverstatus',
    description: 'Der Zustand dieses Polyfin-Servers auf einen Blick.',
    version: 'Version',
    serverId: 'Server-ID',
    database: 'Datenbank',
    databaseReady: 'Bereit',
    databaseUnavailable: 'Nicht verfügbar',
    loading: 'Serverstatus wird geladen…',
    errorTitle: 'Server nicht erreichbar',
    errorBody:
      'Die Verwaltungs-API hat nicht geantwortet. Prüfe, ob Polyfin läuft, und versuche es dann erneut.',
    staleWarning:
      'Die letzte Aktualisierung ist fehlgeschlagen. Die Angaben unten sind möglicherweise veraltet.',
    retry: 'Erneut versuchen',
    retrying: 'Neuer Versuch…',
    autoRefresh: 'Wird alle 10 Sekunden automatisch aktualisiert.',
    updatedAt: (time: string) => `Zuletzt aktualisiert um ${time}`,
  },
  devices: {
    empty: 'Kein Gerät ist angemeldet.',
    lastActivity: 'Letzte Aktivität',
    address: 'Adresse',
    signOut: 'Abmelden',
    signOutConfirm: (device: string) =>
      `„${device}“ abmelden? Die App muss sich dann erneut anmelden.`,
    signedOut: (device: string) => `„${device}“ wurde abgemeldet.`,
  },
  stremioTypes: {
    movie: 'Filme',
    series: 'Serien',
    channel: 'Kanäle',
    tv: 'TV',
    track: 'Titel',
    album: 'Alben',
    artist: 'Interpreten',
    playlist: 'Wiedergabelisten',
    other: 'Andere Videos',
  },
  stremioResources: {
    catalog: 'Kataloge',
    meta: 'Details',
    stream: 'Streams',
    subtitles: 'Untertitel',
    addon_catalog: 'Add-on-Kataloge',
    search: 'Suche',
    isrc: 'Titelabgleich',
    resolve: 'Wiedergabelinks',
    settings: 'Einstellungen',
  },
  forbidden: {
    title: 'Nicht erlaubt',
    description: 'Nur Administratoren können diese Seite öffnen.',
    home: 'Zurück zur Startseite',
  },
  time: {
    justNow: 'gerade eben',
  },
  ui: {
    on: 'Ein',
    off: 'Aus',
    show: 'Anzeigen',
    hide: 'Ausblenden',
    close: 'Schließen',
    dismiss: 'Verwerfen',
    notifications: 'Benachrichtigungen',
    unsaved: 'Ungespeicherte Änderungen',
    allSaved: 'Alle Änderungen gespeichert',
    discard: 'Änderungen verwerfen',
    searchList: 'Liste durchsuchen',
    noMatch: 'Keine Treffer.',
    secret: {
      saved: 'Gespeichert',
      notSet: 'Nicht festgelegt',
      removing: 'Wird beim Speichern entfernt',
      pastePlaceholder: 'Hier einfügen',
      replacePlaceholder: 'Neuen Wert einfügen',
      replace: 'Ersetzen',
      cancelReplace: 'Gespeicherten Wert behalten',
      remove: 'Entfernen',
      keep: 'Behalten',
      removeHint: 'Speichere, um ihn zu entfernen, oder wähle „Behalten“.',
      show: 'Schlüssel anzeigen',
      hide: 'Schlüssel ausblenden',
      savedHidden: 'Gespeicherter Schlüssel, ausgeblendet',
    },
  },
}

export default common
