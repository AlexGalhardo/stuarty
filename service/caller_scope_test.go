package service

import (
	"context"
	"errors"
	"testing"
)

// SDD-003 §6.2: a refusal for a missing scope names it in details.scope; a
// refusal because the key is limited to another project carries no scope.
func TestForbiddenNamesTheMissingScope(t *testing.T) {
	ctx := WithCaller(context.Background(), Caller{Actor: "key:k", Scopes: []string{ScopeRead}, ProjectID: "prj_a"})

	_, err := authorizeScope(ctx, ScopeEdit, "prj_a")

	var e *Error
	if !errors.As(err, &e) || e.Code != CodeForbidden {
		t.Fatalf("missing scope = %v, want E_FORBIDDEN", err)
	}

	if got := e.Details["scope"]; got != ScopeEdit {
		t.Errorf("details.scope = %v, want %q", got, ScopeEdit)
	}

	_, err = authorizeScope(ctx, ScopeRead, "prj_b")
	if !errors.As(err, &e) || e.Code != CodeForbidden {
		t.Fatalf("other project = %v, want E_FORBIDDEN", err)
	}

	if _, ok := e.Details["scope"]; ok {
		t.Errorf("a project refusal carries details.scope: %v", e.Details)
	}
}
