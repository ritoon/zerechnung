package schema

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/internal/xmltree"
	"github.com/ritoon/zerechnung/invoice"
)

// d22bTables is the complete UN/CEFACT CII D22B schema of the FNFE-MPE
// package (France_RFE 1.4.0.04), compiled for the tests only: they check that
// the EXTENDED schema, against which EXTENDED-CTC-FR invoices are checked, is
// included in it.
var d22bTables = newTables(d22bNamespaces[:], d22bTypes[:], d22bParticles[:], d22bAttributes[:], d22bFacetTable[:], false)

// TestCIITables checks invariants of the generated CII D16B and D22B tables.
func TestCIITables(t *testing.T) {
	if D16BVersion() != "D16B" || d22bSchemaVersion != "D22B" {
		t.Errorf("versions %q, %q", D16BVersion(), d22bSchemaVersion)
	}
	for _, tc := range []struct {
		name  string
		t     *tables
		roots []particle
	}{
		{"D16B", d16bTables(), d16bRoots[:]},
		{"D22B", d22bTables, d22bRoots[:]},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tb := tc.t
			if len(tc.roots) != 1 || tc.roots[0].name != "CrossIndustryInvoice" || tb.namespaces[tc.roots[0].ns] != nsRSM {
				t.Fatalf("roots %+v", tc.roots)
			}
			recursive := false
			for i := range tb.types {
				ty := &tb.types[i]
				if int(ty.first)+int(ty.n) > len(tb.particles) || int(ty.afirst)+int(ty.an) > len(tb.attributes) {
					t.Fatalf("type %d (%s) out of bounds", i, ty.name)
				}
				if ty.profiles != 0 || int(ty.facets) >= len(tb.facetTable) || ty.content == contentAny {
					t.Errorf("type %d (%s): profiles %#x, facets %d, content %d", i, ty.name, ty.profiles, ty.facets, ty.content)
				}
				seen := map[string]bool{}
				for _, p := range tb.particles[ty.first : ty.first+ty.n] {
					if seen[p.name] {
						t.Errorf("type %s: %s twice", ty.name, p.name)
					}
					seen[p.name] = true
					if int(p.typ) >= len(tb.types) || int(p.ns) >= len(tb.namespaces) {
						t.Errorf("type %s: particle %s out of bounds", ty.name, p.name)
					}
					recursive = recursive || int(p.typ) == i
				}
			}
			for _, a := range tb.attributes {
				if int(a.facets) >= len(tb.facetTable) {
					t.Errorf("attribute %s: facets %d", a.name, a.facets)
				}
			}
			// ram:GroupedWorkItemType and ram:SpecifiedBinaryFileType contain
			// themselves: the tables keep the recursion.
			if !recursive {
				t.Error("no recursive type")
			}
		})
	}
}

const nsRSM = "urn:un:unece:uncefact:data:standard:CrossIndustryInvoice:100"

// inclusion compares the schema of a with the schema of b: it lists, by
// path, what a accepts and b does not. Shared and recursive types are
// compared once.
type inclusion struct {
	a, b  *tables
	seen  map[[2]uint16]bool
	diffs []string
}

func (in *inclusion) diff(path, format string, args ...any) {
	in.diffs = append(in.diffs, path+": "+fmt.Sprintf(format, args...))
}

// types compares type ai of a with type bi of b at path.
func (in *inclusion) types(path string, ai, bi uint16) {
	if in.seen[[2]uint16{ai, bi}] {
		return
	}
	in.seen[[2]uint16{ai, bi}] = true
	ta, tb := &in.a.types[ai], &in.b.types[bi]
	in.attributes(path, ta, tb)
	switch {
	case ta.content == contentSimple && tb.content == contentSimple:
		if !in.simple(ta.simple, ta.facets, tb.simple, tb.facets) {
			in.diff(path, "value %s (facets %d) not accepted by %s (facets %d)", typeLabel(ta.simpleName, ta.simple), ta.facets, typeLabel(tb.simpleName, tb.simple), tb.facets)
		}
	case ta.content != tb.content && !(ta.content == contentEmpty && optional(in.b, tb)):
		in.diff(path, "content %d, %d in b", ta.content, tb.content)
	case ta.content == contentElements && ta.choice != tb.choice:
		in.diff(path, "choice %t, %t in b", ta.choice, tb.choice)
	case ta.content == contentElements && ta.choice:
		if ta.min < tb.min || ta.max > tb.max {
			in.diff(path, "choice occurs %d..%d, %d..%d in b", ta.min, ta.max, tb.min, tb.max)
		}
		for _, pa := range in.a.particles[ta.first : ta.first+ta.n] {
			j := in.find(tb, 0, &pa)
			if j < 0 {
				in.diff(path, "%s not in the choice of b", pa.name)
				continue
			}
			in.particle(path, &pa, &in.b.particles[int(tb.first)+j])
		}
	case ta.content == contentElements:
		pb := in.b.particles[tb.first : tb.first+tb.n]
		next := 0 // b particles before next are matched or skipped
		for _, pa := range in.a.particles[ta.first : ta.first+ta.n] {
			j := in.find(tb, next, &pa)
			if j < 0 {
				in.diff(path, "%s not in b (or not in the same order)", pa.name)
				continue
			}
			for _, skipped := range pb[next:j] {
				if skipped.min > 0 {
					in.diff(path, "%s required by b", skipped.name)
				}
			}
			in.particle(path, &pa, &pb[j])
			next = j + 1
		}
		for _, skipped := range pb[next:] {
			if skipped.min > 0 {
				in.diff(path, "%s required by b", skipped.name)
			}
		}
	}
}

