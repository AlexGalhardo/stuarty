#!/usr/bin/env bash
# Builds the native capture helper stuarty-capture of capture/ (ADR-0049, SDD-025, rule .agents/rules/rust.md):
#   bash scripts/build-capture.sh        build it for this machine and copy it to capture/dist/ with its SHA-256
#   bash scripts/build-capture.sh gate   the Rust gate of capture/: fmt, clippy, tests and cargo deny
# An archive or an installer for Windows carries stuarty-capture.exe next to the server executable (or in
# libexec/stuarty/): the server looks for it there (SDD-025 section 7). The helper exists on Windows only in this
# version; on another system the build works but the server answers E_CAPTURE_UNAVAILABLE.
# Reproducible: the toolchain is the one of rust-toolchain.toml, dependencies come from Cargo.lock (--locked), the build
# path is fixed (capture/target), SOURCE_DATE_EPOCH defaults to the commit time of HEAD and --remap-path-prefix hides the
# paths of the machine. Runs on bash 3.2.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
mode="${1:-build}"

die() { printf 'build-capture: error: %s\n' "$*" >&2; exit 1; }

# A path the Rust toolchain understands (Git Bash on Windows gives /c/... paths).
native() { if command -v cygpath >/dev/null 2>&1; then cygpath -m "$1"; else printf '%s' "$1"; fi; }

sha256() {
	if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | cut -d' ' -f1; else shasum -a 256 "$1" | cut -d' ' -f1; fi
}

case "$mode" in
build | gate) ;;
*) die "unknown mode '$mode' (build or gate)" ;;
esac

command -v cargo >/dev/null 2>&1 || die "cargo is not installed (rustup, then the toolchain of rust-toolchain.toml)"

manifest="$(native "$root/capture/Cargo.toml")"

if [ "$mode" = gate ]; then
	cd "$root/capture"
	cargo fmt --check
	cargo clippy --all-targets -- -D warnings
	cargo test
	cargo deny check
	exit 0
fi

if [ -z "${SOURCE_DATE_EPOCH:-}" ]; then
	SOURCE_DATE_EPOCH="$(git -C "$root" log -1 --format=%ct 2>/dev/null || echo 0)"
fi
export SOURCE_DATE_EPOCH

cargo_home="$(native "${CARGO_HOME:-$HOME/.cargo}")"
rustup_home="$(native "${RUSTUP_HOME:-$HOME/.rustup}")"
src="$(native "$root")"
export CARGO_TARGET_DIR="${CAPTURE_TARGET_DIR:-$src/capture/target}"
export RUSTFLAGS="--remap-path-prefix=$src=/build --remap-path-prefix=$cargo_home=/cargo --remap-path-prefix=$rustup_home=/rustup"
# An incremental cache is not part of a reproducible build.
export CARGO_INCREMENTAL=0

exe=stuarty-capture
case "$(uname -s)" in MINGW* | MSYS* | CYGWIN*) exe=stuarty-capture.exe ;; esac

cargo build --locked --release --bin stuarty-capture --manifest-path "$manifest"

dist="$root/capture/dist"
mkdir -p "$dist"
cp "$CARGO_TARGET_DIR/release/$exe" "$dist/$exe"
sha256 "$dist/$exe" >"$dist/$exe.sha256"
printf 'build-capture: %s sha256 %s\n' "$exe" "$(cat "$dist/$exe.sha256")"
