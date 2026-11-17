package command

import "testing"

func TestShapeSizeLimit(t *testing.T) {
	args := `{"shape": "rect", "width": 99999999, "height": 150, "start": "0f", "duration": "30f", "id": "itm_s6t7v8w9x0"}`
	_, err := Default().Apply(base().d, op2("shape.add", args), Env{})
	if err == nil || AsError(err).Path != "args.width" {
		t.Fatalf("huge width accepted: %v", err)
	}
}
