package ui

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"

	"stream-deck-plus-emulator/internal/deck"
	"stream-deck-plus-emulator/internal/link"
)

const testHost = "127.0.0.1:8484"

type fakeLinker struct {
	mu    sync.Mutex
	calls []string
	ch    chan link.Status
}

func (f *fakeLinker) record(call string) error {
	f.mu.Lock()
	f.calls = append(f.calls, call)
	f.mu.Unlock()
	return nil
}
func (f *fakeLinker) Connect(id string) error    { return f.record("connect " + id) }
func (f *fakeLinker) Disconnect(id string) error { return f.record("disconnect " + id) }
func (f *fakeLinker) Statuses() []link.Status {
	return []link.Status{
		{ID: "plus", State: link.Connected},
		{ID: "neo", State: link.Disconnected},
		{ID: "mk2", State: link.Failed, Code: link.CodeClientFailed, Detail: "boom"},
	}
}
func (f *fakeLinker) Subscribe() (<-chan link.Status, func()) { return f.ch, func() {} }

type rig struct {
	h       http.Handler
	devices map[string]*deck.Device
	linker  *fakeLinker
}

func newRig(hosts []string) *rig {
	r := &rig{devices: map[string]*deck.Device{}, linker: &fakeLinker{ch: make(chan link.Status, 1)}}
	var list []*deck.Device
	for _, m := range deck.Models() {
		d := deck.New(m)
		r.devices[m.ID] = d
		list = append(list, d)
	}
	r.h = Handler(list, r.linker, hosts)
	return r
}

func (r *rig) do(method, path, body string, headers ...string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "http://"+testHost+path, strings.NewReader(body))
	for i := 0; i+1 < len(headers); i += 2 {
		if headers[i] == "Host" {
			req.Host = headers[i+1]
		} else {
			req.Header.Set(headers[i], headers[i+1])
		}
	}
	rec := httptest.NewRecorder()
	r.h.ServeHTTP(rec, req)
	return rec
}

func errorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct{ Code string }
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body %q: %v", rec.Body.String(), err)
	}
	return body.Code
}

func keyImageReport(key int, payload []byte) []byte {
	r := make([]byte, 1024)
	r[0], r[1], r[2], r[3], r[4] = 0x02, 0x07, byte(key), 1, byte(len(payload))
	copy(r[8:], payload)
	return r
}

func TestIndex(t *testing.T) {
	rec := newRig([]string{testHost}).do("GET", "/", "")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `id="stage"`) {
		t.Errorf("status %d", rec.Code)
	}
}

func TestDevices(t *testing.T) {
	rec := newRig([]string{testHost}).do("GET", "/api/devices", "")
	var list []deviceInfo
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 {
		t.Fatalf("got %d devices", len(list))
	}
	plus, neo, mk2 := list[0], list[1], list[2]
	if plus.ID != "plus" || plus.Dials != 4 || plus.Strip == nil || plus.Strip.W != 800 || plus.Screen != nil || plus.Rotated {
		t.Errorf("plus: %+v", plus)
	}
	if neo.Screen == nil || neo.Screen.W != 248 || neo.TouchKeys != 2 || neo.Strip != nil || !neo.Rotated || neo.KeySize != 96 {
		t.Errorf("neo: %+v", neo)
	}
	if mk2.Rows != 3 || mk2.Cols != 5 || mk2.Link.State != link.Failed || mk2.Link.Code != link.CodeClientFailed || mk2.Link.Detail != "boom" {
		t.Errorf("mk2: %+v", mk2)
	}
	if plus.Link.State != link.Connected {
		t.Errorf("plus link: %+v", plus.Link)
	}
}

