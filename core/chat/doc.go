// Package chat holds the pure rules of the hosted agent chat (ADR-0037, SDD-017):
// the states of a session and their transitions, what a profile may contain, what
// a permission request is and which answers a person may give, how agent text is
// cleaned and clipped, and how a transcript is exported as Markdown. It starts no
// process, reads no file and opens no connection: the process, the protocol and
// the storage are in service/agenthost.
package chat

import "time"

// The limits of SDD-017 sections 3, 4 and 11.
const (
	MaxProfiles        = 16
	MaxCommandLength   = 1024
	MaxArgs            = 32
	MaxArgLength       = 512
	MaxMessageLength   = 20000
	MaxEventText       = 50000
	MaxToolOutput      = 8000
	MaxTitleLength     = 80
	MaxEventsPerChat   = 5000
	MaxLiveSessions    = 4
	MaxSessionsPerProj = 200
	MaxStreamsPerCred  = 8
	MaxLineBytes       = 1 << 20
	MaxOptions         = 8
	MaxOptionName      = 120
	MaxTranscriptPage  = 500
)

// The times of SDD-017 section 4.
const (
	StartTimeout      = 30 * time.Second
	TurnTimeout       = 15 * time.Minute
	CancelGrace       = 5 * time.Second
	InputCloseGrace   = 3 * time.Second
	PermissionTimeout = 10 * time.Minute
	IdleTimeout       = 30 * time.Minute
	SessionLifetime   = 8 * time.Hour
)

// The flood rule of SDD-017 section 4: more than FloodEventsPerSecond events per
// second for FloodSeconds seconds in a row ends the session.
const (
	FloodEventsPerSecond = 200
	FloodSeconds         = 5
)

// ID prefixes of SDD-000 section 2.
const (
	SessionPrefix    = "cht"
	PermissionPrefix = "cpr"
)
