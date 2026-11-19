package interfaces_test

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	interfacev1 "github.com/yoke-project/yoke/proto/yoke/interface/v1"

	"github.com/yoke-project/yoke/internal/core/interfaces"
	"github.com/yoke-project/yoke/internal/core/streams"
	"github.com/yoke-project/yoke/internal/core/unit"
	"github.com/yoke-project/yoke/internal/gate"
)

// std: yoke:a-need-s-channel.01
func TestANeedsChannelAnswersForTheUnitBoundAndWhatTheNeedUses(t *testing.T) {
	w := newWorld()
	r := root(t)
	transports := streams.New(streams.Config{Root: r, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Publish: w.publish})
	t.Cleanup(func() { transports.CloseAll("acquire", streams.UnitExited) })
	manifest := &gate.Manifest{ID: "com.example.station", Commands: []string{"calibrate"}, Queries: []string{"head-status"},
		Streams: []gate.Stream{{ID: "station.spectra"}, {ID: "station.preview", ToleratesLoss: true}}}
	path := filepath.Join(r, "needs", "archive", "source.sock")
	unbind, err := interfaces.BindNeed(path, interfaces.NewSurface(interfaces.Config{
		Channel: gate.Channel{Name: "need:archive/source", Transport: "local", Clients: "single"},
		Bus:     w.bus, Publish: w.publish, Units: bench{}, Sessions: &fakeSessions{}, Transports: transports, Wait: 200 * time.Millisecond,
		Declared: func(id string) (*gate.Manifest, bool) { return manifest, benchUnits[id].kind == unit.Plugin },
		Use:      &interfaces.Use{Unit: "archive", Need: "source", Bound: "acquire", Questions: []string{"head-status"}, Streams: []string{"station.spectra"}},
	}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(unbind)
	for p, want := range map[string]os.FileMode{path: 0o600, filepath.Dir(path): 0o700} {
		if info, err := os.Stat(p); err != nil || info.Mode().Perm() != want {
			t.Errorf("%s is %v, want %o", p, info, want)
		}
	}
	a, err := attachTo(t, path)
	if err != nil {
		t.Fatal(err)
	}
	a.next(t)
	ask := func(unitID string) *interfacev1.Request {
		return &interfacev1.Request{Version: 1, Operation: &interfacev1.Request_Query{Query: &interfacev1.Question{Unit: unitID, Type: "head-status", Payload: []byte{1, 2}}}}
	}
	if f := a.answerTo(t, "q", ask("acquire")); f.GetAnswer().GetQuery() == nil {
		t.Errorf("the question of acquire was answered %v", f)
	}
	for call, c := range map[string]struct {
		r           *interfacev1.Request
		code, names string
	}{
		"idle":    {ask("idle"), "subject.unknown", "idle"},
		"command": {command("acquire", "calibrate", nil), "use.command", "calibrate"},
		"preview": {&interfacev1.Request{Version: 1, Operation: &interfacev1.Request_StreamStart{StreamStart: &interfacev1.UnitStream{Unit: "acquire", Stream: "station.preview"}}}, "use.undeclared", "station.preview"},
	} {
		ref := a.answerTo(t, call, c.r).GetRefusal()
		if ref.GetCode() != c.code || ref.GetItem()+ref.GetSubject().GetIdentity() != c.names {
			t.Errorf("%s was refused %v, want %s naming %s", call, ref, c.code, c.names)
		}
	}
	read := a.answerTo(t, "read", &interfacev1.Request{Version: 1, Operation: &interfacev1.Request_Read{Read: &interfacev1.Read{Kind: "unit"}}})
	if records := read.GetAnswer().GetRead().GetRecords(); len(records) != 1 || records[0].GetUnit().GetDeclared().GetIdentity() != "acquire" {
		t.Errorf("the units read are %v, want acquire alone", records)
	}
	authenticate := &interfacev1.Request{Version: 1, Operation: &interfacev1.Request_Authenticate{Authenticate: &interfacev1.Authenticate{}}}
	if ref := a.answerTo(t, "auth", authenticate).GetRefusal(); ref.GetCode() != "operation.unknown" {
		t.Errorf("authenticate on a need's channel was answered %v", ref)
	}
}
