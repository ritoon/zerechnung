// Floor: Apache Xerces-C++ 3.3.0 (Apache-2.0), the validating XML parser of
// the C++ world, with no knowledge of Factur-X:
//
// - Parse: XercesDOMParser (namespaces on, no validation) on a
//   MemBufInputSource; the DOM document is adopted and released per call.
// - ParseValidate: a second parser with W3C XML Schema validation against
//   the UN/CEFACT CII D16B schema, loaded once with loadGrammar and cached
//   (its cost is in first_call_ns of ParseValidate_Small); errors counted by
//   an ErrorHandler. The schema only: no EN 16931 business rule.
// - Write: DOMLSSerializer of the parsed DOM to a UTF-8 MemBufFormatTarget.
//
// Both parsers live for the whole process and are reused, as a long-lived
// service would.
#include <xercesc/dom/DOM.hpp>
#include <xercesc/framework/MemBufFormatTarget.hpp>
#include <xercesc/framework/MemBufInputSource.hpp>
#include <xercesc/parsers/XercesDOMParser.hpp>
#include <xercesc/sax/ErrorHandler.hpp>
#include <xercesc/sax/SAXParseException.hpp>
#include <xercesc/util/PlatformUtils.hpp>
#include <xercesc/util/XMLString.hpp>
#include <xercesc/util/XercesVersion.hpp>
#include <xercesc/validators/common/Grammar.hpp>

#include <memory>
#include <stdexcept>
#include <string>
#include <vector>

#include "harness.hpp"

using namespace xercesc;

namespace {

const char *XSD_REL = "kosit-config/resources/cii/16b/xsd/CrossIndustryInvoice_100pD16B.xsd";

std::string transcode(const XMLCh *s) {
	if (!s)
		return "";
	char *c = XMLString::transcode(s);
	std::string out(c ? c : "");
	XMLString::release(&c);
	return out;
}

// Counts errors and fatal errors, keeps the first messages.
class Collector : public ErrorHandler {
public:
	size_t errors = 0;
	std::vector<std::string> messages;
	void warning(const SAXParseException &) override {}
	void error(const SAXParseException &e) override { add(e); }
	void fatalError(const SAXParseException &e) override { add(e); }
	void resetErrors() override {
		errors = 0;
		messages.clear();
	}

private:
	void add(const SAXParseException &e) {
		errors++;
		if (messages.size() < 20)
			messages.push_back("XSD:" + std::to_string(e.getLineNumber()) + ": " + transcode(e.getMessage()));
	}
};

// DOM document released when the model is dropped.
struct Doc {
	DOMDocument *d = nullptr;
	explicit Doc(DOMDocument *p) : d(p) {}
	Doc(Doc &&o) noexcept : d(o.d) { o.d = nullptr; }
	Doc(const Doc &) = delete;
	~Doc() {
		if (d)
			d->release();
	}
};

struct Report {
	size_t errors;
	std::vector<std::string> messages;
};

std::unique_ptr<XercesDOMParser> plain, validating;
Collector plain_errors, validating_errors;

std::string what(const XMLException &e) { return transcode(e.getMessage()); }

Doc parse(fx::Bytes xml) {
	if (!plain) {
		plain = std::make_unique<XercesDOMParser>();
		plain->setDoNamespaces(true);
		plain->setValidationScheme(XercesDOMParser::Val_Never);
		plain->setLoadExternalDTD(false);
		plain->setErrorHandler(&plain_errors);
	}
	plain_errors.resetErrors();
	MemBufInputSource src(reinterpret_cast<const XMLByte *>(xml.data()), xml.size(), "invoice.xml", false);
	try {
		plain->parse(src);
	} catch (const XMLException &e) {
		throw std::runtime_error(what(e));
	} catch (const DOMException &e) {
		throw std::runtime_error(transcode(e.getMessage()));
	}
	if (plain_errors.errors)
		throw std::runtime_error(plain_errors.messages.empty() ? "parse error" : plain_errors.messages[0]);
	return Doc(plain->adoptDocument());
}

XercesDOMParser &validator() {
	if (validating)
		return *validating;
	std::string path = fx::artefact(XSD_REL);
	auto p = std::make_unique<XercesDOMParser>();
	p->setDoNamespaces(true);
	p->setDoSchema(true);
	p->setValidationScheme(XercesDOMParser::Val_Always);
	p->setValidationSchemaFullChecking(false);
	p->setLoadExternalDTD(false);
	p->setErrorHandler(&validating_errors);
	validating_errors.resetErrors();
	Grammar *g = nullptr;
	try {
		g = p->loadGrammar(path.c_str(), Grammar::SchemaGrammarType, true);
	} catch (const XMLException &e) {
		throw std::runtime_error("loadGrammar " + path + ": " + what(e));
	}
	if (!g || validating_errors.errors)
		throw std::runtime_error("cannot load " + path + " (run ./run.sh setup)" +
					 (validating_errors.messages.empty() ? "" : ": " + validating_errors.messages[0]));
	p->useCachedGrammarInParse(true);
	validating = std::move(p);
	return *validating;
}

std::string write(const Doc &doc) {
	static DOMImplementationLS *impl =
		static_cast<DOMImplementationLS *>(DOMImplementationRegistry::getDOMImplementation(u"LS"));
	if (!impl)
		throw std::runtime_error("no DOM LS implementation");
	DOMLSSerializer *ser = impl->createLSSerializer();
	DOMLSOutput *out = impl->createLSOutput();
	MemBufFormatTarget target(64 * 1024);
	out->setByteStream(&target);
	out->setEncoding(u"UTF-8");
	bool ok = false;
	try {
		ok = ser->write(doc.d, out);
	} catch (...) {
		out->release();
		ser->release();
		throw;
	}
	std::string s(reinterpret_cast<const char *>(target.getRawBuffer()), target.getLen());
	out->release();
	ser->release();
	if (!ok)
		throw std::runtime_error("DOMLSSerializer::write failed");
	return s;
}

Report validate(fx::Bytes xml) {
	XercesDOMParser &p = validator();
	validating_errors.resetErrors();
	MemBufInputSource src(reinterpret_cast<const XMLByte *>(xml.data()), xml.size(), "invoice.xml", false);
	try {
		p.parse(src);
	} catch (const XMLException &e) {
		throw std::runtime_error(what(e));
	}
	Doc d(p.adoptDocument());
	return Report{validating_errors.errors, validating_errors.messages};
}

} // namespace

