package version

import (
	"strings"
	"testing"
)

// std: yoke:the-versions.01
func TestAVersionIsASemanticVersionOrItIsRefused(t *testing.T) {
	for _, s := range []string{"2.1.0", "2.1.0-rc.1", "2.1.0+build.7"} {
		if _, err := Parse(s); err != nil {
			t.Errorf("%q refused: %v", s, err)
		}
	}
	for _, s := range []string{"2.1", "v2.1.0", "02.1.0", "2.1.0-", ""} {
		_, err := Parse(s)
		if err == nil {
			t.Errorf("%q read as a version", s)
			continue
		}
		if !strings.Contains(err.Error(), "\""+s+"\"") {
			t.Errorf("the refusal of %q does not name it: %v", s, err)
		}
	}
}

// std: yoke:the-versions.02
func TestVersionsAreOrderedAsSemanticVersioningOrdersThem(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want int
	}{
		{"2.10.0", "2.9.0", 1},
		{"1.0.0", "2.0.0", -1},
		{"2.1.1", "2.1.0", 1},
		{"2.1.0-rc.1", "2.1.0", -1},
		{"2.1.0-rc.1", "2.0.9", 1},
		{"2.1.0-rc.2", "2.1.0-rc.10", -1},
		{"2.1.0-alpha", "2.1.0-alpha.1", -1},
		{"2.1.0+a", "2.1.0+b", 0},
		{"2.1.0", "2.1.0", 0},
	} {
		if got := MustParse(c.a).Compare(MustParse(c.b)); got != c.want {
			t.Errorf("%s against %s: %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

// std: yoke:the-versions.03
func TestARangeHoldsWhenEveryComparatorHolds(t *testing.T) {
	for _, c := range []struct {
		r   string
		in  []string
		out []string
	}{
		{">=2.0 <3.0", []string{"2.0.0", "2.5.1", "2.99.0"}, []string{"1.9.9", "3.0.0", "3.0.1"}},
		{">=2.0", []string{"2.0.0", "9.0.0"}, []string{"1.99.99"}},
		{"=2.1.0", []string{"2.1.0"}, []string{"2.1.1", "2.0.0"}},
		{">2.0.0 <=2.1", []string{"2.0.1", "2.1.0"}, []string{"2.0.0", "2.1.1"}},
	} {
		r, err := ParseRange(c.r)
		if err != nil {
			t.Fatalf("%q refused: %v", c.r, err)
		}
		for _, v := range c.in {
			if !r.Contains(MustParse(v)) {
				t.Errorf("%q does not contain %s", c.r, v)
			}
		}
		for _, v := range c.out {
			if r.Contains(MustParse(v)) {
				t.Errorf("%q contains %s", c.r, v)
			}
		}
	}
}

// std: yoke:the-versions.04
func TestARangeThatIsNotOneIsRefused(t *testing.T) {
	for _, s := range []string{"2.0", ">=", ">=two", "~2.0", ">=2.0 || <1.0", ""} {
		_, err := ParseRange(s)
		if err == nil {
			t.Errorf("%q read as a range", s)
			continue
		}
		if !strings.Contains(err.Error(), "\""+s+"\"") {
			t.Errorf("the refusal of %q does not name it: %v", s, err)
		}
	}
}
