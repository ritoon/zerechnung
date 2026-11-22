// Timing and survey harness, a port of bench/ecosystem/common/harness.go to
// the rules of ../PROTOCOL.md: check the result once before timing (that call
// is first_call_ns), "unsupported" when the library cannot do the operation,
// "fail" when it throws or returns a wrong result, then N found by steps
// 1, 2, 5, 10, 20, 50… until one series lasts BENCHTIME seconds, then COUNT
// timed series of N calls on the monotonic clock, with the process CPU time
// of each series (cpu_ns_per_op). One thread, no forced GC.
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));

export const LANG = 'typescript';
export const COUNT = Number(process.env.COUNT || 3);
export const BENCHTIME = Number(process.env.BENCHTIME || 1);

// Several libraries log on every call (fin.cx prints four lines per
// extraction, stackforge warns about the PDF/A output intent). Logging is not
// the operation being measured and would time the terminal, so the console is
// silenced in the measuring processes; results go straight to stdout.
const out = process.stdout.write.bind(process.stdout);
export function silenceConsole() {
	for (const k of ['log', 'info', 'warn', 'debug', 'error', 'trace', 'dir', 'table']) console[k] = () => {};
}
export function emit(obj) {
	out(JSON.stringify(obj) + '\n');
}

// Read from node_modules directly: some packages do not export package.json.
export function pkgVersion(pkg) {
	return JSON.parse(readFileSync(join(here, '..', 'node_modules', pkg, 'package.json'), 'utf8')).version;
}

const isThenable = (v) => v != null && typeof v.then === 'function';
const now = process.hrtime.bigint;
// Process CPU time in microseconds: user + system of every thread of the
// process (main thread, V8 GC and compiler threads, libuv pool).
const cpuNow = () => {
	const u = process.cpuUsage();
	return u.user + u.system;
};
export let sink; // keeps results alive so that no call is optimised away

export function errMessage(e) {
	if (e == null) return String(e);
	const m = e instanceof Error ? `${e.name === 'Error' ? '' : e.name + ': '}${e.message}` : String(e);
	return m.replace(/\s+/g, ' ').trim().slice(0, 400);
}

export class Unsupported extends Error {}
export class Wrong extends Error {}

// series runs n calls and returns the elapsed nanoseconds and the process CPU
// nanoseconds of the same window (CPU clock read once on each side of the
// wall-clock reads, outside them).
async function series(fn, n, isAsync) {
	let r;
	const c0 = cpuNow();
	const t0 = now();
	if (isAsync) for (let i = 0; i < n; i++) r = await fn();
	else for (let i = 0; i < n; i++) r = fn();
	const dt = now() - t0;
	const dc = cpuNow() - c0;
	sink = r;
	return [Number(dt), dc * 1000];
}

const STEPS = [1, 2, 5];
function* steps() {
	for (let m = 1; ; m *= 10) for (const s of STEPS) yield s * m;
}

// measure times fn. check validates the first result (throws Wrong).
export async function measure(fn, check) {
	let first, r0;
	const t0 = now();
	let res = fn();
	const isAsync = isThenable(res);
	if (isAsync) res = await res;
	first = Number(now() - t0);
	r0 = res;
	if (check) await check(r0);
	sink = r0;

	const target = BENCHTIME * 1e9;
	let N = 1;
	for (const n of steps()) {
		const [dt] = await series(fn, n, isAsync);
		N = n;
		if (dt >= target) break;
	}
	const ns = [];
	const cpu = [];
	for (let i = 0; i < COUNT; i++) {
		const [dt, dc] = await series(fn, N, isAsync);
		ns.push(Math.round((dt / N) * 10) / 10);
		cpu.push(Math.round((dc / N) * 10) / 10);
	}
	return { ns_per_op: ns, cpu_ns_per_op: cpu, iters: N, first_call_ns: first };
}

// maxRSSBytes is the peak resident set size of this process
// (process.resourceUsage().maxRSS is in kilobytes on every OS).
export function maxRSSBytes() {
	return process.resourceUsage().maxRSS * 1024;
}

// record builds one bench_typescript.jsonl (or resources_typescript.jsonl) line.
export function record(adapter, bench, fields) {
	return {
		lang: LANG,
		lib: adapter.lib,
		pkg: adapter.pkg,
		version: pkgVersion(adapter.pkg),
		bench,
		status: fields.status,
		ns_per_op: fields.ns_per_op ?? [],
		cpu_ns_per_op: fields.cpu_ns_per_op ?? [],
		iters: fields.iters ?? 0,
		first_call_ns: fields.first_call_ns ?? null,
		note: fields.note ?? '',
	};
}
