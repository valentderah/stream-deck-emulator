package deck

import (
	"bytes"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

var update = flag.Bool("update", false, "rewrite golden files")

func checkGolden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s differs from the golden file; review the change, then rerun with -update\ngot:\n%s", name, got)
	}
}

func dumpPlus() []byte {
	d := New(Plus)
	d.SetAttached(true)
	var b bytes.Buffer
	line := func(name string, data []byte) {
		fmt.Fprintf(&b, "%s %s\n", name, hex.EncodeToString(data))
	}
	fmt.Fprintf(&b, "info %+v\n", d.Info())
	line("device", d.model.deviceDescriptor())
	line("config", configDescriptor())
	line("hid", hidDescriptor())
	line("report", reportDescriptor)
	for i := uint8(stringLangIDs); i <= stringSerial; i++ {
		s, _ := d.model.stringDescriptor(i)
		line(fmt.Sprintf("string%d", i), s)
	}
	for id := byte(featureFirst); id <= featureLast; id++ {
		r, _ := d.featureReport(id)
		line(fmt.Sprintf("feature%02x", id), r)
	}
	next := func(name string) { line(name, <-d.In()) }
	d.SetKey(3, true)
	next("key3down")
	d.SetKey(3, false)
	next("key3up")
	d.PressDial(1, true)
	next("dial1down")
	d.PressDial(1, false)
	next("dial1up")
	d.RotateDial(2, -1)
	next("dial2left")
	d.Touch(TouchShort, 10, 20, 0, 0)
	next("touchshort")
	d.Touch(TouchLong, 799, 99, 0, 0)
	next("touchlong")
	d.Touch(TouchDrag, 10, 20, 300, 40)
	next("touchdrag")
	return b.Bytes()
}

func TestPlusGolden(t *testing.T) {
	checkGolden(t, "plus.golden", dumpPlus())
}
