package migrate

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// Current is the schema version this binary writes and supports. The timeline
// package asserts in a test that its CurrentSchemaVersion is the same number.
const Current = 1

// Violation codes the chain reports; they are the V_* codes of SDD-001 §4.
const (
	CodeSchemaVersion = "V_SCHEMA_VERSION"
	CodeShape         = "V_SHAPE"
)

// Doc is a document as decoded JSON, numbers as json.Number.
type Doc = map[string]any

// Step is the pure function "vN to vN+1".
type Step func(Doc) (Doc, error)

// Steps maps a version N to the step that produces version N+1.
type Steps map[int]Step

// Error is a document that cannot be migrated, with the code and the path of the
// violation it stands for.
type Error struct {
	Code    string
	Path    string
	Message string
}

func (e *Error) Error() string { return e.Message }

// registry holds the steps of this binary. Version 1 is the first schema, so the
// chain is empty; a breaking change adds v1to2 here and raises Current.
var registry = Steps{}

func init() {
	if err := Check(Current, registry); err != nil {
		panic("migrate: " + err.Error())
	}
}

// Check reports the first version between 1 and current that has no step.
func Check(current int, steps Steps) error {
	for v := 1; v < current; v++ {
		if steps[v] == nil {
			return fmt.Errorf("no migration from schema_version %d to %d", v, v+1)
		}
	}
	return nil
}

// Migrate chains the steps from the document's schema_version to Current. A
// document already at Current is returned unchanged; a newer one is refused with
// V_SCHEMA_VERSION naming both numbers, and nothing is written.
func Migrate(doc Doc) (Doc, error) {
	return Run(doc, Current, registry)
}

// Run is Migrate over an explicit chain; tests use it to exercise versions that
// do not exist yet.
func Run(doc Doc, current int, steps Steps) (Doc, error) {
	version, err := versionOf(doc)
	if err != nil {
		return nil, err
	}
	if version > current {
		return nil, &Error{
			Code: CodeSchemaVersion,
			Path: "schema_version",
			Message: fmt.Sprintf(
				"schema_version %d is newer than the %d this program supports; upgrade the program",
				version,
				current,
			),
		}
	}
	for ; version < current; version++ {
		step := steps[version]
		if step == nil {
			return nil, &Error{
				Code:    CodeSchemaVersion,
				Path:    "schema_version",
				Message: fmt.Sprintf("no migration from schema_version %d", version),
			}
		}
		if doc, err = step(doc); err != nil {
			return nil, fmt.Errorf("migrating schema_version %d to %d: %w", version, version+1, err)
		}
		doc["schema_version"] = json.Number(strconv.Itoa(version + 1))
	}
	return doc, nil
}

func versionOf(doc Doc) (int, error) {
	n, ok := doc["schema_version"].(json.Number)
	if !ok {
		return 0, &Error{
			Code:    CodeShape,
			Path:    "schema_version",
			Message: "schema_version is required and must be an integer",
		}
	}
	version, err := strconv.Atoi(string(n))
	if err != nil || version < 0 {
		return 0, &Error{
			Code:    CodeSchemaVersion,
			Path:    "schema_version",
			Message: "schema_version must be a non-negative integer, got " + string(n),
		}
	}
	return version, nil
}
