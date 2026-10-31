-- The audio and subtitle tracks each user chose on a title, a series for
-- its episodes or a movie, while their configuration remembers their
-- selections: the next plays of the title show them by default. A choice
-- is a language, never a track's index, as versions number their tracks
-- differently. audio_language is empty when no audio track was chosen.
-- subtitle is 'none' when no subtitle was chosen, 'off' when the user
-- turned them off, and 'track' for subtitle_language with its forced and
-- hearing-impaired flags. A title is not referenced: series and movies
-- of addons are not all stored.
CREATE TABLE track_choices (
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    title_id uuid NOT NULL,
    audio_language text NOT NULL DEFAULT '' CHECK (octet_length(audio_language) <= 35),
    subtitle text NOT NULL DEFAULT 'none' CHECK (subtitle IN ('none', 'off', 'track')),
    subtitle_language text NOT NULL DEFAULT '' CHECK (octet_length(subtitle_language) <= 35),
    subtitle_forced boolean NOT NULL DEFAULT false,
    subtitle_hearing_impaired boolean NOT NULL DEFAULT false,
    chosen_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, title_id),
    CHECK ((subtitle = 'track') = (subtitle_language <> '')),
    CHECK (audio_language <> '' OR subtitle <> 'none')
);
