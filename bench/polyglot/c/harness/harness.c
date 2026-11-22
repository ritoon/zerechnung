/*
 * Shared harness of the C and C++ benchmarks. See harness.h.
 */
#define _POSIX_C_SOURCE 200809L
#define _DARWIN_C_SOURCE

#include "harness.h"

#include <ctype.h>
#include <dirent.h>
#include <errno.h>
#include <fcntl.h>
#include <pthread.h>
#include <signal.h>
#include <stdarg.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <strings.h>
#include <sys/resource.h>
#include <sys/stat.h>
#include <sys/time.h>
#include <time.h>
#include <unistd.h>

#ifndef FX_DEFAULT_ECOSYSTEM
#define FX_DEFAULT_ECOSYSTEM "../../ecosystem"
#endif
#ifndef FX_DEFAULT_ARTEFACTS
#define FX_DEFAULT_ARTEFACTS ".artefacts"
#endif

/* Stack of the worker thread every operation runs on, as in the Rust harness
 * (official validation stylesheets recurse deeply). */
#define STACK_BYTES ((size_t)512 * 1024 * 1024)

/* Sink of every result length, so that no call can be optimized away. */
volatile size_t fx_sink;

/* ---------------------------------------------------------------- bytes, errors */

void fx_bytes_malloced(fx_bytes *b, unsigned char *p, size_t len) {
	b->data = p;
	b->len = len;
	b->owner = p;
	b->release = NULL;
}

int fx_bytes_copy(fx_bytes *b, const void *p, size_t len) {
	unsigned char *q = malloc(len ? len : 1);
	if (!q)
		return -1;
	if (len)
		memcpy(q, p, len);
	fx_bytes_malloced(b, q, len);
	return 0;
}

void fx_bytes_release(fx_bytes *b) {
	if (b->release)
		b->release(b->owner);
	else
		free(b->owner);
	memset(b, 0, sizeof *b);
}

int fx_fail(fx_err *e, const char *fmt, ...) {
	va_list ap;
	va_start(ap, fmt);
	vsnprintf(e->msg, sizeof e->msg, fmt, ap);
	va_end(ap);
	if (!e->msg[0])
		snprintf(e->msg, sizeof e->msg, "error");
	return -1;
}

void fx_findings_add(fx_findings *f, const char *rule) {
	char **r = realloc(f->rules, (f->nrules + 1) * sizeof *r);
	if (!r)
		return;
	f->rules = r;
	f->rules[f->nrules++] = strdup(rule ? rule : "?");
}

static void findings_free(fx_findings *f) {
	for (size_t i = 0; i < f->nrules; i++)
		free(f->rules[i]);
	free(f->rules);
	memset(f, 0, sizeof *f);
}

/* ---------------------------------------------------------------- strings */

typedef struct {
	char *s;
	size_t len, cap;
} sbuf;

static void sb_grow(sbuf *b, size_t n) {
	if (b->len + n + 1 <= b->cap)
		return;
	size_t cap = b->cap ? b->cap : 256;
	while (cap < b->len + n + 1)
		cap *= 2;
	char *s = realloc(b->s, cap);
	if (!s) {
		fputs("out of memory\n", stderr);
		exit(70);
	}
	b->s = s;
	b->cap = cap;
}

static void sb_put(sbuf *b, const char *s, size_t n) {
	sb_grow(b, n);
	memcpy(b->s + b->len, s, n);
	b->len += n;
	b->s[b->len] = 0;
}

static void sb_puts(sbuf *b, const char *s) { sb_put(b, s, strlen(s)); }

static void sb_printf(sbuf *b, const char *fmt, ...)
#if defined(__GNUC__) || defined(__clang__)
	__attribute__((format(printf, 2, 3)))
#endif
	;
static void sb_printf(sbuf *b, const char *fmt, ...) {
	va_list ap;
	va_start(ap, fmt);
	int n = vsnprintf(NULL, 0, fmt, ap);
	va_end(ap);
	if (n <= 0)
		return;
	sb_grow(b, (size_t)n);
	va_start(ap, fmt);
	vsnprintf(b->s + b->len, (size_t)n + 1, fmt, ap);
	va_end(ap);
	b->len += (size_t)n;
}

/* Length of the valid UTF-8 sequence at s (0 when invalid). */
static size_t utf8_len(const unsigned char *s, size_t avail) {
	unsigned char c = s[0];
	size_t n = c < 0x80 ? 1 : (c & 0xE0) == 0xC0 ? 2 : (c & 0xF0) == 0xE0 ? 3 : (c & 0xF8) == 0xF0 ? 4 : 0;
	if (n == 0 || n > avail || (n == 2 && c < 0xC2))
		return 0;
	for (size_t i = 1; i < n; i++)
		if ((s[i] & 0xC0) != 0x80)
			return 0;
	return n;
}

/* Appends `s` as a JSON string; invalid UTF-8 bytes become U+FFFD. */
static void sb_json(sbuf *b, const char *s) {
	const unsigned char *p = (const unsigned char *)(s ? s : "");
	size_t avail = strlen((const char *)p);
	sb_put(b, "\"", 1);
	while (*p) {
		unsigned char c = *p;
		if (c == '"' || c == '\\') {
			char e[2] = {'\\', (char)c};
			sb_put(b, e, 2);
			p++, avail--;
		} else if (c < 0x20) {
			switch (c) {
			case '\n': sb_puts(b, "\\n"); break;
			case '\t': sb_puts(b, "\\t"); break;
			case '\r': sb_puts(b, "\\r"); break;
			default: sb_printf(b, "\\u%04x", c);
			}
			p++, avail--;
		} else {
			size_t n = utf8_len(p, avail);
			if (n == 0) {
				sb_puts(b, "\xEF\xBF\xBD");
				p++, avail--;
			} else {
				sb_put(b, (const char *)p, n);
				p += n, avail -= n;
			}
		}
	}
	sb_put(b, "\"", 1);
}

/* Newlines replaced by spaces, cut at 400 characters (as the Rust harness). */
static char *shorten(const char *s) {
	sbuf b = {0};
	size_t chars = 0;
	for (const unsigned char *p = (const unsigned char *)(s ? s : ""); *p;) {
		if (chars == 400) {
			sb_puts(&b, "…");
			break;
		}
		size_t n = utf8_len(p, strlen((const char *)p));
		if (n == 0)
			n = 1;
		if (*p == '\n' || *p == '\r')
			sb_put(&b, " ", 1);
		else
			sb_put(&b, (const char *)p, n);
		p += n;
		chars++;
	}
	if (!b.s)
		sb_puts(&b, "");
	return b.s;
}

