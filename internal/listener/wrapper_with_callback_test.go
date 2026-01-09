// Copyright IBM Corp. 2022, 2025
// SPDX-License-Identifier: MPL-2.0

package listener

import (
	"fmt"
	"net"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestListenerProxy(t *testing.T) {
	t.Run("Create a listener wrapper", func(t *testing.T) {
		givenListenerMock := givenListenerMock()

		// When create a new wrapper
		givenListenerWithCallback := WithCloseCallback(givenListenerMock, noop())

		require.NotNil(t, givenListenerWithCallback)
	})

	t.Run("Accept call is proxied to the wrapped listener when is called", func(t *testing.T) {
		givenListenerMock := givenListenerMock()
		givenListenerMock.On("Accept").Return()
		givenListenerWithCallback := WithCloseCallback(givenListenerMock, noop())

		_, err := givenListenerWithCallback.Accept()

		require.NoError(t, err)
		givenListenerMock.AssertExpectations(t)
	})

	t.Run("Close call is proxied to the wrapped listener when is called", func(t *testing.T) {
		givenListenerMock := givenListenerMock()
		givenListenerMock.On("Close").Return(nil)
		givenListenerWithCallback := WithCloseCallback(givenListenerMock, noop())

		err := givenListenerWithCallback.Close()

		require.NoError(t, err)
		givenListenerMock.AssertExpectations(t)
	})

	t.Run("Close call is proxied to the wrapped listener when is called", func(t *testing.T) {
		givenListenerMock := givenListenerMock()
		givenListenerMock.On("Close").Return(nil)
		givenListenerWithCallback := WithCloseCallback(givenListenerMock, nil)

		err := givenListenerWithCallback.Close()

		require.NoError(t, err)
		givenListenerMock.AssertExpectations(t)
	})

	t.Run("Addr call is proxied to the wrapped listener when is called", func(t *testing.T) {
		givenListenerMock := givenListenerMock()
		givenListenerMock.On("Addr").Return()
		givenListenerWithCallback := WithCloseCallback(givenListenerMock, nil)

		givenListenerWithCallback.Addr()

		givenListenerMock.AssertExpectations(t)
	})

	t.Run("Listener wrapper calls back when close is called", func(t *testing.T) {
		var callbackCalled bool
		givenListenerMock := givenListenerMock()
		givenListenerMock.On("Close").Return(nil)
		givenListenerWithCallback := WithCloseCallback(givenListenerMock, func() {
			callbackCalled = true
		})

		err := givenListenerWithCallback.Close()

		require.NoError(t, err)
		require.True(t, callbackCalled, "Callback is expected to be called")
	})

	t.Run("Listener wrapper calls back only once when close is called", func(t *testing.T) {
		var timesCallbackCalled uint32
		givenListenerMock := givenListenerMock()
		givenListenerMock.On("Close").Return(nil)
		givenListenerWithCallback := WithCloseCallback(givenListenerMock, func() {
			atomic.AddUint32(&timesCallbackCalled, 1)
		})

		// When close is called multiple times
		_ = givenListenerWithCallback.Close()
		err := givenListenerWithCallback.Close()

		require.NoError(t, err)
		require.Equal(t, uint32(1), timesCallbackCalled, "Callback is expected to be called only once")
	})

	t.Run("Listener wrapper callback callback is not called when the wrapped listener fails to close", func(t *testing.T) {
		var callbackCalled bool
		givenListenerMock := givenListenerMock()
		givenListenerMock.On("Close").Return(fmt.Errorf("can't close"))
		givenListenerWithCallback := WithCloseCallback(givenListenerMock, func() {
			callbackCalled = true
		})

		// When close is called
		err := givenListenerWithCallback.Close()

		require.Error(t, err)
		require.False(t, callbackCalled, "Callback is not expected to be called")
	})
}

func noop() func() {
	return func() {
	}
}

func givenListenerMock() *ListenerMock {
	return &ListenerMock{}
}

type ListenerMock struct {
	mock.Mock
}

func (l *ListenerMock) Accept() (net.Conn, error) {
	l.Called()
	return nil, nil
}

func (l *ListenerMock) Close() error {
	args := l.Called()
	return args.Error(0)
}

func (l *ListenerMock) Addr() net.Addr {
	l.Called()
	return nil
}
