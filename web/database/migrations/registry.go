// Package migrations holds the schema migrations.
package migrations

import "github.com/AlexGalhardo/vukem/schema"

// All lists the migrations in order; vukem make:migration appends here.
// The editor tables arrive with SDD-005.
func All() []schema.Migration {
	return []schema.Migration{}
}