/* ---------------------------------------------------------------- paths, corpus */

static char *env_or(const char *name, const char *dflt) {
	const char *v = getenv(name);
	return strdup(v && *v ? v : dflt);
}

const char *fx_ecosystem(void) {
	static char *v;
	if (!v)
		v = env_or("FX_ECOSYSTEM", FX_DEFAULT_ECOSYSTEM);
	return v;
}

const char *fx_corpus(void) {
	static char *v;
	if (!v) {
		const char *c = getenv("FX_CORPUS");
		if (c && *c) {
			v = strdup(c);
		} else {
			size_t n = strlen(fx_ecosystem()) + 8;
			v = malloc(n);
			snprintf(v, n, "%s/corpus", fx_ecosystem());
		}
	}
	return v;
}

const char *fx_artefacts(void) {
	static char *v;
	if (!v)
		v = env_or("FX_ARTEFACTS", FX_DEFAULT_ARTEFACTS);
	return v;
}

unsigned char *fx_read_file(const char *path, size_t *len) {
	*len = 0;
	FILE *f = fopen(path, "rb");
	if (!f)
		return NULL;
	sbuf b = {0};
	char chunk[65536];
	size_t n;
	while ((n = fread(chunk, 1, sizeof chunk, f)) > 0)
		sb_put(&b, chunk, n);
	int bad = ferror(f);
	fclose(f);
	if (bad) {
		free(b.s);
		return NULL;
	}
	if (!b.s)
		sb_puts(&b, "");
	*len = b.len;
	return (unsigned char *)b.s;
}

static unsigned char *load(const char *path, size_t *len) {
	unsigned char *d = fx_read_file(path, len);
	if (!d) {
		fprintf(stderr, "load %s: %s\n", path, strerror(errno));
		exit(1);
	}
	return d;
}

static char *join(const char *a, const char *b) {
	size_t n = strlen(a) + strlen(b) + 2;
	char *s = malloc(n);
	snprintf(s, n, "%s/%s", a, b);
	return s;
}

static unsigned char *sample(const char *name, size_t *len) {
	static const struct {
		const char *name, *path;
		int host;
	} samples[] = {
		{"PDFSmall", "XML-Rechnung/FX/EN16931_Einfach.pdf", 0},
		{"PDFLarge", "XML-Rechnung/FX/EN16931_Elektron.pdf", 0},
		{"PDFMinimum", "ZUGFeRDv2/correct/FNFE-factur-x-examples/Facture_FR_MINIMUM.pdf", 0},
		{"PDFExtended", "ZUGFeRDv2/correct/intarsys/EXTENDED/zugferd_2p0_EXTENDED_Warenrechnung.pdf", 0},
		{"XMLSmall", "XML-Rechnung/CII/EN16931_Einfach.cii.xml", 0},
		{"XMLLines", "XML-Rechnung/CII/EN16931_Rabatte.cii.xml", 0},
		{"XMLLarge", "XML-Rechnung/CII/XRECHNUNG_Elektron.cii.xml", 0},
		{"Host", "host.pdf", 1},
		{"HostClassic", "host_classic.pdf", 1},
	};
	for (size_t i = 0; i < sizeof samples / sizeof samples[0]; i++) {
		if (strcmp(samples[i].name, name) == 0) {
			char *p = join(samples[i].host ? fx_ecosystem() : fx_corpus(), samples[i].path);
			unsigned char *d = load(p, len);
			free(p);
			return d;
		}
	}
	fprintf(stderr, "unknown sample %s\n", name);
	exit(1);
}

static const char *input_of(const char *bench) {
	static const char *map[][2] = {
		{"Extract_Small", "PDFSmall"},       {"CheckPDF_Small", "PDFSmall"},
		{"Extract_Large", "PDFLarge"},       {"CheckPDF_Large", "PDFLarge"},
		{"Extract_Minimum", "PDFMinimum"},   {"Extract_Extended", "PDFExtended"},
		{"Parse_Small", "XMLSmall"},         {"ParseValidate_Small", "XMLSmall"},
		{"Write_Small", "XMLSmall"},         {"Parse_Lines", "XMLLines"},
		{"ParseValidate_Lines", "XMLLines"}, {"Write_Lines", "XMLLines"},
		{"Parse_Large", "XMLLarge"},         {"ParseValidate_Large", "XMLLarge"},
		{"Embed_Small", "Host"},             {"EmbedClassic_Small", "HostClassic"},
	};
	for (size_t i = 0; i < sizeof map / sizeof map[0]; i++)
		if (strcmp(map[i][0], bench) == 0)
			return map[i][1];
	fprintf(stderr, "unknown bench %s\n", bench);
	exit(1);
}

typedef struct {
	char **v;
	size_t n;
} strlist;

static void sl_push(strlist *l, char *s) {
	l->v = realloc(l->v, (l->n + 1) * sizeof *l->v);
	l->v[l->n++] = s;
}

static int cmp_str(const void *a, const void *b) { return strcmp(*(char *const *)a, *(char *const *)b); }

static int is_dir(const char *p) {
	struct stat st;
	return stat(p, &st) == 0 && S_ISDIR(st.st_mode);
}

static int has_suffix(const char *s, const char *suf) {
	size_t a = strlen(s), b = strlen(suf);
	return a >= b && strcmp(s + a - b, suf) == 0;
}

/* Entries of `dir`, sorted by name (as the Rust harness sorts paths). */
static strlist list_dir(const char *dir) {
	strlist l = {0};
	DIR *d = opendir(dir);
	if (!d)
		return l;
	struct dirent *e;
	while ((e = readdir(d)) != NULL) {
		if (strcmp(e->d_name, ".") == 0 || strcmp(e->d_name, "..") == 0)
			continue;
		sl_push(&l, join(dir, e->d_name));
	}
	closedir(d);
	if (l.n)
		qsort(l.v, l.n, sizeof *l.v, cmp_str);
	return l;
}

/* The 26 valid CII samples of the surveys. */
static strlist cii_files(void) {
	char *dir = join(fx_corpus(), "XML-Rechnung/CII");
	strlist all = list_dir(dir), out = {0};
	free(dir);
	for (size_t i = 0; i < all.n; i++) {
		const char *base = strrchr(all.v[i], '/') + 1;
		if (has_suffix(base, ".cii.xml") &&
		    strcmp(base, "not_validating_full_invoice_based_onTest_EeISI_300_CENfullmodel.cii.xml") != 0)
			sl_push(&out, all.v[i]);
		else
			free(all.v[i]);
	}
	free(all.v);
	return out;
}

