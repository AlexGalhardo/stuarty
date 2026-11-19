package contract

import (
	"context"
	"encoding/json"
	"slices"
	"sort"
	"strings"

	"github.com/AlexGalhardo/stuarty/service"
)

// The library operations (SDD-018) on the four drivers. The switches of drivers_test.go end by asking these
// functions, which keeps the mapping of one domain in one place.

// libraryArgs is every member a step of the library can carry.
type libraryArgs struct {
	Name              string            `json:"name"`
	Description       string            `json:"description"`
	Body              string            `json:"body"`
	Scope             string            `json:"scope"`
	Tags              []string          `json:"tags"`
	Defaults          map[string]string `json:"defaults"`
	Values            map[string]string `json:"values"`
	Query             string            `json:"query"`
	Kinds             []string          `json:"kinds"`
	IncludeUnreviewed bool              `json:"include_unreviewed"`
	Path              string            `json:"path"`
	Source            string            `json:"source"`
	Replace           bool              `json:"replace"`
	Names             []string          `json:"names"`
}

func libraryStep(s step) (libraryArgs, error) {
	var a libraryArgs

	if len(s.Args) > 0 {
		if err := json.Unmarshal(s.Args, &a); err != nil {
			return a, err
		}
	}

	return a, nil
}

// libraryCall runs a step on a Service. ok is false when the step is not a library operation.
func libraryCall(ctx context.Context, svc service.Service, s step) (res any, ok bool, err error) {
	switch s.Call {
	case "MemoryList":
		res, err = invoke(ctx, s, svc.MemoryList)
	case "MemoryGet":
		res, err = invoke(ctx, s, svc.MemoryGet)
	case "MemoryWrite":
		res, err = invoke(ctx, s, svc.MemoryWrite)
	case "MemoryRemove":
		res, err = invoke(ctx, s, svc.MemoryRemove)
	case "SkillList":
		res, err = invoke(ctx, s, svc.SkillList)
	case "SkillGet":
		res, err = invoke(ctx, s, svc.SkillGet)
	case "SkillWrite":
		res, err = invoke(ctx, s, svc.SkillWrite)
	case "SkillImport":
		res, err = invoke(ctx, s, svc.SkillImport)
	case "SkillMarkReviewed":
		res, err = invoke(ctx, s, svc.SkillMarkReviewed)
	case "SkillLint":
		res, err = invoke(ctx, s, svc.SkillLint)
	case "SkillExport":
		res, err = invoke(ctx, s, svc.SkillExport)
	case "PromptList":
		res, err = invoke(ctx, s, svc.PromptList)
	case "PromptGet":
		res, err = invoke(ctx, s, svc.PromptGet)
	case "PromptWrite":
		res, err = invoke(ctx, s, svc.PromptWrite)
	case "PromptRender":
		res, err = invoke(ctx, s, svc.PromptRender)
	case "LibrarySearch":
		res, err = invoke(ctx, s, svc.LibrarySearch)
	case "LibraryReindex":
		res, err = invoke(ctx, s, svc.LibraryReindex)
	default:
		return nil, false, nil
	}

	return res, true, err
}

// libraryTool maps a step to a call of the MCP tool library. ok is false for a step that is not a library one,
// and err for one the tool does not have (the tool removes nothing, imports nothing, reviews nothing).
func libraryTool(s step) (tool string, args map[string]any, ok bool, err error) {
	a, err := libraryStep(s)
	if err != nil {
		return "", nil, true, err
	}

	args = map[string]any{}
	set := func(k string, v any) {
		switch x := v.(type) {
		case string:
			if x != "" {
				args[k] = x
			}
		case []string:
			if len(x) > 0 {
				args[k] = x
			}
		case map[string]string:
			if len(x) > 0 {
				args[k] = x
			}
		}
	}

	kind := map[string]string{
		"MemoryList": "memory", "MemoryGet": "memory", "MemoryWrite": "memory",
		"SkillList": "skill", "SkillGet": "skill", "SkillWrite": "skill",
		"PromptList": "prompt", "PromptGet": "prompt", "PromptWrite": "prompt",
	}[s.Call]

	switch s.Call {
	case "MemoryList", "SkillList", "PromptList":
		args["action"], args["kind"] = "list", kind
	case "MemoryGet", "SkillGet", "PromptGet":
		args["action"], args["kind"] = "get", kind

		set("name", a.Name)
	case "MemoryWrite", "SkillWrite", "PromptWrite":
		args["action"], args["kind"] = "write", kind

		set("name", a.Name)
		set("description", a.Description)
		set("body", a.Body)
		set("tags", a.Tags)
	case "PromptRender":
		args["action"] = "render"

		set("name", a.Name)
		set("values", a.Values)
	case "SkillLint":
		args["action"] = "lint"

		set("name", a.Name)
	case "LibrarySearch":
		args["action"] = "search"

		set("query", a.Query)
		set("kinds", a.Kinds)
	default:
		return "", nil, false, nil
	}

	return "library", args, true, nil
}

