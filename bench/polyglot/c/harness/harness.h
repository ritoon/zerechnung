/*
 * Shared harness of the C and C++ benchmarks, the C mirror of
 * bench/polyglot/rust/harness/src/lib.rs and of bench/polyglot/PROTOCOL.md.
 *
 * Each library is one executable whose main() calls fx_run() with its
 * identity and a body that registers the operations in the protocol's order:
 * extract, parse, validate, write, embed, check_pdf (or fx_unsupported() for
 * each one the library cannot do).
 *
 * `bench-<lib> bench` measures the operations and writes one JSON line per
 * benchmark to $OUT/<lang>-parts/bench_<lib>.jsonl; `bench-<lib> survey` runs
 * the correctness surveys and writes $OUT/<lang>-parts/survey_<lib>.jsonl.
 * `bench-<lib> resources <Bench>` runs that one benchmark as bench does and
 * appends its line, with the peak RSS of the process (`max_rss_bytes`, from
 * getrusage at the end of the series), to
 * $OUT/<lang>-res-parts/resources_<lib>.jsonl; run.sh starts one such process
 * per benchmark. run.sh merges the parts into bench_<lang>.jsonl,
 * survey_<lang>.jsonl and resources_<lang>.jsonl.
 *
 * Rules, as in the Go and Rust harnesses: a first call outside the
 * measurement checks the result ("fail" with the message when it is wrong or
 * an error, "unsupported" when the library cannot do the operation), then N is
 * found by steps 1, 2, 5, 10, 20, 50... until one series of N calls lasts at
 * least BENCHTIME seconds, then COUNT series of N calls are timed with
 * clock_gettime: CLOCK_MONOTONIC, or on macOS CLOCK_UPTIME_RAW, the clock of
 * Rust's Instant there (CLOCK_MONOTONIC only ticks in microseconds).
 * Next to the wall clock, the CPU time of the process (getrusage
 * RUSAGE_SELF, user + system, all threads) is read before and after each
 * series: `cpu_ns_per_op`, parallel to `ns_per_op`.
 * Results are released inside the timed loop, as Rust drops them there.
 * With FX_DUMP_EMBED=<dir>, the PDF of each embed check call is kept in
 * <dir> for inspection.
 *
 * The C++ benchmarks use this same harness through harness.hpp.
 */
#ifndef FX_HARNESS_H
#define FX_HARNESS_H

#include <stddef.h>