static void walk_pdfs(const char *dir, strlist *out) {
	strlist l = list_dir(dir);
	for (size_t i = 0; i < l.n; i++) {
		if (is_dir(l.v[i])) {
			walk_pdfs(l.v[i], out);
			free(l.v[i]);
		} else if (has_suffix(l.v[i], ".pdf")) {
			sl_push(out, l.v[i]);
		} else {
			free(l.v[i]);
		}
	}
	free(l.v);
}

/* The 104 valid hybrid PDFs of the surveys: every .pdf under
 * ZUGFeRDv2/correct (recursive) and XML-Rechnung/FX/\*.pdf. */
static strlist pdf_files(void) {
	strlist out = {0};
	char *dir = join(fx_corpus(), "ZUGFeRDv2/correct");
	walk_pdfs(dir, &out);
	free(dir);
	dir = join(fx_corpus(), "XML-Rechnung/FX");
	strlist fx = list_dir(dir);
	free(dir);
	for (size_t i = 0; i < fx.n; i++) {
		if (has_suffix(fx.v[i], ".pdf") && !is_dir(fx.v[i]))
			sl_push(&out, fx.v[i]);
		else
			free(fx.v[i]);
	}
	free(fx.v);
	return out;
}

static void sl_free(strlist *l) {
	for (size_t i = 0; i < l->n; i++)
		free(l->v[i]);
	free(l->v);
	memset(l, 0, sizeof *l);
}

static const char *rel(const char *p) {
	size_t n = strlen(fx_corpus());
	if (strncmp(p, fx_corpus(), n) == 0 && p[n] == '/')
		return p + n + 1;
	return p;
}

static int memmem_has(const unsigned char *b, size_t len, const char *needle) {
	size_t n = strlen(needle);
	if (n == 0 || len < n)
		return 0;
	for (size_t i = 0; i + n <= len; i++)
		if (b[i] == (unsigned char)needle[0] && memcmp(b + i, needle, n) == 0)
			return 1;
	return 0;
}

int fx_looks_like_cii(const unsigned char *b, size_t len) {
	return memmem_has(b, len, "CrossIndustryInvoice") || memmem_has(b, len, "CrossIndustryDocument");
}

int fx_is_invoice_name(const char *name) {
	static const char *names[] = {"factur-x.xml", "zugferd-invoice.xml", "xrechnung.xml", "order-x.xml", "cii.xml"};
	for (size_t i = 0; i < sizeof names / sizeof names[0]; i++)
		if (strcasecmp(name, names[i]) == 0)
			return 1;
	return 0;
}

/* ---------------------------------------------------------------- Factur-X helpers */

const char *fx_facturx_relationship(const char *level) {
	return strcmp(level, "MINIMUM") == 0 || strcmp(level, "BASIC WL") == 0 ? "Data" : "Alternative";
}

const char *fx_facturx_level(const unsigned char *xml, size_t len) {
	/* BT-24: the first ID after GuidelineSpecifiedDocumentContextParameter. */
	const char *tag = "GuidelineSpecifiedDocumentContextParameter";
	size_t t = strlen(tag), i = 0;
	for (; i + t <= len; i++)
		if (memcmp(xml + i, tag, t) == 0)
			break;
	if (i + t > len)
		return "EN 16931";
	size_t end = i + 1024 < len ? i + 1024 : len;
	const unsigned char *p = xml + i;
	size_t n = end - i;
	char urn[512] = {0};
	for (size_t k = 0; k + 4 < n; k++) {
		if (memcmp(p + k, "urn:", 4) == 0) {
			size_t m = 0;
			while (k + m < n && p[k + m] != '<' && m + 1 < sizeof urn) {
				urn[m] = (char)p[k + m];
				m++;
			}
			urn[m] = 0;
			break;
		}
	}
	if (strstr(urn, "xrechnung"))
		return "XRECHNUNG";
	if (strstr(urn, "extended"))
		return "EXTENDED";
	if (strstr(urn, "basicwl"))
		return "BASIC WL";
	if (strstr(urn, "minimum"))
		return "MINIMUM";
	if (strstr(urn, "basic"))
		return "BASIC";
	return "EN 16931";
}

size_t fx_facturx_xmp(char *buf, size_t cap, const char *level) {
	int n = snprintf(
		buf, cap,
		"<?xpacket begin=\"\xEF\xBB\xBF\" id=\"W5M0MpCehiHzreSzNTczkc9d\"?>\n"
		"<x:xmpmeta xmlns:x=\"adobe:ns:meta/\">\n"
		" <rdf:RDF xmlns:rdf=\"http://www.w3.org/1999/02/22-rdf-syntax-ns#\">\n"
		"  <rdf:Description rdf:about=\"\" xmlns:pdfaid=\"http://www.aiim.org/pdfa/ns/id/\">\n"
		"   <pdfaid:part>3</pdfaid:part>\n"
		"   <pdfaid:conformance>B</pdfaid:conformance>\n"
		"  </rdf:Description>\n"
		"  <rdf:Description rdf:about=\"\" xmlns:fx=\"urn:factur-x:pdfa:CrossIndustryDocument:invoice:1p0#\">\n"
		"   <fx:DocumentType>INVOICE</fx:DocumentType>\n"
		"   <fx:DocumentFileName>" FX_ATTACHMENT "</fx:DocumentFileName>\n"
		"   <fx:Version>1.0</fx:Version>\n"
		"   <fx:ConformanceLevel>%s</fx:ConformanceLevel>\n"
		"  </rdf:Description>\n"
		"  <rdf:Description rdf:about=\"\" xmlns:pdfaExtension=\"http://www.aiim.org/pdfa/ns/extension/\""
		" xmlns:pdfaSchema=\"http://www.aiim.org/pdfa/ns/schema#\" xmlns:pdfaProperty=\"http://www.aiim.org/pdfa/ns/property#\">\n"
		"   <pdfaExtension:schemas><rdf:Bag><rdf:li rdf:parseType=\"Resource\">\n"
		"    <pdfaSchema:schema>Factur-X PDFA Extension Schema</pdfaSchema:schema>\n"
		"    <pdfaSchema:namespaceURI>urn:factur-x:pdfa:CrossIndustryDocument:invoice:1p0#</pdfaSchema:namespaceURI>\n"
		"    <pdfaSchema:prefix>fx</pdfaSchema:prefix>\n"
		"    <pdfaSchema:property><rdf:Seq>\n"
		"     <rdf:li rdf:parseType=\"Resource\"><pdfaProperty:name>DocumentFileName</pdfaProperty:name><pdfaProperty:valueType>Text</pdfaProperty:valueType><pdfaProperty:category>external</pdfaProperty:category><pdfaProperty:description>The name of the embedded XML document</pdfaProperty:description></rdf:li>\n"
		"     <rdf:li rdf:parseType=\"Resource\"><pdfaProperty:name>DocumentType</pdfaProperty:name><pdfaProperty:valueType>Text</pdfaProperty:valueType><pdfaProperty:category>external</pdfaProperty:category><pdfaProperty:description>The type of the hybrid document</pdfaProperty:description></rdf:li>\n"
		"     <rdf:li rdf:parseType=\"Resource\"><pdfaProperty:name>Version</pdfaProperty:name><pdfaProperty:valueType>Text</pdfaProperty:valueType><pdfaProperty:category>external</pdfaProperty:category><pdfaProperty:description>The version of the standard</pdfaProperty:description></rdf:li>\n"
		"     <rdf:li rdf:parseType=\"Resource\"><pdfaProperty:name>ConformanceLevel</pdfaProperty:name><pdfaProperty:valueType>Text</pdfaProperty:valueType><pdfaProperty:category>external</pdfaProperty:category><pdfaProperty:description>The conformance level of the embedded data</pdfaProperty:description></rdf:li>\n"
		"    </rdf:Seq></pdfaSchema:property>\n"
		"   </rdf:li></rdf:Bag></pdfaExtension:schemas>\n"
		"  </rdf:Description>\n"
		" </rdf:RDF>\n"
		"</x:xmpmeta>\n"
		"<?xpacket end=\"w\"?>",
		level);
	return n > 0 && (size_t)n < cap ? (size_t)n : 0;
}

