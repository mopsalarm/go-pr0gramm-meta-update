BEGIN;

CREATE TABLE IF NOT EXISTS items (
  id       INT PRIMARY KEY NOT NULL,
  promoted INT             NOT NULL,
  up       INT             NOT NULL,
  down     INT             NOT NULL,
  created  INT             NOT NULL,
  image    TEXT            NOT NULL,
  thumb    TEXT            NOT NULL,
  fullsize TEXT            NOT NULL,
  source   TEXT            NOT NULL,
  flags    INT             NOT NULL,
  username TEXT            NOT NULL,
  mark     INT             NOT NULL,
  width    INT             NOT NULL,
  height   INT             NOT NULL,
  audio    BOOL            NOT NULL
);

CREATE TABLE IF NOT EXISTS tags (
  id         INT PRIMARY KEY NOT NULL,
  item_id    INT             NOT NULL, -- REFERENCES items (id),
  up         INT             NOT NULL,
  down       INT             NOT NULL,
  confidence REAL            NOT NULL,
  tag        TEXT            NOT NULL
);

CREATE INDEX tags__item_id
  ON tags (item_id);

CREATE TABLE IF NOT EXISTS users (
  id         INT  NOT NULL PRIMARY KEY,
  name       TEXT NOT NULL,
  registered INT  NOT NULL,
  score      INT  NOT NULL
);

CREATE INDEX IF NOT EXISTS users__name
  ON users (lower("name") text_pattern_ops);

CREATE INDEX IF NOT EXISTS items__created_ts
  ON items (to_timestamp(created));

CREATE INDEX items__promoted
  ON items (promoted)
  WHERE items.promoted > 0;

-- not used right now
-- CREATE INDEX IF NOT EXISTS items__username ON items(lower(username));
