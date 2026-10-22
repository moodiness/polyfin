-- The AIOStreams look of the web player, as it is now (see
-- internal/accounts/aiostreams_theme.css), becomes the default custom CSS:
-- on new servers, and on servers that still hold the look as the last
-- migration setting it left it, without a custom script. A server that
-- changed either keeps both as they are.
ALTER TABLE settings ALTER COLUMN custom_css SET DEFAULT $css$/* AIOStreams look for jellyfin-web: the near-black pages, white pill buttons,
   Inter type, rounded cards with a faint ring and quiet grey text of
   AIOStreams' own Jellyfin client, and Plex-style subtitles.
   Edit or clear it under Settings > Web player > Custom CSS. */
@import url('https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700;800&display=swap');

:root {
  --aio-bg: #070707;
  --aio-paper: #0b0b0b;
  --aio-raised: #111111;
  --aio-field: #161616;
  --aio-hover: rgba(255, 255, 255, 0.06);
  --aio-hover-2: rgba(255, 255, 255, 0.1);
  --aio-ring: rgba(255, 255, 255, 0.05);
  --aio-border: rgba(255, 255, 255, 0.1);
  --aio-text: #e6e6e6;
  --aio-muted: rgba(255, 255, 255, 0.45);
  --aio-muted-2: rgba(255, 255, 255, 0.62);
  --aio-brand: #6152df;
  --aio-brand-soft: #9f92ff;
  --aio-radius: 0.5rem;
  --aio-card: 0.75rem;
  --aio-font: 'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
}

/* Pages */
/* jellyfin-web's palette: near-black, white as the accent in place of its
   light blue, AIOStreams' greys. */
:root, :root[data-theme], [data-mui-color-scheme] {
  --jf-palette-primary-main: #ffffff !important;
  --jf-palette-primary-mainChannel: 255 255 255 !important;
  --jf-palette-primary-dark: #d4d4d4 !important;
  --jf-palette-primary-contrastText: #000000 !important;
  --jf-palette-secondary-main: #9f92ff !important;
  --jf-palette-secondary-contrastText: #000000 !important;
  --jf-palette-background-default: #070707 !important;
  --jf-palette-background-defaultImage: none !important;
  --jf-palette-background-paper: #0b0b0b !important;
  --jf-palette-background-paperChannel: 11 11 11 !important;
  --jf-palette-text-primary: #e6e6e6 !important;
  --jf-palette-text-secondary: rgba(255, 255, 255, 0.62) !important;
  --jf-palette-divider: rgba(255, 255, 255, 0.08) !important;
  --jf-palette-action-hover: rgba(255, 255, 255, 0.06) !important;
  --jf-palette-AppBar-defaultBg: #070707 !important;
  --jf-palette-AppBar-transparentBg: rgba(7, 7, 7, 0) !important;
  --jf-palette-FilledInput-bg: #161616 !important;
  --jf-palette-SnackbarContent-bg: #111111 !important;
  --jf-palette-SnackbarContent-color: #e6e6e6 !important;
}
html, body, .backgroundContainer, .skinBody, .dashboardDocument { background-color: var(--aio-bg) !important; }
/* The backdrop of a title's page fades into the page, as AIOStreams' hero
   does: dark at the bottom and on the left, where the text is. */