int main(int argc, char **argv) {
	try {
		XMLPlatformUtils::Initialize();
	} catch (const XMLException &e) {
		std::fprintf(stderr, "Xerces init: %s\n", what(e).c_str());
		return 1;
	}
	fx_lib lib{"xerces-c", "xerces-c", XERCES_FULLVERSIONDOT};
	int r = fx::run(lib, argc, argv, [](fx_harness *h) {
		auto pa = [](fx::Bytes x) { return parse(x); };
		auto va = [](fx::Bytes x) { return validate(x); };
		auto su = [](const Report &r) { return fx::Findings{r.errors, r.messages}; };
		fx_unsupported(h, FX_EXTRACT, "librairie XML seulement");
		fx::parse(h, "plancher : DOM Xerces (XercesDOMParser réutilisé, sans validation), pas un modèle de facture",
			  pa);
		fx::validate(h,
			     "plancher : XercesDOMParser avec validation XML Schema, XSD UN/CEFACT CII D16B (configuration "
			     "KoSIT XRechnung 3.0.2 du 2026-08-31) chargé une fois (loadGrammar, mis en cache) ; schéma "
			     "seulement, aucune règle métier EN 16931",
			     va, su);
		auto wr = [](const Doc &d) { return write(d); };
		fx::write(h, "plancher : sérialisation du DOM Xerces (DOMLSSerializer, UTF-8), pas un modèle de facture", pa,
			  wr);
		fx_unsupported(h, FX_EMBED, "librairie XML seulement");
		fx_unsupported(h, FX_CHECK_PDF, "librairie XML seulement");
	});
	plain.reset();
	validating.reset();
	XMLPlatformUtils::Terminate();
	return r;
}
