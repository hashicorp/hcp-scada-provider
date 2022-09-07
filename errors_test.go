package provider

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestReverseProviderErrors verifies that init correctly
// sets up reverseProviderErrors.
func TestReverseProviderErrors(t *testing.T) {
	var r = require.New(t)

	for k, v := range ProviderErrors {
		err, found := reverseProviderErrors[v]
		//
		r.True(found, "error %s not found in reverseProviderErrors", v)
		//
		r.Equal(err, k, "expected error %s", v)
	}
}

func TestSet(t *testing.T) {
	var r = require.New(t)
	var et errorTime

	for k, v := range ProviderErrors {
		et.Set(k)
		r.Equal(k, et.error, "expected error %s", v)
		r.NotEqual(time.Time{}, et.Time, "time should not be zero")
	}
}

func TestSetNil(t *testing.T) {
	var r = require.New(t)
	var et errorTime

	// set et.error to a random value
	for k, _ := range ProviderErrors {
		et.Set(k)
		break
	}
	// verify that et has a value for error
	r.NotNil(et.error, "et.error is not set")
	// set et.error to nil
	et.Set(nil)
	// verify that et.error is now nil
	r.Nil(et.error, "et.error is not nil")
}

func TestReset(t *testing.T) {
	var r = require.New(t)
	var et errorTime

	// set et.error to a random value
	for k, _ := range ProviderErrors {
		et.Set(k)
		break
	}
	// verify that et has a value for error
	r.NotNil(et.error, "et.error is not set")
	// reset it
	et.Reset()
	// verify that et.error is now nil
	r.Nil(et.error, "et.error is not nil")
	// verify that the time in et.Time is zero
	r.Equal(time.Time{}, et.Time, "time should be zero and is %v", et.Time)
}

func TestExtract(t *testing.T) {
	var r = require.New(t)
	var et errorTime

	var err1 = ErrPermissionDenied
	var err2 = http.ErrAbortHandler

	// using the correct format
	var err = fmt.Errorf("%s: some problem happened with a function: %v", ProviderErrors[err1], err2)

	// extract err
	et.Extract(err.Error())
	// et.Error should be ErrPermissionDenied
	r.Equal(ErrPermissionDenied, et.error)
	// time should not be zero
	r.NotEqual(time.Time{}, et.Time, "time should not be zero")
}

func TestExtractBadlyFormated(t *testing.T) {
	var r = require.New(t)
	var et errorTime

	var err1 = ErrPermissionDenied
	var err2 = http.ErrAbortHandler

	// using the correct format without a known error string
	var err = fmt.Errorf("%s: some problem happened with a function: %v", err1.Error(), err2)

	// extract err
	et.Extract(err.Error())
	// et.Error should be nil
	r.Nil(et.error)
	// time should be zero
	r.Equal(time.Time{}, et.Time, "time should be zero and is %v", et.Time)

	// try again with a badly formated error
	et.Reset()
	et.Extract("not using the correct format")
	// et.Error should be nil
	r.Nil(et.error)
	// time should be zero
	r.Equal(time.Time{}, et.Time, "time should be zero and is %v", et.Time)
}
