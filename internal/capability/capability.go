package capability

import (
	"net"
)

const (
	// CapabilityGRPC is the capability to call a gRPC server.
	CapabilityGRPC = "gRPC"

	// CapabilityHTTP is the capability to call an HTTP server.
	CapabilityHTTP = "HTTP"
)

// Provider is used to provide a given capability when requested remotely. They
// must return a connection that is bridged or an error.
type Provider func(capability string, meta map[string]string, conn net.Conn) error
