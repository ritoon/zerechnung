package invoice

import (
	"strconv"
	"strings"
)

// Edition is an edition of EN 16931-1, the European standard that defines
// the semantic data model of an electronic invoice, identified by its year.
// A specification identifier (BT-24) names the edition in its first part:
// urn:cen.eu:en16931:2017 for the 2017 edition, which every profile of this
// package implements.
type Edition int

// Editions of EN 16931-1.
const (
	// EditionNone is the edition of an identifier that names none: the
	// Factur-X MINIMUM and BASIC WL identifiers, whose invoices are not
	// EN 16931 invoices, the ZUGFeRD 1.0 ones and unknown identifiers.
	EditionNone Edition = 0
	// Edition2017 is EN 16931-1:2017, the edition the package implements.
	Edition2017 Edition = 2017
	// Edition2026 is EN 16931-1:2026, approved on 18 March 2026 and not
	// backward compatible with the 2017 edition. Its validation artefacts and
	// syntax bindings are not published yet: the package recognizes it, but
	// neither validates nor writes it.
	Edition2026 Edition = 2026
)

// enPrefix starts the specification identifier of every edition of
// EN 16931-1, in lower case.
const enPrefix = "urn:cen.eu:en16931:"

// EditionOf returns the edition of EN 16931-1 named by a specification
// identifier (BT-24): the year after "urn:cen.eu:en16931:", alone or
// followed by an extension ("#compliant#…", "#conformant#…", or ":" in an
// obsolete spelling). Case and surrounding white space are ignored. Any year
// is returned, so that an edition published after this package is
// recognized too; an identifier that names no edition gives EditionNone.
func EditionOf(urn string) Edition {
	u := strings.ToLower(strings.TrimSpace(urn))
	rest, ok := strings.CutPrefix(u, enPrefix)
	if !ok || len(rest) < 4 {
		return EditionNone
	}
	if len(rest) > 4 && rest[4] != '#' && rest[4] != ':' {
		return EditionNone
	}
	year, err := strconv.Atoi(rest[:4])
	if err != nil || rest[0] < '1' || rest[0] > '9' {
		return EditionNone
	}
	return Edition(year)
}

// String returns the reference of the edition, such as "EN 16931-1:2026",
// or "" for EditionNone.
func (e Edition) String() string {
	if e == EditionNone {
		return ""
	}
	return "EN 16931-1:" + strconv.Itoa(int(e))
}

// Supported reports whether the package validates and writes invoices whose
// specification identifier names the edition: EN 16931-1:2017, and the
// identifiers that name no edition (MINIMUM, BASIC WL).
func (e Edition) Supported() bool { return e == EditionNone || e == Edition2017 }
