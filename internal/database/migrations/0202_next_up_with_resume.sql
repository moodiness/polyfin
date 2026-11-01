-- Whether taking an episode off Continue Watching also takes its series off
-- Next Up for the user, as Remove from Next Up does: off for every user,
-- so that nothing changes until they turn it on in My account.
ALTER TABLE users ADD COLUMN next_up_with_resume boolean NOT NULL DEFAULT false;
