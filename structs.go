package scada

import "time"

type ConnectRequest struct {
	Capability string
	Meta       map[string]string

	Severity string
	Message  string
}

type ConnectResponse struct {
	Success bool
}

type DisconnectRequest struct {
	NoRetry bool          // Should the client retry
	Backoff time.Duration // Minimum backoff
	Reason  string
}

type DisconnectResponse struct {
}

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
	Resource Resource

	// Capabilities is the list of services that this provider can provide. This could e.g. be "gRPC" or "HTTP".
	Capabilities map[string]int

	// Meta is the generic metadata for this particular session. It can include information like the EC2 instance name to identify
	// specific nodes.
	Meta map[string]string
}

// Resource contains information to uniquely identify a HCP Resource.
// TODO: Decide whether to make proto link public vs keeping this struct.
type Resource struct {

	// OrganizationID is UUID of organization containing this Resource.
	OrganizationID string

	// ProjectID is UUID of project inside the organization containing this Resource.
	ProjectID string

	// Type is the type of Resource. Can be "hashicorp.cloud.cluster", etc.
	Type string

	// ID is the SlugID of the Resource that is unique within a project.
	ID string
}

type HandshakeResponse struct {
	Authenticated bool
	SessionID     string
	Reason        string
}
