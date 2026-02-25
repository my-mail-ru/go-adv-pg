CREATE SEQUENCE user_id_seq;

CREATE TABLE users (
	id         INTEGER PRIMARY KEY DEFAULT nextval('user_id_seq'),
	name       TEXT NOT NULL,
	type       INTEGER NOT NULL,
	post_count INTEGER NOT NULL DEFAULT 1,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE OR REPLACE FUNCTION users_set_updated() RETURNS TRIGGER AS $$
BEGIN
	NEW.updated_at = now();
	RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE TRIGGER users_set_updated
	BEFORE UPDATE
	ON users
	FOR EACH ROW
	EXECUTE FUNCTION users_set_updated();

CREATE TABLE ext_links (
	user_id      INTEGER NOT NULL REFERENCES users,
	ext_id       INTEGER NOT NULL,
	created_at   TIMESTAMPTZ NOT NULL,
	status       INTEGER NOT NULL,
	link_count   INTEGER NOT NULL DEFAULT 1,
	modified_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
	refreshed_at TIMESTAMPTZ NOT NULL DEFAULT now(),

	PRIMARY KEY (user_id, ext_id)
);

CREATE OR REPLACE FUNCTION ext_links_set_refreshed() RETURNS TRIGGER AS $$
BEGIN
	NEW.refreshed_at = now();
	RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE TRIGGER ext_links_set_refreshed
	BEFORE UPDATE
	ON ext_links
	FOR EACH ROW
	EXECUTE FUNCTION ext_links_set_refreshed();

CREATE TABLE user_views (
	user_id    INTEGER NOT NULL PRIMARY KEY REFERENCES users,
	views      INTEGER NOT NULL
);

CREATE TABLE seen (
	user_id INTEGER NOT NULL PRIMARY KEY REFERENCES users,
	seen_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE user_options (
	user_id    INTEGER NOT NULL REFERENCES users,
	option_id  INTEGER NOT NULL,
	flag       BOOLEAN NOT NULL,
	option     TEXT NOT NULL DEFAULT 'not set',

	PRIMARY KEY (user_id, option_id)
);
