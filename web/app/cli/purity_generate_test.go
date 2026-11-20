package cli

import (
	"context"

	"github.com/AlexGalhardo/stuarty/service"
)

func (s stub) ImageGenerate(context.Context, service.ImageGenerateRequest) (*service.Job, error) {
	return reply[service.Job](s.err)
}

func (s stub) VoiceGenerate(context.Context, service.VoiceGenerateRequest) (*service.Job, error) {
	return reply[service.Job](s.err)
}

func (s stub) VoiceList(context.Context, service.VoiceListRequest) (*service.VoiceListResult, error) {
	return reply[service.VoiceListResult](s.err)
}
