package xmldsig

import (
	"crypto"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/ritoon/zerechnung/internal/sigalg"
	"github.com/ritoon/zerechnung/internal/sigtest"
)

func mustParse(t *testing.T, s string) *document {
	t.Helper()
	d, err := parse([]byte(s))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return d
}

func c14n(t *testing.T, s string, m c14nMethod, comments bool, apex func(*document) *node) string {
	t.Helper()
	d := mustParse(t, s)
	var a *node
	if apex != nil {
		a = apex(d)
	}
	out, err := canonicalize(d, a, nil, m, comments)
	if err != nil {
		t.Fatalf("canonicalize: %v", err)
	}
	return string(out)
}

// The examples of Canonical XML 1.0 (§3), without the DTDs, which the
// package refuses: their default attributes and entities are left out of
// the inputs and of the expected outputs.
func TestCanonicalXMLExamples(t *testing.T) {
	pis := `<?xml version="1.0"?>

<?xml-stylesheet   href="doc.xsl"
   type="text/xsl"   ?>

<doc>Hello, world!<!-- Comment 1 --></doc>

<?pi-without-data     ?>

<!-- Comment 2 -->

<!-- Comment 3 -->`
	whitespace := `<doc>
   <clean>   </clean>
   <dirty>   A   B   </dirty>
   <mixed>
      A
      <clean>   </clean>
      B
      <dirty>   A   B   </dirty>
      C
   </mixed>
</doc>`
	for _, c := range []struct {
		name, in, want string
		m              c14nMethod
	}{
		{"3.1 PIs", pis, `<?xml-stylesheet href="doc.xsl"
   type="text/xsl"   ?>
<doc>Hello, world!</doc>
<?pi-without-data?>`, c14nMethod{}},
		{"3.1 PIs and comments", pis, `<?xml-stylesheet href="doc.xsl"
   type="text/xsl"   ?>
<doc>Hello, world!<!-- Comment 1 --></doc>
<?pi-without-data?>
<!-- Comment 2 -->
<!-- Comment 3 -->`, c14nMethod{comments: true}},
		{"3.2 whitespace", whitespace, whitespace, c14nMethod{}},
		{"3.3 start and end tags", `<doc>
   <e1   />
   <e2   ></e2>
   <e3   name = "elem3"   id="elem3"   />
   <e4   name="elem4"   id="elem4"   ></e4>
   <e5 a:attr="out" b:attr="sorted" attr2="all" attr="I'm"
      xmlns:b="http://www.ietf.org"
      xmlns:a="http://www.w3.org"
      xmlns="http://example.org"/>
   <e6 xmlns="" xmlns:a="http://www.w3.org">
      <e7 xmlns="http://www.ietf.org">
         <e8 xmlns="" xmlns:a="http://www.w3.org">
            <e9 xmlns="" xmlns:a="http://www.ietf.org"/>
         </e8>
      </e7>
   </e6>
</doc>`, `<doc>
   <e1></e1>
   <e2></e2>
   <e3 id="elem3" name="elem3"></e3>
   <e4 id="elem4" name="elem4"></e4>
   <e5 xmlns="http://example.org" xmlns:a="http://www.w3.org" xmlns:b="http://www.ietf.org" attr="I'm" attr2="all" b:attr="sorted" a:attr="out"></e5>
   <e6 xmlns:a="http://www.w3.org">
      <e7 xmlns="http://www.ietf.org">
         <e8 xmlns="">
            <e9 xmlns:a="http://www.ietf.org"></e9>
         </e8>
      </e7>
   </e6>
</doc>`, c14nMethod{}},
		{"3.4 character modifications", `<doc>
   <text>First line&#x0d;&#10;Second line</text>
   <value>&#x32;</value>
   <compute><![CDATA[value>"0" && value<"10" ?"valid":"error"]]></compute>
   <compute expr='value>"0" &amp;&amp; value&lt;"10" ?"valid":"error"'>valid</compute>
   <norm attr=' &apos;   &#x20;&#13;&#xa;&#9;   &apos; '/>
</doc>`, "<doc>\n   <text>First line&#xD;\nSecond line</text>\n   <value>2</value>\n" +
			`   <compute>value&gt;"0" &amp;&amp; value&lt;"10" ?"valid":"error"</compute>` + "\n" +
			`   <compute expr="value>&quot;0&quot; &amp;&amp; value&lt;&quot;10&quot; ?&quot;valid&quot;:&quot;error&quot;">valid</compute>` + "\n" +
			`   <norm attr=" '    &#xD;&#xA;&#x9;   ' "></norm>` + "\n</doc>", c14nMethod{}},
		{"3.6 UTF-8", `<doc>&#169;</doc>`, `<doc>©</doc>`, c14nMethod{}},
		// A literal tab of an attribute value is normalized to a space.
		{"line ends", "<doc a=\"x\ty&#9;\">a\r\nb\rc</doc>", "<doc a=\"x y&#x9;\">a\nb\nc</doc>", c14nMethod{}},
	} {
		if got := c14n(t, c.in, c.m, true, nil); got != c.want {
			t.Errorf("%s:\n%s\nwant:\n%s", c.name, got, c.want)
		}
	}
}

