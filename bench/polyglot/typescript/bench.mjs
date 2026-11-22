// node bench.mjs <lib> <Benchmark>              →  one line of bench_typescript.jsonl
// node bench.mjs --resources <lib> <Benchmark>  →  one line of resources_typescript.jsonl
//
// One process per library and benchmark, so that first_call_ns is the first
// call of the operation in a fresh process and no benchmark warms another.
// With --resources the line also carries max_rss_bytes, the peak resident
// memory of this process, read after the series.
import { pathToFileURL } from 'node:url';
import * as corpus from './lib/corpus.mjs';
import { emit, errMessage, maxRSSBytes, measure, record, silenceConsole, Unsupported, Wrong } from './lib/harness.mjs';
import { loadAdapter } from './adapters/index.mjs';
import { HAND_NOTE } from './lib/handmodels.mjs';

export const BENCHMARKS = {
	Extract_Small: { op: 'extract', input: corpus.pdfSmall },
	Extract_Large: { op: 'extract', input: corpus.pdfLarge },
	Extract_Minimum: { op: 'extract', input: corpus.pdfMinimum },
	Extract_Extended: { op: 'extract', input: corpus.pdfExtended },
	Parse_Small: { op: 'parse', input: corpus.xmlSmall },
	Parse_Lines: { op: 'parse', input: corpus.xmlLines },
	Parse_Large: { op: 'parse', input: corpus.xmlLarge },
	ParseValidate_Small: { op: 'validate', input: corpus.xmlSmall },
	ParseValidate_Lines: { op: 'validate', input: corpus.xmlLines },
	ParseValidate_Large: { op: 'validate', input: corpus.xmlLarge },
	Write_Small: { op: 'write', input: corpus.xmlSmall },
	Write_Lines: { op: 'write', input: corpus.xmlLines },
	Embed_Small: { op: 'embed', input: corpus.xmlSmall, host: corpus.hostPDF },
	EmbedClassic_Small: { op: 'embed', input: corpus.xmlSmall, host: corpus.hostPDFClassic },
	CheckPDF_Small: { op: 'check', input: corpus.pdfSmall },
	CheckPDF_Large: { op: 'check', input: corpus.pdfLarge },
};

function joinNotes(...parts) {
	return parts.filter((p) => p != null && p !== '').join(' ; ');
}

function countNote(r) {
	if (!r || typeof r.errors !== 'number') return '';
	const rules = [...new Set(r.rules ?? [])].slice(0, 6).join(', ');
	return `résultat : ${r.errors} erreur(s)${rules ? ' (' + rules + ')' : ''}`;
}

const checkCount = (r) => {
	if (!r || typeof r.errors !== 'number' || Number.isNaN(r.errors)) throw new Wrong(`résultat de validation illisible : ${JSON.stringify(r)?.slice(0, 200)}`);
};

export async function runBenchmark(adapter, bench) {
	const spec = BENCHMARKS[bench];
	if (!spec) throw new Error(`unknown benchmark ${bench}`);
	const op = spec.op;
	const opNote = adapter.notes?.[op] ?? '';
	const has = op === 'write' ? adapter.write != null : typeof adapter[op] === 'function';
	if (!has) return record(adapter, bench, { status: 'unsupported', note: adapter.unsupported?.[op] ?? '' });

	const sample = spec.input();
	let fn, check, extraNote = '';
	try {
		switch (op) {
			case 'extract':
				fn = () => adapter.extract(sample);
				check = (x) => {
					if (!corpus.looksLikeCII(x)) throw new Wrong('le XML extrait ne contient pas CrossIndustryInvoice');
				};
				break;
			case 'parse':
				fn = () => adapter.parse(sample);
				check = (m) => {
					if (m == null) throw new Wrong('le parseur ne renvoie rien');
				};
				break;
			case 'validate':
				fn = () => adapter.validate(sample);
				check = (r) => {
					checkCount(r);
					extraNote = countNote(r);
				};
				break;
			case 'check':
				fn = () => adapter.check(sample);
				check = (r) => {
					checkCount(r);
					extraNote = countNote(r);
				};
				break;
			case 'write': {
				const w = adapter.write;
				let model = await w.model(sample, bench); // built once, outside the measure
				if (w.handBuilt) extraNote = HAND_NOTE;
				fn = () => w.write(model);
				check = (x) => {
					if (!x || x.length === 0) throw new Wrong('sortie vide');
					if (!corpus.looksLikeCII(x)) throw new Wrong('la sortie n’est pas un XML CII');
				};
				break;
			}
			case 'embed': {
				const host = spec.host();
				fn = () => adapter.embed(host, sample);
				const reread = adapter.reextract ?? adapter.extract;
				check = async (pdf) => {
					if (!pdf || pdf.length === 0) throw new Wrong('PDF vide');
					let back;
					try {
						back = reread ? await reread({ name: 'out.pdf', bytes: Buffer.from(pdf) }) : await fallbackExtract(Buffer.from(pdf));
					} catch (e) {
						throw new Wrong(`le PDF produit ne se relit pas : ${errMessage(e)}`);
					}
					if (!corpus.looksLikeCII(back)) throw new Wrong('le PDF produit ne contient pas la facture');
					if (!reread) extraNote = 'relecture par @attestwire/en16931 (extractFacturX)';
				};
				break;
			}
		}
		const m = await measure(fn, check);
		return record(adapter, bench, { status: 'ok', ...m, note: joinNotes(opNote, extraNote) });
	} catch (e) {
		if (e instanceof Unsupported) return record(adapter, bench, { status: 'unsupported', note: joinNotes(e.message) });
		return record(adapter, bench, { status: 'fail', note: joinNotes(opNote, errMessage(e)) });
	}
}

// Embed round trip for libraries without an extractor of their own: another
// library of the same language, independent of pdf-lib.
async function fallbackExtract(pdf) {
	const { extractFacturX } = await import('@attestwire/en16931');
	return extractFacturX(new Uint8Array(pdf)).xml;
}

if (import.meta.url === pathToFileURL(process.argv[1]).href) {
	const resources = process.argv[2] === '--resources';
	const [lib, bench] = process.argv.slice(resources ? 3 : 2);
	if (process.argv[2] === '--list') {
		process.stdout.write(Object.keys(BENCHMARKS).join(' ') + '\n');
		process.exit(0);
	}
	if (process.argv[2] === '--failed') {
		// run.sh: the measuring process died without writing its line.
		const [, , , id, b, why] = process.argv;
		emit(record(await loadAdapter(id), b, { status: 'fail', note: why }));
		process.exit(0);
	}
	silenceConsole();
	const adapter = await loadAdapter(lib);
	// A library can kill the process from a stray promise or leave the top-level
	// await unsettled: the benchmark is then a "fail" line, never a missing one.
	let done = false;
	const crash = (why) => {
		if (done) return;
		done = true;
		emit(record(adapter, bench, { status: 'fail', note: joinNotes(adapter.notes?.[BENCHMARKS[bench]?.op], why) }));
	};
	process.on('uncaughtException', (e) => {
		crash(`exception non rattrapée : ${errMessage(e)}`);
		process.exit(0);
	});
	process.on('unhandledRejection', (e) => {
		crash(`promesse rejetée hors de l’appel : ${errMessage(e)}`);
		process.exit(0);
	});
	process.on('exit', (code) => crash(`processus terminé (code ${code}) sans résultat`));
	const r = await runBenchmark(adapter, bench);
	done = true;
	if (resources && r.status === 'ok') r.max_rss_bytes = maxRSSBytes();
	emit(r);
	process.exit(0);
}
