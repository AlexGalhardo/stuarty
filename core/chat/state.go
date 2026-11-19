package chat

import "errors"

// State is the state of a session (SDD-017 section 5).
type State string

// The states of a session.
const (
	StateStarting           State = "starting"
	StateIdle               State = "idle"
	StateRunning            State = "running"
	StateAwaitingPermission State = "awaiting_permission"
	StateEnded              State = "ended"
)

// Valid reports whether s is a state.
func (s State) Valid() bool {
	switch s {
	case StateStarting, StateIdle, StateRunning, StateAwaitingPermission, StateEnded:
		return true
	}

	return false
}

// Live reports whether the session still has a process.
func (s State) Live() bool { return s.Valid() && s != StateEnded }

// EndReason is why a session ended.
type EndReason string

// The reasons of SDD-017 section 5.
const (
	EndUser        EndReason = "user"
	EndExited      EndReason = "exited"
	EndProtocol    EndReason = "protocol"
	EndTimeout     EndReason = "timeout"
	EndIdle        EndReason = "idle"
	EndFlood       EndReason = "flood"
	EndShutdown    EndReason = "shutdown"
	EndStartFailed EndReason = "start_failed"
)

// Valid reports whether r is a reason; the empty reason is valid for a live session.
func (r EndReason) Valid() bool {
	switch r {
	case "", EndUser, EndExited, EndProtocol, EndTimeout, EndIdle, EndFlood, EndShutdown, EndStartFailed:
		return true
	}

	return false
}

// Input is what happens to a session.
type Input string

// The inputs of the state machine.
const (
	InputStarted            Input = "started"
	InputSend               Input = "send"
	InputPermissionAsked    Input = "permission_asked"
	InputPermissionAnswered Input = "permission_answered"
	InputTurnEnded          Input = "turn_ended"
	InputCancel             Input = "cancel"
	InputEnd                Input = "end"
)

// The refusals of Next. ErrBusy and ErrEnded are the two reasons of E_INVALID_STATE
// that a person can act on; ErrInvalidTransition is a bug of the caller.
var (
	ErrBusy              = errors.New("chat: a turn is running")
	ErrEnded             = errors.New("chat: the session ended")
	ErrInvalidTransition = errors.New("chat: not allowed in this state")
)

// Next is the one function that moves a session between states.
func Next(s State, in Input) (State, error) {
	if s == StateEnded {
		return s, ErrEnded
	}

	switch in {
	case InputEnd:
		if s.Valid() {
			return StateEnded, nil
		}
	case InputStarted:
		if s == StateStarting {
			return StateIdle, nil
		}
	case InputSend:
		if s == StateIdle {
			return StateRunning, nil
		}

		if s == StateRunning || s == StateAwaitingPermission {
			return s, ErrBusy
		}
	case InputPermissionAsked:
		if s == StateRunning {
			return StateAwaitingPermission, nil
		}
	case InputPermissionAnswered:
		if s == StateAwaitingPermission {
			return StateRunning, nil
		}
	case InputTurnEnded:
		if s == StateRunning || s == StateAwaitingPermission {
			return StateIdle, nil
		}
	case InputCancel:
		// A cancel asks the agent to stop; the state changes when the turn ends. Cancelling
		// a session that waits for a message is allowed and changes nothing.
		if s == StateIdle || s == StateRunning || s == StateAwaitingPermission {
			return s, nil
		}
	}

	return s, ErrInvalidTransition
}
