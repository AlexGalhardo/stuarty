package social

import (
	"sync"

	"github.com/AlexGalhardo/stuarty/core/publish"
)

// Authorizer is implemented by a connector whose consent address is not the one the generic builder makes
// (TikTok names the client `client_key`, separates the scopes with commas and wants the challenge in hex,
// SDD-020 section 34). The challenge it is given is the S256 challenge of the generic flow. A connector that
// does not implement it is signed in with oauth.AuthorizeURL, as before.
type Authorizer interface {
	AuthorizeURL(clientID, redirectURI, state, challenge string) string
}

// Priced is implemented by a connector whose price a person can edit when the network changes it (X,
// SDD-020 section 39). The price lives in the secret document of the connection, where only the connector
// reads it.
type Priced interface {
	// Price is the price kept in the secret of a connection; false when the person set none.
	Price(secret []byte) (publish.Price, bool)
	// SetPrice returns the new secret of the connection with the price kept in it.
	SetPrice(secret []byte, p publish.Price) ([]byte, error)
}

// EffectivePrice is the price a post to the network is estimated at: the one a person set on the connection,
// or the one the connector read from the network's page. False when the network has no price of this kind.
func EffectivePrice(c Connector, secret []byte) (publish.Price, bool) {
	if pc, ok := c.(Priced); ok {
		if p, set := pc.Price(secret); set {
			return p, true
		}
	}

	if p := c.Describe().Price; p != nil {
		return *p, true
	}

	return publish.Price{}, false
}

// Cost is what a post costs to send: the amount, its currency, and whether it is the price of a post with a
// link; zero for a network that charges nothing.
type Cost struct {
	Micros   int64
	Currency string
	Link     bool
	// Date is the date the price was read on the network's page, or edited by a person; empty for a network
	// with a fixed cost.
	Date string
}

// CostFor is the cost of a post to a network: the price, when the network has one (it answers by the text of the
// post), and the fixed cost of its description otherwise.
func CostFor(d Description, price *publish.Price, p *publish.Post) Cost {
	if price != nil {
		micros, link := price.For(publish.Caption(p))
		date := price.Read

		if price.Edited != "" {
			date = price.Edited
		}

		return Cost{Micros: micros, Currency: price.Currency, Link: link, Date: date}
	}

	return Cost{Micros: d.PostCostMicros, Currency: d.PostCostCurrency}
}

// CostOf is the cost of a post to the network the connector serves, given the secret of the connection (for a
// price a person edited).
func CostOf(c Connector, secret []byte, p *publish.Post) Cost {
	var price *publish.Price

	if pr, ok := EffectivePrice(c, secret); ok {
		price = &pr
	}

	return CostFor(c.Describe(), price, p)
}

// Rotation keeps a refresh token that the network replaces at every refresh (SDD-020 section 34). Two sends
// to one connection at once would each present the token the other already replaced; Rotation serialises the
// refreshes of a connection in the process and remembers the newest token it saw, so the second send uses the
// token the first one got. It holds only the latest refresh token of a connection, in memory, and no access
// token.
type Rotation struct {
	mu    sync.Mutex
	locks map[string]*sync.Mutex
	last  map[string]rotated
}

type rotated struct{ from, to string }

// NewRotation returns an empty Rotation.
func NewRotation() *Rotation {
	return &Rotation{locks: map[string]*sync.Mutex{}, last: map[string]rotated{}}
}

// Lock takes the lock of a connection and returns the function that releases it.
func (r *Rotation) Lock(connID string) (unlock func()) {
	r.mu.Lock()

	l, ok := r.locks[connID]
	if !ok {
		l = &sync.Mutex{}
		r.locks[connID] = l
	}

	r.mu.Unlock()
	l.Lock()

	return l.Unlock
}

// Current is the refresh token to present: the one the secret holds, unless this process already traded that
// one for a newer one.
func (r *Rotation) Current(connID, inSecret string) string {
	r.mu.Lock()
	defer r.mu.Unlock()

	if got, ok := r.last[connID]; ok && got.from == inSecret {
		return got.to
	}

	return inSecret
}

// Remember records that the refresh token `from` was traded for `to`.
func (r *Rotation) Remember(connID, from, to string) {
	if from == "" || to == "" || from == to {
		return
	}

	r.mu.Lock()
	r.last[connID] = rotated{from: from, to: to}
	r.mu.Unlock()
}
