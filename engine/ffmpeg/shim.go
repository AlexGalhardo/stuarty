package ffmpeg

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

const maxScoopShimBytes = 16 << 10

// parseScoopShim parses the `path = <target>` line of a Scoop .shim file without
// shell expansion. Relative paths, null bytes and path traversal segments are refused.
func parseScoopShim(data []byte) (string, bool) {
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}

		key, val, ok := strings.Cut(line, "=")
		if !ok || !strings.EqualFold(strings.TrimSpace(key), "path") {
			continue
		}

		val = strings.TrimSpace(val)
		if len(val) >= 2 && ((val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'')) {
			val = val[1 : len(val)-1]
		}

		if val == "" || strings.ContainsRune(val, 0) {
			return "", false
		}

		if slices.Contains(strings.Split(filepath.ToSlash(val), "/"), "..") {
			return "", false
		}

		if !filepath.IsAbs(val) {
			return "", false
		}

		return filepath.Clean(val), true
	}

	return "", false
}

// resolveScoopShim checks whether binPath has a sibling `<stem>.shim` file (Scoop
// shim layout on Windows) and, when valid, returns the resolved real binary path.
// On non-valid shims it returns (binPath, false) so normal discovery continues.
func resolveScoopShim(binPath string) (string, bool) {
	if !filepath.IsAbs(binPath) {
		return binPath, false
	}

	ext := filepath.Ext(binPath)
	shimPath := strings.TrimSuffix(binPath, ext) + ".shim"

	info, err := os.Stat(shimPath)
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxScoopShimBytes {
		return binPath, false
	}

	data, err := os.ReadFile(shimPath)
	if err != nil {
		return binPath, false
	}

	target, ok := parseScoopShim(data)
	if !ok {
		return binPath, false
	}

	if runtime.GOOS == "windows" && !strings.EqualFold(filepath.Ext(target), ".exe") {
		return binPath, false
	}

	if eval, err := filepath.EvalSymlinks(target); err == nil && filepath.IsAbs(eval) {
		target = eval
	}

	//nolint:gosec // G703: parsed absolute path verified free of traversal and validated by openTool
	tinfo, err := os.Stat(target)
	if err != nil || !tinfo.Mode().IsRegular() {
		return binPath, false
	}

	return target, true
}
