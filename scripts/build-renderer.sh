#!/usr/bin/env bash
# Builds the pixel renderer of renderer/ (ADR-0029, rule .agents/rules/rust.md 9):
#   bash scripts/build-renderer.sh            the native crate and the WebAssembly module
#   bash scripts/build-renderer.sh native     only the native crate
#   bash scripts/build-renderer.sh wasm       only the WebAssembly module
#   bash scripts/build-renderer.sh verify     build the module twice from clean and fail unless the digests match
# The module and its glue are written to renderer/dist/, with vukem_renderer_bg.wasm.sha256 next to it.
# Reproducible: the toolchain is the one of rust-toolchain.toml, dependencies come from Cargo.lock
# (--locked), the build path is fixed (renderer/target), SOURCE_DATE_EPOCH defaults to the commit time
# of HEAD and --remap-path-prefix hides the machine paths. The wasm-bindgen CLI must be the version
# the crate is locked to: cargo install --locked wasm-bindgen-cli --version <that version>.
# Runs on bash 3.2 (macOS).
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
mode="${1:-all}"

die() { printf 'build-renderer: error: %s\n' "$*" >&2; exit 1; }

# A path the Rust toolchain understands (Git Bash on Windows gives /c/... paths).
native() { if command -v cygpath >/dev/null 2>&1; then cygpath -m "$1"; else printf '%s' "$1"; fi; }

sha256() {
	if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | cut -d' ' -f1; else shasum -a 256 "$1" | cut -d' ' -f1; fi
}

case "$mode" in
all | native | wasm | verify) ;;
*) die "unknown mode '$mode' (all, native, wasm or verify)" ;;
esac

command -v cargo >/dev/null 2>&1 || die "cargo is not installed (rustup, then the toolchain of rust-toolchain.toml)"

if [ -z "${SOURCE_DATE_EPOCH:-}" ]; then
	SOURCE_DATE_EPOCH="$(git -C "$root" log -1 --format=%ct 2>/dev/null || echo 0)"
fi
export SOURCE_DATE_EPOCH

cargo_home="$(native "${CARGO_HOME:-$HOME/.cargo}")"
rustup_home="$(native "${RUSTUP_HOME:-$HOME/.rustup}")"
src="$(native "$root")"
export CARGO_TARGET_DIR="$src/renderer/target"
export RUSTFLAGS="--remap-path-prefix=$src=/build --remap-path-prefix=$cargo_home=/cargo --remap-path-prefix=$rustup_home=/rustup"
# An incremental cache is not part of a reproducible build.
export CARGO_INCREMENTAL=0

dist="$root/renderer/dist"
manifest="$root/renderer/Cargo.toml"

native_build() {
	cargo build --locked --release --manifest-path "$manifest"
}

locked_bindgen() {
	awk '/^name = "wasm-bindgen"$/ { getline; gsub(/version = |"/, ""); print; exit }' "$root/renderer/Cargo.lock"
}

wasm_build() {
	local want have
	want="$(locked_bindgen)"
	[ -n "$want" ] || die "wasm-bindgen is not in renderer/Cargo.lock"
	command -v wasm-bindgen >/dev/null 2>&1 || die "wasm-bindgen CLI is not installed: cargo install --locked wasm-bindgen-cli --version $want"
	have="$(wasm-bindgen --version | awk '{ print $2 }')"
	[ "$have" = "$want" ] || die "wasm-bindgen CLI is $have, Cargo.lock pins $want: cargo install --locked wasm-bindgen-cli --version $want"

	cargo build --locked --release --target wasm32-unknown-unknown --manifest-path "$manifest"

	rm -rf "$dist"
	mkdir -p "$dist"
	wasm-bindgen --target web --out-dir "$(native "$dist")" --out-name vukem_renderer \
		"$CARGO_TARGET_DIR/wasm32-unknown-unknown/release/vukem_renderer.wasm"

	sha256 "$dist/vukem_renderer_bg.wasm" >"$dist/vukem_renderer_bg.wasm.sha256"
	printf 'build-renderer: vukem_renderer_bg.wasm sha256 %s\n' "$(cat "$dist/vukem_renderer_bg.wasm.sha256")"
}

clean_wasm() {
	rm -rf "$CARGO_TARGET_DIR/wasm32-unknown-unknown" "$CARGO_TARGET_DIR/release"
}

case "$mode" in
native) native_build ;;
wasm) wasm_build ;;
all)
	native_build
	wasm_build
	;;
verify)
	clean_wasm
	wasm_build
	first="$(cat "$dist/vukem_renderer_bg.wasm.sha256")"
	clean_wasm
	wasm_build
	second="$(cat "$dist/vukem_renderer_bg.wasm.sha256")"
	[ "$first" = "$second" ] || die "two clean builds differ: $first and $second"
	printf 'build-renderer: reproducible, two clean builds give %s\n' "$first"
	;;
esac