// libraryCommand maps a step to a command of the CLI.
func libraryCommand(s step) (name string, argv []string, stdin string, ok bool, err error) {
	a, err := libraryStep(s)
	if err != nil {
		return "", nil, "", true, err
	}

	flag := func(k, v string) []string {
		if v == "" {
			return nil
		}

		return []string{"--" + k, v}
	}

	each := func(k string, vs []string) []string {
		var out []string
		for _, v := range vs {
			out = append(out, "--"+k, v)
		}

		return out
	}

	pairs := func(k string, m map[string]string) []string {
		keys := make([]string, 0, len(m))
		for key := range m {
			keys = append(keys, key)
		}

		sort.Strings(keys)

		var out []string
		for _, key := range keys {
			out = append(out, "--"+k, key+"="+m[key])
		}

		return out
	}

	switch s.Call {
	case "MemoryList":
		return "memory:list", append(flag("scope", a.Scope), boolFlag("all", a.IncludeUnreviewed)...), "", true, nil
	case "MemoryGet":
		return "memory:show", []string{a.Name}, "", true, nil
	case "MemoryWrite":
		argv = slices.Concat(
			nameArg(a.Name),
			flag("description", a.Description),
			flag("scope", a.Scope),
			each("tag", a.Tags),
		)

		return "memory:write", append(argv, "--body-file", "-"), a.Body, true, nil
	case "MemoryRemove":
		return "memory:remove", []string{a.Name}, "", true, nil
	case "SkillList":
		return "skill:list", boolFlag("all", a.IncludeUnreviewed), "", true, nil
	case "SkillGet":
		return "skill:show", []string{a.Name}, "", true, nil
	case "SkillWrite":
		argv = slices.Concat([]string{a.Name}, flag("description", a.Description))

		return "skill:write", append(argv, "--body-file", "-"), a.Body, true, nil
	case "SkillImport":
		return "skill:import", slices.Concat(
			[]string{a.Path},
			flag("source", a.Source),
			boolFlag("replace", a.Replace),
		), "", true, nil
	case "SkillExport":
		return "skill:export", a.Names, "", true, nil
	case "SkillLint":
		return "skill:lint", []string{a.Name}, "", true, nil
	case "PromptList":
		return "prompt:list", each("tag", a.Tags), "", true, nil
	case "PromptGet":
		return "prompt:show", []string{a.Name}, "", true, nil
	case "PromptWrite":
		argv = slices.Concat(
			[]string{a.Name},
			flag("description", a.Description),
			each("tag", a.Tags),
			pairs("default", a.Defaults),
		)

		return "prompt:write", append(argv, "--body-file", "-"), a.Body, true, nil
	case "PromptRender":
		return "prompt:render", append([]string{a.Name}, pairs("value", a.Values)...), "", true, nil
	case "LibrarySearch":
		argv = slices.Concat(strings.Fields(a.Query), each("kind", a.Kinds), boolFlag("all", a.IncludeUnreviewed))

		return "library:search", argv, "", true, nil
	case "LibraryReindex":
		return "library:reindex", nil, "", true, nil
	}

	return "", nil, "", false, nil
}

func nameArg(name string) []string {
	if name == "" {
		return nil
	}

	return []string{name}
}

func boolFlag(k string, v bool) []string {
	if !v {
		return nil
	}

	return []string{"--" + k}
}
