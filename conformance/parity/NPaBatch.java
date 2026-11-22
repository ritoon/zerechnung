// NPaBatch applies the XSLT of the schematrons of the Dutch Peppol Authority
// (NPa), or those of the EXTENDED-CTC-FR schematrons of the FNFE-MPE, to XML
// files with Saxon, in a single JVM: each stylesheet is compiled once. Used
// by conformance/parity.sh:
//
//	java -cp Saxon-HE-12.10.jar:xmlresolver-5.3.3.jar:xmlresolver-5.3.3-data.jar NPaBatch.java <out dir> <list>
//
// Each line of the list gives a stylesheet and a file, separated by a tab.
// The SVRL report of a file goes to <out dir>/<file name>.svrl. When the
// transformation stops on a dynamic error (a value that does not cast to
// xs:decimal, for instance), the report is an <error> element instead, with
// the error code and the line of the stylesheet that raised it, which
// parity reads.

import java.io.File;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.nio.file.Paths;
import java.util.HashMap;
import java.util.Map;

import javax.xml.transform.stream.StreamSource;

import net.sf.saxon.s9api.Processor;
import net.sf.saxon.s9api.SaxonApiException;
import net.sf.saxon.s9api.XsltCompiler;
import net.sf.saxon.s9api.XsltExecutable;
import net.sf.saxon.s9api.XsltTransformer;

public class NPaBatch {
	public static void main(String[] args) throws Exception {
		Path out = Paths.get(args[0]);
		Files.createDirectories(out);
		Processor proc = new Processor(false);
		XsltCompiler compiler = proc.newXsltCompiler();
		Map<String, XsltExecutable> stylesheets = new HashMap<>();
		for (String line : Files.readAllLines(Paths.get(args[1]), StandardCharsets.UTF_8)) {
			if (line.isBlank()) {
				continue;
			}
			String[] f = line.split("\t", 2);
			XsltExecutable xsl = stylesheets.get(f[0]);
			if (xsl == null) {
				System.err.println("compiling " + f[0]);
				xsl = compiler.compile(new StreamSource(new File(f[0])));
				stylesheets.put(f[0], xsl);
			}
			Path in = Paths.get(f[1]);
			File report = out.resolve(in.getFileName() + ".svrl").toFile();
			try {
				XsltTransformer t = xsl.load();
				t.setSource(new StreamSource(in.toFile()));
				t.setDestination(proc.newSerializer(report));
				t.transform();
			} catch (SaxonApiException e) {
				String code = e.getErrorCode() == null ? "" : e.getErrorCode().getLocalName();
				error(report, code, e.getLineNumber(), e.getMessage());
			} catch (Exception e) {
				error(report, "", -1, e.toString());
			}
		}
	}

	private static void error(File report, String code, int line, String msg) throws Exception {
		String xml = "<error code=\"" + escape(code) + "\" line=\"" + line + "\">" + escape(String.valueOf(msg)) + "</error>\n";
		Files.writeString(report.toPath(), xml, StandardCharsets.UTF_8);
	}

	private static String escape(String s) {
		return s.replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;").replace("\"", "&quot;");
	}
}
