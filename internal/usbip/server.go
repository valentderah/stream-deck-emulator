// Package usbip implements the device side of the USB/IP protocol (v1.1.1)
// for emulated devices with one interrupt IN endpoint each.
package usbip

import (
	"encoding/binary"
	"errors"
	"io"
	"log"
	"net"
	"sort"
	"sync"
)

// SpeedHigh is USB_SPEED_HIGH.
const SpeedHigh = 3

// ErrStall makes a control transfer fail with a STALL.
var ErrStall = errors.New("usbip: stall")

// ErrExported is returned by Export for a bus id that is already exported.
var ErrExported = errors.New("usbip: bus id already exported")

var be = binary.BigEndian

// Setup is a decoded USB control setup packet.
type Setup struct {
	RequestType uint8
	Request     uint8
	Value       uint16
	Index       uint16
	Length      uint16
}

type Interface struct {
	Class, SubClass, Protocol uint8
}

type Info struct {
	Speed                         uint32
	VendorID, ProductID           uint16
	BCDDevice                     uint16
	Class, SubClass, Protocol     uint8
	ConfigurationValue, NumConfig uint8
	Interfaces                    []Interface
}

// Device is an emulated USB device.
type Device interface {
	Info() Info
	// Control handles a transfer on endpoint 0. out is the data stage of a
	// host-to-device request; the result is the data stage of a
	// device-to-host one.
	Control(s Setup, out []byte) ([]byte, error)
	// Out handles data written to an OUT endpoint.
	Out(ep uint32, data []byte)
	// In yields the reports to deliver on the interrupt IN endpoint.
	In() <-chan []byte
	// SetAttached is called when a client session starts and ends.
	SetAttached(attached bool)
}

type export struct {
	busID  string
	devNum uint32
	dev    Device

	conn net.Conn      // the attached client, if any
	done chan struct{} // closed when that client's session has ended
}

// Server exports devices to USB/IP clients. A device is visible to clients
// from Export until Unexport.
type Server struct {
	// OnSession, if set, is called when a client attaches to or detaches
	// from an exported device. Set it before Serve.
	OnSession func(busID string, attached bool)

	mu      sync.Mutex
	exports map[string]*export
	devNums map[string]uint32
}

func NewServer() *Server {
	return &Server{exports: map[string]*export{}, devNums: map[string]uint32{}}
}

// Export makes dev attachable under busID.
func (s *Server) Export(busID string, dev Device) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.exports[busID]; ok {
		return ErrExported
	}
	devNum, ok := s.devNums[busID]
	if !ok {
		devNum = firstDevNum + uint32(len(s.devNums))
		s.devNums[busID] = devNum
	}
	s.exports[busID] = &export{busID: busID, devNum: devNum, dev: dev}
	return nil
}

// Unexport hides the device and ends its session, which makes the client
// drop the virtual device. It returns once the session is gone.
func (s *Server) Unexport(busID string) {
	s.mu.Lock()
	e := s.exports[busID]
	delete(s.exports, busID)
	var conn net.Conn
	var done chan struct{}
	if e != nil {
		conn, done = e.conn, e.done
	}
	s.mu.Unlock()
	if conn != nil {
		conn.Close()
		<-done
	}
}

// Serve accepts USB/IP clients on ln until it is closed.
func (s *Server) Serve(ln net.Listener) error {
	for {
		c, err := ln.Accept()
		if err != nil {
			return err
		}
		go func() {
			defer c.Close()
			if err := s.handle(c); err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, net.ErrClosed) {
				log.Printf("usbip: %s: %v", c.RemoteAddr(), err)
			}
		}()
	}
}

func (s *Server) handle(c net.Conn) error {
	var op [opHeaderSize]byte
	if _, err := io.ReadFull(c, op[:]); err != nil {
		return err
	}
	switch code := be.Uint16(op[offOpCode:]); code {
	case opReqDevlist:
		exports := s.snapshot()
		buf := opHeader(opRepDevlist, opStatusOK)
		buf = be.AppendUint32(buf, uint32(len(exports)))
		for _, e := range exports {
			info := e.dev.Info()
			buf = appendDevice(buf, e, info)
			for _, i := range info.Interfaces {
				buf = append(buf, i.Class, i.SubClass, i.Protocol, 0) // padding
			}
		}
		_, err := c.Write(buf)
		return err
	case opReqImport:
		var busID [busIDSize]byte
		if _, err := io.ReadFull(c, busID[:]); err != nil {
			return err
		}
		e := s.claim(cString(busID[:]), c)
		if e == nil {
			_, err := c.Write(opHeader(opRepImport, opStatusError))
			return err
		}
		defer s.release(e)
		if _, err := c.Write(appendDevice(opHeader(opRepImport, opStatusOK), e, e.dev.Info())); err != nil {
			return err
		}
		log.Printf("usbip: %s attached %s", c.RemoteAddr(), e.busID)
		err := newSession(c, e.dev).run()
		log.Printf("usbip: %s detached %s", c.RemoteAddr(), e.busID)
		return err
	default:
		log.Printf("usbip: unknown op %#04x", code)
		return nil
	}
}

// snapshot returns the exports ordered by bus id.
func (s *Server) snapshot() []*export {
	s.mu.Lock()
	defer s.mu.Unlock()
	list := make([]*export, 0, len(s.exports))
	for _, e := range s.exports {
		list = append(list, e)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].busID < list[j].busID })
	return list
}

// claim binds c to the export of busID, or returns nil if there is none or
// it already has a client.
func (s *Server) claim(busID string, c net.Conn) *export {
	s.mu.Lock()
	e := s.exports[busID]
	if e == nil || e.conn != nil {
		s.mu.Unlock()
		return nil
	}
	e.conn, e.done = c, make(chan struct{})
	s.mu.Unlock()
	e.dev.SetAttached(true)
	s.notify(e.busID, true)
	return e
}

func (s *Server) release(e *export) {
	s.mu.Lock()
	done := e.done
	e.conn, e.done = nil, nil
	s.mu.Unlock()
	e.dev.SetAttached(false)
	s.notify(e.busID, false)
	close(done)
}

func (s *Server) notify(busID string, attached bool) {
	if s.OnSession != nil {
		s.OnSession(busID, attached)
	}
}

func opHeader(code uint16, status uint32) []byte {
	buf := be.AppendUint16(nil, protocolVersion)
	buf = be.AppendUint16(buf, code)
	return be.AppendUint32(buf, status)
}

// appendDevice appends the device record used by both handshake replies.
func appendDevice(buf []byte, e *export, info Info) []byte {
	var path [pathSize]byte
	copy(path[:], sysPathPrefix+e.busID)
	var busID [busIDSize]byte
	copy(busID[:], e.busID)
	buf = append(buf, path[:]...)
	buf = append(buf, busID[:]...)
	buf = be.AppendUint32(buf, busNum)
	buf = be.AppendUint32(buf, e.devNum)
	buf = be.AppendUint32(buf, info.Speed)
	buf = be.AppendUint16(buf, info.VendorID)
	buf = be.AppendUint16(buf, info.ProductID)
	buf = be.AppendUint16(buf, info.BCDDevice)
	return append(buf, info.Class, info.SubClass, info.Protocol,
		info.ConfigurationValue, info.NumConfig, uint8(len(info.Interfaces)))
}

func cString(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}
