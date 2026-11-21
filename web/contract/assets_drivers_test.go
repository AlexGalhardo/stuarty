package contract

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
)

// The asset library on the three surfaces (SDD-011 section 14.5): the encodings the drivers need.

// assetToolCall encodes a library operation as a media tool call. AssetUses has no tool: the detail of an
// entry carries its uses.
func assetToolCall(call string, args map[string]any) (string, map[string]any, error) {
	switch call {
	case "AssetList":
		lib := map[string]any{}

		for _, k := range []string{"tag", "reusable", "origin", "text", "kind", "section", "project"} {
			if v, ok := args[k]; ok {
				lib[k] = v
				delete(args, k)
			}
		}

		args["library"] = lib

		return "media_inspect", args, nil
	case "AssetGet":
		return "media_inspect", args, nil
	case "AssetSetInfo":
		info := map[string]any{}

		for _, k := range []string{"description", "rights_note", "reusable"} {
			if v, ok := args[k]; ok {
				info[k] = v
				delete(args, k)
			}
		}

		args["info"] = info

		return "media_import", args, nil
	case "AssetTag":
		info := map[string]any{}
		if v, ok := args["add"]; ok {
			info["tags"] = v
		}

		if v, ok := args["remove"]; ok {
			info["remove_tags"] = v
		}

		return "media_import", map[string]any{"hash": args["hash"], "info": info}, nil
	case "AssetRename":
		return "media_import", map[string]any{"hash": args["hash"], "info": map[string]any{"name": args["name"]}}, nil
	}

	return "", nil, errors.New("the library has no tool for " + call)
}

// assetArgv encodes a library operation as the command line of the CLI.
func assetArgv(call string, raw json.RawMessage) (string, []string, error) {
	var a map[string]any

	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &a); err != nil {
			return "", nil, err
		}
	}

	str := func(k string) string {
		switch v := a[k].(type) {
		case string:
			return v
		case float64:
			return strconv.Itoa(int(v))
		}

		return ""
	}

	var argv []string

	add := func(flag string) {
		if v := str(flag); v != "" {
			argv = append(argv, "--"+flag, v)
		}
	}

	boolean := func(k string) {
		if b, ok := a[k].(bool); ok {
			if b {
				argv = append(argv, "--"+k)
			} else {
				argv = append(argv, "--no-"+k)
			}
		}
	}

	list := func(k string) {
		if items, ok := a[k].([]any); ok {
			parts := make([]string, len(items))
			for i, it := range items {
				parts[i], _ = it.(string)
			}

			argv = append(argv, "--"+k, strings.Join(parts, ","))
		}
	}

	switch call {
	case "AssetList":
		for _, k := range []string{"kind", "section", "tag", "project", "origin", "text", "cursor", "limit"} {
			add(k)
		}

		boolean("reusable")

		return "asset:list", argv, nil
	case "AssetGet":
		return "asset:show", []string{str("hash")}, nil
	case "AssetUses":
		return "asset:uses", append([]string{str("hash")}, argv...), nil
	case "AssetSetInfo":
		argv = []string{str("hash")}

		add("description")
		if v, ok := a["rights_note"].(string); ok {
			argv = append(argv, "--rights-note", v)
		}

		boolean("reusable")

		return "asset:set", argv, nil
	case "AssetTag":
		argv = []string{str("hash")}

		list("add")
		list("remove")

		return "asset:tag", argv, nil
	case "AssetRename":
		return "asset:rename", []string{str("hash"), "--name", str("name")}, nil
	}

	return "", nil, errors.New("the library has no command for " + call)
}

// importFlags encodes the origin and the info of a MediaImport as the flags of media:import.
func importFlags(raw json.RawMessage) []string {
	var a struct {
		Origin map[string]any `json:"origin"`
		Info   struct {
			Tags        []string `json:"tags"`
			Reusable    *bool    `json:"reusable"`
			Description string   `json:"description"`
		} `json:"info"`
	}

	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &a)
	}

	var argv []string

	for _, f := range [][2]string{
		{"kind", "origin-kind"},
		{"source", "origin-source"},
		{"provider", "provider"},
		{"model", "model"},
		{"prompt", "prompt"},
		{"rights_note", "rights-note"},
	} {
		if v, ok := a.Origin[f[0]].(string); ok && v != "" {
			argv = append(argv, "--"+f[1], v)
		}
	}

	if len(a.Info.Tags) > 0 {
		argv = append(argv, "--tag", strings.Join(a.Info.Tags, ","))
	}

	if a.Info.Reusable != nil && *a.Info.Reusable {
		argv = append(argv, "--reusable")
	}

	if a.Info.Description != "" {
		argv = append(argv, "--description", a.Info.Description)
	}

	return argv
}
