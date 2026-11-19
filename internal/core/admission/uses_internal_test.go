package admission

import (
	"testing"

	pluginv1 "github.com/yoke-project/yoke/proto/yoke/plugin/v1"

	"github.com/yoke-project/yoke/internal/core/scope"
	"github.com/yoke-project/yoke/internal/gate"
)

// std: yoke:admission.15
func TestAUseIsGrantedWhereItsCapabilityIs(t *testing.T) {
	m := &gate.Manifest{
		Uses: map[string]gate.Use{"accounts": {Questions: []string{"oauth.token"}, Streams: []string{"oauth.status"}}},
		Capabilities: []gate.Capability{
			{Name: "query.oauth.token.ask", Governs: gate.Object{Kind: "use", Need: "accounts", Of: "query", ID: "oauth.token"}},
			{Name: "stream.oauth.status.follow", Governs: gate.Object{Kind: "use", Need: "accounts", Of: "stream", ID: "oauth.status"}},
		},
	}
	_, _, sc := intersect(m, []string{"query.oauth.token.ask"})
	for key, want := range map[string]pluginv1.Code{
		scope.UseKey("accounts", "query", "oauth.token"):   pluginv1.Code_CODE_UNSPECIFIED,
		scope.UseKey("accounts", "stream", "oauth.status"): pluginv1.Code_CODE_SCOPE_WITHHELD,
		scope.UseKey("other", "query", "oauth.token"):      pluginv1.Code_CODE_SCOPE_UNDECLARED,
	} {
		if got := sc.Check(scope.Use, key); got != want {
			t.Errorf("%s is %v, want %v", key, got, want)
		}
	}
}
