#!/usr/bin/env bash
# Test of deploy/backup.sh and deploy/restore.sh with the real stuarty binary (epic 25 task 4):
#
#   bash deploy/test/backup-restore-test.sh
#
# Host A has a workspace with a user, a connected account (a credential in the vault), an original and
# a library file. backup.sh makes a backup; host B, a new empty one, gets the master key of A and
# restore.sh restores it: the checksums of the originals and the library match, the user and the
# connection are there, and the vault opens with the key. Then the refusals: another master key, a
# damaged archive, a workspace that already has data. The master key is never in the archive.
# docker and curl are the stand-ins of deploy/test/stub/; the binary is built from this checkout
# unless STUARTY_TEST_BIN names one. Exit 0 when everything holds.
set -euo pipefail
# shellcheck source=deploy/test/common.sh
. "$(dirname "${BASH_SOURCE[0]}")/common.sh"

new_work
bin="$(build_binary)"
export STUB_BIN="$bin"

# make_host NAME [MASTER_KEY_FILE]: a copy of deploy/ with its keys (install.sh --no-start) and an empty volume.
make_host() {
	local name="$1" key="${2:-}"
	local root="$WORK/$name"
	mkdir -p "$root/repo/deploy" "$root/data"
	cp "$REPO_DIR"/deploy/{compose.yml,Caddyfile,.env.example,lib.sh,install.sh,backup.sh,restore.sh} "$root/repo/deploy/"
	if [ -n "$key" ]; then
		mkdir -p "$root/repo/deploy/secrets"
		cp "$key" "$root/repo/deploy/secrets/master_key"
	fi
	bash "$root/repo/deploy/install.sh" --domain "$name.example.com" --email me@example.com --version 0.30.0 --no-start >/dev/null </dev/null
}

A="$WORK/a"
make_host a
KEY_A="$A/repo/deploy/secrets/master_key"
DATA_A="$A/data"
export STUB_DATA="$DATA_A"
export STUB_KEY_FILE="$KEY_A"

stu() { STUARTY_HOME="$DATA_A" STUARTY_MASTER_KEY_FILE="$KEY_A" "$bin" "$@"; }

# Seed host A.
printf 'c4n4ry correct horse battery\n' | stu user:create ana@example.com --admin >/dev/null
printf 'a-secret-token\n' | stu connection:add --kind test --label Blog --endpoint https://blog.example.com --accepts text >/dev/null
mkdir -p "$DATA_A/store/ab" "$DATA_A/library/memory" "$DATA_A/exports/p" "$DATA_A/artifacts/x"
head -c 200000 /dev/urandom >"$DATA_A/store/ab/abcdef0123"
printf 'a note\n' >"$DATA_A/library/memory/note.md"
printf 'an export\n' >"$DATA_A/exports/p/j.mp4"
pass "host A seeded: a user, a connection, an original, a library file"

# 1. The backup.
out="$WORK/backups"
(cd "$A/repo" && bash deploy/backup.sh --out "$out") >"$WORK/backup.log" 2>&1 || { cat "$WORK/backup.log" >&2; fail "backup.sh failed"; }
tarfile="$(ls "$out"/stuarty-backup-*.tar)"
assert "the archive and its checksum exist" test -s "$tarfile" -a -s "$tarfile.sha256"
assert "the checksum of the archive verifies" bash -c "cd '$out' && sha256sum -c '$(basename "$tarfile").sha256'"
assert "the archive holds the manifest and the database" bash -c "tar -tf '$tarfile' | grep -q '/manifest.json$' && tar -tf '$tarfile' | grep -q '/db.sqlite$' && tar -tf '$tarfile' | grep -q '/vault.json$'"
assert "the archive leaves out the exports" bash -c "! tar -tf '$tarfile' | grep -q 'exports/'"
assert "the backup said that the master key is not in it" grep -q 'master key is NOT in this backup' "$WORK/backup.log"
assert "the copy was removed from the volume" bash -c "! ls '$DATA_A/backups' | grep -q stuarty-backup"
raw_key="$(tr -d '\n' <"$KEY_A")"
mkdir -p "$WORK/unpacked"
tar -C "$WORK/unpacked" -xf "$tarfile"
assert "the master key is not in any file of the archive" bash -c "! grep -rqF '$raw_key' '$WORK/unpacked'"
assert "nothing named master_key is in the archive" bash -c "! tar -tf '$tarfile' | grep -qi 'master'"

# 2. Restore on a new host that has the master key of A.
make_host b "$KEY_A"
B="$WORK/b"
export STUB_DATA="$B/data" STUB_KEY_FILE="$B/repo/deploy/secrets/master_key"
(cd "$B/repo" && bash deploy/restore.sh "$tarfile") >"$WORK/restore.log" 2>&1 || { cat "$WORK/restore.log" >&2; fail "restore.sh failed"; }
pass "restore.sh restored into the empty host"
assert "the original is identical" cmp "$DATA_A/store/ab/abcdef0123" "$B/data/store/ab/abcdef0123"
assert "the library file is identical" cmp "$DATA_A/library/memory/note.md" "$B/data/library/memory/note.md"
assert "the exports were not restored" test ! -e "$B/data/exports/p/j.mp4"
assert "the import copy was removed" bash -c "! ls '$B/data/backups' | grep -q import-"
[ "$(tree_sums "$DATA_A/store")" = "$(tree_sums "$B/data/store")" ] && pass "the checksums of the media store match" || fail "the media store differs"
[ "$(tree_sums "$DATA_A/library")" = "$(tree_sums "$B/data/library")" ] && pass "the checksums of the library match" || fail "the library differs"

