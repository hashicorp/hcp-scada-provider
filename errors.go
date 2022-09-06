package provider

import (
	"errors"
	"time"
)

// Todo: good strings and good descriptions
var (
	// these are provider side errors
	ErrProviderNotStarted = errors.New("the provider is not started")                            // the provider is not started
	ErrInvalidCredentials = errors.New("could not obtain a token with the supplied credentials") // could not obtain a token with the configured credentials
	// this is a broker side error
	ErrPermissionDenied = errors.New("principal does not have the permision to register as a provider") // the principal behind the creds does not have permission to register as provider.
)

// ProviderErrors maintains a mapping between error types and their variable names.
// The broker is using those to return error codes over RPC connections. RPC calls
// provide only the type returned by errors.New().
var ProviderErrors = map[error]string{
	ErrProviderNotStarted: "ErrProviderNotStarted",
	ErrInvalidCredentials: "ErrInvalidCredentials",
	ErrPermissionDenied:   "ErrPermissionDenied",
}

// errorTime is a container for an error
// and a timestamp of when that error occured.
type errorTime struct {
	error
	time.Time
}

func (et *errorTime) Set(err error) {
	et.error = err
	et.Time = time.Now()
}

func (et *errorTime) Reset() {
	et.error = nil
	et.Time = time.Time{}
}
