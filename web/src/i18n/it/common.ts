import type en from '../en/common'

const common: typeof en = {
  documentTitle: 'Amministrazione Polyfin',
  header: {
    productName: 'Polyfin',
  },
  language: {
    label: 'Lingua di visualizzazione',
    en: { short: 'EN', name: 'English' },
    fr: { short: 'FR', name: 'Français' },
    de: { short: 'DE', name: 'Deutsch' },
    es: { short: 'ES', name: 'Español' },
    it: { short: 'IT', name: 'Italiano' },
    'pt-BR': { short: 'PT', name: 'Português (Brasil)' },
  },
  common: {
    enterNumber: 'Inserisci un numero.',
    loading: 'Caricamento…',
    save: 'Salva',
    saving: 'Salvataggio…',
    cancel: 'Annulla',
    retry: 'Riprova',
    never: 'Mai',
    colon: ':',
    passwordRule: 'Almeno 8 caratteri.',
    nameRule: "Da 1 a 64 caratteri: lettere, cifre, spazi e - _ ' . @ +",
    passwordMismatch: 'Le due password non corrispondono.',
    moveUp: (name: string) => `Sposta ${name} in alto`,
    moveDown: (name: string) => `Sposta ${name} in basso`,
    moved: (name: string, position: number, count: number) =>
      `${name}: ora in posizione ${position} di ${count}.`,
  },
  errors: {
    generic: 'Si è verificato un errore. Riprova.',
    network: 'Impossibile raggiungere il server. Verifica che Polyfin sia in esecuzione.',
    unauthenticated: 'La sessione è scaduta. Accedi di nuovo.',
    forbidden: 'Non hai i permessi per eseguire questa operazione.',
    cross_origin: 'La richiesta è stata rifiutata perché non proviene da questa pagina.',
    invalid_setup_code:
      'Questo codice di configurazione non è valido. Copialo di nuovo dal log del server.',
    invalid_name: "I nomi devono avere da 1 a 64 caratteri: lettere, cifre, spazi e - _ ' . @ +",
    invalid_password: 'La password deve avere almeno 8 caratteri.',
    setup_complete: 'La configurazione è già stata completata. Accedi.',
    too_many_attempts: 'Troppi tentativi. Attendi qualche minuto prima di riprovare.',
    invalid_credentials: 'Nome o password errati.',
    account_disabled: 'Questo account è disabilitato. Chiedi a un amministratore di abilitarlo.',
    wrong_password: 'La password attuale non è corretta.',
    invalid_code:
      'Questo codice non è valido. Inserisci il codice a 6 cifre mostrato ora dalla tua applicazione, oppure uno dei tuoi codici di recupero.',
    two_step_expired: 'L’accesso ha richiesto troppo tempo. Inserisci di nuovo nome e password.',
    two_step_on: 'L’accesso in due passaggi è già attivo.',
    two_step_off: 'L’accesso in due passaggi è disattivato per questo account.',
    two_step_not_started:
      'Ricomincia: attiva l’accesso in due passaggi per ottenere una nuova chiave.',
    two_step_required:
      'Su questo server gli amministratori devono usare l’accesso in due passaggi: non può essere disattivato.',
    two_step_own: 'Disattiva il tuo accesso in due passaggi da Il mio account.',
    not_found: 'Questo elemento non esiste più. L’elenco è stato aggiornato.',
    not_set:
      'Nessun valore salvato: è stato rimosso, oppure non può essere decifrato con POLYFIN_SECRET_KEY. Ricarica la pagina.',
    not_revealable: 'Questa connessione non contiene alcuna chiave da mostrare.',
    unknown_code:
      'Nessun dispositivo è in attesa con questo codice. Potrebbe essere scaduto o digitato in modo errato.',
    quick_connect_disabled: 'Quick Connect è disattivato su questo server.',
    name_taken: 'Questo nome è già usato da un altro utente.',
    invalid_invite_uses:
      'Il numero di account che un link di invito può creare deve essere un numero intero da 1 a 100.',
    invalid_invite_expiry: 'Scegli dall’elenco quando scade il link di invito.',
    invalid_invite_model: 'Questo utente non esiste più. Scegline un altro.',
    invite_unknown: 'Questo link di invito non funziona.',
    invite_used_up: 'Questo link di invito è esaurito.',
    invite_expired: 'Questo link di invito è scaduto.',
    invite_revoked: 'Questo link di invito è stato revocato.',
    share_links_off:
      'Non puoi creare link di condivisione: un amministratore li ha disattivati per questo server o per il tuo account.',
    not_shareable: 'Puoi condividere solo un film o un episodio che sei in grado di vedere.',
    invalid_share_expiry: 'Scegli dall’elenco quando scade il link.',
    invalid_share_viewers:
      'Il numero di spettatori contemporanei deve essere un numero intero da 1 a 5.',
    last_administrator:
      'Questo è l’ultimo amministratore abilitato: non può essere eliminato, retrocesso o disabilitato.',
    admin_cannot_join_group:
      'Gli amministratori non possono entrare in un gruppo, perché i loro permessi sono diversi. Ricarica la pagina per vedere chi lo è ora.',
    invalid_group: 'Questo gruppo non esiste più. Ricarica la pagina e scegline un altro.',
    follows_group:
      'Questo utente segue il proprio gruppo: cambia il gruppo, oppure disattiva prima «Segui il gruppo».',
    invalid_copy_from: 'Questo utente non esiste più. Scegline un altro.',
    invalid_server_name: 'Il nome del server deve avere da 1 a 64 caratteri.',
    invalid_language: 'Scegli una lingua dall’elenco.',
    invalid_catalog_limit:
      'I titoli letti per catalogo devono essere un numero intero da 100 a 20.000.',
    invalid_channel_limit:
      'I canali letti per catalogo di Diretta TV devono essere un numero intero da 100 a 50.000.',
    invalid_played_percent: 'La soglia di visione deve essere un numero intero da 50 a 100.',
    invalid_resume_percent: 'La soglia di ripresa deve essere un numero intero da 0 a 50.',
    resume_not_below_played: 'La soglia di ripresa deve essere inferiore alla soglia di visione.',
    invalid_version_list_minutes:
      'Il tempo di conservazione degli elenchi delle versioni deve essere un numero intero di minuti da 1 a 360.',
    invalid_catalog_refresh_minutes:
      'L’intervallo di aggiornamento dei cataloghi deve essere un numero intero di minuti da 1 a 1.440.',
    invalid_analysis_timeout:
      'Il tempo per analizzare una versione deve essere un numero intero di secondi da 5 a 120.',
    invalid_version_attempts: 'Le versioni provate devono essere un numero intero da 1 a 10.',
    invalid_version_order:
      'Scegli al massimo quattro criteri per ordinare le versioni, uno per tipo. Ricarica la pagina.',
    invalid_version_codec_order:
      'L’ordine dei codec video deve elencare ogni codec una sola volta. Ricarica la pagina.',
    invalid_version_source_order:
      'L’ordine delle origini deve elencare origini del server, ciascuna una sola volta. Ricarica la pagina.',
    invalid_hide_versions_larger:
      'La dimensione oltre la quale le versioni vengono nascoste deve essere 0, oppure un numero intero di GB da 1 a 1.000.',
    invalid_hidden_version_codecs:
      'Questo elenco di codec nascosti non è supportato. Ricarica la pagina.',
    invalid_hidden_version_words:
      'Inserisci al massimo 50 parole che nascondono le versioni, una per riga, ciascuna di al massimo 100 caratteri.',
    invalid_hidden_releases:
      'Questo elenco di tipi di release nascosti non è supportato. Ricarica la pagina.',
    invalid_max_conversions:
      'Le conversioni video contemporanee devono essere un numero intero da 0 a 32.',
    invalid_max_conversion_height: 'Scegli dall’elenco la qualità massima dei video convertiti.',
    invalid_subtitle_ocr: 'Scegli dall’elenco quando convertire in testo i sottotitoli grafici.',
    invalid_subtitle_ocr_tracks:
      'Le tracce di sottotitoli grafici lette contemporaneamente devono essere un numero intero da 1 a 4.',
    invalid_default_language: 'Scegli una lingua predefinita dall’elenco.',
    invalid_encoder_preset: 'Scegli una velocità di codifica dall’elenco.',
    invalid_video_quality: 'La qualità video deve essere 0, oppure un numero intero da 1 a 51.',
    invalid_hardware_acceleration: 'Scegli una scheda grafica dall’elenco.',
    invalid_hardware_decoding_codecs:
      'Questo elenco di formati decodificati dalla scheda grafica non è supportato. Ricarica la pagina.',
    invalid_adaptive_quality: 'Scegli dall’elenco quando offrire più qualità.',
    invalid_tone_mapping_algorithm: 'Scegli un metodo di tone mapping dall’elenco.',
    invalid_tone_mapping_peak:
      'La luminosità di picco deve essere 0, oppure un numero intero di nit da 100 a 10.000.',
    invalid_tone_mapping_desat: 'La desaturazione delle alte luci deve essere un numero da 0 a 10.',
    invalid_processor_tone_mapping_height:
      'Scegli dall’elenco la qualità massima dell’HDR convertito dal processore.',
    invalid_deinterlace_method: 'Scegli un metodo di deinterlacciamento dall’elenco.',
    invalid_downmix_algorithm: 'Scegli un mixaggio stereo dall’elenco.',
    invalid_downmix_boost: 'Il volume del mixaggio stereo deve essere un numero da 0,5 a 3.',
    invalid_max_audio_channels: 'Scegli dall’elenco il numero massimo di canali audio.',
    invalid_audio_bitrate_per_channel:
      'Il bitrate audio per canale deve essere 0 o un numero intero di kb/s da 32 a 320.',
    invalid_encoding_threads: 'I thread del processore devono essere un numero intero da 0 a 64.',
    invalid_ahead_seconds:
      'I secondi preparati in anticipo devono essere un numero intero da 30 a 600.',
    parental_control:
      'Il controllo parentale si applica a questo account: mantiene gli add-on del server, che forniscono le classificazioni su cui si basa.',
    invalid_parental_control:
      'Questa impostazione del controllo parentale non è supportata. Ricarica la pagina.',
    invalid_max_playbacks: 'Le riproduzioni simultanee devono essere un numero intero da 0 a 20.',
    invalid_max_bitrate: 'Scegli la qualità massima dall’elenco.',
    invalid_sync_play: 'Scegli un’opzione di «Guarda insieme» dall’elenco.',
    invalid_manifest_url:
      'Inserisci l’indirizzo del manifest di un add-on che inizi con https://, http:// o stremio:// e termini con /manifest.json.',
    addon_exists: 'Questo add-on è già installato qui.',
    addon_unreachable:
      'Impossibile raggiungere l’add-on. Controlla l’indirizzo e che l’add-on sia online, poi riprova.',
    invalid_manifest: 'Questo indirizzo non ha restituito il manifest di un add-on Stremio.',
    private_network:
      'Questo add-on si trova su un indirizzo di rete locale. Solo gli amministratori possono installare add-on di questo tipo.',
    invalid_order: 'L’elenco degli add-on è cambiato nel frattempo. È stato aggiornato: riprova.',
    invalid_library:
      'Una libreria fa riferimento a un catalogo che non esiste più o non può essere sfogliato. Rimuovi le librerie segnalate come non più disponibili, poi salva di nuovo.',
    invalid_library_name: 'Il nome di una libreria deve contenere da 1 a 64 caratteri.',
    invalid_library_genre:
      'Il genere di una libreria non è offerto dal suo catalogo. Scegline un altro dall’elenco, poi salva di nuovo.',
    invalid_library_max_items:
      'Il numero massimo di titoli di una libreria deve essere un numero intero da 1 a 20.000, oppure vuoto.',
    invalid_smart_list_name:
      'Il nome di una lista intelligente deve contenere da 1 a 64 caratteri.',
    invalid_smart_list_shown_as:
      'Scegli se la lista viene mostrata come libreria o come collezione.',
    invalid_smart_list_catalogs: 'Scegli da 1 a 50 cataloghi da cui la lista attinge.',
    invalid_smart_list_rules:
      'Una regola è incompleta: assegna un valore a ciascuna e almeno un limite a ogni anno o voto.',
    invalid_smart_list_sort: 'Scegli come ordinare la lista.',
    invalid_smart_list_max_items:
      'Il numero massimo di titoli di una lista intelligente deve essere un numero intero da 1 a 20.000, oppure vuoto.',
    too_many_smart_lists:
      'Ci sono già 100 liste intelligenti qui. Eliminane una per crearne un’altra.',
    invalid_image: 'Scegli un’immagine JPEG, PNG o WebP di massimo 10 MB.',
    invalid_image_url: 'Inserisci l’indirizzo di un’immagine che inizi con https:// o http://.',
    image_unreachable:
      'Impossibile scaricare un’immagine da questo indirizzo. Controlla che apra un’immagine, poi riprova.',
    image_private_network:
      'Questa immagine si trova su un indirizzo di rete locale. Solo gli amministratori possono usare indirizzi di questo tipo.',
    invalid_login_attempts:
      'Le password errate prima del blocco di un account devono essere 0 o un numero intero da 3 a 20.',
    invalid_inactive_device_days:
      'I giorni prima della disconnessione dei dispositivi inutilizzati devono essere un numero intero da 0 a 365.',
    invalid_https_mode: 'Servi HTTPS deve essere Disattivato, File di certificato o Let’s Encrypt.',
    invalid_https_port: 'La porta HTTPS deve essere un numero intero da 1 a 65535.',
    invalid_https_certificate:
      'Il certificato deve essere un percorso assoluto nel container, ad esempio /certs/fullchain.pem.',
    invalid_https_private_key:
      'La chiave privata deve essere un percorso assoluto nel container, ad esempio /certs/privkey.pem.',
    https_certificate_missing: 'Nessun file di certificato in questo percorso del container.',
    https_private_key_missing: 'Nessun file di chiave privata in questo percorso del container.',
    https_certificate_unreadable:
      'Polyfin non riesce a leggere questo file di certificato, oppure non contiene alcun certificato PEM. Controlla che l’utente 65532 possa leggerlo.',
    https_private_key_unreadable:
      'Polyfin non riesce a leggere questo file di chiave privata, oppure non contiene alcuna chiave RSA o ECDSA in PEM. Controlla che l’utente 65532 possa leggerlo.',
    https_files_mismatch: 'La chiave privata non corrisponde al certificato.',
    invalid_acme_hosts:
      'I nomi host devono essere nomi pubblici come media.example.org, uno per riga, senza caratteri jolly né indirizzi IP, al massimo 100 e almeno uno per Let’s Encrypt.',
    invalid_acme_email: 'L’email deve essere un indirizzo email, oppure vuota.',
    acme_terms_required: 'Accetta i termini di servizio di Let’s Encrypt per usarlo.',
    invalid_acme_challenge: 'La verifica deve essere HTTP-01 o TLS-ALPN-01.',
    invalid_known_proxies:
      'I proxy noti devono essere indirizzi IP, reti come 172.18.0.0/16 o nomi host come traefik, uno per riga, al massimo 64.',
    personal_addons_disabled:
      'I tuoi add-on sono stati disattivati da un amministratore. Vengono conservati, ma non puoi aggiungerne, aggiornarli né attivarli.',
    invalid_hidden_libraries:
      'L’elenco delle librerie è cambiato nel frattempo. Ricarica la pagina.',
    invalid_blocked_genres:
      'I generi devono contenere da 1 a 100 caratteri e se ne possono bloccare al massimo 100.',
    invalid_access_schedules:
      'Gli orari vanno dalle 00:00 alle 24:00 e ogni fine deve venire dopo il rispettivo inizio (al massimo 50 righe).',
    outside_allowed_hours:
      'Questo account non può essere usato in questo momento. Riprova durante gli orari consentiti.',
    invalid_guide_url:
      'Inserisci l’indirizzo di una guida che inizi con https:// o http://, di massimo 4.096 caratteri, o il percorso assoluto di un file del server.',
    invalid_app:
      'Il nome dell’applicazione deve contenere da 1 a 64 caratteri, senza caratteri speciali.',
    invalid_trickplay_interval:
      'Il tempo tra due miniature deve essere un numero intero di secondi da 5 a 60.',
    invalid_trickplay_width: 'Scegli la larghezza delle miniature dall’elenco.',
    invalid_thumbnail_storage_gb:
      'Lo spazio per le immagini deve essere un numero intero di GB da 1 a 50.',
    invalid_recording_padding:
      'I minuti prima e dopo una registrazione devono essere numeri interi da 0 a 60.',
    invalid_recording_retention_days:
      'I giorni di conservazione delle registrazioni devono essere un numero intero da 0 a 3.650.',
    invalid_convert_recordings_hours:
      'Scegli dagli elenchi due orari diversi tra cui convertire le registrazioni.',
    invalid_quality_group: 'Scegli il gruppo di qualità dall’elenco.',
    invalid_live_tv_refresh_hours:
      'L’intervallo di aggiornamento deve essere un numero intero di ore da 1 a 168.',
    invalid_local_scan_hours:
      'L’intervallo di scansione deve essere un numero intero di ore da 0 a 168.',
    invalid_custom_css: 'Il CSS personalizzato non deve superare 2 MB.',
    invalid_custom_js: 'Il JavaScript personalizzato non deve superare 2 MB.',
    invalid_web_theme: 'Scegli LumaaGlaass, Nessuno o Personalizzato.',
    theme_copy_running: 'Una copia del tema è già in corso. Riprova tra un momento.',
    theme_copy_nothing: 'I testi personalizzati salvati non caricano nulla da altrove.',
    theme_copy_failed:
      'Non è stato possibile salvare alcun file, quindi nessuna copia è stata fatta.',
    custom_texts_changed: 'I testi personalizzati sono cambiati durante la copia. Riprova.',
    invalid_login_disclaimer: 'Il messaggio di accesso non deve superare 8 KB.',
    invalid_publicmetadb_key:
      'PublicMetaDB non ha accettato questa chiave. Copiala di nuovo dal tuo account PublicMetaDB. Non è stato salvato nulla.',
    publicmetadb_unreachable:
      'Impossibile raggiungere PublicMetaDB per verificare la chiave. Non è stato salvato nulla: riprova più tardi.',
    invalid_theintrodb_key:
      'TheIntroDB non ha accettato questa chiave. Copiala di nuovo dal tuo account TheIntroDB. Non è stato salvato nulla.',
    theintrodb_unreachable:
      'Impossibile raggiungere TheIntroDB per verificare la chiave, oppure al momento limita le richieste. Non è stato salvato nulla: riprova più tardi.',
    invalid_mdblist_ratings_key:
      'MDBList non ha accettato questa chiave. Copiala di nuovo dal tuo account MDBList. Non è stato salvato nulla.',
    mdblist_unreachable:
      'Impossibile raggiungere MDBList per verificare la chiave. Non è stato salvato nulla: riprova più tardi.',
    invalid_tmdb_key:
      'TMDB non ha accettato questa chiave. Copia di nuovo la chiave API o il token di accesso in lettura dal tuo account TMDB. Non è stato salvato nulla.',
    tmdb_unreachable:
      'Impossibile raggiungere TMDB per verificare la chiave. Non è stato salvato nulla: riprova più tardi.',
    invalid_critic_rating_shows: 'Scegli dall’elenco quale punteggio mostra il voto della critica.',
    invalid_measure_loudness: 'Scegli dall’elenco di quali brani misurare il volume.',
    invalid_segment_order:
      'L’ordine delle origini dei marcatori di salto deve elencare ogni origine una sola volta. Ricarica la pagina.',
    invalid_trakt_app:
      'Il client ID e il client secret di Trakt devono avere al massimo 256 caratteri, senza spazi né caratteri speciali. Copiali di nuovo dalla tua applicazione Trakt.',
    invalid_simkl_app:
      'Il client ID di Simkl deve avere al massimo 256 caratteri, senza spazi né caratteri speciali. Copialo di nuovo dalla tua applicazione Simkl.',
    invalid_backup_hour: 'Scegli l’orario dei backup dall’elenco.',
    invalid_backups_kept: 'Il numero di backup conservati deve essere un numero intero da 1 a 90.',
    invalid_torrent_route:
      'Gli stream torrent richiedono un solo percorso: il nome di un’interfaccia di rete (solo Linux), un proxy SOCKS5 nel formato host:porta oppure una configurazione WireGuard.',
    invalid_wireguard_config: 'La configurazione WireGuard non è valida.',
    invalid_torrent_limit: 'I limiti devono essere numeri interi di Mbps da 0 a 10.000.',
    invalid_torrent_trackers:
      'Inserisci al massimo 30 tracker, ognuno un URL di annuncio HTTP, HTTPS o UDP, UDP con la sua porta, di al massimo 500 caratteri.',
    invalid_cache_size: 'Lo spazio su disco deve essere un numero intero di GB da 1 a 2.000.',
    invalid_vaapi_device: 'Scegli la scheda grafica dall’elenco.',
    invalid_recordings_folder:
      'La cartella delle registrazioni deve essere un percorso assoluto di una cartella in cui Polyfin può scrivere.',
    invalid_backup_folder:
      'La cartella dei backup deve essere un percorso assoluto di una cartella in cui Polyfin può scrivere.',
    invalid_conversion_folder:
      'La cartella di conversione deve essere un percorso assoluto di una cartella in cui Polyfin può scrivere.',
    invalid_reading_folder:
      'La cartella dei file in lettura deve essere un percorso assoluto di una cartella in cui Polyfin può scrivere.',
    invalid_cache_folder:
      'La cartella della cache deve essere un percorso assoluto di una cartella in cui Polyfin può scrivere.',
    too_many_destinations: 'Ci sono già 10 destinazioni del backup: eliminane prima una.',
    too_many_home_rows: 'Ci sono già 20 righe della Home: eliminane prima una.',
    invalid_home_row:
      'Dai alla riga un nome di massimo 100 caratteri e scegli il suo catalogo o la sua collezione.',
    invalid_destination_kind: 'Scegli il tipo di destinazione dall’elenco.',
    invalid_destination_address:
      'Inserisci l’indirizzo come richiesto: un indirizzo http:// o https:// senza percorso per uno storage S3, un nome host o un indirizzo IP per un server SFTP, l’indirizzo di una cartella per un server WebDAV.',
    invalid_bucket:
      'Inserisci il nome del bucket: da 3 a 63 lettere minuscole, cifre, punti, trattini o trattini bassi.',
    invalid_destination_folder:
      'La cartella non deve risalire con «..» né contenere caratteri di controllo, e deve occupare al massimo 512 byte.',
    invalid_region:
      'La regione è composta da lettere minuscole, cifre e trattini, ad esempio eu-west-3.',
    invalid_port: 'La porta è un numero intero da 1 a 65.535.',
    invalid_destination_user: 'Inserisci l’utente, oppure la chiave di accesso per uno storage S3.',
    invalid_destination_secret:
      'Inserisci la password, la chiave segreta per uno storage S3 oppure, per un server SFTP, una chiave privata.',
    invalid_private_key:
      'La chiave privata deve essere una chiave privata OpenSSH o PEM senza passphrase.',
    backups_off: 'I backup sono disattivati: prima attivali qui sopra.',
    pg_restore_missing:
      'pg_restore non è stato trovato sul server: installa gli strumenti client di PostgreSQL 18, inclusi nell’immagine Docker.',
    backup_not_found: 'Questo backup non è più disponibile: scegline un altro.',
    not_a_backup: 'Questo file non è un backup del database di Polyfin.',
    newer_backup:
      'Questo backup è stato creato da una versione più recente di Polyfin: prima aggiorna Polyfin.',
    changed_backup:
      'Questo backup è stato creato da un Polyfin le cui migrazioni del database sono diverse da quelle di questo.',
    restore_expired: 'Il backup scelto non è più pronto: sceglilo di nuovo.',
    unreadable_secrets:
      'Questo backup contiene chiavi che POLYFIN_SECRET_KEY non riesce a decifrare: conferma con Ripristina comunque.',
    restoring: 'È già in corso un ripristino.',
    wrong_server_name: 'Digita il nome del server esattamente come mostrato.',
    too_large: 'Il file è troppo grande.',
    destination_failed: 'Impossibile leggere la destinazione.',
    restore_failed: 'Impossibile avviare il ripristino: non è stato modificato nulla.',
    invalid_collection_read_hour:
      'Scegli dall’elenco l’orario di lettura delle collezioni, oppure Mai.',
    invalid_collection_library_apps:
      'Scegli dall’elenco le applicazioni che mostrano le librerie di collezioni per collezione.',
    invalid_remuxdb_url:
      'L’indirizzo di RemuxDB deve essere un indirizzo web che inizia con http:// o https://.',
    invalid_update_tool_url:
      'L’indirizzo dello strumento di aggiornamento deve essere un indirizzo web che inizia con http:// o https://, senza nome utente né frammento.',
    invalid_update_tool_token:
      'Il token dello strumento di aggiornamento non contiene spazi e ha al massimo 1024 caratteri.',
    invalid_provider:
      'Controlla il provider di identità: il suo emittente è un indirizzo web che inizia con http:// o https://, senza query né frammento; l’ID client, il segreto e i claim non contengono spazi; un claim nomina l’utente.',
    restart_unconfirmed: 'Polyfin non sa se qualcosa lo riavvia: conferma prima l’avviso.',
    no_update: 'Polyfin Desktop non offre questo aggiornamento al momento.',
    not_desktop: 'Polyfin non è eseguito in Polyfin Desktop.',
    no_update_tool: 'Nessuno strumento di aggiornamento è impostato nelle impostazioni generali.',
    update_tool_timeout:
      'Lo strumento di aggiornamento non ha risposto entro 50 secondi: forse sta ancora aggiornando.',
    update_tool_unreachable: 'Lo strumento di aggiornamento non è raggiungibile.',
    invalid_public_address:
      'L’indirizzo pubblico deve essere un indirizzo web che inizia con http:// o https://, senza parametri di query.',
    invalid_local_address:
      'L’indirizzo sulla rete locale deve essere un indirizzo web che inizia con http:// o https://, ad esempio http://192.168.1.10:8096, senza parametri di query.',
    invalid_dlna_name: 'Il nome sulla rete può essere lungo al massimo 256 byte.',
    invalid_dlna_user: 'Questo utente non esiste più. Scegline un altro, oppure «Nessuno».',
    invalid_dlna_profile:
      'Questo non è un profilo DLNA nel formato XML del plugin DLNA di Jellyfin.',
    invalid_smtp_host:
      'Inserisci il nome host del server SMTP, ad esempio smtp.example.org, senza spazi.',
    invalid_smtp_port: 'La porta SMTP deve essere un numero intero da 1 a 65535.',
    invalid_smtp_security: 'Scegli dall’elenco come viene cifrata la connessione SMTP.',
    invalid_smtp_account:
      'L’utente e la password SMTP possono avere al massimo 256 caratteri, su una sola riga.',
    invalid_smtp_sender:
      'Inserisci solo l’indirizzo del mittente, ad esempio polyfin@example.org, e un nome di al massimo 128 caratteri.',
    invalid_weekly_summary_day: 'Scegli dall’elenco il giorno del riepilogo settimanale.',
    invalid_weekly_summary_hour: 'Scegli dall’elenco l’ora del riepilogo settimanale.',
    invalid_import_every_minutes:
      'Scegli dall’elenco ogni quanto vengono importati gli account collegati.',
    invalid_target_kind: 'Scegli un tipo di destinazione dall’elenco.',
    invalid_target_name: 'I nomi delle destinazioni devono avere da 1 a 64 caratteri.',
    invalid_target_address: 'Inserisci un indirizzo web che inizi con https:// o http://.',
    private_target_address:
      'Questo indirizzo si trova su una rete locale: solo le destinazioni di un amministratore possono raggiungerlo.',
    invalid_email_address: 'Inserisci un indirizzo email, ad esempio sam@example.org.',
    email_unavailable:
      'L’email richiede un server SMTP: un amministratore lo configura in Impostazioni › Notifiche.',
    invalid_topic:
      'Un argomento ntfy è composto da 1 a 64 lettere, cifre, trattini e trattini bassi.',
    invalid_chat:
      'Inserisci il numero della chat, ad esempio -1001234567890, o il nome di un canale pubblico, ad esempio @news_example.',
    invalid_token:
      'Inserisci il token: al massimo 256 caratteri, senza spazi. Quello di un bot Telegram è simile a 123456:ABC-DEF.',
    invalid_user_key: 'Una chiave utente Pushover contiene solo lettere e cifre.',
    invalid_events: 'Uno degli eventi scelti non è disponibile per questa destinazione.',
    invalid_reminder_settings: 'Scegli 1, 5, 10, 15, 30 o 60 minuti.',
    too_many_targets: 'Ci sono già 20 destinazioni. Eliminane una per aggiungerne un’altra.',
    target_unreadable:
      'Impossibile decifrare l’indirizzo o il token di questa destinazione con POLYFIN_SECRET_KEY. Inseriscilo di nuovo.',
    invalid_source_name: 'I nomi delle origini devono avere da 1 a 64 caratteri.',
    invalid_source_address:
      'Inserisci un indirizzo che inizi con https:// o http://, o per una scaletta il percorso assoluto di un file del server, e, per un account Xtream Codes, un nome utente e una password.',
    file_missing:
      'Non c’è nessun file in questo percorso del server. Con Docker, indica il suo percorso nel container, in un volume montato.',
    file_unreadable:
      'Polyfin non può leggere questo file: verifica che sia un file e non una cartella, e che Polyfin possa leggerlo.',
    local_file_refused: 'Solo gli amministratori possono usare un file del server.',
    invalid_user_agent:
      'Uno user agent ha al massimo 512 caratteri, senza a capo né caratteri di controllo.',
    tuner_added: 'Questo tuner è già un’origine del server.',
    invalid_folder_name: 'I nomi delle cartelle devono avere da 1 a 64 caratteri.',
    invalid_folder_path:
      'Inserisci il percorso completo della cartella dove è in esecuzione Polyfin, diverso dalla radice del sistema: nel suo container, iniziando con /; su Windows, con la sua unità, ad esempio D:\\Film, o la sua condivisione, ad esempio \\\\nas\\media\\Film.',
    invalid_share_address:
      'Inserisci un indirizzo come smb://server/condivisione/cartella, https://server/cartella, s3://bucket/prefisso, sftp://server/cartella o rclone://server:porta/cartella, senza utente né password: hanno campi propri.',
    invalid_share_user:
      'Gli utenti possono avere al massimo 256 caratteri, le password al massimo 1.024.',
    invalid_share_endpoint:
      'Inserisci l’endpoint come indirizzo http:// o https://, ad esempio https://s3.example.org, e una regione di al massimo 64 caratteri senza spazi.',
    invalid_share_key:
      'Incolla una chiave privata OpenSSH o PEM completa e, se ne ha una, la passphrase che la sblocca.',
    invalid_rclone_remote:
      'Inserisci il nome di uno dei remote di rclone, senza due punti né barre, di al massimo 128 caratteri.',
    invalid_poll_minutes:
      'Inserisci un numero di minuti da 0 a 1.440 (0 per cercare modifiche solo durante le scansioni).',
    invalid_folder_kind: 'Scegli se la cartella contiene film o serie TV.',
    invalid_imdb_id:
      'Un identificativo IMDb è formato da tt seguito da cifre, ad esempio tt0063350.',
    invalid_episode:
      'Scrivi l’episodio come lo indica il nome di un file, ad esempio S01E05, o S01E05-E06 per due.',
    title_search_failed: 'Impossibile cercare negli add-on dei metadati. Riprova tra un momento.',
    invalid_channel_list:
      'Questo indirizzo non ha restituito un elenco canali. Controllalo e verifica che l’account sia ancora valido.',
    channel_list_too_large:
      'Questo elenco canali è troppo grande (al massimo 100 MB o 100.000 canali).',
    iptv_login_refused: 'Il server IPTV ha rifiutato questo nome utente o questa password.',
    invalid_message: 'Scrivi un messaggio da 1 a 500 caratteri.',
    not_controllable:
      'Questa applicazione non accetta il controllo remoto: da qui non è possibile fermarla né inviarle un messaggio.',
    remote_control_refused:
      'Il tuo account non può controllare le applicazioni di altri utenti. Un altro amministratore può concedergli questo permesso nella pagina Utenti.',
    too_soon: 'Questo add-on è stato controllato meno di un minuto fa. Riprova tra un momento.',
    invalid_limit: 'Questo elenco non può mostrare così tante voci.',
    addon_kind_changed:
      'Questo indirizzo ora porta a un altro tipo di add-on (un add-on musicale invece che video, o viceversa). Installalo invece come nuovo add-on.',
    invalid_addon_settings:
      'L’add-on ha rifiutato queste impostazioni: un valore non è tra le sue scelte, è troppo lungo o fuori intervallo. Controlla i campi e riprova.',
    invalid_options:
      'Queste opzioni di importazione non sono accettate. Ricarica la pagina e riprova.',
    invalid_category_name: 'I nomi delle categorie devono avere da 1 a 64 caratteri.',
    category_not_custom: 'Puoi eliminare solo le tue categorie.',
    invalid_channel_name: 'I nomi dei canali devono avere da 1 a 100 caratteri.',
    invalid_logo:
      'Inserisci un indirizzo del logo che inizi con https:// o http://, di al massimo 4.096 caratteri.',
    invalid_description: 'Le descrizioni possono avere al massimo 2.000 caratteri.',
    invalid_category: 'Questa categoria non esiste più. Ricarica la pagina.',
    invalid_number: 'Inserisci un numero intero da 1 a 99.999, oppure lascia vuoto il numero.',
    invalid_move:
      'Questo canale può essere spostato solo all’interno della sua categoria. Ricarica la pagina.',
    invalid_bulk: 'Scegli dei canali, o una parola chiave di almeno 2 caratteri.',
    invalid_streams: 'Gli stream sono cambiati nel frattempo. Ricarica il canale.',
    invalid_stream_url:
      'Inserisci un indirizzo dello stream che inizi con https:// o http://, di al massimo 4.096 caratteri.',
    stream_not_custom: 'Puoi rimuovere solo gli stream che hai aggiunto.',
    too_many_guides: 'Un catalogo può avere al massimo 10 guide.',
    invalid_guide: 'Questa guida non esiste più. Ricarica la pagina.',
    invalid_mode: 'Questa modalità di abbinamento non è supportata. Ricarica la pagina.',
    invalid_mapping:
      'Questo canale della guida non è più nelle guide del catalogo. Cerca di nuovo.',
    invalid_jellyfin_address:
      'Inserisci l’indirizzo del server Jellyfin, ad esempio http://192.168.1.10:8096.',
    jellyfin_key_refused:
      'Jellyfin ha rifiutato questa chiave API. Creane una nel suo pannello di controllo, in Chiavi API, e incollala di nuovo.',
    jellyfin_key_limited:
      'Questa chiave o questo account potrebbe non poter elencare gli utenti del server. Usa una chiave API del suo pannello di controllo o l’account di un amministratore.',
    jellyfin_key_owner_only:
      'Questa chiave legge solo i dati di visione del suo proprietario. Inserisci le password degli altri utenti sul server, oppure deseleziona «Importa dati di visione» per loro.',
    jellyfin_sign_in_refused: 'Jellyfin ha rifiutato questo nome o questa password.',
    jellyfin_sign_in_forbidden:
      'Jellyfin non consente l’accesso a questo account: potrebbe essere disabilitato o fuori dagli orari consentiti.',
    jellyfin_password_refused:
      'Jellyfin ha rifiutato questa password. Lascia il campo vuoto se l’account non ne ha una.',
    jellyfin_other_user:
      'Questa password ha aperto l’account di un altro utente sul server. Connettiti invece con una chiave API del suo pannello di controllo.',
    jellyfin_unreachable:
      'Nessuna risposta da questo indirizzo. Controllalo e verifica che Jellyfin sia in esecuzione.',
    not_jellyfin:
      'Un server ha risposto a questo indirizzo, ma non è Jellyfin. Controlla l’indirizzo.',
    jellyfin_import_running:
      'È già in corso un’importazione. Attendi che finisca oppure interrompila.',
    unknown_jellyfin_user:
      'Un utente Jellyfin non è più presente su quel server. Connettiti di nuovo per leggerne gli utenti.',
    emby_user_key:
      'Questa chiave appartiene a un utente ed Emby non indica a chi. Crea una chiave API nelle impostazioni di Emby oppure connettiti con un account utente.',
    server_import_running:
      'È in corso un’altra importazione su questo server o in questo account. Riprova tra qualche minuto.',
    server_imports_disabled:
      'L’importazione della cronologia di visione da un altro server o da un’app è disattivata su questo server.',
    letterboxd_export_invalid:
      'Questo file non è un’esportazione di Letterboxd: scegli il file ZIP fornito da Letterboxd, così com’è. Contiene watched.csv o diary.csv.',
    letterboxd_export_too_large:
      'Questo file supera i 20 MB o elenca più di 50.000 film: scegli il file ZIP fornito da Letterboxd.',
    invalid_nuvio_address:
      'Inserisci l’indirizzo del backend Nuvio, ad esempio https://api.nuvio.tv, oppure lascia il campo vuoto per usare quello di Nuvio.',
    nuvio_sign_in_refused: 'Nuvio ha rifiutato questa email o questa password.',
    nuvio_unreachable:
      'Il backend Nuvio non ha risposto. Controllane l’indirizzo e riprova più tardi.',
    not_nuvio:
      'Qualcosa ha risposto a questo indirizzo, ma non è un backend Nuvio. Controlla l’indirizzo oppure lascia il campo vuoto per usare quello di Nuvio.',
    nuvio_rate_limited:
      'Nuvio chiede di attendere prima di accedere di nuovo. Riprova tra qualche minuto.',
    nuvio_backup_unsupported:
      'Nuvio ha restituito i dati dell’account in un formato che questa versione di Polyfin non legge. Polyfin legge la versione 1 del backup dell’account di Nuvio.',
    stremio_sign_in_refused: 'Stremio ha rifiutato questa email o questa password.',
    stremio_pin_refused: 'Stremio ha rifiutato questo PIN.',
    stremio_key_refused:
      'Stremio ha rifiutato questa chiave di autenticazione. Copiala di nuovo dall’app web di Stremio, con l’accesso al tuo account.',
    stremio_link_expired:
      'Il codice di collegamento è scaduto o è già stato usato. Richiedine un altro.',
    stremio_unreachable: 'Stremio non ha risposto. Riprova più tardi.',
    stremio_unexpected:
      'Stremio ha risposto in un modo che questa versione di Polyfin non legge. Riprova più tardi.',
    app_import_expired:
      'I dati letti dall’account non vengono più conservati dopo qualche minuto. Accedi di nuovo.',
    app_import_running: 'È in corso un’importazione in questo account. Attendi che finisca.',
    app_not_connected: 'Questa app non è più collegata all’account.',
    app_connection_stopped:
      'L’app ha rifiutato l’accesso conservato da Polyfin. Accedi di nuovo, con «Resta connesso» selezionato.',
    invalid_ends_on: 'Scegli la data di fine dell’account oppure «Mai».',
    administrator_cannot_end:
      'L’account di un amministratore non ha scadenza. Rendilo prima un membro per assegnargli una data di fine.',
    invalid_invite_account_end: 'Scegli dall’elenco quando terminano gli account.',
  },
  status: {
    title: 'Stato del server',
    description: 'Lo stato di salute di questo server Polyfin a colpo d’occhio.',
    version: 'Versione',
    serverId: 'ID del server',
    database: 'Database',
    databaseReady: 'Pronto',
    databaseUnavailable: 'Non disponibile',
    loading: 'Caricamento dello stato del server…',
    errorTitle: 'Server non raggiungibile',
    errorBody:
      'L’API di amministrazione non ha risposto. Verifica che Polyfin sia in esecuzione, poi riprova.',
    staleWarning:
      'L’ultimo aggiornamento non è riuscito. I dettagli qui sotto potrebbero non essere aggiornati.',
    retry: 'Riprova',
    retrying: 'Nuovo tentativo…',
    autoRefresh: 'Si aggiorna automaticamente ogni 10 secondi.',
    updatedAt: (time: string) => `Ultimo aggiornamento alle ${time}`,
  },
  devices: {
    empty: 'Nessun dispositivo ha effettuato l’accesso.',
    lastActivity: 'Ultima attività',
    address: 'Indirizzo',
    signOut: 'Disconnetti',
    signOutConfirm: (device: string) =>
      `Disconnettere «${device}»? L’applicazione dovrà accedere di nuovo.`,
    signedOut: (device: string) => `«${device}» è stato disconnesso.`,
  },
  stremioTypes: {
    movie: 'Film',
    series: 'Serie TV',
    channel: 'Canali',
    tv: 'TV',
    track: 'Brani',
    album: 'Album',
    artist: 'Artisti',
    playlist: 'Scalette',
    other: 'Altri video',
  },
  stremioResources: {
    catalog: 'Cataloghi',
    meta: 'Dettagli',
    stream: 'Stream',
    subtitles: 'Sottotitoli',
    addon_catalog: 'Cataloghi di add-on',
    search: 'Ricerca',
    isrc: 'Abbinamento dei brani',
    resolve: 'Link di riproduzione',
    settings: 'Impostazioni',
  },
  forbidden: {
    title: 'Accesso negato',
    description: 'Solo gli amministratori possono aprire questa pagina.',
    home: 'Torna alla Home',
  },
  time: {
    justNow: 'proprio ora',
  },
  ui: {
    on: 'Attivo',
    off: 'Disattivato',
    show: 'Mostra',
    hide: 'Nascondi',
    close: 'Chiudi',
    dismiss: 'Ignora',
    notifications: 'Notifiche',
    unsaved: 'Modifiche non salvate',
    allSaved: 'Tutte le modifiche sono salvate',
    discard: 'Annulla modifiche',
    searchList: 'Cerca nell’elenco',
    noMatch: 'Nessun risultato.',
    secret: {
      saved: 'Salvato',
      notSet: 'Non impostato',
      removing: 'Verrà rimosso al salvataggio',
      pastePlaceholder: 'Incollalo qui',
      replacePlaceholder: 'Incolla il nuovo valore',
      replace: 'Sostituisci',
      cancelReplace: 'Mantieni quello salvato',
      remove: 'Rimuovi',
      keep: 'Mantienilo',
      removeHint: 'Salva per rimuoverlo, oppure scegli «Mantienilo».',
      show: 'Mostra chiave',
      hide: 'Nascondi chiave',
      savedHidden: 'Chiave salvata, nascosta',
    },
  },
}

export default common
