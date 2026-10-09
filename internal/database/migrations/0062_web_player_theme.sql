-- Custom CSS and Custom JavaScript load the LumaaGlaass theme by default,
-- from jsDelivr at a pinned commit of its repository: on new servers, and
-- on servers whose two fields were both still empty. A server that set
-- either keeps both as they are. Moving to another commit takes a migration
-- replacing these defaults, and the values still equal to them.
ALTER TABLE settings
    ALTER COLUMN custom_css SET DEFAULT $css$@import url('https://cdn.jsdelivr.net/gh/Dyhlio/LumaaGlaass@f1afec5e4b924ef26553056e88688bdeba3fafca/assets/lumaaglaass.css');$css$,
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
    script.src = 'https://cdn.jsdelivr.net/gh/Dyhlio/LumaaGlaass@f1afec5e4b924ef26553056e88688bdeba3fafca/assets/lumaaglaass.js';
    script.onerror = () => {
        script.remove();
        console.error('LumaaGlaass could not be loaded.');
    };

    document.head.appendChild(script);
})();$js$;
UPDATE settings SET custom_css = DEFAULT, custom_js = DEFAULT
    WHERE custom_css = '' AND custom_js = '';
