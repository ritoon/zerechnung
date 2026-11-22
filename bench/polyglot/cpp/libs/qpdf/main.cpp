// Floor: qpdf 12.4.2 (Apache-2.0), the content-preserving PDF library of the
// C++ world, with no knowledge of Factur-X:
//
// - Extract: QPDFEmbeddedFileDocumentHelper::getEmbeddedFiles() (the whole
//   /Names/EmbeddedFiles tree), first file under a standard invoice name,
//   stream decoded with qpdf_dl_generalized.
// - Embed: QPDFEFStreamObjectHelper / QPDFFileSpecObjectHelper /
//   replaceEmbeddedFile, plus by hand /AF (where the invoice replaces the
//   one the host may keep), /AFRelationship (by level) and a Factur-X XMP
//   packet replacing /Metadata; written by QPDFWriter (always a full
//   rewrite) with a newline before endstream as PDF/A wants. No PDF/A
//   conversion.
#include <qpdf/Buffer.hh>
#include <qpdf/DLL.h>
#include <qpdf/QPDF.hh>
#include <qpdf/QPDFEFStreamObjectHelper.hh>
#include <qpdf/QPDFEmbeddedFileDocumentHelper.hh>
#include <qpdf/QPDFFileSpecObjectHelper.hh>
#include <qpdf/QPDFObjectHandle.hh>
#include <qpdf/QPDFWriter.hh>

#include <memory>
#include <stdexcept>
#include <string>

#include "harness.hpp"

namespace {

// Bytes of a qpdf Buffer, handed to the harness without a copy.
struct BufBytes {
	std::shared_ptr<Buffer> b;
	const char *data() const { return reinterpret_cast<const char *>(b->getBuffer()); }
	size_t size() const { return b->getSize(); }
};

BufBytes extract(fx::Bytes pdf) {
	QPDF q;
	q.setSuppressWarnings(true);
	q.processMemoryFile("invoice.pdf", pdf.data(), pdf.size());
	QPDFEmbeddedFileDocumentHelper efdh(q);
	if (!efdh.hasEmbeddedFiles())
		throw std::runtime_error("no /Names/EmbeddedFiles");
	for (auto const &[name, fs] : efdh.getEmbeddedFiles()) {
		if (!fx_is_invoice_name(name.c_str()))
			continue;
		QPDFObjectHandle s = fs->getEmbeddedFileStream();
		if (!s.isStream())
			throw std::runtime_error("no /EF stream for " + name);
		return BufBytes{s.getStreamData(qpdf_dl_generalized)};
	}
	throw std::runtime_error("no attachment under a standard invoice name");
}

BufBytes embed(fx::Bytes host, fx::Bytes xml) {
	QPDF q;
	q.setSuppressWarnings(true);
	q.processMemoryFile("host.pdf", host.data(), host.size());
	QPDFEmbeddedFileDocumentHelper efdh(q);

	auto efs = QPDFEFStreamObjectHelper::createEFStream(q, std::string(xml));
	efs.setSubtype("text/xml").setCreationDate(FX_MOD_DATE).setModDate(FX_MOD_DATE);
	auto fs = QPDFFileSpecObjectHelper::createFileSpec(q, FX_ATTACHMENT, efs);
	fs.setDescription("Factur-X invoice");
	QPDFObjectHandle fso = fs.getObjectHandle();
	if (!fso.isIndirect())
		fso = q.makeIndirectObject(fso);
	const char *level = fx_facturx_level(reinterpret_cast<const unsigned char *>(xml.data()), xml.size());
	fso.replaceKey("/AFRelationship", QPDFObjectHandle::newName(std::string("/") + fx_facturx_relationship(level)));
	efdh.replaceEmbeddedFile(FX_ATTACHMENT, QPDFFileSpecObjectHelper(fso));

	// /AF: an earlier invoice file (such as the one the host keeps from its
	// original Factur-X) is replaced, so that the PDF has one.
	QPDFObjectHandle root = q.getRoot();
	QPDFObjectHandle af = root.getKey("/AF");
	QPDFObjectHandle keep = QPDFObjectHandle::newArray();
	if (af.isArray())
		for (auto &o : af.aitems()) {
			QPDFObjectHandle n = o.getKeyIfDict("/UF");
			if (!n.isString())
				n = o.getKeyIfDict("/F");
			if (!n.isString() || !fx_is_invoice_name(n.getUTF8Value().c_str()))
				keep.appendItem(o);
		}
	keep.appendItem(fso);
	root.replaceKey("/AF", keep);

	char packet[8192];
	size_t n = fx_facturx_xmp(packet, sizeof packet, level);
	QPDFObjectHandle meta = q.newStream(std::string(packet, n));
	meta.getDict().replaceKey("/Type", QPDFObjectHandle::newName("/Metadata"));
	meta.getDict().replaceKey("/Subtype", QPDFObjectHandle::newName("/XML"));
	root.replaceKey("/Metadata", meta);

	QPDFWriter w(q);
	w.setOutputMemory();
	w.setNewlineBeforeEndstream(true); // as PDF/A requires (ISO 19005-1 6.1.7)
	w.write();
	return BufBytes{w.getBufferSharedPointer()};
}

} // namespace

int main(int argc, char **argv) {
	fx_lib lib{"qpdf", "qpdf", QPDF_VERSION};
	return fx::run(lib, argc, argv, [](fx_harness *h) {
		auto ex = [](fx::Bytes p) { return extract(p); };
		auto em = [](fx::Bytes host, fx::Bytes xml) { return embed(host, xml); };
		fx::extract(h,
			    "plancher : QPDFEmbeddedFileDocumentHelper::getEmbeddedFiles, premier fichier sous un nom "
			    "de facture standard, flux décodé ; pas une librairie Factur-X",
			    ex);
		fx_unsupported(h, FX_PARSE, "librairie PDF seulement");
		fx_unsupported(h, FX_VALIDATE, "librairie PDF seulement");
		fx_unsupported(h, FX_WRITE, "librairie PDF seulement");
		fx::embed(h,
			  "plancher : QPDFEFStreamObjectHelper + QPDFFileSpecObjectHelper + replaceEmbeddedFile ; "
			  "/AF (la facture remplace celle de l'hôte), /AFRelationship Alternative ou Data selon le "
			  "profil et XMP Factur-X écrits à la main (XMP de l'hôte remplacé) ; réécriture complète par "
			  "QPDFWriter (fin de ligne avant endstream), sans conversion PDF/A",
			  em, ex);
		fx_unsupported(h, FX_CHECK_PDF, "librairie PDF seulement");
	});
}
