package conformance

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"github.com/yoke-project/yoke/internal/verify"
)

// RenderedLine opens a rendering, and is what lets its cases be at L2.
const RenderedLine = verify.RenderedLine

// Contract is one contract's cases, and what heads their description.
type Contract struct {
	Name    string // the rendering is <Name>.std.md, and each case <repository>:<Name>.<nn>
	Feature string
	Item    string
	Cases   []Case
}

// Plugin is the plugin contract, as the suite measures it.
func Plugin() Contract {
	return Contract{Name: "plugin", Feature: "the plugin contract, as the conformance suite measures it against a real Core through a family's harness",
		Item: "yoke-project/yoke#19", Cases: Cases()}
}

// Contracts are the contracts the suite measures, by name.
func Contracts() map[string]Contract { return map[string]Contract{"plugin": Plugin()} }

// Render is a contract's description, in the form every description has, from the fields each case
// carries: nobody writes a description of the suite's cases by hand.
func Render(c Contract) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "%s\n# The %s contract, at L2\n\n| | |\n| --- | --- |\n| **Feature** | %s |\n| **Planning item** | %s |\n", RenderedLine, c.Name, c.Feature, c.Item)
	for _, k := range c.Cases {
		fmt.Fprintf(&b, "\n## %s — %s\n\n| Field | Value |\n| --- | --- |\n", k.ID, k.Title)
		for _, f := range [][2]string{
			{"Cites", strings.Join(k.Cites, " · ")}, {"Level", "L2"}, {"Method", "test"}, {"Not applicable in", "—"},
			{"Label", "blocking"}, {"Precondition", k.Precondition}, {"Action", k.Issues}, {"Expected", k.Requires},
		} {
			fmt.Fprintf(&b, "| **%s** | %s |\n", f[0], f[1])
		}
	}
	return b.Bytes()
}

// Current fails when the rendering committed at path is not what the cases render to now.
func Current(path string, c Contract) error {
	committed, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if !bytes.Equal(committed, Render(c)) {
		return fmt.Errorf("%s is not the current rendering of the %s contract's cases: render it again with `yoke-conformance --render %s`", path, c.Name, c.Name)
	}
	return nil
}
