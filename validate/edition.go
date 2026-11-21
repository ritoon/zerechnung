package validate

import (
	"strings"

	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/invoice"
)

// ruleEdition reports a specification identifier (BT-24) that names an
// edition of EN 16931-1 the package does not support, such as
// EN 16931-1:2026.
const ruleEdition = "EN16931-EDITION-01"

// editionCatalog lists ruleEdition, part of the EN16931 rule set but
// reported whatever the rule sets.
var editionCatalog = []RuleInfo{
	{ID: ruleEdition, Severity: Error, Description: "The specification identifier (BT-24) names EN 16931-1:2017, or no edition (MINIMUM, BASIC WL): the rules of the package implement EN 16931-1:2017, and the other editions, such as EN 16931-1:2026 whose validation artefacts are not published yet, are not supported."},
}

// checkEdition reports a specification identifier (BT-24) that names an
// edition of EN 16931-1 other than 2017, and tells whether the other rules
// must be skipped: they implement EN 16931-1:2017 and would give findings
// that do not hold for another edition, so they only apply when
// opts.Profile forces a profile.
func checkEdition(doc *rules.Doc, opts *Options, r *Report) (skip bool) {
	urn := strings.TrimSpace(doc.SpecificationID())
	e := invoice.EditionOf(urn)
	if e.Supported() {
		return false
	}
	c := &rules.Checker{Doc: doc, Report: r, Source: SourceEN16931}
	n := doc.Context.Elem("GuidelineSpecifiedDocumentContextParameter").Elem("ID")
	if opts.Profile != invoice.ProfileUnknown {
		c.Fail(ruleEdition, Error, n, "the specification identifier (BT-24) %q names %s, which ZeRechnung does not support yet: the rules of EN 16931-1:2017 for the profile %s are applied as requested, and may not hold for this edition",
			urn, e, opts.Profile)
		return false
	}
	c.Fail(ruleEdition, Error, n, "the specification identifier (BT-24) %q names %s, which ZeRechnung does not support yet: no other rule is applied, the rules of the package implementing EN 16931-1:2017. Force a profile (Options.Profile, zerechnung validate -profile) to apply them anyway",
		urn, e)
	return true
}
