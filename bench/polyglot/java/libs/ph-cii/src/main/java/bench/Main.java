package bench;

import java.util.ArrayList;
import java.util.List;

import bench.harness.Harness;
import bench.harness.Harness.Findings;
import bench.harness.Harness.Lib;
import bench.harness.Harness.Op;
import com.helger.cii.d16b.CIID16BCrossIndustryInvoiceTypeMarshaller;
import com.helger.diagnostics.error.IError;
import com.helger.diagnostics.error.list.ErrorList;
import un.unece.uncefact.data.standard.crossindustryinvoice._100.CrossIndustryInvoiceType;

/**
 * ph-cii 10.1.0 (phax/ph-cii): JAXB binding of the UN/CEFACT CII D16B schema
 * (ph-cii-d16b), read and write through ph-jaxb's GenericJAXBMarshaller, with
 * optional XSD validation. No business rules, no PDF.
 */
public final class Main {

    static final Lib LIB = new Lib("ph-cii", "com.helger.cii:ph-cii-d16b", "10.1.0");

    /** Marshallers are not thread safe but reusable; one per configuration. */
    static final CIID16BCrossIndustryInvoiceTypeMarshaller PLAIN =
            (CIID16BCrossIndustryInvoiceTypeMarshaller) new CIID16BCrossIndustryInvoiceTypeMarshaller().setUseSchema(false);
    static final CIID16BCrossIndustryInvoiceTypeMarshaller CHECKED = new CIID16BCrossIndustryInvoiceTypeMarshaller();

    static CrossIndustryInvoiceType parse(byte[] xml) {
        return PLAIN.read(xml);
    }

    record Checked(CrossIndustryInvoiceType doc, ErrorList errors) {
    }

    /** Read with the D16B schema (the marshaller's default), collecting the
     *  validation events instead of only logging them. */
    static Checked validate(byte[] xml) {
        ErrorList errors = new ErrorList();
        CHECKED.setCollectErrors(errors);
        return new Checked(CHECKED.read(xml), errors);
    }

    static Findings summarize(Checked c) {
        List<String> rules = new ArrayList<>();
        for (IError e : c.errors()) {
            if (e.isError()) {
                rules.add("XSD");
            }
        }
        if (rules.isEmpty() && c.doc() == null) {
            rules.add("XSD: not read");
        }
        return Findings.of(rules);
    }

    static byte[] write(CrossIndustryInvoiceType doc) {
        return PLAIN.getAsBytes(doc);
    }

    public static void main(String[] args) {
        Harness.run(LIB, args, h -> {
            h.unsupported(Op.EXTRACT, "pas de lecture de PDF");
            h.parse("CIID16BCrossIndustryInvoiceTypeMarshaller.read(byte[]) sans schéma (setUseSchema(false)) "
                    + "→ CrossIndustryInvoiceType (JAXB)", Main::parse);
            h.validate("CIID16BCrossIndustryInvoiceTypeMarshaller.read avec le XSD CII D16B (réglage par défaut) et "
                    + "setCollectErrors : XSD seulement, aucune règle métier EN 16931", Main::validate, Main::summarize);
            h.write("getAsBytes(CrossIndustryInvoiceType) sans schéma, modèle lu par read", Main::parse, Main::write);
            h.unsupported(Op.EMBED, "pas d'écriture de PDF");
            h.unsupported(Op.CHECK_PDF, "pas de lecture de PDF");
        });
    }
}
