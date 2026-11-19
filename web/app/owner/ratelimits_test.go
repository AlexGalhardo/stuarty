package owner

import "testing"

func TestRateLimitsFromEnvReadsTheHeavyLimit(t *testing.T) {
	r, err := rateLimitsFromEnv(envOf(map[string]string{"STUARTY_RATE_LIMIT_HEAVY_PER_MINUTE": "25"}), false)
	if err != nil || r.Heavy != 25 {
		t.Fatalf("heavy = %d, err = %v; want 25", r.Heavy, err)
	}

	r, err = rateLimitsFromEnv(envOf(nil), false)
	if err != nil || r.Heavy != 0 {
		t.Fatalf("unset: heavy = %d, err = %v; want 0 (the service default)", r.Heavy, err)
	}

	zero := envOf(map[string]string{"STUARTY_RATE_LIMIT_HEAVY_PER_MINUTE": "0"})
	if _, err := rateLimitsFromEnv(zero, true); err == nil {
		t.Fatal("a zero heavy limit must be a boot error in network mode")
	}
}
