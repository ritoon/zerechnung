package bench;

import java.io.ByteArrayInputStream;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;
import java.util.regex.Matcher;
import java.util.regex.Pattern;

import javax.xml.transform.stream.StreamSource;

import bench.harness.Harness;
import bench.harness.Harness.Findings;
import bench.harness.Harness.Lazy;
import bench.harness.Harness.Lib;
import bench.harness.Harness.Op;
import net.sf.saxon.s9api.DocumentBuilder;
import net.sf.saxon.s9api.Processor;
import net.sf.saxon.s9api.QName;
import net.sf.saxon.s9api.XPathCompiler;
import net.sf.saxon.s9api.XPathExecutable;
import net.sf.saxon.s9api.XPathSelector;
import net.sf.saxon.s9api.XdmDestination;
import net.sf.saxon.s9api.XdmItem;
import net.sf.saxon.s9api.XdmNode;
import net.sf.saxon.s9api.XsltExecutable;
import net.sf.saxon.s9api.XsltTransformer;

/**
 * Reference floor: Saxon-HE 13.1 (s9api) running the official validation
 * stylesheets, the Java port of python/adapters/saxonche_schematron.py.
 *
 * <ul>
 * <li>CEN EN 16931 CII 1.3.17 (ConnectingEurope/eInvoicing-EN16931,
 * xslt/EN16931-CII-validation.xslt) for every invoice;</li>
 * <li>KoSIT XRechnung 3.0.2 schematron 2.6.0 (itplr-kosit/xrechnung-schematron,
 * schematron/cii/XRechnung-CII-validation.xsl) as well when BT-24 names
 * XRechnung, as the CII scenario of validator-configuration-xrechnung.</li>
 * </ul>
 *
 * <p>An error is any failed assertion or successful report of flag fatal or
 * error. For XRechnung, the custom levels of validator-configuration-xrechnung
 * v2026-08-31 apply: CII-SR-465 and CII-SR-466 (warning in the CEN rules)
 * become errors, CII-SR-475 and CII-SR-476 become information. No XSD (the CEN
 * CII artefacts do not ship one).
 */
public final class Main {

    static final Lib LIB = new Lib("saxon-schematron", "net.sf.saxon:Saxon-HE", "13.1");

    static final String CEN_VERSION = "1.3.17";
    static final String XR_VERSION = "2.6.0";

    static final Pattern BT24 =
            Pattern.compile("GuidelineSpecifiedDocumentContextParameter>\\s*<(?:\\w+:)?ID>([^<]+)<");
    static final Map<String, String> KOSIT_CUSTOM_LEVEL = Map.of(
            "CII-SR-465", "error", "CII-SR-466", "error", "CII-SR-475", "information", "CII-SR-476", "information");
    static final QName ID = new QName("id");
    static final QName FLAG = new QName("flag");

    record Compiled(Processor proc, DocumentBuilder builder, XsltExecutable cen, XsltExecutable xr,
                    XPathExecutable findings) {
    }

    /** Both stylesheets compiled once, on first use (inside first_call_ns). */
    static final Lazy<Compiled> COMPILED = Lazy.of(v -> {
        Path cen = Harness.artefacts().resolve("cen-" + CEN_VERSION + "/xslt/EN16931-CII-validation.xslt");
        Path xr = Harness.artefacts().resolve("xrechnung-" + XR_VERSION + "/schematron/cii/XRechnung-CII-validation.xsl");
        for (Path p : List.of(cen, xr)) {
            if (!Files.isRegularFile(p)) {
                throw new IllegalStateException(p + " missing: run ./run.sh setup");
            }
        }
        Processor proc = new Processor(false);
        var comp = proc.newXsltCompiler();
        XPathCompiler xp = proc.newXPathCompiler();
        xp.declareNamespace("svrl", "http://purl.oclc.org/dsdl/svrl");
        return new Compiled(proc, proc.newDocumentBuilder(),
                comp.compile(new StreamSource(cen.toFile())), comp.compile(new StreamSource(xr.toFile())),
                xp.compile("//svrl:failed-assert | //svrl:successful-report"));
    });

    static List<String> errors(Compiled c, XsltExecutable x, XdmNode doc, boolean xrechnung) throws Exception {
        XsltTransformer t = x.load();
        t.setInitialContextNode(doc);
        XdmDestination d = new XdmDestination();
        t.setDestination(d);
        t.transform();
        XPathSelector sel = c.findings().load();
        sel.setContextItem(d.getXdmNode());
        List<String> out = new ArrayList<>();
        for (XdmItem it : sel) {
            XdmNode n = (XdmNode) it;
            String rule = n.getAttributeValue(ID) == null ? "?" : n.getAttributeValue(ID);
            String flag = n.getAttributeValue(FLAG) == null ? "error" : n.getAttributeValue(FLAG).toLowerCase();
            if (xrechnung) {
                flag = KOSIT_CUSTOM_LEVEL.getOrDefault(rule, flag);
            }
            if (flag.equals("fatal") || flag.equals("error")) {
                out.add(rule);
            }
        }
        return out;
    }

    static Findings validate(byte[] xml) throws Exception {
        Compiled c = COMPILED.get();
        XdmNode doc = c.builder().build(new StreamSource(new ByteArrayInputStream(xml)));
        Matcher m = BT24.matcher(new String(xml, 0, Math.min(xml.length, 8192), StandardCharsets.UTF_8));
        boolean xrechnung = m.find() && m.group(1).contains("xrechnung");
        List<String> rules = errors(c, c.cen(), doc, xrechnung);
        if (xrechnung) {
            rules.addAll(errors(c, c.xr(), doc, true));
        }
        return Findings.of(rules);
    }

    public static void main(String[] args) {
        Harness.run(LIB, args, h -> {
            h.unsupported(Op.EXTRACT, "validateur seul");
            h.unsupported(Op.PARSE, "validateur seul");
            h.validate("XSLT officiels CEN EN 16931 CII " + CEN_VERSION + " (+ KoSIT XRechnung 3.0.2 schematron "
                    + XR_VERSION + " si BT-24 XRechnung) via Saxon-HE 13.1 (s9api), compilés au premier appel ; "
                    + "erreurs fatal/error avec les niveaux personnalisés KoSIT v2026-08-31 ; pas de XSD",
                    Main::validate, f -> f);
            h.unsupported(Op.WRITE, "validateur seul");
            h.unsupported(Op.EMBED, "validateur seul");
            h.unsupported(Op.CHECK_PDF, "validateur seul");
        });
    }
}
