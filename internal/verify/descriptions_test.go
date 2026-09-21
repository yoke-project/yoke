package verify_test

import (
	"strings"
	"testing"
)

// std: yoke:descriptions-and-markers.01
func TestADescriptionInTheFormIsReadInOrder(t *testing.T) {
	root := tree(t, map[string]string{
		"sample.std.md": description(
			caseBlock("yoke:sample.01", "the first case", fields()),
			caseBlock("yoke:sample.02", "the second case", fields()),
		),
	})

	out := accepted(t, "descriptions", root)

	want := []string{"yoke:sample.01", "yoke:sample.02"}
	got := strings.Fields(strings.TrimSpace(out))
	if len(got) != len(want) {
		t.Fatalf("read %v, want the two identifiers in the file's order", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("case %d is %q, want %q", i, got[i], want[i])
		}
	}
}

// std: yoke:descriptions-and-markers.02
func TestAFieldMissingRepeatedOutOfOrderOrUnknownIsRefused(t *testing.T) {
	repeated := append(fields(), [2]string{"Cites", "testing/30 §The file"})
	outOfOrder := fields()
	outOfOrder[1], outOfOrder[2] = outOfOrder[2], outOfOrder[1]
	ninth := append(fields(), [2]string{"Owner", "the maintainer"})

	for _, c := range []struct {
		name  string
		fs    [][2]string
		terms []string
	}{
		{"missing", drop(fields(), "Expected"), []string{"yoke:sample.01", "Expected"}},
		{"repeated", repeated, []string{"yoke:sample.01", "Cites"}},
		{"out of order", outOfOrder, []string{"yoke:sample.01", "Level"}},
		{"unknown", ninth, []string{"yoke:sample.01", "Owner"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			root := tree(t, map[string]string{
				"sample.std.md": description(caseBlock("yoke:sample.01", "a case", c.fs)),
			})
			refused(t, "descriptions", root, c.terms...)
		})
	}
}

// std: yoke:descriptions-and-markers.03
func TestAValueOutsideWhatAFieldAllowsIsRefused(t *testing.T) {
	for _, c := range []struct {
		name, field, value string
	}{
		{"the contract level", "Level", "L2"},
		{"a method nobody defines", "Method", "verify"},
		{"a dimension no environment declares", "Not applicable in", "colour: green"},
		{"blocking carrying an item", "Label", "blocking — yoke-project/yoke#3"},
	} {
		t.Run(c.name, func(t *testing.T) {
			root := tree(t, map[string]string{
				"sample.std.md": description(
					caseBlock("yoke:sample.01", "a case", set(fields(), c.field, c.value)),
				),
			})
			refused(t, "descriptions", root, "yoke:sample.01", c.field, c.value)
		})
	}
}

// std: yoke:descriptions-and-markers.04
func TestAnIdentifierThatDoesNotMatchItsFileOrRepeatsIsRefused(t *testing.T) {
	t.Run("another feature", func(t *testing.T) {
		root := tree(t, map[string]string{
			"sample.std.md": description(caseBlock("yoke:other.01", "a case", fields())),
		})
		refused(t, "descriptions", root, "yoke:other.01", "sample")
	})

	t.Run("another repository", func(t *testing.T) {
		root := tree(t, map[string]string{
			"sample.std.md": description(caseBlock("yoke-sdk-go:sample.01", "a case", fields())),
		})
		refused(t, "descriptions", root, "yoke-sdk-go:sample.01", "yoke")
	})

	t.Run("one digit", func(t *testing.T) {
		root := tree(t, map[string]string{
			"sample.std.md": description(caseBlock("yoke:sample.1", "a case", fields())),
		})
		refused(t, "descriptions", root, "yoke:sample.1")
	})

	t.Run("a number twice", func(t *testing.T) {
		root := tree(t, map[string]string{
			"sample.std.md": description(
				caseBlock("yoke:sample.01", "a case", fields()),
				caseBlock("yoke:sample.01", "the same number again", fields()),
			),
		})
		refused(t, "descriptions", root, "yoke:sample.01")
	})
}

// std: yoke:descriptions-and-markers.05
func TestANotBlockingLabelCitesAnItem(t *testing.T) {
	t.Run("with no item", func(t *testing.T) {
		root := tree(t, map[string]string{
			"sample.std.md": description(
				caseBlock("yoke:sample.01", "a case", set(fields(), "Label", "not blocking")),
			),
		})
		refused(t, "descriptions", root, "yoke:sample.01", "Label")
	})

	t.Run("citing one", func(t *testing.T) {
		root := tree(t, map[string]string{
			"sample.std.md": description(
				caseBlock("yoke:sample.01", "a case",
					set(fields(), "Label", "not blocking — yoke-project/yoke#3")),
			),
		})
		accepted(t, "descriptions", root)
	})
}

// std: yoke:descriptions-and-markers.09
func TestAStruckCaseKeepsItsNumberAndAsksForNothing(t *testing.T) {
	root := tree(t, map[string]string{
		"sample.std.md": description(
			caseBlock("yoke:sample.01", "the case that remains", fields()),
			"## ~~yoke:sample.02 — the case that went away~~ — the rule it cited was withdrawn\n",
		),
		"sample_test.go": "package sample\n\n// std: yoke:sample.01\nfunc TestTheCaseThatRemains() {}\n",
	})

	accepted(t, "descriptions", root)
	out := accepted(t, "markers", root)
	if strings.Contains(out, "yoke:sample.02") {
		t.Errorf("the struck case is in the map: %s", out)
	}
}

// std: yoke:descriptions-and-markers.10
func TestAStruckNumberCannotComeBackAndNothingMayNameIt(t *testing.T) {
	struck := "## ~~yoke:sample.02 — the case that went away~~\n"

	t.Run("reused by a live case", func(t *testing.T) {
		root := tree(t, map[string]string{
			"sample.std.md": description(
				struck,
				caseBlock("yoke:sample.02", "the number again", fields()),
			),
		})
		refused(t, "descriptions", root, "yoke:sample.02")
	})

	t.Run("named by a test", func(t *testing.T) {
		root := tree(t, map[string]string{
			"sample.std.md": description(
				caseBlock("yoke:sample.01", "the case that remains", fields()),
				struck,
			),
			"sample_test.go": "package sample\n\n// std: yoke:sample.01\nfunc TestOne() {}\n\n" +
				"// std: yoke:sample.02\nfunc TestTwo() {}\n",
		})
		refused(t, "markers", root, "yoke:sample.02")
	})
}
