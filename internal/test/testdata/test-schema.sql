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
	user_id    INTEGER NOT NULL REFERENCES users,
	ext_id     INTEGER NOT NULL,
	created_at TIMESTAMPTZ NOT NULL,
	status     INTEGER NOT NULL,
	link_count INTEGER NOT NULL DEFAULT 1,

	PRIMARY KEY (user_id, ext_id)
);

CREATE TABLE user_views (
	user_id    INTEGER NOT NULL PRIMARY KEY REFERENCES users,
	views      INTEGER NOT NULL
);

CREATE TABLE seen (
	user_id INTEGER NOT NULL PRIMARY KEY REFERENCES users,
	seen_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
