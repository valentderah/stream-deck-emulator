package usbip

import (
	"bytes"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

const deviceRecordSize = 312

type fakeDev struct {
	in chan []byte

	mu       sync.Mutex
	attached []bool
}

func newFakeDev() *fakeDev { return &fakeDev{in: make(chan []byte, 1)} }

func (d *fakeDev) Info() Info {
	return Info{Speed: SpeedHigh, VendorID: 0x1234, ProductID: 0x5678, ConfigurationValue: 1, NumConfig: 1, Interfaces: []Interface{{Class: 3}}}
}
func (d *fakeDev) Control(Setup, []byte) ([]byte, error) { return []byte{1, 2, 3}, nil }
func (d *fakeDev) Out(uint32, []byte)                    {}
func (d *fakeDev) In() <-chan []byte                     { return d.in }
func (d *fakeDev) SetAttached(attached bool) {
	d.mu.Lock()
	d.attached = append(d.attached, attached)
	d.mu.Unlock()
}

func (d *fakeDev) history() []bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]bool(nil), d.attached...)
}

func start(t *testing.T, onSession func(string, bool)) (*Server, string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := NewServer()
	s.OnSession = onSession
	go s.Serve(ln)
	t.Cleanup(func() { ln.Close() })
	return s, ln.Addr().String()
}

func dial(t *testing.T, addr string) net.Conn {
	t.Helper()
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	c.SetDeadline(time.Now().Add(5 * time.Second))
	t.Cleanup(func() { c.Close() })
	return c
}

func read(t *testing.T, c net.Conn, n int) []byte {
	t.Helper()
	buf := make([]byte, n)
	if _, err := io.ReadFull(c, buf); err != nil {
		t.Fatal(err)
	}
	return buf
}

func devlist(t *testing.T, addr string) []string {
	t.Helper()
	c := dial(t, addr)
	c.Write(opHeader(opReqDevlist, 0))
	read(t, c, 8)
	var ids []string
	for n := be.Uint32(read(t, c, 4)); n > 0; n-- {
		rec := read(t, c, deviceRecordSize)
		ids = append(ids, cString(rec[256:288]))
		read(t, c, 4*int(rec[deviceRecordSize-1]))
	}
	return ids
}

// importDev attaches to busID and returns the connection and the reply status.
func importDev(t *testing.T, addr, busID string) (net.Conn, uint32) {
	t.Helper()
	c := dial(t, addr)
	var id [32]byte
	copy(id[:], busID)
	c.Write(append(opHeader(opReqImport, 0), id[:]...))
	status := be.Uint32(read(t, c, 8)[4:])
	if status == 0 {
		read(t, c, deviceRecordSize)
	}
	return c, status
}

func TestDevlistReflectsExports(t *testing.T) {
	s, addr := start(t, nil)
	if got := devlist(t, addr); len(got) != 0 {
		t.Fatalf("empty server lists %v", got)
	}
	s.Export("1-2", newFakeDev())
	s.Export("1-1", newFakeDev())
	if got := devlist(t, addr); len(got) != 2 || got[0] != "1-1" || got[1] != "1-2" {
		t.Errorf("got %v", got)
	}
	s.Unexport("1-1")
	if got := devlist(t, addr); len(got) != 1 || got[0] != "1-2" {
		t.Errorf("after unexport: %v", got)
	}
}

func TestExportTwice(t *testing.T) {
	s := NewServer()
	if err := s.Export("1-1", newFakeDev()); err != nil {
		t.Fatal(err)
	}
	if err := s.Export("1-1", newFakeDev()); !errors.Is(err, ErrExported) {
		t.Errorf("got %v", err)
	}
}

func TestImportUnexportedFails(t *testing.T) {
	_, addr := start(t, nil)
	if _, status := importDev(t, addr, "1-1"); status == 0 {
		t.Error("import of an unexported bus id succeeded")
	}
}

func TestSecondImportRejected(t *testing.T) {
	s, addr := start(t, nil)
	s.Export("1-1", newFakeDev())
	if _, status := importDev(t, addr, "1-1"); status != 0 {
		t.Fatalf("first import: status %d", status)
	}
	if _, status := importDev(t, addr, "1-1"); status == 0 {
		t.Error("second import succeeded")
	}
}

