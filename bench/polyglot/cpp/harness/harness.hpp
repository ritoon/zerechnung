// C++ face of the shared C harness (../../c/harness/harness.h): the same
// timing loop (wall and CPU time), JSON lines, surveys and resources pass
// (`bench-<lib> resources <Bench>`), with C++ closures as adapters.
//
// Every adapter is a callable that returns its result by value and throws on
// error; the wrappers below turn the result into harness bytes or opaque
// reports and every exception into a "fail" message, so that a library error
// never ends the process. The closures are passed by reference: the harness
// runs the benchmarks before the registration call returns.
#pragma once

#include <cstdio>
#include <cstring>
#include <exception>
#include <memory>
#include <new>
#include <string>
#include <string_view>
#include <type_traits>
#include <utility>
#include <vector>

#include "harness.h"

namespace fx {

using Bytes = std::string_view;

// Runs f, turning any exception into a message in err; returns 0 on success.
template <class F> int guarded(fx_err *err, F &&f) noexcept {
	try {
		f();
		return 0;
	} catch (const std::exception &e) {
		return fx_fail(err, "%s", e.what());
	} catch (const std::string &s) {
		return fx_fail(err, "%s", s.c_str());
	} catch (const char *s) {
		return fx_fail(err, "%s", s);
	} catch (...) {
		return fx_fail(err, "unknown C++ exception");
	}
}

// Hands a result over to the harness without copying its bytes: the object
// moves to the heap and is deleted when the harness releases the bytes.
template <class T> void give(fx_bytes *out, T &&value) {
	using V = std::decay_t<T>;
	auto *p = new V(std::forward<T>(value));
	out->data = reinterpret_cast<const unsigned char *>(p->data());
	out->len = p->size();
	out->owner = p;
	out->release = [](void *o) { delete static_cast<V *>(o); };
}

// PDF -> invoice XML. f(Bytes) returns a contiguous byte container
// (std::string, std::vector<char>…).
template <class F> void extract(fx_harness *h, const char *note, F &f) {
	fx_extract(
		h, note,
		[](void *ctx, const unsigned char *p, size_t n, fx_bytes *out, fx_err *err) -> int {
			auto &fn = *static_cast<F *>(ctx);
			return guarded(err, [&] { give(out, fn(Bytes(reinterpret_cast<const char *>(p), n))); });
		},
		&f);
}

// Extract callback usable as the read-back of embed.
template <class F> fx_extract_fn extract_fn() {
	return [](void *ctx, const unsigned char *p, size_t n, fx_bytes *out, fx_err *err) -> int {
		auto &fn = *static_cast<F *>(ctx);
		return guarded(err, [&] { give(out, fn(Bytes(reinterpret_cast<const char *>(p), n))); });
	};
}

// XML -> model. f(Bytes) returns the model (any movable type).
template <class F> void parse(fx_harness *h, const char *note, F &f) {
	using M = decltype(f(Bytes()));
	fx_parse(
		h, note,
		[](void *ctx, const unsigned char *p, size_t n, void **model, fx_err *err) -> int {
			auto &fn = *static_cast<F *>(ctx);
			return guarded(err, [&] { *model = new M(fn(Bytes(reinterpret_cast<const char *>(p), n))); });
		},
		[](void *, void *m) { delete static_cast<M *>(m); }, &f);
}

// XML -> complete validation. f(Bytes) returns a report, summarize(report)
// returns fx::Findings outside the timing.
struct Findings {
	size_t errors = 0;
	std::vector<std::string> rules;
};

template <class F, class S> struct ValidateCtx {
	F *f;
	S *s;
};

template <class F, class S> void validate(fx_harness *h, const char *note, F &f, S &s) {
	using R = decltype(f(Bytes()));
	static ValidateCtx<F, S> c;
	c = {&f, &s};
	fx_validate(
		h, note,
		[](void *ctx, const unsigned char *p, size_t n, void **report, fx_err *err) -> int {
			auto &fn = *static_cast<ValidateCtx<F, S> *>(ctx)->f;
			return guarded(err, [&] { *report = new R(fn(Bytes(reinterpret_cast<const char *>(p), n))); });
		},
		[](void *ctx, void *report, fx_findings *out) {
			auto &sum = *static_cast<ValidateCtx<F, S> *>(ctx)->s;
			Findings fd = sum(*static_cast<R *>(report));
			out->errors = fd.errors;
			for (auto &r : fd.rules)
				fx_findings_add(out, r.c_str());
		},
		[](void *, void *r) { delete static_cast<R *>(r); }, &c);
}

// Model -> CII XML; the model is built once, outside the timing, by parse.
template <class P, class W> struct WriteCtx {
	P *parse;
	W *write;
};

template <class P, class W> void write(fx_harness *h, const char *note, P &parse, W &write) {
	using M = decltype(parse(Bytes()));
	static WriteCtx<P, W> c;
	c = {&parse, &write};
	fx_write(
		h, note,
		[](void *ctx, const unsigned char *p, size_t n, void **model, fx_err *err) -> int {
			auto &fn = *static_cast<WriteCtx<P, W> *>(ctx)->parse;
			return guarded(err, [&] { *model = new M(fn(Bytes(reinterpret_cast<const char *>(p), n))); });
		},
		[](void *, void *m) { delete static_cast<M *>(m); },
		[](void *ctx, void *model, fx_bytes *out, fx_err *err) -> int {
			auto &fn = *static_cast<WriteCtx<P, W> *>(ctx)->write;
			return guarded(err, [&] { give(out, fn(*static_cast<M *>(model))); });
		},
		&c);
}

// Host PDF + XML -> Factur-X PDF, read back with extract.
template <class F, class E> void embed(fx_harness *h, const char *note, F &f, E &extract) {
	fx_embed(
		h, note,
		[](void *ctx, const unsigned char *host, size_t hn, const unsigned char *xml, size_t xn, fx_bytes *out,
		   fx_err *err) -> int {
			auto &fn = *static_cast<F *>(ctx);
			return guarded(err, [&] {
				give(out, fn(Bytes(reinterpret_cast<const char *>(host), hn),
					     Bytes(reinterpret_cast<const char *>(xml), xn)));
			});
		},
		&f, extract_fn<E>(), &extract);
}

// Native complete check of a hybrid PDF. f(Bytes) returns a report.
template <class F> void check_pdf(fx_harness *h, const char *note, F &f) {
	using R = decltype(f(Bytes()));
	fx_check_pdf(
		h, note,
		[](void *ctx, const unsigned char *p, size_t n, void **report, fx_err *err) -> int {
			auto &fn = *static_cast<F *>(ctx);
			return guarded(err, [&] { *report = new R(fn(Bytes(reinterpret_cast<const char *>(p), n))); });
		},
		[](void *, void *r) { delete static_cast<R *>(r); }, &f);
}

inline std::string artefact(const char *rel) { return std::string(fx_artefacts()) + "/" + rel; }

// Body of a library executable: a closure taking the harness.
template <class B> int run(const fx_lib &lib, int argc, char **argv, B &&body) {
	using BB = std::decay_t<B>;
	BB b(std::forward<B>(body));
	return fx_run(
		"cpp", lib, argc, argv,
		[](fx_harness *h, void *ctx) {
			fx_err err{};
			if (guarded(&err, [&] { (*static_cast<BB *>(ctx))(h); }) != 0)
				std::fprintf(stderr, "   body aborted: %s\n", err.msg);
		},
		&b);
}

} // namespace fx
