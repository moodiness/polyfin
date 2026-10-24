-- The web player's look becomes a choice, web_theme: the LumaaGlaass theme
-- Polyfin bundles, which Polyfin writes the custom CSS and script for
-- itself, jellyfin-web's own look, or the administrator's own Custom CSS
-- and JavaScript, which custom_css and custom_js now hold alone and keep
-- while another choice is made. A server whose two texts still loaded the
-- bundled theme chooses it, with both texts emptied; one with both empty,
-- jellyfin-web's own look; any other keeps its texts as Custom.
ALTER TABLE settings ADD COLUMN web_theme text NOT NULL DEFAULT 'custom';
UPDATE settings SET web_theme = 'lumaaglaass', custom_css = '', custom_js = ''
    WHERE custom_css = $css$@import url('polyfin/themes/lumaaglaass/3deef82debc0ed6a/assets/lumaaglaass.css');$css$
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
    script.src = 'polyfin/themes/lumaaglaass/3deef82debc0ed6a/assets/lumaaglaass.js';
    script.onerror = () => {
        script.remove();
        console.error('LumaaGlaass could not be loaded.');
    };

    document.head.appendChild(script);
})();$js$;
UPDATE settings SET web_theme = 'none' WHERE custom_css = '' AND custom_js = '' AND web_theme = 'custom';
ALTER TABLE settings
    ALTER COLUMN web_theme SET DEFAULT 'lumaaglaass',
    ALTER COLUMN custom_css SET DEFAULT '',
    ALTER COLUMN custom_js SET DEFAULT '';

-- The copy Polyfin keeps of the files the Custom texts load from outside,
-- at most one: the folder of its files under the data folder's themes,
-- when it was made, the files it holds, each with the address it came from
-- and, for those the texts name, the text naming it, and the addresses it
-- could not save, with why.
CREATE TABLE theme_copy (
    singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    id text NOT NULL,
    saved_at timestamptz NOT NULL,
    files jsonb NOT NULL,
    unsaved jsonb NOT NULL
);
