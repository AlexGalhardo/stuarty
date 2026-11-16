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

// rendererShipped is the workspace member whose dependencies are shipped in the WebAssembly module:
// the core module of SDD-013 section 4.1, which the Go export embeds.
const rendererShipped = "vukem-renderer"

// rendererTarget is the target whose normal dependencies end up in the shipped module.
const rendererTarget = "wasm32-unknown-unknown"

// gpuHostShipped is the workspace member of the native GPU host, the binary vukem-render
// (SDD-013 section 4.9), and gpuHostTargets are the platforms of the release archives, whose
// dependency closures are joined: a crate that only one platform links (the windows crates, the
// metal bindings) is shipped on that platform and needs its notice.
const gpuHostShipped = "vukem-renderer-gpu"

var gpuHostTargets = []string{
	"x86_64-unknown-linux-gnu", "aarch64-unknown-linux-gnu",
	"x86_64-pc-windows-msvc", "aarch64-pc-windows-msvc",
	"x86_64-apple-darwin", "aarch64-apple-darwin",
}

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

// rustCrates lists the crates compiled into the WebAssembly renderer (the normal dependencies of
// the core crate for the wasm32 target) and into the native GPU host (the normal dependencies of
// its crate for each release platform), without the proc macros and what only they need (they run
// at build time and are not shipped). It returns nothing while renderer/ has no Cargo.lock.
func rustCrates(ctx context.Context, run Runner, root string) ([]Component, error) {
	if _, err := os.Stat(filepath.Join(root, "renderer", "Cargo.lock")); err != nil {
		return nil, nil //nolint:nilerr // no renderer workspace yet
	}

	seen := map[string]bool{}

	var comps []Component

	roots := map[string]string{rendererTarget: rendererShipped}
	targets := []string{rendererTarget}

	for _, t := range gpuHostTargets {
		roots[t] = gpuHostShipped
		targets = append(targets, t)
	}

	for _, target := range targets {
		got, err := crateClosure(ctx, run, root, target, roots[target])
		if err != nil {
			return nil, err
		}

		for i := range got {
			if key := got[i].Name + "@" + got[i].Version; !seen[key] {
				seen[key] = true

				comps = append(comps, got[i])
			}
		}
	}

	slices.SortFunc(
		comps,
		func(a, b Component) int { return strings.Compare(a.Name+"@"+a.Version, b.Name+"@"+b.Version) },
	)

	return comps, nil
}

// crateClosure lists the registry crates that the workspace member named shipped links for target.
func crateClosure(ctx context.Context, run Runner, root, target, shipped string) ([]Component, error) {
	out, err := run(ctx, root, nil, "cargo", "metadata", "--format-version", "1", "--locked",
		"--filter-platform", target, "--manifest-path", rendererManifest)
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

	var queue []string

	for _, id := range md.WorkspaceMembers {
		if pkgs[id].Name == shipped {
			queue = append(queue, id)
		}
	}

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
			c, err := crateComponent(root, p)
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

	return comps, nil
}

// crateTextsDir holds, by hand, the licence text of a crate that ships none in its package (the
// text of its repository, which the manifest names by SPDX id). The folder is named after the crate.
const crateTextsDir = "licenses/crate-texts"

func crateComponent(root string, p cargoPackage) (Component, error) {
	dir := filepath.Dir(p.ManifestPath)

	names, texts, err := readLicenseFiles(dir)
	if err != nil {
		return Component{}, fmt.Errorf("crate %s: %w", p.Name, err)
	}

	if len(names) == 0 {
		fallback := filepath.Join(root, filepath.FromSlash(crateTextsDir), p.Name)
		if fnames, ftexts, ferr := readLicenseFiles(fallback); ferr == nil {
			dir, names, texts = fallback, fnames, ftexts
		}
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
		Group: groupCrates, Name: p.Name, Version: p.Version, SPDX: legacySPDX(p.License), Copyright: copyright,
		Sources: absAll(dir, names), Licenses: outputPaths("crates", p.Name, p.Version, names),
	}, nil
}

// legacySPDX turns the "MIT/Apache-2.0" of older manifests into "MIT OR Apache-2.0": cargo
// accepted "/" as OR before SPDX expressions were required, and no SPDX id contains a slash.
func legacySPDX(expr string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(expr, "/", " OR ")), " ")
}
