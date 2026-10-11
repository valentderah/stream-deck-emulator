package link

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// TestMain lets the test binary stand in for the usbip client.
func TestMain(m *testing.M) {
	switch os.Getenv("LINK_TEST_CLIENT") {
	case "ok":
		os.Exit(0)
	case "fail":
		fmt.Fprintln(os.Stderr, "usbip: error: device busy")
		os.Exit(3)
	case "silent":
		os.Exit(4)
	}
	os.Exit(m.Run())
}

func TestCLIArgs(t *testing.T) {
	c := CLI{Path: "usbip", Host: "127.0.0.1", Port: "3240"}
	want := []string{"attach", "--remote", "127.0.0.1", "--bus-id", "1-2", "--once"}
	if got := c.args("1-2", "--once"); !reflect.DeepEqual(got, want) {
		t.Errorf("got %v", got)
	}
	c.Port = "3241"
	want = []string{"--tcp-port", "3241", "attach", "--remote", "127.0.0.1", "--bus-id", "1-2", "--stop"}
	if got := c.args("1-2", "--stop"); !reflect.DeepEqual(got, want) {
		t.Errorf("custom port: got %v", got)
	}
}

func TestCLICommand(t *testing.T) {
	c := CLI{Host: "127.0.0.1", Port: "3240"}
	if got := c.Command("1-1"); got != "usbip attach --remote 127.0.0.1 --bus-id 1-1" {
		t.Errorf("got %q", got)
	}
}

func TestCLIAvailable(t *testing.T) {
	if (CLI{}).Available() {
		t.Error("empty path is available")
	}
	if !(CLI{Path: "x"}).Available() {
		t.Error("path set but not available")
	}
}

func TestCLIInstallURL(t *testing.T) {
	if got := (CLI{}).InstallURL(); (got != "") != (runtime.GOOS == "windows") {
		t.Errorf("got %q on %s", got, runtime.GOOS)
	}
}

func TestFindCLIExplicit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "my usbip.exe")
	if got := FindCLI(path); got != path {
		t.Errorf("got %q", got)
	}
}

func TestCLIRun(t *testing.T) {
	c := CLI{Path: os.Args[0], Host: "127.0.0.1", Port: "3240"}
	ctx := context.Background()

	t.Setenv("LINK_TEST_CLIENT", "ok")
	if err := c.Attach(ctx, "1-1"); err != nil {
		t.Errorf("ok client: %v", err)
	}

	t.Setenv("LINK_TEST_CLIENT", "fail")
	err := c.Attach(ctx, "1-1")
	if err == nil || err.Error() != "usbip: error: device busy" {
		t.Errorf("failing client: %v", err)
	}

	t.Setenv("LINK_TEST_CLIENT", "silent")
	if err := c.Stop(ctx, "1-1"); err == nil || !strings.Contains(err.Error(), "exit status 4") {
		t.Errorf("silent failure: %v", err)
	}
}
