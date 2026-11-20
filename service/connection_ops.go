package service

import (
	"context"
	"errors"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/internal/outbound"
	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/connections/oauth"
	"github.com/AlexGalhardo/stuarty/service/providers"
	"github.com/AlexGalhardo/stuarty/service/publish/destinations"
)

// ConnectionListRequest has no member.
type ConnectionListRequest struct{}

// ConnectionListResult is every connection of the workspace, oldest first.
type ConnectionListResult struct {
	Connections []connections.Connection `json:"connections"`
}

// ConnectionRef names a connection.
type ConnectionRef struct {
	ID string `json:"id"`
}

// ConnectionCreateRequest is SDD-014 section 5. Secret is the credential of the
// destination; it goes into the vault and is in no result.
type ConnectionCreateRequest struct {
	Kind     string   `json:"kind"`
	Label    string   `json:"label"`
	Endpoint string   `json:"endpoint"`
	Hosts    []string `json:"hosts,omitempty"`
	Accepts  []string `json:"accepts"`
	Secret   string   `json:"secret"`
}

// ConnectionTestResult is the outcome of one request through a connection.
type ConnectionTestResult struct {
	ID     string `json:"id"`
	OK     bool   `json:"ok"`
	Status int    `json:"status"`
	// Message is the start of the answer of the destination, delimited as untrusted text.
	Message string `json:"message" untrusted:"true"`
}

// connectionCaller is what the five operations share: the scope of the method,
// and no project-limited key, since a connection belongs to the workspace.
func (s *svc) connectionCaller(ctx context.Context, method string) (Caller, error) {
	c, err := authorize(ctx, method, "")
	if err != nil {
		return Caller{}, err
	}

	if c.ProjectID != "" {
		return Caller{}, newError(CodeForbidden, "the key is limited to one project and cannot use connections.").
			withHint("use a key without a project limit")
	}

	return c, nil
}

// connectionError maps the errors of the registry, the gate and the vault to
// the codes of SDD-014 section 7. The text of a vault error says how to
// recover and never holds a secret or a key.
func connectionError(err error) error {
	if err == nil {
		return nil
	}

	if fe, ok := errors.AsType[*connections.FieldError](err); ok {
		return newError(command.CodeValidation, "%s", fe.Message).withPath(fe.Path)
	}

	switch {
	case errors.Is(err, connections.ErrNotFound), errors.Is(err, outbound.ErrNoConnection):
		return newError(CodeConnectionNotFound, "the connection does not exist or was removed.").
			withHint("list the connections with connection:list")
	case errors.Is(err, outbound.ErrDestination), errors.Is(err, outbound.ErrNotPublic):
		return newError(CodeConnectionDestination, "the request is for a destination the connection does not allow.").
			withHint("a connection reaches only the hosts it lists, over https")
	case errors.Is(err, connections.ErrNeedsSignIn):
		return providerError(&providers.Error{Kind: providers.KindAuth, Code: signInProviderCode})
	case errors.Is(err, connections.ErrSignInUnavailable):
		return providerError(&providers.Error{Kind: providers.KindUnavailable})
	}

	if reason, hint, ok := connections.Locked(err); ok {
		return newError(CodeVaultLocked, "the vault of credentials cannot be opened.").
			withDetail("reason", reason).withHint("%s", hint)
	}

	return asError(err)
}

// ConnectionList returns every connection.
func (s *svc) ConnectionList(ctx context.Context, _ ConnectionListRequest) (*ConnectionListResult, error) {
	if _, err := s.connectionCaller(ctx, "ConnectionList"); err != nil {
		return nil, err
	}

	list, err := s.conns.List(ctx)
	if err != nil {
		return nil, connectionError(err)
	}

	return &ConnectionListResult{Connections: list}, nil
}

// ConnectionGet returns one connection.
func (s *svc) ConnectionGet(ctx context.Context, req ConnectionRef) (*connections.Connection, error) {
	if _, err := s.connectionCaller(ctx, "ConnectionGet"); err != nil {
		return nil, err
	}

	conn, err := s.conns.Get(ctx, req.ID)
	if err != nil {
		return nil, connectionError(err)
	}

	return &conn, nil
}

