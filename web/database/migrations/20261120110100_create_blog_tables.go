package migrations

import "github.com/AlexGalhardo/vukem/schema"

// The blog destinations (SDD-020 part 5, SDD-005 section 5.15.1): the post of a publication, what the read API serves
// and the remote id of a post at each destination. No table holds a secret: the settings of a destination are the
// secret of its connection and live in the vault.
func m20261120110100CreateBlogTables() schema.Migration {
	return schema.Migration{
		ID: "20261120110100_create_blog_tables",
		Up: func(s *schema.Schema) error {
			stmts := []string{
				`ALTER TABLE publications ADD COLUMN post_item_id TEXT NOT NULL DEFAULT ''`,
				`CREATE TABLE published_posts (
  slug          TEXT    NOT NULL,
  post_item_id  TEXT    NOT NULL,
  project_id    TEXT    NOT NULL,
  connection_id TEXT    NOT NULL,
  owner_user_id TEXT,
  title         TEXT    NOT NULL,
  summary       TEXT    NOT NULL DEFAULT '',
  tags          TEXT    NOT NULL DEFAULT '[]',
  language      TEXT    NOT NULL DEFAULT '',
  canonical     TEXT    NOT NULL DEFAULT '',
  status        TEXT    NOT NULL CHECK (status IN ('draft','published')),
  markdown      TEXT    NOT NULL,
  html          TEXT    NOT NULL,
  images        TEXT    NOT NULL DEFAULT '[]',
  published_ms  INTEGER NOT NULL,
  updated_ms    INTEGER NOT NULL,
  PRIMARY KEY (slug)
)`,
				`CREATE UNIQUE INDEX published_posts_post_item_id_unique ON published_posts (post_item_id, connection_id)`,
				`CREATE INDEX published_posts_published_ms_index ON published_posts (status, published_ms)`,
				`CREATE TABLE blog_remote_posts (
  connection_id TEXT    NOT NULL,
  post_item_id  TEXT    NOT NULL,
  remote_id     TEXT    NOT NULL,
  url           TEXT    NOT NULL DEFAULT '',
  updated_ms    INTEGER NOT NULL,
  PRIMARY KEY (connection_id, post_item_id)
)`,
			}

			for _, q := range stmts {
				if err := s.Raw(q); err != nil {
					return err
				}
			}

			return nil
		},
		Down: func(s *schema.Schema) error {
			for _, t := range []string{"blog_remote_posts", "published_posts"} {
				if err := s.DropIfExists(t); err != nil {
					return err
				}
			}

			return s.Raw(`ALTER TABLE publications DROP COLUMN post_item_id`)
		},
	}
}
