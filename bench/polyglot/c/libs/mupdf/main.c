/*
 * MuPDF 1.28.6 (Artifex, AGPL-3.0): the only general-purpose C PDF library
 * with a ZUGFeRD / Factur-X function of its own (source/pdf/pdf-zugferd.c):
 *
 * - Extract: pdf_zugferd_xml(), the library's function. It reads the XMP
 *   description of the Factur-X / ZUGFeRD extension schema (fx: or zf:),
 *   takes the file name it declares and loads that file among the document's
 *   associated files (/AF). No XMP declaration or no /AF: no invoice.
 * - Embed: no library function; a floor written by hand with MuPDF's
 *   primitives (pdf_add_embedded_file, /Names/EmbeddedFiles, /AF where the
 *   invoice replaces the one the host may keep, /AFRelationship by level,
 *   Factur-X XMP), full rewrite by pdf_write_document
 *   with the default options. Read back with pdf_zugferd_xml().
 * - pdf_zugferd_profile() only detects the profile from the XMP: no check.
 */
#include <mupdf/fitz.h>
#include <mupdf/pdf.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "harness.h"

static const fx_lib LIB = {"mupdf", "mupdf", FZ_VERSION};

static fz_context *G;

static void quiet(void *user, const char *message) {
	(void)user;
	(void)message;
}

static void release_buffer(void *b) { fz_drop_buffer(G, b); }

static void take_buffer(fx_bytes *out, fz_buffer *buf) {
	unsigned char *data = NULL;
	size_t len = fz_buffer_storage(G, buf, &data);
	out->data = data;
	out->len = len;
	out->owner = buf;
	out->release = release_buffer;
}

static int extract(void *ctx, const unsigned char *pdf, size_t len, fx_bytes *out, fx_err *err) {
	(void)ctx;
	fz_context *c = G;
	fz_stream *stm = NULL;
	pdf_document *doc = NULL;
	fz_buffer *buf = NULL;
	fz_var(stm);
	fz_var(doc);
	fz_var(buf);
	fz_try(c) {
		stm = fz_open_memory(c, pdf, len);
		doc = pdf_open_document_with_stream(c, stm);
		buf = pdf_zugferd_xml(c, doc);
		if (!buf)
			fz_throw(c, FZ_ERROR_ARGUMENT,
				 "pdf_zugferd_xml: no invoice (no fx:/zf: XMP description, or the declared file is not an "
				 "associated file /AF)");
	}
	fz_always(c) {
		pdf_drop_document(c, doc);
		fz_drop_stream(c, stm);
	}
	fz_catch(c) {
		fz_drop_buffer(c, buf);
		return fx_fail(err, "%s", fz_caught_message(c));
	}
	take_buffer(out, buf);
	return 0;
}

/* Inserts (key, value) into a flat name-tree /Names array, keeping the keys
 * sorted; an entry under the same key is replaced. */
static void name_tree_insert(fz_context *c, pdf_document *doc, pdf_obj *arr, const char *key, pdf_obj *value) {
	int n = pdf_array_len(c, arr), at = n;
	for (int i = 0; i + 1 < n; i += 2) {
		int cmp = strcmp(pdf_to_text_string(c, pdf_array_get(c, arr, i)), key);
		if (cmp == 0) {
			pdf_array_put(c, arr, i + 1, value);
			return;
		}
		if (cmp > 0) {
			at = i;
			break;
		}
	}
	pdf_array_insert_drop(c, arr, pdf_new_text_string(c, key), at);
	pdf_array_insert(c, arr, value, at + 1);
	(void)doc;
}

