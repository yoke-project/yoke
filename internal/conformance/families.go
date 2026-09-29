package conformance

// FamilyCases are the plugin contract's cases for the families, the capabilities and the grants, which
// run after the next life of case 6 is admitted. The suite drives the deployment through the
// administrative contract, as any client does.
func FamilyCases() []Case {
	return []Case{
		{Contract: "plugin", ID: "yoke:plugin.07", Title: "an occurrence outside the granted scope is refused, and the Session goes on",
			Cites:        []string{"specs/50.30", "specs/50.64", "specs/50.105", "arch/50-plugin-surface/06 §Four families, closed", "arch/50-plugin-surface/04 §Revocation"},
			Precondition: "the harness of case 6, admitted and granted nothing",
			Issues:       "`report` of the occurrence its Manifest declares, at 40",
			Requires:     "an observation `refused` with `scope.withheld`, and no end of the Session",
			Run:          occurrenceWithheld},
		{Contract: "plugin", ID: "yoke:plugin.08", Title: "a grant reaches the unit at its next admission",
			Cites:        []string{"specs/50.30", "specs/50.32", "specs/60.33", "arch/50-plugin-surface/03 §The grant is an intersection, computed once"},
			Precondition: "the harness of case 7",
			Issues:       "every capability the Manifest declares granted and the unit restarted, on the administrative surface; then `start` in the life that follows",
			Requires:     "each grant effective at the next admission; the next life accepted without restriction, granted every capability, stream, command and query declared",
			Run:          grantedAtNextLife},
		{Contract: "plugin", ID: "yoke:plugin.09", Title: "the Core's question reaches the unit, and its answer comes back, opaque",
			Cites:        []string{"specs/50.61", "specs/50.69", "specs/60.46", "arch/50-plugin-surface/05 §The eight", "arch/60-administrative-surface/04 §The one operation whose content the Core does not read"},
			Precondition: "the harness of case 8, granted everything",
			Issues:       "a question of the type its Manifest declares, asked of the unit on the administrative surface with some bytes; then `answer` with other bytes",
			Requires:     "an observation `question` of that type carrying the bytes asked; the administrative answer is the bytes answered",
			Run:          questionAnswered},
		{Contract: "plugin", ID: "yoke:plugin.10", Title: "an occurrence is carried at the author's severity",
			Cites:        []string{"specs/50.64", "specs/50.104", "specs/90.34", "arch/50-plugin-surface/05 §The event family is where a unit declares a severity"},
			Precondition: "the harness of case 8, granted everything, and a subscription to `unit.occurrence.reported` on the administrative surface",
			Issues:       "`report` of the occurrence its Manifest declares, at 70, with a line",
			Requires:     "an event about the unit carrying the occurrence, severity 70 and the unit as its actor",
			Run:          occurrenceCarried},
		{Contract: "plugin", ID: "yoke:plugin.11", Title: "a health report is carried as the unit graded it",
			Cites:        []string{"specs/50.65", "specs/50.66", "specs/90.34", "arch/50-plugin-surface/05 §What a health report carries"},
			Precondition: "the harness of case 8, and a subscription to `unit.condition.changed` on the administrative surface",
			Issues:       "`report-health` at 80, with a line",
			Requires:     "an event about the unit at severity 80, with the unit as its actor",
			Run:          healthCarried},
		{Contract: "plugin", ID: "yoke:plugin.12", Title: "disabling the plugin revokes the Session, and the process ends",
			Cites:        []string{"specs/50.49", "specs/50.51", "specs/60.29", "specs/90.29", "arch/50-plugin-surface/04 §Revocation"},
			Precondition: "the harness of case 8, its Session open",
			Issues:       "the plugin disabled on the administrative surface",
			Requires:     "the end observed as a revocation, the plugin disabled, and then the harness gone",
			Run:          disabledRevoked},
	}
}

func notYetFamily(r *Run) Outcome { return Fail("", "the case to be performed", "nothing yet") }

// std: yoke:plugin.07
func occurrenceWithheld(r *Run) Outcome { return notYetFamily(r) }

// std: yoke:plugin.08
func grantedAtNextLife(r *Run) Outcome { return notYetFamily(r) }

// std: yoke:plugin.09
func questionAnswered(r *Run) Outcome { return notYetFamily(r) }

// std: yoke:plugin.10
func occurrenceCarried(r *Run) Outcome { return notYetFamily(r) }

// std: yoke:plugin.11
func healthCarried(r *Run) Outcome { return notYetFamily(r) }

// std: yoke:plugin.12
func disabledRevoked(r *Run) Outcome { return notYetFamily(r) }
