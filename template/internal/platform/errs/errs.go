// Package errs gives the application a small, closed set of error kinds.
//
// The point is the boundary between a module and the UI. A service returns an
// error; the screen that called it has to decide whether to show "not found",
// "that name is taken", or a generic "something went wrong", and it should not
// have to string-match or type-assert its way to that decision.
package errs

import (
	"errors"
	"fmt"
)

// Kind classifies an error well enough for the UI to react to it.
type Kind int

const (
	// Internal is the zero value on purpose: an error that nobody bothered to
	// classify is one the user cannot act on, and "something went wrong" is the
	// honest thing to show for it.
	Internal Kind = iota

	// NotFound: the thing asked for does not exist.
	NotFound

	// Invalid: the caller supplied something the domain rejects. The message is
	// safe, and meant, to show to the user.
	Invalid

	// Conflict: the operation collides with existing state, e.g. a duplicate.
	Conflict
)

func (k Kind) String() string {
	switch k {
	case NotFound:
		return "not_found"
	case Invalid:
		return "invalid"
	case Conflict:
		return "conflict"
	default:
		return "internal"
	}
}

// Error carries a Kind alongside the usual message and cause.
type Error struct {
	Kind Kind
	Msg  string
	Err  error
}

func (e *Error) Error() string {
	if e.Err == nil {
		return e.Msg
	}
	return fmt.Sprintf("%s: %v", e.Msg, e.Err)
}

func (e *Error) Unwrap() error { return e.Err }

// NotFoundf builds a NotFound error.
func NotFoundf(format string, a ...any) *Error {
	return &Error{Kind: NotFound, Msg: fmt.Sprintf(format, a...)}
}

// Invalidf builds an Invalid error. Write the message for the user, not the log:
// it is the one kind the UI shows verbatim.
func Invalidf(format string, a ...any) *Error {
	return &Error{Kind: Invalid, Msg: fmt.Sprintf(format, a...)}
}

// Conflictf builds a Conflict error.
func Conflictf(format string, a ...any) *Error {
	return &Error{Kind: Conflict, Msg: fmt.Sprintf(format, a...)}
}

// Internalf wraps cause as an Internal error.
func Internalf(cause error, format string, a ...any) *Error {
	return &Error{Kind: Internal, Msg: fmt.Sprintf(format, a...), Err: cause}
}

// KindOf reports the Kind of err, unwrapping as it goes. Anything that is not an
// *Error is Internal, including nil, which no caller should be asking about.
func KindOf(err error) Kind {
	var e *Error
	if errors.As(err, &e) {
		return e.Kind
	}
	return Internal
}

// UserMessage returns a message safe to put in front of a user.
//
// Internal errors deliberately collapse to a fixed string: their text is written
// for whoever reads the log and tends to leak paths, SQL, or driver noise that
// tells the user nothing and the curious rather more than we meant to.
func UserMessage(err error) string {
	if err == nil {
		return ""
	}
	var e *Error
	if errors.As(err, &e) && e.Kind != Internal {
		return e.Msg
	}
	return "Something went wrong. Check the log for details."
}