/* ---------------------------------------------------------------- harness state */

typedef struct {
	char *bench, *note;
} skip_t;

struct fx_harness {
	const char *lang;
	fx_lib lib;
	int survey;
	int resources; /* one benchmark in this process, record with max_rss_bytes */
	int count;
	double benchtime;
	FILE *out;
	char **only;
	int nonly;
	skip_t *skipped;
	size_t nskipped;
	void (*body)(fx_harness *, void *);
	void *ctx;
};

/* Line written by the crash handler if a library kills the process while a
 * benchmark or a survey runs (prepared before each call). */
static char crash_line[8192];
static int crash_fd = -1;

static void on_crash(int sig) {
	if (crash_fd >= 0 && crash_line[0]) {
		char tail[64];
		int n = snprintf(tail, sizeof tail, "%d\"}\n", sig);
		ssize_t w = write(crash_fd, crash_line, strlen(crash_line));
		w = write(crash_fd, tail, (size_t)n);
		(void)w;
	}
	static const char msg[] = "   crashed: signal; results so far are kept\n";
	ssize_t w = write(2, msg, sizeof msg - 1);
	(void)w;
	_exit(70);
}

static void common_head(fx_harness *h, sbuf *b) {
	sb_puts(b, "{\"lang\":");
	sb_json(b, h->lang);
	sb_puts(b, ",\"lib\":");
	sb_json(b, h->lib.lib);
	sb_puts(b, ",\"pkg\":");
	sb_json(b, h->lib.pkg);
	sb_puts(b, ",\"version\":");
	sb_json(b, h->lib.version);
}

/* Prepares the line the crash handler writes; the signal number and the
 * closing of the JSON object are appended there. */
static void arm_crash_bench(fx_harness *h, const char *bench) {
	sbuf b = {0};
	common_head(h, &b);
	sb_puts(&b, ",\"bench\":");
	sb_json(&b, bench);
	sb_puts(&b, ",\"status\":\"fail\",\"ns_per_op\":[],\"cpu_ns_per_op\":[],\"iters\":0,\"first_call_ns\":0,"
		    "\"note\":\"crash: signal ");
	snprintf(crash_line, sizeof crash_line, "%s", b.s);
	free(b.s);
}

static void arm_crash_survey(fx_harness *h, const char *op, size_t ok, size_t total, const char *file) {
	sbuf b = {0};
	common_head(h, &b);
	sb_puts(&b, ",\"op\":");
	sb_json(&b, op);
	sb_printf(&b, ",\"ok\":%zu,\"total\":%zu,\"failures\":[],\"note\":\"survey stopped, crash on ", ok, total);
	/* the file name goes inside the note string: escape it without quotes */
	sbuf f = {0};
	sb_json(&f, file);
	sb_put(&b, f.s + 1, f.len - 2);
	free(f.s);
	sb_puts(&b, ": signal ");
	snprintf(crash_line, sizeof crash_line, "%s", b.s);
	free(b.s);
}

static void disarm(void) { crash_line[0] = 0; }

static void put_line(fx_harness *h, sbuf *b) {
	sb_puts(b, "\n");
	fputs(b->s, h->out);
	fflush(h->out);
	free(b->s);
	memset(b, 0, sizeof *b);
}

/* Peak resident set size of the process (getrusage ru_maxrss: bytes on
 * macOS, kilobytes on Linux). */
static unsigned long long max_rss_bytes(void) {
	struct rusage ru;
	if (getrusage(RUSAGE_SELF, &ru) != 0)
		return 0;
#ifdef __APPLE__
	return (unsigned long long)ru.ru_maxrss;
#else
	return (unsigned long long)ru.ru_maxrss * 1024ull;
#endif
}

static int cmp_double(const void *a, const void *b) {
	double x = *(const double *)a, y = *(const double *)b;
	return x < y ? -1 : x > y;
}

static double median(const double *v, int n) {
	if (n <= 0)
		return 0;
	double *s = malloc((size_t)n * sizeof *s);
	memcpy(s, v, (size_t)n * sizeof *s);
	qsort(s, (size_t)n, sizeof *s, cmp_double);
	double m = n % 2 ? s[n / 2] : (s[n / 2 - 1] + s[n / 2]) / 2;
	free(s);
	return m;
}

static void series(sbuf *b, const char *key, const double *v, int n) {
	sb_printf(b, ",\"%s\":[", key);
	for (int i = 0; i < n; i++)
		sb_printf(b, "%s%.1f", i ? "," : "", v[i]);
	sb_puts(b, "]");
}

/* One record: `ns` and `cpu` are the wall and CPU ns/op of the `nns` series
 * (NULL and 0 when nothing was timed). In the resources pass, the record of a
 * measured benchmark also carries the peak RSS of the process, read here,
 * after the series. */
