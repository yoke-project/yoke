package conformance

// Fixture is the plugin an administrative run's Core declares, and nothing composes: what the harness
// administers.
const Fixture = "com.yoke.conformance.fixture"

// fixtureManifest declares the fixture with one capability, so that a grant has something to name.
const fixtureManifest = "manifest: 1\nid: " + Fixture + "\nprotocol: 1\nstreams: [ { id: fixture.data } ]\n" +
	"capabilities: [ { name: stream.data.publish, governs: { stream: fixture.data } } ]\n"

// AdministrativeCases are the administrative contract's cases, in the order a run performs them.
func AdministrativeCases() []Case {
	return []Case{
		{ID: "yoke:administrative.01", Title: "the library reaches the instance by its addresses, and reads it", Contract: "administrative",
			Cites:        []string{"specs/60.7", "specs/60.36", "specs/90.4", "arch/60-administrative-surface/01 §One pair per instance", "arch/60-administrative-surface/05 §Which subjects are readable"},
			Precondition: "a Core started by the suite in the service form, and the harness launched by the suite with the instance's root in `CONFORMANCE_INSTANCE`",
			Issues:       "`read` of the kind `instance`",
			Requires:     "one record, the instance's, ready and in the service form",
			Run:          readTheInstance},
		{ID: "yoke:administrative.02", Title: "a change answers what it replaced, and an effect already true succeeds", Contract: "administrative",
			Cites:        []string{"specs/60.31", "specs/60.34", "arch/60-administrative-surface/04 §What every answer carries"},
			Precondition: "the fixture plugin declared, and enabled",
			Issues:       "`disable` of the fixture, twice",
			Requires:     "the first answers that it was enabled, effective immediately; the second that it was not, effective immediately",
			Run:          disableTwice},
		{ID: "yoke:administrative.03", Title: "a refusal travels as its code, with what it names", Contract: "administrative",
			Cites:        []string{"specs/60.57", "specs/90.23", "arch/60-administrative-surface/07 §This surface's codes", "arch/90-sdks/05 §The observable model may not differ"},
			Precondition: "a unit nobody declared, and a capability the fixture's Manifest does not declare",
			Issues:       "`stop-unit` of `nobody`; `grant` of `head.move` to the fixture",
			Requires:     "`subject.unknown` naming the kind `unit` and the identity `nobody`; `capability.undeclared` naming `head.move`",
			Run:          refusalsTravel},
		{ID: "yoke:administrative.04", Title: "a grant takes effect at the next admission", Contract: "administrative",
			Cites:        []string{"specs/60.33", "arch/60-administrative-surface/04 §What every answer carries"},
			Precondition: "the fixture, granted nothing",
			Issues:       "`grant` of `stream.data.publish` to the fixture",
			Requires:     "that it was not granted, effective at the next admission",
			Run:          grantAtNextAdmission},
		{ID: "yoke:administrative.05", Title: "a subscription opens with a snapshot, and continues with what happens", Contract: "administrative",
			Cites:        []string{"specs/60.36", "specs/60.49", "specs/90.32", "arch/60-administrative-surface/06 §What a subscription promises"},
			Precondition: "the fixture, disabled",
			Issues:       "`subscribe` to the subject kind `plugin`; then `enable` of the fixture",
			Requires:     "a snapshot observed holding the fixture's record, then an event `plugin.policy.changed` about the fixture",
			Run:          subscriptionContinues},
		{ID: "yoke:administrative.06", Title: "the log is queried", Contract: "administrative",
			Cites:        []string{"specs/60.43", "arch/60-administrative-surface/05 §The log store, queried and followed"},
			Precondition: "the Core, ready",
			Issues:       "`query-log`, from the beginning",
			Requires:     "entries, among them `instance.ready`",
			Run:          logQueried},
	}
}

func notYet(r *Run) Outcome { return Fail("", "the case to be performed", "nothing yet") }

// std: yoke:administrative.01
func readTheInstance(r *Run) Outcome { return notYet(r) }

// std: yoke:administrative.02
func disableTwice(r *Run) Outcome { return notYet(r) }

// std: yoke:administrative.03
func refusalsTravel(r *Run) Outcome { return notYet(r) }

// std: yoke:administrative.04
func grantAtNextAdmission(r *Run) Outcome { return notYet(r) }

// std: yoke:administrative.05
func subscriptionContinues(r *Run) Outcome { return notYet(r) }

// std: yoke:administrative.06
func logQueried(r *Run) Outcome { return notYet(r) }
