-- Custom CSS and Custom JavaScript load the LumaaGlaass theme Polyfin
-- bundles, from the web client's own address, no longer from a CDN at the
-- main branch of its repository: on new servers, and on servers whose two
-- fields still held the previous defaults. A server that changed either
-- keeps both as they are, never a stylesheet and a script from two versions
-- of the theme. The addresses carry the version of the bundled files.
ALTER TABLE settings
    ALTER COLUMN custom_css SET DEFAULT $css$@import url('polyfin/themes/lumaaglaass/3deef82debc0ed6a/assets/lumaaglaass.css');$css$,
    ALTER COLUMN custom_js SET DEFAULT $js$(() => {
    window.LumaaGlaassOptions = {
        preferences: true,
        homeCarousel: false,
        collectionFilter: true
    };

    const id = 'lg-script';
    if (document.getElementById(id)) return;

    const script = document.createElement('script');
    script.id = id;
    script.src = 'polyfin/themes/lumaaglaass/3deef82debc0ed6a/assets/lumaaglaass.js';
    script.onerror = () => {
        script.remove();
        console.error('LumaaGlaass could not be loaded.');
    };

    document.head.appendChild(script);
})();$js$;
UPDATE settings SET custom_css = DEFAULT, custom_js = DEFAULT
    WHERE custom_css = $css$@import url('https://cdn.jsdelivr.net/gh/Dyhlio/LumaaGlaass@main/assets/lumaaglaass.css');$css$
    AND custom_js = $js$(() => {
    window.LumaaGlaassOptions = {
        preferences: true,
        homeCarousel: false,
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
