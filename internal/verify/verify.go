// Package verify reads the descriptions a repository carries, checks the form testing/30 fixes, and
// joins each case to the test that performs it.
package verify

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"sort"
)

// A Finding is one thing wrong, named where it was found.
type Finding struct {
	File string
	Msg  string
}

func (f Finding) String() string {
	if f.File == "" {
		return f.Msg
	}
	return f.File + ": " + f.Msg
}

// Run performs one subcommand, writing what it produces to out and every finding to errOut. It
// returns the exit status: zero when every check the subcommand performs holds.
func Run(args []string, out, errOut io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(errOut, "yoke-verify: no subcommand; expected descriptions, markers or record")
		return 2
	}

	subcommand := args[0]
	flags := flag.NewFlagSet("yoke-verify "+subcommand, flag.ContinueOnError)
	flags.SetOutput(errOut)
	repository := flags.String("repository", "", "the repository the identifiers name; the root's own name by default")
	emit := flags.String("emit", "", "what to write when every check holds: `json`, the cases as they were parsed (473)")
	var options recordOptions
	if subcommand == "record" {
		options.register(flags)
	}
	if err := flags.Parse(args[1:]); err != nil {
		return 2
	}

	root := flags.Arg(0)
	if root == "" {
		root = "."
	}
	if *repository == "" {
		absolute, err := filepath.Abs(root)
		if err != nil {
			fmt.Fprintf(errOut, "yoke-verify: %v\n", err)
			return 2
		}
		*repository = filepath.Base(absolute)
	}

	descriptions, findings := readDescriptions(root)

	switch subcommand {
	case "descriptions":
		findings = append(findings, checkDescriptions(descriptions, *repository)...)
		if len(findings) > 0 {
			return report(errOut, findings)
		}
		if *emit == "json" {
			written, err := json.MarshalIndent(emitted(descriptions), "", "  ")
			if err != nil {
				fmt.Fprintf(errOut, "yoke-verify: %v\n", err)
				return 2
			}
			fmt.Fprintln(out, string(written))
			return 0
		}
		if *emit != "" {
			fmt.Fprintf(errOut, "yoke-verify: no such emission %q; expected json\n", *emit)
			return 2
		}
		for _, description := range descriptions {
			for _, c := range description.cases {
				if !c.struck {
					fmt.Fprintln(out, c.id)
				}
			}
		}
		return 0

	case "markers":
		pairs, scanned := scanMarkers(root, descriptions)
		findings = append(findings, scanned...)
		if len(findings) > 0 {
			return report(errOut, findings)
		}
		for _, p := range pairs {
			fmt.Fprintf(out, "{\"case\":%q,\"test\":%q,\"file\":%q}\n", p.id, p.test, p.file)
		}
		return 0

	case "record":
		findings = append(findings, checkDescriptions(descriptions, *repository)...)
		pairs, scanned := scanMarkers(root, descriptions)
		findings = append(findings, scanned...)
		if len(findings) > 0 {
			return report(errOut, findings)
		}
		record, built := buildRecord(root, *repository, descriptions, pairs, options)
		if len(built) > 0 {
			return report(errOut, built)
		}
		written, err := json.MarshalIndent(record, "", "  ")
		if err != nil {
			fmt.Fprintf(errOut, "yoke-verify: %v\n", err)
			return 2
		}
		fmt.Fprintln(out, string(written))
		return 0

	default:
		fmt.Fprintf(errOut, "yoke-verify: no subcommand %q; expected descriptions, markers or record\n", subcommand)
		return 2
	}
}

// report writes every finding, in a stable order, and returns the exit status of a refusal.
func report(errOut io.Writer, findings []Finding) int {
	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].File != findings[j].File {
			return findings[i].File < findings[j].File
		}
		return findings[i].Msg < findings[j].Msg
	})
	for _, f := range findings {
		fmt.Fprintln(errOut, f)
	}
	return 1
}
