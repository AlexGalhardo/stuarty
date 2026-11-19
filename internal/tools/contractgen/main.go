// Command contractgen writes the contracts of ADR-0014 from the Go types: the JSON
// Schema of the timeline document, one schema per command, the registry snapshot
// commands.json, errors.json and limits.json. It is run by `task codegen` from the
// repository root; a CI step fails when the working tree changes afterwards.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/command"
	coreimage "github.com/AlexGalhardo/stuarty/core/image"
	"github.com/AlexGalhardo/stuarty/core/lint"
	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// httpStatus is the status of each core error code (SDD-000 section 8).
var httpStatus = map[string]int{
	command.CodeValidation: 400, command.CodeInvalidTime: 400, command.CodeUnknownCommand: 400,
	command.CodeNotFound:         404,
	command.CodeRevisionConflict: 409, command.CodeIdempotencyMismatch: 409, command.CodeOverlap: 409,
	command.CodeInvalidState: 409, command.CodeInUse: 409, command.CodeNotEmpty: 409, command.CodeIDExists: 409,
	command.CodeLimit: 422, command.CodeOutOfSource: 422, command.CodeFontUnknown: 422, command.CodeSlot: 422,
	command.CodeTypeMismatch: 422,
	command.CodeInternal:     500,
}

func main() {
	if err := run("core/schema"); err != nil {
		fmt.Fprintln(os.Stderr, "contractgen:", err)
		os.Exit(1)
	}
}

func write(path string, v any) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o600)
}

func run(out string) error {
	if err := write(filepath.Join(out, "timeline.v1.schema.json"), timeline.Schema()); err != nil {
		return err
	}
	if err := writeCommands(out); err != nil {
		return err
	}

	if err := writeImageCommands(out); err != nil {
		return err
	}
	var errs []map[string]any
	for _, code := range command.Codes() {
		errs = append(
			errs,
			map[string]any{"code": code, "retryable": command.Errorf(code, "x").Retryable, "http": httpStatus[code]},
		)
	}
	if err := write(filepath.Join(out, "errors.json"), errs); err != nil {
		return err
	}
	if err := write(filepath.Join(out, "lint-rules.json"), lintRules()); err != nil {
		return err
	}
	return write(filepath.Join(out, "limits.json"), map[string]any{
		"tracks": timeline.MaxTracks, "items": timeline.MaxItems, "assets": timeline.MaxAssets,
		"caption_words": timeline.MaxCaptionWords, "words_per_caption": 60, "keyframes_per_property": 100,
		"max_frames": timeline.MaxFrames, "batch_operations": 200, "batch_bytes": 512 * 1024,
	})
}

// lintRules is the rule table of core/lint as data for the surfaces: the code, the
// severity, the depths it runs at, whether it blocks an export and the key
// of its title in the catalogue of texts.
func lintRules() []map[string]any {
	var rules []map[string]any
	for _, r := range lint.Catalog() {
		rules = append(rules, map[string]any{
			"code":     r.Code,
			"severity": r.Severity,
			"depth":    strings.Split(strings.ReplaceAll(r.Run, " ", ""), ","),
			"blocking": lint.Blocking(r.Code),
			"key":      "lint.rule." + r.Code + ".title",
		})
	}
	return rules
}

// writeImageCommands writes the registry of the image domain (SDD-021 section 3): one schema per operation under
// core/schema/image/commands and the snapshot core/schema/image/commands.json.
func writeImageCommands(out string) error {
	return writeRegistry(coreimage.Registry(), filepath.Join(out, "image"))
}

func writeCommands(out string) error {
	return writeRegistry(command.Default(), out)
}

func writeRegistry(reg *command.Registry, out string) error {
	var list []map[string]any
	for _, c := range reg.List() {
		raw, err := reg.Schema(c.Name)
		if err != nil {
			return err
		}
		var schema map[string]any
		if err := json.Unmarshal(raw, &schema); err != nil {
			return err
		}
		if err := write(filepath.Join(out, "commands", c.Name+".schema.json"), schema); err != nil {
			return err
		}
		var example any
		if err := json.Unmarshal([]byte(c.Example), &example); err != nil {
			return fmt.Errorf("%s: %w", c.Name, err)
		}
		list = append(list, map[string]any{
			"name":        c.Name,
			"summary":     c.Summary,
			"description": c.Description,
			"args_schema": schema,
			"example":     example,
			"errors":      slices.Clone(c.Errors),
			"scope":       c.Scope,
			"internal":    c.Internal,
		})
	}
	return write(filepath.Join(out, "commands.json"), list)
}
