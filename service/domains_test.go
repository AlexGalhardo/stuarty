package service

import (
	"reflect"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
)

// TestOperationDomains fails when a Service method has no domain, or a domain that SDD-000 section 1.1 does not
// list, and when the table names a method the Service no longer has.
func TestOperationDomains(t *testing.T) {
	t.Parallel()

	svc := reflect.TypeFor[Service]()
	seen := map[string]bool{}

	for m := range svc.Methods() {
		seen[m.Name] = true

		d, ok := OperationDomain(m.Name)
		if !ok {
			t.Errorf("Service.%s has no domain: add it to OperationDomains (SDD-000 section 1.1)", m.Name)

			continue
		}

		if !command.KnownDomain(d) {
			t.Errorf("Service.%s has domain %q, which SDD-000 section 1.1 does not list", m.Name, d)
		}
	}

	for name := range OperationDomains {
		if !seen[name] {
			t.Errorf("OperationDomains names %s, which Service does not have", name)
		}
	}
}
