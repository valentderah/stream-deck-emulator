package main

import (
	"net"
	"reflect"
	"slices"
	"strings"
	"testing"
)

var known = []string{"plus", "neo", "mk2"}

func TestParseConnect(t *testing.T) {
	for in, want := range map[string][]string{
		"":              nil,
		"plus":          {"plus"},
		"neo, plus":     {"neo", "plus"},
		"mk2,,mk2,plus": {"mk2", "plus"},
	} {
		got, err := parseConnect(in, known)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("%q: got %v, %v", in, got, err)
		}
	}
	_, err := parseConnect("plus,xl", known)
	if err == nil {
		t.Fatal("unknown model accepted")
	}
	for _, part := range []string{`"xl"`, "plus, neo, mk2"} {
		if !strings.Contains(err.Error(), part) {
			t.Errorf("error %q does not mention %s", err, part)
		}
	}
}

func TestClientEndpoint(t *testing.T) {
	for in, want := range map[string][2]string{
		"127.0.0.1:3240":   {"127.0.0.1", "3240"},
		":3241":            {"127.0.0.1", "3241"},
		"0.0.0.0:3240":     {"127.0.0.1", "3240"},
		"[::]:3240":        {"127.0.0.1", "3240"},
		"192.168.1.5:3240": {"192.168.1.5", "3240"},
	} {
		host, port, err := clientEndpoint(in)
		if err != nil || host != want[0] || port != want[1] {
			t.Errorf("%q: got %q %q %v", in, host, port, err)
		}
	}
	if _, _, err := clientEndpoint("nonsense"); err == nil {
		t.Error("address without a port accepted")
	}
}

func TestUIHosts(t *testing.T) {
	want := []string{"127.0.0.1:8484", "localhost:8484"}
	if got := uiHosts("127.0.0.1:8484"); !reflect.DeepEqual(got, want) {
		t.Errorf("loopback: got %v", got)
	}
	if got := uiHosts("localhost:9000"); !reflect.DeepEqual(got, []string{"localhost:9000", "127.0.0.1:9000"}) {
		t.Errorf("localhost: got %v", got)
	}
	wantBare := []string{"127.0.0.1:80", "localhost:80", "127.0.0.1", "localhost"}
	if got := uiHosts("127.0.0.1:80"); !reflect.DeepEqual(got, wantBare) {
		t.Errorf("port 80: got %v", got)
	}
	if got := uiHosts("[::1]:80"); !reflect.DeepEqual(got, []string{"[::1]:80", "localhost:80", "127.0.0.1:80", "[::1]", "localhost", "127.0.0.1"}) {
		t.Errorf("ipv6 port 80: got %v", got)
	}
	if got := uiHosts("127.0.0.1:8484"); slices.Contains(got, "127.0.0.1") {
		t.Errorf("port-less host accepted off port 80: %v", got)
	}
	for _, addr := range []string{":8484", "0.0.0.0:8484", "192.168.1.5:8484"} {
		if got := uiHosts(addr); got != nil {
			t.Errorf("%s: got %v, want no host check", addr, got)
		}
	}
}

// A flag with port 0 must yield the address the listener really got.
func TestBoundAddresses(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	addr := ln.Addr().String()
	_, port, _ := net.SplitHostPort(addr)
	if port == "0" {
		t.Fatalf("listener address %q has no port", addr)
	}
	hosts := uiHosts(addr)
	if !slices.Contains(hosts, addr) || !slices.Contains(hosts, "localhost:"+port) {
		t.Errorf("hosts %v miss the bound port %s", hosts, port)
	}
	host, got, err := clientEndpoint(addr)
	if err != nil || host != "127.0.0.1" || got != port {
		t.Errorf("endpoint %q %q %v", host, got, err)
	}
}
