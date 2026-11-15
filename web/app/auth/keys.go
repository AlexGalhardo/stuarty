package auth

import "context"

// KeyPrefix starts the secret of every API key, so secret scanners recognise a
// leak (SDD-007 section 4.2).
const KeyPrefix = "vvk_"

// RevokeKeysOf revokes every key of a user at once.
func (n *Network) RevokeKeysOf(ctx context.Context, userID string) error {
	return n.tokens.RevokeAll(ctx, userID)
}
