package usbip

import (
	"encoding/binary"
	"errors"
	"io"
	"net"
	"sync"
)

var errTransferLength = errors.New("transfer buffer length out of range")

type pendingIn struct {
	seq    uint32
	length int
}

// session serves the URBs of one attached client.
type session struct {
	c   net.Conn
	dev Device

	wmu sync.Mutex

	mu      sync.Mutex
	pending []pendingIn
	wake    chan struct{}
	done    chan struct{}
}

func newSession(c net.Conn, dev Device) *session {
	return &session{c: c, dev: dev, wake: make(chan struct{}, 1), done: make(chan struct{})}
}

func (s *session) run() error {
	defer close(s.done)
	go s.pump()

	hdr := make([]byte, urbHeaderSize)
	for {
		if _, err := io.ReadFull(s.c, hdr); err != nil {
			return err
		}
		seq := be.Uint32(hdr[offURBSeqNum:])
		switch cmd := be.Uint32(hdr[offURBCommand:]); cmd {
		case cmdSubmit:
			dir, ep := be.Uint32(hdr[offURBDirection:]), be.Uint32(hdr[offURBEndpoint:])
			length := int(int32(be.Uint32(hdr[offURBLength:])))
			if length < 0 || length > maxTransferLength {
				return errTransferLength
			}
			var out []byte
			if dir == dirOut && length > 0 {
				out = make([]byte, length)
				if _, err := io.ReadFull(s.c, out); err != nil {
					return err
				}
			}
			setup := hdr[offURBSetup : offURBSetup+setupSize]
			if err := s.submit(seq, dir, ep, length, setup, out); err != nil {
				return err
			}
		case cmdUnlink:
			status := statusOK
			if s.cancel(be.Uint32(hdr[offURBUnlinkSeq:])) {
				status = statusConnReset
			}
			if err := s.reply(retUnlink, seq, status, 0, nil); err != nil {
				return err
			}
		default:
			return errors.New("unknown urb command")
		}
	}
}

func (s *session) submit(seq, dir, ep uint32, length int, setup, out []byte) error {
	if ep == controlEndpoint {
		le := binary.LittleEndian
		in, err := s.dev.Control(Setup{
			RequestType: setup[offSetupRequestType],
			Request:     setup[offSetupRequest],
			Value:       le.Uint16(setup[offSetupValue:]),
			Index:       le.Uint16(setup[offSetupIndex:]),
			Length:      le.Uint16(setup[offSetupLength:]),
		}, out)
		if err != nil {
			return s.reply(retSubmit, seq, statusStall, 0, nil)
		}
		if dir == dirOut {
			return s.reply(retSubmit, seq, statusOK, len(out), nil)
		}
		if len(in) > length {
			in = in[:length]
		}
		return s.reply(retSubmit, seq, statusOK, len(in), in)
	}
	if dir == dirOut {
		s.dev.Out(ep, out)
		return s.reply(retSubmit, seq, statusOK, len(out), nil)
	}
	s.mu.Lock()
	s.pending = append(s.pending, pendingIn{seq, length})
	s.mu.Unlock()
	select {
	case s.wake <- struct{}{}:
	default:
	}
	return nil
}

func (s *session) cancel(seq uint32) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, p := range s.pending {
		if p.seq == seq {
			s.pending = append(s.pending[:i], s.pending[i+1:]...)
			return true
		}
	}
	return false
}

// pump completes pending interrupt IN transfers with the device's reports.
func (s *session) pump() {
	for {
		var report []byte
		select {
		case <-s.done:
			return
		case report = <-s.dev.In():
		}
		p, ok := s.nextPending()
		if !ok {
			return
		}
		if len(report) > p.length {
			report = report[:p.length]
		}
		if err := s.reply(retSubmit, p.seq, statusOK, len(report), report); err != nil {
			return
		}
	}
}

func (s *session) nextPending() (pendingIn, bool) {
	for {
		s.mu.Lock()
		if len(s.pending) > 0 {
			p := s.pending[0]
			s.pending = s.pending[1:]
			s.mu.Unlock()
			return p, true
		}
		s.mu.Unlock()
		select {
		case <-s.done:
			return pendingIn{}, false
		case <-s.wake:
		}
	}
}

func (s *session) reply(cmd, seq uint32, status, actualLength int, data []byte) error {
	buf := make([]byte, urbHeaderSize, urbHeaderSize+len(data))
	be.PutUint32(buf[offURBCommand:], cmd)
	be.PutUint32(buf[offURBSeqNum:], seq)
	be.PutUint32(buf[offURBStatus:], uint32(int32(status)))
	if cmd == retSubmit {
		be.PutUint32(buf[offURBLength:], uint32(int32(actualLength)))
	}
	buf = append(buf, data...)
	s.wmu.Lock()
	defer s.wmu.Unlock()
	_, err := s.c.Write(buf)
	return err
}
