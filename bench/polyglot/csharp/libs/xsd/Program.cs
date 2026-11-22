// Floor: XSD validation alone with the BCL (System.Xml.Schema.XmlSchemaSet, a
// validating XmlReader), against the UN/CEFACT CII D16B schemas that the KoSIT
// configuration ships (validator-configuration-xrechnung v2026-08-31,
// resources/cii/16b/xsd). Not a Factur-X library: it shows the cost of the
// schema layer that most libraries put in front of their rules.

using System.Xml;
using System.Xml.Schema;
using Bench;

var lib = new Lib("xsd", "System.Xml.Schema (BCL)", Environment.Version.ToString());

XmlSchemaSet? schemas = null;

XmlSchemaSet Schemas()
{
    if (schemas != null)
    {
        return schemas;
    }
    var main = Path.Combine(Corpus.Artefacts, "kosit-config-2026-08-31", "resources", "cii", "16b", "xsd", "CrossIndustryInvoice_100pD16B.xsd");
    if (!File.Exists(main))
    {
        throw new FileNotFoundException($"{main} missing: run ./run.sh setup");
    }
    var set = new XmlSchemaSet { XmlResolver = new XmlUrlResolver() };
    set.Add(null, main);
    set.Compile();
    return schemas = set;
}

List<string> Validate(byte[] xml)
{
    var errors = new List<string>();
    var settings = new XmlReaderSettings
    {
        ValidationType = ValidationType.Schema,
        Schemas = Schemas(),
        DtdProcessing = DtdProcessing.Prohibit,
        XmlResolver = null,
    };
    settings.ValidationEventHandler += (_, e) =>
    {
        if (e.Severity == XmlSeverityType.Error)
        {
            errors.Add(e.Message.Length > 120 ? e.Message[..120] : e.Message);
        }
    };
    using var ms = new MemoryStream(xml, writable: false);
    using var reader = XmlReader.Create(ms, settings);
    while (reader.Read())
    {
    }
    return errors;
}

return Harness.Run(lib, args, h =>
{
    h.Unsupported(Op.Extract, "plancher de validation XSD seulement");
    h.Unsupported(Op.Parse, "plancher de validation XSD seulement");
    h.Validate(
        "plancher : XmlReader validant (XmlSchemaSet compilé une fois) contre les XSD UN/CEFACT CII D16B de la configuration KoSIT v2026-08-31 ; pas de règles métier",
        Validate,
        rules => Findings.Of(rules));
    foreach (var op in new[] { Op.Write, Op.Embed, Op.CheckPdf })
    {
        h.Unsupported(op, "plancher de validation XSD seulement");
    }
});
