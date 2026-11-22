// Reference validator: SaxonC-HE 13.1.1 (Saxonica, MPL-2.0), Saxon compiled
// to a native library by GraalVM (no JVM), running the official stylesheets,
// as bench/polyglot/python/adapters/saxonche_schematron.py does with saxonche
// (the Python binding of the same SaxonC):
//
// - CEN EN 16931 CII 1.3.17 (ConnectingEurope/eInvoicing-EN16931,
//   xslt/EN16931-CII-validation.xslt) for every invoice;
// - KoSIT XRechnung 3.0.2 schematron 2.6.0 (itplr-kosit/xrechnung-schematron,
//   schematron/cii/XRechnung-CII-validation.xsl) in addition when BT-24 names
//   XRechnung, as the CII scenario of validator-configuration-xrechnung.
//
// Both stylesheets are compiled on the first call (in first_call_ns), then
// reused. Counted as the KoSIT validator does: every assertion flagged fatal
// or error, with the custom levels of validator-configuration-xrechnung
// v2026-08-31 for the XRechnung scenario (CII-SR-465 and CII-SR-466 raised to
// error, CII-SR-475 and CII-SR-476 lowered to information). The SVRL report is
// read with pugixml inside the timed call, as the Python adapter reads it with
// lxml. No XSD (not part of the CEN CII artefacts).
#include <saxonc/SaxonApiException.h>
#include <saxonc/SaxonProcessor.h>
#include <saxonc/XdmNode.h>
#include <saxonc/Xslt30Processor.h>
#include <saxonc/XsltExecutable.h>

#include <pugixml.hpp>

#include <cstring>
#include <map>
#include <memory>
#include <stdexcept>
#include <string>
#include <vector>

#include "harness.hpp"

