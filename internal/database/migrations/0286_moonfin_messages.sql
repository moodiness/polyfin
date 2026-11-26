-- The messages the administrator writes to the users of Moonfin's apps
-- (see package moonfin), which the apps show in their messages window, in
-- the order of position. body is Markdown. delivery 'inbox' only marks
-- the menu button unread; 'popup' opens the window on its own once.
-- audience 'all' shows a message to every user, 'admins' to
-- administrators, 'users' to those of moonfin_message_users. A message
-- shows from start_at, if any, until end_at, if any. link_label and
-- link_url, both set or both empty, make a button that opens the address.
CREATE TABLE moonfin_messages (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    position integer NOT NULL,
    title text NOT NULL CHECK (char_length(title) <= 120),
    body text NOT NULL CHECK (char_length(body) <= 2000),
    color text NOT NULL CHECK (color IN ('white', 'blue', 'green', 'yellow', 'red')),
    delivery text NOT NULL CHECK (delivery IN ('inbox', 'popup')),
    audience text NOT NULL CHECK (audience IN ('all', 'users', 'admins')),
    start_at timestamptz,
    end_at timestamptz,
    link_label text NOT NULL DEFAULT '' CHECK (char_length(link_label) <= 60),
    link_url text NOT NULL DEFAULT '' CHECK (char_length(link_url) <= 2000),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CHECK (title <> '' OR body <> ''),
    CHECK (start_at IS NULL OR end_at IS NULL OR end_at > start_at),
    CHECK ((link_label = '') = (link_url = ''))
);

-- The users a message of the audience 'users' is shown to.
CREATE TABLE moonfin_message_users (
    message_id uuid NOT NULL REFERENCES moonfin_messages (id) ON DELETE CASCADE,
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    PRIMARY KEY (message_id, user_id)
);

CREATE INDEX moonfin_message_users_user ON moonfin_message_users (user_id);