func TestCanonicalSubtree(t *testing.T) {
	doc := `<r xmlns="urn:d" xmlns:a="urn:a" xmlns:b="urn:b" xml:lang="it" xml:space="preserve"><a:x b:c="1"><y/><!--c--></a:x></r>`
	x := func(d *document) *node { return d.root.childElements()[0] }
	// The attributes sort by namespace URI: the xml namespace,
	// http://www.w3.org/XML/1998/namespace, comes before urn:b.
	for _, c := range []struct {
		name string
		m    c14nMethod
		want string
	}{
		{"Canonical XML 1.0", c14nMethod{}, `<a:x xmlns="urn:d" xmlns:a="urn:a" xmlns:b="urn:b" xml:lang="it" xml:space="preserve" b:c="1"><y></y></a:x>`},
		{"Canonical XML 1.1", c14nMethod{v11: true}, `<a:x xmlns="urn:d" xmlns:a="urn:a" xmlns:b="urn:b" xml:lang="it" xml:space="preserve" b:c="1"><y></y></a:x>`},
		{"exclusive", c14nMethod{exclusive: true}, `<a:x xmlns:a="urn:a" xmlns:b="urn:b" b:c="1"><y xmlns="urn:d"></y></a:x>`},
		{"exclusive, #default listed", c14nMethod{exclusive: true, prefixes: map[string]bool{"#default": true}}, `<a:x xmlns="urn:d" xmlns:a="urn:a" xmlns:b="urn:b" b:c="1"><y></y></a:x>`},
		{"exclusive with comments", c14nMethod{exclusive: true, comments: true}, `<a:x xmlns:a="urn:a" xmlns:b="urn:b" b:c="1"><y xmlns="urn:d"></y><!--c--></a:x>`},
	} {
		if got := c14n(t, doc, c.m, true, x); got != c.want {
			t.Errorf("%s:\n%s\nwant:\n%s", c.name, got, c.want)
		}
	}
	// Canonical XML 1.1 does not fix up xml:base.
	d := mustParse(t, `<r xml:base="http://example.org/"><x/></r>`)
	if _, err := canonicalize(d, d.root.childElements()[0], nil, c14nMethod{v11: true}, false); !errors.Is(err, errXMLBase) {
		t.Errorf("xml:base with Canonical XML 1.1: %v", err)
	}
	if got := c14n(t, `<r xml:base="http://example.org/"><x/></r>`, c14nMethod{}, false, x); got != `<x xml:base="http://example.org/"></x>` {
		t.Errorf("xml:base with Canonical XML 1.0: %s", got)
	}
	// Canonical XML 1.0 inherits every attribute of the xml namespace,
	// xml:id included; 1.1 does not inherit xml:id.
	if got := c14n(t, `<r xml:id="i" xml:lang="it"><x/></r>`, c14nMethod{}, false, x); got != `<x xml:id="i" xml:lang="it"></x>` {
		t.Errorf("xml:id with Canonical XML 1.0: %s", got)
	}
	if got := c14n(t, `<r xml:id="i" xml:lang="it"><x/></r>`, c14nMethod{v11: true}, false, x); got != `<x xml:lang="it"></x>` {
		t.Errorf("xml:id with Canonical XML 1.1: %s", got)
	}
	// Line ends are normalized in comments and processing instructions
	// too (XML 1.0 §2.11).
	if got := c14n(t, "<r><!--a\r\nb\rc--><?pi x\r\ny?>t\r\n</r>", c14nMethod{comments: true}, true, nil); got != "<r><!--a\nb\nc--><?pi x\ny?>t\n</r>" {
		t.Errorf("CR LF: %q", got)
	}
	// Excluded subtrees leave no trace.
	d = mustParse(t, `<r><a/><b><c/></b></r>`)
	out, err := canonicalize(d, nil, func(n *node) bool { return n.local == "b" }, c14nMethod{}, false)
	if err != nil || string(out) != `<r><a></a></r>` {
		t.Errorf("excluded subtree: %s, %v", out, err)
	}
}

