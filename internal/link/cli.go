package link

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

const (
	defaultPort = "3240"
	// win2URL is where to download usbip-win2, the client for Windows.
	win2URL = "https://github.com/vadimgrn/usbip-win2/releases"
)

// installPaths are where the usbip-win2 installer puts the client.
var installPaths = []string{`C:\Program Files\USBip\usbip.exe`}

// CLI drives the usbip command-line client. Host and Port are where the
// client reaches the emulator's USB/IP server.
type CLI struct {
	Path, Host, Port string
}

// FindCLI returns the path of the usbip client: explicit if set, else the
// one on PATH, else a known install location. It returns "" if there is none.
func FindCLI(explicit string) string {
	if explicit != "" {
		return explicit
	}
	if path, err := exec.LookPath("usbip"); err == nil {
		return path
	}
	for _, path := range installPaths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

func (c CLI) Available() bool { return c.Path != "" }

func (c CLI) InstallURL() string {
	if runtime.GOOS == "windows" {
		return win2URL
	}
	return ""
}

func (c CLI) Attach(ctx context.Context, busID string) error {
	return c.run(ctx, c.args(busID, "--once")...)
}

func (c CLI) Stop(ctx context.Context, busID string) error {
	return c.run(ctx, c.args(busID, "--stop")...)
}

func (c CLI) Command(busID string) string {
	return "usbip " + strings.Join(c.args(busID), " ")
}

// args builds an attach command line. The port is a global option and must
// come before the subcommand.
func (c CLI) args(busID string, extra ...string) []string {
	var args []string
	if c.Port != defaultPort {
		args = append(args, "--tcp-port", c.Port)
	}
	args = append(args, "attach", "--remote", c.Host, "--bus-id", busID)
	return append(args, extra...)
}

func (c CLI) run(ctx context.Context, args ...string) error {
	out, err := exec.CommandContext(ctx, c.Path, args...).CombinedOutput()
	if err == nil {
		return nil
	}
	if msg := strings.TrimSpace(string(out)); msg != "" {
		return errors.New(msg)
	}
	return err
}
