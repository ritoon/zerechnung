// International.EInvoicing 1.1.0 (Epic Dice Studio, MIT): a managed model of
// EN 16931 with UBL, CII, Factur-X / ZUGFeRD and XRechnung readers and
// writers, and its own Schematron engine (an XPath subset evaluated over the
// preprocessed official .sch files, no XSLT). Packages used here:
//
// - International.EInvoicing: facade, CII/UBL readers and writers, EN 16931
//   rules (CEN artefacts 1.3.16, embedded) through AddDefaults();
// - .Validation.XRechnung: KoSIT XRechnung CII/UBL rules (embedded);
// - .Validation.Xsd: UN/CEFACT CII D16B schemas (embedded);
// - .Countries.Germany: the XRechnung profiles;
// - .FacturX.PdfSharp: reading and writing the PDF part on PDFsharp 6.2.4.
//
// The Factur-X profile rules (FeRD schematron) and the French rules are
// fetched, not shipped (AddFacturXRulesFrom, AddFrance): they are not used, so
// the validation is the most complete offline set the packages carry.

using System.Text;
using Bench;
using International.EInvoicing;
using International.EInvoicing.Countries.Germany;
using International.EInvoicing.FacturX;
using International.EInvoicing.FacturX.Pdf;
using International.EInvoicing.FacturX.PdfSharp;
using International.EInvoicing.Model;
using International.EInvoicing.Validation;
using International.EInvoicing.Validation.XRechnung;
using International.EInvoicing.Validation.Xsd;

var lib = new Lib("international-einvoicing", "International.EInvoicing", "1.1.0");

// One instance, built once (as the README advises): rule sets are loaded and
// compiled lazily on the first Validate, which lands in first_call_ns.
var pdfReader = new PdfSharpAttachmentReader();
var pdfWriter = new PdfSharpAttachmentWriter();
var einvoicing = EInvoicing.Create(
    e => e.AddDefaults().AddGermany().AddXRechnungRules().AddCiiSchema(),
    pdfReader);

const long MaxAttachmentBytes = 64L * 1024 * 1024; // the library's default limit

byte[] Extract(byte[] pdf)
{
    using var ms = new MemoryStream(pdf, writable: false);
    return pdfReader.FindAttachment(ms, FacturXAttachment.KnownFileNames, MaxAttachmentBytes)?.Xml
        ?? throw new InvalidDataException("FindAttachment returned null (no factur-x.xml, zugferd-invoice.xml or xrechnung.xml)");
}

EInvoice Parse(byte[] xml)
{
    var read = einvoicing.Read(xml);
    if (read.TryGetInvoice(out var invoice))
    {
        return invoice;
    }
    var first = read.Errors.FirstOrDefault();
    throw new InvalidDataException($"Read: no invoice ({read.Kind}){(first is null ? "" : ": " + first)}");
}

string Text(byte[] xml)
{
    using var reader = new StreamReader(new MemoryStream(xml, writable: false), Encoding.UTF8, detectEncodingFromByteOrderMarks: true);
    return reader.ReadToEnd();
}

static Findings Summarize(ValidationReport r) =>
    Findings.Of(r.Errors.Select(m => m.RuleIdentifier));

byte[] Embed(byte[] host, byte[] xml)
{
    using var src = new MemoryStream(host, writable: false);
    using var dst = new MemoryStream();
    pdfWriter.Attach(src, new FacturXAttachment(FacturXAttachment.FacturXFileName, xml), FacturXProfiles.En16931, dst);
    return dst.ToArray();
}

return Harness.Run(lib, args, h =>
{
    h.Extract(
        "PdfSharpAttachmentReader.FindAttachment (PDFsharp 6.2.4) : noms factur-x.xml, zugferd-invoice.xml, ZUGFeRD-invoice.xml, xrechnung.xml, octets bruts",
        Extract);
    h.Parse("EInvoicing.Read(byte[]) → EInvoice (détection de syntaxe et de profil, lecteur CII)", Parse);
    h.Validate(
        "EInvoicing.Validate(string) : XSD CII D16B + règles EN 16931 CII 1.3.16 + XRechnung CII (KoSIT, embarquées) si BT-24 XRechnung, moteur Schematron maison (sous-ensemble XPath) ; règles Factur-X et françaises non embarquées (à télécharger), donc absentes",
        x => einvoicing.Validate(Text(x)),
        Summarize);
    h.Write("EInvoicing.Write(EInvoice, DocumentFormat.Cii), modèle lu par Read", Parse, m => einvoicing.Write(m, DocumentFormat.Cii));
    h.Embed(
        "PdfSharpAttachmentWriter.Attach(profil EN 16931) : fichier associé factur-x.xml (/AF, arbre des noms), XMP Factur-X ajouté ou complété, XML embarqué tel quel ; ModDate = DateTimeOffset.UtcNow",
        Embed,
        Extract);
    h.Unsupported(Op.CheckPdf, "pas de contrôle de PDF hybride : la lecture d'un PDF compare seulement le XMP au XML (avertissements), sans PDF/A");
});
