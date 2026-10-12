package reclaim

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/yoke-project/yoke/internal/core/engine"
	"github.com/yoke-project/yoke/internal/core/engine/enginetest"
	"github.com/yoke-project/yoke/internal/installroot"
)

// std: yoke:the-removal.06
func TestThePassRemovesYokesImagesFromTheEngine(t *testing.T) {
	enginetest.Not(t, enginetest.Docker)
	// The engine's store is the account's, and other cases tag images in the reserved namespace while this
	// one runs: this case names an application of its own and narrows the pass to it.
	const id = "com.yoke.reclaim"
	refs := map[string]string{}
	for _, name := range []string{"yoke-reclaim-old", "yoke-reclaim-new", "yoke-reclaim-mine"} {
		ref, archive := enginetest.OCIArchive(t, name)
		if out, err := enginetest.CLI("load", "-i", archive).CombinedOutput(); err != nil {
			t.Fatalf("load %s: %v\n%s", name, err, out)
		}
		refs[name] = ref
	}
	for name, ver := range map[string]string{"yoke-reclaim-old": "2.0.0", "yoke-reclaim-new": "2.1.0"} {
		tag := "yoke/" + id + ":" + ver
		if out, err := enginetest.CLI("tag", refs[name], tag).CombinedOutput(); err != nil {
			t.Fatalf("tag %s: %v\n%s", tag, err, out)
		}
		t.Cleanup(func() { enginetest.CLI("rmi", "-f", tag).Run() })
	}
	o := setup(t)
	installAs(t, o, id, "2.0.0", installroot.Complete)
	installAs(t, o, id, "2.1.0", installroot.Complete)
	s := enginetest.Serve(t)
	o.Engine = func(ctx context.Context) (*engine.Engine, error) { return engine.Reach(ctx, "unix://"+s.Socket) }
	r, err := Prune(o, id)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(r.Removed, id+" 2.0.0") || !slices.ContainsFunc(r.Removed, func(s string) bool {
		return strings.HasPrefix(s, "image ") && strings.HasSuffix(s, "yoke/"+id+":2.0.0")
	}) {
		t.Errorf("the pass named %v", r.Removed)
	}
	held := func(ref string) bool { return enginetest.CLI("image", "inspect", ref).Run() == nil }
	if held("yoke/" + id + ":2.0.0") {
		t.Error("the tag of the removed version is still in the store")
	}
	if !held("yoke/" + id + ":2.1.0") {
		t.Error("the tag of the kept version was removed")
	}
	if !held(refs["yoke-reclaim-mine"]) {
		t.Error("the account's own image was removed")
	}
}
