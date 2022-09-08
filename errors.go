package provider

import (
	"errors"
	"strings"
	"time"
)

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

// timeError is a container for an error
// and a timestamp of when that error occured.
type timeError struct {
	time.Time
	error
}

// Error implements the error interface on timeError.
func (te *timeError) Error() string {
	if te.error != nil {
		return te.error.Error()
	} else {
		return ""
	}
}

// NewTimeError tries to map err to one of the known error values in ErrorPrefixes and
// it returns a timeError value set to either the error it found in ErrorPrefixes,
// or set to the caller's err. Time is always set to Now().
//
// Its used to return error codes over RPC connections because RPC calls
// provide only the type returned by errors.New().
//
// We encode the type that is meant to be returned in the error string as the first value, followed with a ":'.
//
//  return fmt.Errorf("%s: some problem happened with a function: %v", provider.ErrorPrefixes[ErrPermissionDenied], errors.New("some IO problem"))
func NewTimeError(err error) timeError {
	var extract = func(err error) error { // split err along ":"
		split := strings.Split(err.Error(), ":")
		if len(split) < 2 {
			return err
		}

		// if split[0] is a known error in reverseProviderErrors,
		// extract it and otherwise do nothing.
		if err, found := reverseErrorPrefixes[split[0]]; found {
			return err
		}

		return err
	}
	//
	if err != nil {
		err = extract(err)
	}
	//
	return timeError{
		Time:  time.Now(),
		error: err,
	}
}
