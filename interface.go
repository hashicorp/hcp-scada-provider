package provider

import (
	"net"
)

// SCADAProvider allows to expose services via SCADA capabilities.
type SCADAProvider interface {
	// UpdateMeta updates the internal map of meta-data values
	// and performs a rehandshake to update the broker with the new values.
	UpdateMeta(map[string]string)

	// GetMeta returns the provider's current meta-data.
	GetMeta() map[string]string

	// Listen will expose the provided capability and make new connections
	// available through the returned listener. Closing the listener will stop
	// exposing the provided capability.
	//
	// The method will return an existing listener if the capability already existed.
	// Listeners can be retrieved even when the provider is stopped (e.g. before it is
	// started). New capabilities and new-meta data can be added at any time.
	//
	// The listener will only be closed, if it is closed explicitly by calling Close().
	// The listener will not be closed due to errors or when the provider is stopped.
	// The listener can hence be used after a restart of the provider.
	Listen(capability string) (net.Listener, error)

	// Start will register the provider on the SCADA broker and expose the
	// registered capabilities.
	Start() error

	// Stop will try to gracefully close the currently active SCADA session. This will
	// not close the capability listeners.
	Stop() error

	// SessionStatus will return the status of the SCADA connection.
	SessionStatus() SessionStatus
}

// SessionStatus is used to express the current status of the SCADA session.
type SessionStatus = string

const (
	// SessionStatusDisconnected is the state of the SCADA session if the
	// provider has not been started or has been stopped.
	SessionStatusDisconnected = SessionStatus("disconnected")

	// SessionStatusConnecting is the initial state of the SCADA connection
	// as well as the state it will be in if the connection got disrupted and
	// the library is trying to reconnect.
	//
	// The connection will transition to connected once the SCADA session is
	// established.
	SessionStatusConnecting = SessionStatus("connecting")

	// SessionStatusConnected is the state of the SCADA session if the
	// session is established and active.
	SessionStatusConnected = SessionStatus("connected")

	// SessionStatusRetrying is the state of a SCADA session that was
	// previous connected and is now in a wait-connect cycle
	SessionStatusWaiting = SessionStatus("waiting")
)
