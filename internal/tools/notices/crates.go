package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// rendererManifest is the Cargo workspace of the pixel renderer (ADR-0029).
const rendererManifest = "renderer/Cargo.toml"

// rendererTarget is the target whose normal dependencies end up in the shipped module.
const rendererTarget = "wasm32-unknown-unknown"

type cargoMetadata struct {
	Packages         []cargoPackage `json:"packages"`
	WorkspaceMembers []string       `json:"workspace_members"`
	Resolve          struct {
		Nodes []struct {
			ID   string `json:"id"`
			Deps []struct {
				Pkg      string `json:"pkg"`
				DepKinds []struct {
					Kind *string `json:"kind"`
				} `json:"dep_kinds"`
			} `json:"deps"`
		} `json:"nodes"`
	} `json:"resolve"`
}

type cargoPackage struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	Version      string  `json:"version"`
	License      string  `json:"license"`
	Source       *string `json:"source"`
	ManifestPath string  `json:"manifest_path"`
	Targets      []struct {
		Kind []string `json:"kind"`
	} `json:"targets"`
}

func (p cargoPackage) isProcMacro() bool {
	for _, t := range p.Targets {
		if slices.Contains(t.Kind, "proc-macro") {
			return true
		}
	}

	return false
}

// rustCrates lists the crates compiled into the WebAssembly renderer: the normal dependencies
// of the workspace for the wasm32 target, without the proc macros and what only they need (they run at
// build time and are not shipped). It returns nothing while renderer/ has no Cargo.lock.
func rustCrates(ctx context.Context, run Runner, root string) ([]Component, error) {
	if _, err := os.Stat(filepath.Join(root, "renderer", "Cargo.lock")); err != nil {
		return nil, nil //nolint:nilerr // no renderer workspace yet
	}

	out, err := run(ctx, root, nil, "cargo", "metadata", "--format-version", "1", "--locked",
		"--filter-platform", rendererTarget, "--manifest-path", rendererManifest)
	if err != nil {
		return nil, fmt.Errorf("cargo metadata: %w", err)
	}

	var md cargoMetadata
	if err := json.Unmarshal(out, &md); err != nil {
		return nil, fmt.Errorf("cargo metadata: %w", err)
	}

	pkgs := map[string]cargoPackage{}
	for _, p := range md.Packages {
		pkgs[p.ID] = p
	}

	deps := map[string][]string{}

	for _, n := range md.Resolve.Nodes {
		for _, d := range n.Deps {
			for _, k := range d.DepKinds {
				if k.Kind == nil {
					deps[n.ID] = append(deps[n.ID], d.Pkg)

					break
				}
			}
		}
	}

	seen := map[string]bool{}
	queue := slices.Clone(md.WorkspaceMembers)

	var comps []Component

	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]

		if seen[id] {
			continue
		}

		seen[id] = true

		p := pkgs[id]
		if p.Source != nil {
			c, err := crateComponent(p)
			if err != nil {
				return nil, err
			}

			comps = append(comps, c)
		}

		for _, d := range deps[id] {
			if !pkgs[d].isProcMacro() {
				queue = append(queue, d)
			}
		}
	}

	slices.SortFunc(
		comps,
		func(a, b Component) int { return strings.Compare(a.Name+"@"+a.Version, b.Name+"@"+b.Version) },
	)

	return comps, nil
}

func crateComponent(p cargoPackage) (Component, error) {
	dir := filepath.Dir(p.ManifestPath)

	names, texts, err := readLicenseFiles(dir)
	if err != nil {
		return Component{}, fmt.Errorf("crate %s: %w", p.Name, err)
	}

	if len(names) == 0 {
		return Component{}, fmt.Errorf("crate %s: no licence file in %s", p.Name, dir)
	}

	var copyright string

	// The Apache-2.0 text has no copyright line of the author, only the boilerplate of its appendix.
	for _, n := range names {
		if isNotice(n) || detect(texts[n]) == "Apache-2.0" {
			continue
		}

		if copyright = copyrightLine(texts[n]); copyright != "" {
			break
		}
	}

	return Component{
		Group: groupCrates, Name: p.Name, Version: p.Version, SPDX: p.License, Copyright: copyright,
		Sources: absAll(dir, names), Licenses: outputPaths("crates", p.Name, p.Version, names),
	}, nil
}
