# 25: VPS package: Caddy, install, update and backup

## Goal

A creator with a small rented server runs one command and has their own Stuarty on their own domain, with HTTPS, and knows how to update it and how to get their data back if the server dies.

## Context and related decisions

- ADR-0034, ADR-0033 (the master key on a server), ADR-0019 (network mode), ADR-0025 (releases, as amended), ADR-0045 (no release pipeline yet).
- Author decisions D-057, D-074. PRD-003 RF-218, RF-203, RF-204; PRD-004 NFR-060.
- Research note `web/07-stack-desktop-web-self-hosted`.

## Scope

- Modules: `deploy/` (new: `compose.yml`, `Caddyfile`, `.env.example`, `install.sh`, `update.sh`, `backup.sh`, `restore.sh`), the Dockerfile in `.config/` (only what the package needs), `scripts/image-smoke.sh`, `service/` (workspace export and import if the existing backup does not cover the vault and the library), `site/` or `docs/` (the installation guide source, published by epic 40), SDD-009 (the deployment section).
- One host, Docker Compose, two containers: Stuarty and Caddy, both with fixed version tags.

## Out of scope

- Publishing images to a registry and signing them: that returns with the releases (roadmap section 4). The package builds the image locally from a tagged source until then.
- Kubernetes, clusters, several replicas (the scheduler lease allows it later; it is not offered).
- Managed backups to a cloud provider.

## Dependencies

24.

## Technical design

- **Compose**: Stuarty listens only on the internal network; Caddy terminates TLS with automatic certificates for the domain in `.env` and proxies to it, including the SSE stream (no buffering) and large uploads (limits stated). Volumes: the workspace, Caddy's data. The master key is a Docker secret read through `STUARTY_MASTER_KEY_FILE` (epic 04).
- **`install.sh`**: checks Docker and Compose versions, asks for the domain and the admin e-mail, generates the master key and the first admin password, writes `.env` with restrictive permissions, starts the stack, waits for health, and prints the URL and where the key is. It is idempotent and never overwrites an existing key.
- **`update.sh`**: backs up first, builds or pulls the given version, runs migrations by starting the new container, checks health, and rolls back to the previous image and backup when health fails.
- **`backup.sh` and `restore.sh`**: a consistent snapshot of the database (the SQLite backup API through a server command, never a raw copy of a live file), the media store, the library files and the vault file; the master key is **not** in the backup, and the script says so and says where it is. Restore into an empty workspace, verified by a checksum manifest.
- **Hardening**: the container runs as a non-root user with a read-only root file system and dropped capabilities; only ports 80 and 443 are published; network mode refuses to start without a password or with the default one (existing behaviour, re-tested).
- **Outbound on a server**: Caddy's certificate requests are the one outbound the package itself needs; the guide states it.

## Tasks

- [ ] 1. Amend SDD-009 with the deployment layout, what a backup contains and the update procedure. Verify: `task lint:md` exits 0.
- [ ] 2. Write `compose.yml`, the `Caddyfile` and `.env.example` with fixed tags. Verify: `docker compose -f deploy/compose.yml config` exits 0 and `task image:build && task image:smoke` pass.
- [ ] 3. Write `install.sh` and test it in a clean container that has Docker available, or with a local domain and Caddy's internal certificates. Verify: `bash deploy/test/install-test.sh` (new) exits 0.
- [ ] 4. Implement the consistent workspace backup command if missing, and `backup.sh` and `restore.sh`. Verify: `go test ./service/... -run TestWorkspaceBackup` passes and `bash deploy/test/backup-restore-test.sh` (new) exits 0 with matching checksums.
- [ ] 5. Write `update.sh` with rollback. Verify: `bash deploy/test/update-test.sh` (new) exits 0, including a forced health failure that rolls back.
- [ ] 6. Check hardening. Verify: `bash scripts/image-smoke.sh stuarty:local` passes with the non-root and read-only checks.
- [ ] 7. Write the installation, update, backup and restore guide in English and in Portuguese. Verify: `task lint:md` exits 0 and `bash scripts/docs-commands-smoke.sh` passes for the guide's commands.
- [ ] 8. One real installation on a rented server by the author, or marked as not verified. Verify: the result or the note is in the hand-off.

## Test strategy

- Scripted: install, backup and restore, update and rollback, in containers on the developer machine.
- Image: the smoke script with hardening checks.
- Manual: one real server with a real domain.

## Acceptance criteria

1. On a clean host with Docker, `install.sh` ends with Stuarty reachable over HTTPS on the given domain and prints the first login.
2. Running `install.sh` again changes nothing and keeps the master key.
3. `backup.sh` then `restore.sh` into an empty host reproduces projects, media, library and connections (connections work once the master key is put back).
4. `update.sh` with a version whose health check fails leaves the previous version running with its data.
5. The container runs as non-root with a read-only root file system; only 80 and 443 are published.
6. Every image tag in `deploy/` is a fixed version; the HTTP server is Caddy.

## Risks and mitigation

- **A backup without the master key surprises the user at restore time.** Mitigation: both scripts say it in plain words; the guide has a section on keeping the key.
- **A raw copy of a live SQLite file.** Mitigation: the backup command uses the online backup API; a test writes during the backup.
- **STOP** before choosing or changing a base image or the Caddy version line, and for anything that needs a registry.

## Size

M

## Hand-off for review

The diff of `epic/25-vps-package`; the outputs of the three scripted tests; the image smoke output; whether a real server was verified.
