package scada

import (
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/rpc"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/go-hclog"
	msgpackrpc "github.com/hashicorp/net-rpc-msgpackrpc"

	"github.com/hashicorp/hcp-scada-provider/internal/capability"
	"github.com/hashicorp/hcp-scada-provider/internal/capability/listener"
	"github.com/hashicorp/hcp-scada-provider/internal/client"
	"github.com/hashicorp/hcp-scada-provider/internal/client/dialer/tcp"
	"github.com/hashicorp/hcp-scada-provider/internal/resource"
)

const (
	// defaultBackoff is the amount of time we back off if we encounter and
	// error, and no specific backoff is available.
	defaultBackoff = 10 * time.Second

	// disconnectDelay is how long we delay the disconnect to allow the RPC to
	// complete.
	disconnectDelay = time.Second
)

type handler struct {
	provider capability.Provider
	listener net.Listener
}

// New creates a new SCADA provider instance using the given configuration.
func New(config *Config) (SCADAProvider, error) {
	if config.Logger == nil {
		return nil, fmt.Errorf("failed to initialize SCADA provider: Logger must be provided")
	}
	if config.HCPConfig == nil {
		return nil, fmt.Errorf("failed to initialize SCADA provider: HCP Config must be provided")
	}
	err := resource.Validate(config.Resource)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize SCADA provider: %w", err)
	}

	return construct(config)

}

// Provider is a high-level interface to SCADA by which instances declare
// themselves as a Service providing capabilities. Provider manages the
// client/server interactions required, making it simpler to integrate.
type Provider struct {
	config *Config
	logger hclog.Logger

	client     *client.Client
	clientLock sync.Mutex

	handlers     map[string]handler
	handlersLock sync.RWMutex

	noRetry     bool          // set when the server instructs us to not retry
	backoff     time.Duration // set when the server provides a longer backoff
	backoffLock sync.Mutex

	sessionStatus SessionStatus
	sessionLock   sync.RWMutex

	meta     map[string]string
	metaLock sync.RWMutex

	running     bool
	stopCh      chan struct{}
	runningLock sync.Mutex
}

// construct is used to create a new provider
func construct(config *Config) (*Provider, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	// Create a closed channel to use for stopCh, as the provider is initially
	// stopped.
	closedCh := make(chan struct{})
	close(closedCh)

	p := &Provider{
		config:        config,
		logger:        config.Logger.Named("scada-provider"),
		meta:          map[string]string{},
		handlers:      map[string]handler{},
		sessionStatus: SessionStatusDisconnected,
		stopCh:        closedCh,
	}

	return p, nil
}

// Start will register the provider on the SCADA broker and expose the
// registered capabilities.
func (p *Provider) Start() error {
	p.runningLock.Lock()
	defer p.runningLock.Unlock()

	// Check if the provider is already running
	if p.running {
		return nil
	}

	// Set session status to connecting
	p.sessionLock.Lock()
	p.sessionStatus = SessionStatusConnecting
	p.sessionLock.Unlock()

	// Set the provider to its running state
	p.stopCh = make(chan struct{})
	p.running = true

	// Run the provider
	go p.run()

	return nil
}

// Stop will try to gracefully close the currently active SCADA session. This will
// not close the capability listeners.
func (p *Provider) Stop() error {
	p.runningLock.Lock()
	defer p.runningLock.Unlock()

	// Check if the provider is already stopped
	if !p.running {
		return nil
	}

	p.sessionLock.Lock()
	defer p.sessionLock.Unlock()

	p.logger.Info("stopping")
	p.running = false
	p.sessionStatus = SessionStatusDisconnected
	close(p.stopCh)
	return nil
}

// isStopped checks if the provider has been stopped.
// TODO: replace with SessionStatus
func (p *Provider) isStopped() bool {
	select {
	case <-p.stopCh:
		return true
	default:
		return false
	}
}

// SetMetaValue sets the meta-data value. Empty values will not be
// transmitted to the broker.
//
// In the future new meta-data value will be transmitted with the
// next handshake. For now new meta-data can only be set when
// the provider is stopped.
func (p *Provider) SetMetaValue(key, value string) {
	p.metaLock.Lock()
	defer p.metaLock.Unlock()

	// Remove the value if it is empty
	if value == "" {
		delete(p.meta, key)
		return
	}

	p.meta[key] = value
}

// GetMeta returns the provider's current meta-data.
func (p *Provider) GetMeta() map[string]string {
	p.metaLock.RLock()
	defer p.metaLock.RUnlock()

	return p.meta
}

