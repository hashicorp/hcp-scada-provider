package provider

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestReverseProviderErrors verifies that init correctly
// sets up reverseProviderErrors.
func TestReverseProviderErrors(t *testing.T) {
	var r = require.New(t)

	for k, v := range ErrorPrefixes {
		err, found := reverseErrorPrefixes[v]
		r.True(found, "error %s not found in reverseProviderErrors", v)
		r.Equal(err, k, "expected error %s", v)
	}
}
