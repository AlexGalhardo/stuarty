package timeline

import (
	"errors"

	"github.com/AlexGalhardo/stuarty/core/timeline/migrate"
)

// Migrate chains the migrations of the migrate package from the document's
// schema_version to the current one. A document already at the current version is
// returned unchanged; a newer one is refused with V_SCHEMA_VERSION and nothing is
// written. A template is a timeline document and migrates the same way.
func Migrate(doc RawDoc) (RawDoc, error) {
	out, err := migrate.Migrate(map[string]any(doc))
	if err != nil {
		if me, ok := errors.AsType[*migrate.Error](err); ok {
			return nil, &ViolationsError{[]Violation{{Code: me.Code, Path: me.Path, Message: me.Message}}}
		}
		return nil, err
	}
	return RawDoc(out), nil
}
