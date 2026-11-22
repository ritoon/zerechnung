package bench;

import java.nio.charset.StandardCharsets;
import java.util.List;

import bench.harness.Harness;
import bench.harness.Harness.Findings;
import bench.harness.Harness.Lib;
import bench.harness.Harness.Op;
import io.github.jcodeforge.invoice4jbase.datamodels.pojos.Invoice;
import io.github.jcodeforge.invoice4jbase.exceptions.InvoiceValidationException;
import io.github.jcodeforge.invoice4jbase.exceptions.XsdValidationException;
import io.github.jcodeforge.invoice4jzugferd.ZugferdInvoiceReader;
import io.github.jcodeforge.invoice4jzugferd.ZugferdInvoiceWriter;
import io.github.jcodeforge.invoice4jzugferd.validation.ZugferdBasicValidator;

/**
 * invoice4j-zugferd 0.5.0 (jcodeforge/invoice4j, on invoice4j-base): CII read
 * and write for the five Factur-X profiles, Factur-X 1.09 XSD of the profile,
 * three business rules (BR-CO-15, BR-CO-26, BR-S-2). The PDF reader and writer
 * only take java.io.File arguments.
 */
public final class Main {

    static final Lib LIB = new Lib("invoice4j", "io.github.jcodeforge:invoice4j-zugferd", "0.5.0");

    static final ZugferdInvoiceReader PLAIN = ZugferdInvoiceReader.builder().validate(false).build();
    static final ZugferdInvoiceReader CHECKED = ZugferdInvoiceReader.builder().build();
    static final ZugferdInvoiceWriter WRITER = ZugferdInvoiceWriter.builder().validate(false).build();

    static Invoice parse(byte[] xml) {
        return PLAIN.readFromString(new String(xml, StandardCharsets.UTF_8));
    }

    /**
     * The reader's default validation (Factur-X 1.09 XSD of the detected
     * profile), then ZugferdBasicValidator on the model. Both stop at the
     * first error, so a flagged file has one finding. An unknown BT-24
     * (XRechnung) is not a finding: the reader rejects the document.
     */
    static Findings validate(byte[] xml) {
        String s = new String(xml, StandardCharsets.UTF_8);
        CHECKED.detectProfile(s);
        Invoice inv;
        try {
            inv = CHECKED.readFromString(s);
        } catch (XsdValidationException e) {
            return Findings.of(List.of("XSD"));
        }
        try {
            new ZugferdBasicValidator().validate(inv);
        } catch (InvoiceValidationException e) {
            String m = e.getMessage() == null ? "" : e.getMessage();
            int colon = m.indexOf(':');
            return Findings.of(List.of(colon > 0 ? m.substring(0, colon) : m));
        }
        return Findings.of(List.of());
    }

    public static void main(String[] args) {
        Harness.run(LIB, args, h -> {
            h.unsupported(Op.EXTRACT, "ZugferdPdfReader.read(File) seulement (pas d'API sur des octets en mémoire)");
            h.parse("ZugferdInvoiceReader (validate(false)).readFromString → Invoice ; profil détecté par BT-24, "
                    + "XRechnung refusé (« Unsupported ZUGFeRD profile »)", Main::parse);
            h.validate("ZugferdInvoiceReader (validation par défaut : XSD Factur-X 1.09 du profil) + ZugferdBasicValidator "
                    + "(3 règles : BR-CO-15, BR-CO-26, BR-S-2), arrêt à la première erreur ; XRechnung non pris en charge ; "
                    + "les .sch embarqués ne sont jamais exécutés", Main::validate, f -> f);
            h.write("ZugferdInvoiceWriter (profil EN16931, validate(false)).writeToString, modèle lu par readFromString ; "
                    + "le lecteur perd les identifiants du vendeur (TVA, immatriculation), d'où BR-S-02 et BR-CO-26 au "
                    + "schematron CEN sur la sortie", Main::parse, WRITER::writeToString);
            h.unsupported(Op.EMBED, "ZugferdPdfWriter.write(File, File) seulement (pas d'API sur des octets en mémoire)");
            h.unsupported(Op.CHECK_PDF, "pas de contrôle de PDF");
        });
    }
}
