package provider

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/rpc"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	cloud "github.com/hashicorp/hcp-sdk-go/clients/cloud-shared/v1/models"
	msgpackrpc "github.com/hashicorp/net-rpc-msgpackrpc/v2"
	"github.com/hashicorp/yamux"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"

	"github.com/hashicorp/hcp-scada-provider/internal/client"
	"github.com/hashicorp/hcp-scada-provider/internal/listener"
	"github.com/hashicorp/hcp-scada-provider/internal/test"
	"github.com/hashicorp/hcp-scada-provider/types"
)

const (
	resourceID     = "my-cluster"
	resourceType   = "hashicorp.test.linked-cluster"
	projectID      = "a802238a-8d73-4565-a48b-16cbaf6bd129"
	organizationID = "ac7fb6d9-a3cc-4add-8158-045ff7d32475"
)

// isStopped checks if the provider has been stopped.
func (p *Provider) isStopped() bool {
	p.runningLock.Lock()
	defer p.runningLock.Unlock()
	return !p.running
}

func TestProviderImplementsSCADAProvider(t *testing.T) {
	r := require.New(t)

	var givenProvider SCADAProvider
	givenProvider, _ = New(testProviderConfig())
	r.NotNil(givenProvider)
}

func TestSCADAProvider(t *testing.T) {
	t.Run("SCADA provider initialization fails if no Logger is provided", func(t *testing.T) {
		_, err := New(&Config{
			Service:   "test",
			Logger:    nil,
			HCPConfig: test.NewStaticHCPCloudDevConfig(),
			Resource: cloud.HashicorpCloudLocationLink{
				ID:       "ID",
				Type:     "Type",
				Location: &cloud.HashicorpCloudLocationLocation{},
			},
		})

		r := require.New(t)
		r.EqualError(err, "failed to initialize SCADA provider: Logger must be provided")
	})

	t.Run("SCADA provider initialization fails if no HCP Config is provided", func(t *testing.T) {
		_, err := New(&Config{
			Service:   "test",
			Logger:    hclog.Default(),
			HCPConfig: nil,
			Resource: cloud.HashicorpCloudLocationLink{
				ID:       "ID",
				Type:     "Type",
				Location: &cloud.HashicorpCloudLocationLocation{},
			},
		})

		r := require.New(t)
		r.EqualError(err, "failed to initialize SCADA provider: HCPConfig must be provided")
	})

	t.Run("SCADA provider initialization fails if no Resource link location is provided", func(t *testing.T) {
		_, err := New(&Config{
			Service:   "test",
			Logger:    hclog.Default(),
			HCPConfig: test.NewStaticHCPCloudDevConfig(),
			Resource: cloud.HashicorpCloudLocationLink{
				ID:       "ID",
				Type:     "",
				Location: nil,
			},
		})

		r := require.New(t)
		r.EqualError(err, "failed to initialize SCADA provider: missing resource location")
	})

	t.Run("SCADA provider initialization fails if no Resource link type is provided", func(t *testing.T) {
		_, err := New(&Config{
			Service:   "test",
			Logger:    hclog.Default(),
			HCPConfig: test.NewStaticHCPCloudDevConfig(),
			Resource: cloud.HashicorpCloudLocationLink{
				ID:       "ID",
				Type:     "",
				Location: &cloud.HashicorpCloudLocationLocation{},
			},
		})

		r := require.New(t)
		r.EqualError(err, "failed to initialize SCADA provider: missing resource type")
	})

	t.Run("SCADA provider initialization fails if no Resource ID is provided", func(t *testing.T) {
		_, err := New(&Config{
			Service:   "test",
			Logger:    hclog.Default(),
			HCPConfig: test.NewStaticHCPCloudDevConfig(),
			Resource: cloud.HashicorpCloudLocationLink{
				ID:       "",
				Type:     "Type",
				Location: &cloud.HashicorpCloudLocationLocation{},
			},
		})

		r := require.New(t)
		r.EqualError(err, "failed to initialize SCADA provider: missing resource ID")
	})

	t.Run("SCADA provider initialization succeeds when all provided parameters are valid", func(t *testing.T) {
		givenSCADAProvider, err := New(&Config{
			Service:   "test",
			Logger:    hclog.Default(),
			HCPConfig: test.NewStaticHCPCloudDevConfig(),
			Resource: cloud.HashicorpCloudLocationLink{
				ID:       "ID",
				Type:     "Type",
				Location: &cloud.HashicorpCloudLocationLocation{},
			},
		})

		r := require.New(t)
		r.NoError(err)
		r.NotNil(givenSCADAProvider)
	})
}

