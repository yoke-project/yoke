package interfaces_test

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	interfacev1 "github.com/yoke-project/yoke/proto/yoke/interface/v1"
	pluginv1 "github.com/yoke-project/yoke/proto/yoke/plugin/v1"

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
	sessions := &fakeSessions{}
	unbind, err := interfaces.BindNeed(path, interfaces.NewSurface(interfaces.Config{
		Channel: gate.Channel{Name: "need:archive/source", Transport: "local", Clients: "single"},
		Bus:     w.bus, Publish: w.publish, Units: bench{}, Sessions: sessions, Transports: transports, Wait: 200 * time.Millisecond,
		Declared: func(id string) (*gate.Manifest, bool) { return manifest, benchUnits[id].kind == unit.Plugin },
		Use:      &interfaces.Use{Unit: "archive", Need: "source", Bound: "acquire", Questions: []string{"head-status"}, Streams: []string{"station.spectra"}},
		UseGranted: func(u, need, of, id string) bool {
			return u == "archive" && need == "source" && of == "query" && id == "head-status"
		},
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
	sessions.mu.Lock()
	if len(sessions.asked) != 1 || sessions.asked[0].GetAsker() != "unit:archive" {
		t.Errorf("the unit was asked %v, want it asked by unit:archive", sessions.asked)
	}
	sessions.mu.Unlock()
	for call, c := range map[string]struct {
		r           *interfacev1.Request
		code, names string
	}{
		"idle":    {ask("idle"), "subject.unknown", "idle"},
		"command": {command("acquire", "calibrate", nil), "use.command", "calibrate"},
		"spectra": {&interfacev1.Request{Version: 1, Operation: &interfacev1.Request_StreamStart{StreamStart: &interfacev1.UnitStream{Unit: "acquire", Stream: "station.spectra"}}}, "scope.withheld", "station.spectra"},
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

// std: yoke:a-need-s-channel.02
func TestACausedQuestionTakesItsChainAndDeadline(t *testing.T) {
	w := newWorld()
	r := root(t)
	transports := streams.New(streams.Config{Root: r, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Publish: w.publish})
	manifest := &gate.Manifest{ID: "com.example.station", Commands: []string{"calibrate"}, Queries: []string{"head-status"}}
	now := time.Now()
	outstanding := map[string]struct {
		chain    string
		deadline time.Time
	}{"c-1": {"c0ffee", now.Add(10 * time.Second)}, "c-2": {"beef", now.Add(550 * time.Millisecond)}}
	sessions := &fakeSessions{}
	config := func(ch gate.Channel, use *interfaces.Use) interfaces.Config {
		return interfaces.Config{Channel: ch, Bus: w.bus, Publish: w.publish, Units: bench{}, Sessions: sessions, Transports: transports,
			Declared: func(id string) (*gate.Manifest, bool) { return manifest, benchUnits[id].kind == unit.Plugin },
			Use:      use, UseGranted: func(u, need, of, id string) bool { return id == "head-status" },
			Outstanding: func(u, id string) (string, uint64, bool) {
				o, ok := outstanding[id]
				return o.chain, uint64(o.deadline.UnixMilli()), ok && u == "archive"
			}}
	}
	path := filepath.Join(r, "needs", "archive", "source.sock")
	unbind, err := interfaces.BindNeed(path, interfaces.NewSurface(config(gate.Channel{Name: "need:archive/source", Transport: "local", Clients: "single"},
		&interfaces.Use{Unit: "archive", Need: "source", Bound: "acquire", Questions: []string{"head-status"}})))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(unbind)
	b, err := interfaces.BindServing(r, "", mode, []gate.Channel{{Name: "panel", Transport: "local", Clients: "single"}}, func(ch gate.Channel) interfacev1.InterfaceServer {
		return interfaces.NewSurface(config(ch, nil))
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { b.Close() })
	need, err := attachTo(t, path)
	if err != nil {
		t.Fatal(err)
	}
	need.next(t)
	panel, err := attachTo(t, b.Address("panel"))
	if err != nil {
		t.Fatal(err)
	}
	panel.next(t)
	ask := func(cause string) *interfacev1.Request {
		return &interfacev1.Request{Version: 1, Operation: &interfacev1.Request_Query{Query: &interfacev1.Question{Unit: "acquire", Type: "head-status", Payload: []byte("secret"), Cause: cause}}}
	}
	lastAsked := func() *pluginv1.Query_Question {
		sessions.mu.Lock()
		defer sessions.mu.Unlock()
		if len(sessions.asked) == 0 {
			return nil
		}
		return sessions.asked[len(sessions.asked)-1]
	}
	if f := need.answerTo(t, "1", ask("c-1")); f.GetAnswer().GetQuery() == nil {
		t.Fatalf("the caused question was answered %v", f)
	}
	q := lastAsked()
	if want := uint64(now.Add(10*time.Second - interfaces.CausedMargin).UnixMilli()); q.GetChain() != "c0ffee" || q.GetDeadline() != want {
		t.Errorf("the unit was asked %v, want chain c0ffee and deadline %d", q, want)
	}
	chained := w.of("unit.question.chained")
	if len(chained) != 1 || chained[0].Subject.ID != "acquire" || strings.Contains(string(chained[0].Detail), "secret") ||
		!strings.Contains(string(chained[0].Detail), "c0ffee") || !strings.Contains(string(chained[0].Detail), "unit:archive") {
		t.Errorf("the chained events are %+v", chained)
	}
	before := len(sessions.asked)
	for call, want := range map[string]string{"9": "use.cause.unknown", "2": "deadline.exhausted"} {
		if ref := need.answerTo(t, call, ask("c-"+call)).GetRefusal(); ref.GetCode() != want {
			t.Errorf("a question caused by c-%s was answered %v, want %s", call, ref, want)
		}
	}
	if len(sessions.asked) != before {
		t.Errorf("a refused caused question reached the unit")
	}
	if ref := need.answerTo(t, "c", command("acquire", "calibrate", nil)).GetRefusal(); ref.GetCode() != "use.command" {
		t.Errorf("the command was answered %v", ref)
	}
	refused := w.of("unit.use.refused")
	if len(refused) != 1 || refused[0].Subject.ID != "archive" || !strings.Contains(string(refused[0].Detail), "use.command") || !strings.Contains(string(refused[0].Detail), "calibrate") {
		t.Errorf("the refused uses are %+v", refused)
	}
	if f := panel.answerTo(t, "p", ask("")); f.GetAnswer().GetQuery() == nil {
		t.Fatalf("panel's question was answered %v", f)
	}
	q = lastAsked()
	ahead := time.Until(time.UnixMilli(int64(q.GetDeadline())))
	if len(q.GetChain()) != 32 || q.GetChain() == "c0ffee" || ahead < 25*time.Second || ahead > 31*time.Second {
		t.Errorf("panel's question carried %v", q)
	}
	if ref := panel.answerTo(t, "pc", ask("c-1")).GetRefusal(); ref.GetCode() != "use.cause.unknown" {
		t.Errorf("a cause on a declared channel was answered %v", ref)
	}
}
