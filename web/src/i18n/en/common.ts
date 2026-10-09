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
    not_found: 'This item no longer exists. The list has been refreshed.',
    not_set:
      'Nothing is saved: it was removed, or it cannot be decrypted with POLYFIN_SECRET_KEY. Reload the page.',
    not_revealable: 'This connection holds no key to show.',
    unknown_code: 'No device is waiting with this code. It may have expired or been mistyped.',
    quick_connect_disabled: 'Quick Connect is turned off on this server.',
    name_taken: 'This name is already used by another user.',
    last_administrator:
      'This is the last enabled administrator: it cannot be deleted, demoted or disabled.',
    invalid_server_name: 'The server name must be 1 to 64 characters long.',
    invalid_language: 'Choose the server language from the list.',
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
    invalid_max_conversions: 'Video conversions at once must be a whole number from 0 to 32.',
    invalid_max_conversion_height: 'Choose the maximum quality of converted video from the list.',
    invalid_encoder_preset: 'Choose an encoding speed from the list.',
    invalid_video_quality: 'Video quality must be 0, or a whole number from 1 to 51.',
    invalid_hardware_acceleration: 'Choose a graphics card from the list.',
    invalid_hardware_decoding_codecs:
      'This list of formats read on the graphics card is not supported. Reload the page.',
    invalid_tone_mapping_algorithm: 'Choose a tone mapping method from the list.',
    invalid_tone_mapping_peak:
      'Peak brightness must be 0, or a whole number of nits from 100 to 10,000.',
    invalid_tone_mapping_desat: 'Highlight desaturation must be a number from 0 to 10.',
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
      'Enter a guide address starting with https:// or http://, of at most 4,096 characters.',
    invalid_app: 'The app name must be 1 to 64 characters long, without special characters.',
    invalid_trickplay_interval:
      'The time between two thumbnails must be a whole number of seconds from 5 to 60.',
    invalid_trickplay_width: 'Choose the thumbnail width from the list.',
    invalid_thumbnail_storage_gb: 'The space for images must be a whole number of GB from 1 to 50.',
    invalid_recording_padding:
      'Minutes before and after a recording must be whole numbers from 0 to 60.',
    invalid_recording_retention_days:
      'Days to keep recordings must be a whole number from 0 to 3,650.',
    invalid_quality_group: 'Choose the quality group from the list.',
    invalid_live_tv_refresh_hours:
      'The refresh interval must be a whole number of hours from 1 to 168.',
    invalid_custom_css: 'The custom CSS must take at most 2 MB.',
    invalid_custom_js: 'The custom JavaScript must take at most 2 MB.',
    invalid_login_disclaimer: 'The sign-in message must take at most 8 KB.',
    invalid_publicmetadb_key:
      'PublicMetaDB did not accept this key. Copy it again from your PublicMetaDB account. Nothing was saved.',
    publicmetadb_unreachable:
      'PublicMetaDB could not be reached to check the key. Nothing was saved: try again later.',
    invalid_theintrodb_key:
      'TheIntroDB did not accept this key. Copy it again from your TheIntroDB account. Nothing was saved.',
    theintrodb_unreachable:
      'TheIntroDB could not be reached to check the key, or limits requests for now. Nothing was saved: try again later.',
    invalid_segment_order:
      'The order of the skip marker sources must list each source once. Reload the page.',
    invalid_trakt_app:
      'The Trakt client ID and secret must be at most 256 characters, without spaces or special characters. Copy them again from your Trakt app.',
    invalid_simkl_app:
      'The Simkl client ID must be at most 256 characters, without spaces or special characters. Copy it again from your Simkl app.',
    invalid_backup_hour: 'Choose the hour of the backups from the list.',
    invalid_backups_kept: 'The number of backups kept must be a whole number from 1 to 90.',
    invalid_cache_size: 'The disk space must be a whole number of GB from 1 to 2000.',
    invalid_vaapi_device: 'Choose the graphics card from the list.',
    invalid_recordings_folder:
      'The recordings folder must be an absolute path to a folder Polyfin can write to.',
    invalid_backup_folder:
      'The backups folder must be an absolute path to a folder Polyfin can write to.',
    invalid_collection_read_hour:
      'Choose the hour collections are read at, or Never, from the list.',
    invalid_remuxdb_url:
      'The RemuxDB address must be a web address starting with http:// or https://.',
    invalid_source_name: 'Source names must be 1 to 64 characters long.',
    invalid_source_address:
      'Enter an address starting with https:// or http://, and for an Xtream Codes account a username and a password.',
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
    jellyfin_unreachable:
      'Nothing answered at this address. Check it, and that Jellyfin is running.',
    not_jellyfin: 'A server answered at this address, but it is not Jellyfin. Check the address.',
    jellyfin_import_running:
      'An import from Jellyfin is already running. Wait for it to end, or stop it.',
    unknown_jellyfin_user:
      'A Jellyfin user is no longer on that server. Connect again to read its users.',
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
