/*
 * alloccount: counts the calls to the C allocator of a process, for the
 * allocation measure of harness.py (see ../PROTOCOL.md).
 *
 * Loaded into the measuring process with DYLD_INSERT_LIBRARIES (macOS) or
 * LD_PRELOAD (Linux), with PYTHONMALLOC=malloc so that every Python object
 * goes through malloc too. Between alloccount_start() and alloccount_stop(),
 * every call to malloc, calloc, realloc, posix_memalign or aligned_alloc
 * counts as one allocation of the size asked for, whoever makes it: the
 * interpreter, a C extension such as lxml (libxml2) or pikepdf (qpdf), or a
 * Rust one. Memory a library manages with mmap itself (the heap of a GraalVM
 * native image, for instance) is not seen.
 *
 * Built by run.sh setup:
 *   cc -O2 -shared -fPIC -o .alloc/alloccount.so alloccount.c
 */
#include <errno.h>
#include <stdatomic.h>
#include <stddef.h>
#include <stdlib.h>

static atomic_int on;
static atomic_ullong allocs, bytes;

static inline void note(size_t size)
{
	if (atomic_load_explicit(&on, memory_order_relaxed)) {
		atomic_fetch_add_explicit(&allocs, 1, memory_order_relaxed);
		atomic_fetch_add_explicit(&bytes, size, memory_order_relaxed);
	}
}

void alloccount_start(void)
{
	atomic_store(&allocs, 0);
	atomic_store(&bytes, 0);
	atomic_store(&on, 1);
}

void alloccount_stop(unsigned long long *n, unsigned long long *b)
{
	atomic_store(&on, 0);
	*n = atomic_load(&allocs);
	*b = atomic_load(&bytes);
}

#ifdef __APPLE__

/* dyld interposing: calls from every other image go to these functions,
 * whose own calls to malloc… reach the system allocator. */
static void *ac_malloc(size_t size)
{
	note(size);
	return malloc(size);
}

static void *ac_calloc(size_t n, size_t size)
{
	note(n * size);
	return calloc(n, size);
}

static void *ac_realloc(void *p, size_t size)
{
	note(size);
	return realloc(p, size);
}

static int ac_posix_memalign(void **p, size_t align, size_t size)
{
	note(size);
	return posix_memalign(p, align, size);
}

static void *ac_aligned_alloc(size_t align, size_t size)
{
	note(size);
	return aligned_alloc(align, size);
}

#define INTERPOSE(repl, orig)                                                        \
	__attribute__((used)) static const struct {                                      \
		const void *r, *o;                                                           \
	} interpose_##orig __attribute__((section("__DATA,__interpose"))) = {          \
		(const void *)repl, (const void *)orig}

INTERPOSE(ac_malloc, malloc);
INTERPOSE(ac_calloc, calloc);
INTERPOSE(ac_realloc, realloc);
INTERPOSE(ac_posix_memalign, posix_memalign);
INTERPOSE(ac_aligned_alloc, aligned_alloc);

#else

/* glibc: these definitions take precedence over the C library's, and call
 * its internal entry points. free is left to glibc. */
extern void *__libc_malloc(size_t);
extern void *__libc_calloc(size_t, size_t);
extern void *__libc_realloc(void *, size_t);
extern void *__libc_memalign(size_t, size_t);

void *malloc(size_t size)
{
	note(size);
	return __libc_malloc(size);
}

void *calloc(size_t n, size_t size)
{
	note(n * size);
	return __libc_calloc(n, size);
}

void *realloc(void *p, size_t size)
{
	note(size);
	return __libc_realloc(p, size);
}

int posix_memalign(void **p, size_t align, size_t size)
{
	note(size);
	*p = __libc_memalign(align, size);
	return *p ? 0 : ENOMEM;
}

void *aligned_alloc(size_t align, size_t size)
{
	note(size);
	return __libc_memalign(align, size);
}

#endif