func TestKeyImage(t *testing.T) {
	r := newRig([]string{testHost})
	if rec := r.do("GET", "/api/devices/mk2/key/14", ""); rec.Code != http.StatusNoContent {
		t.Errorf("blank key: status %d", rec.Code)
	}
	r.devices["mk2"].Out(0, keyImageReport(14, []byte{0xff, 0xd8}))
	rec := r.do("GET", "/api/devices/mk2/key/14", "")
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "image/jpeg" || rec.Body.Len() != 2 {
		t.Errorf("status %d, type %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	for _, path := range []string{"/api/devices/mk2/key/15", "/api/devices/mk2/key/-1", "/api/devices/mk2/key/x"} {
		if rec := r.do("GET", path, ""); rec.Code != http.StatusNotFound {
			t.Errorf("%s: status %d", path, rec.Code)
		}
	}
}

func TestLocaleTraversal(t *testing.T) {
	r := newRig([]string{testHost})
	for _, path := range []string{"/locales/../x", "/locales/../server.go", "/locales/%2e%2e/server.go", "/locales/..%2fserver.go"} {
		rec := r.do("GET", path, "")
		for hops := 0; rec.Code == http.StatusMovedPermanently && hops < 3; hops++ {
			rec = r.do("GET", rec.Header().Get("Location"), "")
		}
		if rec.Code == http.StatusOK || strings.Contains(rec.Body.String(), "package ui") {
			t.Errorf("%s: status %d served %q", path, rec.Code, rec.Body.String())
		}
	}
}

func TestStripAndScreen(t *testing.T) {
	r := newRig([]string{testHost})
	if rec := r.do("GET", "/api/devices/plus/strip", ""); rec.Code != 200 || rec.Header().Get("Content-Type") != "image/png" {
		t.Errorf("plus strip: status %d", rec.Code)
	}
	if rec := r.do("GET", "/api/devices/neo/strip", ""); rec.Code != http.StatusNotFound {
		t.Errorf("neo strip: status %d", rec.Code)
	}
	if rec := r.do("GET", "/api/devices/neo/screen", ""); rec.Code != http.StatusNoContent {
		t.Errorf("blank neo screen: status %d", rec.Code)
	}
	if rec := r.do("GET", "/api/devices/plus/screen", ""); rec.Code != http.StatusNotFound {
		t.Errorf("plus screen: status %d", rec.Code)
	}
}

func TestUnknownDevice(t *testing.T) {
	r := newRig([]string{testHost})
	for _, req := range [][2]string{
		{"GET", "/api/devices/xl/key/0"},
		{"POST", "/api/devices/xl/input"},
		{"POST", "/api/devices/xl/connect"},
		{"POST", "/api/devices/xl/disconnect"},
	} {
		rec := r.do(req[0], req[1], "{}")
		if rec.Code != http.StatusNotFound || errorCode(t, rec) != "unknown_device" {
			t.Errorf("%s %s: status %d", req[0], req[1], rec.Code)
		}
	}
	if len(r.linker.calls) != 0 {
		t.Errorf("linker called for an unknown device: %v", r.linker.calls)
	}
}

func TestInput(t *testing.T) {
	r := newRig([]string{testHost})
	neo := r.devices["neo"]
	neo.SetAttached(true)
	rec := r.do("POST", "/api/devices/neo/input", `{"type":"key","index":9,"down":true}`)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if report := <-neo.In(); report[4+9] != 1 {
		t.Errorf("report %x", report[:16])
	}
}

func TestInputErrors(t *testing.T) {
	r := newRig([]string{testHost})
	for body, want := range map[string]string{
		`{"type":"dialPress","index":0,"down":true}`: "unsupported_input",
		`{"type":"key","index":10,"down":true}`:      "unsupported_input",
		`{"type":"touch","kind":1,"x":1,"y":1}`:      "unsupported_input",
		`{"type":"shake"}`:                           "bad_request",
		`{`:                                          "bad_request",
	} {
		rec := r.do("POST", "/api/devices/neo/input", body)
		if rec.Code != http.StatusBadRequest || errorCode(t, rec) != want {
			t.Errorf("%s: status %d, code %q", body, rec.Code, errorCode(t, rec))
		}
	}
}

func TestConnectDisconnect(t *testing.T) {
	r := newRig([]string{testHost})
	if rec := r.do("POST", "/api/devices/neo/connect", ""); rec.Code != http.StatusNoContent {
		t.Errorf("connect: status %d", rec.Code)
	}
	if rec := r.do("POST", "/api/devices/neo/disconnect", ""); rec.Code != http.StatusNoContent {
		t.Errorf("disconnect: status %d", rec.Code)
	}
	if want := []string{"connect neo", "disconnect neo"}; !reflect.DeepEqual(r.linker.calls, want) {
		t.Errorf("calls %v", r.linker.calls)
	}
	if rec := r.do("GET", "/api/devices/neo/connect", ""); rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET connect: status %d", rec.Code)
	}
}

func TestGuard(t *testing.T) {
	r := newRig([]string{testHost, "localhost:8484"})
	cases := []struct {
		name    string
		headers []string
		want    int
	}{
		{"no origin", nil, http.StatusNoContent},
		{"same origin", []string{"Origin", "http://" + testHost}, http.StatusNoContent},
		{"localhost alias", []string{"Host", "localhost:8484", "Origin", "http://localhost:8484"}, http.StatusNoContent},
		{"foreign origin", []string{"Origin", "http://evil.example"}, http.StatusForbidden},
		{"foreign host", []string{"Host", "evil.example:8484"}, http.StatusForbidden},
		{"rebound host with matching origin", []string{"Host", "evil.example:8484", "Origin", "http://evil.example:8484"}, http.StatusForbidden},
	}
	for _, c := range cases {
		rec := r.do("POST", "/api/devices/plus/connect", "", c.headers...)
		if rec.Code != c.want {
			t.Errorf("%s: status %d, want %d", c.name, rec.Code, c.want)
		}
		if c.want == http.StatusForbidden && errorCode(t, rec) != "forbidden" {
			t.Errorf("%s: code %q", c.name, errorCode(t, rec))
		}
	}
	if len(r.linker.calls) != 3 {
		t.Errorf("linker calls %v", r.linker.calls)
	}
}

