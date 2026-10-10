package engine_test

import (
	"fmt"
	"testing"
)

// std: yoke:the-needs-in-a-container.01
func TestDevicesMountsGroupsAndTheNetworkPerEngineAndMode(t *testing.T) {
	needing := launch()
	needing.Devices = []string{"/dev/ttyUSB0"}
	needing.Mounts = []string{"/var/lib/yoke/state/storage/cache", "/run/user/1000/wayland-1"}
	needing.Groups = []int{20}
	needing.Network = true
	asRoot := podman(false)
	for _, c := range []struct {
		name     string
		identity *fake
		groups   string
		kept     string
	}{
		{"podman rootless", podman(true), "<nil>", "map[run.oci.keep_original_groups:1]"},
		{"docker rootless", docker(true, "1.40", "1.51"), "<nil>", "<nil>"},
		{"podman as root", asRoot, "[20]", "<nil>"},
	} {
		s := &scripted{identity: c.identity}
		e := reached(t, s)
		if _, err := e.Create(ctx(t), needing); err != nil {
			t.Fatal(err)
		}
		if _, err := e.Create(ctx(t), launch()); err != nil {
			t.Fatal(err)
		}
		asked := s.requests()
		host, _ := asked[0].body["HostConfig"].(map[string]any)
		devices := fmt.Sprint(host["Devices"])
		if devices != "[map[CgroupPermissions:rwm PathInContainer:/dev/ttyUSB0 PathOnHost:/dev/ttyUSB0]]" ||
			fmt.Sprint(host["Binds"]) != "[/run/yk:/run/yk /var/lib/yoke/state/storage/cache:/var/lib/yoke/state/storage/cache /run/user/1000/wayland-1:/run/user/1000/wayland-1]" ||
			host["NetworkMode"] != nil || fmt.Sprint(host["GroupAdd"]) != c.groups || fmt.Sprint(host["Annotations"]) != c.kept {
			t.Errorf("%s: needing, the host configuration is %v", c.name, host)
		}
		host, _ = asked[1].body["HostConfig"].(map[string]any)
		if host["Devices"] != nil || fmt.Sprint(host["Binds"]) != "[/run/yk:/run/yk]" || host["GroupAdd"] != nil || host["Annotations"] != nil || host["NetworkMode"] != "none" {
			t.Errorf("%s: needing nothing, the host configuration is %v", c.name, host)
		}
	}
}