static void bench_line(fx_harness *h, const char *bench, const char *status, const double *ns, const double *cpu,
		       int nns, unsigned long long iters, unsigned long long first, const char *note) {
	int ok = strcmp(status, "ok") == 0;
	unsigned long long rss = h->resources && ok ? max_rss_bytes() : 0;
	sbuf b = {0};
	common_head(h, &b);
	sb_puts(&b, ",\"bench\":");
	sb_json(&b, bench);
	sb_puts(&b, ",\"status\":");
	sb_json(&b, status);
	series(&b, "ns_per_op", ns, nns);
	series(&b, "cpu_ns_per_op", cpu, nns);
	double min = 0;
	for (int i = 0; i < nns; i++)
		if (i == 0 || ns[i] < min)
			min = ns[i];
	sb_printf(&b, ",\"iters\":%llu,\"first_call_ns\":%llu,\"note\":", iters, first);
	sb_json(&b, note);
	if (h->resources && ok)
		sb_printf(&b, ",\"max_rss_bytes\":%llu", rss);
	sb_puts(&b, "}");
	put_line(h, &b);
	if (ok && h->resources) {
		fprintf(stderr, "   %-22s %14.1f ns/op %14.1f cpu ns/op %9.1f MiB  (N=%llu)\n", bench, median(ns, nns),
			median(cpu, nns), (double)rss / 1048576.0, iters);
	} else if (ok) {
		fprintf(stderr, "   %-22s %14.1f ns/op  (N=%llu)\n", bench, min, iters);
	} else {
		char *s = shorten(note);
		fprintf(stderr, "   %-22s %14s  %.100s\n", bench, status, s);
		free(s);
	}
}

static int wanted(const fx_harness *h, const char *bench) {
	if (!h->only)
		return 1;
	for (int i = 0; i < h->nonly; i++)
		if (strcmp(h->only[i], bench) == 0)
			return 1;
	return 0;
}

/* Whether `bench` is to be measured; a bench marked with fx_skip() is
 * recorded as unsupported instead. */
static int gate(fx_harness *h, const char *bench) {
	if (!wanted(h, bench))
		return 0;
	for (size_t i = 0; i < h->nskipped; i++) {
		if (strcmp(h->skipped[i].bench, bench) == 0) {
			bench_line(h, bench, "unsupported", NULL, NULL, 0, 0, 0, h->skipped[i].note);
			return 0;
		}
	}
	return 1;
}

void fx_skip(fx_harness *h, const char *bench, const char *note) {
	h->skipped = realloc(h->skipped, (h->nskipped + 1) * sizeof *h->skipped);
	h->skipped[h->nskipped].bench = strdup(bench);
	h->skipped[h->nskipped].note = strdup(note);
	h->nskipped++;
}

static const char *const benches_extract[] = {"Extract_Small", "Extract_Large", "Extract_Minimum", "Extract_Extended", NULL};
static const char *const benches_parse[] = {"Parse_Small", "Parse_Lines", "Parse_Large", NULL};
static const char *const benches_validate[] = {"ParseValidate_Small", "ParseValidate_Lines", "ParseValidate_Large", NULL};
static const char *const benches_write[] = {"Write_Small", "Write_Lines", NULL};
static const char *const benches_embed[] = {"Embed_Small", "EmbedClassic_Small", NULL};
static const char *const benches_check[] = {"CheckPDF_Small", "CheckPDF_Large", NULL};

static const char *const *benches_of(fx_op op) {
	switch (op) {
	case FX_EXTRACT: return benches_extract;
	case FX_PARSE: return benches_parse;
	case FX_VALIDATE: return benches_validate;
	case FX_WRITE: return benches_write;
	case FX_EMBED: return benches_embed;
	case FX_CHECK_PDF: return benches_check;
	}
	return benches_extract;
}

void fx_unsupported(fx_harness *h, fx_op op, const char *note) {
	if (h->survey)
		return;
	for (const char *const *b = benches_of(op); *b; b++)
		if (wanted(h, *b))
			bench_line(h, *b, "unsupported", NULL, NULL, 0, 0, 0, note);
}

int fx_is_survey(const fx_harness *h) { return h->survey; }

/* ---------------------------------------------------------------- timing */

/* Monotonic clock. On macOS, CLOCK_MONOTONIC only ticks in microseconds:
 * CLOCK_UPTIME_RAW (42 ns on Apple silicon) is the clock behind Rust's
 * std::time::Instant there. */
#if defined(__APPLE__) && defined(CLOCK_UPTIME_RAW)
#define FX_CLOCK CLOCK_UPTIME_RAW
#else
#define FX_CLOCK CLOCK_MONOTONIC
#endif

static unsigned long long now_ns(void) {
	struct timespec ts;
	clock_gettime(FX_CLOCK, &ts);
	return (unsigned long long)ts.tv_sec * 1000000000ull + (unsigned long long)ts.tv_nsec;
}

/* CPU time of the process, user + system, all its threads (getrusage
 * RUSAGE_SELF), read once before and once after each timed series. It ticks
 * in microseconds, negligible over a series of BENCHTIME seconds. */
static unsigned long long cpu_ns(void) {
	struct rusage ru;
	if (getrusage(RUSAGE_SELF, &ru) != 0)
		return 0;
	return ((unsigned long long)ru.ru_utime.tv_sec + (unsigned long long)ru.ru_stime.tv_sec) * 1000000000ull +
	       ((unsigned long long)ru.ru_utime.tv_usec + (unsigned long long)ru.ru_stime.tv_usec) * 1000ull;
}

/* One call of the measured operation; returns 0 on success. The result is
 * released by the call itself (inside the timed loop). */
typedef struct {
	int (*call)(void *self);
	void *self;
} op_call;

static void measure(fx_harness *h, const char *bench, unsigned long long first, const char *note, op_call c) {
	unsigned long long n = 1;
	unsigned step = 0;
	unsigned long long target = (unsigned long long)(h->benchtime * 1e9);
	for (;;) {
		unsigned long long t = now_ns();
		for (unsigned long long i = 0; i < n; i++)
			c.call(c.self);
		if (now_ns() - t >= target)
			break;
		step++;
		static const unsigned long long mant[3] = {1, 2, 5};
		unsigned long long p = 1;
		for (unsigned k = 0; k < step / 3; k++)
			p *= 10;
		n = mant[step % 3] * p;
	}
	double *ns = calloc((size_t)h->count, sizeof *ns);
	double *cpu = calloc((size_t)h->count, sizeof *cpu);
	for (int s = 0; s < h->count; s++) {
		unsigned long long c0 = cpu_ns();
		unsigned long long t = now_ns();
		for (unsigned long long i = 0; i < n; i++)
			c.call(c.self);
		unsigned long long t1 = now_ns();
		unsigned long long c1 = cpu_ns();
		ns[s] = (double)(t1 - t) / (double)n;
		cpu[s] = (double)(c1 - c0) / (double)n;
	}
	bench_line(h, bench, "ok", ns, cpu, h->count, n, first, note);
	free(ns);
	free(cpu);
}

