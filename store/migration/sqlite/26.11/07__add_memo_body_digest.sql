ALTER TABLE memo ADD COLUMN body_digest TEXT NOT NULL DEFAULT '';

CREATE TABLE memo_body_namespace (
  id INTEGER NOT NULL PRIMARY KEY CHECK (id = 1),
  namespace TEXT NOT NULL
);

INSERT INTO memo_body_namespace (id, namespace) VALUES (1, lower(hex(randomblob(16))));
