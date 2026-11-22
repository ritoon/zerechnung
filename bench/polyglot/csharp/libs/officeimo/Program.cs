// OfficeIMO.Invoicing 3.6.2 (+ .Validation and .Pdf 3.6.2, Evotec, MIT): one
// typed invoice model with bounded CII/UBL parsers and writers (EN 16931
// 1.3.16, XRechnung 3.0.2, Factur-X 1.09.2 / ZUGFeRD 2.5.2, Peppol BIS 3.0.21),
// model checks (semantics, arithmetic), and OfficeIMO.Pdf, a managed PDF
// engine that reads attachments, attaches Factur-X XML to an existing PDF and
// assesses PDF/A-3 + Factur-X readiness.
//
// Validation: OfficeIMO.Invoicing.Validation runs the XSD stage in .NET from
// the KoSIT archive (validator-configuration-xrechnung v2026-08-31, the one it
// pins; run.sh setup downloads it) but needs Java and SaxonJ-HE for the
// business-rule stage. That stage is left out (no JVM in this bench), so the
// validation measured here is XSD + the core model checks.

using Bench;
using OfficeIMO.Invoicing;
using OfficeIMO.Invoicing.Validation;
using OfficeIMO.Pdf;

var lib = new Lib("officeimo", "OfficeIMO.Invoicing + .Validation + .Pdf", "3.6.2");

string[] names = ["factur-x.xml", "zugferd-invoice.xml", "ZUGFeRD-invoice.xml", "xrechnung.xml"];

bool IsInvoice(PdfExtractedAttachment a) =>
    names.Any(n => string.Equals(n, a.FileName, StringComparison.OrdinalIgnoreCase)
        || string.Equals(n, a.UnicodeFileName, StringComparison.OrdinalIgnoreCase)
        || string.Equals(n, a.Name, StringComparison.OrdinalIgnoreCase));

byte[] Extract(byte[] pdf)
{
    using var ms = new MemoryStream(pdf, writable: false);
    var doc = PdfDocument.Load(ms);
    return doc.ExtractAttachments().FirstOrDefault(IsInvoice)?.Bytes
        ?? throw new InvalidDataException("no attachment under a standard invoice name");
}

static InvoiceReadResult Parse(byte[] xml) => InvoiceParser.Read(xml);

// The XSD stage only: without a SaxonInvoiceRulesRunner the business rules are NotRun.
InvoiceValidator? validator = null;
InvoiceValidator Validator()
{
    if (validator != null)
    {
        return validator;
    }
    var zip = Path.Combine(Corpus.Artefacts, "dl", "xrechnung-3.0.2-validator-configuration-2026-08-31.zip");
    if (!File.Exists(zip))
    {
        throw new FileNotFoundException($"{zip} missing: run ./run.sh setup");
    }
    return validator = new InvoiceValidator(InvoiceRuleBundle.Load(zip, null, null), null!);
}

(InvoiceModelValidationResult Model, InvoiceValidationReport Xsd) Validate(byte[] xml)
{
    var read = InvoiceParser.Read(xml);
    var release = read.Declaration?.Profile == InvoiceProfile.XRechnung
        ? InvoiceSpecificationRelease.XRechnung_3_0_2_2026_08_31
        : InvoiceSpecificationRelease.En16931_1_3_16;
    var model = InvoiceModelValidator.ValidateSource(read);
    var xsd = Validator().ValidateAsync(xml, release, CancellationToken.None).GetAwaiter().GetResult();
    return (model, xsd);
}

static Findings Summarize((InvoiceModelValidationResult Model, InvoiceValidationReport Xsd) r)
{
    if (r.Xsd.SchemaStatus is InvoiceValidationStatus.Failed or InvoiceValidationStatus.NotRun)
    {
        throw new InvalidOperationException($"XSD stage {r.Xsd.SchemaStatus}: {r.Xsd.Diagnostics.FirstOrDefault()?.Message}");
    }
    return Findings.Of(r.Model.Diagnostics.Where(d => d.Severity == InvoiceDiagnosticSeverity.Error).Select(d => d.Code)
        .Concat(r.Xsd.Diagnostics.Where(d => d.Severity == InvoiceDiagnosticSeverity.Error).Select(d => "schema:" + d.Code)));
}

static byte[] Write(InvoiceReadResult read) =>
    read.Write(new InvoiceXmlOptions(
        InvoiceSpecificationRelease.En16931_1_3_16,
        InvoiceSyntax.Cii,
        read.Declaration?.Profile ?? InvoiceProfile.En16931), false);

// The Factur-X helpers (AddFacturXInvoiceXml, XMP) belong to PdfCreateOptions,
// i.e. to PDFs OfficeIMO generates; an existing PDF only takes a generic
// associated file through the attachment editor.
static byte[] Embed(byte[] host, byte[] xml)
{
    using var ms = new MemoryStream(host, writable: false);
    var doc = PdfDocument.Load(ms);
    var file = new PdfEmbeddedFile("factur-x.xml", xml, "text/xml", PdfAssociatedFileRelationship.Alternative, "Factur-X invoice");
    // Replace when the host already carries a factur-x.xml (Add refuses duplicates).
    var result = doc.Attachments.Edit(session =>
    {
        if (session.Attachments.Any(a => string.Equals(a.FileName, file.FileName, StringComparison.OrdinalIgnoreCase)))
        {
            session.Replace(file.FileName, file);
        }
        else
        {
            session.Add(file);
        }
    });
    return result.ToBytes();
}

static PdfComplianceReadinessReport Check(byte[] pdf)
{
    using var ms = new MemoryStream(pdf, writable: false);
    return PdfDocument.Load(ms).AssessCompliance(PdfComplianceProfile.FacturX);
}

return Harness.Run(lib, args, h =>
{
    h.Extract(
        "OfficeIMO.Pdf PdfDocument.Load + ExtractAttachments : analyse complète du PDF, /Names/EmbeddedFiles et /AF, flux décodés ; premier fichier nommé factur-x.xml, zugferd-invoice.xml, ZUGFeRD-invoice.xml ou xrechnung.xml",
        Extract);
    h.Parse("InvoiceParser.Read (analyseur borné CII/UBL, données non reprises signalées)", Parse);
    h.Validate(
        "InvoiceParser.Read + InvoiceModelValidator.ValidateSource (sémantique, arithmétique) + InvoiceValidator sans exécuteur : étape XSD seulement (CII D16B de l'archive KoSIT v2026-08-31) ; règles métier EN 16931 / XRechnung non exécutées (Java + SaxonJ-HE exigés)",
        Validate,
        Summarize);
    h.Write("InvoiceReadResult.Write(EN 16931 1.3.16, CII, profil lu), modèle lu par InvoiceParser.Read ; contrôles du modèle avant écriture", Parse, Write);
    h.Embed(
        "adaptateur : OfficeIMO.Pdf PdfDocument.Load + Attachments.Edit (Add, ou Replace si l'hôte porte déjà un factur-x.xml) + ToBytes : fichier associé générique, sans XMP Factur-X ni OutputIntent (les aides Factur-X ne servent qu'aux PDF générés par OfficeIMO)",
        Embed,
        Extract);
    h.CheckPdf(
        "OfficeIMO.Pdf PdfDocument.Load + AssessCompliance(FacturX) : relecture des exigences PDF/A-3 + Factur-X du fichier (XMP, fichier associé, OutputIntent, polices…) ; le XML n'est pas validé",
        Check);
});
