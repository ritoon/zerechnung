package bench;

import java.io.ByteArrayInputStream;
import java.io.ByteArrayOutputStream;
import java.util.ArrayList;
import java.util.List;

import bench.harness.Harness;
import bench.harness.Harness.Findings;
import bench.harness.Harness.Lib;
import org.mustangproject.Invoice;
import org.mustangproject.ZUGFeRD.Profiles;
import org.mustangproject.ZUGFeRD.ZUGFeRD2PullProvider;
import org.mustangproject.ZUGFeRD.ZUGFeRDExporterFromPDFA;
import org.mustangproject.ZUGFeRD.ZUGFeRDImporter;
import org.mustangproject.ZUGFeRD.ZUGFeRDInvoiceImporter;
import org.mustangproject.validator.ESeverity;
import org.mustangproject.validator.ValidationResultItem;
import org.mustangproject.validator.ZUGFeRDValidator;

/**
 * Mustang 2.26.0 (ZUGFeRD/mustangproject): org.mustangproject:library (read,
 * write, embed on PDFBox) and org.mustangproject:validator (XSD + Factur-X /
 * XRechnung schematrons through ph-schematron and Saxon-HE, PDF/A through
 * veraPDF), in one JVM.
 */
public final class Main {

    static final Lib LIB = new Lib("mustang", "org.mustangproject:library+validator", "2.26.0");

    /** The report string and a snapshot of the context's items (the context
     *  is reused by the next call). */
    record Report(String xml, List<ValidationResultItem> items) {
    }

    static byte[] extract(byte[] pdf) {
        return new ZUGFeRDImporter(new ByteArrayInputStream(pdf)).getRawXML();
    }

    /** One pass: setRawXML(…, false) builds the DOM, extractInvoice fills the
     *  model. fromXML + extractInvoice (the documented route) would build the
     *  model twice. */
    static Invoice parse(byte[] xml) throws Exception {
        ZUGFeRDInvoiceImporter zii = new ZUGFeRDInvoiceImporter();
        zii.setRawXML(xml, false);
        return zii.extractInvoice();
    }

    static Report validate(byte[] doc, String name) {
        ZUGFeRDValidator zv = new ZUGFeRDValidator();
        String xml = zv.validate(doc, name);
        return new Report(xml, List.copyOf(zv.getContext().getResults()));
    }

    /** Errors: items of severity error, fatal or exception (notices and
     *  warnings do not invalidate, as in ValidationContext.isValid). */
    static Findings summarize(Report r) {
        List<String> rules = new ArrayList<>();
        for (ValidationResultItem i : r.items()) {
            ESeverity s = i.getSeverity();
            if (s == ESeverity.error || s == ESeverity.fatal || s == ESeverity.exception) {
                String id = i.getID();
                rules.add(id != null && !id.isEmpty() ? id : "section " + i.getSection());
            }
        }
        return Findings.of(rules);
    }

    static byte[] write(Invoice inv) {
        ZUGFeRD2PullProvider p = new ZUGFeRD2PullProvider();
        p.setProfile(Profiles.getByName("EN16931"));
        p.generateXML(inv);
        return p.getXML();
    }

    static byte[] embed(byte[] host, byte[] xml) throws Exception {
        ZUGFeRDExporterFromPDFA e = new ZUGFeRDExporterFromPDFA();
        e.load(host);
        e.setXML(xml);
        ByteArrayOutputStream out = new ByteArrayOutputStream(host.length + xml.length + 65536);
        e.export(out);
        return out.toByteArray();
    }

    public static void main(String[] args) {
        Harness.run(LIB, args, h -> {
            h.extract("new ZUGFeRDImporter(InputStream).getRawXML() ; l'import analyse aussi le XML "
                    + "en modèle Invoice (setRawXML avec analyse automatique), inclus dans la mesure", Main::extract);
            h.parse("ZUGFeRDInvoiceImporter.setRawXML(xml, false) + extractInvoice → Invoice "
                    + "(refuse une facture dont il ne retrouve pas les totaux)", Main::parse);
            h.validate("ZUGFeRDValidator.validate(byte[], nom) : XSD + schematron Factur-X 1.09.2 / ZUGFeRD 2.5.2 du profil "
                    + "(EN 16931 pour les exemples EN16931), XRechnung 3.0.2 (schematron KoSIT 2.4.0 + CEN CII 1.3.16) si BT-24 "
                    + "XRechnung, contrôle arithmétique ; règles BR-FR seulement si vendeur et acheteur sont français ; "
                    + "erreurs = items error/fatal/exception ; XSLT compilés au premier appel",
                    x -> validate(x, "invoice.xml"), Main::summarize);
            h.write("ZUGFeRD2PullProvider.generateXML (profil EN16931), modèle lu par setRawXML + extractInvoice",
                    Main::parse, Main::write);
            h.embed("ZUGFeRDExporterFromPDFA.load + setXML + export (profil EN16931 par défaut) ; hôtes PDF/A-3B, "
                    + "donc ZUGFeRDExporterFromA3 ; un hôte non PDF/A est refusé (PDF/A-1 converti en A-3 au niveau des "
                    + "métadonnées) ; enregistré sans compression ; le PDF produit (PDF/A-3U) garde dans /AF la "
                    + "spécification factur-x.xml résiduelle de l'hôte à côté de la nouvelle : le validateur Mustang "
                    + "l'accepte, Paniolo le refuse (FX-MULTIPLE-INVOICES)", Main::embed, Main::extract);
            h.checkPdf("ZUGFeRDValidator.validate sur le PDF : PDF/A-3 par veraPDF, XMP et pièce jointe Factur-X, "
                    + "puis validation complète du XML extrait (comme ParseValidate)", x -> validate(x, "invoice.pdf"));
        });
    }
}
