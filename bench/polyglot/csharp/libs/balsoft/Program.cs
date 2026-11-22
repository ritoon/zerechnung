// Balsoft.Hive.EInvoice 1.1.0 + Balsoft.Hive.EInvoice.Pdf 1.1.0 (Balsoft,
// Apache-2.0): a hand-written EN 16931 model with CII and UBL readers and
// writers, a validator coded in C# (business rules on the model, no XSD, no
// schematron) and hybrid PDF creation on PDFsharp 6.2.4.

using Balsoft.Hive.EInvoice;
using Balsoft.Hive.EInvoice.Cii;
using Balsoft.Hive.EInvoice.Pdf;
using Balsoft.Hive.EInvoice.Reading;
using Balsoft.Hive.EInvoice.Validation;
using Bench;

var lib = new Lib("balsoft", "Balsoft.Hive.EInvoice + Balsoft.Hive.EInvoice.Pdf", "1.1.0");

static byte[] Extract(byte[] pdf) =>
    HybridPdf.ExtractXml(pdf) ?? throw new InvalidDataException("ExtractXml returned null (no embedded XML)");

static ReadResult Parse(byte[] xml) => InvoiceReader.Read(xml);

// The validator works on the model; the stated totals that the model does not
// keep are compared with the computed ones by ReadResult.TotalsDiscrepancies.
static (ValidationResult Rules, IReadOnlyList<string> Totals) Validate(byte[] xml)
{
    var read = InvoiceReader.Read(xml);
    return (InvoiceValidator.Validate(read.Invoice, read.Profile ?? InvoiceProfile.EN16931), read.TotalsDiscrepancies());
}

static Findings Summarize((ValidationResult Rules, IReadOnlyList<string> Totals) r) =>
    Findings.Of(r.Rules.Errors.Select(i => i.RuleId).Concat(r.Totals.Select(t => t.Split(' ')[0])));

var noValidation = new WriterOptions { ValidateBeforeWriting = false };

return Harness.Run(lib, args, h =>
{
    h.Extract("HybridPdf.ExtractXml : arbre des noms /EmbeddedFiles (factur-x.xml, xrechnung.xml, zugferd-invoice.xml, sinon le premier .xml), octets bruts", Extract);
    h.Parse("InvoiceReader.Read (XDocument puis lecteur CII ou UBL selon l'espace de noms racine)", Parse);
    h.Validate(
        "InvoiceReader.Read + InvoiceValidator.Validate(profil lu, EN16931 à défaut) + ReadResult.TotalsDiscrepancies : règles codées en C# sur le modèle (BR-*, codes, profils XRechnung/Factur-X), pas de XSD ni de schematron",
        Validate,
        Summarize);
    h.Write(
        "CiiWriter.Write(profil lu) avec ValidateBeforeWriting=false (activé par défaut : la validation est mesurée par ParseValidate) ; modèle lu par InvoiceReader.Read",
        Parse,
        r => CiiWriter.Write(r.Invoice, r.Profile ?? InvoiceProfile.EN16931, noValidation));
    h.Embed(
        "HybridPdf.Create(octets XML, EN16931) : recopie les pages dans un nouveau document PDFsharp, factur-x.xml en fichier associé, OutputIntent sRGB, polices manquantes prises dans les dossiers système, XMP remplacé par mise à jour incrémentale ; dates = DateTimeOffset.Now (réglable, laissé par défaut)",
        (host, xml) => HybridPdf.Create(host, xml, InvoiceProfile.EN16931),
        Extract);
    h.Unsupported(Op.CheckPdf, "pas de contrôle de PDF hybride");
});
