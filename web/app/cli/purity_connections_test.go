package cli

import (
	"context"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/connections"
)

func (s stub) ConnectionList(context.Context, service.ConnectionListRequest) (*service.ConnectionListResult, error) {
	return reply[service.ConnectionListResult](s.err)
}

func (s stub) ConnectionGet(context.Context, service.ConnectionRef) (*connections.Connection, error) {
	return reply[connections.Connection](s.err)
}

func (s stub) ConnectionCreate(context.Context, service.ConnectionCreateRequest) (*connections.Connection, error) {
	return reply[connections.Connection](s.err)
}

func (s stub) ConnectionRemove(context.Context, service.ConnectionRef) (*connections.Connection, error) {
	return reply[connections.Connection](s.err)
}

func (s stub) ConnectionTest(context.Context, service.ConnectionRef) (*service.ConnectionTestResult, error) {
	return reply[service.ConnectionTestResult](s.err)
}