// TestNamespaceCost checks that parsing and canonicalization stay linear
// with many namespace declarations: copying the namespaces in scope for each
// element made a 300 KB document take tens of seconds.
func TestNamespaceCost(t *testing.T) {
	var b strings.Builder
	b.WriteString("<r")
	for i := range 4000 {
		fmt.Fprintf(&b, ` xmlns:n%d="urn:n%d"`, i, i)
	}
	b.WriteString(">")
	for i := range 50000 {
		if i%2 == 0 {
			b.WriteString("<a/>")
		} else {
			b.WriteString(`<n1:a xmlns:n2="urn:other"/>`)
		}
	}
	b.WriteString("</r>")
	start := time.Now()
	d := mustParse(t, b.String())
	for _, m := range []c14nMethod{{}, {exclusive: true, prefixes: map[string]bool{"n3": true}}} {
		if _, err := canonicalize(d, nil, nil, m, false); err != nil {
			t.Fatal(err)
		}
		if _, err := canonicalize(d, d.root.childElements()[1], nil, m, false); err != nil {
			t.Fatal(err)
		}
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Errorf("%v to parse and canonicalize", elapsed)
	}
	// The child redeclaring n2 renders it, its sibling does not.
	if got := c14n(t, `<r xmlns:a="urn:a"><x xmlns:a="urn:b"/><y xmlns:a="urn:a"/></r>`, c14nMethod{}, false, nil); got != `<r xmlns:a="urn:a"><x xmlns:a="urn:b"></x><y></y></r>` {
		t.Errorf("redeclarations: %s", got)
	}
}

func TestParseRefuses(t *testing.T) {
	for name, s := range map[string]string{
		"DOCTYPE":         `<!DOCTYPE r [<!ENTITY e "x">]><r>&e;</r>`,
		"unknown entity":  `<r>&e;</r>`,
		"two roots":       `<r/><s/>`,
		"no root":         `<!-- c -->`,
		"unclosed":        `<r><s></r>`,
		"unbound prefix":  `<p:r/>`,
		"bad char ref":    `<r a="&#0;"/>`,
		"lt in attribute": `<r a="<"/>`,
		"too deep":        strings.Repeat("<a>", MaxDepth+1) + strings.Repeat("</a>", MaxDepth+1),
		// A reader that took the last declaration would read other names
		// than the ones canonicalized.
		"default namespace declared twice": `<r xmlns="" xmlns="urn:x"/>`,
		"prefix declared twice":            `<r xmlns:a="urn:a" xmlns:a="urn:b"><a:x/></r>`,
		"attribute repeated":               `<r a="1" a="2"/>`,
	} {
		if _, err := parse([]byte(s)); err == nil {
			t.Errorf("%s: parsed", name)
		}
	}
}

var fattura = []byte(`<?xml version="1.0" encoding="UTF-8"?>
<p:FatturaElettronica xmlns:p="http://ivaservizi.agenziaentrate.gov.it/docs/xsd/fatture/v1.2" versione="FPR12">
  <FatturaElettronicaHeader><Numero>1</Numero></FatturaElettronicaHeader>
  <FatturaElettronicaBody a="&lt;&amp;"><![CDATA[x < y]]></FatturaElettronicaBody>
</p:FatturaElettronica>
`)

