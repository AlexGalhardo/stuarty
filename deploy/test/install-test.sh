#!/usr/bin/env bash
# Test of deploy/install.sh and of the static rules of deploy/ (epic 25 task 3, SDD-009 section 21):
#
#   bash deploy/test/install-test.sh
#
# It runs install.sh against the docker and curl stand-ins of deploy/test/stub/. With a real Docker
# it also checks `docker compose config` (no image is pulled or built). With a Go toolchain it runs
# the real binary to show that network mode refuses to start with no key, a short key or a public
# plain-HTTP address. Exit 0 when everything holds.
set -euo pipefail
# shellcheck source=deploy/test/common.sh
. "$(dirname "${BASH_SOURCE[0]}")/common.sh"

new_work
export STUB_FAIL_VERSIONS=""
run_install() { (cd "$WORK" && STUARTY_HEALTH_TIMEOUT=6 STUARTY_HTTPS_TIMEOUT=6 bash "$DEP/install.sh" "$@" </dev/null); }

# 1. Without answers and without a terminal it asks nothing and fails with a clear message.
if out="$(run_install 2>&1)"; then fail "install.sh with no answers must fail"; fi
case "$out" in *"is needed: pass it as a flag"*) pass "no answers and no terminal: clear failure" ;; *) fail "unexpected message: $out" ;; esac
assert "nothing was written on that failure" test ! -f "$DEP/.env"

# 2. A checkout that is not at a tag has no version to build.
if out="$(run_install --domain stuarty.example.com --email me@example.com 2>&1)"; then fail "install.sh with no version must fail"; fi
case "$out" in *"not at a release tag"*) pass "no tag: clear failure" ;; *) fail "unexpected message: $out" ;; esac

# 3. A bad domain or e-mail is refused.
if run_install --domain "bad domain" --email me@example.com --version 0.30.0 >/dev/null 2>&1; then fail "a bad domain must fail"; fi
pass "a bad domain is refused"
# A value whose first line is good: grep -E would match that line alone and the rest would land in .env.
nl=$'\n'
for bad in "--domain=stuarty.example.com${nl}STUARTY_VERSION=9.9.9" "--email=me@example.com${nl}X=1" "--email=a\$b@example.com" "--email=a}@example.com" "--version=0.30.0${nl}X=1"; do
	flag="${bad%%=*}" value="${bad#*=}"
	args=(--domain stuarty.example.com --email me@example.com --version 0.30.0 "$flag" "$value")
	if run_install "${args[@]}" >/dev/null 2>&1; then fail "install.sh accepted $flag $(printf '%q' "$value")"; fi
done
assert "a value with a second line, a \$ or a brace is refused, and nothing is written" test ! -f "$DEP/.env"
if STUARTY_TLS_DIRECTIVE="tls internal${nl}}" run_install --domain stuarty.example.com --email me@example.com --version 0.30.0 >/dev/null 2>&1; then fail "a TLS directive other than tls internal was accepted"; fi
pass "STUARTY_TLS_DIRECTIVE is only empty or tls internal"

# 4. The install: writes .env and three keys, builds, starts, waits for health and HTTPS, prints the login.
out="$(run_install --domain stuarty.example.com --email me@example.com --version 0.30.0)"
for f in master_key app_key admin_password; do assert "secrets/$f exists and is not empty" test -s "$DEP/secrets/$f"; done
assert ".env has the domain" grep -qx 'STUARTY_DOMAIN=stuarty.example.com' "$DEP/.env"
assert ".env has the version" grep -qx 'STUARTY_VERSION=0.30.0' "$DEP/.env"
assert ".env holds no key and no password" bash -c "! grep -qiE 'key=|password=' '$DEP/.env'"
assert "the image was built once" grep -qx 'docker compose --project-directory '"$DEP"' -f '"$DEP"'/compose.yml build stuarty' "$STUB_STATE/calls.log"
assert "the stack was started" grep -q ' up -d' "$STUB_STATE/calls.log"
assert "https was checked" grep -q 'https://stuarty.example.com/up' "$STUB_STATE/calls.log"
case "$out" in *"https://stuarty.example.com"*) pass "prints the address" ;; *) fail "the address is not printed" ;; esac
case "$out" in *"$(cat "$DEP/secrets/admin_password")"*) pass "prints the first password" ;; *) fail "the first password is not printed" ;; esac
case "$out" in *"secrets/master_key"*) pass "says where the master key is" ;; *) fail "the master key location is not printed" ;; esac
key_bytes="$(tr -d '\n' <"$DEP/secrets/master_key" | base64 -d 2>/dev/null | wc -c | tr -d ' ')"
[ "$key_bytes" = 32 ] && pass "the master key is 32 bytes" || fail "the master key is $key_bytes bytes"
if on_windows; then
	echo "skip file modes: this file system has no Unix modes"
