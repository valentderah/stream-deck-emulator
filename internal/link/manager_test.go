package link

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"stream-deck-plus-emulator/internal/usbip"
)

type fakeDev struct {
	mu     sync.Mutex
	resets int
}

func (d *fakeDev) Info() usbip.Info                            { return usbip.Info{} }
func (d *fakeDev) Control(usbip.Setup, []byte) ([]byte, error) { return nil, nil }
func (d *fakeDev) Out(uint32, []byte)                          {}
func (d *fakeDev) In() <-chan []byte                           { return nil }
func (d *fakeDev) SetAttached(bool)                            {}
func (d *fakeDev) Reset() {
	d.mu.Lock()
	d.resets++
	d.mu.Unlock()
}
func (d *fakeDev) resetCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.resets
}

type fakeExp struct {
	mu       sync.Mutex
	exported map[string]bool
	exports  int
}

func (e *fakeExp) Export(busID string, _ usbip.Device) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.exported[busID] {
		return usbip.ErrExported
	}
	e.exported[busID] = true
	e.exports++
	return nil
}
func (e *fakeExp) Unexport(busID string) {
	e.mu.Lock()
	delete(e.exported, busID)
	e.mu.Unlock()
}
func (e *fakeExp) has(busID string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.exported[busID]
}
func (e *fakeExp) count() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.exports
}

type fakeClient struct {
	available bool
	// attach is the behaviour of Attach; nil means succeed.
	attach func(ctx context.Context, busID string) error

	mu      sync.Mutex
	stopped []string
}

func (c *fakeClient) Available() bool { return c.available }
func (c *fakeClient) Attach(ctx context.Context, busID string) error {
	if c.attach == nil {
		return nil
	}
	return c.attach(ctx, busID)
}
func (c *fakeClient) Stop(_ context.Context, busID string) error {
	c.mu.Lock()
	c.stopped = append(c.stopped, busID)
	c.mu.Unlock()
	return nil
}
func (c *fakeClient) Command(busID string) string { return "usbip attach -b " + busID }
func (c *fakeClient) stops() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.stopped)
}

type rig struct {
	m      *Manager
	exp    *fakeExp
	client *fakeClient
	dev    *fakeDev
}

func newRig(timeout time.Duration) *rig {
	r := &rig{exp: &fakeExp{exported: map[string]bool{}}, client: &fakeClient{available: true}, dev: &fakeDev{}}
	r.m = New(r.exp, r.client, timeout, []Target{{ID: "plus", BusID: "1-1", Dev: r.dev}})
	return r
}

func (r *rig) status() Status { return r.m.Statuses()[0] }

func (r *rig) wait(t *testing.T, want State) Status {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		s := r.status()
		if s.State == want {
			return s
		}
		if time.Now().After(deadline) {
			t.Fatalf("state %q, want %q", s.State, want)
		}
		time.Sleep(time.Millisecond)
	}
}

// attaches makes the fake client behave like a working one: the server
// reports a session before Attach returns.
func (r *rig) attaches() {
	r.client.attach = func(_ context.Context, busID string) error {
		r.m.SessionChanged(busID, true)
		return nil
	}
}

func TestConnect(t *testing.T) {
	r := newRig(time.Second)
	r.attaches()
	events, cancel := r.m.Subscribe()
	defer cancel()
	if err := r.m.Connect("plus"); err != nil {
		t.Fatal(err)
	}
	if s := <-events; s.State != Connecting || s.ID != "plus" {
		t.Errorf("first event %+v", s)
	}
	if s := <-events; s.State != Connected {
		t.Errorf("second event %+v", s)
	}
	if !r.exp.has("1-1") {
		t.Error("not exported")
	}
}

func TestConnectTwice(t *testing.T) {
	r := newRig(time.Second)
	r.attaches()
	r.m.Connect("plus")
	r.m.Connect("plus")
	r.wait(t, Connected)
	r.m.Connect("plus")
	if n := r.exp.count(); n != 1 {
		t.Errorf("exported %d times", n)
	}
	if r.status().State != Connected {
		t.Errorf("state %q", r.status().State)
	}
}

func TestClientFailure(t *testing.T) {
	r := newRig(time.Second)
	r.client.attach = func(context.Context, string) error { return errors.New("no driver") }
	r.m.Connect("plus")
	s := r.wait(t, Failed)
	if s.Code != CodeClientFailed || s.Detail != "no driver" {
		t.Errorf("status %+v", s)
	}
	if r.exp.has("1-1") {
		t.Error("still exported after a failure")
	}
}

func TestAttachTimeout(t *testing.T) {
	r := newRig(30 * time.Millisecond)
	r.m.Connect("plus") // the client returns, no session ever starts
	s := r.wait(t, Failed)
	if s.Code != CodeAttachTimeout {
		t.Errorf("status %+v", s)
	}
	if r.exp.has("1-1") {
		t.Error("still exported after a timeout")
	}
}

