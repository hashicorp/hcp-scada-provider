// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package listener

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hashicorp/hcp-scada-provider/capability"
)

func TestListener_Listener(t *testing.T) {
	require := require.New(t)
	list := newScadaListener("armon/test")
	defer list.Close()

	var raw interface{} = list
	_, ok := raw.(net.Listener)
	require.True(ok)

	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()

	go func() {
		_ = list.Push(a)
	}()
	out, err := list.Accept()
	require.NoError(err)
	require.Equal(a, out)
}

func TestListener_Addr(t *testing.T) {
	var addr interface{} = capability.NewAddr("armon/test")
	_, ok := addr.(net.Addr)
	require.True(t, ok)
}