static void fail_line(fx_harness *h, const char *bench, unsigned long long first, const char *msg, const char *note) {
	char *s = shorten(msg);
	sbuf b = {0};
	sb_puts(&b, s);
	if (note && *note) {
		sb_puts(&b, " — ");
		sb_puts(&b, note);
	}
	bench_line(h, bench, "fail", NULL, NULL, 0, 0, first, b.s);
	free(s);
	free(b.s);
}

static void survey_line(fx_harness *h, const char *op, size_t ok, size_t total, sbuf *failures, const char *note) {
	fprintf(stderr, "   SURVEY %s: %zu/%zu\n", op, ok, total);
	sbuf b = {0};
	common_head(h, &b);
	sb_puts(&b, ",\"op\":");
	sb_json(&b, op);
	sb_printf(&b, ",\"ok\":%zu,\"total\":%zu,\"failures\":[%s],\"note\":", ok, total, failures->s ? failures->s : "");
	sb_json(&b, note);
	sb_puts(&b, "}");
	put_line(h, &b);
}

static void failure_entry(sbuf *f, const char *file, const char *error) {
	char *s = shorten(error);
	if (f->len)
		sb_puts(f, ",");
	sb_puts(f, "{\"file\":");
	sb_json(f, file);
	sb_puts(f, ",\"error\":");
	sb_json(f, s);
	sb_puts(f, "}");
	free(s);
}

/* ---------------------------------------------------------------- extract */

typedef struct {
	fx_extract_fn f;
	void *ctx;
	const unsigned char *data;
	size_t len;
} extract_call;

static int extract_once(void *self) {
	extract_call *c = self;
	fx_bytes out = {0};
	fx_err e;
	int r = c->f(c->ctx, c->data, c->len, &out, &e);
	if (r == 0) {
		fx_sink += out.len;
		fx_bytes_release(&out);
	}
	return r;
}

void fx_extract(fx_harness *h, const char *note, fx_extract_fn f, void *ctx) {
	if (!h->survey) {
		for (const char *const *b = benches_extract; *b; b++) {
			if (!gate(h, *b))
				continue;
			size_t len;
			unsigned char *data = sample(input_of(*b), &len);
			arm_crash_bench(h, *b);
			fx_bytes out = {0};
			fx_err e = {{0}};
			unsigned long long t = now_ns();
			int r = f(ctx, data, len, &out, &e);
			unsigned long long first = now_ns() - t;
			if (r != 0) {
				fail_line(h, *b, first, e.msg, note);
			} else if (!fx_looks_like_cii(out.data, out.len)) {
				fx_bytes_release(&out);
				fail_line(h, *b, first, "extracted bytes are not a CII invoice", note);
			} else {
				fx_bytes_release(&out);
				extract_call c = {f, ctx, data, len};
				measure(h, *b, first, note, (op_call){extract_once, &c});
			}
			disarm();
			free(data);
		}
		return;
	}
	strlist files = pdf_files();
	size_t ok = 0;
	sbuf failures = {0};
	for (size_t i = 0; i < files.n; i++) {
		size_t len;
		unsigned char *data = load(files.v[i], &len);
		arm_crash_survey(h, "extract", ok, files.n, rel(files.v[i]));
		fx_bytes out = {0};
		fx_err e = {{0}};
		if (f(ctx, data, len, &out, &e) != 0) {
			failure_entry(&failures, rel(files.v[i]), e.msg);
		} else {
			if (fx_looks_like_cii(out.data, out.len))
				ok++;
			else
				failure_entry(&failures, rel(files.v[i]), "extracted bytes are not a CII invoice");
			fx_bytes_release(&out);
		}
		disarm();
		free(data);
	}
	survey_line(h, "extract", ok, files.n, &failures, note);
	free(failures.s);
	sl_free(&files);
}

/* ---------------------------------------------------------------- parse */

typedef struct {
	fx_parse_fn f;
	fx_free_fn free_model;
	void *ctx;
	const unsigned char *data;
	size_t len;
} parse_call;

static int parse_once(void *self) {
	parse_call *c = self;
	void *m = NULL;
	fx_err e;
	int r = c->f(c->ctx, c->data, c->len, &m, &e);
	fx_sink += (size_t)(uintptr_t)m;
	if (r == 0 && c->free_model)
		c->free_model(c->ctx, m);
	return r;
}

void fx_parse(fx_harness *h, const char *note, fx_parse_fn f, fx_free_fn free_model, void *ctx) {
	if (!h->survey) {
		for (const char *const *b = benches_parse; *b; b++) {
			if (!gate(h, *b))
				continue;
			size_t len;
			unsigned char *data = sample(input_of(*b), &len);
			arm_crash_bench(h, *b);
			void *m = NULL;
			fx_err e = {{0}};
			unsigned long long t = now_ns();
			int r = f(ctx, data, len, &m, &e);
			unsigned long long first = now_ns() - t;
			if (r != 0) {
				fail_line(h, *b, first, e.msg, note);
			} else {
				if (free_model)
					free_model(ctx, m);
				parse_call c = {f, free_model, ctx, data, len};
				measure(h, *b, first, note, (op_call){parse_once, &c});
			}
			disarm();
			free(data);
		}
		return;
	}
	strlist files = cii_files();
	size_t ok = 0;
	sbuf failures = {0};
	for (size_t i = 0; i < files.n; i++) {
		size_t len;
		unsigned char *data = load(files.v[i], &len);
		arm_crash_survey(h, "parse", ok, files.n, rel(files.v[i]));
		void *m = NULL;
		fx_err e = {{0}};
		if (f(ctx, data, len, &m, &e) != 0) {
			failure_entry(&failures, rel(files.v[i]), e.msg);
		} else {
			ok++;
			if (free_model)
				free_model(ctx, m);
		}
		disarm();
		free(data);
	}
	survey_line(h, "parse", ok, files.n, &failures, note);
	free(failures.s);
	sl_free(&files);
}

/* ---------------------------------------------------------------- validate */

