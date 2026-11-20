#!/usr/bin/env bash
# Test of deploy/update.sh and its rollback (epic 25 task 5):
#
#   bash deploy/test/update-test.sh
#
# A host installed at v0.30.0 (a git checkout with tags, the stand-ins of docker and curl, the real
# stuarty binary for the backup and the restore) with a workspace that holds a user, an original and a
# library file. Updating to a healthy version backs up first and moves to it. Updating to a version
# whose health check fails (forced) writes to the data when it starts, then rolls back: the previous
# version runs again, the checkout and .env are back, and the data is the one of the backup. A failed
# build changes nothing. Exit 0 when everything holds.
set -euo pipefail
# shellcheck source=deploy/test/common.sh
. "$(dirname "${BASH_SOURCE[0]}")/common.sh"

new_work
bin="$(build_binary)"
export STUB_BIN="$bin"

HOST="$WORK/host"
REPO="$HOST/repo"
DEP="$REPO/deploy"
export STUB_DATA="$HOST/data"
mkdir -p "$DEP" "$STUB_DATA"
cp "$REPO_DIR"/deploy/{compose.yml,Caddyfile,.env.example,lib.sh,install.sh,update.sh,backup.sh,restore.sh} "$DEP/"

g() { git -C "$REPO" -c user.name=test -c user.email=test@example.com -c commit.gpgsign=false -c tag.gpgsign=false "$@"; }
g init -q
g config core.autocrlf false
g add deploy
g commit -q -m "release 0.30.0"
g tag v0.30.0
for v in 0.31.0 0.32.0 0.33.0; do
	g commit -q --allow-empty -m "release $v"
	g tag "v$v"
done
g checkout -q --detach v0.30.0

(cd "$REPO" && STUARTY_HEALTH_TIMEOUT=6 STUARTY_HTTPS_TIMEOUT=6 bash deploy/install.sh --domain host.example.com --email me@example.com) >/dev/null </dev/null
export STUB_KEY_FILE="$DEP/secrets/master_key"
[ "$(cat "$STUB_STATE/running")" = 0.30.0 ] && pass "installed and running 0.30.0" || fail "0.30.0 is not running"

stu() { STUARTY_HOME="$STUB_DATA" STUARTY_MASTER_KEY_FILE="$STUB_KEY_FILE" "$bin" "$@"; }
printf 'c4n4ry correct horse battery\n' | stu user:create ana@example.com --admin >/dev/null 2>&1
mkdir -p "$STUB_DATA/store/ab" "$STUB_DATA/library/memory"
head -c 100000 /dev/urandom >"$STUB_DATA/store/ab/abcdef0123"
printf 'a note\n' >"$STUB_DATA/library/memory/note.md"
before="$(tree_sums "$STUB_DATA/store"; tree_sums "$STUB_DATA/library")"

update() { (cd "$REPO" && STUARTY_HEALTH_TIMEOUT=6 STUARTY_HTTPS_TIMEOUT=6 bash deploy/update.sh "$@") </dev/null; }
head_tag() { g describe --tags --exact-match 2>/dev/null; }
version() { grep -E '^STUARTY_VERSION=' "$DEP/.env" | cut -d= -f2; }

# 1. Refusals that change nothing.
if update 9.9.9 >"$WORK/u1.log" 2>&1; then fail "an update to a tag that does not exist succeeded"; fi
grep -q 'no tag v9.9.9' "$WORK/u1.log" && pass "an unknown version is refused" || fail "no message about the missing tag"
if update latest >"$WORK/u2.log" 2>&1; then fail "'latest' was accepted"; fi
pass "latest is not a version"
echo "dirty" >>"$DEP/compose.yml"
if update 0.31.0 >"$WORK/u3.log" 2>&1; then fail "a dirty checkout was updated"; fi
grep -q 'uncommitted changes' "$WORK/u3.log" && pass "a dirty checkout is refused" || fail "no message about the dirty checkout"
g restore deploy/compose.yml
[ "$(version)" = 0.30.0 ] && [ "$(head_tag)" = v0.30.0 ] && pass "the refusals changed nothing" || fail "a refusal changed the version"
update 0.30.0 | grep -q 'already running' && pass "the same version is a no-op" || fail "the same version was not a no-op"