func newSigner(t *testing.T, key crypto.Signer) (*sigtest.PKI, []*x509.Certificate, crypto.Signer) {
	t.Helper()
	p := sigtest.New(t)
	leaf, k := p.Leaf(t, sigtest.LeafOptions{Key: key})
	return p, []*x509.Certificate{leaf, p.Intermediate}, k
}

func signDoc(t *testing.T, doc []byte, o *SignOptions) string {
	t.Helper()
	out, err := SignEnveloped(doc, o)
	if err != nil {
		t.Fatalf("SignEnveloped: %v", err)
	}
	return string(out)
}

func TestSignVerify(t *testing.T) {
	for _, c := range []struct {
		name   string
		key    crypto.Signer
		pss    bool
		scheme sigalg.Scheme
	}{
		{"RSA", nil, false, sigalg.PKCS1v15},
		{"PSS", nil, true, sigalg.PSS},
		{"ECDSA", sigtest.ECKey(t), false, sigalg.ECDSA},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, chain, key := newSigner(t, c.key)
			signed := signDoc(t, fattura, &SignOptions{Signer: key, Chain: chain, PSS: c.pss, SigningTime: sigtest.Epoch})
			if !strings.HasSuffix(signed, "</ds:Signature></p:FatturaElettronica>\n") {
				t.Errorf("the signature is not the last child of the root:\n%s", signed)
			}
			res, err := Verify([]byte(signed))
			if err != nil {
				t.Fatalf("Verify: %v", err)
			}
			if !res.Certificate.Equal(chain[0]) || len(res.Certificates) != 2 || res.Algorithm.Scheme != c.scheme {
				t.Errorf("certificate %v, %d certificates, algorithm %v", res.Certificate.Subject, len(res.Certificates), res.Algorithm)
			}
			if !res.XAdES || !res.SigningTime.Equal(sigtest.Epoch) || res.SigningCertificate != "SigningCertificateV2" || !res.DataObjectFormat || res.Policy != nil {
				t.Errorf("result %+v", res)
			}
		})
	}
}

func TestSignErrors(t *testing.T) {
	p, chain, key := newSigner(t, nil)
	other, _ := p.Leaf(t, sigtest.LeafOptions{Key: sigtest.ECKey(t)})
	signed := signDoc(t, fattura, &SignOptions{Signer: key, Chain: chain, SigningTime: sigtest.Epoch})
	for name, c := range map[string]struct {
		doc string
		o   *SignOptions
	}{
		"no options":     {string(fattura), nil},
		"no certificate": {string(fattura), &SignOptions{Signer: key, SigningTime: sigtest.Epoch}},
		"no time":        {string(fattura), &SignOptions{Signer: key, Chain: chain}},
		"another key":    {string(fattura), &SignOptions{Signer: key, Chain: []*x509.Certificate{other}, SigningTime: sigtest.Epoch}},
		"SHA-1":          {string(fattura), &SignOptions{Signer: key, Chain: chain, SigningTime: sigtest.Epoch, Hash: crypto.SHA1}},
		"already signed": {signed, &SignOptions{Signer: key, Chain: chain, SigningTime: sigtest.Epoch}},
		"not XML":        {"fattura", &SignOptions{Signer: key, Chain: chain, SigningTime: sigtest.Epoch}},
		"empty root":     {`<r/>`, &SignOptions{Signer: key, Chain: chain, SigningTime: sigtest.Epoch}},
	} {
		if _, err := SignEnveloped([]byte(c.doc), c.o); err == nil {
			t.Errorf("%s: signed", name)
		}
	}
}