typedef struct {
	fx_validate_fn f;
	fx_free_fn free_report;
	void *ctx;
	const unsigned char *data;
	size_t len;
} validate_call;

static int validate_once(void *self) {
	validate_call *c = self;
	void *rep = NULL;
	fx_err e;
	int r = c->f(c->ctx, c->data, c->len, &rep, &e);
	fx_sink += (size_t)(uintptr_t)rep;
	if (r == 0 && c->free_report)
		c->free_report(c->ctx, rep);
	return r;
}

void fx_validate(fx_harness *h, const char *note, fx_validate_fn f, fx_summarize_fn summarize,
		 fx_free_fn free_report, void *ctx) {
	if (!h->survey) {
		for (const char *const *b = benches_validate; *b; b++) {
			if (!gate(h, *b))
				continue;
			size_t len;
			unsigned char *data = sample(input_of(*b), &len);
			arm_crash_bench(h, *b);
			void *rep = NULL;
			fx_err e = {{0}};
			unsigned long long t = now_ns();
			int r = f(ctx, data, len, &rep, &e);
			unsigned long long first = now_ns() - t;
			if (r != 0) {
				fail_line(h, *b, first, e.msg, note);
			} else {
				if (free_report)
					free_report(ctx, rep);
				validate_call c = {f, free_report, ctx, data, len};
				measure(h, *b, first, note, (op_call){validate_once, &c});
			}
			disarm();
			free(data);
		}
		return;
	}
	strlist files = cii_files();
	size_t flagged = 0;
	sbuf failures = {0};
	for (size_t i = 0; i < files.n; i++) {
		size_t len;
		unsigned char *data = load(files.v[i], &len);
		arm_crash_survey(h, "validate", flagged, files.n, rel(files.v[i]));
		void *rep = NULL;
		fx_err e = {{0}};
		if (f(ctx, data, len, &rep, &e) != 0) {
			flagged++;
			char *s = shorten(e.msg);
			if (failures.len)
				sb_puts(&failures, ",");
			sb_puts(&failures, "{\"file\":");
			sb_json(&failures, rel(files.v[i]));
			sb_puts(&failures, ",\"errors\":0,\"rules\":[],\"error\":");
			sb_json(&failures, s);
			sb_puts(&failures, "}");
			free(s);
		} else {
			fx_findings fd = {0};
			summarize(ctx, rep, &fd);
			if (free_report)
				free_report(ctx, rep);
			if (fd.errors > 0) {
				flagged++;
				if (failures.len)
					sb_puts(&failures, ",");
				sb_puts(&failures, "{\"file\":");
				sb_json(&failures, rel(files.v[i]));
				sb_printf(&failures, ",\"errors\":%zu,\"rules\":[", fd.errors);
				for (size_t k = 0; k < fd.nrules; k++) {
					if (k)
						sb_puts(&failures, ",");
					sb_json(&failures, fd.rules[k]);
				}
				sb_puts(&failures, "]}");
			}
			findings_free(&fd);
		}
		disarm();
		free(data);
	}
	survey_line(h, "validate", flagged, files.n, &failures, note);
	free(failures.s);
	sl_free(&files);
}

/* ---------------------------------------------------------------- write */

typedef struct {
	fx_write_fn f;
	void *ctx;
	void *model;
} write_call;

static int write_once(void *self) {
	write_call *c = self;
	fx_bytes out = {0};
	fx_err e;
	int r = c->f(c->ctx, c->model, &out, &e);
	if (r == 0) {
		fx_sink += out.len;
		fx_bytes_release(&out);
	}
	return r;
}

void fx_write(fx_harness *h, const char *note, fx_parse_fn parse, fx_free_fn free_model, fx_write_fn write,
	      void *ctx) {
	if (h->survey)
		return;
	for (const char *const *b = benches_write; *b; b++) {
		if (!gate(h, *b))
			continue;
		size_t len;
		unsigned char *data = sample(input_of(*b), &len);
		arm_crash_bench(h, *b);
		void *m = NULL;
		fx_err e = {{0}};
		if (parse(ctx, data, len, &m, &e) != 0) {
			char msg[FX_ERR_MAX + 16];
			snprintf(msg, sizeof msg, "parse: %s", e.msg);
			char *s = shorten(msg);
			bench_line(h, *b, "fail", NULL, NULL, 0, 0, 0, s);
			free(s);
			disarm();
			free(data);
			continue;
		}
		fx_bytes out = {0};
		unsigned long long t = now_ns();
		int r = write(ctx, m, &out, &e);
		unsigned long long first = now_ns() - t;
		if (r != 0) {
			fail_line(h, *b, first, e.msg, note);
		} else if (!fx_looks_like_cii(out.data, out.len)) {
			fx_bytes_release(&out);
			fail_line(h, *b, first, "written bytes are not a CII invoice", note);
		} else {
			fx_bytes_release(&out);
			write_call c = {write, ctx, m};
			measure(h, *b, first, note, (op_call){write_once, &c});
		}
		if (free_model)
			free_model(ctx, m);
		disarm();
		free(data);
	}
}

/* ---------------------------------------------------------------- embed */

typedef struct {
	fx_embed_fn f;
	void *ctx;
	const unsigned char *host, *xml;
	size_t host_len, xml_len;
} embed_call;

static int embed_once(void *self) {
	embed_call *c = self;
	fx_bytes out = {0};
	fx_err e;
	int r = c->f(c->ctx, c->host, c->host_len, c->xml, c->xml_len, &out, &e);
	if (r == 0) {
		fx_sink += out.len;
		fx_bytes_release(&out);
	}
	return r;
}

/* Debugging aid: with FX_DUMP_EMBED=<dir>, the PDF produced by the check call
 * (outside the timing) is written to <dir>/<lang>_<lib>_<bench>.pdf, so that
 * it can be inspected with other tools. */
static void dump_embed(const fx_harness *h, const char *bench, const fx_bytes *out) {
	const char *dir = getenv("FX_DUMP_EMBED");
	if (!dir || !*dir)
		return;
	char path[4096];
	snprintf(path, sizeof path, "%s/%s_%s_%s.pdf", dir, h->lang, h->lib.lib, bench);
	FILE *f = fopen(path, "wb");
	if (!f || fwrite(out->data, 1, out->len, f) != out->len)
		fprintf(stderr, "   cannot write %s\n", path);
	if (f)
		fclose(f);
}

