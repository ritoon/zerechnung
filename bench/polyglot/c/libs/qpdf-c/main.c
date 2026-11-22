/*
 * Floor: qpdf 12.4.2 through its C API (qpdf-c.h, object handles qpdf_oh_*),
 * with the invoice extraction written by hand here, as the lopdf floor of the
 * Rust bench: read the document from memory, walk /Root/Names/EmbeddedFiles
 * (with /Kids), take the first file under a standard invoice name, decode its
 * stream. Not a Factur-X library.
 */
#include <qpdf/qpdf-c.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "harness.h"


/* Looks for the invoice in a name-tree node; returns its file specification
 * handle, or 0. */
static qpdf_oh find_in_tree(qpdf_data q, qpdf_oh node, int depth) {
	if (depth > 32 || !qpdf_oh_is_dictionary(q, node))
		return 0;
	qpdf_oh names = qpdf_oh_get_key_if_dict(q, node, "/Names");
	if (qpdf_oh_is_array(q, names)) {
		int n = qpdf_oh_get_array_n_items(q, names);
		for (int i = 0; i + 1 < n; i += 2) {
			qpdf_oh k = qpdf_oh_get_array_item(q, names, i);
			if (qpdf_oh_is_string(q, k) && fx_is_invoice_name(qpdf_oh_get_utf8_value(q, k)))
				return qpdf_oh_get_array_item(q, names, i + 1);
		}
	}
	qpdf_oh kids = qpdf_oh_get_key_if_dict(q, node, "/Kids");
	if (qpdf_oh_is_array(q, kids)) {
		int n = qpdf_oh_get_array_n_items(q, kids);
		for (int i = 0; i < n; i++) {
			qpdf_oh fs = find_in_tree(q, qpdf_oh_get_array_item(q, kids, i), depth + 1);
			if (fs)
				return fs;
		}
	}
	return 0;
}

static int error_of(qpdf_data q, fx_err *err, const char *what) {
	if (qpdf_has_error(q))
		return fx_fail(err, "%s: %s", what, qpdf_get_error_full_text(q, qpdf_get_error(q)));
	return fx_fail(err, "%s", what);
}

static int extract(void *ctx, const unsigned char *pdf, size_t len, fx_bytes *out, fx_err *err) {
	(void)ctx;
	qpdf_data q = qpdf_init();
	qpdf_silence_errors(q);
	qpdf_set_suppress_warnings(q, QPDF_TRUE);
	int rc = -1;
	if (qpdf_read_memory(q, "invoice.pdf", (const char *)pdf, len, NULL) & QPDF_ERRORS) {
		rc = error_of(q, err, "qpdf_read_memory");
		goto done;
	}
	qpdf_oh root = qpdf_get_root(q);
	qpdf_oh names = qpdf_oh_get_key_if_dict(q, root, "/Names");
	qpdf_oh tree = qpdf_oh_get_key_if_dict(q, names, "/EmbeddedFiles");
	if (!qpdf_oh_is_dictionary(q, tree)) {
		rc = fx_fail(err, "no /Names/EmbeddedFiles");
		goto done;
	}
	qpdf_oh fs = find_in_tree(q, tree, 0);
	if (!fs || !qpdf_oh_is_dictionary(q, fs)) {
		rc = fx_fail(err, "no attachment under a standard invoice name");
		goto done;
	}
	qpdf_oh ef = qpdf_oh_get_key_if_dict(q, fs, "/EF");
	qpdf_oh stream = qpdf_oh_get_key_if_dict(q, ef, "/UF");
	if (!qpdf_oh_is_stream(q, stream))
		stream = qpdf_oh_get_key_if_dict(q, ef, "/F");
	if (!qpdf_oh_is_stream(q, stream)) {
		rc = fx_fail(err, "no /EF stream");
		goto done;
	}
	unsigned char *buf = NULL;
	size_t n = 0;
	QPDF_BOOL filtered = QPDF_FALSE;
	if (qpdf_oh_get_stream_data(q, stream, qpdf_dl_generalized, &filtered, &buf, &n) & QPDF_ERRORS) {
		rc = error_of(q, err, "qpdf_oh_get_stream_data");
		goto done;
	}
	if (!filtered) {
		qpdf_oh_free_buffer(&buf);
		rc = fx_fail(err, "stream filter not supported by qpdf");
		goto done;
	}
	fx_bytes_malloced(out, buf, n);
	rc = 0;
done:
	qpdf_cleanup(&q);
	return rc;
}

static void body(fx_harness *h, void *ctx) {
	(void)ctx;
	fx_extract(h,
		   "plancher : extraction écrite à la main sur l'API C de qpdf (qpdf_read_memory, arbre "
		   "/Names/EmbeddedFiles avec /Kids, qpdf_oh_get_stream_data) ; pas une librairie Factur-X",
		   extract, NULL);
	fx_unsupported(h, FX_PARSE, "plancher d'extraction seulement");
	fx_unsupported(h, FX_VALIDATE, "plancher d'extraction seulement");
	fx_unsupported(h, FX_WRITE, "plancher d'extraction seulement");
	fx_unsupported(h, FX_EMBED, "plancher d'extraction seulement (injection mesurée avec l'API C++ de qpdf, banc cpp)");
	fx_unsupported(h, FX_CHECK_PDF, "plancher d'extraction seulement");
}

int main(int argc, char **argv) {
	fx_lib lib = {"qpdf-c", "qpdf", qpdf_get_qpdf_version()};
	return fx_run("c", lib, argc, argv, body, NULL);
}