// optional reports whether every element of t is optional in b, so that
// an empty element of type t is valid.
func optional(b *tables, t *ctype) bool {
	if t.content != contentElements {
		return t.content == contentEmpty
	}
	if t.choice {
		return t.min == 0
	}
	for _, p := range b.particles[t.first : t.first+t.n] {
		if p.min > 0 {
			return false
		}
	}
	return true
}

// find returns the index, from start, of the particle of tb with the name
// and namespace of pa, or -1.
func (in *inclusion) find(tb *ctype, start int, pa *particle) int {
	for j := start; j < int(tb.n); j++ {
		pb := &in.b.particles[int(tb.first)+j]
		if pb.name == pa.name && in.b.namespaces[pb.ns] == in.a.namespaces[pa.ns] {
			return j
		}
	}
	return -1
}

func (in *inclusion) particle(path string, pa, pb *particle) {
	p := path + "/" + pa.name
	if pa.min < pb.min || pa.max > pb.max {
		in.diff(p, "occurs %d..%s, %d..%s in b", pa.min, occurs(pa.max), pb.min, occurs(pb.max))
	}
	in.types(p, pa.typ, pb.typ)
}

func occurs(n uint8) string {
	if n == unbounded {
		return "unbounded"
	}
	return fmt.Sprint(n)
}

// attributes compares the attributes of ta and tb.
func (in *inclusion) attributes(path string, ta, tb *ctype) {
	as := in.a.attributes[ta.afirst : ta.afirst+ta.an]
	bs := in.b.attributes[tb.afirst : tb.afirst+tb.an]
	for _, a := range as {
		i := slices.IndexFunc(bs, func(b attribute) bool { return b.name == a.name })
		switch {
		case i < 0:
			in.diff(path, "attribute %s not in b", a.name)
		case !in.simple(a.kind, a.facets, bs[i].kind, bs[i].facets):
			in.diff(path, "attribute %s: value (facets %d) not accepted by b (facets %d)", a.name, a.facets, bs[i].facets)
		}
	}
	for _, b := range bs {
		i := slices.IndexFunc(as, func(a attribute) bool { return a.name == b.name })
		if b.required && (i < 0 || !as[i].required) {
			in.diff(path, "attribute %s required by b", b.name)
		}
	}
}

// simple reports whether every value of kind ka with facets fa of a is
// accepted by kind kb with facets fb of b. It is exact for enumerations and
// conservative otherwise.
func (in *inclusion) simple(ka simpleKind, fa uint16, kb simpleKind, fb uint16) bool {
	if kb == kString && fb == 0 {
		return true
	}
	if fa != 0 && len(in.a.facetTable[fa].enum) > 0 {
		for _, v := range in.a.facetTable[fa].enum {
			if in.a.checkValue(v, ka, fa) == "" && in.b.checkValue(v, kb, fb) != "" {
				return false
			}
		}
		return true
	}
	if ka != kb && !(ka == kInteger && kb == kDecimal) {
		return false
	}
	if fb == 0 {
		return true
	}
	if fa == 0 {
		return false
	}
	a, b := &in.a.facetTable[fa], &in.b.facetTable[fb]
	return len(b.enum) == 0 &&
		(len(b.patterns) == 0 || slices.Equal(a.patterns, b.patterns) && a.collapse == b.collapse) &&
		(b.minLen < 0 || a.minLen >= b.minLen) &&
		(b.maxLen < 0 || a.maxLen >= 0 && a.maxLen <= b.maxLen)
}

// compareSchemas lists what the root ra of a accepts and the root rb of b
// does not.
func compareSchemas(a *tables, ra *particle, b *tables, rb *particle) []string {
	in := &inclusion{a: a, b: b, seen: map[[2]uint16]bool{}}
	if ra.name != rb.name || a.namespaces[ra.ns] != b.namespaces[rb.ns] {
		return []string{"different roots"}
	}
	in.types("/"+ra.name, ra.typ, rb.typ)
	return in.diffs
}

