package gate_test

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/yoke-project/yoke/internal/gate"
)

const sixTypes = `parameters:
  shared:
    site_label: { type: string, pattern: "^[A-Za-z ]{1,16}$", required: true, label: "Label" }
    verbose: { type: boolean, required: false, default: false, label: "Verbose" }
  a:
    retention_days: { type: integer, range: [1, 3650], required: false, default: 90, label: "Days" }
    mode: { type: enum, values: [fast, slow], required: false, default: fast, label: "Mode" }
    instrument: { type: path, required: false, default: /dev/null, label: "Instrument" }
    upstream: { type: address, required: false, default: "upstream.local:9000", label: "Upstream" }
`

// std: yoke:the-parameters.01
func TestDeclarationsAreReadAndHandedOn(t *testing.T) {
	r, d := descriptor(t, identity+sixTypes+unit)
	if r.Refused() {
		t.Fatalf("refused: %v", r.Findings)
	}
	p := d.Parameters["a"]["retention_days"]
	if p.Type != "integer" || p.Min == nil || *p.Min != 1 || *p.Max != 3650 || p.Required || !p.HasDefault || p.Default != "90" || p.Label != "Days" {
		t.Errorf("retention_days is %+v", p)
	}
	if s := d.Parameters["shared"]["site_label"]; !s.Required || s.HasDefault || s.Pattern == "" {
		t.Errorf("site_label is %+v", s)
	}
	if e := d.Parameters["a"]["mode"]; !slices.Equal(e.Values, []string{"fast", "slow"}) {
		t.Errorf("mode is %+v", e)
	}
	if len(d.Parameters["a"]) != 4 || len(d.Parameters["shared"]) != 2 {
		t.Errorf("declared %v", d.Parameters)
	}
}

func declares(t *testing.T, declaration string) gate.Report {
	t.Helper()
	r, _ := descriptor(t, identity+"parameters:\n  a:\n    p: "+declaration+"\n"+unit)
	return r
}

// std: yoke:the-parameters.02
func TestADeclarationIsWhatItsTypeAllows(t *testing.T) {
	for declaration, code := range map[string]string{
		`{ type: string, required: true, label: L, colour: red }`:                  "key.unknown",
		`{ type: float, required: true, label: L }`:                                "field.value",
		`{ type: string, range: [1, 2], required: true, label: L }`:                "field.value",
		`{ type: integer, pattern: "x", required: true, label: L }`:                "field.value",
		`{ type: enum, required: true, label: L }`:                                 "field.required",
		`{ type: string, pattern: "([", required: true, label: L }`:                "field.value",
		`{ type: integer, range: [1, 9], required: false, default: 20, label: L }`: "field.value",
		`{ type: string, required: true }`:                                         "field.required",
	} {
		r := declares(t, declaration)
		got := found(r, code)
		if len(got) == 0 || !strings.Contains(got[0].Location, "parameters.a.p") {
			t.Errorf("%s\n  gives %v, want %s at parameters.a.p", declaration, r.Findings, code)
		}
	}
}

// std: yoke:the-parameters.03
func TestExactlyOneArrangementOfRequiredAndDefaultIsLegal(t *testing.T) {
	for _, declaration := range []string{
		`{ type: string, required: true, default: x, label: L }`,
		`{ type: string, required: false, label: L }`,
	} {
		r := declares(t, declaration)
		if got := found(r, "parameter.arrangement"); len(got) != 1 || !strings.Contains(got[0].Location, "parameters.a.p") {
			t.Errorf("%s gives %v", declaration, r.Findings)
		}
	}
}

// std: yoke:the-parameters.04
func TestAGroupIsNamedAfterAUnitAChannelOrShared(t *testing.T) {
	r, _ := descriptor(t, identity+"parameters:\n  panel:\n    p: { type: string, required: true, label: L }\n"+unit)
	if got := found(r, "field.value"); len(got) != 1 || !strings.Contains(got[0].Message, "panel") {
		t.Errorf("a group naming nothing gives %v", r.Findings)
	}
}

// std: yoke:the-parameters.05
func TestSuppliedValuesAreCheckedAgainstTheDeclarations(t *testing.T) {
	_, d := descriptor(t, identity+sixTypes+unit)
	device := filepath.Join(t.TempDir())
	good := map[string]map[string]string{"shared": {"site_label": "Bench A"}, "a": {"instrument": device, "mode": "slow"}}
	values, findings := gate.Values(d, good)
	if len(findings) != 0 {
		t.Fatalf("good values gave %v", findings)
	}
	if values["shared"]["site_label"] != "Bench A" || values["a"]["mode"] != "slow" || values["a"]["retention_days"] != "90" ||
		values["shared"]["verbose"] != "false" || values["a"]["instrument"] != device {
		t.Errorf("resolved to %v", values)
	}
	for _, c := range []struct {
		group, name, value, code string
	}{
		{"shared", "site_label", "Bench #1", "parameter.value"},
		{"a", "retention_days", "4000", "parameter.value"},
		{"a", "mode", "medium", "parameter.value"},
		{"shared", "verbose", "maybe", "parameter.value"},
		{"a", "instrument", "/nonexistent/device", "parameter.value"},
		{"a", "colour", "red", "key.unknown"},
	} {
		supplied := map[string]map[string]string{"shared": {"site_label": "Bench A"}, "a": {}}
		supplied[c.group][c.name] = c.value
		_, findings := gate.Values(d, supplied)
		if len(findings) != 1 || findings[0].Code != c.code || !strings.Contains(findings[0].Message, c.name) ||
			(c.code == "parameter.value" && !strings.Contains(findings[0].Message, c.value)) {
			t.Errorf("%s.%s = %s gives %v, want one %s", c.group, c.name, c.value, findings, c.code)
		}
	}
	_, findings = gate.Values(d, map[string]map[string]string{})
	if len(findings) != 1 || findings[0].Code != "parameter.missing" || !strings.Contains(findings[0].Message, "site_label") {
		t.Errorf("no value for the required parameter gives %v", findings)
	}
}