#ifdef __cplusplus
extern "C" {
#endif

/* Who is measured. */
typedef struct {
	const char *lib;     /* short name used in the results ("mupdf") */
	const char *pkg;     /* the package the adapter calls */
	const char *version; /* the exact version built against */
} fx_lib;

/* The six operation families of the protocol. */
typedef enum { FX_EXTRACT, FX_PARSE, FX_VALIDATE, FX_WRITE, FX_EMBED, FX_CHECK_PDF } fx_op;

/* Bytes produced by an operation. `release(owner)` frees them; with a NULL
 * release, `free(owner)` is called (owner may be NULL for static data). */
typedef struct {
	const unsigned char *data;
	size_t len;
	void *owner;
	void (*release)(void *owner);
} fx_bytes;

/* Takes ownership of a malloc'ed buffer. */
void fx_bytes_malloced(fx_bytes *b, unsigned char *p, size_t len);
/* Copies `len` bytes into a new malloc'ed buffer. Returns -1 when out of memory. */
int fx_bytes_copy(fx_bytes *b, const void *p, size_t len);
void fx_bytes_release(fx_bytes *b);

/* Error message of a failed call. */
#define FX_ERR_MAX 2048
typedef struct {
	char msg[FX_ERR_MAX];
} fx_err;

/* Formats the message into `e` and returns -1, for `return fx_fail(e, ...)`. */
int fx_fail(fx_err *e, const char *fmt, ...)
#if defined(__GNUC__) || defined(__clang__)
	__attribute__((format(printf, 2, 3)))
#endif
	;

/* What a validation found, read off the library's report outside the timing. */
typedef struct {
	size_t errors; /* number of errors (0 = accepted) */
	size_t nrules;
	char **rules; /* rule identifiers of the errors, in the library's order */
} fx_findings;

void fx_findings_add(fx_findings *f, const char *rule);

/* Adapter callbacks. Each returns 0 on success, or -1 with a message in
 * `err`. `ctx` is the pointer given at registration. */
typedef int (*fx_extract_fn)(void *ctx, const unsigned char *pdf, size_t len, fx_bytes *out, fx_err *err);
typedef int (*fx_parse_fn)(void *ctx, const unsigned char *xml, size_t len, void **model, fx_err *err);
typedef int (*fx_validate_fn)(void *ctx, const unsigned char *xml, size_t len, void **report, fx_err *err);
typedef void (*fx_summarize_fn)(void *ctx, void *report, fx_findings *out);
typedef int (*fx_write_fn)(void *ctx, void *model, fx_bytes *out, fx_err *err);
typedef int (*fx_embed_fn)(void *ctx, const unsigned char *host, size_t host_len, const unsigned char *xml,
			   size_t xml_len, fx_bytes *out, fx_err *err);
typedef int (*fx_check_fn)(void *ctx, const unsigned char *pdf, size_t len, void **report, fx_err *err);
/* Frees a model or a report (may be NULL when there is nothing to free). */
typedef void (*fx_free_fn)(void *ctx, void *p);

typedef struct fx_harness fx_harness;

/* Entry point of every library executable: `bench-<lib> bench|survey [Bench...]`
 * or `bench-<lib> resources Bench`.
 * `lang` is "c" or "cpp". The body runs on a thread with a large stack. */
int fx_run(const char *lang, fx_lib lib, int argc, char **argv, void (*body)(fx_harness *h, void *ctx), void *ctx);

/* Marks one benchmark of an otherwise supported family as unsupported (call
 * before registering the family). */
void fx_skip(fx_harness *h, const char *bench, const char *note);
/* Marks an operation the library cannot perform. */
void fx_unsupported(fx_harness *h, fx_op op, const char *note);

/* PDF -> bytes of the invoice XML. */
void fx_extract(fx_harness *h, const char *note, fx_extract_fn f, void *ctx);
/* XML -> the library's model. */
void fx_parse(fx_harness *h, const char *note, fx_parse_fn f, fx_free_fn free_model, void *ctx);
/* XML -> complete validation; `summarize` reads the errors off the report
 * outside the timing. */
void fx_validate(fx_harness *h, const char *note, fx_validate_fn f, fx_summarize_fn summarize,
		 fx_free_fn free_report, void *ctx);
/* Model -> CII XML, the model built once outside the timing by `parse`. */
void fx_write(fx_harness *h, const char *note, fx_parse_fn parse, fx_free_fn free_model, fx_write_fn write,
	      void *ctx);
/* Host PDF + XML Small -> Factur-X PDF; the result must read back with
 * `extract` (called with `extract_ctx`) and yield a CII invoice. */
void fx_embed(fx_harness *h, const char *note, fx_embed_fn f, void *ctx, fx_extract_fn extract, void *extract_ctx);
/* Native complete check of a hybrid PDF (container + XML). */
void fx_check_pdf(fx_harness *h, const char *note, fx_check_fn f, fx_free_fn free_report, void *ctx);

/* Whether this run is the survey. */
int fx_is_survey(const fx_harness *h);

/* bench/ecosystem, where the corpus and the host PDFs live (FX_ECOSYSTEM). */
const char *fx_ecosystem(void);
/* Root of the ZUGFeRD/corpus checkout (FX_CORPUS). */
const char *fx_corpus(void);
/* Directory of the artefacts fetched by run.sh setup (FX_ARTEFACTS). */
const char *fx_artefacts(void);

/* The sanity check applied to every extracted or embedded XML. */
int fx_looks_like_cii(const unsigned char *b, size_t len);

/* Reads a whole file; returns NULL (and sets *len to 0) on error. */
unsigned char *fx_read_file(const char *path, size_t *len);

/* ---------------------------------------------------------------- Factur-X
 * Helpers shared by the embedding floors (qpdf, PoDoFo, MuPDF): the values
 * a Factur-X container declares, written once here. */

/* Fixed modification date of the embedded file (PROTOCOL: "date de
 * modification fixe"). */
#define FX_MOD_DATE "D:20260101000000Z"
#define FX_MOD_TIME 1767225600 /* the same, in seconds since 1970 */
#define FX_ATTACHMENT "factur-x.xml"

/* Factur-X conformance level of the invoice, from its guideline (BT-24):
 * "MINIMUM", "BASIC WL", "BASIC", "EN 16931", "EXTENDED" or "XRECHNUNG". */
const char *fx_facturx_level(const unsigned char *xml, size_t len);

/* /AFRelationship of the invoice for a level, as Factur-X 1.0.07 (§ 6.2.2)
 * and ZUGFeRD 2.3 recommend: "Data" for MINIMUM and BASIC WL, whose XML does
 * not carry the whole invoice, "Alternative" otherwise. A PDF name
 * without the slash. */
const char *fx_facturx_relationship(const char *level);

/* Writes into `buf` an XMP packet declaring PDF/A-3B and the Factur-X
 * extension schema (fx:DocumentType INVOICE, fx:DocumentFileName
 * factur-x.xml, fx:Version 1.0, fx:ConformanceLevel `level`). Returns the
 * packet length, or 0 when `cap` is too small. */
size_t fx_facturx_xmp(char *buf, size_t cap, const char *level);

/* Standard attachment names of the extraction floors, compared without case
 * (factur-x.xml, zugferd-invoice.xml, xrechnung.xml, order-x.xml, cii.xml,
 * and ZUGFeRD-invoice.xml of ZUGFeRD 1.0 through the case folding). */
int fx_is_invoice_name(const char *name);

#ifdef __cplusplus
}
#endif

#endif