void fx_embed(fx_harness *h, const char *note, fx_embed_fn f, void *ctx, fx_extract_fn extract, void *extract_ctx) {
	if (h->survey)
		return;
	size_t xml_len;
	unsigned char *xml = sample("XMLSmall", &xml_len);
	for (const char *const *b = benches_embed; *b; b++) {
		if (!gate(h, *b))
			continue;
		size_t host_len;
		unsigned char *host = sample(input_of(*b), &host_len);
		arm_crash_bench(h, *b);
		fx_bytes out = {0};
		fx_err e = {{0}};
		unsigned long long t = now_ns();
		int r = f(ctx, host, host_len, xml, xml_len, &out, &e);
		unsigned long long first = now_ns() - t;
		if (r != 0) {
			fail_line(h, *b, first, e.msg, note);
		} else {
			dump_embed(h, *b, &out);
			fx_bytes back = {0};
			fx_err e2 = {{0}};
			int r2 = extract(extract_ctx, out.data, out.len, &back, &e2);
			fx_bytes_release(&out);
			if (r2 != 0) {
				char msg[FX_ERR_MAX + 64];
				snprintf(msg, sizeof msg, "embedded PDF does not round-trip: %s", e2.msg);
				fail_line(h, *b, first, msg, note);
			} else if (!fx_looks_like_cii(back.data, back.len)) {
				fx_bytes_release(&back);
				fail_line(h, *b, first, "embedded PDF does not round-trip: no CII invoice read back", note);
			} else {
				fx_bytes_release(&back);
				embed_call c = {f, ctx, host, xml, host_len, xml_len};
				measure(h, *b, first, note, (op_call){embed_once, &c});
			}
		}
		disarm();
		free(host);
	}
	free(xml);
}

/* ---------------------------------------------------------------- check_pdf */

typedef struct {
	fx_check_fn f;
	fx_free_fn free_report;
	void *ctx;
	const unsigned char *data;
	size_t len;
} check_call;

static int check_once(void *self) {
	check_call *c = self;
	void *rep = NULL;
	fx_err e;
	int r = c->f(c->ctx, c->data, c->len, &rep, &e);
	fx_sink += (size_t)(uintptr_t)rep;
	if (r == 0 && c->free_report)
		c->free_report(c->ctx, rep);
	return r;
}

void fx_check_pdf(fx_harness *h, const char *note, fx_check_fn f, fx_free_fn free_report, void *ctx) {
	if (h->survey)
		return;
	for (const char *const *b = benches_check; *b; b++) {
		if (!gate(h, *b))
			continue;
		size_t len;
		unsigned char *data = sample(input_of(*b), &len);
		arm_crash_bench(h, *b);
		void *rep = NULL;
		fx_err e = {{0}};
		unsigned long long t = now_ns();
		int r = f(ctx, data, len, &rep, &e);
		unsigned long long first = now_ns() - t;
		if (r != 0) {
			fail_line(h, *b, first, e.msg, note);
		} else {
			if (free_report)
				free_report(ctx, rep);
			check_call c = {f, free_report, ctx, data, len};
			measure(h, *b, first, note, (op_call){check_once, &c});
		}
		disarm();
		free(data);
	}
}

/* ---------------------------------------------------------------- entry point */

static void *worker(void *arg) {
	fx_harness *h = arg;
	h->body(h, h->ctx);
	return NULL;
}

static int mkdir_p(const char *path) {
	char *p = strdup(path);
	for (char *s = p + 1; *s; s++) {
		if (*s == '/') {
			*s = 0;
			mkdir(p, 0755);
			*s = '/';
		}
	}
	int r = mkdir(p, 0755);
	free(p);
	return r == 0 || errno == EEXIST ? 0 : -1;
}

int fx_run(const char *lang, fx_lib lib, int argc, char **argv, void (*body)(fx_harness *h, void *ctx), void *ctx) {
	int survey = 0, resources = 0;
	if (argc > 1 && strcmp(argv[1], "bench") == 0) {
		survey = 0;
	} else if (argc > 1 && strcmp(argv[1], "survey") == 0) {
		survey = 1;
	} else if (argc == 3 && strcmp(argv[1], "resources") == 0) {
		/* The resources pass: this process runs one benchmark exactly as
		 * bench does, and its record adds the peak RSS of the process. */
		resources = 1;
	} else {
		fprintf(stderr, "usage: %s bench|survey [BenchName...] | resources BenchName\n", argv[0]);
		return 2;
	}
	static fx_harness h;
	h.lang = lang;
	h.lib = lib;
	h.survey = survey;
	h.resources = resources;
	if (argc > 2) {
		h.only = argv + 2;
		h.nonly = argc - 2;
	}
	const char *count = getenv("COUNT");
	h.count = count && atoi(count) > 0 ? atoi(count) : 3;
	const char *bt = getenv("BENCHTIME");
	h.benchtime = bt && strtod(bt, NULL) > 0 ? strtod(bt, NULL) : 1.0;

	const char *outv = getenv("OUT");
	char dir[4096];
	snprintf(dir, sizeof dir, "%s/%s-%s", outv && *outv ? outv : "results", lang, resources ? "res-parts" : "parts");
	if (mkdir_p(dir) != 0) {
		fprintf(stderr, "create %s: %s\n", dir, strerror(errno));
		return 1;
	}
	char path[4300];
	const char *kind = survey ? "survey" : resources ? "resources" : "bench";
	snprintf(path, sizeof path, "%s/%s_%s.jsonl", dir, kind, lib.lib);
	/* A run limited to some benchmarks (always the case for resources)
	 * appends to the part file. */
	h.out = fopen(path, h.only ? "a" : "w");
	if (!h.out) {
		fprintf(stderr, "open %s: %s\n", path, strerror(errno));
		return 1;
	}
	crash_fd = fileno(h.out);
	int sigs[] = {SIGSEGV, SIGBUS, SIGILL, SIGFPE, SIGABRT};
	for (size_t i = 0; i < sizeof sigs / sizeof sigs[0]; i++)
		signal(sigs[i], on_crash);

	if (!resources) /* run.sh prints "== resources <lib>" once for its processes */
		fprintf(stderr, "== %s %s (%s@%s)\n", kind, lib.lib, lib.pkg, lib.version);
	h.body = body;
	h.ctx = ctx;
	pthread_attr_t attr;
	pthread_attr_init(&attr);
	pthread_attr_setstacksize(&attr, STACK_BYTES);
	pthread_t t;
	if (pthread_create(&t, &attr, worker, &h) != 0) {
		fprintf(stderr, "spawn worker thread\n");
		return 1;
	}
	pthread_join(t, NULL);
	pthread_attr_destroy(&attr);
	fclose(h.out);
	return 0;
}
