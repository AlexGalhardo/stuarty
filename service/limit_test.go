package service

import "testing"

func TestCheckLimitRefusesANegativeLimit(t *testing.T) {
	t.Parallel()

	if checkLimit(-1) == nil || checkLimit(0) != nil || checkLimit(5) != nil {
		t.Error("checkLimit must refuse only a negative limit")
	}
}
