package trunk

import (
	"slices"
	"testing"

	"github.com/yoke-project/yoke/internal/core/config"
	"github.com/yoke-project/yoke/internal/gate"
)

// std: yoke:the-trunk.14
func TestTheInstanceRecordSaysWhatThisCoreReads(t *testing.T) {
	var cfg config.Config
	cfg.Plugins.Manifests, cfg.Plugins.Executables = "/etc/yoke/plugins.d", "/usr/lib/yoke/plugins"
	plugins := []string{"plugins.manifests", "plugins.executables", "plugins.scan_interval"}
	for form, listed := range map[Form]bool{Service: true, Application: false} {
		r := (&State{Form: form, Config: cfg}).instanceRecord()
		for _, key := range plugins {
			if _, ok := r.GetParameters()[key]; ok != listed {
				t.Errorf("the %s form's record lists %s: %t, want %t", r.GetForm(), key, ok, listed)
			}
		}
	}

	st := &State{Form: Service, Config: cfg}
	st.noteWeaker(gate.Finding{Code: "channel.address.loopback", Location: "channels.remote.address"})
	st.noteWeaker(gate.Finding{Code: "confinement.no_cgroup_delegation"})
	want := []string{"channel.address.loopback at channels.remote.address", "confinement.no_cgroup_delegation"}
	if got := st.instanceRecord().GetWeaker(); !slices.Equal(got, want) {
		t.Errorf("the weaker arrangements read %q, want %q", got, want)
	}
}
