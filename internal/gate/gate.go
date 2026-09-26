// Package gate is the one validator: given a document, and the Manifests and host facts it is joined
// against, it returns a report — every finding, never an exception at the first — and, when nothing
// was refused, the deployment the document describes.
//
// It has no process, no configuration and no state of its own, it executes nothing it checks, and it
// authorises nothing: what a Plugin may do is read from the Registry by the Core. A finding carries a
// code, a class, the document and the path to the field, and a message naming the value. The checks run
// in phases, and a phase after a refusal, or one whose inputs were not given, is named as not run.
package gate

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

// Kind says which of the two documents a document is: they are two schemas over one body.
type Kind int

const (
	Descriptor Kind = iota
	Composition
)

// Class is whether a finding refuses the document or reports an arrangement that gives something up.
type Class string

const (
	Refusal Class = "refusal"
	Weaker  Class = "weaker"
)

// The phases, in the order they run.
const (
	PhaseReading       = "reading"
	PhaseShape         = "shape"
	PhaseInternalJoins = "internal joins"
	PhaseCrossDocument = "cross-document"
	PhaseHostFacts     = "host facts"
	PhaseWeaker        = "weaker arrangements"
)

// Finding is one thing a pass found.
type Finding struct {
	Code     string // <subject>.<condition>
	Class    Class
	Document string // the path of the document it is in
	Location string // the path to the field, `units.acquire.bind.instrument`
	Message  string
}

// Report is everything a pass found, and the phases it did not reach.
type Report struct {
	Findings []Finding
	NotRun   []string
}

// Refused says whether anything in the report refuses the document.
func (r Report) Refused() bool {
	for _, f := range r.Findings {
		if f.Class == Refusal {
			return true
		}
	}
	return false
}

// Document is one document as it was read: its bytes, or why it could not be.
type Document struct {
	Path  string
	Kind  Kind
	Bytes []byte
	Err   error
}

// Read reads the document at path. A document that cannot be read is still a document, refused at
// reading.
func Read(path string, kind Kind) Document {
	b, err := os.ReadFile(path)
	return Document{Path: path, Kind: kind, Bytes: b, Err: err}
}

// Input is what a pass is given.
type Input struct {
	Document Document
}

// Check runs every phase it has the inputs for, and returns the report and — when nothing was refused —
// the deployment.
func Check(in Input) (Report, *Deployment) {
	c := &checker{doc: in.Document}
	phases := []struct {
		name string
		run  func() bool // false when its inputs were not given
	}{
		{PhaseReading, c.reading},
		{PhaseShape, c.shape},
		{PhaseInternalJoins, c.joins},
		{PhaseCrossDocument, func() bool { return false }},
		{PhaseHostFacts, func() bool { return false }},
		{PhaseWeaker, c.weaker},
	}
	stopped := false
	for _, p := range phases {
		if stopped {
			c.report.NotRun = append(c.report.NotRun, p.name)
			continue
		}
		if !p.run() {
			c.report.NotRun = append(c.report.NotRun, p.name)
		}
		stopped = c.report.Refused()
	}
	if c.report.Refused() {
		return c.report, nil
	}
	return c.report, c.deployment
}

type checker struct {
	doc        Document
	report     Report
	root       *node
	deployment *Deployment
}

func (c *checker) refuse(code, location, format string, args ...any) {
	c.report.Findings = append(c.report.Findings, Finding{
		Code: code, Class: Refusal, Document: c.doc.Path, Location: location, Message: fmt.Sprintf(format, args...),
	})
}

func (c *checker) note(code, location, format string, args ...any) {
	c.report.Findings = append(c.report.Findings, Finding{
		Code: code, Class: Weaker, Document: c.doc.Path, Location: location, Message: fmt.Sprintf(format, args...),
	})
}

func (c *checker) reading() bool {
	if c.doc.Err != nil {
		what := "cannot be read"
		if errors.Is(c.doc.Err, os.ErrNotExist) {
			what = "is absent"
		}
		c.refuse("document.unreadable", "", "the document %s %s: %v", c.doc.Path, what, c.doc.Err)
		return true
	}
	root, err := parse(c.doc.Bytes)
	if err != nil {
		c.refuse("document.malformed", "", "the document %s does not parse: %v", c.doc.Path, err)
		return true
	}
	c.root = root
	return true
}

// joined is a location with one more step.
func joined(location, step string) string {
	if location == "" {
		return step
	}
	return location + "." + step
}

// nameOK is the rule for a name that becomes a path component: validated, never transformed.
func nameOK(name string) bool {
	return name != "" && !strings.ContainsAny(name, "/\x00")
}
