package admin_test

import (
	"testing"

	administrativev1 "github.com/yoke-project/yoke/proto/yoke/administrative/v1"
)

// std: yoke:what-a-channel-sees.04
func TestTheAdministrativeSurfaceReadsEveryChannel(t *testing.T) {
	b := newBench(t, map[string]*fakeUnit{}, map[string]string{})
	b.core.Channels = func() []*administrativev1.ChannelRecord {
		return []*administrativev1.ChannelRecord{
			{Declared: &administrativev1.ChannelRecord_Declared{Name: "panel", Projection: "local", AddressClass: "local"},
				Observed: &administrativev1.ChannelRecord_Observed{Attached: true, Client: "davide"}},
			{Declared: &administrativev1.ChannelRecord_Declared{Name: "remote", Projection: "http+ws", AddressClass: "loopback"},
				Observed: &administrativev1.ChannelRecord_Observed{}},
		}
	}
	read := func(identity string) []*administrativev1.Record {
		resp, ref := b.call(t, v1(&administrativev1.Request{Operation: &administrativev1.Request_Read{Read: &administrativev1.Read{Kind: "channel", Identity: identity}}}))
		if ref != nil {
			t.Fatalf("reading channel %q was refused %v", identity, ref)
		}
		return resp.GetRead().GetRecords()
	}
	all := read("")
	if len(all) != 2 || all[0].GetChannel().GetDeclared().GetName() != "panel" || all[0].GetChannel().GetObserved().GetClient() != "davide" ||
		all[1].GetChannel().GetDeclared().GetAddressClass() != "loopback" || all[1].GetChannel().GetObserved().GetAttached() {
		t.Errorf("the channels read as %v", all)
	}
	if one := read("remote"); len(one) != 1 || one[0].GetChannel().GetDeclared().GetName() != "remote" {
		t.Errorf("one channel read as %v", one)
	}
}
