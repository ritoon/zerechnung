package sk

import "strings"

// The Slovak identifiers of a party:
//
//   - the tax identification number (DIČ, daňové identifikačné číslo), ten
//     digits, which the tax administrator assigns to every taxable person
//     and which identifies a Slovak participant in Peppol (scheme 0245);
//   - the VAT identification number (IČ DPH, identifikačné číslo pre daň),
//     SK followed by ten digits, assigned to a VAT payer;
//   - the identification number of the organisation (IČO, identifikačné
//     číslo organizácie), eight digits, assigned to a legal person or an
//     entrepreneur; no ISO 6523 scheme identifies it (0154 and 0158 are the
//     Czech IČO).
//
// No official source publishes a check digit for them: only their format is
// checked.

// ValidDIC reports whether s is a DIČ: ten digits.
func ValidDIC(s string) bool { return digits(s, 10) }

// ValidICDPH reports whether s is an IČ DPH: SK followed by ten digits.
func ValidICDPH(s string) bool {
	rest, ok := strings.CutPrefix(s, "SK")
	return ok && digits(rest, 10)
}

// ValidICO reports whether s is an IČO: eight digits.
func ValidICO(s string) bool { return digits(s, 8) }

// digits reports whether s has n ASCII digits and nothing else.
func digits(s string, n int) bool {
	if len(s) != n {
		return false
	}
	for i := range len(s) {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// companyForms are the legal forms of the companies and cooperatives that
// the Commercial Code registers in the commercial register, and that their
// business name has to give (§ 57(1) for the general partnership, § 78 for
// the limited partnership, § 107(1) for the limited liability company,
// § 154(1) for the joint-stock company, § 220a(2) for the simple
// joint-stock company, § 221(2) for the cooperative): the abbreviations, in
// lower case and without spaces, that end a name, and the full names.
var companyForms = []struct {
	form     string
	suffixes []string
	names    []string
}{
	{"s.r.o.", []string{"s.r.o.", "s.r.o", "spol.sr.o.", "spol.sr.o"}, []string{"spoločnosťsručenímobmedzeným"}},
	{"a.s.", []string{"a.s.", "a.s"}, []string{"akciováspoločnosť"}},
	{"j.s.a.", []string{"j.s.a.", "j.s.a"}, []string{"jednoducháspoločnosťnaakcie"}},
	{"k.s.", []string{"k.s.", "k.s", "kom.spol."}, []string{"komanditnáspoločnosť"}},
	{"v.o.s.", []string{"v.o.s.", "v.o.s", "ver.obch.spol."}, []string{"verejnáobchodnáspoločnosť"}},
	{"družstvo", nil, []string{"družstvo"}},
}

// CompanyForm returns the legal form of a company of the commercial register
// that the business name gives ("s.r.o.", "a.s.", "j.s.a.", "k.s.",
// "v.o.s." or "družstvo"), "" when it gives none: an abbreviation at the
// end of the name, with or without spaces, or the full name of the form.
func CompanyForm(name string) string {
	compact := strings.ToLower(strings.Join(strings.Fields(name), ""))
	if compact == "" {
		return ""
	}
	for _, f := range companyForms {
		for _, s := range f.suffixes {
			// The abbreviation is a word of its own: the name does not
			// end in a word that merely ends with its letters.
			if rest, ok := strings.CutSuffix(compact, s); ok && boundary(name, len(s)) && rest != "" {
				return f.form
			}
		}
		for _, n := range f.names {
			if strings.Contains(compact, n) {
				return f.form
			}
		}
	}
	return ""
}

// boundary reports whether the abbreviation that ends name, whose compact
// form has n bytes, starts a word: what precedes it in name is a space, a
// comma or a dot.
func boundary(name string, n int) bool {
	name = strings.TrimRight(name, " \t\r\n")
	// Walk back over the n non-space bytes of the abbreviation.
	i := len(name)
	for n > 0 && i > 0 {
		i--
		if c := name[i]; c != ' ' && c != '\t' && c != '\r' && c != '\n' {
			n--
		}
	}
	if i == 0 {
		return false
	}
	switch name[i-1] {
	case ' ', '\t', '\r', '\n', ',', '.':
		return true
	}
	return false
}
