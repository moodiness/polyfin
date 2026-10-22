-- The web player's default custom CSS is the AIOStreams theme, held whole
-- in the setting, and its default custom script none: on new servers, and on
-- servers whose two fields still held the LumaaGlaass defaults. A server
-- that changed either keeps both as they are.
ALTER TABLE settings
    ALTER COLUMN custom_css SET DEFAULT $css$/* AIOStreams theme for jellyfin-web: the near-black surfaces, violet brand,
   Inter type and soft 0.5rem corners of AIOStreams' own Jellyfin client.
   Edit or clear it under Settings > Web player > Custom CSS. */
@import url('https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700;800&display=swap');

:root {
  --aio-bg: #070707;
  --aio-paper: #0b0b0b;
  --aio-raised: #101010;
  --aio-field: #161616;
  --aio-hover: rgba(255, 255, 255, 0.06);
  --aio-hover-2: rgba(255, 255, 255, 0.08);
  --aio-border: rgba(255, 255, 255, 0.1);
  --aio-text: #d1d1d1;
  --aio-muted: rgba(255, 255, 255, 0.4);
  --aio-muted-2: rgba(255, 255, 255, 0.6);
  --aio-brand: #6152df;
  --aio-brand-press: #5243cb;
  --aio-brand-soft: #9f92ff;
  --aio-brand-light: #c7c2ff;
  --aio-radius: 0.5rem;
  --aio-radius-lg: 0.75rem;
}

html, body, .backgroundContainer, .backdropContainer + .backgroundContainer,
.mainAnimatedPages, .page, .libraryPage, .itemDetailPage, .dashboardDocument {
  background-color: var(--aio-bg) !important;
}
.backgroundContainer.withBackdrop { background-color: rgba(7, 7, 7, 0.86) !important; }

body, .emby-button, .emby-input, .emby-select, .emby-textarea, h1, h2, h3, .cardText,
.listItemBodyText, .sectionTitle, .itemName {
  font-family: 'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif !important;
}
body { color: var(--aio-text); }
h1, h2, h3, .sectionTitle, .itemName { letter-spacing: -0.02em; font-weight: 700; }
.secondaryText, .fieldDescription, .cardText-secondary, .listItemBodyText.secondary,
.itemMiscInfo, .mediaInfoItem { color: var(--aio-muted-2) !important; }

/* Header and side menu */
.skinHeader, .skinHeader.semiTransparent {
  background: rgba(7, 7, 7, 0.78) !important;
  backdrop-filter: blur(16px) saturate(140%);
  -webkit-backdrop-filter: blur(16px) saturate(140%);
  border-bottom: 1px solid var(--aio-border);
  box-shadow: none !important;
}
.skinHeader.noHeaderRight, .headerTabs { background: transparent !important; }
.mainDrawer, .mainDrawer-scrollContainer, .MuiDrawer-paper {
  background: var(--aio-paper) !important;
  border-right: 1px solid var(--aio-border);
}
.navMenuOption { border-radius: var(--aio-radius); margin: 0.1em 0.6em; }
.navMenuOption:hover { background: var(--aio-hover) !important; }
.navMenuOption-selected { background: rgba(97, 82, 223, 0.18) !important; color: var(--aio-brand-light) !important; }

