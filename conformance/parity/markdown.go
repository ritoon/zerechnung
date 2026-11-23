package main

import (
	"fmt"
	"slices"
	"sort"
	"strings"
)

// errorRules returns the compared rules raised as errors, sorted.
func errorRules(r result) []string {
	var out []string
	for _, rule := range sortedRules(r.Rules) {
		if r.Rules[rule] == levelError {
			out = append(out, rule)
		}
	}
	return out
}

// sameError reports whether both sides raise at least one common error.
func sameError(c comparison) bool {
	for _, r := range errorsOf(c.Ref.Rules) {
		for _, m := range errorsOf(c.Ours.Rules) {
			if same(r, m) {
				return true
			}
		}
	}
	return false
}

// level gives a difference with the levels at which each side raises the
// rule (or its equivalent), this package first, "—" for a rule not raised,
// unless one side raises it as an error and the other not at all:
// "+CII-SR-465" → "`+CII-SR-465` (error / warning)".
func level(c comparison, d string) string {
	if d == "" || (d[0] != '+' && d[0] != '-') {
		return "`" + d + "`"
	}
	rule := d[1:]
	var mine, ref string
	if d[0] == '+' {
		mine = c.Ours.Rules[rule]
		for r, l := range c.Ref.Rules {
			if same(r, rule) && rank(l) > rank(ref) {
				ref = l
			}
		}
	} else {
		ref = c.Ref.Rules[rule]
		for m, l := range c.Ours.Rules {
			if same(rule, m) && rank(l) > rank(mine) {
				mine = l
			}
		}
	}
	if mine == levelError && ref == "" || mine == "" && ref == levelError {
		return "`" + d + "`"
	}
	dash := func(l string) string {
		if l == "" {
			return "—"
		}
		return l
	}
	return fmt.Sprintf("`%s` (%s / %s)", d, dash(mine), dash(ref))
}

func cell(rules []string) string {
	if len(rules) == 0 {
		return "—"
	}
	return "`" + strings.Join(rules, "` `") + "`"
}

func isVariant(c comparison) bool { return strings.HasPrefix(c.Invoice, "variants/") }

