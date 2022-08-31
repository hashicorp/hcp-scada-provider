package provider

import (
	"context"
	"errors"
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
	"golang.org/x/sync/errgroup"

	"github.com/hashicorp/hcp-scada-provider/internal/client"
	"github.com/hashicorp/hcp-scada-provider/internal/client/dialer/tcp"
	"github.com/hashicorp/hcp-scada-provider/internal/listener"
	"github.com/hashicorp/hcp-scada-provider/internal/resource"
	"github.com/hashicorp/hcp-scada-provider/types"
)

const (
	// defaultBackoff is the amount of time we back off if we encounter an
	// error, and no specific backoff is available.
	defaultBackoff = 10 * time.Second

	// disconnectDelay is the amount of time to wait between the moment
	// the disconnect RPC call is received and actually disconnecting the provider.
	disconnectDelay = time.Second
)

var (
	errNoRetry    = errors.New("provider is configured to not retry a connection")
	errNotRunning = errors.New("provider is not running")
)

type handler struct {
	provider listener.Provider
	listener net.Listener
}

// New creates a new SCADA provider instance using the configuration in config.
func New(config *Config) (SCADAProvider, error) {
	if config.Logger == nil {
		return nil, fmt.Errorf("failed to initialize SCADA provider: Logger must be provided")
	}
	if config.HCPConfig == nil {
		return nil, fmt.Errorf("failed to initialize SCADA provider: HCPConfig must be provided")
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

	handlers     map[string]handler
	handlersLock sync.RWMutex

	noRetry     bool          // set when the server instructs us to not retry
	backoff     time.Duration // set when the server provides a longer backoff
	backoffLock sync.Mutex

	meta     map[string]string
	metaLock sync.RWMutex

	running     bool
	runningLock sync.Mutex

	sessionStatus SessionStatus

	actions chan action

	cancel context.CancelFunc
}

// construct is used to create a new provider.
func construct(config *Config) (*Provider, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	p := &Provider{
		config:        config,
		logger:        config.Logger.Named("scada-provider"),
		meta:          map[string]string{},
		handlers:      map[string]handler{},
		sessionStatus: SessionStatusDisconnected,
		actions:       make(chan action),
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

	p.logger.Info("starting")

	// Set the provider to its running state
	p.running = true
	// Run the provider
	p.cancel = p.run()

	return nil
}

// Stop will gracefully close the currently active SCADA session. This will
// not close the capability listeners.
func (p *Provider) Stop() error {
	p.runningLock.Lock()
	defer p.runningLock.Unlock()

	// Check if the provider is already stopped
	if !p.running {
		return nil
	}

	p.logger.Info("stopping")

	// Stop the provider
	p.cancel()
	// Set the provider to its non-running state
	p.running = false

	return nil
}

// isStopped checks if the provider has been stopped.
func (p *Provider) isStopped() bool {
	p.runningLock.Lock()
	defer p.runningLock.Unlock()
	return !p.running
}

// SetMetaValue sets the meta-data value. Empty values will not be
// transmitted to the broker.
//
// New meta data can be added at any time.
func (p *Provider) SetMetaValue(key, value string) {
	p.metaLock.Lock()
	defer p.metaLock.Unlock()

	// Remove the value if it is empty
	if value == "" {
		delete(p.meta, key)
	} else {
		p.meta[key] = value
	}
	// tell the run loop to re-handshake and update the broker
	p.action(actionRehandshake)
}

// GetMeta returns the provider's current meta-data.
func (p *Provider) GetMeta() map[string]string {
	return p.meta
}

// Listen will expose the provided capability and make new connections
// available through the returned listener. Closing the listener will stop
// exposing the provided capability.
//
// The method will return an existing listener if the capability already existed.
// Listeners can be retrieved even when the provider is stopped (e.g. before it is
// started). New capabilities and new meta data can be added at any time.
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

	// re-handshake to update the broker
	p.action(actionRehandshake)

	return capListenerProxy, nil
}

// SessionStatus returns the status of the SCADA connection.
func (p *Provider) SessionStatus() SessionStatus {
	return p.sessionStatus
}

func (p *Provider) backoffReset() {
	// Reset the previous backoff
	p.backoffLock.Lock()
	p.noRetry = false
	p.backoff = 0
	p.backoffLock.Unlock()
}

// backoffDuration is used to compute the next backoff duration.
// it returns the backoff time to wait for and a bool that will be
// set to true if no retries should be attempted.
func (p *Provider) backoffDuration() (time.Duration, bool) {
	// Use the default backoff
	backoff := defaultBackoff

	// Check for a server specified backoff
	p.backoffLock.Lock()
	defer p.backoffLock.Unlock()
	if p.backoff != 0 {
		backoff = p.backoff
	}
	if p.noRetry {
		backoff = 0
	}

	// Use the test backoff
	if p.config.TestBackoff != 0 {
		backoff = p.config.TestBackoff
	}

	return backoff, p.noRetry
}

// wait is used to delay dialing on an error.
// it will return an error if the connection should not be
// retried.
func (p *Provider) wait(ctx context.Context) error {
	// Compute the backoff time
	backoff, noRetry := p.backoffDuration()
	// is this a no retry situation?
	if noRetry {
		return errNoRetry
	}

	// Setup a wait timer
	var wait <-chan time.Time
	if backoff > 0 {
		backoff = backoff + time.Duration(rand.Uint32())%backoff
		p.logger.Debug("backing off", "seconds", backoff.Seconds())
		wait = time.After(backoff)
	}

	// Wait until timer or shutdown
	select {
	case <-wait:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// run is a long running routine to manage the provider.
func (p *Provider) run() context.CancelFunc {
	var statuses = make(chan SessionStatus)

	// setup a context that will
	// cancel on stop
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		defer cancel()
		var cl *client.Client
		// engage in running the provider
		for {
			select {
			case status := <-statuses:
				switch status {
				case SessionStatusWaiting:
					p.sessionStatus = SessionStatusWaiting
					// backoff
					go func() {
						if err := p.wait(ctx); err != nil {
							// wait returns an errors if we shouldn't retry
							// or if ctx is canceled()
							statuses <- SessionStatusDisconnected
						} else {
							statuses <- SessionStatusConnecting
						}
					}()

				case SessionStatusConnecting:
					p.sessionStatus = SessionStatusConnecting
					// Try to connect a session
					go func() {
						// if we get canceled() during this,
						// connect will error out and we go to SessionStatusWaiting
						if client, err := p.connect(ctx); err != nil {
							// connect closes client if any error
							// occured at handshake() except for resp.Authenticated == false
							statuses <- SessionStatusWaiting
						} else {
							cl = client
							statuses <- SessionStatusConnected
						}
					}()

				case SessionStatusConnected:
					p.sessionStatus = SessionStatusConnected
					// reset any longer backoff period set by the Disconnect RPC call
					p.backoffReset()
					go func(client *client.Client) {
						// Handle the session
						if err := p.handleSession(ctx, client); err != nil {
							// handleSession will always close client
							// on errors or if the ctx is canceled().
							// go to the waiting state
							statuses <- SessionStatusWaiting
						}
					}(cl)

				case SessionStatusDisconnected:
					p.sessionStatus = SessionStatusDisconnected
					// after officially disconnecting, reset the backoff period
					p.backoffReset()
					return
				}

			case action := <-p.actions:
				switch action {
				case actionDisconnect:
					// this is a disconnect signal
					// received via RPC, we are certain to be
					// connected. During testing we are using mocks
					// and the client will never have been created.
					if p.sessionStatus != SessionStatusConnected {
						continue
					}

					if cl != nil {
						cl.Close()
					}

				case actionRehandshake:
					if p.sessionStatus != SessionStatusConnected {
						continue
					}
					// handshake will close cl on errors
					if _, err := p.handshake(ctx, cl); err != nil {
					}
				}

				// ¯\_(ツ)_/¯
			}
		}
	}()

	// initialize the for loop
	statuses <- SessionStatusConnecting
	return cancel
}

// handleSession is used to handle an established session.
func (p *Provider) handleSession(ctx context.Context, yamux net.Listener) error {
	var done = make(chan bool)
	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		// make the other go routine return
		// if yamux.Accept() errors out
		defer close(done)
		defer yamux.Close()
		for {
			if conn, err := yamux.Accept(); err != nil {
				select {
				case <-ctx.Done():
					// Do not log an error if we are shutting down
				default:
					p.logger.Error("failed to accept connection", "error", err)
				}
				return err
			} else {
				p.logger.Debug("accepted connection")
				go p.handleConnection(ctx, conn)
			}
		}
	})

	g.Go(func() error {
		// return nil here so that g.Wait()
		// always picks the error the Accept() routine
		// returned.
		for {
			select {
			case <-done:
				// the other go routine returned with an error
				// and closed the yamux client
				return nil

			case <-ctx.Done():
				// make the other go routine return
				// if ctx is canceled()
				yamux.Close()
				return nil
			}
		}
	})

	return g.Wait()
}

