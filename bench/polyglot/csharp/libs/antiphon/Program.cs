// Antiphon.Invoicing 1.0.0 (Clockwork Otter Foundry, commercial, closed source;
// the evaluation licence is free, fully functional, needs no key and has no
// time limit). The suite is split into packages: a typed EN 16931 model
// (Antiphon.Invoicing), a CII parser and generator, Factur-X PDF extraction and
// embedding on PDFsharp 6.2.4, and a layered validator (well-formedness, XSD,
// Schematron) whose Schematron rule sets are shipped pre-compiled to an
// intermediate representation run by its own XPath 2 evaluator (no XSLT, no
// Java). Rule sets used here: CEN EN 16931 CII and KoSIT XRechnung CII. There
// is no Factur-X profile rule set (FeRD) and no French one; the Peppol CII set
// is a separate package that no example of the corpus implies.
//
// Every call goes through the plain (non-licence) entry points: no licence key
// is read or checked.

using System.Text;
using System.Xml.Linq;
using Antiphon.Generation.Cii;
using Antiphon.Parsing.Cii;
using Antiphon.Pdf;
using Antiphon.Rules.Ir;
using Antiphon.Validation.Model;
using Antiphon.Validation.Pipeline;
using Bench;
using CenCii = Antiphon.Rules.Cen.Cii.RuleSetInfo;
using XrCii = Antiphon.Rules.XRechnung.Cii.RuleSetInfo;

var lib = new Lib("antiphon", "Antiphon.Invoicing (+ Parsing.Cii, Generation.Cii, Validation.Pipeline, Rules.Cen.Cii, Rules.XRechnung.Cii, Xsd.Cii, Pdf)", "1.0.0");

// The compiled rule sets are deserialised once, on the first validation (part
// of first_call_ns), then reused, as a caller would keep them.
Dictionary<string, RuleSetIr>? ruleSets = null;
Dictionary<string, RuleSetIr> RuleSets() => ruleSets ??= new()
{
    [CenCii.SourceId] = IrSerializer.Deserialize(CenCii.GetCompiledIrJson()),
    [XrCii.SourceId] = IrSerializer.Deserialize(XrCii.GetCompiledIrJson()),
};

static byte[] Extract(byte[] pdf)
{
    using var ms = new MemoryStream(pdf, writable: false);
    return FacturXExtractor.ExtractBytes(ms)
        ?? throw new InvalidDataException("ExtractBytes returned null (no attachment under a known name)");
}

static ParseResult Parse(byte[] xml)
{
    using var ms = new MemoryStream(xml, writable: false);
    return CiiInvoiceParser.Parse(XDocument.Load(ms));
}

static string Text(byte[] xml)
{
    using var reader = new StreamReader(new MemoryStream(xml, writable: false), Encoding.UTF8, detectEncodingFromByteOrderMarks: true);
    return reader.ReadToEnd();
}

static Findings Summarize(ValidationResult r)
{
    if (r.ResolvedFormat is null || r.ResolvedFormat.Format is null)
    {
        throw new InvalidOperationException("format not detected: no rule set run");
    }
    // OfficialFatalViolations: the fatal violations of the XSD layer (no
    // SourceId) and of the official rule sets.
    return Findings.Of(r.OfficialFatalViolations.Select(v => string.IsNullOrEmpty(v.SourceId) ? "schema:" + v.RuleId : v.RuleId));
}

return Harness.Run(lib, args, h =>
{
    h.Extract(
        "FacturXExtractor.ExtractBytes : noms factur-x.xml, zugferd-invoice.xml, ZUGFeRD-invoice.xml, xrechnung.xml, order-x.xml (puis sans casse), PDFsharp 6.2.4, octets bruts",
        Extract);
    h.Parse("CiiInvoiceParser.Parse(XDocument) → Invoice + éléments non repris (contenu COMFORT)", Parse);
    h.Validate(
        $"LayeredValidator.Validate : XSD CII D16B (Antiphon.Xsd.Cii) puis règles choisies par FormatDetector, Schematron CEN EN 16931 CII ({CenCii.Version}, {CenCii.AssertCount} assertions) et KoSIT XRechnung CII ({XrCii.Version}, {XrCii.AssertCount} assertions) précompilés, évaluateur XPath maison ; erreurs = OfficialFatalViolations ; pas de règles Factur-X ni françaises",
        x => LayeredValidator.Validate(Text(x), RuleSets()),
        Summarize);
    h.Write(
        "CiiInvoiceGenerator.GenerateXml(Invoice, Comfort), modèle lu par CiiInvoiceParser.Parse",
        Parse,
        r => Encoding.UTF8.GetBytes(CiiInvoiceGenerator.GenerateXml(r.Invoice, CiiProfile.Comfort)));
    h.Embed(
        "FacturXEmbedder.Embed(PDF, XML, EN 16931) : fichier associé factur-x.xml (/AF, /Names/EmbeddedFiles), XMP PDF/A-3 + Factur-X ; l'hôte doit déjà être PDF/A (contrat documenté, non vérifié par la librairie)",
        (host, xml) => FacturXEmbedder.Embed(host, xml),
        Extract);
    h.Unsupported(Op.CheckPdf, "pas de contrôle de PDF hybride (extraction et injection seulement)");
});
