// Floor: PoDoFo 1.1.3 (LGPL-2.0+ / MPL-2.0), a general-purpose C++ PDF
// library, with no knowledge of Factur-X:
//
// - Extract: PdfMemDocument::LoadFromBuffer, the PdfEmbeddedFiles name tree
//   (ToDictionary walks /Kids), first file under a standard invoice name,
//   PdfFileSpec::GetEmbeddedData.
// - Embed: PdfDocument::CreateFileSpec + SetEmbeddedData, added to the
//   PdfEmbeddedFiles tree, plus by hand /AF (where the invoice replaces the
//   one the host may keep), /AFRelationship (by level) and a Factur-X XMP
//   packet replacing /Metadata; PdfMemDocument::Save (full
//   rewrite) with NoMetadataUpdate, so that PoDoFo keeps that XMP packet and
//   does not stamp the current time. No PDF/A conversion.
#include <podofo/podofo.h>

#include <stdexcept>
#include <string>

#include "harness.hpp"

using namespace PoDoFo;

namespace {

std::string extract(fx::Bytes pdf) {
	PdfMemDocument doc;
	doc.LoadFromBuffer(bufferview(pdf.data(), pdf.size()));
	auto *names = doc.GetNames();
	if (!names)
		throw std::runtime_error("no /Names");
	auto *tree = names->GetTree<PdfEmbeddedFiles>();
	if (!tree)
		throw std::runtime_error("no /Names/EmbeddedFiles");
	PdfEmbeddedFiles::Map files;
	tree->ToDictionary(files);
	for (auto const &[key, fs] : files) {
		std::string name(key.GetString());
		if (!fx_is_invoice_name(name.c_str()) || !fs)
			continue;
		auto data = fs->GetEmbeddedData();
		if (!data.has_value())
			throw std::runtime_error("no embedded data for " + name);
		return std::string(data->data(), data->size());
	}
	throw std::runtime_error("no attachment under a standard invoice name");
}

std::string embed(fx::Bytes host, fx::Bytes xml) {
	PdfMemDocument doc;
	doc.LoadFromBuffer(bufferview(host.data(), host.size()));

	std::shared_ptr<PdfFileSpec> fs(doc.CreateFileSpec());
	fs->SetFilename(PdfString(FX_ATTACHMENT));
	fs->SetEmbeddedData(charbuff(std::string(xml)));
	PdfDictionary &fsd = fs->GetDictionary();
	const char *level = fx_facturx_level(reinterpret_cast<const unsigned char *>(xml.data()), xml.size());
	fsd.AddKey(PdfName("AFRelationship"), PdfName(fx_facturx_relationship(level)));
	fsd.AddKey(PdfName("Desc"), PdfString("Factur-X invoice"));
	// /Subtype and /Params /ModDate of the embedded file stream.
	if (auto *ef = fsd.FindKeyAs<PdfDictionary *>("EF")) {
		for (const char *k : {"F", "UF"}) {
			PdfObject *s = ef->FindKey(k);
			if (s && s->IsDictionary()) {
				s->GetDictionary().AddKey(PdfName("Subtype"), PdfName("text/xml"));
				PdfDictionary params;
				params.AddKey(PdfName("ModDate"), PdfString(FX_MOD_DATE));
				s->GetDictionary().AddKey(PdfName("Params"), params);
			}
		}
	}
	PdfObject &fso = fs->GetObject();
	doc.GetOrCreateNames().GetOrCreateTree<PdfEmbeddedFiles>().AddValue(PdfString(FX_ATTACHMENT), fs);

	// /AF: an earlier invoice file (such as the one the host keeps from its
	// original Factur-X) is replaced, so that the PDF has one.
	PdfDictionary &cat = doc.GetCatalog().GetDictionary();
	PdfArray keep;
	if (PdfArray *af = cat.FindKeyAs<PdfArray *>("AF")) {
		for (unsigned i = 0; i < af->GetSize(); i++) {
			const PdfObject *o = af->FindAt(i);
			const PdfDictionary *od = o && o->IsDictionary() ? &o->GetDictionary() : nullptr;
			const PdfObject *n = od ? od->FindKey("UF") : nullptr;
			if (!n || !n->IsString())
				n = od ? od->FindKey("F") : nullptr;
			if (n && n->IsString() && fx_is_invoice_name(std::string(n->GetString().GetString()).c_str()))
				continue;
			keep.Add((*af)[i]); // the reference itself, not the object
		}
	}
	keep.Add(fso.GetIndirectReference());
	cat.AddKey(PdfName("AF"), keep);

	char packet[8192];
	size_t n = fx_facturx_xmp(packet, sizeof packet, level);
	PdfObject &meta = doc.GetObjects().CreateDictionaryObject(PdfName("Metadata"));
	meta.GetDictionary().AddKey(PdfName("Subtype"), PdfName("XML"));
	meta.GetOrCreateStream().SetData(bufferview(packet, n), true);
	cat.AddKeyIndirect(PdfName("Metadata"), meta);

	std::string out;
	out.reserve(host.size() + xml.size() + 16384);
	StringStreamDevice dev(out);
	doc.Save(dev, PdfSaveOptions::NoMetadataUpdate);
	return out;
}

} // namespace

int main(int argc, char **argv) {
	PdfCommon::SetMaxLoggingSeverity(PdfLogSeverity::None);
	fx_lib lib{"podofo", "podofo", PODOFO_VERSION_STRING};
	return fx::run(lib, argc, argv, [](fx_harness *h) {
		auto ex = [](fx::Bytes p) { return extract(p); };
		auto em = [](fx::Bytes host, fx::Bytes xml) { return embed(host, xml); };
		fx::extract(h,
			    "plancher : PdfMemDocument::LoadFromBuffer, arbre PdfEmbeddedFiles (ToDictionary), premier "
			    "fichier sous un nom de facture standard, GetEmbeddedData ; pas une librairie Factur-X",
			    ex);
		fx_unsupported(h, FX_PARSE, "librairie PDF seulement");
		fx_unsupported(h, FX_VALIDATE, "librairie PDF seulement");
		fx_unsupported(h, FX_WRITE, "librairie PDF seulement");
		fx::embed(h,
			  "plancher : CreateFileSpec + SetEmbeddedData dans l'arbre PdfEmbeddedFiles ; /AF (la "
			  "facture remplace celle de l'hôte), /AFRelationship Alternative ou Data selon le profil et "
			  "XMP Factur-X écrits à la main (XMP de l'hôte remplacé) ; PdfMemDocument::Save (réécriture "
			  "complète, NoMetadataUpdate), sans conversion PDF/A",
			  em, ex);
		fx_unsupported(h, FX_CHECK_PDF, "librairie PDF seulement");
	});
}
