// En16931 0.1.0 (fassbender.dev, EUPL-1.2): the EN 16931 semantic model in C#,
// read from and written to CII and UBL through an intermediate representation
// (XSLT cii2ir / ir2cii), and validation by the official XSLT run with
// SaxonCS-HE 13.1.0, shipped as content files: CEN 1.3.16, KoSIT XRechnung
// 3.0.2 (schematron 2.6.0), FeRD Factur-X 1.09.2 BASIC and EN 16931.
//
// One specification per BT-24: urn:cen.eu:en16931:2017 → FacturXEn16931
// (CEN + Factur-X EN 16931 rules; the library maps that identifier to it as to
// Core, which carries the CEN rules alone), XRechnung 3.0 → XRechnung.

using System.Xml;
using Bench;
using En16931;
using En16931.Model;
using En16931.Spec;
using En16931.Specs;

var lib = new Lib("en16931", "En16931", "0.1.0");

ISpecificationParser[] specs = [FacturXEn16931.Instance, XRechnung.Instance, FacturXBasic.Instance];
var parser = Parser.Create(specs);

static Document Load(byte[] xml)
{
    using var reader = XmlReader.Create(new MemoryStream(xml, writable: false), new XmlReaderSettings { DtdProcessing = DtdProcessing.Prohibit, XmlResolver = null });
    return new Document(reader);
}

IInvoice Parse(byte[] xml)
{
    using var reader = XmlReader.Create(new MemoryStream(xml, writable: false), new XmlReaderSettings { DtdProcessing = DtdProcessing.Prohibit, XmlResolver = null });
    return parser.Parse(reader);
}

ISpecificationValidator SpecOf(Document doc)
{
    var id = doc.Specification.ToString();
    return specs.OfType<ISpecificationValidator>().FirstOrDefault(s => s.SpecificationIdentifier.ToString() == id)
        ?? throw new NotSupportedException($"no specification for BT-24 {id}");
}

// Validate throws ValidationException carrying the failed assertions.
List<string> Validate(byte[] xml)
{
    var doc = Load(xml);
    try
    {
        SpecOf(doc).Validate(ref doc);
        return [];
    }
    catch (ValidationException e)
    {
        return e.Errors.Select(x => x?.ToString() ?? "?").ToList();
    }
}

byte[] Write(IInvoice invoice)
{
    var doc = ((ISpecificationParser)FacturXEn16931.Instance).Serialize(invoice, Schema.CiiD16b);
    using var ms = new MemoryStream();
    using (var w = XmlWriter.Create(ms))
    {
        doc.WriteTo(w);
    }
    return ms.ToArray();
}

return Harness.Run(lib, args, h =>
{
    h.Unsupported(Op.Extract, "pas de PDF");
    h.Parse("Parser.Parse (XSLT cii2ir via SaxonCS-HE puis désérialisation du modèle EN 16931 ; valide d'abord selon la spécification de BT-24)", Parse);
    h.Validate(
        "ISpecificationValidator.Validate (FacturXEn16931 pour urn:cen.eu:en16931:2017, XRechnung pour XRechnung 3.0) : XSLT officiels CEN 1.3.16, FeRD Factur-X 1.09.2 EN 16931, KoSIT XRechnung 3.0.2 / 2.6.0 via SaxonCS-HE 13.1.0 ; erreurs = ValidationException.Errors",
        Validate,
        rules => Findings.Of(rules.Select(r => r.Length > 80 ? r[..80] : r)));
    h.Write("ISpecificationParser.Serialize(modèle, CiiD16b) (XSLT ir2cii), modèle lu par Parser.Parse", Parse, Write);
    h.Unsupported(Op.Embed, "pas de PDF");
    h.Unsupported(Op.CheckPdf, "pas de PDF");
});
