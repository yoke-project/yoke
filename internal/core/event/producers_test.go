package event_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yoke-project/yoke/internal/core/event"
)

// makes is the constructor that makes each type.
var makes = map[string]string{
	"instance.ready":             "InstanceReady",
	"instance.stopping":          "InstanceStopping",
	"unit.state.changed":         "StateChanged",
	"unit.condition.changed":     "ConditionChanged",
	"unit.occurrence.reported":   "OccurrenceReported",
	"unit.stream.activated":      "StreamActivated",
	"unit.stream.stopped":        "StreamStopped",
	"document.resolved":          "DocumentResolved",
	"document.rejected":          "DocumentRejected",
	"connection.opened":          "ConnectionOpened",
	"channel.attached":           "ChannelAttached",
	"channel.detached":           "ChannelDetached",
	"channel.subscription.stale": "SubscriptionStale",
	"connection.closed":          "ConnectionClosed",
	"plugin.policy.changed":      "PolicyChanged",
}

// std: yoke:names-and-filtering.02
func TestEveryDeclaredTypeHasAProducer(t *testing.T) {
	var sources strings.Builder
	filepath.WalkDir("../..", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == "event" {
			return fs.SkipDir
		}
		if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			data, _ := os.ReadFile(path)
			sources.Write(data)
		}
		return nil
	})
	for _, name := range event.Names() {
		constructor, known := makes[name]
		if !known {
			t.Errorf("%s is declared, and this case knows no constructor that makes it", name)
			continue
		}
		if !strings.Contains(sources.String(), "event."+constructor+"(") {
			t.Errorf("%s is declared, and nothing in the Core makes it", name)
		}
	}
}