static int embed(void *ctx, const unsigned char *host, size_t host_len, const unsigned char *xml, size_t xml_len,
		 fx_bytes *out, fx_err *err) {
	(void)ctx;
	fz_context *c = G;
	fz_stream *stm = NULL;
	pdf_document *doc = NULL;
	fz_buffer *xb = NULL, *xmp = NULL, *res = NULL;
	fz_output *o = NULL;
	pdf_obj *fs = NULL, *md = NULL, *meta = NULL;
	char packet[8192];
	const char *level = fx_facturx_level(xml, xml_len);
	size_t plen = fx_facturx_xmp(packet, sizeof packet, level);
	fz_var(stm);
	fz_var(doc);
	fz_var(xb);
	fz_var(xmp);
	fz_var(res);
	fz_var(o);
	fz_var(fs);
	fz_var(md);
	fz_var(meta);
	fz_try(c) {
		stm = fz_open_memory(c, host, host_len);
		doc = pdf_open_document_with_stream(c, stm);
		pdf_obj *root = pdf_dict_get(c, pdf_trailer(c, doc), PDF_NAME(Root));
		if (!pdf_is_dict(c, root))
			fz_throw(c, FZ_ERROR_FORMAT, "no /Root");

		xb = fz_new_buffer_from_copied_data(c, xml, xml_len);
		fs = pdf_add_embedded_file(c, doc, FX_ATTACHMENT, "text/xml", xb, FX_MOD_TIME, FX_MOD_TIME, 0);
		pdf_dict_put_name(c, fs, PDF_NAME(AFRelationship), fx_facturx_relationship(level));
		pdf_dict_put_text_string(c, fs, PDF_NAME(Desc), "Factur-X invoice");

		pdf_obj *names = pdf_dict_get(c, root, PDF_NAME(Names));
		if (!pdf_is_dict(c, names))
			names = pdf_dict_put_dict(c, root, PDF_NAME(Names), 1);
		pdf_obj *tree = pdf_dict_get(c, names, PDF_NAME(EmbeddedFiles));
		if (!pdf_is_dict(c, tree))
			tree = pdf_dict_put_dict(c, names, PDF_NAME(EmbeddedFiles), 1);
		if (pdf_dict_get(c, tree, PDF_NAME(Kids)))
			fz_throw(c, FZ_ERROR_UNSUPPORTED, "embedded-file name tree with /Kids not handled by this floor");
		pdf_obj *arr = pdf_dict_get(c, tree, PDF_NAME(Names));
		if (!pdf_is_array(c, arr))
			arr = pdf_dict_put_array(c, tree, PDF_NAME(Names), 2);
		name_tree_insert(c, doc, arr, FX_ATTACHMENT, fs);

		/* /AF: an earlier invoice file (such as the one the host keeps from
		 * its original Factur-X) is replaced, so that the PDF has one. */
		pdf_obj *af = pdf_dict_get(c, root, PDF_NAME(AF));
		if (!pdf_is_array(c, af))
			af = pdf_dict_put_array(c, root, PDF_NAME(AF), 1);
		for (int i = pdf_array_len(c, af) - 1; i >= 0; i--) {
			pdf_obj *o = pdf_array_get(c, af, i);
			const char *n = pdf_dict_get_text_string(c, o, PDF_NAME(UF));
			if (!*n)
				n = pdf_dict_get_text_string(c, o, PDF_NAME(F));
			if (fx_is_invoice_name(n))
				pdf_array_delete(c, af, i);
		}
		pdf_array_push(c, af, fs);

		xmp = fz_new_buffer_from_copied_data(c, (const unsigned char *)packet, plen);
		md = pdf_new_dict(c, doc, 2);
		pdf_dict_put(c, md, PDF_NAME(Type), PDF_NAME(Metadata));
		pdf_dict_put(c, md, PDF_NAME(Subtype), PDF_NAME(XML));
		meta = pdf_add_stream(c, doc, xmp, md, 0);
		pdf_dict_put(c, root, PDF_NAME(Metadata), meta);

		res = fz_new_buffer(c, host_len + xml_len + 16384);
		o = fz_new_output_with_buffer(c, res);
		pdf_write_document(c, doc, o, &pdf_default_write_options);
		fz_close_output(c, o);
	}
	fz_always(c) {
		fz_drop_output(c, o);
		pdf_drop_obj(c, meta);
		pdf_drop_obj(c, md);
		pdf_drop_obj(c, fs);
		fz_drop_buffer(c, xmp);
		fz_drop_buffer(c, xb);
		pdf_drop_document(c, doc);
		fz_drop_stream(c, stm);
	}
	fz_catch(c) {
		fz_drop_buffer(c, res);
		return fx_fail(err, "%s", fz_caught_message(c));
	}
	take_buffer(out, res);
	return 0;
}

static void body(fx_harness *h, void *ctx) {
	(void)ctx;
	fx_extract(h,
		   "pdf_zugferd_xml (fonction ZUGFeRD de MuPDF) : nom du fichier lu dans le XMP fx:/zf:, pièce jointe "
		   "cherchée parmi les fichiers associés /AF",
		   extract, NULL);
	fx_unsupported(h, FX_PARSE, "pas de lecture du XML de la facture (MuPDF lit seulement le XMP du PDF)");
	fx_unsupported(h, FX_VALIDATE, "pas de validation");
	fx_unsupported(h, FX_WRITE, "pas de génération de XML");
	fx_embed(h,
		 "plancher : injection écrite à la main avec les primitives de MuPDF (pdf_add_embedded_file, "
		 "/Names/EmbeddedFiles, /AF où la facture remplace celle de l'hôte, /AFRelationship Alternative "
		 "ou Data selon le profil, XMP Factur-X remplacé), réécriture complète par pdf_write_document "
		 "(options par défaut), sans conversion PDF/A ; relu par pdf_zugferd_xml",
		 embed, NULL, extract, NULL);
	fx_unsupported(h, FX_CHECK_PDF,
		       "pas de contrôle : pdf_zugferd_profile ne fait que lire le profil déclaré dans le XMP");
}

int main(int argc, char **argv) {
	G = fz_new_context(NULL, NULL, FZ_STORE_DEFAULT);
	if (!G) {
		fprintf(stderr, "fz_new_context failed\n");
		return 1;
	}
	fz_set_warning_callback(G, quiet, NULL);
	fz_set_error_callback(G, quiet, NULL);
	int r = fx_run("c", LIB, argc, argv, body, NULL);
	fz_drop_context(G);
	return r;
}
