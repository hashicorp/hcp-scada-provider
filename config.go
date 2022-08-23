package provider

import (
	"fmt"
	"time"

	"github.com/hashicorp/go-hclog"
	cloud "github.com/hashicorp/hcp-sdk-go/clients/cloud-shared/v1/models"

	sdk "github.com/hashicorp/hcp-sdk-go/config"

	"github.com/hashicorp/hcp-scada-provider/internal/resource"
)

// Config is used to parameterize a provider
type Config struct {
	// Service is the name to identify the client.
	Service string

	// Resource contains information about the Resource the provider will
	// register as.
	Resource cloud.HashicorpCloudLocationLink

	// HCPConfig is the HCP specific configuration, it provides information
	// necessary to talk to HCP APIs.
	HCPConfig sdk.HCPConfig

	// Logger is the Logger to use for logs.
	Logger hclog.Logger

	// TestBackoff is used to force the provider to retry more aggressively.
	TestBackoff time.Duration
}

func (c *Config) Validate() error {
	if c == nil {
		return fmt.Errorf("missing config")
	}

	if c.Service == "" {
		return fmt.Errorf("missing Service")
	}

	err := resource.Validate(c.Resource)
	if err != nil {
		return fmt.Errorf("resource is invalid: %w", err)
	}
	if c.HCPConfig == nil {
		return fmt.Errorf("missing HCPConfig")
	}
	if c.Logger == nil {
		return fmt.Errorf("missing Logger")
	}

	return nil
}
