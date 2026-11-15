package httpapi

import (
	"testing"

	"github.com/AlexGalhardo/vukem/errs"

	"github.com/AlexGalhardo/vukem-video/service"
)

func TestClientClosedIsNotAServerStatus(t *testing.T) {
	t.Parallel()

	he := httpError(&service.Error{Code: service.CodeClientClosed, Message: "closed"})
	if he.Status != service.StatusClientClosed || errs.Status(he) >= 500 {
		t.Errorf("status = %d, want 499 (vukem logs 5xx at ERROR)", he.Status)
	}
}
