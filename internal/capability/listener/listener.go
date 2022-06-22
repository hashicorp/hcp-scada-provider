package listener

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/hashicorp/hcp-scada-provider/internal/capability"
)

// New creates a new SCADA listener using the given configuration.
// Requests for the HTTP capability are passed off to the listener that is
// returned.
func New(serviceID string) (capability.Provider, net.Listener, error) {
	// Create a listener and handler
	list := newScadaListener(serviceID)
	provider := func(capability string, meta map[string]string,
		conn net.Conn) error {
		return list.Push(conn)
	}

	return provider, list, nil
}

// scadaListener is used to return a net.Listener for incoming SCADA connections
type scadaListener struct {
	addr    *scadaAddr
	pending chan net.Conn

	closed   bool
	closedCh chan struct{}
	l        sync.Mutex
}

// newScadaListener returns a new listener
func newScadaListener(serviceID string) *scadaListener {
	l := &scadaListener{
		addr:     &scadaAddr{serviceID},
		pending:  make(chan net.Conn),
		closedCh: make(chan struct{}),
	}
	return l
}

// Push is used to add a connection to the queu
func (s *scadaListener) Push(conn net.Conn) error {
	select {
	case s.pending <- conn:
		return nil
	case <-time.After(time.Second):
		return fmt.Errorf("accept timed out")
	case <-s.closedCh:
		return fmt.Errorf("scada listener closed")
	}
}

func (s *scadaListener) Accept() (net.Conn, error) {
	select {
	case conn := <-s.pending:
		return conn, nil
	case <-s.closedCh:
		return nil, fmt.Errorf("scada listener closed")
	}
}

func (s *scadaListener) Close() error {
	s.l.Lock()
	defer s.l.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	close(s.closedCh)
	return nil
}

func (s *scadaListener) Addr() net.Addr {
	return s.addr
}

// scadaAddr is used to return a net.Addr for SCADA
type scadaAddr struct {
	serviceID string
}

func (s *scadaAddr) Network() string {
	return "SCADA"
}

func (s *scadaAddr) String() string {
	return fmt.Sprintf("SCADA::%s", s.serviceID)
}
