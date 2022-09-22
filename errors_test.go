package provider

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

var (
	errTimeErrorUnknown = errors.New("testing error")
	errTimeErrorKnown   = errors.New("ErrProviderNotStarted: testing error")
)

// TestReverseErrorPrefixes verifies that init correctly
// sets up reverseErrorPrefixes.
func TestReverseErrorPrefixes(t *testing.T) {
	var r = require.New(t)

	for k, v := range ErrorPrefixes {
		err, found := reverseErrorPrefixes[v]
		r.True(found, "error %s not found in reverseProviderErrors", v)
		r.Equal(err, k, "expected error %s", v)
	}
}

func TestNewTimeErrorUnknown(t *testing.T) {
	var r = require.New(t)

	te := NewTimeError(errTimeErrorUnknown)
	r.NotEmpty(te.Time)
	r.True(te.Time.Before(time.Now()))
	r.Equal(errTimeErrorUnknown, te.error)
}

// TestNewTimeErrorBase checks the standard behavior of timeError.
// creating a new timeError with an unknown value and checking it's value.
func TestNewTimeErrorKnown(t *testing.T) {
	var r = require.New(t)

	te := NewTimeError(errTimeErrorKnown)
	r.NotEmpty(te.Time)
	r.True(te.Time.Before(time.Now()))
	r.Equal(ErrProviderNotStarted, te.error)
}

// TestNewTimeErrorError confirms that timeError.Error() functions correctly.
func TestNewTimeErrorError(t *testing.T) {
	var r = require.New(t)

	te := NewTimeError(errTimeErrorUnknown)
	r.Equal(errTimeErrorUnknown.Error(), te.Error())

}

// TestNewTimeErrorNil assuages that NewTimeError functions correctly when created with a nil value.
func TestNewTimeErrorNil(t *testing.T) {
	var r = require.New(t)

	te := NewTimeError(nil)
	r.NotEmpty(te.Time)
	r.True(te.Time.Before(time.Now()))
	r.Nil(te.error)
}

// TestNewTimeErrorErrorNil is a check on the behavior of timeError.Error()
// when timeError.error is nil.
func TestNewTimeErrorErrorNil(t *testing.T) {
	var r = require.New(t)

	te := NewTimeError(nil)
	r.Equal("", te.Error())
}

func TestPrefixErrorRetrieveError(t *testing.T) {
	var r = require.New(t)

	var err *oauth2.RetrieveError
	if err := PrefixError("failed to get access token", err); err != nil {
		r.Equal("ErrInvalidCredentials: failed to get access token: <nil>", err.Error())
	} else {
		t.Error("expected a prefixed error")
	}
}

func TestPrefixErrorNil(t *testing.T) {
	var r = require.New(t)

	var err error
	if err := PrefixError("failed to get access token", err); err != nil {
		r.Equal("failed to get access token", err.Error())
	} else {
		t.Error("expected a prefixed error")
	}
}

func TestPrefixErrorOther(t *testing.T) {
	var r = require.New(t)

	var err = errTimeErrorUnknown
	if err := PrefixError("failed to get access token", err); err != nil {
		r.Equal("failed to get access token: testing error", err.Error())
	} else {
		t.Error("expected a prefixed error")
	}
}