// Listen will expose the provided capability and make new connections
// available through the returned listener. Closing the listener will stop
// exposing the provided capability.
//
// The method will return an existing listener if the capability already existed.
// Listeners can be retrieved even when the provider is stopped (e.g. before it is
// started). For now new capabilities can only be added while the provider is stopped
// (once re-handshakes are available new capabilities can be added regardless of the
// state of the provider).
//
// The listener will only be closed, if it is closed explicitly by calling Close().
// The listener will not be closed due to errors or when the provider is stopped.
// The listener can hence be used after a restart of the provider.
func (p *Provider) Listen(capability string) (net.Listener, error) {
	// Check if the capability already exists
	p.handlersLock.RLock()
	capHandler, ok := p.handlers[capability]
	p.handlersLock.RUnlock()

	if ok {
		return capHandler.listener, nil
	}

	// Get write lock
	p.handlersLock.Lock()
	defer p.handlersLock.Unlock()

	// Ensure that no concurrent call has set the listener in the meantime
	if capHandler, ok = p.handlers[capability]; ok {
		return capHandler.listener, nil
	}

	// Generate a provider and listener for the new capability
	capProvider, capListener, err := listener.New(capability)
	if err != nil {
		return nil, err
	}

	// Assign an OnClose callback on a listener, to make sure the handler is removed for the capacity.
	capListenerProxy := listener.WithCloseCallback(capListener, func() {
		p.handlersLock.Lock()
		defer p.handlersLock.Unlock()

		delete(p.handlers, capability)
	})

	p.handlers[capability] = handler{
		provider: capProvider,
		listener: capListenerProxy,
	}

	return capListenerProxy, nil
}

// SessionStatus will return the status of the SCADA connection.
func (p *Provider) SessionStatus() SessionStatus {
	p.sessionLock.RLock()
	defer p.sessionLock.RUnlock()

	return p.sessionStatus
}

// backoffDuration is used to compute the next backoff duration
func (p *Provider) backoffDuration() time.Duration {
	// Use the default backoff
	backoff := defaultBackoff

	// Check for a server specified backoff
	p.backoffLock.Lock()
	if p.backoff != 0 {
		backoff = p.backoff
	}
	if p.noRetry {
		backoff = 0
	}
	p.backoffLock.Unlock()

	// Use the test backoff
	if p.config.TestBackoff != 0 {
		backoff = p.config.TestBackoff
	}

	return backoff
}

// wait is used to delay dialing on an error
func (p *Provider) wait() {
	// Compute the backoff time
	backoff := p.backoffDuration()

	// Setup a wait timer
	var wait <-chan time.Time
	if backoff > 0 {
		jitter := time.Duration(rand.Uint32()) % backoff
		wait = time.After(backoff + jitter)
	}

	// Wait until timer or shutdown
	select {
	case <-wait:
	case <-p.stopCh:
	}
}

// run is a long running routine to manage the provider
func (p *Provider) run() {
	for !p.isStopped() {
		// Setup a new connection
		client, err := p.clientSetup()
		if err != nil {
			p.wait()
			continue
		}

		// Handle the session
		doneCh := make(chan struct{})
		go p.handleSession(client, doneCh)

		// Wait for session termination or shutdown
		select {
		case <-doneCh:
			p.wait()
		case <-p.stopCh:
			p.clientLock.Lock()
			client.Close()
			p.clientLock.Unlock()
			return
		}
	}
}

// handleSession is used to handle an established session
func (p *Provider) handleSession(list net.Listener, doneCh chan struct{}) {
	defer close(doneCh)
	defer list.Close()
	// Accept new connections
	for !p.isStopped() {
		conn, err := list.Accept()
		if err != nil {
			// Do not log an error if we are shutting down
			if !p.isStopped() {
				p.logger.Error("failed to accept connection", "error", err)
			}
			return
		}
		p.logger.Debug("accepted connection")
		go p.handleConnection(conn)
	}
}

// handleConnection handles an incoming connection
func (p *Provider) handleConnection(conn net.Conn) {
	// Create an RPC server to handle inbound
	pe := &providerEndpoint{p: p}
	rpcServer := rpc.NewServer()
	_ = rpcServer.RegisterName("Provider", pe)
	rpcCodec := msgpackrpc.NewCodec(false, false, conn)

	defer func() {
		if !pe.hijacked() {
			conn.Close()
		}
	}()

	for !p.isStopped() {
		if err := rpcServer.ServeRequest(rpcCodec); err != nil {
			if err != io.EOF && !strings.Contains(err.Error(), "closed") {
				p.logger.Error("RPC error", "error", err)
			}
			return
		}

		// Handle potential hijack in Provider.Connect
		if pe.hijacked() {
			cb := pe.getHijack()
			cb(conn)
			return
		}
	}
}

