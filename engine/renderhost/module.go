package renderhost

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	_ "embed" // the module and its digest are part of the binary (SDD-013 section 4.1)
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/tetratelabs/wazero"
)

// moduleBytes is the core module of renderer/, built by scripts/build-renderer.sh embed, and moduleDigest
// is its SHA-256 as the script wrote it. The module is refused unless the two agree (ADR-0029, Consequences).
var (
	//go:embed renderer.wasm
	moduleBytes []byte
	//go:embed renderer.wasm.sha256
	moduleDigest string
)

// maxMemoryPages is the memory limit of the module, 1 GiB in pages of 64 KiB: the maximum the module
// declares and the limit of the runtime (SDD-013 section 4.1 and 4.6).
const maxMemoryPages = 16384

// APIVersion is the vr_api_version this host knows.
const APIVersion = 3

// exports are the functions the module must export, and the only ones (SDD-013 section 4.2).
var exports = []string{
	"vr_api_version", "vr_alloc", "vr_free", "vr_init", "vr_put_texture", "vr_put_bitmap", "vr_put_lut", "vr_release",
	"vr_prepare", "vr_prepared_ptr", "vr_prepared_len", "vr_prepared_clear", "vr_put_prepared",
	"vr_render", "vr_error",
	"vr_text_font_put", "vr_text_run", "vr_text_validate_font", "vr_text_result_ptr", "vr_text_result_len",
	"vr_text_result_clear",
}

// shared holds the compiled module once per process, in two runtimes: the one that stops a call when its
// context ends, for renders that can be cancelled, and the one that does not, for text requests, which
// are bounded by construction. The check that makes a call stoppable sits on every loop of the module and
// makes it about five times slower, which a bitmap that takes milliseconds does not need to pay.
var shared [2]struct {
	once     sync.Once
	runtime  wazero.Runtime
	compiled wazero.CompiledModule
	err      error
}

// ModuleSHA256 is the digest of the embedded module, as lower-case hex.
func ModuleSHA256() string { return strings.TrimSpace(moduleDigest) }

// verifyDigest checks the module bytes against the digest embedded next to them.
func verifyDigest(module []byte, digest string) error {
	sum := sha256.Sum256(module)
	want, err := hex.DecodeString(strings.TrimSpace(digest))
	if err != nil || len(want) != sha256.Size {
		return errors.New("renderhost: the embedded module digest is not a SHA-256")
	}

	if subtle.ConstantTimeCompare(sum[:], want) != 1 {
		return fmt.Errorf("renderhost: the embedded module (%x) does not match its digest (%x)", sum, want)
	}

	return nil
}

// newRuntime is the sandbox of the module (SDD-013 section 4.6): no WASI, no file system, no clock, no
// random source, no environment, no arguments, and a memory limit; cancelling the context stops a render.
func newRuntime(ctx context.Context) wazero.Runtime { return newRuntimeFor(ctx, true) }

// newRuntimeFor is newRuntime for a runtime that stops a call when its context ends (cancellable) or one that
// lets the call run to its end.
func newRuntimeFor(ctx context.Context, cancellable bool) wazero.Runtime {
	cfg := wazero.NewRuntimeConfig().
		WithMemoryLimitPages(maxMemoryPages).
		WithCloseOnContextDone(cancellable)

	return wazero.NewRuntimeWithConfig(ctx, cfg)
}

// compile checks and compiles the embedded module once per process.
func compile(ctx context.Context) (wazero.Runtime, wazero.CompiledModule, error) {
	return compileFor(ctx, true)
}

// compileFor is compile for the cancellable runtime or the one that is not.
func compileFor(ctx context.Context, cancellable bool) (wazero.Runtime, wazero.CompiledModule, error) {
	sh := &shared[0]
	if !cancellable {
		sh = &shared[1]
	}

	sh.once.Do(func() {
		sh.err = func() error {
			if err := verifyDigest(moduleBytes, moduleDigest); err != nil {
				return err
			}

			rt := newRuntimeFor(context.WithoutCancel(ctx), cancellable)

			cm, err := rt.CompileModule(context.WithoutCancel(ctx), moduleBytes)
			if err != nil {
				_ = rt.Close(context.WithoutCancel(ctx))

				return fmt.Errorf("renderhost: compiling the module: %w", err)
			}

			if err := checkInterface(cm); err != nil {
				_ = rt.Close(context.WithoutCancel(ctx))

				return err
			}

			sh.runtime, sh.compiled = rt, cm

			return nil
		}()
	})

	return sh.runtime, sh.compiled, sh.err
}

// checkInterface refuses a module that imports anything or does not export the interface of section 4.2:
// a module with an import could reach what the sandbox leaves out.
func checkInterface(cm wazero.CompiledModule) error {
	if n := len(cm.ImportedFunctions()) + len(cm.ImportedMemories()); n != 0 {
		return fmt.Errorf("renderhost: the module imports %d things; it may import nothing", n)
	}

	have := cm.ExportedFunctions()
	for _, name := range exports {
		if _, ok := have[name]; !ok {
			return fmt.Errorf("renderhost: the module does not export %s", name)
		}
	}

	if len(have) != len(exports) {
		return fmt.Errorf("renderhost: the module exports %d functions, want %d", len(have), len(exports))
	}

	if _, ok := cm.ExportedMemories()["memory"]; !ok {
		return errors.New("renderhost: the module does not export its memory")
	}

	return nil
}
