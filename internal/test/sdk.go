// Copyright IBM Corp. 2022, 2025
// SPDX-License-Identifier: MPL-2.0

package test

import (
	"context"
	"crypto/tls"
	"net/url"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"

	sdk "github.com/hashicorp/hcp-sdk-go/config"
)

// NewStaticHCPConfig creates an instance of HCPConfig using provided endpoint and TLS setting. If TLS is enabled it
// will not verify the server's certificate.
func NewStaticHCPConfig(scadaEndpoint string, useTLS bool) *staticHCPConfig {
	var tlsConfig *tls.Config

	if useTLS {
		tlsConfig = &tls.Config{
			InsecureSkipVerify: true,
		}
	}

	return &staticHCPConfig{
		scadaEndpoint:  scadaEndpoint,
		scadaTLSConfig: tlsConfig,
		tokenSource:    staticTokenSource{},
	}
}

// NewStaticHCPConfigErrorTokenSource is the same as NewStaticHCPConfig but has a token source that always returns err
func NewStaticHCPConfigErrorTokenSource(scadaEndpoint string, useTLS bool, err error) *staticHCPConfig {
	config := NewStaticHCPConfig(scadaEndpoint, useTLS)
	config.tokenSource = ErrorTokenSource{error: err}
	return config
}

// NewStaticHCPCloudDevConfig will return a static configuration that is configured with cloud-dev's Traefik SCADA port.
func NewStaticHCPCloudDevConfig() *staticHCPConfig {
	config := NewStaticHCPConfig("localhost:28083", true)

	return config
}

func NewStaticHCPCloudDevConfigWithClientCredentials(clientID, clientSecret string) *staticHCPConfig {
	config := NewStaticHCPConfig("localhost:28083", true)

	// Perform oauth flow
	oauthConfig := &clientcredentials.Config{
		ClientID:       clientID,
		ClientSecret:   clientSecret,
		TokenURL:       "https://hashicorp-cloud-dev-local.auth0.com/oauth/token",
		EndpointParams: url.Values{"audience": {"https://api.hashicorp.cloud"}},
	}

	config.tokenSource = oauthConfig.TokenSource(context.Background())

	return config
}

// staticHCPConfig is an implementation of HCPConfig with immutable fields. The zero value of staticHCPConfig
// is not safe to use, use NewStaticHCPConfig or NewStaticHCPCloudDevConfig to create an instance of a config instead.
type staticHCPConfig struct {
	sdk.HCPConfig

	scadaEndpoint  string
	scadaTLSConfig *tls.Config
	tokenSource    oauth2.TokenSource
}

func (c *staticHCPConfig) Token() (*oauth2.Token, error) {
	return c.tokenSource.Token()
}

func (c *staticHCPConfig) SCADAAddress() string {
	return c.scadaEndpoint
}

func (c *staticHCPConfig) SCADATLSConfig() *tls.Config {
	return c.scadaTLSConfig
}

type staticTokenSource struct {
	accesstoken string
}

func (s staticTokenSource) Token() (*oauth2.Token, error) {
	return &oauth2.Token{AccessToken: s.accesstoken}, nil
}

// ErrorTokenSource is an implementation of an oauth2 token source
// that always returns an error when its Token() function is called.
type ErrorTokenSource struct {
	error
}

func (m ErrorTokenSource) Token() (*oauth2.Token, error) {
	return nil, m.error
}

var _ sdk.HCPConfig = &staticHCPConfig{}
