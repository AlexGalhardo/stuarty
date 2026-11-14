package timeline

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// migration moves a raw document from one schema version to the next (SDD-001 §6).
type migration func(RawDoc) (RawDoc, error)

// migrations[n] is the pure function "vN to vN+1". Version 1 is the first schema,
// so the chain is empty; a breaking change adds v1to2 here and raises
// CurrentSchemaVersion.
var migrations = map[int]migration{}

// Migrate chains the migrations from the document's schema_version to the current
// one. A document already at the current version is returned unchanged; a newer one
// is refused with V_SCHEMA_VERSION and nothing is written.
func Migrate(doc RawDoc) (RawDoc, error) {
	return migrateChain(doc, CurrentSchemaVersion, migrations)
}

func migrateChain(doc RawDoc, current int, steps map[int]migration) (RawDoc, error) {
	version, err := rawVersion(doc)
	if err != nil {
		return nil, err
	}
	if version > current {
		return nil, &ViolationsError{[]Violation{{
			Code: VSchemaVersion, Path: "schema_version",
			Message: fmt.Sprintf("schema_version %d is newer than the %d this program supports; upgrade the program", version, current),
		}}}
	}
	for ; version < current; version++ {
		step, ok := steps[version]
		if !ok {
			return nil, &ViolationsError{[]Violation{{
				Code: VSchemaVersion, Path: "schema_version",
				Message: fmt.Sprintf("no migration from schema_version %d", version),
			}}}
		}
		if doc, err = step(doc); err != nil {
			return nil, fmt.Errorf("migrating schema_version %d to %d: %w", version, version+1, err)
		}
		doc["schema_version"] = json.Number(strconv.Itoa(version + 1))
	}
	return doc, nil
}

func rawVersion(doc RawDoc) (int, error) {
	n, ok := doc["schema_version"].(json.Number)
	if !ok {
		return 0, &ViolationsError{[]Violation{{Code: VShape, Path: "schema_version", Message: "schema_version is required and must be an integer"}}}
	}
	version, err := strconv.Atoi(string(n))
	if err != nil || version < 1 {
		return 0, &ViolationsError{[]Violation{{Code: VSchemaVersion, Path: "schema_version", Message: "schema_version must be a positive integer, got " + string(n)}}}
	}
	return version, nil
}
