// Floor: poppler-cpp 26.11.0 (GPL-2.0+), the C++ wrapper of the Poppler
// rendering library, with no knowledge of Factur-X. Extract only:
// document::load_from_raw_data, document::embedded_files() (the
// /Names/EmbeddedFiles tree), first file under a standard invoice name,
// embedded_file::data(). Poppler-cpp has no writing API: no Embed.
#include <poppler-document.h>
#include <poppler-embedded-file.h>
#include <poppler-global.h>
#include <poppler-version.h>

#include <memory>
#include <stdexcept>
#include <string>
#include <vector>

#include "harness.hpp"

namespace {

void quiet(const std::string &, void *) {}

std::vector<char> extract(fx::Bytes pdf) {
	std::unique_ptr<poppler::document> doc(poppler::document::load_from_raw_data(pdf.data(), (int)pdf.size()));
	if (!doc)
		throw std::runtime_error("load_from_raw_data: cannot open the PDF");
	if (!doc->has_embedded_files())
		throw std::runtime_error("no embedded files");
	for (poppler::embedded_file *f : doc->embedded_files()) {
		if (!f || !f->is_valid())
			continue;
		std::vector<char> n = f->unicodeName().to_utf8();
		std::string name(n.begin(), n.end());
		if (!fx_is_invoice_name(name.c_str()))
			continue;
		return f->data();
	}
	throw std::runtime_error("no attachment under a standard invoice name");
}

} // namespace

int main(int argc, char **argv) {
	poppler::set_debug_error_function(quiet, nullptr);
	static std::string version = poppler::version_string();
	fx_lib lib{"poppler-cpp", "poppler-cpp", version.c_str()};
	return fx::run(lib, argc, argv, [](fx_harness *h) {
		auto ex = [](fx::Bytes p) { return extract(p); };
		fx::extract(h,
			    "plancher : document::load_from_raw_data, embedded_files(), premier fichier sous un nom de "
			    "facture standard, embedded_file::data() ; pas une librairie Factur-X",
			    ex);
		fx_unsupported(h, FX_PARSE, "librairie PDF seulement");
		fx_unsupported(h, FX_VALIDATE, "librairie PDF seulement");
		fx_unsupported(h, FX_WRITE, "librairie PDF seulement");
		fx_unsupported(h, FX_EMBED, "poppler-cpp n'a pas d'API d'écriture");
		fx_unsupported(h, FX_CHECK_PDF, "librairie PDF seulement");
	});
}
