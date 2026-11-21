package servicetest

import (
	"context"
	"errors"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/AlexGalhardo/stuarty/service"
)

// gate is a Gate the test opens and closes.
type gate struct{ locked atomic.Bool }

func (g *gate) Check() error {
	if g.locked.Load() {
		return errors.New("locked")
	}

	return nil
}

// callEvery calls every method of the Service with a zero request and the caller of the local token, and
// returns the code of the error of each. A panic on a zero request (when the gate is open and the method gets
// to read it) is reported as "panic" and is not what these tests look at.
func callEvery(ctx context.Context, t *testing.T, svc service.Service) map[string]string {
	t.Helper()

	out := map[string]string{}
	typ := reflect.TypeFor[service.Service]()

	for m := range typ.Methods() {
		args := []reflect.Value{reflect.ValueOf(ctx)}
		for i := 1; i < m.Type.NumIn(); i++ {
			args = append(args, reflect.Zero(m.Type.In(i)))
		}

		out[m.Name] = codeOfCall(reflect.ValueOf(svc).MethodByName(m.Name), args)
	}

	return out
}

func codeOfCall(fn reflect.Value, args []reflect.Value) (code string) {
	defer func() {
		if recover() != nil {
			code = "panic"
		}
	}()

	res := fn.Call(args)
	if last := res[len(res)-1]; !last.IsNil() {
		if se, ok := errors.AsType[*service.Error](last.Interface().(error)); ok {
			return se.Code
		}

		return "error"
	}

	return ""
}

func TestSecurity_T107_EveryOperationIsRefusedWhileTheStudioIsLocked(t *testing.T) {
	t.Parallel()

	g := &gate{}
	e := newEnvWith(t, func(_ *env, o *service.Options) { o.Gate = g })

	all := service.WithCaller(context.Background(), service.Caller{
		Actor: "local:ui", Scopes: service.AllScopes, Browser: true,
	})

	g.locked.Store(true)

	locked := map[string]map[string]string{
		"a full caller": callEvery(all, t, e.svc),
		"no caller":     callEvery(context.Background(), t, e.svc),
	}

	for who, codes := range locked {
		for name, code := range codes {
			allowed := service.LockedAllowed[name]

			switch {
			case allowed && code == service.CodeLocked:
				t.Errorf("%s: %s is on the allowed list and answered E_LOCKED", who, name)
			case !allowed && code != service.CodeLocked:
				t.Errorf("%s: %s answered %q while locked, want E_LOCKED", who, name, code)
			}
		}
	}

	// The allowed list names real operations, so a rename cannot leave a stale entry that allows nothing.
	for name := range service.LockedAllowed {
		if _, ok := locked["a full caller"][name]; !ok {
			t.Errorf("LockedAllowed names %s, which is not a method of Service", name)
		}
	}

	// Open again: no operation answers E_LOCKED (the others fail for what a zero request is).
	g.locked.Store(false)

	for name, code := range callEvery(all, t, e.svc) {
		if code == service.CodeLocked {
			t.Errorf("%s answered E_LOCKED with the gate open", name)
		}
	}
}

func TestLockedErrorHasTheCodeTheHintAndTheStatus(t *testing.T) {
	t.Parallel()

	g := &gate{}
	g.locked.Store(true)

	e := newEnvWith(t, func(_ *env, o *service.Options) { o.Gate = g })

	_, err := e.svc.ProjectList(service.WithCaller(context.Background(), service.Caller{
		Actor: "local:ui", Scopes: service.AllScopes,
	}), service.ProjectListRequest{})

	var se *service.Error
	if !errors.As(err, &se) || se.Code != service.CodeLocked {
		t.Fatalf("got %v", err)
	}

	if se.HTTPStatus() != 423 || se.Retryable || se.Details["reason"] != "locked" ||
		se.Hint != "unlock Stuarty with your password" {
		t.Fatalf("the shape of the error: %+v status %d", se, se.HTTPStatus())
	}
}
