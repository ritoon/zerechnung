package bench;

import java.util.List;
import java.util.Map;

import bench.harness.Harness;
import bench.harness.Harness.Lib;
import bench.harness.Harness.Op;
import org.apache.pdfbox.Loader;
import org.apache.pdfbox.pdmodel.PDDocument;
import org.apache.pdfbox.pdmodel.PDDocumentNameDictionary;
import org.apache.pdfbox.pdmodel.common.PDNameTreeNode;
import org.apache.pdfbox.pdmodel.common.filespecification.PDComplexFileSpecification;
import org.apache.pdfbox.pdmodel.common.filespecification.PDEmbeddedFile;

/**
 * Floor: Apache PDFBox 3.0.8 with a hand-written extraction (the embedded
 * files name tree, first standard invoice file name), the Java counterpart of
 * the Rust lopdf and Python pypdf floors. Extraction only.
 */
public final class Main {

    static final Lib LIB = new Lib("pdfbox", "org.apache.pdfbox:pdfbox", "3.0.8");

    /** Standard invoice attachment names, compared ignoring ASCII case (as the
     *  Rust lopdf floor does; ZUGFeRD 1 writes ZUGFeRD-invoice.xml). */
    static final List<String> NAMES = List.of("factur-x.xml", "zugferd-invoice.xml", "xrechnung.xml",
            "order-x.xml", "cii.xml");

    static byte[] extract(byte[] pdf) throws Exception {
        try (PDDocument doc = Loader.loadPDF(pdf)) {
            PDDocumentNameDictionary names = doc.getDocumentCatalog().getNames();
            if (names == null || names.getEmbeddedFiles() == null) {
                throw new IllegalStateException("no embedded files");
            }
            PDComplexFileSpecification spec = find(names.getEmbeddedFiles(), 0);
            if (spec == null) {
                throw new IllegalStateException("no invoice attachment");
            }
            PDEmbeddedFile f = spec.getEmbeddedFileUnicode() != null ? spec.getEmbeddedFileUnicode()
                    : spec.getEmbeddedFile();
            if (f == null) {
                throw new IllegalStateException("attachment without /EF stream");
            }
            return f.toByteArray();
        }
    }

    /** Depth-first walk of the name tree (this node's names in key order,
     *  then its kids); the first file under a standard name wins. */
    static PDComplexFileSpecification find(PDNameTreeNode<PDComplexFileSpecification> node, int depth) throws Exception {
        if (depth > 32) {
            return null;
        }
        Map<String, PDComplexFileSpecification> leaves = node.getNames();
        if (leaves != null) {
            for (Map.Entry<String, PDComplexFileSpecification> e : leaves.entrySet()) {
                for (String n : NAMES) {
                    if (n.equalsIgnoreCase(e.getKey())) {
                        return e.getValue();
                    }
                }
            }
        }
        List<PDNameTreeNode<PDComplexFileSpecification>> kids = node.getKids();
        if (kids != null) {
            for (PDNameTreeNode<PDComplexFileSpecification> k : kids) {
                PDComplexFileSpecification s = find(k, depth + 1);
                if (s != null) {
                    return s;
                }
            }
        }
        return null;
    }

    public static void main(String[] args) {
        Harness.run(LIB, args, h -> {
            h.extract("plancher : Loader.loadPDF + arbre /EmbeddedFiles avec /Kids (premier fichier sous un nom "
                    + "standard, casse ignorée : factur-x.xml, zugferd-invoice.xml, xrechnung.xml, order-x.xml, cii.xml) "
                    + "+ PDEmbeddedFile.toByteArray (/UF, sinon /F), écrit à la main", Main::extract);
            h.unsupported(Op.PARSE, "plancher d'extraction seulement");
            h.unsupported(Op.VALIDATE, "plancher d'extraction seulement");
            h.unsupported(Op.WRITE, "plancher d'extraction seulement");
            h.unsupported(Op.EMBED, "plancher d'extraction seulement");
            h.unsupported(Op.CHECK_PDF, "plancher d'extraction seulement");
        });
    }
}
