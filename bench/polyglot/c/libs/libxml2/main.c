/*
 * Floor: libxml2 2.15.4, the XML library of the C world (and of lxml), with no
 * knowledge of Factur-X. Parse = DOM (xmlCtxtReadMemory), Write = DOM
 * serialization (xmlDocDumpMemory), ParseValidate = DOM + XML Schema
 * validation against the UN/CEFACT CII D16B schema (xmlSchemaValidateDoc).
 * The schema only: no EN 16931 business rule.
 */
#include <libxml/parser.h>
#include <libxml/tree.h>
#include <libxml/xmlerror.h>
#include <libxml/xmlschemas.h>
#include <libxml/xmlversion.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "harness.h"

static const fx_lib LIB = {"libxml2", "libxml2", LIBXML_DOTTED_VERSION};

/* The CII D16B schema of the KoSIT XRechnung 3.0.2 configuration
 * (2026-08-31), fetched by run.sh setup. */
#define XSD_REL "kosit-config/resources/cii/16b/xsd/CrossIndustryInvoice_100pD16B.xsd"
#define MAX_MESSAGES 20

typedef struct {
	char *msgs[MAX_MESSAGES];
	size_t n, errors;
} errlist;

static void collect(void *user, const xmlError *err) {
	errlist *l = user;
	if (!err || err->level < XML_ERR_ERROR)
		return;
	l->errors++;
	if (l->n < MAX_MESSAGES) {
		char buf[256];
		snprintf(buf, sizeof buf, "XSD:%d: %s", err->line, err->message ? err->message : "?");
		size_t k = strlen(buf);
		while (k && (buf[k - 1] == '\n' || buf[k - 1] == ' '))
			buf[--k] = 0;
		l->msgs[l->n++] = strdup(buf);
	}
}

static void errlist_free(errlist *l) {
	for (size_t i = 0; i < l->n; i++)
		free(l->msgs[i]);
	free(l);
}

/* XML -> DOM, the parser errors kept in `l` (or ignored when NULL). */
static xmlDocPtr read_doc(const unsigned char *xml, size_t len, errlist *l, fx_err *err) {
	xmlParserCtxtPtr p = xmlNewParserCtxt();
	if (!p) {
		fx_fail(err, "xmlNewParserCtxt: out of memory");
		return NULL;
	}
	errlist local = {0};
	xmlCtxtSetErrorHandler(p, collect, l ? (void *)l : (void *)&local);
	xmlDocPtr doc = xmlCtxtReadMemory(p, (const char *)xml, (int)len, "invoice.xml", NULL, XML_PARSE_NONET);
	if (!doc) {
		errlist *src = l ? l : &local;
		fx_fail(err, "xmlCtxtReadMemory: %s", src->n ? src->msgs[0] : "not well-formed");
	}
	for (size_t i = 0; i < local.n; i++)
		free(local.msgs[i]);
	xmlFreeParserCtxt(p);
	return doc;
}

static int parse(void *ctx, const unsigned char *xml, size_t len, void **model, fx_err *err) {
	(void)ctx;
	xmlDocPtr doc = read_doc(xml, len, NULL, err);
	if (!doc)
		return -1;
	*model = doc;
	return 0;
}

static void free_doc(void *ctx, void *m) {
	(void)ctx;
	xmlFreeDoc(m);
}

static void release_xml(void *p) { xmlFree(p); }

static int write_doc(void *ctx, void *model, fx_bytes *out, fx_err *err) {
	(void)ctx;
	xmlChar *mem = NULL;
	int size = 0;
	xmlDocDumpMemory(model, &mem, &size);
	if (!mem)
		return fx_fail(err, "xmlDocDumpMemory failed");
	out->data = mem;
	out->len = (size_t)size;
	out->owner = mem;
	out->release = release_xml;
	return 0;
}

/* The compiled schema, built on the first validation (its cost is in
 * first_call_ns of ParseValidate_Small), then reused. */
static xmlSchemaPtr schema;
static char schema_error[1024];

static xmlSchemaPtr compiled(fx_err *err) {
	if (schema)
		return schema;
	if (schema_error[0]) {
		fx_fail(err, "%s", schema_error);
		return NULL;
	}
	char path[4096];
	snprintf(path, sizeof path, "%s/%s", fx_artefacts(), XSD_REL);
	xmlSchemaParserCtxtPtr pc = xmlSchemaNewParserCtxt(path);
	errlist l = {0};
	if (pc) {
		xmlSchemaSetParserStructuredErrors(pc, collect, &l);
		schema = xmlSchemaParse(pc);
		xmlSchemaFreeParserCtxt(pc);
	}
	if (!schema)
		snprintf(schema_error, sizeof schema_error, "cannot compile %s (run ./run.sh setup): %s", path,
			 l.n ? l.msgs[0] : "missing file");
	for (size_t i = 0; i < l.n; i++)
		free(l.msgs[i]);
	if (!schema)
		fx_fail(err, "%s", schema_error);
	return schema;
}

static int validate(void *ctx, const unsigned char *xml, size_t len, void **report, fx_err *err) {
	(void)ctx;
	xmlSchemaPtr s = compiled(err);
	if (!s)
		return -1;
	errlist *l = calloc(1, sizeof *l);
	xmlDocPtr doc = read_doc(xml, len, l, err);
	if (!doc) {
		errlist_free(l);
		return -1;
	}
	xmlSchemaValidCtxtPtr vc = xmlSchemaNewValidCtxt(s);
	xmlSchemaSetValidStructuredErrors(vc, collect, l);
	int r = xmlSchemaValidateDoc(vc, doc);
	xmlSchemaFreeValidCtxt(vc);
	xmlFreeDoc(doc);
	if (r < 0) {
		errlist_free(l);
		return fx_fail(err, "xmlSchemaValidateDoc: internal error %d", r);
	}
	if (r > 0 && l->errors == 0)
		l->errors = 1;
	*report = l;
	return 0;
}

static void summarize(void *ctx, void *report, fx_findings *out) {
	(void)ctx;
	errlist *l = report;
	out->errors = l->errors;
	for (size_t i = 0; i < l->n; i++)
		fx_findings_add(out, l->msgs[i]);
}

static void free_report(void *ctx, void *r) {
	(void)ctx;
	errlist_free(r);
}

static void body(fx_harness *h, void *ctx) {
	(void)ctx;
	fx_unsupported(h, FX_EXTRACT, "librairie XML seulement");
	fx_parse(h, "plancher : DOM libxml2 (xmlCtxtReadMemory, XML_PARSE_NONET), pas un modèle de facture", parse,
		 free_doc, NULL);
	fx_validate(h,
		    "plancher : DOM + xmlSchemaValidateDoc, XSD UN/CEFACT CII D16B (configuration KoSIT XRechnung "
		    "3.0.2 du 2026-08-31), schéma compilé au premier appel ; schéma seulement, aucune règle "
		    "métier EN 16931",
		    validate, summarize, free_report, NULL);
	fx_write(h, "plancher : sérialisation du DOM libxml2 (xmlDocDumpMemory), pas un modèle de facture", parse,
		 free_doc, write_doc, NULL);
	fx_unsupported(h, FX_EMBED, "librairie XML seulement");
	fx_unsupported(h, FX_CHECK_PDF, "librairie XML seulement");
}

int main(int argc, char **argv) {
	xmlInitParser();
	int r = fx_run("c", LIB, argc, argv, body, NULL);
	if (schema)
		xmlSchemaFree(schema);
	return r;
}