func markdown(all []comparison, never []neverFired, unused []string, coverage []kositCoverage) string {
	var b strings.Builder
	b.WriteString("| Référence | Factures comparées | Mêmes règles | Écarts justifiés | Écarts inattendus | Même verdict |\n|---|---|---|---|---|---|\n")
	type row struct {
		title    string
		key      string
		variants bool
	}
	var rows []row
	for _, ref := range references {
		rows = append(rows, row{ref.Title, ref.Key, false})
		if ref.Key == "kosit" && slices.ContainsFunc(all, isVariant) {
			rows = append(rows, row{ref.Title + " : variants des tests de mutation", ref.Key, true})
		}
	}
	for _, ref := range rows {
		var n, same, known, unexpected, verdict int
		for _, c := range all {
			if c.Reference != ref.key || isVariant(c) != ref.variants {
				continue
			}
			n++
			switch {
			case len(c.Unexpected) > 0:
				unexpected++
			case len(c.Known) > 0:
				known++
			default:
				same++
			}
			if c.Ours.valid() == c.Ref.valid() {
				verdict++
			}
		}
		fmt.Fprintf(&b, "| %s | %d | %d | %d | %d | %d |\n", ref.title, n, same, known, unexpected, verdict)
	}
	b.WriteString("\nPour chaque facture, les règles levées en erreur de part et d'autre sont comparées : celles des schematrons officiels (EN 16931, Factur-X, XRechnung, Peppol), sous leur identifiant ou, pour les listes de codes des schematrons Factur-X, sous celui de la règle CEN équivalente, et la présence d'erreurs de schéma XSD (`SCHEMA`). Avec les variants, les XSLT de la NPa et celles de la FNFE-MPE (EXTENDED-CTC-FR et statuts BR-FR-CDV), les règles levées en avertissement le sont aussi ; avec les schematrons CIUS-PT, CIUS-RO et CIUS-HR, les règles levées en avertissement aussi, et sur le même élément (voir plus bas). Le verdict est valide quand aucune de ces règles n'est en erreur.\n")
	variantSection(&b, all, coverage)
	npaSection(&b, all, never)
	fnfeSection(&b, all)
	cdvSection(&b, all)
	ptSection(&b, all)
	roSection(&b, all)
	hrSection(&b, all)

	// Negative cases: one line per invoice, one column per reference.
	type neg struct {
		refs map[string]comparison
		ours result
	}
	negs := map[string]*neg{}
	var names []string
	for _, c := range all {
		if !c.Negative {
			continue
		}
		n := negs[c.Invoice]
		if n == nil {
			n = &neg{refs: map[string]comparison{}, ours: c.Ours}
			negs[c.Invoice] = n
			names = append(names, c.Invoice)
		}
		n.refs[c.Reference] = c
	}
	sort.Strings(names)
	if len(names) > 0 {
		b.WriteString("\n### Cas négatifs officiels\n\nFactures invalides à dessein : dossiers `fail` du corpus ZUGFeRD et exemple `not_validating` de `XML-Rechnung/UBL`. Erreurs levées, parmi les règles comparées.\n\n| Facture | ZeRechnung |")
		for _, ref := range references {
			fmt.Fprintf(&b, " %s |", ref.Key)
		}
		b.WriteString(" Même erreur |\n|---|---|")
		for range references {
			b.WriteString("---|")
		}
		b.WriteString("---|\n")
		for _, name := range names {
			n := negs[name]
			fmt.Fprintf(&b, "| `%s` | %s |", strings.TrimPrefix(name, "corpus/"), cell(errorRules(n.ours)))
			verdict := "—"
			for _, ref := range references {
				c, ok := n.refs[ref.Key]
				if !ok {
					b.WriteString(" hors périmètre |")
					continue
				}
				fmt.Fprintf(&b, " %s |", cell(errorRules(c.Ref)))
				if len(errorRules(c.Ref)) > 0 {
					if sameError(c) {
						if verdict == "—" {
							verdict = "oui"
						}
					} else {
						verdict = "non"
					}
				}
			}
			fmt.Fprintf(&b, " %s |\n", verdict)
		}
	}

	var knownLines, unexpectedLines []string
	for _, c := range all {
		for _, k := range c.Known {
			d, why, _ := strings.Cut(k, " : ")
			knownLines = append(knownLines, fmt.Sprintf("| %s | `%s` | %s | %s |", c.Reference, strings.TrimPrefix(c.Invoice, "corpus/"), level(c, d), why))
		}
		if len(c.Unexpected) > 0 {
			var ds []string
			for _, d := range c.Unexpected {
				ds = append(ds, level(c, d))
			}
			unexpectedLines = append(unexpectedLines, fmt.Sprintf("| %s | `%s` | %s |", c.Reference, strings.TrimPrefix(c.Invoice, "corpus/"), strings.Join(ds, " ")))
		}
	}
	const legend = "`+` : règle levée par ZeRechnung seul, ou à un niveau plus élevé ; `-` : par la référence seule, ou à un niveau plus élevé ; entre parenthèses, les niveaux chez ZeRechnung puis chez la référence (— : règle non levée), sauf pour une erreur d'un côté et rien de l'autre. Pour les schematrons CIUS-PT, CIUS-RO et CIUS-HR, `RÈGLE@ÉLÉMENT` : la règle et l'élément où elle est levée.\n\n"
	if len(knownLines) > 0 {
		b.WriteString("\n### Écarts justifiés\n\n" + legend + "| Référence | Facture | Écart | Raison |\n|---|---|---|---|\n")
		b.WriteString(strings.Join(knownLines, "\n") + "\n")
	}
	if len(unexpectedLines) > 0 {
		b.WriteString("\n### Écarts inattendus\n\n" + legend + "| Référence | Facture | Écarts |\n|---|---|---|\n")
		b.WriteString(strings.Join(unexpectedLines, "\n") + "\n")
	}
	if len(unused) > 0 {
		b.WriteString("\n### Écarts connus devenus sans objet\n\n")
		for _, u := range unused {
			b.WriteString("- " + u + "\n")
		}
	}
	return b.String()
}

