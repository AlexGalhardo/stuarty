package meta

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/connections/oauth"
	"github.com/AlexGalhardo/stuarty/service/social"
	"github.com/AlexGalhardo/stuarty/service/untrusted"
)

// maxTitle is the most of the title of an account that is kept; it is text from outside.
const maxTitle = 100

// Page is a Facebook Page the sign-in reached, with the professional Instagram account linked to it, if any.
// It holds the token of the Page for the length of the sign-in, and is never returned by a connector.
type Page struct {
	ID    string
	Name  string
	Token string
	// IGID and IGUsername are the linked Instagram professional account; empty when there is none.
	IGID       string
	IGUsername string
}

// Kind is what differs between the networks that share the sign-in.
type Kind struct {
	// Network is the name of the network and the kind of its connections: instagram or facebook.
	Network string
	Name    string
	// Scopes are the permissions the sign-in asks for.
	Scopes []string
	// Fields names the account when read from the Graph API: id,name for a Page, id,username for Instagram.
	Fields string
	// Offer says which account a Page gives the person to choose, and false when it gives none.
	Offer func(p Page) (id, title string, ok bool)
}

// Base is the connector part every Meta network shares: the sign-in, the account and the Graph client. A
// network embeds it and adds Describe, Validate, Publish and Status.
type Base struct {
	cfg  Config
	kind Kind
}

// NewBase returns the shared part for a network; a zero field of cfg takes its default.
func NewBase(cfg Config, k Kind) *Base { return &Base{cfg: cfg.WithDefaults(), kind: k} }

// Config is the configuration the base uses, with the defaults filled.
func (b *Base) Config() Config { return b.cfg }

// Network is the name of the network.
func (b *Base) Network() string { return b.kind.Network }

type offered struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Token string `json:"token"`
}

// secret is the secret of a Meta connection, the one document in the vault (SDD-020 section 17). After the
// sign-in it holds the accounts offered, each with the token of its Page; after the choice it holds the
// chosen account and its token alone.
type secret struct {
	ClientID     string    `json:"client_id"`
	ClientSecret string    `json:"client_secret"`
	Offered      []offered `json:"offered,omitempty"`
	AccountID    string    `json:"account_id,omitempty"`
	AccountTitle string    `json:"account_title,omitempty"`
	Token        string    `json:"token,omitempty"`
}

func (s *secret) marshal() ([]byte, error) {
	return json.Marshal(s) //nolint:gosec // G117: this is the secret of a connection, made to go into the vault
}

func (b *Base) parse(raw []byte) (secret, error) {
	var s secret
	if err := json.Unmarshal(raw, &s); err != nil || s.ClientID == "" || s.ClientSecret == "" {
		return secret{}, social.NewError(
			social.KindAuth, b.kind.Network, ReasonBadSecret, 0, errors.New("meta: the secret is not a client"),
		)
	}

	return s, nil
}

// Chosen is the account a connection publishes to and the token of its Page. The token lives in memory for
// the length of one operation.
type Chosen struct {
	ID    string
	Title string
	Token string
}

// Chosen reads the chosen account of a credential. A connection with no choice yet is not signed in.
func (b *Base) Chosen(cred connections.Credential) (Chosen, error) {
	s, err := b.parse(cred.Secret)
	if err != nil {
		return Chosen{}, err
	}

	if s.Token == "" || s.AccountID == "" {
		return Chosen{}, social.NewError(social.KindAuth, b.kind.Network, ReasonNeedsSignIn, 0, nil)
	}

	return Chosen{ID: s.AccountID, Title: s.AccountTitle, Token: s.Token}, nil
}

// AuthConfig implements social.Connector.
func (b *Base) AuthConfig() social.AuthConfig {
	return social.AuthConfig{AuthURL: b.cfg.AuthURL, TokenURL: b.cfg.graph("oauth/access_token"), Scopes: b.kind.Scopes}
}

// Begin implements social.Connector.
func (b *Base) Begin(clientID, clientSecret string) ([]byte, error) {
	clientID, clientSecret = strings.TrimSpace(clientID), strings.TrimSpace(clientSecret)

	if clientID == "" || clientSecret == "" {
		return nil, errors.New("meta: the app id and the app secret are required")
	}

	s := secret{ClientID: clientID, ClientSecret: clientSecret}

	return s.marshal()
}

// ClientID implements social.Connector.
func (b *Base) ClientID(raw []byte) (string, bool) {
	s, err := b.parse(raw)

	return s.ClientID, err == nil
}

// SignedIn implements social.Connector: true once an account is chosen.
func (b *Base) SignedIn(raw []byte) (social.Account, bool) {
	s, err := b.parse(raw)
	if err != nil || s.Token == "" || s.AccountID == "" {
		return social.Account{}, false
	}

	return social.Account{ID: s.AccountID, Title: s.AccountTitle}, true
}

var _ social.Chooser = (*Base)(nil)

// Choices implements social.Chooser: the accounts of the sign-in, with no token.
func (b *Base) Choices(raw []byte) []social.Account {
	s, err := b.parse(raw)
	if err != nil || s.Token != "" {
		return nil
	}

	out := make([]social.Account, 0, len(s.Offered))
	for _, o := range s.Offered {
		out = append(out, social.Account{ID: o.ID, Title: o.Title})
	}

	return out
}

