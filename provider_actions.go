package provider

type action int

const (
	actionDefault     = iota // default value
	actionRehandshake        // perform the re-handshake
	actionDisconnect         // disconnect from the broker
)

// action sends one of the actions to the run loop
// while making sure the provider is running and cannot be stopped.
func (p *Provider) action(a action) error {
	p.runningLock.Lock()
	defer p.runningLock.Unlock()

	if !p.running {
		return errNotRunning
	}

	p.actions <- a
	return nil
}