// variantSection writes the part of the report on the variants, the
// documents of the mutation cases of the EN 16931 and XRechnung rules, and
// the share of the assertions of the KoSIT XSLT that the invoices raise.
func variantSection(b *strings.Builder, all []comparison, coverage []kositCoverage) {
	byPkg := map[string]int{}
	var pkgs []string
	for _, c := range all {
		if c.Reference != "kosit" || !isVariant(c) {
			continue
		}
		pkg, _, _ := strings.Cut(strings.TrimPrefix(c.Invoice, "variants/"), "/")
		if byPkg[pkg] == 0 {
			pkgs = append(pkgs, pkg)
		}
		byPkg[pkg]++
	}
	if len(pkgs) == 0 && len(coverage) == 0 {
		return
	}
	b.WriteString("\n### Variants EN 16931 et XRechnung : validateur KoSIT\n\n")
	if len(pkgs) > 0 {
		sort.Strings(pkgs)
		var parts []string
		for _, p := range pkgs {
			parts = append(parts, fmt.Sprintf("%d de `internal/rules/%s`", byPkg[p], p))
		}
		fmt.Fprintf(b, "Les variants sont les documents des tests de mutation des règles (%s) : chacun casse une ou plusieurs règles d'une facture valide, ou vérifie qu'un cas limite n'en casse aucune. Le validateur KoSIT les reçoit tous ; les règles y sont comparées à tous les niveaux, erreurs, avertissements et informations. Chaque variant doit aussi lever chez KoSIT les règles que son test attend (`?RÈGLE` sinon) : un variant qui cesse de tester sa règle fait échouer le job.\n", strings.Join(parts, ", "))
	}
	var failed []string
	for _, c := range all {
		if c.Reference == "kosit" && strings.HasPrefix(c.Ref.Stopped, kositFailure) {
			failed = append(failed, fmt.Sprintf("| `%s` | %s |", c.Invoice, clip(strings.TrimPrefix(c.Ref.Stopped, kositFailure))))
		}
	}
	if len(failed) > 0 {
		b.WriteString("\nKoSIT n'a pas pu évaluer ces documents : un schematron s'est arrêté sur une erreur dynamique, que seul le tableau de son journal signale (le rapport XML montre une étape valide, sans message). Seules les règles que KoSIT y lève sont comparées.\n\n| Facture | Erreur |\n|---|---|\n")
		b.WriteString(strings.Join(failed, "\n") + "\n")
	}
	if len(coverage) == 0 {
		return
	}
	b.WriteString("\nAssertions des XSLT de la configuration KoSIT levées, à n'importe quel niveau, sur au moins une facture :\n\n| XSLT | Assertions | Corpus et suite KoSIT | Variants | Au total |\n|---|---|---|---|---|\n")
	var total, raised int
	for _, c := range coverage {
		fmt.Fprintf(b, "| `%s` | %d | %d | %d | %d (%s) |\n", c.Stylesheet, c.Assertions, c.Corpus, c.Variants, c.Raised, percent(c.Raised, c.Assertions))
		total += c.Assertions
		raised += c.Raised
	}
	fmt.Fprintf(b, "| **Total** | %d | | | %d (%s) |\n", total, raised, percent(raised, total))
	b.WriteString("\nPar famille de règles (levées / assertions) :\n\n| XSLT |")
	var names []string
	seen := map[string]bool{}
	for _, f := range families {
		for _, c := range coverage {
			if c.byFamily[f.name] > 0 && !seen[f.name] {
				seen[f.name] = true
				names = append(names, f.name)
			}
		}
	}
	for _, c := range coverage {
		if c.byFamily["autres"] > 0 && !seen["autres"] {
			seen["autres"] = true
			names = append(names, "autres")
		}
	}
	for _, n := range names {
		fmt.Fprintf(b, " %s |", n)
	}
	b.WriteString("\n|---|")
	for range names {
		b.WriteString("---|")
	}
	b.WriteString("\n")
	for _, c := range coverage {
		fmt.Fprintf(b, "| `%s` |", c.Stylesheet)
		for _, n := range names {
			cell := "—"
			for _, f := range c.Families {
				if f.Family == n {
					cell = fmt.Sprintf("%d / %d", f.Raised, f.Assertions)
				}
			}
			fmt.Fprintf(b, " %s |", cell)
		}
		b.WriteString("\n")
	}
	for _, c := range coverage {
		if len(c.Never) == 0 {
			continue
		}
		fmt.Fprintf(b, "\n<details><summary>%s : %d assertions jamais levées</summary>\n\n%s\n\n</details>\n", c.Stylesheet, len(c.Never), "`"+strings.Join(c.Never, "` `")+"`")
	}
}