else
	[ "$(mode_of "$DEP/.env")" = 600 ] && pass ".env is 0600" || fail ".env is not 0600"
	[ "$(mode_of "$DEP/secrets")" = 700 ] && pass "secrets/ is 0700" || fail "secrets/ is not 0700"
fi

# 5. Idempotent: a second run changes no file, keeps every key, builds nothing and does not print the password again.
before="$(tree_sums "$DEP")"
builds_before="$(grep -c ' build ' "$STUB_STATE/calls.log")"
out2="$(run_install)"
after="$(tree_sums "$DEP")"
[ "$before" = "$after" ] && pass "a second run changes no file" || fail "a second run changed a file"
[ "$(grep -c ' build ' "$STUB_STATE/calls.log")" = "$builds_before" ] && pass "a second run builds nothing" || fail "a second run built again"
case "$out2" in *"$(cat "$DEP/secrets/admin_password")"*) fail "the second run printed the password again" ;; *) pass "the second run does not print the password" ;; esac
if run_install --domain other.example.com >/dev/null 2>&1; then fail "another domain on an installed host must fail"; fi
[ "$(tree_sums "$DEP")" = "$before" ] && pass "another domain is refused and nothing changes" || fail "the refusal changed a file"

# 6. A host prepared for a restore keeps the master key put there, and starts nothing.
rm -f "$DEP/.env" "$DEP/secrets/app_key" "$DEP/secrets/admin_password"
printf 'bXktb2xkLW1hc3Rlci1rZXktMzItYnl0ZXMtbG9uZyE=\n' >"$DEP/secrets/master_key"
sum_key="$(sha256sum "$DEP/secrets/master_key" | cut -d' ' -f1)"
: >"$STUB_STATE/calls.log"
run_install --domain new.example.com --email me@example.com --version 0.30.0 --no-start >/dev/null
[ "$(sha256sum "$DEP/secrets/master_key" | cut -d' ' -f1)" = "$sum_key" ] && pass "an existing master key is never replaced" || fail "the master key was replaced"
assert "--no-start starts nothing" bash -c "! grep -q ' up ' '$STUB_STATE/calls.log'"

# 6b. A container that turns unhealthy makes `docker compose up -d` itself fail (Caddy waits for
# `service_healthy`): the script still shows the log and says what to do.
if out="$(STUB_FAIL_VERSIONS=0.30.0 run_install 2>&1)"; then fail "an unhealthy start reported success"; fi
case "$out" in *"did not become healthy"*) pass "an unhealthy start is reported with the log" ;; *) fail "unexpected message: $out" ;; esac

# 7. The static rules of the compose file and the Caddyfile.
for f in compose.yml Caddyfile .env.example install.sh; do
	assert "$f names no other web server" bash -c "! grep -qiE 'nginx|apache|traefik|httpd' '$REPO_DIR/deploy/$f'"
