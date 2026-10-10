-- Custom CSS and Custom JavaScript load the LumaaGlaass theme from the main
-- branch of its repository by default, no longer from a pinned commit, so
-- servers follow the theme without a Polyfin release: on new servers, and
-- on servers whose two fields still held the theme at that commit. A server
-- that changed either keeps both as they are, never a stylesheet and a
-- script from two versions of the theme.
ALTER TABLE settings
    ALTER COLUMN custom_css SET DEFAULT $css$@import url('https://cdn.jsdelivr.net/gh/Dyhlio/LumaaGlaass@main/assets/lumaaglaass.css');$css$,
    ALTER COLUMN custom_js SET DEFAULT $js$(() => {
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
UPDATE settings SET custom_css = DEFAULT, custom_js = DEFAULT
    WHERE custom_css = $css$@import url('https://cdn.jsdelivr.net/gh/Dyhlio/LumaaGlaass@f1afec5e4b924ef26553056e88688bdeba3fafca/assets/lumaaglaass.css');$css$
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
    script.src = 'https://cdn.jsdelivr.net/gh/Dyhlio/LumaaGlaass@f1afec5e4b924ef26553056e88688bdeba3fafca/assets/lumaaglaass.js';
    script.onerror = () => {
        script.remove();
        console.error('LumaaGlaass could not be loaded.');
    };

    document.head.appendChild(script);
})();$js$;