func percent(n, total int) string {
	if total == 0 {
		return "—"
	}
	return fmt.Sprintf("%.1f %%", 100*float64(n)/float64(total))
}

// npaSection writes the part of the report on the NPa XSLT: the invoices,
// the transformations stopped on a dynamic error, and the NLCIUS assertions
// that never fail.
func npaSection(b *strings.Builder, all []comparison, never []neverFired) {
	var tests, repo, variants int
	var stopped []string
	for _, c := range all {
		if c.Reference != "npa" {
			continue
		}
		switch {
		case strings.HasPrefix(c.Invoice, "nl/simplerinvoicing/"):
			tests++
		case strings.HasPrefix(c.Invoice, "nl/zerechnung/"):
			repo++
		default:
			variants++
		}
		if c.Ref.Stopped != "" {
			stopped = append(stopped, fmt.Sprintf("| `%s` | %s |", strings.TrimPrefix(c.Invoice, "nl/"), c.Ref.Stopped))
		}
	}
	if tests+repo+variants == 0 {
		return
	}
	fmt.Fprintf(b, "\n### NLCIUS : XSLT de la NPa\n\nSaxon applique à chaque facture la XSLT qu'un validateur choisit : NLCIUS-CII pour une facture CII, SI-UBL 2.0 pour une facture UBL, ou son extension G-rekening quand le `cbc:CustomizationID` la déclare. Factures : %d fichiers de test SimplerInvoicing (SI-UBL 2.0.3.2, NLCIUS-CII 1.0.3, G-rekening 1.0), %d factures NLCIUS du dépôt et %d variantes, les documents des tests de mutation de `internal/rules/nl`.\n\n", tests, repo, variants)
	b.WriteString("Les règles levées en erreur et en avertissement sont comparées sous leur identifiant : celles des motifs NLCIUS (BR-NL, BR-GA, SI-V20-INV-R000, BII2-T10-R001, contrôle des éléments vides) et les règles EN 16931 que chaque XSLT inclut (CEN 1.3.16). ZeRechnung valide avec les règles néerlandaises (`validate.Netherlands`) ; ses constats de schéma XSD, de profil Factur-X et Peppol, que les XSLT n'ont pas, restent hors de la comparaison.\n")
	if len(stopped) > 0 {
		b.WriteString("\nArrêts sur erreur dynamique : la transformation s'arrête, et seule l'assertion qui l'a levée est comparée ; ZeRechnung la tient pour échouée.\n\n| Facture | Arrêt |\n|---|---|\n")
		b.WriteString(strings.Join(stopped, "\n") + "\n")
	}
	if len(never) > 0 {
		b.WriteString("\nAssertions des motifs NLCIUS qu'aucune facture ne fait échouer :\n\n| Syntaxe / motif | Assertion | Niveau | Message | Justification |\n|---|---|---|---|---|\n")
		for _, n := range never {
			why := n.Known
			if why == "" {
				why = "**non justifiée**"
			}
			fmt.Fprintf(b, "| %s | `%s` | %s | %s | %s |\n", n.where(), n.ID, n.Flag, clip(n.Message), why)
		}
	}
}

// schGroup is an origin of the files of the comparison with a schematron
// compiled by SchXslt, and its title in the report.
type schGroup struct{ prefix, title string }

// ptGroups are the origins of the files of the CIUS-PT comparison, in the
// order of the report.
var ptGroups = []schGroup{
	{"pt/espap/", "Exemples d'eSPap (`FX_PT`)"},
	{"pt/zerechnung/", "Factures du dépôt (`internal/rules/pt/testdata`)"},
	{"pt/variants/", "Variantes de ces factures (`go test ./internal/rules/pt -run '^TestVariants$' -variants DIR`)"},
}

