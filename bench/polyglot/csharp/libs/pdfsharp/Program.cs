// Floor: PDFsharp 6.2.4 (empira, MIT; 6.5.0 and later are AGPL-3.0-only), the
// PDF library under ZUGFeRD.PDF-csharp (as PDFsharp-extended), Balsoft,
// International.EInvoicing, FacturXDotNet and Securibox (7.0 preview), with
// the invoice extraction written here: open in import mode, walk
// /Root/Names/EmbeddedFiles (with /Kids), take the first file under a standard
// invoice name (the names of the Rust floor), decoded bytes. Not a Factur-X
// library.

using Bench;
using PdfSharp.Pdf;
using PdfSharp.Pdf.Advanced;
using PdfSharp.Pdf.IO;

var lib = new Lib("pdfsharp", "PDFsharp", "6.2.4");

string[] names = ["factur-x.xml", "zugferd-invoice.xml", "xrechnung.xml", "order-x.xml", "cii.xml"];

static PdfDictionary? Deref(PdfItem? item) => item switch
{
    PdfReference r => r.Value as PdfDictionary,
    PdfDictionary d => d,
    _ => null,
};

static void Walk(PdfDictionary? node, int depth, List<(string Name, PdfItem Spec)> output)
{
    if (node is null || depth > 32)
    {
        return;
    }
    if (node.Elements.GetArray("/Names") is { } pairs)
    {
        for (var i = 0; i + 1 < pairs.Elements.Count; i += 2)
        {
            if (pairs.Elements[i] is PdfString s)
            {
                output.Add((s.Value, pairs.Elements[i + 1]));
            }
        }
    }
    if (node.Elements.GetArray("/Kids") is { } kids)
    {
        foreach (var kid in kids.Elements)
        {
            Walk(Deref(kid), depth + 1, output);
        }
    }
}

byte[] Extract(byte[] pdf)
{
    using var doc = PdfReader.Open(new MemoryStream(pdf, writable: false), PdfDocumentOpenMode.Import);
    var tree = Deref(doc.Internals.Catalog.Elements["/Names"])?.Elements["/EmbeddedFiles"] is { } ef0 ? Deref(ef0) : null;
    if (tree is null)
    {
        throw new InvalidDataException("no /Names/EmbeddedFiles");
    }
    var files = new List<(string Name, PdfItem Spec)>();
    Walk(tree, 0, files);
    var match = files.FirstOrDefault(f => names.Any(n => string.Equals(n, f.Name, StringComparison.OrdinalIgnoreCase)));
    var spec = Deref(match.Spec) ?? throw new InvalidDataException("no attachment under a standard invoice name");
    var ef = Deref(spec.Elements["/EF"]) ?? throw new InvalidDataException("no /EF");
    var stream = Deref(ef.Elements["/UF"] ?? ef.Elements["/F"])?.Stream ?? throw new InvalidDataException("no embedded file stream");
    return stream.UnfilteredValue;
}

return Harness.Run(lib, args, h =>
{
    h.Extract(
        "plancher : extraction écrite à la main sur PDFsharp (PdfReader.Open en Import, arbre /Names/EmbeddedFiles avec /Kids, flux décodé) ; pas une librairie Factur-X",
        Extract);
    foreach (var op in new[] { Op.Parse, Op.Validate, Op.Write, Op.Embed, Op.CheckPdf })
    {
        h.Unsupported(op, "plancher d'extraction seulement");
    }
});