func TestNoHostCheck(t *testing.T) {
	r := newRig(nil)
	rec := r.do("POST", "/api/devices/plus/connect", "", "Host", "192.168.1.5:8484", "Origin", "http://192.168.1.5:8484")
	if rec.Code != http.StatusNoContent {
		t.Errorf("status %d", rec.Code)
	}
	rec = r.do("POST", "/api/devices/plus/connect", "", "Host", "192.168.1.5:8484", "Origin", "http://evil.example")
	if rec.Code != http.StatusForbidden {
		t.Errorf("foreign origin: status %d", rec.Code)
	}
}

func TestLocaleNotFound(t *testing.T) {
	r := newRig([]string{testHost})
	if rec := r.do("GET", "/locales/en.json", ""); rec.Code != 200 || !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
		t.Errorf("en: status %d, type %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	for _, path := range []string{"/locales/de.json", "/locales/en.json.bak", "/static/missing.js"} {
		if rec := r.do("GET", path, ""); rec.Code != http.StatusNotFound {
			t.Errorf("%s: status %d", path, rec.Code)
		}
	}
}

func catalog(t *testing.T, lang string) map[string]string {
	t.Helper()
	data, err := assets.ReadFile("locales/" + lang + ".json")
	if err != nil {
		t.Fatal(err)
	}
	var c map[string]string
	if err := json.Unmarshal(data, &c); err != nil {
		t.Fatalf("%s: %v", lang, err)
	}
	return c
}

func keys(c map[string]string) []string {
	list := make([]string, 0, len(c))
	for k := range c {
		list = append(list, k)
	}
	sort.Strings(list)
	return list
}

func TestCatalogsMatch(t *testing.T) {
	en, ru := catalog(t, "en"), catalog(t, "ru")
	if !reflect.DeepEqual(keys(en), keys(ru)) {
		t.Errorf("key sets differ\nen: %v\nru: %v", keys(en), keys(ru))
	}
	for _, c := range []map[string]string{en, ru} {
		for k, v := range c {
			if strings.TrimSpace(v) == "" {
				t.Errorf("empty entry %q", k)
			}
		}
	}
}

func TestCodesTranslated(t *testing.T) {
	en := catalog(t, "en")
	for _, code := range append(link.Codes(), Codes()...) {
		if _, ok := en["error."+code]; !ok {
			t.Errorf("no catalog entry for code %q", code)
		}
	}
	for _, state := range []link.State{link.Disconnected, link.Connecting, link.Connected, link.Failed} {
		if _, ok := en["status."+string(state)]; !ok {
			t.Errorf("no catalog entry for state %q", state)
		}
	}
}

func TestEventsSnapshot(t *testing.T) {
	r := newRig(nil)
	srv := httptest.NewServer(r.h)
	defer srv.Close()
	res, err := http.Get(srv.URL + "/api/events")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if ct := res.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("content type %q", ct)
	}

	// 3 brightness + 2 Neo touch keys + 3 link statuses.
	var kinds []string
	var last message
	scanner := bufio.NewScanner(res.Body)
	for len(kinds) < 8 && scanner.Scan() {
		data, ok := strings.CutPrefix(scanner.Text(), "data: ")
		if !ok {
			continue
		}
		if err := json.Unmarshal([]byte(data), &last); err != nil {
			t.Fatal(err)
		}
		kinds = append(kinds, last.Device+":"+last.Kind)
	}
	want := []string{"plus:brightness", "neo:brightness", "neo:touchkey", "neo:touchkey", "mk2:brightness", "plus:link", "neo:link", "mk2:link"}
	if !reflect.DeepEqual(kinds, want) {
		t.Errorf("got %v", kinds)
	}
	if last.Link == nil || last.Link.Detail != "boom" {
		t.Errorf("last message %+v", last)
	}

	r.devices["plus"].Out(0, keyImageReport(2, []byte{1}))
	for scanner.Scan() {
		data, ok := strings.CutPrefix(scanner.Text(), "data: ")
		if !ok {
			continue
		}
		var m message
		json.Unmarshal([]byte(data), &m)
		if m.Device != "plus" || m.Kind != "key" || m.Index != 2 {
			t.Errorf("live event %+v", m)
		}
		break
	}
}