// TestSchemaInclusion checks that the Factur-X schemas are included in the
// complete CII schemas applied by the reference validators: EN 16931 in D16B
// (KoSIT, XRECHNUNG), but for the number of preceding invoices, and EXTENDED
// in D22B (FNFE-MPE, EXTENDED-CTC-FR). An invoice valid against the Factur-X
// schema of its profile is then valid against the reference schema.
func TestSchemaInclusion(t *testing.T) {
	for _, tc := range []struct {
		name    string
		p       int
		b       *tables
		rb      *particle
		allowed []string // differences
	}{
		{"EN 16931 in D16B", pEN16931, d16bTables(), &d16bRoots[0], []string{
			"/CrossIndustryInvoice/SupplyChainTradeTransaction/ApplicableHeaderTradeSettlement/InvoiceReferencedDocument: occurs 0..unbounded, 0..1 in b",
		}},
		{"EXTENDED in D22B", pExtended, d22bTables, &d22bRoots[0], nil},
		// The smaller profiles are included in EN 16931, thus in D16B
		// but for the same difference.
		{"BASIC in D16B", pBasic, d16bTables(), &d16bRoots[0], []string{
			"/CrossIndustryInvoice/SupplyChainTradeTransaction/ApplicableHeaderTradeSettlement/InvoiceReferencedDocument: occurs 0..unbounded, 0..1 in b",
		}},
		{"MINIMUM in D22B", pMinimum, d22bTables, &d22bRoots[0], nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := compareSchemas(ciiTables(), &roots[tc.p], tc.b, tc.rb)
			if !slices.Equal(got, tc.allowed) {
				t.Errorf("differences:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(tc.allowed, "\n"))
			}
		})
	}
	// The comparison finds what D16B accepts and EN 16931 does not.
	if got := compareSchemas(d16bTables(), &d16bRoots[0], ciiTables(), &roots[pEN16931]); len(got) < 100 {
		t.Errorf("D16B in EN 16931: %d differences", len(got))
	}
}

// checkTables checks root against the schema root r of t.
func checkTables(root *xmltree.Node, t *tables, r *particle) *rules.Report {
	rep := &rules.Report{}
	c := &rules.Checker{Doc: rules.NewDoc(root, invoice.ProfileUnknown), Report: rep, Source: rules.SourceSyntax}
	if root.Name != r.name || root.Space != t.namespaces[r.ns] {
		c.Fail(RuleRoot, rules.Error, root, "the root element is %s", display(root))
		return rep
	}
	v := validator{c: c, t: t}
	v.element(root, &t.types[r.typ])
	return rep
}

// TestKoSITCII checks the CII instances of the KoSIT XRechnung test suite
// (FX_KOSIT, a checkout of github.com/itplr-kosit/xrechnung-testsuite): all
// of them are XRECHNUNG invoices, valid against the D16B schema as in the
// KoSIT validator.
func TestKoSITCII(t *testing.T) {
	dir := os.Getenv("FX_KOSIT")
	if dir == "" {
		t.Skip("FX_KOSIT not set")
	}
	paths, err := filepath.Glob(filepath.Join(dir, "src", "test", "*", "*", "*_uncefact.xml"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no CII instance under %s (%v)", dir, err)
	}
	for _, path := range paths {
		root, err := xmltree.Parse(readFile(t, path))
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		doc := rules.NewDoc(root, invoice.ProfileUnknown)
		r := &rules.Report{}
		Check(&rules.Checker{Doc: doc, Report: r, Source: rules.SourceSyntax})
		if doc.Profile != invoice.ProfileXRechnung || len(r.Findings) > 0 {
			t.Errorf("%s (%s):\n%s", filepath.Base(path), doc.Profile, r)
		}
	}
	t.Logf("%d CII instances valid against D16B", len(paths))
}

// TestFNFE checks the CII and Factur-X examples of the FNFE-MPE package
// (FX_FNFE, a checkout of github.com/fnfempe/France_RFE): each is valid
// against the schema of its profile and against D22B, the schema the package
// validates CII invoices with.
func TestFNFE(t *testing.T) {
	dir := os.Getenv("FX_FNFE")
	if dir == "" {
		t.Skip("FX_FNFE not set")
	}
	paths, err := filepath.Glob(filepath.Join(dir, "FNFE_RFE_INVOICE", "Z.example", "TEST", "*", "*", "*.xml"))
	if err != nil {
		t.Fatal(err)
	}
	profiles := map[invoice.Profile]int{}
	for _, path := range paths {
		root, err := xmltree.Parse(readFile(t, path))
		if err != nil || root.Name != "CrossIndustryInvoice" {
			continue
		}
		doc := rules.NewDoc(root, invoice.ProfileUnknown)
		r := &rules.Report{}
		Check(&rules.Checker{Doc: doc, Report: r, Source: rules.SourceSyntax})
		if len(r.Findings) > 0 {
			t.Errorf("%s (%s):\n%s", filepath.Base(path), doc.Profile, r)
		}
		if r := checkTables(root, d22bTables, &d22bRoots[0]); len(r.Findings) > 0 {
			t.Errorf("%s against D22B:\n%s", filepath.Base(path), r)
		}
		profiles[doc.Profile]++
	}
	if len(profiles) == 0 {
		t.Fatalf("no CII example under %s", dir)
	}
	t.Logf("profiles of the CII examples: %v", profiles)
}