// TestXAdESEPES signs a Facturae invoice with the signature policy of
// Facturae 3.1, as the Spanish administration requires (XAdES-EPES).
func TestXAdESEPES(t *testing.T) {
	doc := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<fe:Facturae xmlns:fe="http://www.facturae.gob.es/formato/Versiones/Facturae3_2_2.xml" xmlns:ds="http://www.w3.org/2000/09/xmldsig#">
  <FileHeader><SchemaVersion>3.2.2</SchemaVersion><Modality>I</Modality><InvoiceIssuerType>EM</InvoiceIssuerType></FileHeader>
</fe:Facturae>`)
	digest, _ := base64.StdEncoding.DecodeString("Ohixl6upD6av8N7pEvDABhEL6hM=")
	policy := &Policy{
		Identifier:  "http://www.facturae.es/politica_de_firma_formato_facturae/politica_de_firma_formato_facturae_v3_1.pdf",
		Description: "Política de Firma FacturaE v3.1",
		Hash:        crypto.SHA1,
		Digest:      digest,
	}
	_, chain, key := newSigner(t, nil)
	signed := signDoc(t, doc, &SignOptions{Signer: key, Chain: chain, SigningTime: sigtest.Epoch, Policy: policy})
	if !strings.Contains(signed, `<xades:SigPolicyHash><ds:DigestMethod Algorithm="http://www.w3.org/2000/09/xmldsig#sha1"/><ds:DigestValue>Ohixl6upD6av8N7pEvDABhEL6hM=</ds:DigestValue>`) {
		t.Errorf("no policy hash:\n%s", signed)
	}
	res, err := Verify([]byte(signed))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !reflect.DeepEqual(res.Policy, policy) {
		t.Errorf("policy %+v, want %+v", res.Policy, policy)
	}
}

var (
	reID       = regexp.MustCompile(`<ds:Signature [^>]*Id="([^"]+)"`)
	reSigValue = regexp.MustCompile(`(-sigvalue">)([^<]+)(</ds:SignatureValue>)`)
	reSig      = regexp.MustCompile(`(?s)<ds:Signature .*</ds:Signature>`)
	reRef      = regexp.MustCompile(`<ds:Reference [^>]*URI="#[^"]*"[^>]*>.*?</ds:Reference>`)
)

// resign recomputes the signature value of signed after its SignedInfo
// changed, as the signer would have written it.
func resign(t *testing.T, signed string, key crypto.Signer) string {
	t.Helper()
	d := mustParse(t, signed)
	si := d.root.elems(NSDS, "Signature")[0].elem(NSDS, "SignedInfo")
	cm, err := methodOf(si.childElements()[0])
	if err != nil {
		t.Fatal(err)
	}
	c, err := canonicalize(d, si, nil, cm, true)
	if err != nil {
		t.Fatal(err)
	}
	a := sigalg.Algorithm{Scheme: sigalg.PKCS1v15, Hash: crypto.SHA256}
	sig, err := sigalg.Sign(key, a, sigalg.Digest(a.Hash, c))
	if err != nil {
		t.Fatal(err)
	}
	return reSigValue.ReplaceAllString(signed, "${1}"+base64.StdEncoding.EncodeToString(sig)+"${3}")
}

