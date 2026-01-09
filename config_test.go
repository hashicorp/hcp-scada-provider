// Copyright IBM Corp. 2022, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"testing"

	"github.com/hashicorp/go-hclog"
	cloud "github.com/hashicorp/hcp-sdk-go/clients/cloud-shared/v1/models"
	sdk "github.com/hashicorp/hcp-sdk-go/config"
	requirepkg "github.com/stretchr/testify/require"

	"github.com/hashicorp/hcp-scada-provider/internal/test"
)

type stubHCPConfig struct {
	sdk.HCPConfig
}

func validConfig() *Config {
	return &Config{
		Service: "my-service",
		Resource: cloud.HashicorpCloudLocationLink{
			ID:   "resource-id",
			Type: "hashicorp.test.resource",
			Location: &cloud.HashicorpCloudLocationLocation{
				OrganizationID: "3e77d3bd-e5ac-4bb5-8f1b-c1e10e6dd8fa",
				ProjectID:      "aeafc081-f112-4d0b-b962-e6dde88207f3",
			},
		},
		HCPConfig: stubHCPConfig{},
		Logger:    hclog.NewNullLogger(),
	}
}

func TestConfig_Valid(t *testing.T) {
	require := requirepkg.New(t)
	require.NoError(validConfig().Validate())
}

func TestConfig_Invalid(t *testing.T) {
	testCases := []struct {
		name          string
		mutate        func(*Config)
		expectedError string
	}{
		{
			name: "missing service",
			mutate: func(config *Config) {
				config.Service = ""
			},
			expectedError: "failed to initialize SCADA provider: missing Service",
		},
		{
			name: "invalid resource",
			mutate: func(config *Config) {
				config.Resource.ID = ""
			},
			expectedError: "failed to initialize SCADA provider: missing resource ID",
		},
		{
			name: "missing HCP Config",
			mutate: func(config *Config) {
				config.HCPConfig = nil
			},
			expectedError: "failed to initialize SCADA provider: HCPConfig must be provided",
		},
		{
			name: "missing Logger",
			mutate: func(config *Config) {
				config.Logger = nil
			},
			expectedError: "failed to initialize SCADA provider: Logger must be provided",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			require := requirepkg.New(t)

			config := validConfig()
			testCase.mutate(config)

			require.EqualError(config.Validate(), testCase.expectedError)
		})
	}
}

func testProviderConfig() *Config {
	return &Config{
		Service: "test",
		HCPConfig: test.NewStaticHCPConfig(
			"127.0.0.1:65500", // Blackhole
			false,
		),
		Resource: cloud.HashicorpCloudLocationLink{
			ID:   resourceID,
			Type: resourceType,
			Location: &cloud.HashicorpCloudLocationLocation{
				ProjectID:      projectID,
				OrganizationID: organizationID,
			},
		},
		Logger: hclog.L(),
	}
}
