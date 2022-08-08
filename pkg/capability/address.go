package capability

import (
	"fmt"
)

// NewAddr will return a new address struct for the passed capabiilty.
func NewAddr(capability string) *Addr {
	return &Addr{capability: capability}
}

// Addr is an address that represents a SCADA capability.
type Addr struct {
	capability string
}

// Network will return the name of the network.
func (a *Addr) Network() string {
	return "scada"
}

// String will return the string form of the address.
func (a *Addr) String() string {
	return fmt.Sprintf("scada::%s", a.capability)
}

// Capability is the SCADA capability of the address.
func (a *Addr) Capability() string {
	return a.capability
}
