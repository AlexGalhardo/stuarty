package service

import (
	"errors"
	"fmt"
	"testing"
)

func TestQueueFullErrorCarriesTheRunningJobs(t *testing.T) {
	t.Parallel()

	err := fmt.Errorf("create: %w", &QueueFullError{Running: []string{"job_a", "job_b"}})

	var full *QueueFullError
	if !errors.As(err, &full) {
		t.Fatal("errors.As does not find the QueueFullError through a wrap")
	}

	if len(full.Running) != 2 || full.Error() != "service: job limit reached" {
		t.Errorf("got %v and %q", full.Running, full.Error())
	}
}