// TestTransforms checks the other forms of the enveloped-signature
// transform and canonicalizations that other tools write.
func TestTransforms(t *testing.T) {
	_, chain, key := newSigner(t, nil)
	signed := signDoc(t, fattura, &SignOptions{Signer: key, Chain: chain, SigningTime: sigtest.Epoch})
	enveloped := `<ds:Transform Algorithm="` + TransformEnveloped + `"/>`
	c14n10 := `<ds:Transform Algorithm="` + C14N10 + `"/>`
	for _, c := range []struct {
		name, from, to string
		ok             bool
	}{
		{"XPath", enveloped, `<ds:Transform Algorithm="` + TransformXPath + `"><ds:XPath>not(ancestor-or-self::ds:Signature)</ds:XPath></ds:Transform>`, true},
		{"XPath, other prefix", enveloped, `<ds:Transform Algorithm="` + TransformXPath + `"><ds:XPath xmlns:s="` + NSDS + `"> not( ancestor-or-self::s:Signature ) </ds:XPath></ds:Transform>`, true},
		{"XPath, prefix of another namespace", enveloped, `<ds:Transform Algorithm="` + TransformXPath + `"><ds:XPath xmlns:s="urn:x">not(ancestor-or-self::s:Signature)</ds:XPath></ds:Transform>`, false},
		{"XPath, other expression", enveloped, `<ds:Transform Algorithm="` + TransformXPath + `"><ds:XPath>true()</ds:XPath></ds:Transform>`, false},
		{"XPath Filter 2.0", enveloped, `<ds:Transform Algorithm="` + TransformXPath2 + `"><f:XPath xmlns:f="` + TransformXPath2 + `" Filter="subtract">/descendant::ds:Signature</f:XPath></ds:Transform>`, true},
		{"XPath Filter 2.0, //", enveloped, `<ds:Transform Algorithm="` + TransformXPath2 + `"><f:XPath xmlns:f="` + TransformXPath2 + `" Filter="subtract"> //ds:Signature </f:XPath></ds:Transform>`, true},
		{"XPath Filter 2.0, intersect", enveloped, `<ds:Transform Algorithm="` + TransformXPath2 + `"><f:XPath xmlns:f="` + TransformXPath2 + `" Filter="intersect">/descendant::ds:Signature</f:XPath></ds:Transform>`, false},
		{"XPath Filter 2.0, two expressions", enveloped, `<ds:Transform Algorithm="` + TransformXPath2 + `"><f:XPath xmlns:f="` + TransformXPath2 + `" Filter="subtract">/descendant::ds:Signature</f:XPath><f:XPath xmlns:f="` + TransformXPath2 + `" Filter="union">/</f:XPath></ds:Transform>`, false},
		{"XPath Filter 2.0, other element", enveloped, `<ds:Transform Algorithm="` + TransformXPath2 + `"><f:XPath xmlns:f="` + TransformXPath2 + `" Filter="subtract">/descendant::ds:Object</f:XPath></ds:Transform>`, false},
		{"no enveloped-signature transform", enveloped, ``, false},
		{"canonicalization before the enveloped transform", enveloped + c14n10, c14n10 + enveloped, false},
		{"exclusive canonicalization", enveloped + c14n10, enveloped + `<ds:Transform Algorithm="` + ExcC14N + `"><ec:InclusiveNamespaces xmlns:ec="` + ExcC14N + `" PrefixList="p"/></ds:Transform>`, true},
		{"Canonical XML 1.1", enveloped + c14n10, enveloped + `<ds:Transform Algorithm="` + C14N11 + `"/>`, true},
		{"unknown transform", enveloped, `<ds:Transform Algorithm="http://www.w3.org/TR/1999/REC-xslt-19991116"/>`, false},
		{"SHA-1 digest", `<ds:DigestMethod Algorithm="http://www.w3.org/2001/04/xmlenc#sha256"/>`, `<ds:DigestMethod Algorithm="` + NSDS + `sha1"/>`, false},
	} {
		mod := strings.Replace(signed, c.from, c.to, 1)
		if mod == signed {
			t.Fatalf("%s: nothing replaced", c.name)
		}
		_, err := Verify([]byte(resign(t, mod, key)))
		if c.ok && err != nil {
			t.Errorf("%s: %v", c.name, err)
		} else if !c.ok && !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v, want ErrInvalid", c.name, err)
		}
	}
	// The canonicalization of SignedInfo.
	for _, m := range []string{ExcC14N, C14N11, C14N10WithComments} {
		mod := strings.Replace(signed, `<ds:CanonicalizationMethod Algorithm="`+C14N10+`"/>`, `<ds:CanonicalizationMethod Algorithm="`+m+`"/>`, 1)
		if _, err := Verify([]byte(resign(t, mod, key))); err != nil {
			t.Errorf("SignedInfo in %s: %v", m, err)
		}
	}
}

