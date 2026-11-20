package owner

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// Acceptance criterion 4 of epic 09: adding a provider is a package under service/providers and one line
// in adapters(). The list of the build and the folders of service/providers are the same set.
func TestAddingAProviderNeedsOnePackageAndOneLineInTheOwner(t *testing.T) {
	t.Parallel()

	entries, err := os.ReadDir(filepath.Join("..", "..", "..", "service", "providers"))
	if err != nil {
		t.Fatal(err)
	}

	support := []string{"wire", "providertest"}

	var dirs []string

	for _, e := range entries {
		if e.IsDir() && !slices.Contains(support, e.Name()) {
			dirs = append(dirs, e.Name())
		}
	}

	var registered []string
	for _, p := range adapters() {
		registered = append(registered, p.Descriptor().ID)
	}

	slices.Sort(dirs)
	slices.Sort(registered)

	if !slices.Equal(dirs, registered) {
		t.Fatalf(
			"the adapter folders %v and the providers the owner registers %v must be the same set",
			dirs,
			registered,
		)
	}
}