namespace {

const char *CEN_REL = "cen-1.3.17/xslt/EN16931-CII-validation.xslt";
const char *XR_REL = "xrechnung-2.6.0/schematron/cii/XRechnung-CII-validation.xsl";

const std::map<std::string, std::string> KOSIT_CUSTOM_LEVEL = {
	{"CII-SR-465", "error"}, {"CII-SR-466", "error"}, {"CII-SR-475", "information"}, {"CII-SR-476", "information"}};

struct State {
	SaxonProcessor *proc = nullptr;
	Xslt30Processor *xslt = nullptr;
	XsltExecutable *cen = nullptr, *xr = nullptr;
	std::string error;
};
State S;

std::string saxon_message(SaxonApiException &e) {
	const char *m = e.getMessageWithErrorCode();
	if (!m || !*m)
		m = e.getMessage();
	return m ? m : "SaxonApiException";
}

void compile() {
	if (S.cen)
		return;
	if (!S.error.empty())
		throw std::runtime_error(S.error);
	try {
		for (const char *rel : {CEN_REL, XR_REL}) {
			std::string p = fx::artefact(rel);
			if (FILE *f = std::fopen(p.c_str(), "rb"))
				std::fclose(f);
			else
				throw std::runtime_error(p + " missing: run ./run.sh setup");
		}
		S.proc = new SaxonProcessor(false);
		S.xslt = S.proc->newXslt30Processor();
		// Base URI of the static context: SaxonC wants an absolute path.
		S.xslt->setcwd(fx_artefacts());
		S.cen = S.xslt->compileFromFile(fx::artefact(CEN_REL).c_str());
		S.xr = S.xslt->compileFromFile(fx::artefact(XR_REL).c_str());
		if (!S.cen || !S.xr)
			throw std::runtime_error("compileFromFile returned null");
	} catch (SaxonApiException &e) {
		S.error = "compile: " + saxon_message(e);
		throw std::runtime_error(S.error);
	} catch (const std::exception &e) {
		S.error = e.what();
		throw;
	}
}

// BT-24 names XRechnung (same test as the Python adapter's regex).
bool is_xrechnung(fx::Bytes xml) {
	static const char tag[] = "GuidelineSpecifiedDocumentContextParameter>";
	size_t at = xml.find(tag);
	if (at == fx::Bytes::npos)
		return false;
	size_t p = at + sizeof tag - 1;
	while (p < xml.size() && std::strchr(" \t\r\n", xml[p]))
		p++;
	if (p >= xml.size() || xml[p] != '<')
		return false;
	size_t gt = xml.find('>', p);
	if (gt == fx::Bytes::npos)
		return false;
	fx::Bytes name = xml.substr(p + 1, gt - p - 1);
	size_t colon = name.find(':');
	if ((colon == fx::Bytes::npos ? name : name.substr(colon + 1)) != "ID")
		return false;
	size_t lt = xml.find('<', gt);
	fx::Bytes id = xml.substr(gt + 1, lt == fx::Bytes::npos ? fx::Bytes::npos : lt - gt - 1);
	return id.find("xrechnung") != fx::Bytes::npos;
}

std::string_view local(const char *qname) {
	const char *c = std::strchr(qname, ':');
	return c ? c + 1 : qname;
}

// Rules of the SVRL failed assertions and successful reports flagged fatal or
// error (after the KoSIT levels when `xrechnung`).
void errors(const char *svrl, bool xrechnung, std::vector<std::string> &out) {
	pugi::xml_document d;
	if (!d.load_string(svrl))
		throw std::runtime_error("unreadable SVRL report");
	struct Walker : pugi::xml_tree_walker {
		bool xr;
		std::vector<std::string> *out;
		bool for_each(pugi::xml_node &n) override {
			if (n.type() != pugi::node_element)
				return true;
			std::string_view l = local(n.name());
			if (l != "failed-assert" && l != "successful-report")
				return true;
			std::string rule = n.attribute("id").as_string("?");
			std::string flag = n.attribute("flag").as_string("error");
			for (auto &c : flag)
				c = (char)std::tolower((unsigned char)c);
			if (xr) {
				auto it = KOSIT_CUSTOM_LEVEL.find(rule);
				if (it != KOSIT_CUSTOM_LEVEL.end())
					flag = it->second;
			}
			if (flag == "fatal" || flag == "error")
				out->push_back(rule);
			return true;
		}
	} w;
	w.xr = xrechnung;
	w.out = &out;
	d.traverse(w);
}

struct Report {
	std::vector<std::string> rules;
};

Report validate(fx::Bytes xml) {
	compile();
	if (xml.size() >= 3 && std::memcmp(xml.data(), "\xEF\xBB\xBF", 3) == 0)
		xml.remove_prefix(3);
	std::string text(xml);
	bool xr = is_xrechnung(xml);
	Report rep;
	XdmNode *node = nullptr;
	const char *svrl = nullptr;
	try {
		node = S.proc->parseXmlFromString(text.c_str(), "UTF-8");
		svrl = S.cen->transformToString(node);
		errors(svrl ? svrl : "", xr, rep.rules);
		SaxonProcessor::deleteString(svrl);
		svrl = nullptr;
		if (xr) {
			svrl = S.xr->transformToString(node);
			errors(svrl ? svrl : "", true, rep.rules);
			SaxonProcessor::deleteString(svrl);
			svrl = nullptr;
		}
	} catch (SaxonApiException &e) {
		SaxonProcessor::deleteString(svrl);
		delete node;
		throw std::runtime_error(saxon_message(e));
	} catch (...) {
		SaxonProcessor::deleteString(svrl);
		delete node;
		throw;
	}
	delete node;
	return rep;
}

} // namespace

int main(int argc, char **argv) {
	fx_lib lib{"saxonc-he", "SaxonC-HE", "13.1.1"};
	return fx::run(lib, argc, argv, [](fx_harness *h) {
		auto va = [](fx::Bytes x) { return validate(x); };
		auto su = [](const Report &r) { return fx::Findings{r.rules.size(), r.rules}; };
		fx_unsupported(h, FX_EXTRACT, "validateur seul");
		fx_unsupported(h, FX_PARSE, "validateur seul");
		fx::validate(h,
			     "XSLT officiels CEN EN 16931 CII 1.3.17 (+ KoSIT XRechnung 3.0.2 schematron 2.6.0 si BT-24 "
			     "XRechnung) via SaxonC-HE 13.1.1, compilés au premier appel ; erreurs fatal/error avec les "
			     "niveaux KoSIT",
			     va, su);
		fx_unsupported(h, FX_WRITE, "validateur seul");
		fx_unsupported(h, FX_EMBED, "validateur seul");
		fx_unsupported(h, FX_CHECK_PDF, "validateur seul");
		// Same thread as the GraalVM isolate that SaxonProcessor created.
		delete S.cen;
		delete S.xr;
		delete S.xslt;
		delete S.proc;
		if (S.proc)
			SaxonProcessor::release();
	});
}
