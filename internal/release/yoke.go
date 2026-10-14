package release

// Yoke is what `yoke`'s programs tag hands over as files: the host's artifact — the Core, the manager, the
// guardian and the program the Core asks the host's authentication service through, with the packaging
// inputs a distribution installs beside them — and the developer's two, the suite with a Core for it to
// drive and the composer, and the verification tool.
var Yoke = []Artifact{
	{Name: "yoke", Packages: []string{"./cmd/yoke-core", "./cmd/yoke", "./cmd/yoke-guardian", "./cmd/yoke-auth"},
		Files: []string{"packaging/yoke.service", "packaging/core.yaml", "packaging/yoke.tmpfiles", "packaging/yoke.sysusers"}},
	{Name: "yoke-conformance", Packages: []string{"./cmd/yoke-conformance", "./cmd/yoke-core", "./cmd/yoke-compose"}},
	{Name: "yoke-verify", Packages: []string{"./cmd/yoke-verify"}},
}
