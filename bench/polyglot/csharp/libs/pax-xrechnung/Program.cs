// pax.XRechnung.NET 0.3.3 (ipax77; MIT on NuGet, GPL-3.0 in the repository):
// an XRechnung UBL model, writer and validators. For CII it only offers
// ZugferdXmlValidator, a local validator for ZUGFeRD 2.3.3 / Factur-X 1.07.3
// EN 16931 (Comfort): the Factur-X 1.07.3 EN16931 XSD (XmlSchemaSet), then the
// FeRD XSLT run by SaxonCS-HE 13.0.0, both embedded. It accepts only invoices
// whose BT-24 is exactly urn:cen.eu:en16931:2017. No CII reader or writer, no
// PDF.

using System.Text;
using Bench;
using pax.XRechnung.NET;

var lib = new Lib("pax-xrechnung", "pax.XRechnung.NET", "0.3.3");

static Findings Summarize(InvoiceValidationResult r)
{
    if (r.Error is { Length: > 0 } e)
    {
        throw new InvalidOperationException(e);
    }
    return Findings.Of(r.Validations
        .Where(m => m.Severity == System.Xml.Schema.XmlSeverityType.Error)
        .Select(m => m.Code is { Length: > 0 } c ? c : "xsd"));
}

return Harness.Run(lib, args, h =>
{
    h.Unsupported(Op.Extract, "pas de PDF");
    h.Unsupported(Op.Parse, "pas de lecteur CII (modèle XRechnung UBL seulement)");
    h.Skip("ParseValidate_Large", "ZugferdXmlValidator n'accepte que BT-24 = urn:cen.eu:en16931:2017 ; XRechnung CII refusé");
    h.Validate(
        "ZugferdXmlValidator.Validate(string) : XSD Factur-X 1.07.3 EN16931 puis XSLT FeRD EN16931 (ZUGFeRD 2.3.3) via SaxonCS-HE 13.0.0, compilés au premier appel ; BT-24 = urn:cen.eu:en16931:2017 exigé (XRechnung refusé)",
        x => ZugferdXmlValidator.Validate(Encoding.UTF8.GetString(x)),
        Summarize);
    h.Unsupported(Op.Write, "pas d'écriture CII (XmlInvoiceWriter écrit de l'UBL)");
    h.Unsupported(Op.Embed, "pas de PDF");
    h.Unsupported(Op.CheckPdf, "pas de PDF");
});