/* Tabs */
.emby-tab-button { color: var(--aio-muted-2) !important; border-radius: var(--aio-radius); }
.emby-tab-button:hover { color: #fff !important; background: var(--aio-hover); }
.emby-tab-button-active, .emby-tab-button-active.emby-button-tv { color: #fff !important; }
.emby-tab-button-active::after, .emby-tab-button.emby-tab-button-active {
  border-bottom-color: var(--aio-brand) !important;
}

/* Cards */
.cardBox, .cardScalable, .cardImageContainer, .cardContent, .card .cardPadder,
.cardOverlayContainer, .blurhash-canvas {
  border-radius: var(--aio-radius-lg) !important;
}
.cardBox { background: transparent !important; transition: transform 160ms ease; }
.cardImageContainer { background-color: var(--aio-raised) !important; }
.card:focus .cardBox.visualCardBox, .card:focus .cardBox:not(.visualCardBox) .cardScalable,
.card:hover .cardScalable { box-shadow: 0 0 0 2px var(--aio-brand) !important; }
.cardOverlayContainer { background: rgba(7, 7, 7, 0.55) !important; }
.visualCardBox { background: var(--aio-paper) !important; border: 1px solid var(--aio-border); }
.countIndicator, .playedIndicator, .indicator {
  background: var(--aio-brand) !important; color: #fff !important;
}
.itemProgressBarForeground, .playbackProgress > div { background-color: var(--aio-brand) !important; }
.itemProgressBar { background: rgba(255, 255, 255, 0.15) !important; border-radius: 99px; overflow: hidden; }

/* Buttons */
.emby-button, .raised, .fab, .button-flat { border-radius: var(--aio-radius) !important; }
.raised, .button-alt, .emby-button.raised {
  background: var(--aio-field) !important; color: var(--aio-text) !important;
  border: 1px solid var(--aio-border) !important; box-shadow: none !important;
}
.raised:hover, .button-alt:hover { background: #1c1c1c !important; }
.button-submit, .emby-button.button-submit, .btnPlay.detailButton, .detailButton-primary,
.fab, .emby-button[is="emby-button"].button-submit {
  background: var(--aio-brand) !important; color: #fff !important; border-color: transparent !important;
}
.button-submit:hover, .button-submit:focus, .fab:hover { background: var(--aio-brand-press) !important; }
.button-link, a.button-link, .emby-button.button-link {
  color: var(--aio-brand-soft) !important;
}
.paper-icon-button-light:hover:not(:disabled) { color: var(--aio-brand-soft) !important; background: var(--aio-hover) !important; }
.detailButton { border-radius: 999px !important; background: var(--aio-hover-2) !important; }
.detailButton:hover { background: rgba(255, 255, 255, 0.14) !important; }

/* Fields */
.emby-input, .emby-select, .emby-textarea, .emby-select-withcolor, .searchfields-txtSearch,
.emby-input[type="text"], .emby-input[type="password"] {
  background: var(--aio-field) !important; color: var(--aio-text) !important;
  border: 1px solid var(--aio-border) !important; border-radius: var(--aio-radius) !important;
}
.emby-input:focus, .emby-select:focus, .emby-textarea:focus {
  border-color: var(--aio-brand) !important; box-shadow: 0 0 0 3px rgba(97, 82, 223, 0.25) !important;
}
.inputLabelFocused, .selectLabelFocused, .textareaLabelFocused { color: var(--aio-brand-soft) !important; }
.emby-checkbox:checked + span + .checkboxOutline, .emby-checkbox:checked + .checkboxOutline {
  background-color: var(--aio-brand) !important; border-color: var(--aio-brand) !important;
}
.emby-checkbox:focus + span + .checkboxOutline { border-color: var(--aio-brand-soft) !important; }
.mdl-switch__input:checked + .mdl-switch__label + .mdl-switch__trackContainer > .mdl-switch__track,
.mdl-switch__input:checked + .mdl-switch__trackContainer > .mdl-switch__track { background: rgba(97, 82, 223, 0.55) !important; }
.mdl-switch__input:checked + .mdl-switch__label + .mdl-switch__trackContainer > .mdl-switch__thumb,
.mdl-switch__input:checked + .mdl-switch__trackContainer > .mdl-switch__thumb { background: var(--aio-brand-soft) !important; }

/* Sliders (seek bar, volume) */
.mdl-slider-background-lower, .mdl-slider-lower, .sliderBubble { background-color: var(--aio-brand) !important; }
.mdl-slider::-webkit-slider-thumb { background: var(--aio-brand-light) !important; }
.mdl-slider::-moz-range-thumb { background: var(--aio-brand-light) !important; }

/* Lists, panels, dialogs and menus */
.listItem:hover, .listItem:focus, .actionSheetMenuItem:hover { background: var(--aio-hover) !important; }
.paperList, .detailSection .paperList, .verticalSection-extrabottompadding .paperList,
.collapseContent, .readOnlyContent, .detailPagePrimaryContainer.padded-left {
  background: var(--aio-paper) !important; border: 1px solid var(--aio-border); border-radius: var(--aio-radius-lg) !important;
}
.dialog, .formDialog, .actionSheet, .actionsheetMenu, .focuscontainer.dialog, .dialogContainer .dialog {
  background: var(--aio-paper) !important; color: var(--aio-text) !important;
  border: 1px solid var(--aio-border); border-radius: var(--aio-radius-lg) !important;
  box-shadow: 0 28px 70px -20px rgba(0, 0, 0, 0.9) !important;
}
.formDialogHeader, .formDialogFooter { background: var(--aio-paper) !important; border-color: var(--aio-border) !important; }
.dialogBackdropOpened { background: rgba(0, 0, 0, 0.7) !important; }
.toast { background: var(--aio-raised) !important; border: 1px solid var(--aio-border); border-radius: var(--aio-radius) !important; color: var(--aio-text); }

/* Details page */
.detailPageWrapperContainer, .detailPagePrimaryContainer { background: transparent !important; }
.itemBackdrop::after { background: linear-gradient(to top, var(--aio-bg) 0%, rgba(7, 7, 7, 0.4) 55%, transparent 100%) !important; }
.mediaInfoCriticRating, .starRatingContainer .material-icons { color: var(--aio-brand-soft) !important; }
.genres a, .itemGenres a, .itemExternalLinks a { color: var(--aio-brand-soft) !important; }

/* Video player */
.videoOsdBottom, .osdControls {
  background: linear-gradient(to top, rgba(7, 7, 7, 0.92), transparent) !important;
}
.osdTitle, .osdSecondaryText { font-family: 'Inter', sans-serif; }
.upNextContainer, .skip-button, .skipIntro button {
  background: var(--aio-paper) !important; border: 1px solid var(--aio-border) !important; border-radius: var(--aio-radius) !important;
}

/* Dashboard (MUI) */
:root {
  --mui-palette-primary-main: #6152df;
  --mui-palette-primary-dark: #5243cb;
  --mui-palette-primary-light: #9f92ff;
  --mui-palette-background-default: #070707;
  --mui-palette-background-paper: #0b0b0b;
  --mui-palette-divider: rgba(255, 255, 255, 0.1);
}
.MuiPaper-root { background-image: none !important; }

/* Scrollbars */
::-webkit-scrollbar { width: 10px; height: 10px; }
::-webkit-scrollbar-track { background: var(--aio-bg); }
::-webkit-scrollbar-thumb { background: #363636; border-radius: 99px; }
::-webkit-scrollbar-thumb:hover { background: #484848; }
::selection { background: rgba(97, 82, 223, 0.45); }

/* Subtitles, as Plex shows them: white, semi-bold sans serif with a thin
   black outline and soft shadow, no box behind, low in the frame. Styled
   ASS/SSA subtitles keep their own look. */
.videoSubtitles {
  bottom: 6% !important;
  padding: 0 6% !important;
  font-family: 'Inter', 'Helvetica Neue', Arial, sans-serif !important;
}
.videoSubtitlesInner, .videoSubtitles .videoSubtitlesInner span {
  background: transparent !important;
  color: #fff !important;
  font-family: 'Inter', 'Helvetica Neue', Arial, sans-serif !important;
  font-weight: 600 !important;
  font-size: clamp(18px, 3.4vh, 46px) !important;
  line-height: 1.25 !important;
  letter-spacing: 0.005em;
  text-shadow:
    -1px -1px 0 #000, 1px -1px 0 #000, -1px 1px 0 #000, 1px 1px 0 #000,
    0 0 2px #000, 0 2px 6px rgba(0, 0, 0, 0.85) !important;
}
video::cue {
  background: transparent !important;
  color: #fff;
  font-family: 'Inter', 'Helvetica Neue', Arial, sans-serif;
  font-weight: 600;
  text-shadow:
    -1px -1px 0 #000, 1px -1px 0 #000, -1px 1px 0 #000, 1px 1px 0 #000,
    0 2px 6px rgba(0, 0, 0, 0.85);
}
$css$,
    ALTER COLUMN custom_js SET DEFAULT '';
UPDATE settings SET custom_css = DEFAULT, custom_js = DEFAULT
    WHERE custom_css = $css$@import url('https://cdn.jsdelivr.net/gh/Dyhlio/LumaaGlaass@main/assets/lumaaglaass.css');$css$
    AND custom_js = $js$(() => {
    window.LumaaGlaassOptions = {
        preferences: true,
        homeCarousel: true,
        collectionFilter: true
    };

    const id = 'lg-script';
    if (document.getElementById(id)) return;

    const script = document.createElement('script');
    script.id = id;
    script.src = 'https://cdn.jsdelivr.net/gh/Dyhlio/LumaaGlaass@main/assets/lumaaglaass.js';
    script.onerror = () => {
        script.remove();
        console.error('LumaaGlaass could not be loaded.');
    };

    document.head.appendChild(script);
})();$js$;
