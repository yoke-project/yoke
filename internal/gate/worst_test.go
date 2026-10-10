package gate_test

import (
	"fmt"
	"testing"
)

var sprintf = fmt.Sprintf

const groups = `
policy: { retention: { bytes: 10MB } }
units:
  a: { kind: oneshot, exec: /bin/true, policy: { retention: { bytes: 1MB } } }
  b: { kind: oneshot, exec: /bin/true%s }
`

// std: yoke:the-worst-case.01
func TestTheWorstCaseIsTheSumOfEachGroupsBytesLimit(t *testing.T) {
	r, dep := composition(t, sprintf(groups, ""))
	if dep == nil {
		t.Fatalf("refused: %v", r.Findings)
	}
	if bytes, bounded := dep.WorstCase(); !bounded || bytes != 21_000_000 {
		t.Errorf("the worst case is %d, bounded %v; want 21000000, bounded", bytes, bounded)
	}
}

// std: yoke:the-worst-case.02
func TestAGroupWithNoSizeLimitMakesTheWorstCaseUnbounded(t *testing.T) {
	r, dep := composition(t, sprintf(groups, ", policy: { retention: { bytes: 0 } }"))
	if dep == nil || r.Refused() {
		t.Fatalf("refused: %v", r.Findings)
	}
	if bytes, bounded := dep.WorstCase(); bounded || bytes != 0 {
		t.Errorf("the worst case is %d, bounded %v; want unbounded, with no figure", bytes, bounded)
	}
}