func TestResetTicker(t *testing.T) {
	var duration = 1 * time.Millisecond
	var ticker = time.NewTicker(duration)
	defer ticker.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), duration*4)
	defer cancel()

	r := require.New(t)

	// the base case
	select {
	case <-ctx.Done():
		t.Errorf("context canceled before the ticker ticked")
	case <-ticker.C:

	}

	var now = time.Now()
	duration = 1 * time.Minute

	// run resetTicker and expect it will return duration * expiryFactor.
	var durationAfterFactor = calculateExpiryFactor(duration)
	var newDuration = tickerReset(now, now.Add(duration), ticker)
	// newDuration should have been reset to the value of duration
	r.Equal(durationAfterFactor, newDuration, "newDuration expected to be %v and is %v", durationAfterFactor, newDuration)

	// run resetTicker with a expiry time of zero and expect it to return
	// the default expiry time
	durationAfterFactor = calculateExpiryFactor(expiryDefault)
	newDuration = tickerReset(now, time.Time{}, ticker)
	r.Equal(durationAfterFactor, newDuration, "newDuration expected to be %v and is %v", durationAfterFactor, newDuration)

	// run resetTicker with the now time after expiry and expect it to return
	// the default expiry time
	durationAfterFactor = calculateExpiryFactor(expiryDefault)
	newDuration = tickerReset(now.Add(duration), now, ticker)
	r.Equal(durationAfterFactor, newDuration, "newDuration expected to be %v and is %v", durationAfterFactor, newDuration)
}

func TestProvider_StartStop(t *testing.T) {
	require := require.New(t)

	// Create the provider, it should be stopped
	p, err := newProvider(testProviderConfig())
	require.NoError(err)
	require.True(p.isStopped())

	// Start the provider
	err = p.Start()
	require.NoError(err)
	require.False(p.isStopped())

	// Starting should be idempotent
	err = p.Start()
	require.NoError(err)
	require.False(p.isStopped())

	// Stop the provider
	err = p.Stop()
	require.NoError(err)
	require.True(p.isStopped())

	// Stopping should be idempotent
	err = p.Stop()
	require.NoError(err)
	require.True(p.isStopped())

	// Re-Start the provider
	err = p.Start()
	require.NoError(err)
	require.False(p.isStopped())

	// Stop the provider again
	err = p.Stop()
	require.NoError(err)
	require.True(p.isStopped())
}

func TestProvider_backoff(t *testing.T) {
	require := require.New(t)
	p, err := newProvider(testProviderConfig())
	require.NoError(err)

	err = p.Start()
	require.NoError(err)
	defer func(p *Provider) {
		_ = p.Stop()
	}(p)

	backoffDuration, noRetry := p.backoffDuration()
	require.EqualValues(backoffDuration, defaultBackoff)

	// Set a new minimum
	p.backoffLock.Lock()
	p.backoff = 60 * time.Second
	p.backoffLock.Unlock()
	backoffDuration, noRetry = p.backoffDuration()
	require.EqualValues(backoffDuration, 60*time.Second)

	// Set no retry
	p.backoffLock.Lock()
	p.noRetry = true
	p.backoffLock.Unlock()
	backoffDuration, noRetry = p.backoffDuration()
	require.EqualValues(noRetry, true)
}

func testListener(t *testing.T) (string, net.Listener) {
	require := require.New(t)
	list, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(err)
	addr := fmt.Sprintf("127.0.0.1:%d", list.Addr().(*net.TCPAddr).Port)

	return addr, list
}

