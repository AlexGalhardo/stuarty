#!/usr/bin/env bash
# Backs up the workspace of this installation to one file (SDD-009 section 21.4).
#
#   bash deploy/backup.sh [--out <folder>]      (default: deploy/backups/)
#
# It writes <folder>/stuarty-backup-<UTC>.tar and a .sha256 beside it. The server may keep running:
# the database is copied through SQLite's online backup, never as a raw file. The backup holds the
# database, your originals, the library, the fonts and the vault of connected accounts. It does NOT
# hold the master key (deploy/secrets/master_key): keep a copy of that where you keep your other
# secrets. Without it a restored vault cannot be opened and you connect the accounts again.
set -euo pipefail
# The archive holds the database (accounts, password hashes, every project) and the sealed vault:
# only the owner of this host account reads it, whatever the umask of the shell.
umask 077

# shellcheck source=deploy/lib.sh
. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

out_dir="$DEPLOY_DIR/backups"
while [ $# -gt 0 ]; do
	case "$1" in
		--out) out_dir="${2:?--out needs a folder}"; shift 2 ;;
		-h | --help) sed -n '2,11p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; exit 0 ;;
		*) die "unknown argument: $1 (see --help)" ;;
	esac
done

check_docker
require_env
mkdir -p "$out_dir"
out_dir="$(cd "$out_dir" && pwd)"

name="stuarty-backup-$(date -u +%Y%m%dT%H%M%SZ)"
tar_file="$out_dir/$name.tar"

# The copy lives in /data/backups of the volume while it is streamed out, and is removed at the end.
cleanup() {
	rm -f "$tar_file.part"
	dc run --rm --no-deps -T --entrypoint rm stuarty -rf "/data/backups/$name" >/dev/null 2>&1 || true
}
trap cleanup EXIT

say "backing up the workspace of $(env_get STUARTY_DOMAIN) (the server may keep running)..."
dc run --rm --no-deps -T stuarty workspace:backup "/data/backups/$name" >&2
dc run --rm --no-deps -T --entrypoint tar stuarty -C /data/backups -cf - "$name" >"$tar_file.part"

tar -tf "$tar_file.part" | grep -qx "$name/manifest.json" || die "the archive has no manifest: the backup is not usable"
mv "$tar_file.part" "$tar_file"
(cd "$out_dir" && sha256sum "$name.tar" >"$name.tar.sha256")

say ""
say "backup written: $tar_file"
say "checksum:       $tar_file.sha256"
say ""
say "The master key is NOT in this backup. It is in $SECRETS_DIR/master_key: keep a copy of it"
say "where you keep your other secrets. To restore on a new host: put the key at deploy/secrets/master_key,"
say "run deploy/install.sh --no-start, then deploy/restore.sh <this file>."
