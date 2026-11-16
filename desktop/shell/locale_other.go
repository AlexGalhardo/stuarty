//go:build !windows

package shell

// systemLocale reads the first of the usual locale variables.
func systemLocale(getenv func(string) string) string {
	for _, name := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := getenv(name); v != "" {
			return v
		}
	}
	return ""
}
