package openrouter_test

import (
	"testing"

	"github.com/AlexGalhardo/stuarty/service/providers/providertest"
)

// TestMain refuses every dial that is not to a loopback address: no test of this package reaches a real
// provider.
func TestMain(m *testing.M) { providertest.Main(m) }
