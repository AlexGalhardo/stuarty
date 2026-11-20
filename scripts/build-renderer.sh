#!/usr/bin/env bash
# Builds the pixel renderer of renderer/ (ADR-0029, rule .agents/rules/rust.md 9):
#   bash scripts/build-renderer.sh            the native crate and the WebAssembly core module
#   bash scripts/build-renderer.sh native     only the native crate
#   bash scripts/build-renderer.sh wasm       only the WebAssembly core module
#   bash scripts/build-renderer.sh verify     build the module twice from clean and fail unless the digests match
#   bash scripts/build-renderer.sh embed      verify, then write the module and its digest where Go embeds them
#   bash scripts/build-renderer.sh check      verify, then fail unless the embedded module is the one this source builds
#   bash scripts/build-renderer.sh host       the GPU render child process stuarty-render (SDD-013 section 4.9) for this machine,
#                                             copied to renderer/dist/ with its SHA-256; an archive or installer that carries
#                                             ffmpeg carries this file next to the server executable or in libexec/stuarty/
# The core module (SDD-013 section 4.1) has plain C-ABI exports and no import, so it needs no
# wasm-bindgen glue; it is written to renderer/dist/stuarty_renderer.wasm with stuarty_renderer.wasm.sha256 next
# to it. embed copies both to engine/renderhost/ (renderer.wasm and renderer.wasm.sha256).
# Reproducible: the toolchain is the one of rust-toolchain.toml, dependencies come from Cargo.lock
# (--locked), the build path is fixed (renderer/target), SOURCE_DATE_EPOCH defaults to the commit time
# of HEAD and --remap-path-prefix hides the machine paths and the separators of the source paths, so the
# digest is the same on every system. The memory of the module is capped at 16,384 pages (1 GiB).
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
all | native | wasm | verify | embed | check | host) ;;
*) die "unknown mode '$mode' (all, native, wasm, verify, embed, check or host)" ;;
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
# The crate sources are named relative to renderer/ in panic locations, with the separator of the
# system: the two remaps below turn them into the same text everywhere (no leading slash, which Git
# Bash would rewrite as a Windows path).
remap="--remap-path-prefix=$src=/build --remap-path-prefix=$cargo_home=/cargo --remap-path-prefix=$rustup_home=/rustup"
remap="$remap --remap-path-prefix=stuarty-renderer/src=renderer-src/ --remap-path-prefix=stuarty-renderer\\src=renderer-src/"
export RUSTFLAGS="$remap"
# An incremental cache is not part of a reproducible build.
export CARGO_INCREMENTAL=0

dist="$root/renderer/dist"
manifest="$root/renderer/Cargo.toml"
embedded="$root/engine/renderhost"

native_build() {
	cargo build --locked --release -p stuarty-renderer --manifest-path "$manifest"
}

# --remap-path-prefix rewrites the start of a source path but not the separators after it, so a Windows
# build keeps "/cargo\registry\src\..." and "stuarty-renderer-text\src\..." in the panic locations where a
# Linux build has slashes (VV-276). The slashes replace the backslashes in place, one byte for one byte, so
# no offset of the module moves; only strings that end in .rs and start with a remapped prefix or with a
# crate of this workspace are touched.
normalize_paths() {
	command -v perl >/dev/null 2>&1 || die "perl is not installed (it makes the source paths of the module the same on every system)"
	perl -e 'binmode STDIN; binmode STDOUT; local $/; $_ = <STDIN>; s{((?:/cargo|/rustup|/build|stuarty-renderer(?:-text|-gpu)?)[\\/][ -~]{1,300}?\.rs)}{(my $p = $1) =~ tr{\\}{/}; $p}ge; print' \
		<"$1" >"$1.normalized"
	mv "$1.normalized" "$1"
}

wasm_build() {
	# 16,384 pages of 64 KiB: the memory limit of SDD-013 section 4.1, also set in wazero.
	RUSTFLAGS="$remap -C link-arg=--max-memory=1073741824" \
		cargo build --locked --release -p stuarty-renderer --lib --target wasm32-unknown-unknown --manifest-path "$manifest"

	rm -rf "$dist"
	mkdir -p "$dist"
	cp "$CARGO_TARGET_DIR/wasm32-unknown-unknown/release/stuarty_renderer.wasm" "$dist/stuarty_renderer.wasm"
	normalize_paths "$dist/stuarty_renderer.wasm"
	sha256 "$dist/stuarty_renderer.wasm" >"$dist/stuarty_renderer.wasm.sha256"
	printf 'build-renderer: stuarty_renderer.wasm sha256 %s\n' "$(cat "$dist/stuarty_renderer.wasm.sha256")"
}

host_build() {
	local exe=stuarty-render
	case "$(uname -s)" in MINGW* | MSYS* | CYGWIN*) exe=stuarty-render.exe ;; esac

	cargo build --locked --release --bin stuarty-render --manifest-path "$manifest"

	mkdir -p "$dist"
	cp "$CARGO_TARGET_DIR/release/$exe" "$dist/$exe"
	sha256 "$dist/$exe" >"$dist/$exe.sha256"
	printf 'build-renderer: %s sha256 %s\n' "$exe" "$(cat "$dist/$exe.sha256")"
}

clean_wasm() {
	rm -rf "$CARGO_TARGET_DIR/wasm32-unknown-unknown" "$CARGO_TARGET_DIR/release"
}

verify_build() {
	local first second
	clean_wasm
	wasm_build
	first="$(cat "$dist/stuarty_renderer.wasm.sha256")"
	clean_wasm
	wasm_build
	second="$(cat "$dist/stuarty_renderer.wasm.sha256")"
	[ "$first" = "$second" ] || die "two clean builds differ: $first and $second"
	printf 'build-renderer: reproducible, two clean builds give %s\n' "$first"
}

case "$mode" in
native) native_build ;;
wasm) wasm_build ;;
all)
	native_build
	wasm_build
	;;
host) host_build ;;
verify) verify_build ;;
embed)
	verify_build
	cp "$dist/stuarty_renderer.wasm" "$embedded/renderer.wasm"
	cp "$dist/stuarty_renderer.wasm.sha256" "$embedded/renderer.wasm.sha256"
	printf 'build-renderer: embedded in engine/renderhost/\n'
	;;
check)
	verify_build
	have="$(sha256 "$embedded/renderer.wasm")"
	want="$(cat "$dist/stuarty_renderer.wasm.sha256")"
	[ "$have" = "$want" ] || die "engine/renderhost/renderer.wasm ($have) is not what the source builds ($want): run bash scripts/build-renderer.sh embed"
	printf 'build-renderer: the embedded module is the one this source builds\n'
	;;
esac
