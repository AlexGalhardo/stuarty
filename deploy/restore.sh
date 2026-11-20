#!/usr/bin/env bash
# Restores a backup made by backup.sh into the workspace of this installation (SDD-009 section 21.4).
#
#   bash deploy/restore.sh <stuarty-backup-....tar> [--replace] [--without-vault]
#
# On a new host: put your master key at deploy/secrets/master_key, run deploy/install.sh --no-start,
# then this script, then deploy/install.sh again to start. The stack must be stopped. The backup is
# checked against its checksum manifest before anything is written, and the master key must be the one
# that sealed the vault: otherwise the restore stops with a clear message and writes nothing (add
# --without-vault to restore everything else and connect the accounts again). A workspace that already
# has data is refused; --replace moves it to replaced-<time>/ inside the volume first, deleting nothing.
set -euo pipefail

# shellcheck source=deploy/lib.sh
. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

file="" flags=()
while [ $# -gt 0 ]; do
	case "$1" in
		--replace | --without-vault) flags+=("$1"); shift ;;
		-h | --help) sed -n '2,11p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; exit 0 ;;
		-*) die "unknown argument: $1 (see --help)" ;;
		*) [ -z "$file" ] || die "one backup file only"; file="$1"; shift ;;
	esac
done
[ -n "$file" ] || die "usage: bash deploy/restore.sh <stuarty-backup-....tar> [--replace] [--without-vault]"
[ -f "$file" ] || die "$file does not exist"
file="$(cd "$(dirname "$file")" && pwd)/$(basename "$file")"

check_docker
require_env
[ -s "$SECRETS_DIR/master_key" ] || die "$SECRETS_DIR/master_key is missing: a backup does not hold the master key. Put the key that sealed the vault there (or use --without-vault and connect the accounts again)"
if [ -n "$(dc ps -q 2>/dev/null || true)" ]; then
	die "the stack is running: stop it first (docker compose --project-directory $DEPLOY_DIR -f $DEPLOY_DIR/compose.yml stop), restore, then run deploy/install.sh to start it"
fi

if [ -f "$file.sha256" ]; then
	(cd "$(dirname "$file")" && sha256sum -c "$(basename "$file").sha256" >/dev/null) || die "the checksum of $file does not match $file.sha256: the file is damaged"
	say "checksum of the archive: ok"
else
	warn "$file.sha256 is not there: only the manifest inside the backup is checked"
fi

name="import-$(date -u +%Y%m%dT%H%M%SZ)"
cleanup() { dc run --rm --no-deps -T --entrypoint rm stuarty -rf "/data/backups/$name" >/dev/null 2>&1 || true; }
trap cleanup EXIT

say "restoring $(basename "$file")..."
dc run --rm --no-deps -T --entrypoint sh stuarty -c "mkdir -p /data/backups/$name && tar -C /data/backups/$name --strip-components=1 -xf -" <"$file"
dc run --rm --no-deps -T stuarty workspace:restore "/data/backups/$name" "${flags[@]+"${flags[@]}"}"

say ""
say "restored. Start Stuarty with: bash deploy/install.sh"