func TestClientNotFound(t *testing.T) {
	r := newRig(30 * time.Millisecond)
	r.client.available = false
	r.m.Connect("plus")
	s := r.status()
	if s.State != Connecting || s.Code != CodeClientNotFound || s.Detail != "usbip attach -b 1-1" {
		t.Fatalf("status %+v", s)
	}
	time.Sleep(60 * time.Millisecond)
	if r.status().State != Connecting {
		t.Error("manual mode timed out")
	}
	r.m.SessionChanged("1-1", true)
	if s := r.status(); s.State != Connected || s.Code != "" {
		t.Errorf("after a manual attach: %+v", s)
	}
}

func TestDisconnect(t *testing.T) {
	r := newRig(time.Second)
	r.attaches()
	r.m.Connect("plus")
	r.wait(t, Connected)
	if err := r.m.Disconnect("plus"); err != nil {
		t.Fatal(err)
	}
	if r.status().State != Disconnected || r.exp.has("1-1") {
		t.Errorf("status %+v, exported %v", r.status(), r.exp.has("1-1"))
	}
	if r.client.stops() != 1 || r.dev.resetCount() != 1 {
		t.Errorf("stops %d, resets %d", r.client.stops(), r.dev.resetCount())
	}
	r.m.SessionChanged("1-1", false) // the server reports the closed session
	if r.status().State != Disconnected {
		t.Error("late session end changed the state")
	}
}

func TestDisconnectWhileConnecting(t *testing.T) {
	r := newRig(time.Second)
	release := make(chan struct{})
	r.client.attach = func(context.Context, string) error {
		<-release
		return errors.New("refused")
	}
	r.m.Connect("plus")
	r.m.Disconnect("plus")
	close(release)
	time.Sleep(20 * time.Millisecond)
	if s := r.status(); s.State != Disconnected || s.Code != "" {
		t.Errorf("late client failure changed the state: %+v", s)
	}
}

func TestSessionEndsOnItsOwn(t *testing.T) {
	r := newRig(time.Second)
	r.attaches()
	r.m.Connect("plus")
	r.wait(t, Connected)
	r.m.SessionChanged("1-1", false)
	r.wait(t, Disconnected)
	deadline := time.Now().Add(2 * time.Second)
	for r.exp.has("1-1") || r.dev.resetCount() != 1 {
		if time.Now().After(deadline) {
			t.Fatalf("exported %v, resets %d", r.exp.has("1-1"), r.dev.resetCount())
		}
		time.Sleep(time.Millisecond)
	}
}

func TestReconnectAfterFailure(t *testing.T) {
	r := newRig(time.Second)
	r.client.attach = func(context.Context, string) error { return errors.New("x") }
	r.m.Connect("plus")
	r.wait(t, Failed)
	r.attaches()
	r.m.Connect("plus")
	r.wait(t, Connected)
}

func TestUnknownDevice(t *testing.T) {
	r := newRig(time.Second)
	if err := r.m.Connect("xl"); !errors.Is(err, ErrUnknown) {
		t.Errorf("connect: %v", err)
	}
	if err := r.m.Disconnect("xl"); !errors.Is(err, ErrUnknown) {
		t.Errorf("disconnect: %v", err)
	}
}

func TestConnectAfterHostDrop(t *testing.T) {
	for i := 0; i < 100; i++ {
		r := newRig(time.Second)
		r.attaches()
		r.m.Connect("plus")
		r.wait(t, Connected)

		// Keep the cleanup goroutine from running before Connect can race it.
		r.m.op.Lock()
		r.m.SessionChanged("1-1", false)
		r.m.op.Unlock()
		if err := r.m.Connect("plus"); err != nil {
			t.Fatal(err)
		}
		if s := r.wait(t, Connected); s.Code != "" {
			t.Fatalf("round %d: %+v", i, s)
		}
		time.Sleep(time.Millisecond) // let a late cleanup run
		if !r.exp.has("1-1") || r.exp.count() != 2 || r.status().State != Connected {
			t.Fatalf("round %d: exported %v, exports %d, status %+v", i, r.exp.has("1-1"), r.exp.count(), r.status())
		}
		if n := r.dev.resetCount(); n != 1 {
			t.Fatalf("round %d: %d resets", i, n)
		}
	}
}

func TestConnectBeforeCleanupRuns(t *testing.T) {
	r := newRig(time.Second)
	r.attaches()
	r.m.Connect("plus")
	r.wait(t, Connected)
	r.m.op.Lock()
	r.m.SessionChanged("1-1", false)
	r.m.op.Unlock()
	// Connect completes the pending cleanup itself, whichever goroutine wins.
	r.m.Connect("plus")
	if s := r.wait(t, Connected); s.Code != "" {
		t.Errorf("status %+v", s)
	}
}

func TestDisconnectCancelsAttempt(t *testing.T) {
	r := newRig(time.Second)
	started := make(chan struct{})
	cancelled := make(chan struct{})
	r.client.attach = func(ctx context.Context, _ string) error {
		close(started)
		<-ctx.Done()
		close(cancelled)
		return ctx.Err()
	}
	r.m.Connect("plus")
	<-started
	r.m.Disconnect("plus")
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("attach context not cancelled by Disconnect")
	}
	time.Sleep(20 * time.Millisecond)
	if s := r.status(); s.State != Disconnected || s.Code != "" {
		t.Errorf("cancelled attempt changed the state: %+v", s)
	}
}
