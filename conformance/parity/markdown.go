package main

import (
	"fmt"
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

func markdown(all []comparison, never []neverFired, unused []string) string {
	var b strings.Builder
	b.WriteString("| Référence | Factures comparées | Mêmes règles | Écarts justifiés | Écarts inattendus | Même verdict |\n|---|---|---|---|---|---|\n")
	for _, ref := range references {
		var n, same, known, unexpected, verdict int
		for _, c := range all {
			if c.Reference != ref.Key {
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
		fmt.Fprintf(&b, "| %s | %d | %d | %d | %d | %d |\n", ref.Title, n, same, known, unexpected, verdict)
	}
	b.WriteString("\nPour chaque facture, les règles levées en erreur de part et d'autre sont comparées : celles des schematrons officiels (EN 16931, Factur-X, XRechnung, Peppol), sous leur identifiant ou, pour les listes de codes des schematrons Factur-X, sous celui de la règle CEN équivalente, et la présence d'erreurs de schéma XSD (`SCHEMA`). Avec les XSLT de la NPa, les règles levées en avertissement le sont aussi ; avec le schematron CIUS-PT, les règles levées en avertissement aussi, et sur le même élément (voir plus bas). Le verdict est valide quand aucune de ces règles n'est en erreur.\n")
	npaSection(&b, all, never)
	ptSection(&b, all)

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
	const legend = "`+` : règle levée par ZeRechnung seul, ou à un niveau plus élevé ; `-` : par la référence seule, ou à un niveau plus élevé ; entre parenthèses, les niveaux chez ZeRechnung puis chez la référence (— : règle non levée), sauf pour une erreur d'un côté et rien de l'autre. Pour le schematron CIUS-PT, `RÈGLE@ÉLÉMENT` : la règle et l'élément où elle est levée.\n\n"
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

// ptGroups are the origins of the files of the CIUS-PT comparison, in the
// order of the report.
var ptGroups = []struct{ prefix, title string }{
	{"pt/espap/", "Exemples d'eSPap (`FX_PT`)"},
	{"pt/zerechnung/", "Factures du dépôt (`internal/rules/pt/testdata`)"},
	{"pt/variants/", "Variantes de ces factures (`go test ./internal/rules/pt -run '^TestVariants$' -variants DIR`)"},
}

// ptSection writes the part of the report on the CIUS-PT schematron: the
// files by origin, and the transformations stopped on a dynamic error,
// counted by error code and by assertion.
func ptSection(b *strings.Builder, all []comparison) {
	type count struct{ n, same, stopped, known, unexpected int }
	counts := make([]count, len(ptGroups))
	stops := map[string]map[string]int{} // error code → assertion → files
	total := 0
	for _, c := range all {
		if c.Reference != "pt" {
			continue
		}
		g := len(ptGroups) - 1
		for i, pg := range ptGroups {
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
			for rule := range c.Ref.Rules {
				stops[code][rule]++
			}
		}
	}
	if total == 0 {
		return
	}
	b.WriteString("\n### CIUS-PT : schematron d'eSPap\n\nSaxon applique à chaque fichier le schematron CIUS-PT 2.1.1 d'eSPap compilé en XSLT par SchXslt 1.10.1. ZeRechnung valide chaque fichier comme une facture CIUS-PT (`validate.Options.Profile`), quel que soit son BT-24, et garde les constats des règles du schematron : jeu EN 16931, que le schematron remplace pour une facture CIUS-PT, et jeu `Portugal` sauf ses propres contrôles (PT-NIF, PT-SYNTAX). Les règles levées en erreur et en avertissement sont comparées, chacune sur l'élément où elle est levée.\n\n")
	b.WriteString("| Fichiers | Comparés | Mêmes règles, aux mêmes éléments | Arrêts de Saxon, assertion tenue pour échouée par ZeRechnung | Écarts justifiés | Écarts inattendus |\n|---|---|---|---|---|---|\n")
	for i, pg := range ptGroups {
		k := counts[i]
		fmt.Fprintf(b, "| %s | %d | %d | %d | %d | %d |\n", pg.title, k.n, k.same, k.stopped, k.known, k.unexpected)
	}
	if len(stops) == 0 {
		return
	}
	b.WriteString("\nArrêts sur une erreur de type XPath dans le test d'une assertion : Saxon arrête la transformation, et son rapport ne contient que l'erreur et sa ligne dans la XSLT. ZeRechnung tient l'assertion pour échouée (voir la documentation de `internal/rules/pt`) : seule cette assertion est comparée, sans son élément, que Saxon ne donne pas.\n\n| Erreur | Fichiers | Assertions (fichiers) |\n|---|---|---|\n")
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
