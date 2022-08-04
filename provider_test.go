package scada

import (
	"fmt"
	"net"
	"net/rpc"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	cloud "github.com/hashicorp/hcp-sdk-go/clients/cloud-shared/v1/models"
	msgpackrpc "github.com/hashicorp/net-rpc-msgpackrpc"
	"github.com/hashicorp/yamux"
	"github.com/stretchr/testify/require"

	"github.com/hashicorp/hcp-scada-provider/internal/capability/listener"
	"github.com/hashicorp/hcp-scada-provider/internal/client"
	"github.com/hashicorp/hcp-scada-provider/internal/test"
)

const (
	resourceID     = "my-cluster"
	resourceType   = "hashicorp.test.linked-cluster"
	projectID      = "a802238a-8d73-4565-a48b-16cbaf6bd129"
	organizationID = "ac7fb6d9-a3cc-4add-8158-045ff7d32475"
)

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
		r.EqualError(err, "failed to initialize SCADA provider: HCP Config must be provided")
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

func TestProvider_StartStop(t *testing.T) {
	require := require.New(t)

	// Create the provider, it should be stopped
	p, err := construct(testProviderConfig())
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
	p, err := construct(testProviderConfig())
	require.NoError(err)

	err = p.Start()
	require.NoError(err)
	defer func(p *Provider) {
		_ = p.Stop()
	}(p)

	require.EqualValues(p.backoffDuration(), defaultBackoff)

	// Set a new minimum
	p.backoffLock.Lock()
	p.backoff = 60 * time.Second
	p.backoffLock.Unlock()
	require.EqualValues(p.backoffDuration(), 60*time.Second)

	// Set no retry
	p.backoffLock.Lock()
	p.noRetry = true
	p.backoffLock.Unlock()
	require.EqualValues(p.backoffDuration(), 0)
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
	expect *HandshakeRequest
}

func (t *TestHandshake) Handshake(arg *HandshakeRequest, resp *HandshakeResponse) error {
	require.EqualValues(t.t, t.expect, arg)
	resp.Authenticated = true
	resp.SessionID = "foobarbaz"
	return nil
}

func testHandshake(t *testing.T, list net.Listener, expect *HandshakeRequest) {
	require := require.New(t)
	conn, err := list.Accept()
	require.NoError(err)
	defer conn.Close()

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
}

func TestProvider_Setup(t *testing.T) {
	require := require.New(t)
	addr, list := testListener(t)
	defer list.Close()

	config := testProviderConfig()
	config.HCPConfig = test.NewStaticHCPConfig(addr, false)

	p, err := construct(config)
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

	exp := &HandshakeRequest{
		Service: "test",
		Resource: Resource{
			ID:             resourceID,
			Type:           resourceType,
			ProjectID:      projectID,
			OrganizationID: organizationID,
		},
		ServiceVersion: "0.0.1",
		Capabilities:   map[string]int{"foo": 1},
		Meta:           map[string]string{},
	}
	testHandshake(t, list, exp)

	start := time.Now()
	for time.Since(start) < time.Second {
		if p.SessionStatus() != SessionStatusConnecting {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	require.Equal(SessionStatusConnected, p.SessionStatus())
}

func TestProvider_ListenerStop(t *testing.T) {
	require := require.New(t)

	givenCapability := "foo"
	givenProvider, _ := New(testProviderConfig())
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

	p, err := construct(config)
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
	go p.handleSession(client, make(chan struct{}))

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
	require := require.New(t)
	config := testProviderConfig()
	p, err := construct(config)
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
	go p.handleSession(client, make(chan struct{}))

	stream, _ := server.Open()
	cc := msgpackrpc.NewCodec(false, false, stream)

	// Make the connect rpc
	args := &DisconnectRequest{
		NoRetry: true,
		Backoff: 300 * time.Second,
	}
	resp := &DisconnectResponse{}
	err = msgpackrpc.CallWithCodec(cc, "Provider.Disconnect", args, resp)
	require.NoError(err)

	p.backoffLock.Lock()
	defer p.backoffLock.Unlock()

	require.Equal(300*time.Second, p.backoff)
	require.True(p.noRetry)

	require.Equal(SessionStatusDisconnected, p.SessionStatus())
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
