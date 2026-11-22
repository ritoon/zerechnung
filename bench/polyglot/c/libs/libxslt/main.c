/*
 * libxslt 1.1.45, the only XSLT processor of the C world, tried on the
 * official validation stylesheets: CEN EN 16931 CII 1.3.17
 * (EN16931-CII-validation.xslt) and KoSIT XRechnung schematron 2.6.0
 * (XRechnung-CII-validation.xsl). Both are XSLT 2.0 (XPath 2.0 functions,
 * typed casts such as xs:decimal(...)), and libxslt implements XSLT 1.0 only:
 * the compilation fails, which is checked here at run time and reported as
 * "unsupported" with libxslt's own messages. Nothing is measured.
 */
#include <libxml/parser.h>
#include <libxml/xmlversion.h>
#include <libxslt/transform.h>
#include <libxslt/xsltInternals.h>
#include <libxslt/xsltconfig.h>
#include <libxslt/xsltutils.h>
#include <stdarg.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "harness.h"

static const fx_lib LIB = {"libxslt", "libxslt", LIBXSLT_DOTTED_VERSION};

#define CEN_REL "cen-1.3.17/xslt/EN16931-CII-validation.xslt"
#define XR_REL "xrechnung-2.6.0/schematron/cii/XRechnung-CII-validation.xsl"

typedef struct {
	int errors;
	char first[512];
	char version[256];
} compile_log;

static compile_log *current;

/* libxslt reports through a printf-like callback, often in pieces: count the
 * "compilation error" lines and keep the first XPath expression it rejects. */
static void on_error(void *ctx, const char *fmt, ...) {
	(void)ctx;
	if (!current)
		return;
	char buf[1024];
	va_list ap;
	va_start(ap, fmt);
	vsnprintf(buf, sizeof buf, fmt, ap);
	va_end(ap);
	if (strstr(buf, "compilation error"))
		current->errors++;
	if (!current->version[0] && strstr(buf, "only 1.1 features"))
		snprintf(current->version, sizeof current->version, "%s", buf);
	if (!current->first[0] && strstr(buf, "could not compile")) {
		snprintf(current->first, sizeof current->first, "%s", buf);
		size_t k = strlen(current->first);
		while (k && current->first[k - 1] == '\n')
			current->first[--k] = 0;
	}
}

/* Tries to compile `rel`; returns 1 when libxslt accepted it. */
static int try_compile(const char *rel, compile_log *log) {
	char path[4096];
	snprintf(path, sizeof path, "%s/%s", fx_artefacts(), rel);
	FILE *f = fopen(path, "rb");
	if (!f) {
		snprintf(log->first, sizeof log->first, "%s missing (run ./run.sh setup)", path);
		log->errors = -1;
		return 0;
	}
	fclose(f);
	current = log;
	xsltSetGenericErrorFunc(NULL, on_error);
	xmlSetGenericErrorFunc(NULL, on_error);
	xsltStylesheetPtr s = xsltParseStylesheetFile((const xmlChar *)path);
	current = NULL;
	if (s) {
		xsltFreeStylesheet(s);
		return 1;
	}
	return 0;
}

static void body(fx_harness *h, void *ctx) {
	(void)ctx;
	compile_log cen = {0}, xr = {0};
	int cen_ok = try_compile(CEN_REL, &cen);
	int xr_ok = try_compile(XR_REL, &xr);
	char note[2048];
	if (cen.errors < 0 || xr.errors < 0) {
		snprintf(note, sizeof note, "artefacts absents : %s%s", cen.errors < 0 ? cen.first : "",
			 xr.errors < 0 ? xr.first : "");
	} else if (!cen_ok || !xr_ok) {
		snprintf(note, sizeof note,
			 "libxslt %s ne fait que XSLT 1.0 : la feuille officielle CEN EN 16931 CII 1.3.17 (XSLT 2.0) "
			 "ne compile pas (%d erreurs de compilation, dont « %s »), ni la feuille KoSIT XRechnung "
			 "2.6.0 (%d erreurs) ; vérifié à l'exécution par xsltParseStylesheetFile",
			 LIBXSLT_DOTTED_VERSION, cen.errors, cen.first, xr.errors);
	} else {
		snprintf(note, sizeof note, "inattendu : libxslt a compilé les feuilles XSLT 2.0");
	}
	if (!fx_is_survey(h))
		fprintf(stderr, "   compile CEN: %s (%d errors), KoSIT: %s (%d errors)\n", cen_ok ? "ok" : "refused",
			cen.errors, xr_ok ? "ok" : "refused", xr.errors);
	fx_unsupported(h, FX_EXTRACT, "moteur XSLT seulement");
	fx_unsupported(h, FX_PARSE, "moteur XSLT seulement");
	fx_unsupported(h, FX_VALIDATE, note);
	fx_unsupported(h, FX_WRITE, "moteur XSLT seulement");
	fx_unsupported(h, FX_EMBED, "moteur XSLT seulement");
	fx_unsupported(h, FX_CHECK_PDF, "moteur XSLT seulement");
}

int main(int argc, char **argv) {
	xmlInitParser();
	return fx_run("c", LIB, argc, argv, body, NULL);
}
