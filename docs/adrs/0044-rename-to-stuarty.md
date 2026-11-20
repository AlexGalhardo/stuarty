# ADR-0044: The code is renamed to Stuarty in one epic, with a workspace migration

- Status: Accepted (2026-11-18)
- Deciders: author
- Amends: ADR-0001 (the module path and the binary name)
- Links: author decisions D-047, D-068; ADR-0031; SDD-000, SDD-005, SDD-009

## Context

The repository is already `AlexGalhardo/stuarty`. The former name, `vukem-video`, is in about 1,190 tracked files: the Go module path, the binary, the environment variables (`VUKEM_VIDEO_*`), the workspace folder (`~/.vukem-video`), the container image, the installers, the documentation site and the specifications that tests read.

## Decision

1. **One dedicated epic, at the start of the work, renames everything**: the module path, the binary, the environment variables, the workspace folder, the container image, the installers, the site and the specifications. Other epics start after it, so that no open branch carries the old name.
2. **Existing installations are migrated.** On first start the product finds a workspace of the former name and moves or adopts it after a backup; the former environment variables are read for a deprecation period that the epic states, with a warning.
3. **The version numbering continues.** The first release under the new name is v0.30.0; nothing restarts at v0.1.0.
4. **Until that epic is merged, documents name the product Stuarty in prose and keep the real identifiers** (`vukem-video serve`, `VUKEM_VIDEO_HOME`), because those are what the code accepts.
5. **The vukem framework is not renamed**: it is another project (ADR-0047).

## Alternatives

- **Rename gradually**, as each epic touches a file. Rejected: months with two names.
- **Rename only the public surface.** Rejected: the code would keep saying a name the product no longer has.

## Consequences

Positive: one breaking change, early, with a migration.

Negative: the epic touches almost every file and blocks the others for its duration.

Negative: published links, the container image name and the documentation site address change; the epic lists each redirect it can keep.
