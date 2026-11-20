package perfgate

import "testing"

func TestEnabledOnlyWithOne(t *testing.T) {
	for value, want := range map[string]bool{"": false, "0": false, "true": false, "1": true} {
		t.Setenv(Env, value)

		if got := Enabled(); got != want {
			t.Errorf("%s=%q: Enabled() = %v, want %v", Env, value, got, want)
		}
	}
}

func TestRequireSkipsByDefault(t *testing.T) {
	t.Setenv(Env, "")

	ran := false

	t.Run("budget", func(t *testing.T) {
		Require(t)

		ran = true
	})

	if ran {
		t.Fatal("the budget ran without " + Env + "=1")
	}
}

func TestRequireRunsWhenOn(t *testing.T) {
	t.Setenv(Env, "1")

	ran := false

	t.Run("budget", func(t *testing.T) {
		Require(t)

		ran = true
	})

	if !ran {
		t.Fatal("the budget did not run with " + Env + "=1")
	}
}