// Choose implements social.Chooser: the secret with the one account and its token, and no other.
func (b *Base) Choose(raw []byte, accountID string) ([]byte, error) {
	s, err := b.parse(raw)
	if err != nil {
		return nil, err
	}

	i := slices.IndexFunc(s.Offered, func(o offered) bool { return o.ID == accountID && accountID != "" })
	if i < 0 {
		return nil, errors.New("meta: that account is not one the sign-in reached")
	}

	pick := s.Offered[i]
	s.AccountID, s.AccountTitle, s.Token, s.Offered = pick.ID, pick.Title, pick.Token, nil

	return s.marshal()
}

type pageList struct {
	Data []struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		AccessToken string `json:"access_token"`
		IG          *struct {
			ID       string `json:"id"`
			Username string `json:"username"`
		} `json:"instagram_business_account"`
	} `json:"data"`
}

// Exchange implements social.Connector: the code for a token, that for a long-lived one, the Pages it
// reaches with their tokens, and the accounts offered to the person. The new secret goes to the vault; the
// user token is dropped here.
func (b *Base) Exchange(
	ctx context.Context, cred connections.Credential, code, verifier, redirectURI string,
) ([]byte, error) {
	s, err := b.parse(cred.Secret)
	if err != nil {
		return nil, err
	}

	tokenURL := b.cfg.graph("oauth/access_token")

	short, err := oauth.Exchange(ctx, cred.Client, tokenURL, s.ClientID, s.ClientSecret, code, verifier, redirectURI)
	if err != nil {
		return nil, TokenError(b.kind.Network, err)
	}

	long, err := oauth.Request(ctx, cred.Client, tokenURL, url.Values{
		"grant_type": {"fb_exchange_token"}, "client_id": {s.ClientID}, "client_secret": {s.ClientSecret},
		"fb_exchange_token": {short.AccessToken},
	})
	if err != nil {
		return nil, TokenError(b.kind.Network, err)
	}

	var list pageList

	q := url.Values{"fields": {"id,name,access_token,instagram_business_account{id,username}"}, "limit": {"100"}}

	if cerr := b.Call(ctx, cred.Client, http.MethodGet, "me/accounts", q, long.AccessToken, &list); cerr != nil {
		return nil, cerr
	}

	s.Offered = s.Offered[:0]
	s.AccountID, s.AccountTitle, s.Token = "", "", ""

	for _, p := range list.Data {
		page := Page{ID: p.ID, Name: p.Name, Token: p.AccessToken}
		if p.IG != nil {
			page.IGID, page.IGUsername = p.IG.ID, p.IG.Username
		}

		id, title, ok := b.kind.Offer(page)
		if !ok || id == "" || page.Token == "" {
			continue
		}

		kept, _ := untrusted.Cut(untrusted.Clean(title), maxTitle)
		s.Offered = append(s.Offered, offered{ID: id, Title: kept, Token: page.Token})
	}

	if len(s.Offered) == 0 {
		return nil, social.NewError(social.KindNotEligible, b.kind.Network, ReasonNoAccounts, http.StatusOK, nil)
	}

	return s.marshal()
}

type accountAnswer struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Username string `json:"username"`
}

// Account implements social.Connector: the account the stored token belongs to, read again from Meta.
func (b *Base) Account(ctx context.Context, cred connections.Credential) (social.Account, error) {
	acc, _, err := b.read(ctx, cred)

	return acc, err
}

func (b *Base) read(ctx context.Context, cred connections.Credential) (social.Account, []byte, error) {
	ch, err := b.Chosen(cred)
	if err != nil {
		return social.Account{}, nil, err
	}

	var raw json.RawMessage

	q := url.Values{"fields": {b.kind.Fields}}

	if cerr := b.Call(ctx, cred.Client, http.MethodGet, ch.ID, q, ch.Token, &raw); cerr != nil {
		return social.Account{}, raw, cerr
	}

	var a accountAnswer
	if json.Unmarshal(raw, &a) != nil || a.ID != ch.ID {
		return social.Account{}, raw, social.NewError(
			social.KindAuth,
			b.kind.Network,
			ReasonTokenInvalid,
			http.StatusOK,
			nil,
		)
	}

	title := a.Name
	if a.Username != "" {
		title = "@" + a.Username
	}

	kept, _ := untrusted.Cut(untrusted.Clean(title), maxTitle)

	return social.Account{ID: a.ID, Title: kept}, raw, nil
}

// Test implements connections.Connector: one read of the account, the harmless call that also proves the
// token works. The answer is shown as untrusted text, and never holds a credential.
func (b *Base) Test(ctx context.Context, cred connections.Credential) (connections.TestResult, error) {
	_, raw, err := b.read(ctx, cred)
	if err != nil {
		if GateError(err) {
			return connections.TestResult{}, err
		}

		if se, ok := social.AsError(err); ok {
			return connections.TestResult{Message: "the sign-in does not work: " + se.Kind + "."}, nil
		}

		return connections.TestResult{}, err
	}

	return connections.TestResult{OK: true, Status: http.StatusOK, Message: connections.SafeMessage(cred, raw)}, nil
}
