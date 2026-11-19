package migrations

import "github.com/AlexGalhardo/vukem/schema"

// The hosted agent chat (SDD-017 sections 3 and 9): the agents the user may host, the chat sessions of a project
// and their transcripts. No table holds a credential, an environment or a header: the session token lives in memory
// only. A transcript follows its project: the rows go with it.
func m20261119220000CreateChatTables() schema.Migration {
	return schema.Migration{
		ID: "20261119220000_create_chat_tables",
		Up: func(s *schema.Schema) error {
			stmts := []string{
				`CREATE TABLE agent_profiles (
  name       TEXT    NOT NULL,
  command    TEXT    NOT NULL,
  args_json  TEXT    NOT NULL DEFAULT '[]',
  created_by TEXT    NOT NULL,
  created_ms INTEGER NOT NULL,
  updated_ms INTEGER NOT NULL,
  PRIMARY KEY (name)
)`,
				`CREATE TABLE chat_sessions (
  id            TEXT    NOT NULL,
  project_id    TEXT    NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
  profile       TEXT    NOT NULL,
  owner_user_id TEXT,
  started_by    TEXT    NOT NULL,
  state         TEXT    NOT NULL CHECK (state IN ('starting','idle','running','awaiting_permission','ended')),
  end_reason    TEXT    NOT NULL DEFAULT '',
  title         TEXT    NOT NULL DEFAULT '',
  created_ms    INTEGER NOT NULL,
  updated_ms    INTEGER NOT NULL,
  ended_ms      INTEGER,
  event_count   INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (id)
)`,
				`CREATE INDEX chat_sessions_project_id_created_ms_index ON chat_sessions (project_id, created_ms DESC)`,
				`CREATE TABLE chat_events (
  session_id TEXT    NOT NULL REFERENCES chat_sessions (id) ON DELETE CASCADE,
  seq        INTEGER NOT NULL CHECK (seq >= 1),
  kind       TEXT    NOT NULL,
  at_ms      INTEGER NOT NULL,
  body_json  TEXT    NOT NULL,
  PRIMARY KEY (session_id, seq)
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
			for _, t := range []string{"chat_events", "chat_sessions", "agent_profiles"} {
				if err := s.DropIfExists(t); err != nil {
					return err
				}
			}

			return nil
		},
	}
}
