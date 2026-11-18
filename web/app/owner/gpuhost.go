package owner

import (
	"os"
	"path/filepath"
	"runtime"
)

// gpuHostName is the native GPU render host the exporter starts as a child process.
const gpuHostName = "vukem-render"

// locateGPUHost returns the absolute path of vukem-render next to the server executable or under
// ../libexec/vukem-video, the places FFmpeg is looked for. It returns "" when there is none, and the
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
		filepath.Join(execDir, "..", "libexec", "vukem-video", name),
	} {
		if st, err := os.Stat(p); err == nil && st.Mode().IsRegular() {
			return filepath.Clean(p)
		}
	}

	return ""
}

// gpuHostForRenderer is the vukem-render path for the configured renderer; "" unless it is gpu.
func gpuHostForRenderer(renderer, execDir string) string {
	if renderer != "gpu" {
		return ""
	}

	return locateGPUHost(execDir, runtime.GOOS)
}
