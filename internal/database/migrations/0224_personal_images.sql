-- Each user's image of the Watchlist view and of the smart lists shown as
-- libraries, which Polyfin draws from the first titles they list for that
-- user (see library.personalImages): what it shows, as generated_images
-- keeps it for an item. Image requests come without a user, and find the
-- image by its tag: kept here, it is found again after a restart.
-- smart_list_id names the smart list the view is, null for the Watchlist.
CREATE TABLE personal_images (
    user_id uuid NOT NULL REFERENCES users ON DELETE CASCADE,
    view_id uuid NOT NULL,
    smart_list_id uuid REFERENCES smart_lists ON DELETE CASCADE,
    name text NOT NULL,
    titles uuid[] NOT NULL,
    posters text[] NOT NULL CHECK (cardinality(posters) = cardinality(titles)),
    confined boolean NOT NULL DEFAULT false,
    chosen_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, view_id)
);

CREATE INDEX personal_images_smart_list ON personal_images (smart_list_id) WHERE smart_list_id IS NOT NULL;
