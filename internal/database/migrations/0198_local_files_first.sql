-- Whether a user is offered the versions of local folders and network
-- shares before the others, whatever the version order's criteria: off
-- for every user and every group, so that nothing changes until an
-- administrator gives it.
ALTER TABLE users ADD COLUMN local_first boolean NOT NULL DEFAULT false;
ALTER TABLE user_groups ADD COLUMN local_first boolean NOT NULL DEFAULT false;