type TestHandshake struct {
	t      *testing.T
	expect *types.HandshakeRequest
}

func (t *TestHandshake) Handshake(arg *types.HandshakeRequest, resp *types.HandshakeResponse) error {
	require.EqualValues(t.t, t.expect, arg)
	resp.Authenticated = true
	resp.SessionID = "foobarbaz"
	return nil
}

func testHandshake(t *testing.T, list net.Listener, expect *types.HandshakeRequest) net.Conn {
	require := require.New(t)
	conn, err := list.Accept()
	require.NoError(err)

	preamble := make([]byte, len(client.ClientPreamble))
	n, err := conn.Read(preamble)
	require.NoError(err)
	require.Len(preamble, n)

	server, _ := yamux.Server(conn, yamux.DefaultConfig())
	stream, err := server.Accept()
	require.NoError(err)
	defer stream.Close()
	rpcCodec := msgpackrpc.NewCodec(true, true, stream)

	rpcSrv := rpc.NewServer()
	_ = rpcSrv.RegisterName("Session", &TestHandshake{t, expect})
	require.NoError(rpcSrv.ServeRequest(rpcCodec))

	return conn
}

func TestProvider_Setup(t *testing.T) {
	require := require.New(t)
	addr, list := testListener(t)
	defer list.Close()

	config := testProviderConfig()
	config.HCPConfig = test.NewStaticHCPConfig(addr, false)

	p, err := newProvider(config)
	require.NoError(err)

	require.Equal(SessionStatusDisconnected, p.SessionStatus())

	err = p.Start()
	require.NoError(err)
	defer func(p *Provider) {
		_ = p.Stop()
	}(p)

	require.Equal(SessionStatusConnecting, p.SessionStatus())

	_, err = p.Listen("foo")
	require.NoError(err)

	exp := &types.HandshakeRequest{
		Service: "test",
		Resource: &cloud.HashicorpCloudLocationLink{
			ID: resourceID,
			Location: &cloud.HashicorpCloudLocationLocation{
				ProjectID:      projectID,
				OrganizationID: organizationID,
			},
			Type: resourceType,
		},
		ServiceVersion: "0.0.1",
		Capabilities:   map[string]int{"foo": 1},
		Meta:           map[string]string{},
	}
	conn := testHandshake(t, list, exp)
	defer conn.Close()

	start := time.Now()
	for time.Since(start) < 1*time.Second {
		if p.SessionStatus() != SessionStatusConnecting {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	require.Equal(SessionStatusConnected, p.SessionStatus())
}

func TestProvider_ListenerStop(t *testing.T) {
	require := require.New(t)

	var givenProvider SCADAProvider
	givenCapability := "foo"
	givenProvider, _ = New(testProviderConfig())
	givenListener, _ := givenProvider.Listen(givenCapability)

	err := givenListener.Close()
	require.NoError(err)

	require.Eventually(func() bool {
		if _, capabilityExists := givenProvider.(*Provider).handlers[givenCapability]; !capabilityExists {
			return true
		}
		return false
	}, 1*time.Second, 10*time.Millisecond)
	require.NotContains(givenProvider.(*Provider).handlers, givenCapability)
}

func fooCapability(t *testing.T) listener.Provider {
	return func(capa string, meta map[string]string, conn net.Conn) error {
		require := require.New(t)
		require.Equal("foo", capa)
		require.Len(meta, 1)
		require.Contains(meta, "zip")
		require.Equal(meta["zip"], "zap")
		_, err := conn.Write([]byte("foobarbaz"))
		require.NoError(err)
		require.NoError(conn.Close())
		return nil
	}
}

func TestProvider_Connect(t *testing.T) {
	require := require.New(t)
	config := testProviderConfig()

	p, err := newProvider(config)
	p.handlers["foo"] = handler{
		provider: fooCapability(t),
	}
	require.NoError(err)

	err = p.Start()
	require.NoError(err)
	defer func(p *Provider) {
		_ = p.Stop()
	}(p)

	// Setup RPC client
	a, b := testConn(t)
	client, _ := yamux.Client(a, yamux.DefaultConfig())
	server, _ := yamux.Server(b, yamux.DefaultConfig())
	go p.handleSession(context.Background(), client)

	stream, _ := server.Open()
	cc := msgpackrpc.NewCodec(false, false, stream)

	// Make the connect rpc
	args := &ConnectRequest{
		Capability: "foo",
		Meta: map[string]string{
			"zip": "zap",
		},
	}
	resp := &ConnectResponse{}
	err = msgpackrpc.CallWithCodec(cc, "Provider.Connect", args, resp)
	require.NoError(err)

	// Should be successful!
	require.True(resp.Success)

	// At this point, we should be connected
	out := make([]byte, 9)
	_, err = stream.Read(out)
	require.NoError(err)
	require.Equal("foobarbaz", string(out))
}

func TestProvider_Disconnect(t *testing.T) {
	const testBackoff = 300 * time.Second
	require := require.New(t)
	config := testProviderConfig()
	p, err := newProvider(config)
	require.NoError(err)

	err = p.Start()
	require.NoError(err)
	defer func(p *Provider) {
		_ = p.Stop()
	}(p)

	// Setup RPC client
	a, b := testConn(t)
	client, _ := yamux.Client(a, yamux.DefaultConfig())
	server, _ := yamux.Server(b, yamux.DefaultConfig())
	go p.handleSession(context.Background(), client)

	stream, _ := server.Open()
	cc := msgpackrpc.NewCodec(false, false, stream)

	// Make the connect rpc
	args := &DisconnectRequest{
		NoRetry: true,
		Backoff: testBackoff,
	}
	resp := &DisconnectResponse{}
	err = msgpackrpc.CallWithCodec(cc, "Provider.Disconnect", args, resp)
	require.NoError(err)

	p.backoffLock.Lock()
	defer p.backoffLock.Unlock()

	require.Equal(testBackoff, p.backoff)
	require.True(p.noRetry)

	// give it some time to update state
	time.Sleep(2 * disconnectDelay)
	require.Equal(SessionStatusWaiting, p.SessionStatus())
}

func testConn(t *testing.T) (net.Conn, net.Conn) {
	require := require.New(t)
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(err)

	var serverConn net.Conn
	doneCh := make(chan struct{})
	go func() {
		defer close(doneCh)
		defer l.Close()
		var err error
		serverConn, err = l.Accept()
		require.NoError(err)
	}()

	clientConn, err := net.Dial("tcp", l.Addr().String())
	require.NoError(err)
	<-doneCh

	return clientConn, serverConn
}

func TestProviderSetupRetrieveError(t *testing.T) {
	require := require.New(t)
	addr, list := testListener(t)
	defer list.Close()

	config := testProviderConfig()
	// Get a HCP config that has a mock oauth2 Token function that always returns *oauth2.RetrieveError
	config.HCPConfig = test.NewStaticHCPConfigErrorTokenSource(addr, false, &oauth2.RetrieveError{
		Response: &http.Response{
			StatusCode: http.StatusUnauthorized,
		},
	})

	p, err := New(config)
	require.NoError(err)

	require.Equal(SessionStatusDisconnected, p.SessionStatus())

	err = p.Start()
	require.NoError(err)
	defer p.Stop()

	require.Equal(SessionStatusConnecting, p.SessionStatus())

	_, err = p.Listen("foo")
	require.NoError(err)

	conn, err := list.Accept()
	defer conn.Close()
	require.NoError(err)

	preamble := make([]byte, len(client.ClientPreamble))
	n, err := conn.Read(preamble)
	require.NoError(err)
	require.Len(preamble, n)

	server, _ := yamux.Server(conn, yamux.DefaultConfig())
	server.Accept()

	// wait until the provider disconnects because of the oauth2 error
	require.Eventually(func() bool {
		if p.SessionStatus() == SessionStatusWaiting {
			return true
		}
		return false
	}, 1*time.Second, 10*time.Millisecond)

	// at this point, last error should be ErrInvalidCredentials
	_, err = p.LastError()
	require.Error(err)
	require.Equal(ErrInvalidCredentials, err)
}