.backgroundContainer.withBackdrop {
  opacity: 1 !important;
  background: linear-gradient(to top, #070707 12%, rgba(7, 7, 7, 0.55) 55%, rgba(7, 7, 7, 0.2) 100%),
    linear-gradient(to right, rgba(7, 7, 7, 0.9) 0%, rgba(7, 7, 7, 0.35) 45%, rgba(7, 7, 7, 0) 70%) !important;
}
.noBackdropTransparency .detailPageWrapperContainer { background: var(--aio-bg) !important; }
.detailRibbon { background: transparent !important; }
body { color: var(--aio-text); font-family: var(--aio-font) !important; }
body *:not(.material-icons):not([class*="material-icons"]):not(.md-icon) { font-family: var(--aio-font) !important; }
h1, h2, h3, .sectionTitle, .itemName, .pageTitle { color: #fff; letter-spacing: -0.02em; }
h1, .itemName.infoText, .pageTitle { font-weight: 800 !important; }
.sectionTitle, h2.sectionTitle { font-size: 1.45em !important; font-weight: 700 !important; }
.sectionTitle-cards { padding-bottom: 0.35em; }
.secondaryText, .fieldDescription, .cardText-secondary, .listItemBodyText.secondary,
.itemMiscInfo, .mediaInfoItem, .itemMiscInfo-secondary, .overview-controls { color: var(--aio-muted-2) !important; }
.overview, .itemOverview, .detail-clamp-text { color: rgba(255, 255, 255, 0.78) !important; line-height: 1.6; }
a { color: inherit; }

/* Header: transparent over the page, blurred once scrolled */
.skinHeader, .skinHeader.semiTransparent, .skinHeader-withBackground {
  background: linear-gradient(to bottom, rgba(7, 7, 7, 0.92), rgba(7, 7, 7, 0)) !important;
  box-shadow: none !important;
  border: none !important;
}
.skinHeader.skinHeader-blurred, .skinHeader.headroom--not-top {
  background: rgba(7, 7, 7, 0.82) !important;
  backdrop-filter: blur(18px) saturate(140%);
  -webkit-backdrop-filter: blur(18px) saturate(140%);
}
.headerTabs, .headerTabs.sectionTabs { background: transparent !important; }
.pageTitleWithLogo, .pageTitleWithDefaultLogo { filter: grayscale(1) brightness(2); }

/* Side menu: a dark rail */
.mainDrawer, .mainDrawer-scrollContainer, .MuiDrawer-paper {
  background: var(--aio-bg) !important;
  border-right: 1px solid var(--aio-ring) !important;
}
.navMenuOption { border-radius: 999px; margin: 0.15em 0.75em; padding-left: 1em !important; color: var(--aio-muted-2) !important; }
.navMenuOption:hover { background: var(--aio-hover) !important; color: #fff !important; }
.navMenuOption-selected, .navMenuOption-selected:hover { background: var(--aio-hover-2) !important; color: #fff !important; }
.navMenuOption-selected .navMenuOptionIcon { color: #fff !important; }
.sidebarHeader { color: var(--aio-muted) !important; font-size: 0.75em; text-transform: uppercase; letter-spacing: 0.08em; }
.sidebarDivider { background: var(--aio-ring) !important; }

/* Tabs: pills */
.emby-tabs-slider { gap: 0.25em; }
.emby-tab-button { color: var(--aio-muted-2) !important; border-radius: 999px !important; padding: 0.55em 1.1em !important; }
.emby-tab-button:hover { color: #fff !important; background: var(--aio-hover) !important; }
.emby-tab-button-active, .emby-tab-button-active.emby-button-tv { color: #000 !important; background: #fff !important; }
.emby-tab-button-active::after, .emby-tab-button::after { display: none !important; }

/* Cards: rounded art with a faint ring, plain text below */
.cardBox, .cardScalable, .cardImageContainer, .cardContent, .cardPadder,
.cardOverlayContainer, .blurhash-canvas, .cardImage, .coveredImage {
  border-radius: var(--aio-card) !important;
}
.cardBox, .visualCardBox { background: transparent !important; border: none !important; box-shadow: none !important; }
.cardScalable { overflow: hidden; box-shadow: 0 0 0 1px var(--aio-ring) !important; transition: transform 180ms ease, box-shadow 180ms ease; }
.cardImageContainer { background-color: var(--aio-raised) !important; }
.card:hover .cardScalable, .card:focus .cardScalable { transform: scale(1.025); box-shadow: 0 0 0 2px rgba(255, 255, 255, 0.9) !important; }
.card:focus .cardBox, .card.show-focus:focus .cardBox { border: none !important; }
.cardOverlayContainer { background: linear-gradient(to top, rgba(0, 0, 0, 0.75), rgba(0, 0, 0, 0.1)) !important; }
.cardText { color: var(--aio-text) !important; font-weight: 500; }
.cardText-first { color: #fff !important; }
.cardFooter { padding-top: 0.55em !important; }
.cardIndicators .countIndicator, .countIndicator, .playedIndicator, .indicator {
  background: rgba(0, 0, 0, 0.72) !important; color: #fff !important;
  border: 1px solid var(--aio-border); backdrop-filter: blur(8px);
}
.itemProgressBar { background: rgba(255, 255, 255, 0.2) !important; height: 0.25em !important; }
.itemProgressBarForeground { background-color: #fff !important; }
.defaultCardBackground, .defaultCardBackground1, .defaultCardBackground2,
.defaultCardBackground3, .defaultCardBackground4, .defaultCardBackground5 {
  background-color: var(--aio-raised) !important; color: var(--aio-muted) !important;
}

/* Buttons: white pills for the main action, dark pills for the rest */
.emby-button, .raised, .fab, .button-flat, .button-submit { border-radius: 999px !important; }
.raised, .button-alt, .emby-button.raised, .button-cancel {
  background: rgba(255, 255, 255, 0.08) !important; color: #fff !important;
  border: 1px solid var(--aio-border) !important; box-shadow: none !important;
}
.raised:hover, .button-alt:hover, .button-cancel:hover { background: rgba(255, 255, 255, 0.14) !important; }
.button-submit, .emby-button.button-submit, .btnPlay.detailButton, .detailButton-primary,
.btnResume, .fab, .emby-button[is="emby-button"].button-submit, .mainDetailButtons .btnPlay {
  background: #fff !important; color: #000 !important; border: none !important; font-weight: 600 !important;
}
.button-submit:hover, .button-submit:focus, .btnPlay.detailButton:hover, .fab:hover { background: #e6e6e6 !important; }
.btnPlay .detailButton-icon, .button-submit .material-icons { color: #000 !important; }
.button-link, a.button-link, .emby-button.button-link { color: #fff !important; text-decoration: underline; text-underline-offset: 3px; }
/* The header's selected entry and the user button: white, not light blue. */
.headerButton.selected, .headerTabs .emby-tab-button-active, .navMenuOption-selected, .headerSelectedButton { color: #fff !important; }
.headerUserButtonRound, .headerUserButton .headerButtonIcon { background-color: #1c1c1c !important; color: #fff !important; }
.mdl-spinner__layer, .mdl-spinner__circle { border-color: #fff !important; }
.MuiCircularProgress-root { color: #fff !important; }
.detailButton {
  border-radius: 999px !important; background: rgba(255, 255, 255, 0.08) !important;
  border: 1px solid var(--aio-border) !important; color: #fff !important;
}
.detailButton:hover { background: rgba(255, 255, 255, 0.16) !important; }
.detailButton-content { padding: 0.15em 0.4em; }
.paper-icon-button-light { border-radius: 999px !important; }
.paper-icon-button-light:hover:not(:disabled) { color: #fff !important; background: var(--aio-hover) !important; }

/* Fields */
.emby-input, .emby-select, .emby-textarea, .emby-select-withcolor, .searchfields-txtSearch {
  background: var(--aio-field) !important; color: var(--aio-text) !important;
  border: 1px solid var(--aio-border) !important; border-radius: var(--aio-radius) !important;
}
.emby-input:focus, .emby-select:focus, .emby-textarea:focus, .searchfields-txtSearch:focus {
  border-color: var(--aio-brand) !important; box-shadow: 0 0 0 3px rgba(97, 82, 223, 0.3) !important;
}
.searchfields-txtSearch { border-radius: 999px !important; padding-left: 1.2em !important; }
.inputLabelFocused, .selectLabelFocused, .textareaLabelFocused { color: var(--aio-brand-soft) !important; }
.emby-checkbox:checked + span + .checkboxOutline, .emby-checkbox:checked + .checkboxOutline {
  background-color: #fff !important; border-color: #fff !important;
}
.emby-checkbox:checked + span + .checkboxOutline .checkboxIcon, .emby-checkbox:checked + .checkboxOutline .checkboxIcon { color: #000 !important; }
.mdl-switch__input:checked + .mdl-switch__label + .mdl-switch__trackContainer > .mdl-switch__track,
.mdl-switch__input:checked + .mdl-switch__trackContainer > .mdl-switch__track { background: rgba(255, 255, 255, 0.55) !important; }
.mdl-switch__input:checked + .mdl-switch__label + .mdl-switch__trackContainer > .mdl-switch__thumb,
.mdl-switch__input:checked + .mdl-switch__trackContainer > .mdl-switch__thumb { background: #fff !important; }

/* Sliders (seek bar, volume) */
.mdl-slider-background-lower, .mdl-slider-lower { background-color: #fff !important; }
.mdl-slider-background-upper { background-color: rgba(255, 255, 255, 0.25) !important; }
.mdl-slider::-webkit-slider-thumb { background: #fff !important; }
.mdl-slider::-moz-range-thumb { background: #fff !important; }
.sliderBubble { background: var(--aio-raised) !important; border: 1px solid var(--aio-border); border-radius: var(--aio-radius); }

/* Lists, dialogs and menus */
.listItem { border-radius: var(--aio-radius); }
.listItem:hover, .listItem:focus, .actionSheetMenuItem:hover { background: var(--aio-hover) !important; }
.paperList, .detailSection .paperList, .collapseContent, .readOnlyContent {
  background: var(--aio-paper) !important; border: 1px solid var(--aio-ring); border-radius: var(--aio-card) !important;
}
.dialog, .formDialog, .actionSheet, .actionsheetMenu, .focuscontainer.dialog, .dialogContainer .dialog {
  background: var(--aio-paper) !important; color: var(--aio-text) !important;
  border: 1px solid var(--aio-border); border-radius: 1rem !important;
  box-shadow: 0 28px 70px -20px rgba(0, 0, 0, 0.95) !important;
}
.formDialogHeader, .formDialogFooter { background: var(--aio-paper) !important; border-color: var(--aio-ring) !important; }
.dialogBackdropOpened { background: rgba(0, 0, 0, 0.75) !important; }
.toast { background: var(--aio-raised) !important; border: 1px solid var(--aio-border); border-radius: 999px !important; color: var(--aio-text); }

/* Title pages */
.detailPagePrimaryContainer, .detailPageSecondaryContainer { background: transparent !important; }
.itemBackdrop::after { background: linear-gradient(to top, var(--aio-bg) 0%, rgba(7, 7, 7, 0) 70%) !important; }
.detailImageContainer .card .cardScalable { box-shadow: 0 20px 50px -12px rgba(0, 0, 0, 0.8), 0 0 0 1px var(--aio-ring) !important; }
.detailLogo { filter: drop-shadow(0 4px 18px rgba(0, 0, 0, 0.6)); }
.mediaInfoCriticRating, .starRatingContainer .material-icons, .starIcon { color: #f5c518 !important; }
.genres a, .itemGenres a, .itemExternalLinks a, .itemDetailsGroup a, .detailsGroupItem a { color: var(--aio-text) !important; }
.genres a:hover, .itemGenres a:hover { color: #fff !important; }
/* Phones: the backdrop above already shows the title's art, as AIOStreams'
   mobile pages do, so the image beside the name goes (a wide collection
   image showed there cut to a strip) and the text takes the full width. */
.layout-mobile .itemDetailPage .detailImageContainer { display: none !important; }
.layout-mobile .itemDetailPage .infoWrapper { padding-left: 0 !important; padding-right: 0 !important; }
.layout-mobile .itemDetailPage .detailRibbon { padding: 0.75rem 5% 0.25rem !important; }
.layout-mobile .itemDetailPage .nameContainer, .layout-mobile .itemDetailPage .itemMiscInfo { justify-content: center; text-align: center; }
.layout-mobile .itemDetailPage .mainDetailButtons { justify-content: center; }

/* Video player */
.videoOsdBottom, .osdControls { background: linear-gradient(to top, rgba(0, 0, 0, 0.9), rgba(0, 0, 0, 0)) !important; }
.upNextContainer, .skip-button, .skipIntro button, .btnSkipIntro {
  background: rgba(11, 11, 11, 0.92) !important; border: 1px solid var(--aio-border) !important;
  border-radius: 999px !important; color: #fff !important;
}

/* Dashboard and other MUI pages */
.MuiPaper-root { background-image: none !important; }

/* Scrollbars and selection */
::-webkit-scrollbar { width: 10px; height: 10px; }
::-webkit-scrollbar-track { background: var(--aio-bg); }
::-webkit-scrollbar-thumb { background: #363636; border-radius: 99px; }
::-webkit-scrollbar-thumb:hover { background: #484848; }
::selection { background: rgba(97, 82, 223, 0.45); }

/* Subtitles, as Plex shows them: white, semi-bold sans serif with a thin
   black outline and a soft shadow, no box behind, low on the picture.
   Styled ASS/SSA subtitles keep their own look. In portrait, a 16:9 picture
   is a band across the middle of the screen: they sit just above its bottom
   edge, not at the bottom of the screen, and their size follows its width. */
.videoSubtitles { bottom: 6% !important; padding: 0 6% !important; }
@media (orientation: portrait) {
  .videoSubtitles { bottom: max(2%, calc((100% - 56.25vw) / 2 + 0.6em)) !important; padding: 0 4% !important; }
}
.videoSubtitlesInner, .videoSubtitles .videoSubtitlesInner span {
  background: transparent !important;
  color: #fff !important;
  font-family: var(--aio-font) !important;
  font-weight: 600 !important;
  font-size: clamp(16px, min(3.8vh, 2.4vw), 46px) !important;
  line-height: 1.25 !important;
  text-shadow:
    -1px -1px 0 #000, 1px -1px 0 #000, -1px 1px 0 #000, 1px 1px 0 #000,
    0 0 2px #000, 0 2px 6px rgba(0, 0, 0, 0.85) !important;
}
@media (orientation: portrait) {
  .videoSubtitlesInner, .videoSubtitles .videoSubtitlesInner span { font-size: clamp(14px, 4.4vw, 24px) !important; }
}
/* Safari, and iOS, draw the subtitles themselves, at the bottom of the
   video element: sized as nuvio-bridge's player does. The element fills the
   screen inside the safe areas (the notch, the home bar), so that nothing
   sits under them; in portrait its height is the film's own shape and it is
   centred, so that subtitles sit on the picture, not in the black below it.
   jellyfin-web makes Safari's subtitle text 170% and pushes its box 15% to
   the right: undone, the size follows the screen's width, centred. */
.videoPlayerContainer .htmlvideoplayer {
  position: absolute !important; margin: 0 !important;
  top: env(safe-area-inset-top, 0px) !important; left: env(safe-area-inset-left, 0px) !important;
  right: auto !important; bottom: auto !important; transform: none;
  width: calc(100% - env(safe-area-inset-left, 0px) - env(safe-area-inset-right, 0px)) !important;
  height: calc(100% - env(safe-area-inset-top, 0px) - env(safe-area-inset-bottom, 0px)) !important;
  max-width: none !important; max-height: none !important; object-fit: contain;
}
@media (orientation: portrait) {
  .videoPlayerContainer .htmlvideoplayer {
    top: 50% !important; height: auto !important;
    max-height: calc(100% - env(safe-area-inset-top, 0px) - env(safe-area-inset-bottom, 0px)) !important;
    transform: translateY(-50%) !important;
  }
}
.htmlvideoplayer::-webkit-media-text-track-container { font-size: 100% !important; line-height: normal !important; }
.htmlvideoplayer::-webkit-media-text-track-display {
  max-width: 90% !important; margin-left: 0 !important; left: 5% !important; right: 5% !important;
  width: 90% !important; text-align: center !important;
}
video::cue, .htmlvideoplayer::cue {
  background: transparent !important;
  color: #fff;
  font-family: var(--aio-font);
  font-weight: 600;
  font-size: clamp(16px, 2.6vw, 34px) !important;
  line-height: 1.3 !important;
  text-shadow:
    -1px -1px 0 #000, 1px -1px 0 #000, -1px 1px 0 #000, 1px 1px 0 #000,
    0 2px 6px rgba(0, 0, 0, 0.85);
}
$css$;
UPDATE settings SET custom_css = DEFAULT
    WHERE custom_css = $css$/* AIOStreams look for jellyfin-web: the near-black pages, white pill buttons,
   Inter type, rounded cards with a faint ring and quiet grey text of
   AIOStreams' own Jellyfin client, and Plex-style subtitles.
   Edit or clear it under Settings > Web player > Custom CSS. */
@import url('https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700;800&display=swap');

:root {
  --aio-bg: #070707;
  --aio-paper: #0b0b0b;
  --aio-raised: #111111;
  --aio-field: #161616;
  --aio-hover: rgba(255, 255, 255, 0.06);
  --aio-hover-2: rgba(255, 255, 255, 0.1);
  --aio-ring: rgba(255, 255, 255, 0.05);
  --aio-border: rgba(255, 255, 255, 0.1);
  --aio-text: #e6e6e6;
  --aio-muted: rgba(255, 255, 255, 0.45);
  --aio-muted-2: rgba(255, 255, 255, 0.62);
  --aio-brand: #6152df;
  --aio-brand-soft: #9f92ff;
  --aio-radius: 0.5rem;
  --aio-card: 0.75rem;
  --aio-font: 'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
}

/* Pages */
/* jellyfin-web's palette: near-black, white as the accent in place of its
   light blue, AIOStreams' greys. */
:root, :root[data-theme], [data-mui-color-scheme] {
  --jf-palette-primary-main: #ffffff !important;
  --jf-palette-primary-mainChannel: 255 255 255 !important;
  --jf-palette-primary-dark: #d4d4d4 !important;
  --jf-palette-primary-contrastText: #000000 !important;
  --jf-palette-secondary-main: #9f92ff !important;
  --jf-palette-secondary-contrastText: #000000 !important;
  --jf-palette-background-default: #070707 !important;
  --jf-palette-background-defaultImage: none !important;
  --jf-palette-background-paper: #0b0b0b !important;
  --jf-palette-background-paperChannel: 11 11 11 !important;
  --jf-palette-text-primary: #e6e6e6 !important;
  --jf-palette-text-secondary: rgba(255, 255, 255, 0.62) !important;
  --jf-palette-divider: rgba(255, 255, 255, 0.08) !important;
  --jf-palette-action-hover: rgba(255, 255, 255, 0.06) !important;
  --jf-palette-AppBar-defaultBg: #070707 !important;
  --jf-palette-AppBar-transparentBg: rgba(7, 7, 7, 0) !important;
  --jf-palette-FilledInput-bg: #161616 !important;
  --jf-palette-SnackbarContent-bg: #111111 !important;
  --jf-palette-SnackbarContent-color: #e6e6e6 !important;
}
html, body, .backgroundContainer, .skinBody, .dashboardDocument { background-color: var(--aio-bg) !important; }
/* The backdrop of a title's page fades into the page, as AIOStreams' hero
   does: dark at the bottom and on the left, where the text is. */
.backgroundContainer.withBackdrop {
  opacity: 1 !important;
  background: linear-gradient(to top, #070707 12%, rgba(7, 7, 7, 0.55) 55%, rgba(7, 7, 7, 0.2) 100%),
    linear-gradient(to right, rgba(7, 7, 7, 0.9) 0%, rgba(7, 7, 7, 0.35) 45%, rgba(7, 7, 7, 0) 70%) !important;
}
.noBackdropTransparency .detailPageWrapperContainer { background: var(--aio-bg) !important; }
.detailRibbon { background: transparent !important; }
body { color: var(--aio-text); font-family: var(--aio-font) !important; }
body *:not(.material-icons):not([class*="material-icons"]):not(.md-icon) { font-family: var(--aio-font) !important; }
h1, h2, h3, .sectionTitle, .itemName, .pageTitle { color: #fff; letter-spacing: -0.02em; }
h1, .itemName.infoText, .pageTitle { font-weight: 800 !important; }
.sectionTitle, h2.sectionTitle { font-size: 1.45em !important; font-weight: 700 !important; }
.sectionTitle-cards { padding-bottom: 0.35em; }
.secondaryText, .fieldDescription, .cardText-secondary, .listItemBodyText.secondary,
.itemMiscInfo, .mediaInfoItem, .itemMiscInfo-secondary, .overview-controls { color: var(--aio-muted-2) !important; }
.overview, .itemOverview, .detail-clamp-text { color: rgba(255, 255, 255, 0.78) !important; line-height: 1.6; }
a { color: inherit; }

/* Header: transparent over the page, blurred once scrolled */
.skinHeader, .skinHeader.semiTransparent, .skinHeader-withBackground {
  background: linear-gradient(to bottom, rgba(7, 7, 7, 0.92), rgba(7, 7, 7, 0)) !important;
  box-shadow: none !important;
  border: none !important;
}
.skinHeader.skinHeader-blurred, .skinHeader.headroom--not-top {
  background: rgba(7, 7, 7, 0.82) !important;
  backdrop-filter: blur(18px) saturate(140%);
  -webkit-backdrop-filter: blur(18px) saturate(140%);
}
.headerTabs, .headerTabs.sectionTabs { background: transparent !important; }
.pageTitleWithLogo, .pageTitleWithDefaultLogo { filter: grayscale(1) brightness(2); }

/* Side menu: a dark rail */
.mainDrawer, .mainDrawer-scrollContainer, .MuiDrawer-paper {
  background: var(--aio-bg) !important;
  border-right: 1px solid var(--aio-ring) !important;
}
.navMenuOption { border-radius: 999px; margin: 0.15em 0.75em; padding-left: 1em !important; color: var(--aio-muted-2) !important; }
.navMenuOption:hover { background: var(--aio-hover) !important; color: #fff !important; }
.navMenuOption-selected, .navMenuOption-selected:hover { background: var(--aio-hover-2) !important; color: #fff !important; }
.navMenuOption-selected .navMenuOptionIcon { color: #fff !important; }
.sidebarHeader { color: var(--aio-muted) !important; font-size: 0.75em; text-transform: uppercase; letter-spacing: 0.08em; }
.sidebarDivider { background: var(--aio-ring) !important; }

/* Tabs: pills */
.emby-tabs-slider { gap: 0.25em; }
.emby-tab-button { color: var(--aio-muted-2) !important; border-radius: 999px !important; padding: 0.55em 1.1em !important; }
.emby-tab-button:hover { color: #fff !important; background: var(--aio-hover) !important; }
.emby-tab-button-active, .emby-tab-button-active.emby-button-tv { color: #000 !important; background: #fff !important; }
.emby-tab-button-active::after, .emby-tab-button::after { display: none !important; }

/* Cards: rounded art with a faint ring, plain text below */
.cardBox, .cardScalable, .cardImageContainer, .cardContent, .cardPadder,
.cardOverlayContainer, .blurhash-canvas, .cardImage, .coveredImage {
  border-radius: var(--aio-card) !important;
}
.cardBox, .visualCardBox { background: transparent !important; border: none !important; box-shadow: none !important; }
.cardScalable { overflow: hidden; box-shadow: 0 0 0 1px var(--aio-ring) !important; transition: transform 180ms ease, box-shadow 180ms ease; }
.cardImageContainer { background-color: var(--aio-raised) !important; }
.card:hover .cardScalable, .card:focus .cardScalable { transform: scale(1.025); box-shadow: 0 0 0 2px rgba(255, 255, 255, 0.9) !important; }
.card:focus .cardBox, .card.show-focus:focus .cardBox { border: none !important; }
.cardOverlayContainer { background: linear-gradient(to top, rgba(0, 0, 0, 0.75), rgba(0, 0, 0, 0.1)) !important; }
.cardText { color: var(--aio-text) !important; font-weight: 500; }
.cardText-first { color: #fff !important; }
.cardFooter { padding-top: 0.55em !important; }
.cardIndicators .countIndicator, .countIndicator, .playedIndicator, .indicator {
  background: rgba(0, 0, 0, 0.72) !important; color: #fff !important;
  border: 1px solid var(--aio-border); backdrop-filter: blur(8px);
}
.itemProgressBar { background: rgba(255, 255, 255, 0.2) !important; height: 0.25em !important; }
.itemProgressBarForeground { background-color: #fff !important; }
.defaultCardBackground, .defaultCardBackground1, .defaultCardBackground2,
.defaultCardBackground3, .defaultCardBackground4, .defaultCardBackground5 {
  background-color: var(--aio-raised) !important; color: var(--aio-muted) !important;
}

/* Buttons: white pills for the main action, dark pills for the rest */
.emby-button, .raised, .fab, .button-flat, .button-submit { border-radius: 999px !important; }
.raised, .button-alt, .emby-button.raised, .button-cancel {
  background: rgba(255, 255, 255, 0.08) !important; color: #fff !important;
  border: 1px solid var(--aio-border) !important; box-shadow: none !important;
}
.raised:hover, .button-alt:hover, .button-cancel:hover { background: rgba(255, 255, 255, 0.14) !important; }
.button-submit, .emby-button.button-submit, .btnPlay.detailButton, .detailButton-primary,
.btnResume, .fab, .emby-button[is="emby-button"].button-submit, .mainDetailButtons .btnPlay {
  background: #fff !important; color: #000 !important; border: none !important; font-weight: 600 !important;
}
.button-submit:hover, .button-submit:focus, .btnPlay.detailButton:hover, .fab:hover { background: #e6e6e6 !important; }
.btnPlay .detailButton-icon, .button-submit .material-icons { color: #000 !important; }
.button-link, a.button-link, .emby-button.button-link { color: #fff !important; text-decoration: underline; text-underline-offset: 3px; }
/* The header's selected entry and the user button: white, not light blue. */
.headerButton.selected, .headerTabs .emby-tab-button-active, .navMenuOption-selected, .headerSelectedButton { color: #fff !important; }
.headerUserButtonRound, .headerUserButton .headerButtonIcon { background-color: #1c1c1c !important; color: #fff !important; }
.mdl-spinner__layer, .mdl-spinner__circle { border-color: #fff !important; }
.MuiCircularProgress-root { color: #fff !important; }
.detailButton {
  border-radius: 999px !important; background: rgba(255, 255, 255, 0.08) !important;
  border: 1px solid var(--aio-border) !important; color: #fff !important;
}
.detailButton:hover { background: rgba(255, 255, 255, 0.16) !important; }
.detailButton-content { padding: 0.15em 0.4em; }
.paper-icon-button-light { border-radius: 999px !important; }
.paper-icon-button-light:hover:not(:disabled) { color: #fff !important; background: var(--aio-hover) !important; }

/* Fields */
.emby-input, .emby-select, .emby-textarea, .emby-select-withcolor, .searchfields-txtSearch {
  background: var(--aio-field) !important; color: var(--aio-text) !important;
  border: 1px solid var(--aio-border) !important; border-radius: var(--aio-radius) !important;
}
.emby-input:focus, .emby-select:focus, .emby-textarea:focus, .searchfields-txtSearch:focus {
  border-color: var(--aio-brand) !important; box-shadow: 0 0 0 3px rgba(97, 82, 223, 0.3) !important;
}
.searchfields-txtSearch { border-radius: 999px !important; padding-left: 1.2em !important; }
.inputLabelFocused, .selectLabelFocused, .textareaLabelFocused { color: var(--aio-brand-soft) !important; }
.emby-checkbox:checked + span + .checkboxOutline, .emby-checkbox:checked + .checkboxOutline {
  background-color: #fff !important; border-color: #fff !important;
}
.emby-checkbox:checked + span + .checkboxOutline .checkboxIcon, .emby-checkbox:checked + .checkboxOutline .checkboxIcon { color: #000 !important; }
.mdl-switch__input:checked + .mdl-switch__label + .mdl-switch__trackContainer > .mdl-switch__track,
.mdl-switch__input:checked + .mdl-switch__trackContainer > .mdl-switch__track { background: rgba(255, 255, 255, 0.55) !important; }
.mdl-switch__input:checked + .mdl-switch__label + .mdl-switch__trackContainer > .mdl-switch__thumb,
.mdl-switch__input:checked + .mdl-switch__trackContainer > .mdl-switch__thumb { background: #fff !important; }

/* Sliders (seek bar, volume) */
.mdl-slider-background-lower, .mdl-slider-lower { background-color: #fff !important; }
.mdl-slider-background-upper { background-color: rgba(255, 255, 255, 0.25) !important; }
.mdl-slider::-webkit-slider-thumb { background: #fff !important; }
.mdl-slider::-moz-range-thumb { background: #fff !important; }
.sliderBubble { background: var(--aio-raised) !important; border: 1px solid var(--aio-border); border-radius: var(--aio-radius); }

/* Lists, dialogs and menus */
.listItem { border-radius: var(--aio-radius); }
.listItem:hover, .listItem:focus, .actionSheetMenuItem:hover { background: var(--aio-hover) !important; }
.paperList, .detailSection .paperList, .collapseContent, .readOnlyContent {
  background: var(--aio-paper) !important; border: 1px solid var(--aio-ring); border-radius: var(--aio-card) !important;
}
.dialog, .formDialog, .actionSheet, .actionsheetMenu, .focuscontainer.dialog, .dialogContainer .dialog {
  background: var(--aio-paper) !important; color: var(--aio-text) !important;
  border: 1px solid var(--aio-border); border-radius: 1rem !important;
  box-shadow: 0 28px 70px -20px rgba(0, 0, 0, 0.95) !important;
}
.formDialogHeader, .formDialogFooter { background: var(--aio-paper) !important; border-color: var(--aio-ring) !important; }
.dialogBackdropOpened { background: rgba(0, 0, 0, 0.75) !important; }
.toast { background: var(--aio-raised) !important; border: 1px solid var(--aio-border); border-radius: 999px !important; color: var(--aio-text); }

/* Title pages */
.detailPagePrimaryContainer, .detailPageSecondaryContainer { background: transparent !important; }
.itemBackdrop::after { background: linear-gradient(to top, var(--aio-bg) 0%, rgba(7, 7, 7, 0) 70%) !important; }
.detailImageContainer .card .cardScalable { box-shadow: 0 20px 50px -12px rgba(0, 0, 0, 0.8), 0 0 0 1px var(--aio-ring) !important; }
.detailLogo { filter: drop-shadow(0 4px 18px rgba(0, 0, 0, 0.6)); }
.mediaInfoCriticRating, .starRatingContainer .material-icons, .starIcon { color: #f5c518 !important; }
.genres a, .itemGenres a, .itemExternalLinks a, .itemDetailsGroup a, .detailsGroupItem a { color: var(--aio-text) !important; }
.genres a:hover, .itemGenres a:hover { color: #fff !important; }
/* Phones: the backdrop above already shows the title's art, as AIOStreams'
   mobile pages do, so the image beside the name goes (a wide collection
   image showed there cut to a strip) and the text takes the full width. */
.layout-mobile .itemDetailPage .detailImageContainer { display: none !important; }
.layout-mobile .itemDetailPage .infoWrapper { padding-left: 0 !important; padding-right: 0 !important; }
.layout-mobile .itemDetailPage .detailRibbon { padding: 0.75rem 5% 0.25rem !important; }
.layout-mobile .itemDetailPage .nameContainer, .layout-mobile .itemDetailPage .itemMiscInfo { justify-content: center; text-align: center; }
.layout-mobile .itemDetailPage .mainDetailButtons { justify-content: center; }

/* Video player */
.videoOsdBottom, .osdControls { background: linear-gradient(to top, rgba(0, 0, 0, 0.9), rgba(0, 0, 0, 0)) !important; }
.upNextContainer, .skip-button, .skipIntro button, .btnSkipIntro {
  background: rgba(11, 11, 11, 0.92) !important; border: 1px solid var(--aio-border) !important;
  border-radius: 999px !important; color: #fff !important;
}

/* Dashboard and other MUI pages */
.MuiPaper-root { background-image: none !important; }

/* Scrollbars and selection */
::-webkit-scrollbar { width: 10px; height: 10px; }
::-webkit-scrollbar-track { background: var(--aio-bg); }
::-webkit-scrollbar-thumb { background: #363636; border-radius: 99px; }
::-webkit-scrollbar-thumb:hover { background: #484848; }
::selection { background: rgba(97, 82, 223, 0.45); }

/* Subtitles, as Plex shows them: white, semi-bold sans serif with a thin
   black outline and a soft shadow, no box behind, low on the picture.
   Styled ASS/SSA subtitles keep their own look. In portrait, a 16:9 picture
   is a band across the middle of the screen: they sit just above its bottom
   edge, not at the bottom of the screen, and their size follows its width. */
.videoSubtitles { bottom: 6% !important; padding: 0 6% !important; }
@media (orientation: portrait) {
  .videoSubtitles { bottom: max(2%, calc((100% - 56.25vw) / 2 + 0.6em)) !important; padding: 0 4% !important; }
}
.videoSubtitlesInner, .videoSubtitles .videoSubtitlesInner span {
  background: transparent !important;
  color: #fff !important;
  font-family: var(--aio-font) !important;
  font-weight: 600 !important;
  font-size: clamp(16px, min(3.8vh, 2.4vw), 46px) !important;
  line-height: 1.25 !important;
  text-shadow:
    -1px -1px 0 #000, 1px -1px 0 #000, -1px 1px 0 #000, 1px 1px 0 #000,
    0 0 2px #000, 0 2px 6px rgba(0, 0, 0, 0.85) !important;
}
@media (orientation: portrait) {
  .videoSubtitlesInner, .videoSubtitles .videoSubtitlesInner span { font-size: clamp(14px, 4.4vw, 24px) !important; }
}
video::cue {
  background: transparent !important;
  color: #fff;
  font-family: var(--aio-font);
  font-weight: 600;
  text-shadow:
    -1px -1px 0 #000, 1px -1px 0 #000, -1px 1px 0 #000, 1px 1px 0 #000,
    0 2px 6px rgba(0, 0, 0, 0.85);
}
$css$
    AND custom_js = '';
