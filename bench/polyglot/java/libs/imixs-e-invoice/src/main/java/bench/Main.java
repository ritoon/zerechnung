package bench;

import java.io.ByteArrayInputStream;

import bench.harness.Harness;
import bench.harness.Harness.Lib;
import bench.harness.Harness.Op;
import org.imixs.einvoice.EInvoiceModel;
import org.imixs.einvoice.EInvoiceModelFactory;

/**
 * Imixs e-invoice 1.0.3 (imixs/e-invoice): a read/edit wrapper over the DOM
 * of a CII, UBL or KSeF invoice (getters and setters on the main fields),
 * serialised back with getContent. No validation, no PDF.
 */
public final class Main {

    static final Lib LIB = new Lib("imixs-e-invoice", "org.imixs.util:imixs-e-invoice", "1.0.3");

    static EInvoiceModel parse(byte[] xml) throws Exception {
        return EInvoiceModelFactory.read(new ByteArrayInputStream(xml));
    }

    public static void main(String[] args) {
        Harness.run(LIB, args, h -> {
            h.unsupported(Op.EXTRACT, "pas de lecture de PDF");
            h.parse("EInvoiceModelFactory.read(InputStream) → EInvoiceModelCII (DOM complet, puis lecture des champs "
                    + "d'en-tête, des parties et des lignes par parcours des nœuds)", Main::parse);
            h.unsupported(Op.VALIDATE, "pas de validation");
            h.write("EInvoiceModel.getContent() : resérialise le DOM (indenté) que le modèle enveloppe, modèle lu "
                    + "par EInvoiceModelFactory.read", Main::parse, EInvoiceModel::getContent);
            h.unsupported(Op.EMBED, "pas d'écriture de PDF");
            h.unsupported(Op.CHECK_PDF, "pas de lecture de PDF");
        });
    }
}
