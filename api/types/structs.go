package types

import (
	"time"

	"github.com/hashicorp/hcp-sdk-go/clients/cloud-shared/v1/models"
)

// ConnectRequest holds parameters for the broker RPC Connect call to the provider.
type ConnectRequest struct {
	Capability string
	Meta       map[string]string

	Severity string
	Message  string
}

// ConnectResponse is the response to a Connect RPC call.
type ConnectResponse struct {
	Success bool
}

// DisconnectRequest holds parameters for the broker RPC Disconnect call to the provider.
type DisconnectRequest struct {
	NoRetry bool          // Should the client retry
	Backoff time.Duration // Minimum backoff
	Reason  string
}

// DisconnectResponse is the response to a Disconnect RPC call.
type DisconnectResponse struct {
}

// HandshakeRequest holds parameters for the broker RPC Handshake call to the provider.
type HandshakeRequest struct {
	// Service is the name of a data-plane Service connecting to the broker as a provider. Examples include consul, vault, waypoint, etc.
	Service string

	// ServiceVersion is the version of the data-plane Service running.
	ServiceVersion string

	// ServiceID is the unique identifier of the Service. It can be the Resource's internal ID that will be same for all
	// nodes of a Resource. The Meta field is used to distinguish among the nodes.
	// Deprecated: This is eventually going to be replaced by Resource. Until authorization using that is implemented
	// this field should be continued to be used.
	ServiceID string

	// AccessToken is HCP JWT token used to authenticate and authorize the provider.
	AccessToken string

	// Resource is HCP Resource that is registering as a provider. This is recommended over ServiceID. The Resource's
	// internal ID will be used to map providers to consumers which will be looked up from Resource-manager.
	Resource *models.HashicorpCloudLocationLink

	// Capabilities is the list of services that this provider can provide. This could e.g. be "gRPC" or "HTTP".
	Capabilities map[string]int

	// Meta is the generic metadata for this particular session. It can include information like the EC2 instance name to identify
	// specific nodes.
	Meta map[string]string
}

// HandshakeResponse is the response to a Handshake RPC call.
type HandshakeResponse struct {
	Authenticated bool
	SessionID     string
	Reason        string
}
