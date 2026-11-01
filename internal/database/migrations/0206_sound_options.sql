-- Whether a user's web player boosts the volume of videos in the browser,
-- and whether the server converts the audio of their videos and channels,
-- evened at -16 LUFS, the video copied: both off for every user and every
-- group, so that nothing changes until a user or an administrator turns
-- them on.
ALTER TABLE users ADD COLUMN boost_volume boolean NOT NULL DEFAULT false;
ALTER TABLE users ADD COLUMN convert_sound boolean NOT NULL DEFAULT false;
ALTER TABLE user_groups ADD COLUMN boost_volume boolean NOT NULL DEFAULT false;
ALTER TABLE user_groups ADD COLUMN convert_sound boolean NOT NULL DEFAULT false;