# 2. A healthy update: backup first, then the new version.
update 0.31.0 >"$WORK/u4.log" 2>&1 || { cat "$WORK/u4.log" >&2; fail "update to 0.31.0 failed"; }
[ "$(version)" = 0.31.0 ] && [ "$(cat "$STUB_STATE/running")" = 0.31.0 ] && [ "$(head_tag)" = v0.31.0 ] && pass "0.31.0 runs, the checkout and .env moved" || fail "0.31.0 is not running"
assert "a backup was made before" bash -c "ls '$DEP'/backups/stuarty-backup-*.tar"
backup_line="$(grep -n 'backup.sh\|workspace:backup' "$STUB_STATE/calls.log" | head -n 1 | cut -d: -f1)"
build_line="$(grep -n ' build stuarty' "$STUB_STATE/calls.log" | tail -n 1 | cut -d: -f1)"
[ -n "$backup_line" ] && [ "$backup_line" -lt "$build_line" ] && pass "the backup came before the build" || fail "the backup did not come first"
grep -q 'stuarty:0.30.0' "$STUB_STATE/images" && pass "the previous image is kept" || fail "the previous image is gone"

# 3. A version whose health check fails: rolled back, the data is the one of the backup.
export STUB_FAIL_VERSIONS="0.32.0" STUB_WRITE_VERSIONS="0.32.0"
before="$(tree_sums "$STUB_DATA/store"; tree_sums "$STUB_DATA/library")"
if update 0.32.0 >"$WORK/u5.log" 2>&1; then fail "an update to a version that is not healthy reported success"; fi
grep -q 'rolling back' "$WORK/u5.log" && grep -q 'running again' "$WORK/u5.log" && pass "the failure is reported and says it rolled back" || { cat "$WORK/u5.log" >&2; fail "no rollback message"; }
[ "$(version)" = 0.31.0 ] && pass ".env is back to 0.31.0" || fail ".env is $(version)"
[ "$(head_tag)" = v0.31.0 ] && pass "the checkout is back at v0.31.0" || fail "the checkout is at $(head_tag)"
[ "$(cat "$STUB_STATE/running")" = 0.31.0 ] && pass "the previous version is running again" || fail "running: $(cat "$STUB_STATE/running")"
assert "what the failed version wrote is not in the store" test ! -e "$STUB_DATA/store/zz/written-by-0.32.0"
assert "what the failed version wrote is kept aside" bash -c "ls '$STUB_DATA'/replaced-*/store/zz/written-by-0.32.0"
[ "$(tree_sums "$STUB_DATA/store"; tree_sums "$STUB_DATA/library")" = "$before" ] && pass "the originals and the library are the ones of before" || fail "the data changed"
out="$(stu connection:list --json 2>&1)" && pass "the restored database opens" || fail "the database is damaged after the rollback: $out"
unset STUB_FAIL_VERSIONS STUB_WRITE_VERSIONS

# 4. A build that fails changes nothing that runs.
export STUB_BUILD_FAIL=0.33.0
if update 0.33.0 >"$WORK/u6.log" 2>&1; then fail "an update whose build fails reported success"; fi
grep -q 'build of 0.33.0 failed' "$WORK/u6.log" && pass "a failed build is reported" || fail "no message about the build"
[ "$(version)" = 0.31.0 ] && [ "$(head_tag)" = v0.31.0 ] && [ "$(cat "$STUB_STATE/running")" = 0.31.0 ] && pass "a failed build changed nothing" || fail "a failed build changed the host"
unset STUB_BUILD_FAIL

# 5. The scripts contact nothing but Docker and the address of the installation (no telemetry, no pull).
assert "no docker pull was run" bash -c "! grep -q 'docker pull\| pull ' '$STUB_STATE/calls.log'"
assert "curl only asked for the installed domain" bash -c "! grep '^curl' '$STUB_STATE/calls.log' | grep -v 'host.example.com'"

echo "update-test: all checks passed"