// clientSetup is used to setup a new connection
func (p *Provider) clientSetup() (_ *client.Client, err error) {
	// Reset the previous backoff
	p.backoffLock.Lock()
	p.noRetry = false
	p.backoff = 0
	p.backoffLock.Unlock()

	// Dial a new connection
	opts := client.Opts{
		Dialer: &tcp.Dialer{
			TLSConfig: p.config.HCPConfig.SCADATLSConfig(),
		},
		LogOutput: p.logger.StandardWriter(&hclog.StandardLoggerOptions{InferLevels: true}),
	}
	client, err := client.DialOpts(p.config.HCPConfig.SCADAAddress(), &opts)
	if err != nil {
		p.logger.Error("failed to dial SCADA endpoint", "error", err)
		return nil, err
	}

	// Perform a handshake
	resp, err := p.handshake(client)
	if err != nil {
		p.logger.Error("handshake failed", "error", err)
		client.Close()
		return nil, err
	}
	if resp != nil && resp.SessionID != "" {
		p.logger.Debug("assigned session ID", "id", resp.SessionID)
	}
	if resp != nil && !resp.Authenticated {
		p.logger.Warn("authentication failed", "reason", resp.Reason)
	}

	// Set the new client
	p.clientLock.Lock()
	if p.client != nil {
		p.client.Close()
	}
	p.client = client
	p.clientLock.Unlock()

	p.sessionLock.Lock()
	p.sessionStatus = SessionStatusConnected
	p.sessionLock.Unlock()

	return client, nil
}

// handshake does the initial handshake
func (p *Provider) handshake(client *client.Client) (_ *HandshakeResponse, err error) {
	// Build the set of capabilities based on the registered handlers.
	p.handlersLock.RLock()
	capabilities := make(map[string]int, len(p.handlers))
	for h := range p.handlers {
		capabilities[h] = 1
	}
	p.handlersLock.RUnlock()

	oauthToken, err := p.config.HCPConfig.Token()
	if err != nil {
		return nil, fmt.Errorf("failed to get access token: %w", err)
	}

	resource := p.config.Resource
	req := HandshakeRequest{
		Service: p.config.Service,
		Resource: Resource{
			ID:             resource.ID,
			Type:           resource.Type,
			ProjectID:      resource.Location.ProjectID,
			OrganizationID: resource.Location.OrganizationID,
		},

		AccessToken: oauthToken.AccessToken,

		// TODO: remove once it is not required anymore.
		ServiceVersion: "0.0.1",

		Capabilities: capabilities,
		Meta:         p.GetMeta(),
	}
	resp := new(HandshakeResponse)
	if err := client.RPC("Session.Handshake", &req, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

type hijackFunc func(net.Conn)

// providerEndpoint is used to implement the Provider.* RPC endpoints
// as part of the provider.
type providerEndpoint struct {
	p      *Provider
	hijack hijackFunc
}

// Hijacked is used to check if the connection has been hijacked
func (pe *providerEndpoint) hijacked() bool {
	return pe.hijack != nil
}

// GetHijack returns the hijack function
func (pe *providerEndpoint) getHijack() hijackFunc {
	return pe.hijack
}

// Hijack is used to take over the yamux stream for Provider.Connect
func (pe *providerEndpoint) setHijack(cb hijackFunc) {
	pe.hijack = cb
}

// Connect is invoked by the broker to connect to a capability
func (pe *providerEndpoint) Connect(args *ConnectRequest, resp *ConnectResponse) error {
	pe.p.logger.Info("connect requested", "capability", args.Capability)

	// Handle potential flash
	if args.Severity != "" && args.Message != "" {
		switch hclog.LevelFromString(args.Severity) {
		case hclog.Trace:
			pe.p.logger.Trace("connect message", "msg", args.Message)
		case hclog.Debug:
			pe.p.logger.Debug("connect message", "msg", args.Message)
		case hclog.Info:
			pe.p.logger.Info("connect message", "msg", args.Message)
		case hclog.Warn:
			pe.p.logger.Warn("connect message", "msg", args.Message)
		}
	}

	// Look for the handler
	pe.p.handlersLock.RLock()
	handler := pe.p.handlers[args.Capability].provider
	pe.p.handlersLock.RUnlock()
	if handler == nil {
		pe.p.logger.Warn("requested capability not available", "capability", args.Capability)
		return fmt.Errorf("invalid capability")
	}

	// Hijack the connection
	pe.setHijack(func(a net.Conn) {
		if err := handler(args.Capability, args.Meta, a); err != nil {
			pe.p.logger.Error("handler errored", "capability", args.Capability, "error", err)
		}
	})
	resp.Success = true
	return nil
}

// Disconnect is invoked by the broker to ask us to backoff
func (pe *providerEndpoint) Disconnect(args *DisconnectRequest, resp *DisconnectResponse) error {
	if args.Reason == "" {
		args.Reason = "<no reason provided>"
	}
	pe.p.logger.Info("disconnect requested",
		"retry", !args.NoRetry,
		"backoff", args.Backoff,
		"reason", args.Reason)

	// Use the backoff information
	pe.p.backoffLock.Lock()
	pe.p.noRetry = args.NoRetry
	pe.p.backoff = args.Backoff
	pe.p.backoffLock.Unlock()

	// Clear the session information
	pe.p.sessionLock.Lock()
	pe.p.sessionStatus = SessionStatusDisconnected
	pe.p.sessionLock.Unlock()

	// Force the disconnect
	time.AfterFunc(disconnectDelay, func() {
		pe.p.clientLock.Lock()
		if pe.p.client != nil {
			pe.p.client.Close()
		}
		pe.p.clientLock.Unlock()
	})
	return nil
}

var _ SCADAProvider = &Provider{}
