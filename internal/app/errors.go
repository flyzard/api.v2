package app

import "errors"

// Kind classifies an application-layer failure so a transport (HTTP) can map it
// to a status code without inspecting the wrapped cause.
type Kind int

const (
	KindInternal Kind = iota // unexpected; 500
	KindInvalid              // domain validation rejected the input; 400/422
	KindNotFound             // tenant/series/document missing; 404
	KindConflict             // version conflict, idempotency mismatch, series not issuable; 409
	KindAT                   // AT webservice failure; 502
)

// Error is the application layer's error envelope. Services return only *Error.
type Error struct {
	Kind Kind
	Err  error
}

func (e *Error) Error() string { return e.Err.Error() }
func (e *Error) Unwrap() error { return e.Err }

func newError(k Kind, err error) *Error { return &Error{Kind: k, Err: err} }

// KindOf extracts the Kind from an error produced by this layer, defaulting to
// KindInternal for foreign errors.
func KindOf(err error) Kind {
	if e, ok := errors.AsType[*Error](err); ok {
		return e.Kind
	}
	return KindInternal
}

// Sentinel and repository errors. Repositories return ErrNotFound / ErrVersionConflict;
// the service translates and wraps them.
var (
	ErrNotFound            = errors.New("not found")
	ErrVersionConflict     = errors.New("optimistic version conflict")
	ErrSeriesNotIssuable   = errors.New("series not issuable")
	ErrIdempotencyMismatch = errors.New("idempotency key reused with a different payload")
	ErrAlreadyExists       = errors.New("already exists")
)
