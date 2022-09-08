package provider

import "errors"

var (
	// these are provider side errors
	ErrProviderNotStarted = errors.New("the provider is not started")                            // the provider is not started
	ErrInvalidCredentials = errors.New("could not obtain a token with the supplied credentials") // could not obtain a token with the configured credentials
	// this is a broker side error
	ErrPermissionDenied = errors.New("principal does not have the permision to register as a provider") // the principal behind the creds does not have permission to register as provider.
)

// ErrorPrefixes maintains a mapping between error types and their variable names.
// The broker is using those to return error codes over RPC connections. RPC calls
// provide only the type returned by errors.New().
var ErrorPrefixes = map[error]string{
	ErrProviderNotStarted: "ErrProviderNotStarted",
	ErrInvalidCredentials: "ErrInvalidCredentials",
	ErrPermissionDenied:   "ErrPermissionDenied",
}

// reverseErrorPrefixes keeps a reverse mapping of ProviderErrors.
// It's automatically created at start by init().
var reverseErrorPrefixes map[string]error

func init() {
	reverseErrorPrefixes = make(map[string]error, len(ErrorPrefixes))
	//
	for k, v := range ErrorPrefixes {
		reverseErrorPrefixes[v] = k
	}
}
