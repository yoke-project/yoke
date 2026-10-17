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
	for _, name := range []string{"yoke-reclaim-old-a", "yoke-reclaim-old-b", "yoke-reclaim-new", "yoke-reclaim-mine"} {
		ref, archive := enginetest.OCIArchive(t, name)
		if out, err := enginetest.CLI("load", "-i", archive).CombinedOutput(); err != nil {
			t.Fatalf("load %s: %v\n%s", name, err, out)
		}
		refs[name] = ref
	}
	// Tagged through the engine's API, as installing tags: an engine qualifies the name its own way there,
	// which its command line may not.
	s := enginetest.Serve(t)
	tagger, err := engine.Reach(context.Background(), "unix://"+s.Socket)
	if err != nil {
		t.Fatal(err)
	}
	tagged := map[string]string{"yoke-reclaim-old-a": "a:2.0.0", "yoke-reclaim-old-b": "b:2.0.0", "yoke-reclaim-new": "a:2.1.0"}
	for name, at := range tagged {
		unit, ver, _ := strings.Cut(at, ":")
		if err := tagger.Tag(context.Background(), refs[name], id, unit, ver); err != nil {
			t.Fatalf("tag %s as %s: %v", name, at, err)
		}
		tag := "yoke/" + id + "/" + at
		t.Cleanup(func() {
			for _, q := range []string{"", "localhost/", "docker.io/"} {
				enginetest.CLI("rmi", "-f", q+tag).Run()
			}
		})
	}
	o := setup(t)
	installAs(t, o, id, "2.0.0", installroot.Complete)
	installAs(t, o, id, "2.1.0", installroot.Complete)
	o.Engine = func(ctx context.Context) (*engine.Engine, error) { return engine.Reach(ctx, "unix://"+s.Socket) }
	r, err := Prune(o, id)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(r.Removed, id+" 2.0.0") {
		t.Errorf("the pass did not name the version: %v", r.Removed)
	}
	held := func(ref string) bool { return enginetest.CLI("image", "inspect", ref).Run() == nil }
	for _, unit := range []string{"a", "b"} {
		tag := "yoke/" + id + "/" + unit + ":2.0.0"
		if !slices.ContainsFunc(r.Removed, func(s string) bool { return strings.HasPrefix(s, "image ") && strings.HasSuffix(s, tag) }) {
			t.Errorf("the pass did not name %s: %v", tag, r.Removed)
		}
		if held(tag) {
			t.Errorf("%s of the removed version is still in the store", tag)
		}
	}
	if !held("yoke/" + id + "/a:2.1.0") {
		t.Error("the tag of the kept version was removed")
	}
	if !held(refs["yoke-reclaim-mine"]) {
		t.Error("the account's own image was removed")
	}
}
