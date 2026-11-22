// CiusPtBatch applies the XSLT that SchXslt compiles from a schematron, the
// CIUS-PT one of eSPap or the ROmodel phase of the CIUS-RO one of ANAF, to
// XML files with Saxon, in a single JVM, on one thread per processor: the
// stylesheet is compiled once. Used by conformance/parity.sh:
//
//	java -Xss8m -cp Saxon-HE-12.10.jar:xmlresolver-5.3.3.jar:xmlresolver-5.3.3-data.jar CiusPtBatch.java <xslt> <out dir> <list>
//
// The list gives one file per line. The report of a file goes to
// <out dir>/<file name>.svrl: an svrl:schematron-output element holding the
// failed asserts and successful reports of the SVRL report only, with their
// id, flag, location and text (the fired rules make a full report of some
// 75 KB, for thousands of files). When the transformation stops on a dynamic
// error (several values where the test expects one, a value that does not
// cast to xs:decimal), the report is an <error> element instead, with the
// error code and the line of the stylesheet that raised it, as NPaBatch
// writes it, which parity reads.

import java.io.File;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.nio.file.Paths;
import java.util.ArrayList;
import java.util.List;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.Future;

import javax.xml.transform.stream.StreamSource;

import net.sf.saxon.s9api.Processor;
import net.sf.saxon.s9api.QName;
import net.sf.saxon.s9api.SaxonApiException;
import net.sf.saxon.s9api.XPathCompiler;
import net.sf.saxon.s9api.XPathExecutable;
import net.sf.saxon.s9api.XPathSelector;
import net.sf.saxon.s9api.XdmDestination;
import net.sf.saxon.s9api.XdmItem;
import net.sf.saxon.s9api.XdmNode;
import net.sf.saxon.s9api.XsltExecutable;
import net.sf.saxon.s9api.XsltTransformer;

public class CiusPtBatch {
	private static final String SVRL = "http://purl.oclc.org/dsdl/svrl";

	public static void main(String[] args) throws Exception {
		Path xsl = Paths.get(args[0]);
		Path out = Paths.get(args[1]);
		Files.createDirectories(out);
		List<Path> files = new ArrayList<>();
		for (String line : Files.readAllLines(Paths.get(args[2]), StandardCharsets.UTF_8)) {
			if (!line.isBlank()) {
				files.add(Paths.get(line));
			}
		}
		Processor proc = new Processor(false);
		System.err.println("compiling " + xsl);
		XsltExecutable exec = proc.newXsltCompiler().compile(new StreamSource(xsl.toFile()));
		XPathCompiler xp = proc.newXPathCompiler();
		xp.declareNamespace("svrl", SVRL);
		XPathExecutable findings = xp.compile("//svrl:failed-assert | //svrl:successful-report");
		int threads = Runtime.getRuntime().availableProcessors();
		System.err.println(files.size() + " files, " + threads + " threads");
		ExecutorService pool = Executors.newFixedThreadPool(threads);
		List<Future<?>> done = new ArrayList<>();
		for (Path in : files) {
			done.add(pool.submit(() -> {
				validate(proc, exec, findings, in, out.resolve(in.getFileName() + ".svrl"));
				return null;
			}));
		}
		pool.shutdown();
		for (Future<?> f : done) {
			f.get();
		}
	}

	private static void validate(Processor proc, XsltExecutable exec, XPathExecutable findings, Path in, Path report) throws Exception {
		String xml;
		try {
			XsltTransformer t = exec.load();
			t.setSource(new StreamSource(in.toFile()));
			XdmDestination svrl = new XdmDestination();
			t.setDestination(svrl);
			t.transform();
			XPathSelector s = findings.load();
			s.setContextItem(svrl.getXdmNode());
			StringBuilder b = new StringBuilder("<schematron-output xmlns=\"" + SVRL + "\">\n");
			for (XdmItem item : s) {
				XdmNode n = (XdmNode) item;
				String name = n.getNodeName().getLocalName();
				b.append('<').append(name);
				for (String a : new String[] {"id", "flag", "location"}) {
					String v = n.getAttributeValue(new QName(a));
					if (v != null) {
						b.append(' ').append(a).append("=\"").append(escape(v)).append('"');
					}
				}
				b.append("><text>");
				for (XdmNode text : n.children("text")) {
					b.append(escape(text.getStringValue()));
				}
				b.append("</text></").append(name).append(">\n");
			}
			xml = b.append("</schematron-output>\n").toString();
		} catch (SaxonApiException e) {
			String code = e.getErrorCode() == null ? "" : e.getErrorCode().getLocalName();
			xml = error(code, e.getLineNumber(), e.getMessage());
		} catch (Exception e) {
			xml = error("", -1, e.toString());
		}
		Files.writeString(report, xml, StandardCharsets.UTF_8);
	}

	private static String error(String code, int line, String msg) {
		return "<error code=\"" + escape(code) + "\" line=\"" + line + "\">" + escape(String.valueOf(msg)) + "</error>\n";
	}

	private static String escape(String s) {
		return s.replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;").replace("\"", "&quot;");
	}
}
