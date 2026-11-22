// Floor: PdfPig 0.1.16 (UglyToad, Apache-2.0), the most downloaded managed PDF
// reader on NuGet, with the invoice extraction written here: open the
// document, list the embedded files (/Names/EmbeddedFiles, /Kids included),
// take the first under a standard invoice name (the names of the Rust floor),
// decoded bytes. Not a Factur-X library.

using Bench;
using UglyToad.PdfPig;

var lib = new Lib("pdfpig", "PdfPig", "0.1.16");

string[] names = ["factur-x.xml", "zugferd-invoice.xml", "xrechnung.xml", "order-x.xml", "cii.xml"];

byte[] Extract(byte[] pdf)
{
    using var doc = PdfDocument.Open(pdf);
    if (!doc.Advanced.TryGetEmbeddedFiles(out var files))
    {
        throw new InvalidDataException("no /Names/EmbeddedFiles");
    }
    var file = files.FirstOrDefault(f => names.Any(n => string.Equals(n, f.Name, StringComparison.OrdinalIgnoreCase)))
        ?? throw new InvalidDataException("no attachment under a standard invoice name");
    return file.Memory.ToArray();
}

return Harness.Run(lib, args, h =>
{
    h.Extract(
        "plancher : extraction écrite à la main sur PdfPig (PdfDocument.Open, Advanced.TryGetEmbeddedFiles : arbre /Names/EmbeddedFiles avec /Kids, flux décodé) ; pas une librairie Factur-X",
        Extract);
    foreach (var op in new[] { Op.Parse, Op.Validate, Op.Write, Op.Embed, Op.CheckPdf })
    {
        h.Unsupported(op, "plancher d'extraction seulement");
    }
});