func TestUnexportClosesSession(t *testing.T) {
	sessions := make(chan bool, 4)
	s, addr := start(t, func(busID string, attached bool) {
		if busID != "1-1" {
			t.Errorf("bus id %q", busID)
		}
		sessions <- attached
	})
	dev := newFakeDev()
	s.Export("1-1", dev)
	c, status := importDev(t, addr, "1-1")
	if status != 0 {
		t.Fatalf("status %d", status)
	}
	if !<-sessions {
		t.Fatal("expected a session start")
	}

	s.Unexport("1-1")

	// Unexport returns only after the session is gone.
	select {
	case attached := <-sessions:
		if attached {
			t.Error("expected a session end")
		}
	default:
		t.Fatal("Unexport returned before the session ended")
	}
	if got := dev.history(); len(got) != 2 || !got[0] || got[1] {
		t.Errorf("SetAttached calls: %v", got)
	}
	if _, err := c.Read(make([]byte, 1)); err == nil {
		t.Error("client connection still open")
	}
	s.Unexport("1-1") // no-op
}

func TestReexportAfterUnexport(t *testing.T) {
	s, addr := start(t, nil)
	dev := newFakeDev()
	s.Export("1-1", dev)
	importDev(t, addr, "1-1")
	s.Unexport("1-1")
	if err := s.Export("1-1", dev); err != nil {
		t.Fatal(err)
	}
	if _, status := importDev(t, addr, "1-1"); status != 0 {
		t.Errorf("import after re-export: status %d", status)
	}
	if got := dev.history(); len(got) != 3 || !got[2] {
		t.Errorf("SetAttached calls: %v", got)
	}
}

func TestControlTransfer(t *testing.T) {
	s, addr := start(t, nil)
	s.Export("1-1", newFakeDev())
	c, _ := importDev(t, addr, "1-1")

	hdr := make([]byte, urbHeaderSize)
	be.PutUint32(hdr[0:], cmdSubmit)
	be.PutUint32(hdr[4:], 7)      // seqnum
	be.PutUint32(hdr[12:], dirIn) // direction
	be.PutUint32(hdr[24:], 18)    // transfer buffer length
	copy(hdr[40:], []byte{0x80, 0x06, 0x00, 0x01, 0x00, 0x00, 18, 0x00})
	c.Write(hdr)

	reply := read(t, c, urbHeaderSize)
	if be.Uint32(reply[0:]) != retSubmit || be.Uint32(reply[4:]) != 7 || be.Uint32(reply[20:]) != 0 {
		t.Fatalf("reply header %x", reply)
	}
	if n := be.Uint32(reply[24:]); n != 3 {
		t.Fatalf("actual length %d", n)
	}
	if data := read(t, c, 3); !bytes.Equal(data, []byte{1, 2, 3}) {
		t.Errorf("data %x", data)
	}
}

func TestHostileTransferLength(t *testing.T) {
	for _, length := range []uint32{0xffffffff, 0x80000000, 0x7fffffff, 1 << 20} {
		for _, dir := range []uint32{dirIn, dirOut} {
			s, addr := start(t, nil)
			s.Export("1-1", newFakeDev())
			c, status := importDev(t, addr, "1-1")
			if status != 0 {
				t.Fatalf("import: status %d", status)
			}
			hdr := make([]byte, urbHeaderSize)
			be.PutUint32(hdr[0:], cmdSubmit)
			be.PutUint32(hdr[4:], 1)
			be.PutUint32(hdr[12:], dir)
			be.PutUint32(hdr[16:], 0) // control endpoint
			be.PutUint32(hdr[24:], length)
			copy(hdr[40:], []byte{0x80, 0x06, 0x00, 0x01, 0x00, 0x00, 18, 0x00})
			c.Write(hdr)

			// The server drops the session instead of replying or panicking.
			if _, err := c.Read(make([]byte, 1)); err == nil {
				t.Errorf("length %#x dir %d: got data, want the connection closed", length, dir)
			}
			c.Close()

			// The server and the export survive: a fresh client attaches.
			var again net.Conn
			for i := 0; i < 50; i++ {
				var st uint32
				again, st = importDev(t, addr, "1-1")
				if st == 0 {
					break
				}
				again = nil
				time.Sleep(10 * time.Millisecond)
			}
			if again == nil {
				t.Fatalf("length %#x dir %d: export unusable after hostile length", length, dir)
			}
		}
	}
}