// roGroups are the origins of the files of the CIUS-RO comparison, in the
// order of the report.
var roGroups = []schGroup{
	{"ro/anaf/", "Exemples UBL de l'ANAF (`FX_RO`)"},
	{"ro/zerechnung/", "Factures UBL du dépôt (`internal/rules/ro/testdata`)"},
	{"ro/variants/", "Variantes de ces factures (`go test ./internal/rules/ro -run '^TestVariants$' -variants DIR`)"},
}

// hrGroups are the origins of the files of the CIUS-HR comparison, in the
// order of the report.
var hrGroups = []schGroup{
	{"hr/porezna/", "Exemples de la Porezna uprava (`FX_HR`)"},
	{"hr/zerechnung/", "Factures du dépôt (`internal/rules/hr/testdata`)"},
	{"hr/variants/", "Variantes de ces factures (`go test ./internal/rules/hr -run '^TestVariants$' -variants DIR`)"},
}

// schSection is the part of the report on a schematron compiled by SchXslt.
type schSection struct {
	key     string // reference
	groups  []schGroup
	title   string // of the section
	intro   string
	stopped string // header of the column of the transformations stopped
	stops   string // introduction of the table of the stops
}

// ptSection writes the part of the report on the CIUS-PT schematron.
func ptSection(b *strings.Builder, all []comparison) {
	schSection{
		key:     "pt",
		groups:  ptGroups,
		title:   "CIUS-PT : schematron d'eSPap",
		intro:   "Saxon applique à chaque fichier le schematron CIUS-PT 2.1.1 d'eSPap compilé en XSLT par SchXslt 1.10.1. ZeRechnung valide chaque fichier comme une facture CIUS-PT (`validate.Options.Profile`), quel que soit son BT-24, et garde les constats des règles du schematron : jeu EN 16931, que le schematron remplace pour une facture CIUS-PT, et jeu `Portugal` sauf ses propres contrôles (PT-NIF, PT-SYNTAX). Les règles levées en erreur et en avertissement sont comparées, chacune sur l'élément où elle est levée.",
		stopped: "Arrêts de Saxon, assertion tenue pour échouée par ZeRechnung",
		stops:   "Arrêts sur une erreur de type XPath dans le test d'une assertion : Saxon arrête la transformation, et son rapport ne contient que l'erreur et sa ligne dans la XSLT. ZeRechnung tient l'assertion pour échouée (voir la documentation de `internal/schematron`) : seule cette assertion est comparée, sans son élément, que Saxon ne donne pas.",
	}.write(b, all)
}

// roSection writes the part of the report on the CIUS-RO schematron.
func roSection(b *strings.Builder, all []comparison) {
	schSection{
		key:     "ro",
		groups:  roGroups,
		title:   "CIUS-RO : schematron de l'ANAF",
		intro:   "Saxon applique à chaque fichier UBL la phase ROmodel du schematron CIUS-RO 1.0.9 de l'ANAF, celle des règles roumaines, compilée en XSLT par SchXslt 1.10.1. ZeRechnung valide chaque fichier comme une facture CIUS-RO (`validate.Options.Profile`), quel que soit son BT-24, et garde les constats du jeu `Romania` sauf ses propres contrôles du CIF (RO-CIF). Les règles BR-RO et BR-DEC-RO levées en erreur et en avertissement sont comparées, chacune sur l'élément où elle est levée ; BR-27, règle d'EN 16931 que la phase reprend, est laissée au jeu EN 16931. Le schematron ne lie les règles qu'à UBL : leur transposition en CII n'a pas de référence.",
		stopped: "Arrêts de Saxon",
		stops:   "Arrêts sur une erreur de type XPath dans le test d'une assertion : Saxon arrête la transformation, et son rapport ne contient que l'erreur et sa ligne dans la XSLT. Pour une règle du CIUS-RO, ZeRechnung tient l'assertion pour échouée (voir la documentation de `internal/schematron`) : seule cette assertion est comparée, sans son élément, que Saxon ne donne pas. Pour BR-27, hors de la comparaison, rien n'est comparé.",
	}.write(b, all)
}

