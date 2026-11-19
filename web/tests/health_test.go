package tests

import (
	"testing"

	"github.com/AlexGalhardo/stuarty/web/bootstrap"

	"github.com/AlexGalhardo/vukem/vukemtest"
)

func TestUpAnswersOK(t *testing.T) {
	t.Parallel()

	app := vukemtest.New(t, bootstrap.New)

	app.Client(t).Get("/up").AssertOK().AssertSee(`"status":"ok"`)
}
