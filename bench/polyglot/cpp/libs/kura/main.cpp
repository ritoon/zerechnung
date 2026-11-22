// Dedicated (container level): Kura 1.4.1 (bentopdf-kura, AGPL-3.0 or
// commercial), a C++ PDF/A, PDF/X and e-invoice engine built on qpdf, linked
// statically from its pinned source (pdfa-engine/, PDFium, zxing-cpp and
// QuickJS disabled). It knows Factur-X, ZUGFeRD, XRechnung and Order-X at the
// PDF level and does not read, validate or write the CII itself:
//
// - Extract: pdfa::readInvoice (core/src/einvoice_read.cpp), the first file
//   of /Names/EmbeddedFiles under factur-x.xml, zugferd-invoice.xml,
//   ZUGFeRD-invoice.xml, xrechnung.xml or order-x.xml.
// - Embed: kura_convert(host, "3b", invoice_xml), the SDK entry point that
//   converts the host to PDF/A-3b and attaches the invoice with /AF,
//   /AFRelationship and the fx: XMP extension schema (profile and file name
//   detected from the XML by detectInvoice). The whole PDF/A conversion
//   (fonts, colour spaces, metadata) is in the timing. The protocol's hosts
//   keep in /AF the factur-x.xml of the PDF they come from: Kura lists it
//   again in /Names/EmbeddedFiles next to the new one, and readInvoice reads
//   back that first, older file (the same invoice, in CRLF).
// - CheckPDF: kura_read_invoice (pdfa::checkInvoice,
//   core/src/einvoice_check.cpp), the consistency check between the payload,
//   the attachment name, /AFRelationship, /AF and the XMP. Not a PDF/A
//   validation; a found inconsistency is part of the report, not a failure.
#include <kura/kura.h>
#include <pdfa/einvoice.hh>

#include <stdexcept>
#include <string>
#include <vector>

#include "harness.hpp"

namespace {

std::string extract(fx::Bytes pdf) {
	pdfa::InvoiceRead r = pdfa::readInvoice(reinterpret_cast<const unsigned char *>(pdf.data()), pdf.size(), "");
	if (!r.ok)
		throw std::runtime_error("readInvoice: " + r.error);
	if (r.xml.empty())
		throw std::runtime_error("readInvoice: no factur-x.xml, zugferd-invoice.xml, ZUGFeRD-invoice.xml, "
					 "xrechnung.xml or order-x.xml attachment");
	return std::move(r.xml);
}

// Converted PDF, owned by its kura_result.
struct Converted {
	kura_result *r = nullptr;
	explicit Converted(kura_result *p) : r(p) {}
	Converted(Converted &&o) noexcept : r(o.r) { o.r = nullptr; }
	Converted(const Converted &) = delete;
	~Converted() { kura_result_free(r); }
	const unsigned char *data() const { return r->pdf; }
	size_t size() const { return r->pdf_len; }
};

Converted embed(fx::Bytes host, fx::Bytes xml) {
	kura_options opt{};
	opt.invoice_xml = reinterpret_cast<const unsigned char *>(xml.data());
	opt.invoice_xml_len = xml.size();
	Converted c(kura_convert(reinterpret_cast<const unsigned char *>(host.data()), host.size(), "3b", &opt));
	if (!c.r)
		throw std::runtime_error("kura_convert returned null (out of memory)");
	if (!c.r->ok || !c.r->pdf || !c.r->pdf_len)
		throw std::runtime_error(std::string("kura_convert: ") + (c.r->error_code ? c.r->error_code : "") + " " +
					 (c.r->error ? c.r->error : "no output"));
	return c;
}

struct Check {
	bool consistent;
	std::vector<std::string> problems;
};

Check check(fx::Bytes pdf) {
	kura_invoice *inv = kura_read_invoice(reinterpret_cast<const unsigned char *>(pdf.data()), pdf.size(), nullptr);
	if (!inv)
		throw std::runtime_error("kura_read_invoice returned null (out of memory)");
	Check c{inv->consistent != 0, {}};
	std::string err;
	if (!inv->ok)
		err = std::string("kura_read_invoice: ") + (inv->error_code ? inv->error_code : "") + " " +
		      (inv->error ? inv->error : "unreadable PDF");
	else if (!inv->einvoice)
		err = std::string("kura_read_invoice: ") + (inv->error ? inv->error : "no e-invoice payload");
	for (size_t i = 0; err.empty() && i < inv->problem_count; i++)
		c.problems.emplace_back(inv->problems[i]);
	kura_invoice_free(inv);
	if (!err.empty())
		throw std::runtime_error(err);
	return c;
}

} // namespace

int main(int argc, char **argv) {
	fx_lib lib{"kura", "bentopdf-kura", KURA_VERSION};
	return fx::run(lib, argc, argv, [](fx_harness *h) {
		auto ex = [](fx::Bytes p) { return extract(p); };
		auto em = [](fx::Bytes host, fx::Bytes xml) { return embed(host, xml); };
		auto ch = [](fx::Bytes p) { return check(p); };
		fx::extract(h,
			    "pdfa::readInvoice : premier fichier de /Names/EmbeddedFiles sous un nom Factur-X, "
			    "ZUGFeRD, XRechnung ou Order-X",
			    ex);
		fx_unsupported(h, FX_PARSE, "Kura ne lit pas le CII (détection du profil par motifs seulement)");
		fx_unsupported(h, FX_VALIDATE, "pas de validation XSD ni EN 16931 du CII");
		fx_unsupported(h, FX_WRITE, "Kura n'écrit pas de CII");
		fx::embed(h,
			  "kura_convert niveau 3b avec invoice_xml : conversion complète de l'hôte en PDF/A-3b, pièce "
			  "jointe /AF /AFRelationship et schéma d'extension XMP fx: ; relu par readInvoice ; le "
			  "factur-x.xml que l'hôte garde dans /AF reste à côté du nouveau (deux pièces jointes)",
			  em, ex);
		fx::check_pdf(h,
			      "kura_read_invoice (checkInvoice) : cohérence charge utile, nom de la pièce jointe, "
			      "/AFRelationship, /AF et XMP ; pas une validation PDF/A",
			      ch);
	});
}