stub_b() { STUARTY_HOME="$B/data" STUARTY_MASTER_KEY_FILE="$B/repo/deploy/secrets/master_key" "$bin" "$@"; }
conns="$(stub_b connection:list --json)"
case "$conns" in *Blog*) pass "the connection is in the restored database" ;; *) fail "no connection in: $conns" ;; esac
# The vault opens with the key that was put back: doctor reads it.
doc="$(stub_b doctor --full --json 2>&1 || true)"
case "$doc" in *'"name":"vault","ok":true'*) pass "the vault opens with the master key of host A (doctor)" ;; *) fail "the vault does not open on host B: $doc" ;; esac
head -c 32 /dev/urandom | base64 | tr -d '\n' >"$WORK/wrong.key"
doc="$(STUARTY_HOME="$B/data" STUARTY_MASTER_KEY_FILE="$WORK/wrong.key" "$bin" doctor --full --json 2>&1 || true)"
case "$doc" in *'"name":"vault","ok":false'*) pass "the same vault does not open with another key (doctor)" ;; *) fail "doctor did not flag the wrong key: $doc" ;; esac

# 3. A restore with another master key is refused, with a clear message, and writes nothing.
head -c 32 /dev/urandom | base64 | tr -d '\n' >"$WORK/other.key"
make_host c "$WORK/other.key"
C="$WORK/c"
export STUB_DATA="$C/data" STUB_KEY_FILE="$C/repo/deploy/secrets/master_key"
if (cd "$C/repo" && bash deploy/restore.sh "$tarfile") >"$WORK/restore-c.log" 2>&1; then fail "a restore with another master key succeeded"; fi
grep -q 'master key' "$WORK/restore-c.log" && grep -q -- '--without-vault' "$WORK/restore-c.log" && pass "another master key is refused with a clear message" || { cat "$WORK/restore-c.log" >&2; fail "the message about the master key is unclear"; }
assert "the refused restore wrote no database" test ! -s "$C/data/db.sqlite"
(cd "$C/repo" && bash deploy/restore.sh "$tarfile" --without-vault) >/dev/null 2>&1 || fail "--without-vault restore failed"
assert "--without-vault restored the database and no vault" bash -c "test -s '$C/data/db.sqlite' && test ! -e '$C/data/vault.json'"

# 4. A damaged archive is refused before anything is written.
make_host d "$KEY_A"
D="$WORK/d"
export STUB_DATA="$D/data" STUB_KEY_FILE="$D/repo/deploy/secrets/master_key"
cp "$tarfile" "$WORK/damaged.tar"
cp "$tarfile.sha256" "$WORK/damaged.tar.sha256"
sed -i "s/stuarty-backup-[0-9TZ]*\.tar/damaged.tar/" "$WORK/damaged.tar.sha256"
printf 'x' >>"$WORK/damaged.tar"
if (cd "$D/repo" && bash deploy/restore.sh "$WORK/damaged.tar") >"$WORK/restore-d.log" 2>&1; then fail "a damaged archive was restored"; fi
grep -q 'checksum' "$WORK/restore-d.log" && pass "a damaged archive is refused by its checksum" || { cat "$WORK/restore-d.log" >&2; fail "no checksum message"; }
assert "nothing was written on that host" test ! -s "$D/data/db.sqlite"

# 5. A workspace that already has data is refused; --replace keeps it aside.
export STUB_DATA="$B/data" STUB_KEY_FILE="$B/repo/deploy/secrets/master_key"
if (cd "$B/repo" && bash deploy/restore.sh "$tarfile") >"$WORK/restore-e.log" 2>&1; then fail "a restore over data succeeded"; fi
grep -q 'already has data' "$WORK/restore-e.log" && pass "a workspace with data is refused" || { cat "$WORK/restore-e.log" >&2; fail "no message about existing data"; }
(cd "$B/repo" && bash deploy/restore.sh "$tarfile" --replace) >"$WORK/restore-f.log" 2>&1 || { cat "$WORK/restore-f.log" >&2; fail "--replace failed"; }
assert "--replace kept the old data aside" bash -c "ls -d '$B'/data/replaced-*/store >/dev/null"

# 6. A running stack blocks a restore.
echo "0.30.0" >"$STUB_STATE/running"
if (cd "$B/repo" && bash deploy/restore.sh "$tarfile") >"$WORK/restore-g.log" 2>&1; then fail "a restore with the stack running succeeded"; fi
grep -q 'stack is running' "$WORK/restore-g.log" && pass "a running stack blocks a restore" || fail "no message about the running stack"

echo "backup-restore-test: all checks passed"