// hrSection writes the part of the report on the CIUS-HR schematron.
func hrSection(b *strings.Builder, all []comparison) {
	schSection{
		key:     "hr",
		groups:  hrGroups,
		title:   "CIUS-HR : schematron de la Porezna uprava",
		intro:   "Saxon applique à chaque fichier le schematron CIUS-HR et EXT-HR 2025 de la Porezna uprava (Fiskalizacija 2.0, version du 13.03.2026), compilé en XSLT par SchXslt 1.10.1. ZeRechnung valide chaque fichier comme une facture CIUS-HR (`validate.Options.Profile`), quel que soit son BT-24, et garde les constats du jeu `Croatia` sauf ses propres contrôles (HR-OIB-01, l'OIB d'une adresse électronique du schéma 9934, et HR-SYNTAX-01, une facture CII). Les règles HR-BR levées en erreur et en avertissement sont comparées, chacune sur l'élément où elle est levée. Le CIUS-HR n'est lié qu'à UBL.",
		stopped: "Arrêts de Saxon, assertion tenue pour échouée par ZeRechnung",
		stops:   "Arrêts sur une erreur de type XPath dans le test d'une assertion : Saxon arrête la transformation, et son rapport ne contient que l'erreur et sa ligne dans la XSLT. ZeRechnung tient l'assertion pour échouée (voir la documentation de `internal/schematron`) : seule cette assertion est comparée, sans son élément, que Saxon ne donne pas.",
	}.write(b, all)
}

// stoppedAssertion returns the assertion whose test stopped a
// transformation, from the Stopped of its result ("CODE in the test of ID:
// message").
func stoppedAssertion(stopped string) string {
	_, rest, ok := strings.Cut(stopped, " in the test of ")
	if !ok {
		return ""
	}
	id, _, _ := strings.Cut(rest, ":")
	return id
}

// write writes the files by origin, and the transformations stopped on a
// dynamic error, counted by error code and by assertion.
func (s schSection) write(b *strings.Builder, all []comparison) {
	type count struct{ n, same, stopped, known, unexpected int }
	counts := make([]count, len(s.groups))
	stops := map[string]map[string]int{} // error code → assertion → files
	total := 0
	for _, c := range all {
		if c.Reference != s.key {
			continue
		}
		g := len(s.groups) - 1
		for i, pg := range s.groups {
			if strings.HasPrefix(c.Invoice, pg.prefix) {
				g = i
				break
			}
		}
		k := &counts[g]
		k.n++
		total++
		switch {
		case len(c.Unexpected) > 0:
			k.unexpected++
		case len(c.Known) > 0:
			k.known++
		case c.Ref.Stopped != "":
			k.stopped++
		default:
			k.same++
		}
		if c.Ref.Stopped != "" {
			code, _, _ := strings.Cut(c.Ref.Stopped, " ")
			if stops[code] == nil {
				stops[code] = map[string]int{}
			}
			stops[code][stoppedAssertion(c.Ref.Stopped)]++
		}
	}
	if total == 0 {
		return
	}
	fmt.Fprintf(b, "\n### %s\n\n%s\n\n", s.title, s.intro)
	fmt.Fprintf(b, "| Fichiers | Comparés | Mêmes règles, aux mêmes éléments | %s | Écarts justifiés | Écarts inattendus |\n|---|---|---|---|---|---|\n", s.stopped)
	for i, pg := range s.groups {
		k := counts[i]
		fmt.Fprintf(b, "| %s | %d | %d | %d | %d | %d |\n", pg.title, k.n, k.same, k.stopped, k.known, k.unexpected)
	}
	if len(stops) == 0 {
		return
	}
	fmt.Fprintf(b, "\n%s\n\n| Erreur | Fichiers | Assertions (fichiers) |\n|---|---|---|\n", s.stops)
	why := map[string]string{
		"XPTY0004": "plusieurs valeurs là où le test n'en attend qu'une",
		"FORG0001": "valeur qui ne se convertit pas en nombre",
	}
	for _, code := range sortedRules(stops) {
		n := 0
		var rules []string
		for _, rule := range sortedRules(stops[code]) {
			n += stops[code][rule]
			rules = append(rules, fmt.Sprintf("`%s` (%d)", rule, stops[code][rule]))
		}
		label := "`" + code + "`"
		if w, ok := why[code]; ok {
			label += " : " + w
		}
		fmt.Fprintf(b, "| %s | %d | %s |\n", label, n, strings.Join(rules, ", "))
	}
}