// handleConnection handles an incoming connection.
func (p *Provider) handleConnection(ctx context.Context, conn net.Conn) {
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

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

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

// connect sets up a new connection to a broker.
func (p *Provider) connect(ctx context.Context) (*client.Client, error) {
	// Dial a new connection
	opts := client.Opts{
		Dialer: &tcp.Dialer{
			TLSConfig: p.config.HCPConfig.SCADATLSConfig(),
		},
		LogOutput: p.logger.StandardWriter(&hclog.StandardLoggerOptions{InferLevels: true}),
	}
	client, err := client.DialOptsContext(ctx, p.config.HCPConfig.SCADAAddress(), &opts)
	if err != nil {
		p.logger.Error("failed to dial SCADA endpoint", "error", err)
		return nil, err
	}

	// Perform a handshake
	_, err = p.handshake(ctx, client)
	if err != nil {
		p.logger.Error("handshake failed", "error", err)
		return nil, err
	}

	return client, nil
}

// handshake does the initial handshake.
func (p *Provider) handshake(ctx context.Context, client *client.Client) (*types.HandshakeResponse, error) {
	// Build the set of capabilities based on the registered handlers.
	p.handlersLock.RLock()
	capabilities := make(map[string]int, len(p.handlers))
	for h := range p.handlers {
		capabilities[h] = 1
	}
	p.handlersLock.RUnlock()

	oauthToken, err := p.config.HCPConfig.Token()
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("failed to get access token: %w", err)
	}

	// make sure nobody is writing to the
	// meta map while client.RPC is reading from it
	p.metaLock.RLock()
	defer p.metaLock.RUnlock()

	req := types.HandshakeRequest{
		Service:  p.config.Service,
		Resource: p.config.Resource,

		AccessToken: oauthToken.AccessToken,

		// TODO: remove once it is not required anymore.
		ServiceVersion: "0.0.1",

		Capabilities: capabilities,
		Meta:         p.GetMeta(),
	}
	resp := new(types.HandshakeResponse)
	if err := client.RPC("Session.Handshake", &req, resp); err != nil {
		client.Close()
		return nil, err
	}

	if resp != nil && resp.SessionID != "" {
		p.logger.Debug("assigned session ID", "id", resp.SessionID)
	}
	if resp != nil && !resp.Authenticated {
		p.logger.Warn("authentication failed", "reason", resp.Reason)
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

// hijacked is used to check if the connection has been hijacked.
func (pe *providerEndpoint) hijacked() bool {
	return pe.hijack != nil
}

// getHijack returns the hijack function.
func (pe *providerEndpoint) getHijack() hijackFunc {
	return pe.hijack
}

// setHijack is used to take over the yamux stream for Provider.Connect.
func (pe *providerEndpoint) setHijack(cb hijackFunc) {
	pe.hijack = cb
}

// Connect is invoked by the broker to connect to a capability.
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

// Disconnect is invoked by the broker to ask us to backoff.
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

	// Force the disconnect
	time.AfterFunc(disconnectDelay, func() {
		pe.p.action(actionDisconnect)
	})
	return nil
}

var _ SCADAProvider = &Provider{}