// ConnectionCreate validates a connection, puts its credential in the vault and
// records it, with an audit row.
func (s *svc) ConnectionCreate(ctx context.Context, req ConnectionCreateRequest) (*connections.Connection, error) {
	c, err := s.connectionCaller(ctx, "ConnectionCreate")
	if err != nil {
		return nil, err
	}

	spec, ferr := connections.Normalize(connections.Spec{
		Kind: req.Kind, Label: req.Label, Endpoint: req.Endpoint, Hosts: req.Hosts, Accepts: req.Accepts,
	}, s.conns.Kinds())
	if ferr != nil {
		return nil, connectionError(ferr)
	}

	if n := len(req.Secret); n == 0 || n > connections.MaxSecret {
		return nil, newError(command.CodeValidation, "the secret must be 1 to %d bytes.", connections.MaxSecret).
			withPath("secret")
	}

	// The stored form of a sign-in is made by the sign-in only (SDD-014 section 13): a secret in that form,
	// given by hand, would be a credential nobody signed in for.
	if _, isSignIn := oauth.ParseEnvelope([]byte(req.Secret)); isSignIn {
		return nil, newError(command.CodeValidation, "the secret is not a key: a sign-in is made in the browser.").
			withPath("secret")
	}

	// The settings of a blog destination are checked before they are kept: the folder, the signing secret, the
	// address rule of a webhook that is not local (SDD-020 sections 42 and 46, T-86).
	if d, ok := s.blog.Get(spec.Kind); ok {
		if verr := d.ValidateConfig([]byte(req.Secret), spec.Endpoint); verr != nil {
			path := "secret"
			if _, isEndpoint := errors.AsType[*destinations.EndpointError](verr); isEndpoint {
				path = "endpoint"
			}

			return nil, newError(command.CodeValidation, "%s.", verr.Error()).withPath(path)
		}
	}

	return s.insertConnection(ctx, c, spec, []byte(req.Secret))
}

// insertConnection puts the credential of a validated connection in the vault and records it, with an
// audit row. ConnectionCreate and the sign-in of a social network (SDD-020) share it.
func (s *svc) insertConnection(
	ctx context.Context,
	c Caller,
	spec connections.Spec,
	secret []byte,
) (*connections.Connection, error) {
	count, err := s.store.Connections().Count(ctx)
	if err != nil {
		return nil, asError(err)
	}

	if count >= connections.MaxConnections {
		return nil, newError(
			"E_LIMIT_EXCEEDED", "the workspace has the most connections it may have (%d).", connections.MaxConnections,
		).withHint("remove a connection first")
	}

	id := s.newIDs("con")
	now := s.nowMs()

	if err := s.conns.PutSecret(id, secret); err != nil {
		return nil, connectionError(err)
	}

	row := connections.Row{
		ID: id, Kind: spec.Kind, Label: spec.Label, Endpoint: spec.Endpoint, Hosts: spec.Hosts, Accepts: spec.Accepts,
		SecretRef: id, CreatedMs: now, CreatedBy: c.Actor,
	}

	werr := s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
		if err := tx.Connections().Insert(ctx, row); err != nil {
			return err
		}

		return appendAudit(ctx, tx, now, c.Actor, "connection.create", "", id)
	})
	if werr != nil {
		_ = s.conns.DeleteSecret(id)

		return nil, asError(werr)
	}

	conn := connections.View(row, true)

	return &conn, nil
}

// ConnectionRemove deletes the credential of a connection from the vault and
// then the row, so the next request through it fails.
func (s *svc) ConnectionRemove(ctx context.Context, req ConnectionRef) (*connections.Connection, error) {
	c, err := s.connectionCaller(ctx, "ConnectionRemove")
	if err != nil {
		return nil, err
	}

	row, err := s.store.Connections().Get(ctx, req.ID)
	if err != nil {
		return nil, connectionError(err)
	}

	// A sign-in is made and removed by a person in the browser: an agent, the CLI and the local token read
	// that it exists and cannot take it away (SDD-014 section 13, T-75).
	if !c.Browser && s.isSignIn(ctx, row.ID) {
		return nil, privilegedError()
	}

	if err := s.conns.DeleteSecret(row.SecretRef); err != nil {
		return nil, connectionError(err)
	}

	now := s.nowMs()

	werr := s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
		if err := tx.Connections().Delete(ctx, row.ID); err != nil {
			return err
		}

		return appendAudit(ctx, tx, now, c.Actor, "connection.remove", "", row.ID)
	})
	if werr != nil {
		return nil, connectionError(werr)
	}

	conn := connections.View(row, false)

	return &conn, nil
}

// ConnectionTest makes one request through the connection.
func (s *svc) ConnectionTest(ctx context.Context, req ConnectionRef) (*ConnectionTestResult, error) {
	if _, err := s.connectionCaller(ctx, "ConnectionTest"); err != nil {
		return nil, err
	}

	res, err := s.conns.Test(ctx, req.ID)
	if err != nil {
		return nil, connectionError(err)
	}

	return &ConnectionTestResult{ID: req.ID, OK: res.OK, Status: res.Status, Message: res.Message}, nil
}