done
images="$(grep -E '^[[:space:]]+image:' "$REPO_DIR/deploy/compose.yml" | sed 's/^[[:space:]]*image:[[:space:]]*//')"
echo "$images" | grep -qE '^caddy:[0-9]+\.[0-9]+\.[0-9]+(-alpine)?@sha256:[0-9a-f]{64}$' && pass "caddy has a fixed version and a digest" || fail "the caddy image is not pinned: $images"
echo "$images" | grep -qE '^stuarty:\$\{STUARTY_VERSION:-devel\}$' && pass "stuarty is tagged with its version" || fail "the stuarty image tag is not its version: $images"
assert "no image uses latest, edge or nightly" bash -c "! grep -rniE '(:|=)(latest|edge|nightly)' '$REPO_DIR/deploy/compose.yml' '$REPO_DIR/deploy/.env.example'"
assert "the Caddyfile turns response buffering off" grep -q 'flush_interval -1' "$REPO_DIR/deploy/Caddyfile"
assert "the Caddyfile limits the request body" grep -q 'max_size' "$REPO_DIR/deploy/Caddyfile"
assert "the body limit is 8 GiB, as STUARTY_MAX_IMPORT_BYTES" bash -c "grep -q 'STUARTY_MAX_BODY:-8GiB}' '$REPO_DIR/deploy/compose.yml' && grep -qx 'STUARTY_MAX_BODY=8GiB' '$REPO_DIR/deploy/.env.example'"
assert "the stuarty image is never pulled from a registry" grep -qE '^[[:space:]]+pull_policy: never$' "$REPO_DIR/deploy/compose.yml"

if [ -n "$REAL_DOCKER" ] && "$REAL_DOCKER" compose version >/dev/null 2>&1; then
	cfg="$(PATH="$(dirname "$REAL_DOCKER"):$PATH" "$REAL_DOCKER" compose -f "$REPO_DIR/deploy/compose.yml" --env-file "$REPO_DIR/deploy/.env.example" config)" || fail "docker compose config"
	pass "docker compose config is valid"
	published="$(printf '%s\n' "$cfg" | grep -E '^[[:space:]]+published:' | tr -d ' "' | sort | tr '\n' ' ')"
	[ "$published" = "published:443 published:80 " ] && pass "only ports 80 and 443 are published" || fail "published ports: $published"
	[ "$(printf '%s\n' "$cfg" | grep -c 'read_only: true')" -ge 1 ] && pass "the application has a read-only root file system" || fail "no read_only in the config"
	printf '%s\n' "$cfg" | grep -q 'user:' && fail "the compose file overrides the user of the image" || pass "the user is the non-root user of the image"
else
	echo "skip docker compose config: no docker compose on this machine"
fi

# 8. Network mode refuses to start without a key, with a short one and with a public plain-HTTP address.
if command -v go >/dev/null 2>&1; then
	bin="$(build_binary)"
	home="$WORK/home"
	mkdir -p "$home"
	good_key="$(head -c 32 /dev/urandom | base64 | tr -d '\n')"
	out="$(env -u APP_KEY -u APP_ENV STUARTY_HOME="$home" STUARTY_MODE=network HOST=127.0.0.1 PORT=0 APP_URL=https://x.example.com timeout 20 "$bin" serve 2>&1 || true)"
	case "$out" in *APP_KEY*) pass "network mode refuses to start without APP_KEY" ;; *) fail "no refusal without APP_KEY: $out" ;; esac
	out="$(env -u APP_ENV APP_KEY=short STUARTY_HOME="$home" STUARTY_MODE=network HOST=127.0.0.1 PORT=0 APP_URL=https://x.example.com timeout 20 "$bin" serve 2>&1 || true)"
	case "$out" in *APP_KEY*) pass "network mode refuses a short APP_KEY" ;; *) fail "no refusal of a short key: $out" ;; esac
	out="$(env -u APP_ENV APP_KEY="$good_key" STUARTY_HOME="$home" STUARTY_MODE=network HOST=127.0.0.1 PORT=0 APP_URL=http://x.example.com timeout 20 "$bin" serve 2>&1 || true)"
	case "$out" in *APP_URL* | *TLS* | *https*) pass "network mode refuses a public plain-HTTP APP_URL" ;; *) fail "no refusal of http APP_URL: $out" ;; esac
else
	echo "skip the network mode refusals: no Go toolchain"
fi

echo "install-test: all checks passed"