func TestVerifyRefuses(t *testing.T) {
	p, chain, key := newSigner(t, nil)
	signed := signDoc(t, fattura, &SignOptions{Signer: key, Chain: chain, SigningTime: sigtest.Epoch})
	id := reID.FindStringSubmatch(signed)[1]
	sig := reSig.FindString(signed)
	unsigned := strings.Replace(signed, sig, "", 1)
	header := "<FatturaElettronicaHeader>"
	other, _ := p.Leaf(t, sigtest.LeafOptions{})
	otherB64 := base64.StdEncoding.EncodeToString(other.Raw)
	leafB64 := base64.StdEncoding.EncodeToString(chain[0].Raw)

	if _, err := Verify(fattura); !errors.Is(err, ErrNotSigned) {
		t.Errorf("unsigned document: %v", err)
	}
	for _, c := range []struct {
		name, doc, msg string
	}{
		{"document changed", strings.Replace(signed, "<Numero>1<", "<Numero>2<", 1), "does not match the signed digest"},
		{"attribute changed", strings.Replace(signed, `a="&lt;&amp;"`, `a="&lt;&amp;&amp;"`, 1), "does not match the signed digest"},
		{"signing time changed", strings.Replace(signed, "2026-06-01T12:00:00Z", "2026-06-01T12:00:01Z", 1), "does not match the signed digest"},
		{"signature value changed", reSigValue.ReplaceAllStringFunc(signed, func(s string) string {
			return strings.Replace(s, `-sigvalue">`, `-sigvalue">AAAA`, 1)
		}), ""},
		{"signature in a child of the root", strings.Replace(unsigned, header, header+sig, 1), "not a child of the root"},
		{"two signatures", strings.Replace(signed, sig, sig+sig, 1), "holds 2 signatures"},
		{"another signature in the document", strings.Replace(signed, header, header+sig, 1), "second signature"},
		{"duplicate Id in the signature", strings.Replace(signed, "</ds:Signature>", `<ds:Object><x Id="`+id+`-signedprops"/></ds:Object></ds:Signature>`, 1), "designates 2 elements"},
		{"signed properties outside the signature", strings.Replace(strings.Replace(signed, `Id="`+id+`-signedprops"`, `Id="moved"`, 1), header, header+`<x Id="`+id+`-signedprops"/>`, 1), ""},
		{"signing certificate replaced", strings.Replace(signed, leafB64, otherB64, 1), ""},
		{"DOCTYPE", strings.Replace(signed, "?>\n", "?>\n<!DOCTYPE p:FatturaElettronica>\n", 1), ""},
		{"unknown signature method", strings.Replace(signed, "xmldsig-more#rsa-sha256", "xmldsig-more#rsa-md5", 1), "unsupported signature method"},
		{"KeyInfo after Object", strings.Replace(signed, "<ds:KeyInfo>", "<ds:Object/><ds:KeyInfo>", 1), "unexpected element"},
		// The verifier would read xmlns="", a reader taking the last
		// declaration urn:x: every unprefixed element would change.
		{"default namespace declared twice", strings.Replace(signed, "<p:FatturaElettronica ", `<p:FatturaElettronica xmlns="" xmlns="urn:x" `, 1), "repeated"},
		{"prefix declared twice", strings.Replace(signed, "<p:FatturaElettronica ", `<p:FatturaElettronica xmlns:p="urn:x" `, 1), "repeated"},
		{"too many references", strings.Replace(signed, "</ds:SignedInfo>", strings.Repeat(`<ds:Reference URI="#x"/>`, MaxReferences)+"</ds:SignedInfo>", 1), "more than"},
		{"reference repeated", reRef.ReplaceAllString(signed, "$0$0"), "several references"},
	} {
		_, err := Verify([]byte(c.doc))
		if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), c.msg) {
			t.Errorf("%s: %v, want ErrInvalid (%s)", c.name, err, c.msg)
		}
	}

	// A comment changes nothing: references to the document exclude them.
	if _, err := Verify([]byte(strings.Replace(signed, header, header+"<!-- c -->", 1))); err != nil {
		t.Errorf("comment added: %v", err)
	}
}

func FuzzVerify(f *testing.F) {
	p := sigtest.New(f)
	leaf, key := p.Leaf(f, sigtest.LeafOptions{})
	signed, err := SignEnveloped(fattura, &SignOptions{Signer: key, Chain: []*x509.Certificate{leaf}, SigningTime: sigtest.Epoch})
	if err != nil {
		f.Fatal(err)
	}
	f.Add(signed)
	f.Add(fattura)
	f.Add([]byte(`<r xmlns:a="urn:a"><a:b a:c="&#x9;"><![CDATA[<]]></a:b><?pi x?></r>`))
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = Verify(data)
		d, err := parse(data)
		if err != nil {
			return
		}
		for _, m := range []c14nMethod{{}, {v11: true}, {exclusive: true, comments: true}} {
			_, _ = canonicalize(d, nil, nil, m, true)
			if kids := d.root.childElements(); len(kids) > 0 {
				_, _ = canonicalize(d, kids[0], nil, m, false)
			}
		}
	})
}
