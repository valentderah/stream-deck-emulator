// Package link connects emulated devices to the local USB/IP client and
// tracks whether each one is attached.
package link

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"stream-deck-plus-emulator/internal/usbip"
)

type State string

const (
	Disconnected State = "disconnected"
	Connecting   State = "connecting"
	Connected    State = "connected"
	Failed       State = "error"
)

// Codes sent in a Status. The browser catalog has an entry for each.
const (
	CodeClientNotFound = "client_not_found"
	CodeClientFailed   = "client_failed"
	CodeAttachTimeout  = "attach_timeout"
)

func Codes() []string {
	return []string{CodeClientNotFound, CodeClientFailed, CodeAttachTimeout}
}

const stopTimeout = 5 * time.Second

var ErrUnknown = errors.New("link: unknown device")

// Status is the connection state of one device. Code and Detail explain a
// Failed state, or tell how to attach by hand while Connecting.
type Status struct {
	ID     string `json:"id"`
	State  State  `json:"state"`
	Code   string `json:"code,omitempty"`
	Detail string `json:"detail,omitempty"`
}

// Exporter is the USB/IP server side of a connection.
type Exporter interface {
	Export(busID string, dev usbip.Device) error
	Unexport(busID string)
}

// Client is the local USB/IP client.
type Client interface {
	Available() bool
	// Attach asks the client to import busID from the emulator.
	Attach(ctx context.Context, busID string) error
	// Stop ends the client's attach attempts for busID.
	Stop(ctx context.Context, busID string) error
	// Command is the attach command a person can run by hand.
	Command(busID string) string
}

type Device interface {
	usbip.Device
	Reset()
}

type Target struct {
	ID, BusID string
	Dev       Device
}

type entry struct {
	Target
	status Status
	// gen changes on every state transition started by Connect, Disconnect
	// or a failure, so that late results of an older attempt are ignored.
	gen int
	// cancel ends the client attempt started by the last Connect.
	cancel context.CancelFunc
	// dropped is set when the host ended the session and the export is still
	// to be removed. Whoever holds m.op next completes it.
	dropped bool
}

type Manager struct {
	exp     Exporter
	client  Client
	timeout time.Duration

	op sync.Mutex // serialises calls to exp, client and devices

	mu      sync.Mutex
	order   []*entry
	byID    map[string]*entry
	byBusID map[string]*entry
	subs    map[chan Status]struct{}
}

// New returns a Manager with every target disconnected. timeout bounds the
// time from Connect to an attached session.
func New(exp Exporter, client Client, timeout time.Duration, targets []Target) *Manager {
	m := &Manager{
		exp:     exp,
		client:  client,
		timeout: timeout,
		byID:    map[string]*entry{},
		byBusID: map[string]*entry{},
		subs:    map[chan Status]struct{}{},
	}
	for _, t := range targets {
		e := &entry{Target: t, status: Status{ID: t.ID, State: Disconnected}}
		m.order = append(m.order, e)
		m.byID[t.ID] = e
		m.byBusID[t.BusID] = e
	}
	return m
}

// Connect exports the device and asks the client to attach it. It returns
// at once; the outcome arrives as a Status.
func (m *Manager) Connect(id string) error {
	m.op.Lock()
	defer m.op.Unlock()

	m.mu.Lock()
	e, ok := m.byID[id]
	if !ok {
		m.mu.Unlock()
		return ErrUnknown
	}
	// Finish the removal of a dropped session's export before the state
	// becomes Connecting, so no client can claim the stale export meanwhile.
	// Unexport can wait on SessionChanged, so m.mu must not be held.
	dropped := e.dropped
	e.dropped = false
	m.mu.Unlock()
	if dropped {
		m.release(e)
	}

	m.mu.Lock()
	if e.status.State == Connecting || e.status.State == Connected {
		m.mu.Unlock()
		return nil
	}
	e.gen++
	gen := e.gen
	status := Status{ID: id, State: Connecting}
	manual := !m.client.Available()
	if manual {
		status.Code, status.Detail = CodeClientNotFound, m.client.Command(e.BusID)
	}
	m.set(e, status)
	m.mu.Unlock()

	if err := m.exp.Export(e.BusID, e.Dev); err != nil {
		m.failLocked(e, gen, CodeClientFailed, err.Error())
		return nil
	}
	if !manual {
		ctx, cancel := context.WithTimeout(context.Background(), m.timeout)
		m.mu.Lock()
		e.cancel = cancel
		m.mu.Unlock()
		go m.attach(ctx, cancel, e, gen)
	}
	return nil
}

