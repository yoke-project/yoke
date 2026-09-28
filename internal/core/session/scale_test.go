package session_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	pluginv1 "github.com/yoke-project/yoke/proto/yoke/plugin/v1"

	"github.com/yoke-project/yoke/internal/core/event"
	"github.com/yoke-project/yoke/internal/core/scope"
)

// std: yoke:the-fifth-list-and-the-scale.03
func TestAnOccurrenceIsPublishedOnTheUnitsLife(t *testing.T) {
	h := newHarness(t)
	h.incarnation = 3
	st := openedWith(t, h, scopeOf(scoped(scope.Occurrence, "calibration.drift+")))
	st.send(t, func(e *pluginv1.Envelope) {
		e.Payload = &pluginv1.Envelope_Event{Event: &pluginv1.Event{Occurrence: "calibration.drift", Severity: 70, Line: "drifting", Detail: []byte{0xff, 1}}}
	})
	st.quiet(t, 150*time.Millisecond)
	got := h.publishedEvents()
	if len(got) != 1 {
		t.Fatalf("the Core published %d events", len(got))
	}
	e := got[0]
	if e.Type != "unit.occurrence.reported" || e.Subject != event.UnitSubject("acquire", 3) || e.Occurrence != "calibration.drift" ||
		e.Severity != 70 || e.Line != "drifting" || !bytes.Equal(e.Detail, []byte{0xff, 1}) || e.Actor.Class != event.ByUnit {
		t.Errorf("the Core published %+v", e)
	}
}

// std: yoke:the-fifth-list-and-the-scale.04
func TestASeverityAboveTheScaleIsReadAsItsTop(t *testing.T) {
	h := newHarness(t)
	st := openedWith(t, h, scopeOf(scoped(scope.Occurrence, "calibration.drift+")))
	st.send(t, report("calibration.drift", 150))
	st.quiet(t, 150*time.Millisecond)
	got := h.publishedEvents()
	if len(got) != 1 || got[0].Severity != 99 {
		t.Fatalf("the Core published %+v", got)
	}
	if log := h.log.String(); !strings.Contains(log, "level=WARN") || !strings.Contains(log, "unit=acquire") || !strings.Contains(log, "150") {
		t.Errorf("no warning names the value declared:\n%s", log)
	}
}

// std: yoke:the-fifth-list-and-the-scale.05
func TestAHealthGradeAboveTheScaleIsReadAsItsTop(t *testing.T) {
	h := newHarness(t)
	h.admitScoped("sid-1", "acquire", 100*time.Millisecond, 3, everything())
	st := h.stream(t, "sid-1")
	st.send(t, open)
	for range 10 {
		st.send(t, func(e *pluginv1.Envelope) {
			e.Payload = &pluginv1.Envelope_Health{Health: &pluginv1.Health{Grade: 200}}
		})
		st.quiet(t, 100*time.Millisecond)
	}
	if log := h.log.String(); !strings.Contains(log, "level=WARN") || !strings.Contains(log, "unit=acquire") || !strings.Contains(log, "200") {
		t.Errorf("no warning names the grade declared:\n%s", log)
	}
}

// std: yoke:names-and-filtering.06
func TestAConditionChangesWhenItsGradeDoes(t *testing.T) {
	h := newHarness(t)
	h.incarnation = 1
	st := opened(t, h)
	for _, r := range []struct {
		grade uint32
		line  string
	}{{90, "warm"}, {90, "warm"}, {40, "the lamp is ageing"}} {
		st.send(t, func(e *pluginv1.Envelope) {
			e.Payload = &pluginv1.Envelope_Health{Health: &pluginv1.Health{Grade: r.grade, Line: r.line}}
		})
	}
	st.quiet(t, 150*time.Millisecond)
	var changes []event.Event
	for _, e := range h.publishedEvents() {
		if e.Type == "unit.condition.changed" {
			changes = append(changes, e)
		}
	}
	if len(changes) != 2 {
		t.Fatalf("%d condition changes were published: %+v", len(changes), changes)
	}
	first, second := changes[0], changes[1]
	if first.Severity != 90 || first.Line != "warm" || first.Actor.Class != event.ByUnit || strings.Contains(string(first.Detail), `"from"`) {
		t.Errorf("the first change is %+v %s", first, first.Detail)
	}
	if second.Severity != 40 || second.Line != "the lamp is ageing" || !strings.Contains(string(second.Detail), `"from":90`) {
		t.Errorf("the second change is %+v %s", second, second.Detail)
	}
}
