package owner

import (
	"os"
	"path/filepath"
	"runtime"
)

// gpuHostName is the native GPU render host the exporter starts as a child process.
const gpuHostName = "stuarty-render"

// locateGPUHost returns the absolute path of stuarty-render next to the server executable or under
// ../libexec/stuarty, the places FFmpeg is looked for. It returns "" when there is none, and the
// export then keeps the CPU path.
func locateGPUHost(execDir, goos string) string {
	if execDir == "" || !filepath.IsAbs(execDir) {
		return ""
	}

	name := gpuHostName
	if goos == "windows" {
		name += ".exe"
	}

	for _, p := range []string{
		filepath.Join(execDir, name),
		filepath.Join(execDir, "..", "libexec", "stuarty", name),
	} {
		if st, err := os.Stat(p); err == nil && st.Mode().IsRegular() {
			return filepath.Clean(p)
		}
	}

	return ""
}

// gpuHostForRenderer is the stuarty-render path for the configured renderer; "" unless it is gpu.
func gpuHostForRenderer(renderer, execDir string) string {
	if renderer != "gpu" {
		return ""
	}

	return locateGPUHost(execDir, runtime.GOOS)
}