func (m *Manager) attach(ctx context.Context, cancel context.CancelFunc, e *entry, gen int) {
	defer cancel()
	if err := m.client.Attach(ctx, e.BusID); err != nil {
		code := CodeClientFailed
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			code = CodeAttachTimeout
		}
		m.fail(e, gen, code, err.Error())
		return
	}
	// The client is done; the session should exist. If it does not appear
	// before the deadline, give up.
	<-ctx.Done()
	m.fail(e, gen, CodeAttachTimeout, "")
}

// fail moves an attempt that is still pending to Failed.
func (m *Manager) fail(e *entry, gen int, code, detail string) {
	m.op.Lock()
	defer m.op.Unlock()
	m.failLocked(e, gen, code, detail)
}

// failLocked is fail with m.op held.
func (m *Manager) failLocked(e *entry, gen int, code, detail string) {
	m.mu.Lock()
	if e.gen != gen || e.status.State != Connecting {
		m.mu.Unlock()
		return
	}
	e.gen++
	m.cancelAttempt(e)
	m.set(e, Status{ID: e.ID, State: Failed, Code: code, Detail: detail})
	m.mu.Unlock()
	m.exp.Unexport(e.BusID)
}

// Disconnect detaches the device from the host and stops exporting it.
func (m *Manager) Disconnect(id string) error {
	m.op.Lock()
	defer m.op.Unlock()

	m.mu.Lock()
	e, ok := m.byID[id]
	if !ok {
		m.mu.Unlock()
		return ErrUnknown
	}
	if e.status.State == Disconnected {
		m.mu.Unlock()
		return nil
	}
	live := e.status.State == Connecting || e.status.State == Connected
	e.gen++
	m.cancelAttempt(e)
	m.set(e, Status{ID: id, State: Disconnected})
	m.mu.Unlock()

	if live && m.client.Available() {
		ctx, cancel := context.WithTimeout(context.Background(), stopTimeout)
		if err := m.client.Stop(ctx, e.BusID); err != nil {
			log.Printf("link: %s: stop client attempts: %v", id, err)
		}
		cancel()
	}
	m.exp.Unexport(e.BusID)
	e.Dev.Reset()
	return nil
}

// cancelAttempt ends the client attempt of e, if any. m.mu must be held.
func (m *Manager) cancelAttempt(e *entry) {
	if e.cancel != nil {
		e.cancel()
		e.cancel = nil
	}
}

// release unexports a device and resets it. m.op must be held.
func (m *Manager) release(e *entry) {
	m.exp.Unexport(e.BusID)
	e.Dev.Reset()
}

// SessionChanged records that a client attached to or detached from busID.
// It is the usbip.Server.OnSession callback.
func (m *Manager) SessionChanged(busID string, attached bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.byBusID[busID]
	if !ok {
		return
	}
	switch {
	case attached && e.status.State == Connecting:
		m.set(e, Status{ID: e.ID, State: Connected})
	case !attached && e.status.State == Connected:
		// The host dropped the device.
		e.gen++
		e.dropped = true
		m.set(e, Status{ID: e.ID, State: Disconnected})
		go m.cleanup(e)
	}
}

// cleanup removes the export of a device whose session ended on its own,
// unless a Connect has already done so.
func (m *Manager) cleanup(e *entry) {
	m.op.Lock()
	defer m.op.Unlock()
	m.mu.Lock()
	dropped := e.dropped
	e.dropped = false
	m.mu.Unlock()
	if dropped {
		m.release(e)
	}
}

// Statuses returns every device's status in target order.
func (m *Manager) Statuses() []Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	list := make([]Status, len(m.order))
	for i, e := range m.order {
		list[i] = e.status
	}
	return list
}

func (m *Manager) Subscribe() (<-chan Status, func()) {
	ch := make(chan Status, 64)
	m.mu.Lock()
	m.subs[ch] = struct{}{}
	m.mu.Unlock()
	return ch, func() {
		m.mu.Lock()
		delete(m.subs, ch)
		m.mu.Unlock()
	}
}

// set stores and publishes a status. m.mu must be held.
func (m *Manager) set(e *entry, s Status) {
	e.status = s
	for ch := range m.subs {
		select {
		case ch <- s:
		default:
		}
	}
}
