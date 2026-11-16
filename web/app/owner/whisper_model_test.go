package owner

import (
	"testing"

	"github.com/AlexGalhardo/vukem-video/service"
	"github.com/AlexGalhardo/vukem-video/service/workspace"
)

func TestWhisperProviderTakesAModelByName(t *testing.T) {
	o := &Owner{Dir: workspace.Dir(t.TempDir())}

	_, p, err := o.tools("small", false)
	if err != nil || p == nil {
		t.Skipf("this build pins no helper for the platform: %v", err)
	}

	picker, ok := p.(service.ModelPicker)
	if !ok {
		t.Fatal("the whisper provider is not a ModelPicker")
	}

	if got := picker.InstalledModels(); len(got) != 0 {
		t.Errorf("a fresh workspace has installed models %v", got)
	}

	if got := picker.WithModel("base").Model(); got != "base" {
		t.Errorf("WithModel(base).Model() = %q", got)
	}
}
