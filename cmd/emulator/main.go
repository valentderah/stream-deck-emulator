// emulator exposes virtual Stream Deck devices over USB/IP and shows them in
// a browser.
package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"strings"
	"time"

	"stream-deck-plus-emulator/internal/deck"
	"stream-deck-plus-emulator/internal/link"
	"stream-deck-plus-emulator/internal/ui"
	"stream-deck-plus-emulator/internal/usbip"
)

const (
	attachTimeout = 15 * time.Second
	// httpPort is the port browsers leave out of the Host header.
	httpPort = "80"
)

func main() {
	models := deck.Models()
	ids := make([]string, len(models))
	for i, m := range models {
		ids[i] = m.ID
	}

	usbipAddr := flag.String("usbip", "127.0.0.1:3240", "USB/IP listen address")
	uiAddr := flag.String("ui", "127.0.0.1:8484", "web UI listen address")
	cliPath := flag.String("usbip-cli", "", "path of the usbip client (default: PATH, then the usbip-win2 install directory)")
	connect := flag.String("connect", "", "comma-separated models to connect at start: "+strings.Join(ids, ", "))
	flag.Parse()

	auto, err := parseConnect(*connect, ids)
	if err != nil {
		log.Fatal(err)
	}
	if *cliPath != "" {
		if _, err := os.Stat(*cliPath); err != nil {
			log.Fatalf("-usbip-cli: %v", err)
		}
	}

	usbipLn, err := net.Listen("tcp", *usbipAddr)
	if err != nil {
		log.Fatalf("listen for USB/IP: %v", err)
	}
	uiLn, err := net.Listen("tcp", *uiAddr)
	if err != nil {
		log.Fatalf("listen for the UI: %v", err)
	}
	// Derive everything from the bound addresses: a flag may ask for port 0.
	usbipBound, uiBound := usbipLn.Addr().String(), uiLn.Addr().String()
	host, port, err := clientEndpoint(usbipBound)
	if err != nil {
		log.Fatalf("-usbip: %v", err)
	}
	client := link.CLI{Path: link.FindCLI(*cliPath), Host: host, Port: port}

	srv := usbip.NewServer()
	var devices []*deck.Device
	var targets []link.Target
	for i, m := range models {
		dev := deck.New(m)
		devices = append(devices, dev)
		targets = append(targets, link.Target{ID: m.ID, BusID: fmt.Sprintf("1-%d", i+1), Dev: dev})
	}
	mgr := link.New(srv, client, attachTimeout, targets)
	srv.OnSession = mgr.SessionChanged

	go func() { log.Fatal(srv.Serve(usbipLn)) }()
	go func() { log.Fatal(http.Serve(uiLn, ui.Handler(devices, mgr, uiHosts(uiBound)))) }()

	log.Printf("USB/IP on %s", usbipBound)
	log.Printf("UI on http://%s", uiBound)
	if client.Available() {
		log.Printf("usbip client: %s", client.Path)
	} else {
		log.Print("usbip client not found: attach devices by hand, the UI shows the command")
	}
	for _, id := range auto {
		mgr.Connect(id)
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	<-stop
	go func() {
		<-stop
		log.Print("second interrupt, exiting without detaching")
		os.Exit(1)
	}()
	// Detach cleanly so the host does not keep stale virtual devices.
	for _, id := range ids {
		mgr.Disconnect(id)
	}
}

// parseConnect splits the -connect value into model ids, dropping blanks
// and repeats.
func parseConnect(s string, known []string) ([]string, error) {
	var ids []string
	for _, id := range strings.Split(s, ",") {
		id = strings.TrimSpace(id)
		if id == "" || slices.Contains(ids, id) {
			continue
		}
		if !slices.Contains(known, id) {
			return nil, fmt.Errorf("-connect: unknown model %q, valid models: %s", id, strings.Join(known, ", "))
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// clientEndpoint returns where the local usbip client reaches a server
// listening on listen.
func clientEndpoint(listen string) (host, port string, err error) {
	host, port, err = net.SplitHostPort(listen)
	if err != nil {
		return "", "", err
	}
	if ip := net.ParseIP(host); host == "" || (ip != nil && ip.IsUnspecified()) {
		host = "127.0.0.1"
	}
	return host, port, nil
}

// uiHosts returns the Host header values to accept for a UI bound to
// listen, which should be the bound address. The port-less form is accepted
// for port 80. A UI bound to a loopback address answers only to loopback names;
// one bound elsewhere is reachable under names the emulator cannot know, so
// the result is nil and the host check is off.
func uiHosts(listen string) []string {
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		return nil
	}
	ip := net.ParseIP(host)
	if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return nil
	}
	hosts := []string{listen}
	for _, name := range []string{"localhost", "127.0.0.1"} {
		if alias := net.JoinHostPort(name, port); !slices.Contains(hosts, alias) {
			hosts = append(hosts, alias)
		}
	}
	if port == httpPort {
		for _, h := range slices.Clone(hosts) {
			name, _, _ := net.SplitHostPort(h)
			if strings.Contains(name, ":") {
				name = "[" + name + "]"
			}
			hosts = append(hosts, name)
		}
	}
	return hosts
}
