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

// reverseProviderErrors keeps a reverse mapping of ProviderErrors.
// It's automatically created at start by init().
var reverseProviderErrors map[string]error

func init() {
	reverseProviderErrors = make(map[string]error, len(ProviderErrors))
	//
	for k, v := range ProviderErrors {
		reverseProviderErrors[v] = k
	}
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

// Extract tries to map s to one of the known error values in ProviderErrors
// and if it finds one, it calls Set on et. If none are found it does not change
// anything and returns.
func (et *errorTime) Extract(s string) {
	// if s is a known error in reverseProviderErrors,
	// extract it and otherwise do nothing.
	if err, found := reverseProviderErrors[s]; found {
		et.Set(err)
	}

	return
}
