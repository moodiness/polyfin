/**
 * Shared words: language, generic actions, API errors, time, server status, devices, Stremio
 * vocabulary and the design system (`ui`). Areas add keys here; they never change or remove one.
 */
const common = {
  documentTitle: 'Polyfin administration',
  header: {
    productName: 'Polyfin',
  },
  language: {
    label: 'Interface language',
    en: { short: 'EN', name: 'English' },
    fr: { short: 'FR', name: 'Français' },
    de: { short: 'DE', name: 'Deutsch' },
    es: { short: 'ES', name: 'Español' },
    it: { short: 'IT', name: 'Italiano' },
    'pt-BR': { short: 'PT', name: 'Português (Brasil)' },
  },
  common: {
    enterNumber: 'Enter a number.',
    loading: 'Loading…',
    save: 'Save',
    saving: 'Saving…',
    cancel: 'Cancel',
    retry: 'Try again',
    never: 'Never',
    // The colon after a label: French sets it off with a narrow no-break space.
    colon: ':',
    passwordRule: 'At least 8 characters.',
    nameRule: "1 to 64 characters: letters, digits, spaces and - _ ' . @ +",
    passwordMismatch: 'The two passwords do not match.',
    moveUp: (name: string) => `Move ${name} up`,
    moveDown: (name: string) => `Move ${name} down`,
    moved: (name: string, position: number, count: number) =>
      `${name} moved to position ${position} of ${count}.`,
  },
  errors: {
    generic: 'Something went wrong. Please try again.',
    network: 'The server could not be reached. Check that Polyfin is running.',
    unauthenticated: 'Your session has expired. Please sign in again.',
    forbidden: 'You are not allowed to do this.',
    cross_origin: 'The request was rejected because it did not come from this page.',
    invalid_setup_code: 'This setup code is not valid. Copy it again from the server log.',
    invalid_name: "Names must be 1 to 64 characters: letters, digits, spaces and - _ ' . @ +",
    invalid_password: 'Passwords must be at least 8 characters long.',
    setup_complete: 'Setup is already complete. Sign in instead.',
    too_many_attempts: 'Too many attempts. Wait a few minutes before trying again.',
    invalid_credentials: 'Incorrect name or password.',
    account_disabled: 'This account is disabled. Ask an administrator to enable it.',
    wrong_password: 'Your current password is incorrect.',
    invalid_code:
      'This code is not valid. Enter the 6-digit code your app shows now, or one of your recovery codes.',
    two_step_expired: 'The sign-in took too long. Enter your name and password again.',
    two_step_on: 'Two-step sign-in is already on.',
    two_step_off: 'Two-step sign-in is off for this account.',
    two_step_not_started: 'Start again: turn two-step sign-in on to get a new key.',
    two_step_required:
      'Administrators must use two-step sign-in on this server: it cannot be turned off.',
    two_step_own: 'Turn your own two-step sign-in off from My account.',
    not_found: 'This item no longer exists. The list has been refreshed.',
    not_set:
      'Nothing is saved: it was removed, or it cannot be decrypted with POLYFIN_SECRET_KEY. Reload the page.',
    not_revealable: 'This connection holds no key to show.',
    unknown_code: 'No device is waiting with this code. It may have expired or been mistyped.',
    quick_connect_disabled: 'Quick Connect is turned off on this server.',
    name_taken: 'This name is already used by another user.',
    invalid_invite_uses: 'Accounts an invite link may create must be a whole number from 1 to 100.',
    invalid_invite_expiry: 'Choose when the invite link expires from the list.',
    invalid_invite_model: 'This user no longer exists. Choose another one.',
    invite_unknown: 'This invite link does not work.',
    invite_used_up: 'This invite link is used up.',
    invite_expired: 'This invite link has expired.',
    invite_revoked: 'This invite link was revoked.',
    share_links_off:
      'You cannot make share links: an administrator turned them off for this server or for your account.',
    not_shareable: 'Only a movie or an episode you can see can be shared.',
    invalid_share_expiry: 'Choose when the link expires from the list.',
    invalid_share_viewers: 'Viewers at once must be a whole number from 1 to 5.',
    last_administrator:
      'This is the last enabled administrator: it cannot be deleted, demoted or disabled.',
    admin_cannot_join_group:
      'Administrators cannot join a group, as their permissions differ. Reload the page to see who is one now.',
    invalid_group: 'This group no longer exists. Reload the page and choose another one.',
    follows_group:
      'This user follows their group: change the group, or turn off Follow the group first.',
    invalid_copy_from: 'This user no longer exists. Choose another one.',
    invalid_server_name: 'The server name must be 1 to 64 characters long.',
    invalid_language: 'Choose a language from the list.',
    invalid_catalog_limit: 'Titles read per catalog must be a whole number from 100 to 20,000.',
    invalid_channel_limit:
      'Channels read per Live TV catalog must be a whole number from 100 to 50,000.',
    invalid_played_percent: 'The played threshold must be a whole number from 50 to 100.',
    invalid_resume_percent: 'The resume threshold must be a whole number from 0 to 50.',
    resume_not_below_played: 'The resume threshold must be lower than the played threshold.',
    invalid_version_list_minutes:
      'The time version lists are kept must be a whole number of minutes from 1 to 360.',
    invalid_catalog_refresh_minutes:
      'The catalog refresh time must be a whole number of minutes from 1 to 1,440.',
    invalid_analysis_timeout:
      'The time to analyze a version must be a whole number of seconds from 5 to 120.',
    invalid_version_attempts: 'Versions tried must be a whole number from 1 to 10.',
    invalid_version_order:
      'Choose at most four criteria to order versions, one of each kind. Reload the page.',
    invalid_version_codec_order:
      'The order of the video codecs must list each codec once. Reload the page.',
    invalid_version_source_order:
      'The order of sources must list sources of the server, each once. Reload the page.',
    invalid_hide_versions_larger:
      'The size above which versions are hidden must be 0, or a whole number of GB from 1 to 1,000.',
    invalid_hidden_version_codecs: 'This list of hidden codecs is not supported. Reload the page.',
    invalid_hidden_version_words:
      'Enter at most 50 words hiding versions, one a line, each at most 100 characters.',
    invalid_hidden_releases:
      'This list of hidden kinds of releases is not supported. Reload the page.',
    invalid_max_conversions: 'Video conversions at once must be a whole number from 0 to 32.',
    invalid_max_conversion_height: 'Choose the maximum quality of converted video from the list.',
    invalid_subtitle_ocr: 'Choose from the list when picture subtitles are turned into text.',
    invalid_subtitle_ocr_tracks:
      'Picture subtitle tracks read at once must be a whole number from 1 to 4.',
    invalid_default_language: 'Choose a default language from the list.',
    invalid_encoder_preset: 'Choose an encoding speed from the list.',
    invalid_video_quality: 'Video quality must be 0, or a whole number from 1 to 51.',
    invalid_hardware_acceleration: 'Choose a graphics card from the list.',
    invalid_hardware_decoding_codecs:
      'This list of formats read on the graphics card is not supported. Reload the page.',
    invalid_adaptive_quality: 'Choose when to offer several qualities from the list.',
    invalid_tone_mapping_algorithm: 'Choose a tone mapping method from the list.',
    invalid_tone_mapping_peak:
      'Peak brightness must be 0, or a whole number of nits from 100 to 10,000.',
    invalid_tone_mapping_desat: 'Highlight desaturation must be a number from 0 to 10.',
    invalid_processor_tone_mapping_height:
      'Choose the maximum quality of HDR converted by the processor from the list.',
    invalid_deinterlace_method: 'Choose a deinterlacing method from the list.',
    invalid_downmix_algorithm: 'Choose a stereo mix from the list.',
    invalid_downmix_boost: 'The volume when mixing to stereo must be a number from 0.5 to 3.',
    invalid_max_audio_channels: 'Choose the most audio channels from the list.',
    invalid_audio_bitrate_per_channel:
      'Audio bitrate per channel must be 0, or a whole number of kb/s from 32 to 320.',
    invalid_encoding_threads: 'Processor threads must be a whole number from 0 to 64.',
    invalid_ahead_seconds: 'Seconds prepared ahead must be a whole number from 30 to 600.',
    parental_control:
      'Parental control applies to this account: it keeps the server’s addons, which give the ratings it relies on.',
    invalid_parental_control: 'This parental control setting is not supported. Reload the page.',
    invalid_max_playbacks: 'Playbacks at once must be a whole number from 0 to 20.',
    invalid_max_bitrate: 'Choose the maximum quality from the list.',
    invalid_sync_play: 'Choose a watch together option from the list.',
    invalid_manifest_url:
      'Enter an addon manifest address starting with https://, http:// or stremio:// and ending in /manifest.json.',
    addon_exists: 'This addon is already installed here.',
    addon_unreachable:
      'The addon could not be reached. Check the address and that the addon is online, then try again.',
    invalid_manifest: 'This address did not return a Stremio addon manifest.',
    private_network:
      'This addon is on a local network address. Only administrators can install such addons.',
    invalid_order: 'The addon list changed in the meantime. It has been refreshed: try again.',
    invalid_library:
      'A library refers to a catalog that no longer exists or cannot be browsed. Remove the libraries marked as no longer available, then save again.',
    invalid_library_name: 'Library names must be 1 to 64 characters long.',
    invalid_library_genre:
      'A library’s genre is not offered by its catalog. Choose another one from the list, then save again.',
    invalid_library_max_items:
      'A library’s maximum titles must be a whole number from 1 to 20,000, or empty.',
    invalid_smart_list_name: 'A smart list’s name must be 1 to 64 characters long.',
    invalid_smart_list_shown_as: 'Choose whether the list shows as a library or as a collection.',
    invalid_smart_list_catalogs: 'Choose from 1 to 50 catalogs for the list to draw from.',
    invalid_smart_list_rules:
      'A rule is incomplete: give each one its value, and each year or rating at least one bound.',
    invalid_smart_list_sort: 'Choose how the list is sorted.',
    invalid_smart_list_max_items:
      'A smart list’s maximum titles must be a whole number from 1 to 20,000, or empty.',
    too_many_smart_lists: 'There are already 100 smart lists here. Delete one to make another.',
    invalid_image: 'Choose a JPEG, PNG or WebP image of at most 10 MB.',
    invalid_image_url: 'Enter an image address starting with https:// or http://.',
    image_unreachable:
      'No image could be downloaded from this address. Check that it opens an image, then try again.',
    image_private_network:
      'This image is on a local network address. Only administrators can use such addresses.',
    invalid_login_attempts:
      'Wrong passwords before an account is blocked must be 0, or a whole number from 3 to 20.',
    invalid_inactive_device_days:
      'Days before unused devices are signed out must be a whole number from 0 to 365.',
    invalid_https_mode: 'Serve HTTPS must be Off, Certificate files or Let’s Encrypt.',
    invalid_https_port: 'The HTTPS port must be a whole number from 1 to 65535.',
    invalid_https_certificate:
      'The certificate must be an absolute path in the container, such as /certs/fullchain.pem.',
    invalid_https_private_key:
      'The private key must be an absolute path in the container, such as /certs/privkey.pem.',
    https_certificate_missing: 'There is no certificate file at this path in the container.',
    https_private_key_missing: 'There is no private key file at this path in the container.',
    https_certificate_unreadable:
      'Polyfin cannot read this certificate file, or it holds no PEM certificate. Check that user 65532 may read it.',
    https_private_key_unreadable:
      'Polyfin cannot read this private key file, or it holds no RSA or ECDSA key in PEM. Check that user 65532 may read it.',
    https_files_mismatch: 'The private key does not belong to the certificate.',
    invalid_acme_hosts:
      'Host names must be public names such as media.example.org, one a line, without wildcards or IP addresses, 100 at most, and at least one for Let’s Encrypt.',
    invalid_acme_email: 'The email must be an email address, or empty.',
    acme_terms_required: 'Agree to Let’s Encrypt’s terms of service to use it.',
    invalid_acme_challenge: 'The challenge must be HTTP-01 or TLS-ALPN-01.',
    invalid_known_proxies:
      'Known proxies must be IP addresses, networks such as 172.18.0.0/16 or host names such as traefik, one a line, 64 at most.',
    personal_addons_disabled:
      'Your own addons are turned off by an administrator. They are kept, but you cannot add, refresh or turn on any.',
    invalid_hidden_libraries: 'The library list changed in the meantime. Reload the page.',
    invalid_blocked_genres:
      'Genres must be 1 to 100 characters long, and at most 100 can be blocked.',
    invalid_access_schedules:
      'Hours run from 00:00 to 24:00, and each end must come after its start (50 rows at most).',
    outside_allowed_hours:
      'This account cannot be used at this time. Try again during its allowed hours.',
    invalid_guide_url:
      'Enter a guide address starting with https:// or http://, of at most 4,096 characters, or the absolute path of a file on the server.',
    invalid_app: 'The app name must be 1 to 64 characters long, without special characters.',
    invalid_trickplay_interval:
      'The time between two thumbnails must be a whole number of seconds from 5 to 60.',
    invalid_trickplay_width: 'Choose the thumbnail width from the list.',
    invalid_thumbnail_storage_gb: 'The space for images must be a whole number of GB from 1 to 50.',
    invalid_recording_padding:
      'Minutes before and after a recording must be whole numbers from 0 to 60.',
    invalid_recording_retention_days:
      'Days to keep recordings must be a whole number from 0 to 3,650.',
    invalid_convert_recordings_hours:
      'Choose two different hours from the lists to convert recordings between.',
    invalid_quality_group: 'Choose the quality group from the list.',
    invalid_live_tv_refresh_hours:
      'The refresh interval must be a whole number of hours from 1 to 168.',
    invalid_local_scan_hours: 'The scan interval must be a whole number of hours from 0 to 168.',
    invalid_custom_css: 'The custom CSS must take at most 2 MB.',
    invalid_custom_js: 'The custom JavaScript must take at most 2 MB.',
    invalid_web_theme: 'Choose LumaaGlaass, None or Custom.',
    theme_copy_running: 'A copy of the theme is being made already. Try again in a moment.',
    theme_copy_nothing: 'The saved Custom texts load nothing from elsewhere.',
    theme_copy_failed: 'No file could be saved, so no copy was made.',
    custom_texts_changed: 'The Custom texts changed while the copy was made. Try again.',
    invalid_login_disclaimer: 'The sign-in message must take at most 8 KB.',
    invalid_publicmetadb_key:
      'PublicMetaDB did not accept this key. Copy it again from your PublicMetaDB account. Nothing was saved.',
    publicmetadb_unreachable:
      'PublicMetaDB could not be reached to check the key. Nothing was saved: try again later.',
    invalid_theintrodb_key:
      'TheIntroDB did not accept this key. Copy it again from your TheIntroDB account. Nothing was saved.',
    theintrodb_unreachable:
      'TheIntroDB could not be reached to check the key, or limits requests for now. Nothing was saved: try again later.',
    invalid_mdblist_ratings_key:
      'MDBList did not accept this key. Copy it again from your MDBList account. Nothing was saved.',
    mdblist_unreachable:
      'MDBList could not be reached to check the key. Nothing was saved: try again later.',
    invalid_tmdb_key:
      'TMDB did not accept this key. Copy the API key or the read access token again from your TMDB account. Nothing was saved.',
    tmdb_unreachable:
      'TMDB could not be reached to check the key. Nothing was saved: try again later.',
    invalid_critic_rating_shows: 'Choose from the list which score the critic rating shows.',
    invalid_measure_loudness: 'Choose from the list which songs have their loudness measured.',
    invalid_segment_order:
      'The order of the skip marker sources must list each source once. Reload the page.',
    invalid_trakt_app:
      'The Trakt client ID and secret must be at most 256 characters, without spaces or special characters. Copy them again from your Trakt app.',
    invalid_simkl_app:
      'The Simkl client ID must be at most 256 characters, without spaces or special characters. Copy it again from your Simkl app.',
    invalid_backup_hour: 'Choose the hour of the backups from the list.',
    invalid_backups_kept: 'The number of backups kept must be a whole number from 1 to 90.',
    invalid_torrent_route:
      'Torrent streams need one route alone: a network interface name (Linux only), a SOCKS5 proxy as host:port, or a WireGuard configuration.',
    invalid_wireguard_config: 'The WireGuard configuration is not valid.',
    invalid_torrent_limit: 'The limits must be whole numbers of Mbps from 0 to 10000.',
    invalid_torrent_trackers:
      'Enter at most 30 trackers, each an HTTP, HTTPS or UDP announce URL, UDP with its port, of at most 500 characters.',
    invalid_use_source_proxy: 'The source proxy can only be chosen for a server Stremio addon.',
    invalid_source_proxy:
      'The proxy must be a SOCKS5 or HTTP URL with a host and port, such as socks5h://warp:1080.',
    invalid_cache_size: 'The disk space must be a whole number of GB from 1 to 2000.',
    invalid_vaapi_device: 'Choose the graphics card from the list.',
    invalid_recordings_folder:
      'The recordings folder must be an absolute path to a folder Polyfin can write to.',
    invalid_backup_folder:
      'The backups folder must be an absolute path to a folder Polyfin can write to.',
    invalid_conversion_folder:
      'The conversion folder must be an absolute path to a folder Polyfin can write to.',
    invalid_reading_folder:
      'The folder for files being read must be an absolute path to a folder Polyfin can write to.',
    invalid_cache_folder:
      'The cache folder must be an absolute path to a folder Polyfin can write to.',
    too_many_destinations: 'There are already 10 backup destinations: delete one first.',
    too_many_home_rows: 'There are already 20 home rows: delete one first.',
    invalid_home_row:
      'Give the row a name of at most 100 characters, and choose its catalog or its collection.',
    invalid_destination_kind: 'Choose the kind of destination from the list.',
    invalid_destination_address:
      'Enter the address as asked: an http:// or https:// address without a path for S3 storage, a host name or IP address for an SFTP server, the address of a folder for a WebDAV server.',
    invalid_bucket:
      'Enter the bucket’s name: 3 to 63 lowercase letters, digits, dots, hyphens or underscores.',
    invalid_destination_folder:
      'The folder must not go up with “..”, nor hold control characters, and is at most 512 bytes.',
    invalid_region: 'The region is lowercase letters, digits and hyphens, such as eu-west-3.',
    invalid_port: 'The port is a whole number from 1 to 65535.',
    invalid_destination_user: 'Enter the user, or the access key for S3 storage.',
    invalid_destination_secret:
      'Enter the password, the secret key for S3 storage, or for an SFTP server a private key.',
    invalid_private_key:
      'The private key must be an OpenSSH or PEM private key without a passphrase.',
    backups_off: 'Backups are off: turn them on above first.',
    pg_restore_missing:
      'pg_restore was not found on the server: install PostgreSQL 18’s client tools, which the Docker image includes.',
    backup_not_found: 'This backup is no longer there: choose another one.',
    not_a_backup: 'This file is not a backup of Polyfin’s database.',
    newer_backup: 'This backup was made by a newer Polyfin: update Polyfin first.',
    changed_backup:
      'This backup was made by a Polyfin whose database migrations differ from this one’s.',
    restore_expired: 'The backup chosen is no longer ready: choose it again.',
    unreadable_secrets:
      'This backup holds keys that POLYFIN_SECRET_KEY cannot decrypt: confirm with Restore anyway.',
    restoring: 'A restore is already under way.',
    wrong_server_name: 'Type the server’s name exactly as shown.',
    too_large: 'The file is too large.',
    destination_failed: 'The destination could not be read.',
    restore_failed: 'The restore could not start: nothing was changed.',
    invalid_collection_read_hour:
      'Choose the hour collections are read at, or Never, from the list.',
    invalid_collection_library_apps:
      'Choose the apps that show collection libraries by collection from the list.',
    invalid_remuxdb_url:
      'The RemuxDB address must be a web address starting with http:// or https://.',
    invalid_update_tool_url:
      'The update tool address must be a web address starting with http:// or https://, without a user name or a fragment.',
    invalid_update_tool_token:
      'The update tool token holds no spaces, and 1024 characters at most.',
    restart_unconfirmed:
      'Polyfin cannot tell whether something starts it again: confirm the warning first.',
    no_update: 'Polyfin Desktop offers no such update now.',
    not_desktop: 'Polyfin does not run in Polyfin Desktop.',
    no_update_tool: 'No update tool is set under Settings › General.',
    update_tool_timeout:
      'The update tool did not answer within 50 seconds: it may still be updating.',
    update_tool_unreachable: 'The update tool could not be reached.',
    invalid_public_address:
      'The public address must be a web address starting with http:// or https://, without a query.',
    invalid_local_address:
      'The address on the local network must be a web address starting with http:// or https://, such as http://192.168.1.10:8096, without a query.',
    invalid_dlna_name: 'The name on the network can be at most 256 bytes long.',
    invalid_dlna_user: 'That user no longer exists. Choose another, or No one.',
    invalid_dlna_profile: 'This is not a DLNA profile in the XML format of Jellyfin’s DLNA plugin.',
    invalid_smtp_host:
      'Enter the SMTP server’s host name, such as smtp.example.org, without spaces.',
    invalid_smtp_port: 'The SMTP port must be a whole number from 1 to 65535.',
    invalid_smtp_security: 'Choose how the SMTP connection is encrypted from the list.',
    invalid_smtp_account: 'The SMTP user and password are up to 256 characters, on one line.',
    invalid_smtp_sender:
      'Enter the sender’s address alone, such as polyfin@example.org, and a name of up to 128 characters.',
    invalid_weekly_summary_day: 'Choose the day of the weekly summary from the list.',
    invalid_weekly_summary_hour: 'Choose the hour of the weekly summary from the list.',
    invalid_import_every_minutes: 'Choose how often connected accounts are imported from the list.',
    invalid_target_kind: 'Choose a kind of target from the list.',
    invalid_target_name: 'Target names must be 1 to 64 characters long.',
    invalid_target_address: 'Enter a web address starting with https:// or http://.',
    private_target_address:
      'This address is on a local network: only an administrator’s targets may reach one.',
    invalid_email_address: 'Enter an email address, such as sam@example.org.',
    email_unavailable:
      'Email needs an SMTP server: an administrator sets it under Settings › Notifications.',
    invalid_topic: 'An ntfy topic is 1 to 64 letters, digits, dashes and underscores.',
    invalid_chat:
      'Enter the chat’s number, such as -1001234567890, or a public channel’s name, such as @news_example.',
    invalid_token:
      'Enter the token: up to 256 characters, without spaces. A Telegram bot’s looks like 123456:ABC-DEF.',
    invalid_user_key: 'A Pushover user key is letters and digits only.',
    invalid_events: 'One of the events chosen is not available for this target.',
    invalid_reminder_settings: 'Choose 1, 5, 10, 15, 30 or 60 minutes.',
    too_many_targets: 'There are 20 targets already. Delete one to add another.',
    target_unreadable:
      'This target’s address or token cannot be decrypted with POLYFIN_SECRET_KEY. Enter it again.',
    invalid_source_name: 'Source names must be 1 to 64 characters long.',
    invalid_source_address:
      'Enter an address starting with https:// or http://, or for a playlist the absolute path of a file on the server, and for an Xtream Codes account a username and a password.',
    file_missing:
      'There is no file at this path on the server. With Docker, give its path in the container, in a mounted volume.',
    file_unreadable:
      'Polyfin cannot read this file: check that it is a file, not a folder, and that Polyfin may read it.',
    local_file_refused: 'Only administrators can use a file of the server.',
    invalid_user_agent:
      'A user agent is at most 512 characters, without line breaks or control characters.',
    tuner_added: 'This tuner is already a source of the server.',
    invalid_folder_name: 'Folder names must be 1 to 64 characters long.',
    invalid_folder_path:
      'Enter the folder’s full path where Polyfin runs, other than the system’s root: in its container, starting with /; on Windows, with its drive, such as D:\\Movies, or its share, such as \\\\nas\\media\\Movies.',
    invalid_share_address:
      'Enter an address such as smb://server/share/folder, https://server/folder, s3://bucket/prefix, sftp://server/folder or rclone://server:port/folder, without a user or password in it: they have fields of their own.',
    invalid_share_user: 'Users are at most 256 characters long, passwords at most 1,024.',
    invalid_share_endpoint:
      'Enter the endpoint as an http:// or https:// address, such as https://s3.example.org, and a region of at most 64 characters without spaces.',
    invalid_share_key:
      'Paste a whole OpenSSH or PEM private key, and the passphrase that opens it when it has one.',
    invalid_rclone_remote:
      'Enter the name of one of rclone’s remotes, without a colon or a slash, at most 128 characters.',
    invalid_poll_minutes:
      'Enter a number of minutes from 0 to 1,440, 0 to look for changes only at scans.',
    invalid_folder_kind: 'Choose whether the folder holds movies or shows.',
    invalid_imdb_id: 'An IMDb identifier is tt followed by digits, such as tt0063350.',
    invalid_episode:
      'Write the episode as a file’s name gives it, such as S01E05, or S01E05-E06 for two.',
    title_search_failed: 'The metadata addons could not be searched. Try again in a moment.',
    invalid_channel_list:
      'This address did not return a channel list. Check it, and that the account is still valid.',
    channel_list_too_large: 'This channel list is too large (100 MB or 100,000 channels at most).',
    iptv_login_refused: 'The IPTV server refused this username or password.',
    invalid_message: 'Write a message of 1 to 500 characters.',
    not_controllable:
      'This app does not accept remote control: it cannot be stopped or sent a message from here.',
    remote_control_refused:
      'Your account may not control other users’ apps. Another administrator can give it this permission on the Users page.',
    too_soon: 'This addon was checked less than a minute ago. Try again in a moment.',
    invalid_limit: 'This list cannot show that many entries.',
    addon_kind_changed:
      'This address now leads to another kind of addon (a music addon instead of a video one, or the reverse). Install it as a new addon instead.',
    invalid_addon_settings:
      'The addon refused these settings: a value is not one of its choices, too long, or out of range. Check the fields and try again.',
    invalid_options: 'These import options are not accepted. Reload the page and try again.',
    invalid_category_name: 'Category names must be 1 to 64 characters long.',
    category_not_custom: 'Only your own categories can be deleted.',
    invalid_channel_name: 'Channel names must be 1 to 100 characters long.',
    invalid_logo:
      'Enter a logo address starting with https:// or http://, of at most 4,096 characters.',
    invalid_description: 'Descriptions must be at most 2,000 characters long.',
    invalid_category: 'This category no longer exists. Reload the page.',
    invalid_number: 'Enter a whole number from 1 to 99,999, or leave the number empty.',
    invalid_move: 'This channel can only move within its category. Reload the page.',
    invalid_bulk: 'Choose channels, or a keyword of at least 2 characters.',
    invalid_streams: 'The streams changed in the meantime. Reload the channel.',
    invalid_stream_url:
      'Enter a stream address starting with https:// or http://, of at most 4,096 characters.',
    stream_not_custom: 'Only the streams you added can be removed.',
    too_many_guides: 'A catalog takes at most 10 guides.',
    invalid_guide: 'This guide no longer exists. Reload the page.',
    invalid_mode: 'This mapping mode is not supported. Reload the page.',
    invalid_mapping: 'This guide channel is no longer in the catalog’s guides. Search again.',
    invalid_jellyfin_address:
      'Enter the address of the Jellyfin server, such as http://192.168.1.10:8096.',
    jellyfin_key_refused:
      'Jellyfin refused this API key. Create one in its dashboard, under API Keys, and paste it again.',
    jellyfin_key_limited:
      'This key or account may not list the server’s users. Use an API key from its dashboard, or an administrator’s account.',
    jellyfin_key_owner_only:
      'This key reads only its owner’s watch data. Type the other users’ passwords on the server, or untick “Import watch data” for them.',
    jellyfin_sign_in_refused: 'Jellyfin refused this name or password.',
    jellyfin_sign_in_forbidden:
      'Jellyfin does not let this account sign in: it may be disabled, or outside its allowed hours.',
    jellyfin_password_refused:
      'Jellyfin refused this password. Leave it empty if the account has none.',
    jellyfin_other_user:
      'This password opened another user’s account on the server. Connect with an API key from its dashboard instead.',
    jellyfin_unreachable:
      'Nothing answered at this address. Check it, and that Jellyfin is running.',
    not_jellyfin: 'A server answered at this address, but it is not Jellyfin. Check the address.',
    jellyfin_import_running: 'An import is already running. Wait for it to end, or stop it.',
    unknown_jellyfin_user:
      'A Jellyfin user is no longer on that server. Connect again to read its users.',
    emby_user_key:
      'This key is a user’s own, and Emby does not tell whose it is. Create an API key in Emby’s settings, or connect with a user account.',
    server_import_running:
      'Another import is running on this server or into this account. Try again in a few minutes.',
    server_imports_disabled:
      'Importing your watch history from another server or app is turned off on this server.',
    letterboxd_export_invalid:
      'This file is not a Letterboxd export: choose the ZIP file Letterboxd gives, as it is. It holds watched.csv or diary.csv.',
    letterboxd_export_too_large:
      'This file is larger than 20 MB, or names more than 50,000 movies: choose the ZIP file Letterboxd gives.',
    invalid_nuvio_address:
      'Enter the address of the Nuvio backend, such as https://api.nuvio.tv, or leave it empty for Nuvio’s own.',
    nuvio_sign_in_refused: 'Nuvio refused this email or password.',
    nuvio_unreachable: 'The Nuvio backend did not answer. Check its address, and try again later.',
    not_nuvio:
      'Something answered at this address, but not a Nuvio backend. Check the address, or leave it empty for Nuvio’s own.',
    nuvio_rate_limited: 'Nuvio asks to wait before signing in again. Try again in a few minutes.',
    nuvio_backup_unsupported:
      'Nuvio answered its account data in a form this version of Polyfin does not read. Polyfin reads version 1 of Nuvio’s account backup.',
    stremio_sign_in_refused: 'Stremio refused this email or password.',
    stremio_pin_refused: 'Stremio refused this PIN.',
    stremio_key_refused:
      'Stremio refused this auth key. Copy it again from Stremio’s web app, signed in to your account.',
    stremio_link_expired: 'The link code expired, or it was used already. Ask for another.',
    stremio_unreachable: 'Stremio did not answer. Try again later.',
    stremio_unexpected:
      'Stremio answered in a way this version of Polyfin does not read. Try again later.',
    app_import_expired:
      'What was read of the account is no longer kept, after a few minutes. Sign in again.',
    app_import_running: 'An import into this account is running. Wait for it to end.',
    app_not_connected: 'This app is not connected to the account any longer.',
    app_connection_stopped:
      'The app refused the sign-in Polyfin kept. Sign in again, with Stay connected ticked.',
    invalid_ends_on: 'Choose the date the account ends, or Never.',
    administrator_cannot_end:
      'An administrator’s account never ends. Make them a member first to give it an end date.',
    invalid_invite_account_end: 'Choose when the accounts end from the list.',
  },
  status: {
    title: 'Server status',
    description: 'Health of this Polyfin server at a glance.',
    version: 'Version',
    serverId: 'Server ID',
    database: 'Database',
    databaseReady: 'Ready',
    databaseUnavailable: 'Unavailable',
    loading: 'Loading server status…',
    errorTitle: 'Server unreachable',
    errorBody:
      'The administration API did not respond. Check that Polyfin is running, then try again.',
    staleWarning: 'The latest refresh failed. The details below may be out of date.',
    retry: 'Try again',
    retrying: 'Retrying…',
    autoRefresh: 'Refreshes automatically every 10 seconds.',
    updatedAt: (time: string) => `Last updated at ${time}`,
  },
  devices: {
    empty: 'No device is signed in.',
    lastActivity: 'Last activity',
    address: 'Address',
    signOut: 'Sign out',
    signOutConfirm: (device: string) => `Sign out ${device}? The app will need to sign in again.`,
    signedOut: (device: string) => `${device} has been signed out.`,
  },
  stremioTypes: {
    movie: 'Movies',
    series: 'Series',
    channel: 'Channels',
    other: 'Other videos',
    tv: 'TV',
    track: 'Tracks',
    album: 'Albums',
    artist: 'Artists',
    playlist: 'Playlists',
  },
  stremioResources: {
    catalog: 'Catalogs',
    meta: 'Details',
    stream: 'Streams',
    subtitles: 'Subtitles',
    addon_catalog: 'Addon catalogs',
    search: 'Search',
    isrc: 'Track matching',
    resolve: 'Playback links',
    settings: 'Settings',
  },
  forbidden: {
    title: 'Not allowed',
    description: 'Only administrators can open this page.',
    home: 'Back to the home page',
  },
  time: {
    justNow: 'just now',
  },
  /** The words of the design system in `src/ui/`. */
  ui: {
    on: 'On',
    off: 'Off',
    show: 'Show',
    hide: 'Hide',
    close: 'Close',
    dismiss: 'Dismiss',
    notifications: 'Notifications',
    unsaved: 'Unsaved changes',
    allSaved: 'All changes saved',
    discard: 'Discard changes',
    searchList: 'Search the list',
    noMatch: 'Nothing matches.',
    secret: {
      saved: 'Saved',
      notSet: 'Not set',
      removing: 'Removed when you save',
      pastePlaceholder: 'Paste it here',
      replacePlaceholder: 'Paste the new one',
      replace: 'Replace',
      cancelReplace: 'Keep the saved one',
      remove: 'Remove',
      keep: 'Keep it',
      removeHint: 'Save to remove it, or choose Keep it.',
      show: 'Show key',
      hide: 'Hide key',
      savedHidden: 'Saved key, hidden',
    },
  },
}

export default common
