// Reference validator: SaxonCS-HE 13.1.0 (Saxonica, MPL-2.0, SaxonJ-HE transpiled
// to C#: no Java, no IKVM) running the official XSLT, as the Python bench does
// with saxonche:
//
// - CEN EN 16931 CII 1.3.17 (ConnectingEurope/eInvoicing-EN16931,
//   xslt/EN16931-CII-validation.xslt) for every invoice;
// - KoSIT XRechnung 3.0.2 schematron 2.6.0 (itplr-kosit/xrechnung-schematron,
//   schematron/cii/XRechnung-CII-validation.xsl) as well when BT-24 names
//   XRechnung, as the CII scenario of validator-configuration-xrechnung does.
//
// Artefacts downloaded by `run.sh setup` into .artefacts/ (SHA-256 checked).
// The stylesheets are compiled on the first call (part of first_call_ns) and
// reused. Counting as the KoSIT validator does: an error is any assertion with
// flag fatal or error; for the XRechnung scenario the custom levels of
// validator-configuration-xrechnung v2026-08-31 apply (CII-SR-465 and
// CII-SR-466 raised to error, CII-SR-475 and CII-SR-476 lowered to
// information). No XSD (the CEN CII artefacts do not ship one).

using System.Text.RegularExpressions;
using Bench;
using Saxon.Api;

var lib = new Lib("saxoncs", "SaxonCS-HE", "13.1.0");

return Harness.Run(lib, args, h =>
{
    h.Unsupported(Op.Extract, "validateur seul");
    h.Unsupported(Op.Parse, "validateur seul");
    h.Validate(
        $"XSLT officiels CEN EN 16931 CII {Reference.CenVersion} (+ KoSIT XRechnung 3.0.2 schematron {Reference.XrVersion} si BT-24 XRechnung) via SaxonCS-HE, compilés au premier appel ; erreurs fatal/error avec les niveaux KoSIT v2026-08-31",
        Reference.Validate,
        rules => Findings.Of(rules));
    h.Unsupported(Op.Write, "validateur seul");
    h.Unsupported(Op.Embed, "validateur seul");
    h.Unsupported(Op.CheckPdf, "validateur seul");
});

static partial class Reference
{
    public const string CenVersion = "1.3.17";
    public const string XrVersion = "2.6.0";

    // validator-configuration-xrechnung v2026-08-31, CII scenario: custom levels.
    private static readonly Dictionary<string, string> KositCustomLevel = new(StringComparer.Ordinal)
    {
        ["CII-SR-465"] = "error",
        ["CII-SR-466"] = "error",
        ["CII-SR-475"] = "information",
        ["CII-SR-476"] = "information",
    };

    private sealed record Compiled(Processor Processor, DocumentBuilder Builder, XsltExecutable Cen, XsltExecutable Xr, XPathExecutable Asserts);

    private static Compiled? _compiled;

    private static Compiled Compile()
    {
        if (_compiled != null)
        {
            return _compiled;
        }
        var art = Corpus.Artefacts;
        var cen = Path.Combine(art, $"cen-{CenVersion}", "xslt", "EN16931-CII-validation.xslt");
        var xr = Path.Combine(art, $"xrechnung-{XrVersion}", "schematron", "cii", "XRechnung-CII-validation.xsl");
        foreach (var p in new[] { cen, xr })
        {
            if (!File.Exists(p))
            {
                throw new FileNotFoundException($"{p} missing: run ./run.sh setup");
            }
        }
        var processor = new Processor(false);
        var compiler = processor.NewXsltCompiler();
        XsltExecutable Load(string path)
        {
            using var s = File.OpenRead(path);
            return compiler.Compile(s, new Uri(path));
        }
        var xpath = processor.NewXPathCompiler();
        xpath.DeclareNamespace("svrl", "http://purl.oclc.org/dsdl/svrl");
        _compiled = new Compiled(
            processor,
            processor.NewDocumentBuilder(),
            Load(cen),
            Load(xr),
            xpath.Compile("//svrl:failed-assert | //svrl:successful-report"));
        return _compiled;
    }

    [GeneratedRegex(@"GuidelineSpecifiedDocumentContextParameter>\s*<(?:\w+:)?ID>([^<]+)<")]
    private static partial Regex Bt24();

    private static readonly byte[] Guideline = "GuidelineSpecifiedDocumentContextParameter"u8.ToArray();

    /// <summary>The text around BT-24, found by a byte search (the samples open with a long licence comment).</summary>
    private static string Bt24Window(byte[] xml)
    {
        var i = xml.AsSpan().IndexOf(Guideline);
        return i < 0 ? "" : System.Text.Encoding.UTF8.GetString(xml, i, Math.Min(400, xml.Length - i));
    }

    private static List<string> Errors(Compiled c, XsltExecutable xslt, XdmNode doc, bool xrechnung)
    {
        var dest = new XdmDestination();
        var transformer = xslt.Load30();
        // The KoSIT stylesheet reads the document from global variables.
        transformer.GlobalContextItem = doc;
        transformer.ApplyTemplates(doc, dest);
        var sel = c.Asserts.Load();
        sel.ContextItem = dest.XdmNode;
        var output = new List<string>();
        foreach (var item in sel)
        {
            var el = (XdmNode)item;
            var rule = el.GetAttributeValue("id") ?? "?";
            var flag = (el.GetAttributeValue("flag") ?? "error").ToLowerInvariant();
            if (xrechnung && KositCustomLevel.TryGetValue(rule, out var custom))
            {
                flag = custom;
            }
            if (flag is "fatal" or "error")
            {
                output.Add(rule);
            }
        }
        return output;
    }

    public static List<string> Validate(byte[] xml)
    {
        var c = Compile();
        XdmNode doc;
        using (var ms = new MemoryStream(xml, writable: false))
        {
            doc = c.Builder.Build(ms);
        }
        var m = Bt24().Match(Bt24Window(xml));
        var xrechnung = m.Success && m.Groups[1].Value.Contains("xrechnung", StringComparison.Ordinal);
        var rules = Errors(c, c.Cen, doc, xrechnung);
        if (xrechnung)
        {
            rules.AddRange(Errors(c, c.Xr, doc, true));
        }
        return rules;
    }
}
