package bench;

import java.util.ArrayList;
import java.util.List;

import bench.harness.Harness;
import bench.harness.Harness.Findings;
import bench.harness.Harness.Lib;
import io.github.nalukit.paniolo.Paniolo;
import io.github.nalukit.paniolo.config.FacturXProfile;
import io.github.nalukit.paniolo.config.InvoiceSyntax;
import io.github.nalukit.paniolo.model.Invoice;
import io.github.nalukit.paniolo.validation.Severity;
import io.github.nalukit.paniolo.validation.ValidationMessage;
import io.github.nalukit.paniolo.validation.ValidationResult;

/**
 * Paniolo 1.0.0 (NaluKit/paniolo): EN 16931 model, UBL / CII read and write,
 * validation with the official artefacts it bundles (KoSIT XRechnung
 * configuration, Factur-X / ZUGFeRD schematrons) on Saxon-HE; paniolo-facturx
 * for the PDF side (PDFBox), paniolo-pdfa for PDF/A conformance (veraPDF).
 */
public final class Main {

    static final Lib LIB = new Lib("paniolo", "io.github.nalukit:paniolo+paniolo-facturx+paniolo-pdfa", "1.0.0");

    /** Thread safe and meant to be shared; rules are compiled lazily by the
     *  first validation (inside first_call_ns). */
    static final Paniolo P = Paniolo.create();

    /** Errors: messages of severity ERROR or FATAL, all stages. */
    static Findings summarize(ValidationResult r) {
        List<String> rules = new ArrayList<>();
        for (ValidationMessage m : r.messages()) {
            if (m.severity() == Severity.ERROR || m.severity() == Severity.FATAL) {
                rules.add(m.ruleId() == null || m.ruleId().isEmpty() ? String.valueOf(m.stage()) : m.ruleId());
            }
        }
        return Findings.of(rules);
    }

    static byte[] embed(byte[] host, byte[] xml) {
        return P.embedXml(host, xml, FacturXProfile.EN16931).pdf();
    }

    public static void main(String[] args) {
        Harness.run(LIB, args, h -> {
            h.extract("Paniolo.extractXml(pdf) (paniolo-facturx)", P::extractXml);
            h.parse("Paniolo.read(byte[]) en mode STRICT → Invoice (modèle EN 16931 ; refuse les écarts de montants)",
                    P::read);
            h.validate("Paniolo.validate(byte[]) : XSD + schematrons officiels embarqués choisis par BT-24 "
                    + "(XRechnung 3.0 : configuration KoSIT validator-configuration-xrechnung v2026-08-31, soit CEN CII "
                    + "1.3.16 + KoSIT 2.6.0 et ses niveaux ; EN 16931 simple : les mêmes règles, constats de la CIUS XRechnung "
                    + "(BR-DE, BR-TMP, PEPPOL) ramenés à l'information ; profils Factur-X : schematrons Factur-X 1.09.2) "
                    + "+ contrôle des montants ; "
                    + "erreurs = ERROR/FATAL ; règles compilées au premier appel", P::validate, Main::summarize);
            h.write("Paniolo.toBytes(Invoice, InvoiceSyntax.CII), modèle lu par read", P::read,
                    (Invoice inv) -> P.toBytes(inv, InvoiceSyntax.CII));
            h.embed("Paniolo.embedXml(pdf, xml, FacturXProfile.EN16931).pdf() ; vérifie BT-24, intègre puis revalide le "
                    + "PDF produit (conteneur, PDF/A par veraPDF, XML), revalidation comprise dans la mesure ; ne convertit "
                    + "pas en PDF/A ; refuse un hôte qui contient déjà une facture : host.pdf et host_classic.pdf gardent "
                    + "dans /AF une spécification factur-x.xml avec son flux /UF (seul l'arbre EmbeddedFiles a été retiré), "
                    + "que Paniolo lit (PdfBoxFacturXSupport.allFiles)", Main::embed, P::extractXml);
            h.checkPdf("Paniolo.validate sur le PDF : conteneur Factur-X (pièce jointe, relation, XMP), PDF/A par "
                    + "veraPDF (paniolo-pdfa), puis validation complète du XML embarqué pour le profil déclaré",
                    P::validate);
        });
    }
}
