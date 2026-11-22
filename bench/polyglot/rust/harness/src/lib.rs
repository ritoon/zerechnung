//! Shared harness of the Rust benchmarks, the Rust mirror of
//! `bench/ecosystem/common` and of `bench/polyglot/PROTOCOL.md`.
//!
//! Each library crate is a binary whose `main` calls [`run`] with its identity
//! and a closure that registers the operations in the protocol's order:
//! `extract`, `parse`, `validate`, `write`, `embed`, `check_pdf` (or
//! `unsupported` for each one the library cannot do).
//!
//! `bench-<lib> bench` measures the operations and writes one JSON line per
//! benchmark to `$OUT/rust-parts/bench_<lib>.jsonl`; `bench-<lib> survey` runs
//! the correctness surveys and writes `$OUT/rust-parts/survey_<lib>.jsonl`.
//! `bench-<lib> resources <Bench>` runs that one benchmark as `bench` does and
//! appends its line, with the peak RSS of the process (`max_rss_bytes`, from
//! `getrusage` at the end of the series), to
//! `$OUT/rust-res-parts/resources_<lib>.jsonl`; `run.sh` starts one such
//! process per benchmark. `run.sh` merges the parts into `bench_rust.jsonl`,
//! `survey_rust.jsonl` and `resources_rust.jsonl`.
//!
//! Rules, as in the Go harness: a first call outside the measurement checks the
//! result (`fail` with the message when it is wrong or an error, `unsupported`
//! when the library cannot do the operation), then N is found by steps
//! 1, 2, 5, 10, 20, 50… until one series of N calls lasts at least `BENCHTIME`
//! seconds, then `COUNT` series of N calls are timed with `std::time::Instant`.
//! Next to it, the CPU time of the process (`getrusage(RUSAGE_SELF)`, user +
//! system, all threads) is read before and after each series:
//! `cpu_ns_per_op`, parallel to `ns_per_op`.

use std::fs::{self, File, OpenOptions};
use std::hint::black_box;
use std::io::Write as _;
use std::panic::{self, AssertUnwindSafe};
use std::path::{Path, PathBuf};
use std::sync::Mutex;
use std::time::{Duration, Instant};

use serde_json::{Value, json};

/// Stack of the worker thread every operation runs on: the official validation
/// stylesheets recurse deeply (erechnung-validator asks for 64 MiB).
const STACK_BYTES: usize = 512 * 1024 * 1024;

/// Who is measured.
#[derive(Debug, Clone, Copy)]
pub struct Lib {
    /// Short name used in the results (`en16931`).
    pub lib: &'static str,
    /// The crates.io package(s) the adapter calls.
    pub pkg: &'static str,
    /// The exact version pinned in Cargo.toml.
    pub version: &'static str,
}

/// The six operation families of the protocol.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Op {
    Extract,
    Parse,
    Validate,
    Write,
    Embed,
    CheckPdf,
}

impl Op {
    fn benches(self) -> &'static [&'static str] {
        match self {
            Op::Extract => &["Extract_Small", "Extract_Large", "Extract_Minimum", "Extract_Extended"],
            Op::Parse => &["Parse_Small", "Parse_Lines", "Parse_Large"],
            Op::Validate => &["ParseValidate_Small", "ParseValidate_Lines", "ParseValidate_Large"],
            Op::Write => &["Write_Small", "Write_Lines"],
            Op::Embed => &["Embed_Small", "EmbedClassic_Small"],
            Op::CheckPdf => &["CheckPDF_Small", "CheckPDF_Large"],
        }
    }

    fn survey(self) -> Option<&'static str> {
        match self {
            Op::Extract => Some("extract"),
            Op::Parse => Some("parse"),
            Op::Validate => Some("validate"),
            _ => None,
        }
    }
}

/// What a validation found, read off the library's report outside the timing.
#[derive(Debug, Clone, Default)]
pub struct Findings {
    /// Number of errors (0 = accepted).
    pub errors: usize,
    /// Rule identifiers of the errors, in the library's order.
    pub rules: Vec<String>,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum Mode {
    Bench,
    Survey,
}

// ---------------------------------------------------------------- corpus

/// `bench/ecosystem`, where the corpus and the host PDFs live.
pub fn ecosystem() -> PathBuf {
    if let Ok(v) = std::env::var("FX_ECOSYSTEM") {
        return PathBuf::from(v);
    }
    Path::new(env!("CARGO_MANIFEST_DIR")).join("../../../ecosystem")
}

/// Root of the ZUGFeRD/corpus checkout (`FX_CORPUS` overrides it, as in Go).
pub fn corpus() -> PathBuf {
    if let Ok(v) = std::env::var("FX_CORPUS") {
        return PathBuf::from(v);
    }
    ecosystem().join("corpus")
}

fn load(p: PathBuf) -> Vec<u8> {
    fs::read(&p).unwrap_or_else(|e| panic!("load {}: {e}", p.display()))
}

fn sample(name: &str) -> Vec<u8> {
    let c = corpus();
    match name {
        "PDFSmall" => load(c.join("XML-Rechnung/FX/EN16931_Einfach.pdf")),
        "PDFLarge" => load(c.join("XML-Rechnung/FX/EN16931_Elektron.pdf")),
        "PDFMinimum" => load(c.join("ZUGFeRDv2/correct/FNFE-factur-x-examples/Facture_FR_MINIMUM.pdf")),
        "PDFExtended" => load(c.join("ZUGFeRDv2/correct/intarsys/EXTENDED/zugferd_2p0_EXTENDED_Warenrechnung.pdf")),
        "XMLSmall" => load(c.join("XML-Rechnung/CII/EN16931_Einfach.cii.xml")),
        "XMLLines" => load(c.join("XML-Rechnung/CII/EN16931_Rabatte.cii.xml")),
        "XMLLarge" => load(c.join("XML-Rechnung/CII/XRECHNUNG_Elektron.cii.xml")),
        "Host" => load(ecosystem().join("host.pdf")),
        "HostClassic" => load(ecosystem().join("host_classic.pdf")),
        _ => unreachable!("{name}"),
    }
}

fn input_of(bench: &str) -> &'static str {
    match bench {
        "Extract_Small" | "CheckPDF_Small" => "PDFSmall",
        "Extract_Large" | "CheckPDF_Large" => "PDFLarge",
        "Extract_Minimum" => "PDFMinimum",
        "Extract_Extended" => "PDFExtended",
        "Parse_Small" | "ParseValidate_Small" | "Write_Small" => "XMLSmall",
        "Parse_Lines" | "ParseValidate_Lines" | "Write_Lines" => "XMLLines",
        "Parse_Large" | "ParseValidate_Large" => "XMLLarge",
        "Embed_Small" => "Host",
        "EmbedClassic_Small" => "HostClassic",
        _ => unreachable!("{bench}"),
    }
}

/// The 26 valid CII samples of the surveys.
pub fn cii_files() -> Vec<PathBuf> {
    let dir = corpus().join("XML-Rechnung/CII");
    let mut out: Vec<PathBuf> = fs::read_dir(&dir)
        .unwrap_or_else(|e| panic!("{}: {e}", dir.display()))
        .filter_map(|e| e.ok().map(|e| e.path()))
        .filter(|p| {
            let n = p.file_name().and_then(|n| n.to_str()).unwrap_or("");
            n.ends_with(".cii.xml")
                && n != "not_validating_full_invoice_based_onTest_EeISI_300_CENfullmodel.cii.xml"
        })
        .collect();
    out.sort();
    out
}

/// The 104 valid hybrid PDFs of the surveys: every `.pdf` under
/// `ZUGFeRDv2/correct` (recursive) and `XML-Rechnung/FX/*.pdf`.
pub fn pdf_files() -> Vec<PathBuf> {
    fn walk(dir: &Path, out: &mut Vec<PathBuf>) {
        let Ok(rd) = fs::read_dir(dir) else { return };
        let mut entries: Vec<PathBuf> = rd.filter_map(|e| e.ok().map(|e| e.path())).collect();
        entries.sort();
        for p in entries {
            if p.is_dir() {
                walk(&p, out);
            } else if p.extension().and_then(|e| e.to_str()) == Some("pdf") {
                out.push(p);
            }
        }
    }
    let mut out = Vec::new();
    walk(&corpus().join("ZUGFeRDv2/correct"), &mut out);
    let mut fx: Vec<PathBuf> = fs::read_dir(corpus().join("XML-Rechnung/FX"))
        .map(|rd| rd.filter_map(|e| e.ok().map(|e| e.path())).collect())
        .unwrap_or_default();
    fx.retain(|p| p.extension().and_then(|e| e.to_str()) == Some("pdf"));
    fx.sort();
    out.extend(fx);
    out
}

fn rel(p: &Path) -> String {
    p.strip_prefix(corpus()).unwrap_or(p).display().to_string()
}

/// The sanity check applied to every extracted or embedded XML.
pub fn looks_like_cii(b: &[u8]) -> bool {
    let has = |n: &[u8]| b.windows(n.len()).any(|w| w == n);
    has(b"CrossIndustryInvoice") || has(b"CrossIndustryDocument")
}

// ---------------------------------------------------------------- resources

fn rusage() -> libc::rusage {
    // SAFETY: getrusage fills the zeroed struct it is given.
    unsafe {
        let mut ru: libc::rusage = std::mem::zeroed();
        libc::getrusage(libc::RUSAGE_SELF, &mut ru);
        ru
    }
}

/// CPU time of the process, user + system, all its threads, in ns. It ticks in
/// microseconds, negligible over a series of `BENCHTIME` seconds.
fn cpu_ns() -> u128 {
    let ru = rusage();
    let us = |t: libc::timeval| t.tv_sec as u128 * 1_000_000 + t.tv_usec as u128;
    (us(ru.ru_utime) + us(ru.ru_stime)) * 1_000
}

/// Peak resident set size of the process (`ru_maxrss`: bytes on macOS,
/// kilobytes on Linux).
fn max_rss_bytes() -> u64 {
    let rss = rusage().ru_maxrss as u64;
    if cfg!(target_os = "macos") { rss } else { rss * 1024 }
}

fn median(v: &[f64]) -> f64 {
    let mut s = v.to_vec();
    s.sort_by(f64::total_cmp);
    match s.len() {
        0 => 0.0,
        n if n % 2 == 1 => s[n / 2],
        n => (s[n / 2 - 1] + s[n / 2]) / 2.0,
    }
}

// ---------------------------------------------------------------- output

static PANIC_MSG: Mutex<Option<String>> = Mutex::new(None);

/// Runs `f`, turning a panic into an error message.
fn guarded<T>(f: impl FnOnce() -> Result<T, String>) -> Result<T, String> {
    match panic::catch_unwind(AssertUnwindSafe(f)) {
        Ok(r) => r,
        Err(payload) => {
            let msg = payload
                .downcast_ref::<String>()
                .cloned()
                .or_else(|| payload.downcast_ref::<&str>().map(|s| s.to_string()))
                .or_else(|| PANIC_MSG.lock().ok().and_then(|m| m.clone()))
                .unwrap_or_else(|| "panic".to_owned());
            Err(format!("panic: {msg}"))
        }
    }
}

fn shorten(s: &str) -> String {
    let s = s.replace('\n', " ");
    if s.chars().count() > 400 {
        let cut: String = s.chars().take(400).collect();
        format!("{cut}…")
    } else {
        s
    }
}

/// Registers the operations of one library and writes their results.
pub struct Harness {
    lib: Lib,
    mode: Mode,
    /// The resources pass: one benchmark in this process (mode `Bench`), its
    /// record with `max_rss_bytes`.
    resources: bool,
    count: usize,
    benchtime: Duration,
    out: File,
    only: Option<Vec<String>>,
    skipped: Vec<(String, String)>,
}

impl Harness {
    fn line(&mut self, v: Value) {
        writeln!(self.out, "{v}").expect("write results");
        self.out.flush().ok();
    }

    /// One record: `ns` and `cpu` are the wall and CPU ns/op of the series
    /// (empty when nothing was timed). In the resources pass, the record of a
    /// measured benchmark also carries the peak RSS of the process, read here,
    /// after the series.
    fn bench_line(&mut self, bench: &str, status: &str, ns: &[f64], cpu: &[f64], iters: u64, first: u128, note: &str) {
        let round = |v: &[f64]| v.iter().map(|x| (x * 10.0).round() / 10.0).collect::<Vec<_>>();
        let rss = (self.resources && status == "ok").then(max_rss_bytes);
        let mut v = json!({
            "lang": "rust",
            "lib": self.lib.lib,
            "pkg": self.lib.pkg,
            "version": self.lib.version,
            "bench": bench,
            "status": status,
            "ns_per_op": round(ns),
            "cpu_ns_per_op": round(cpu),
            "iters": iters,
            "first_call_ns": first as u64,
            "note": note,
        });
        if let Some(rss) = rss {
            v["max_rss_bytes"] = json!(rss);
        }
        let tag = match (status, rss) {
            ("ok", Some(rss)) => format!(
                "{:>14.1} ns/op {:>14.1} cpu ns/op {:>9.1} MiB  (N={iters})",
                median(ns),
                median(cpu),
                rss as f64 / 1_048_576.0
            ),
            ("ok", None) => format!("{:>14.1} ns/op  (N={iters})", ns.iter().copied().fold(f64::INFINITY, f64::min)),
            (s, _) => format!("{s:>14}  {}", shorten(note).chars().take(100).collect::<String>()),
        };
        eprintln!("   {:<22} {tag}", bench);
        self.line(v);
    }

    fn wanted(&self, bench: &str) -> bool {
        self.only.as_ref().is_none_or(|o| o.iter().any(|b| b == bench))
    }

    /// Whether `bench` is to be measured; a bench marked with [`Harness::skip`]
    /// is recorded as unsupported instead.
    fn gate(&mut self, bench: &str) -> bool {
        if !self.wanted(bench) {
            return false;
        }
        if let Some((_, note)) = self.skipped.iter().find(|(b, _)| b == bench).cloned() {
            self.bench_line(bench, "unsupported", &[], &[], 0, 0, &note);
            return false;
        }
        true
    }

    /// Marks one benchmark of an otherwise supported family as unsupported
    /// (call before registering the family).
    pub fn skip(&mut self, bench: &str, note: &str) {
        self.skipped.push((bench.to_owned(), note.to_owned()));
    }

    /// Marks an operation the library cannot perform.
    pub fn unsupported(&mut self, op: Op, note: &str) {
        if self.mode == Mode::Bench {
            for b in op.benches() {
                if self.wanted(b) {
                    self.bench_line(b, "unsupported", &[], &[], 0, 0, note);
                }
            }
        }
    }

    /// Times `f` on `data` after the first call `first` (already made) passed.
    fn measure<T>(&mut self, bench: &str, first: u128, note: &str, mut f: impl FnMut() -> T) {
        let mut n: u64 = 1;
        let mut step = 0usize;
        loop {
            let t = Instant::now();
            for _ in 0..n {
                black_box(f());
            }
            if t.elapsed() >= self.benchtime {
                break;
            }
            step += 1;
            n = [1, 2, 5][step % 3] * 10u64.pow((step / 3) as u32);
        }
        let mut ns = Vec::with_capacity(self.count);
        let mut cpu = Vec::with_capacity(self.count);
        for _ in 0..self.count {
            let c = cpu_ns();
            let t = Instant::now();
            for _ in 0..n {
                black_box(f());
            }
            let wall = t.elapsed();
            let c = cpu_ns() - c;
            ns.push(wall.as_nanos() as f64 / n as f64);
            cpu.push(c as f64 / n as f64);
        }
        self.bench_line(bench, "ok", &ns, &cpu, n, first, note);
    }

    /// Runs the first call and either measures or records the failure.
    fn bench_one<T>(
        &mut self,
        bench: &str,
        note: &str,
        check: impl FnOnce(T) -> Result<(), String>,
        mut f: impl FnMut() -> Result<T, String>,
    ) {
        let t = Instant::now();
        let r = guarded(&mut f);
        let first = t.elapsed().as_nanos();
        match r.and_then(|v| guarded(|| check(v))) {
            Ok(()) => self.measure(bench, first, note, || f()),
            Err(e) => {
                let msg = if note.is_empty() { shorten(&e) } else { format!("{} — {note}", shorten(&e)) };
                self.bench_line(bench, "fail", &[], &[], 0, first, &msg);
            }
        }
    }

    fn survey_line(&mut self, op: &str, ok: usize, total: usize, failures: Vec<Value>, note: &str) {
        eprintln!("   SURVEY {op}: {ok}/{total}");
        let v = json!({
            "lang": "rust",
            "lib": self.lib.lib,
            "pkg": self.lib.pkg,
            "version": self.lib.version,
            "op": op,
            "ok": ok,
            "total": total,
            "failures": failures,
            "note": note,
        });
        self.line(v);
    }

    /// PDF → bytes of the invoice XML.
    pub fn extract(&mut self, note: &str, f: impl Fn(&[u8]) -> Result<Vec<u8>, String>) {
        match self.mode {
            Mode::Bench => {
                for b in Op::Extract.benches() {
                    if !self.gate(b) {
                        continue;
                    }
                    let data = sample(input_of(b));
                    let check = |x: Vec<u8>| {
                        if looks_like_cii(&x) { Ok(()) } else { Err("extracted bytes are not a CII invoice".into()) }
                    };
                    self.bench_one(b, note, check, || f(black_box(&data)));
                }
            }
            Mode::Survey => {
                let files = pdf_files();
                let (mut ok, mut failures) = (0, Vec::new());
                for p in &files {
                    let data = load(p.clone());
                    match guarded(|| f(&data)) {
                        Ok(x) if looks_like_cii(&x) => ok += 1,
                        Ok(_) => failures.push(json!({"file": rel(p), "error": "extracted bytes are not a CII invoice"})),
                        Err(e) => failures.push(json!({"file": rel(p), "error": shorten(&e)})),
                    }
                }
                self.survey_line("extract", ok, files.len(), failures, note);
            }
        }
    }

    /// XML → the library's model.
    pub fn parse<M>(&mut self, note: &str, f: impl Fn(&[u8]) -> Result<M, String>) {
        match self.mode {
            Mode::Bench => {
                for b in Op::Parse.benches() {
                    if !self.gate(b) {
                        continue;
                    }
                    let data = sample(input_of(b));
                    self.bench_one(b, note, |_m| Ok(()), || f(black_box(&data)));
                }
            }
            Mode::Survey => {
                let files = cii_files();
                let (mut ok, mut failures) = (0, Vec::new());
                for p in &files {
                    let data = load(p.clone());
                    match guarded(|| f(&data)) {
                        Ok(_) => ok += 1,
                        Err(e) => failures.push(json!({"file": rel(p), "error": shorten(&e)})),
                    }
                }
                self.survey_line("parse", ok, files.len(), failures, note);
            }
        }
    }

    /// XML → complete validation. `f` is timed; `summarize` reads the number
    /// of errors and their rule ids off its result, outside the timing.
    pub fn validate<R>(
        &mut self,
        note: &str,
        f: impl Fn(&[u8]) -> Result<R, String>,
        summarize: impl Fn(&R) -> Findings,
    ) {
        match self.mode {
            Mode::Bench => {
                for b in Op::Validate.benches() {
                    if !self.gate(b) {
                        continue;
                    }
                    let data = sample(input_of(b));
                    self.bench_one(b, note, |_r| Ok(()), || f(black_box(&data)));
                }
            }
            Mode::Survey => {
                let files = cii_files();
                let (mut flagged, mut failures) = (0, Vec::new());
                for p in &files {
                    let data = load(p.clone());
                    match guarded(|| f(&data).map(|r| summarize(&r))) {
                        Ok(fd) if fd.errors == 0 => {}
                        Ok(fd) => {
                            flagged += 1;
                            failures.push(json!({"file": rel(p), "errors": fd.errors, "rules": fd.rules}));
                        }
                        Err(e) => {
                            flagged += 1;
                            failures.push(json!({"file": rel(p), "errors": 0, "rules": [], "error": shorten(&e)}));
                        }
                    }
                }
                self.survey_line("validate", flagged, files.len(), failures, note);
            }
        }
    }

    /// Model → CII XML, the model built once outside the timing by `parse`.
    pub fn write<M, O: AsRef<[u8]>>(
        &mut self,
        note: &str,
        parse: impl Fn(&[u8]) -> Result<M, String>,
        write: impl Fn(&M) -> Result<O, String>,
    ) {
        if self.mode != Mode::Bench {
            return;
        }
        for b in Op::Write.benches() {
            if !self.gate(b) {
                continue;
            }
            let data = sample(input_of(b));
            let model = match guarded(|| parse(&data)) {
                Ok(m) => m,
                Err(e) => {
                    self.bench_line(b, "fail", &[], &[], 0, 0, &format!("parse: {}", shorten(&e)));
                    continue;
                }
            };
            let check = |o: O| {
                if looks_like_cii(o.as_ref()) { Ok(()) } else { Err("written bytes are not a CII invoice".into()) }
            };
            self.bench_one(b, note, check, || write(black_box(&model)));
        }
    }

    /// Host PDF + XML Small → Factur-X PDF; the result must read back (with
    /// `extract`) and yield a CII invoice.
    pub fn embed(
        &mut self,
        note: &str,
        f: impl Fn(&[u8], &[u8]) -> Result<Vec<u8>, String>,
        extract: impl Fn(&[u8]) -> Result<Vec<u8>, String>,
    ) {
        if self.mode != Mode::Bench {
            return;
        }
        let xml = sample("XMLSmall");
        for b in Op::Embed.benches() {
            if !self.gate(b) {
                continue;
            }
            let host = sample(input_of(b));
            let check = |out: Vec<u8>| match guarded(|| extract(&out)) {
                Ok(x) if looks_like_cii(&x) => Ok(()),
                Ok(_) => Err("embedded PDF does not round-trip: no CII invoice read back".into()),
                Err(e) => Err(format!("embedded PDF does not round-trip: {e}")),
            };
            self.bench_one(b, note, check, || f(black_box(&host), black_box(&xml)));
        }
    }

    /// Native complete check of a hybrid PDF (container + XML).
    pub fn check_pdf<R>(&mut self, note: &str, f: impl Fn(&[u8]) -> Result<R, String>) {
        if self.mode != Mode::Bench {
            return;
        }
        for b in Op::CheckPdf.benches() {
            if !self.gate(b) {
                continue;
            }
            let data = sample(input_of(b));
            self.bench_one(b, note, |_r| Ok(()), || f(black_box(&data)));
        }
    }

    /// Whether this run is the survey (to skip expensive setup in bench-only paths).
    pub fn is_survey(&self) -> bool {
        self.mode == Mode::Survey
    }

    #[doc(hidden)]
    pub fn survey_ops(op: Op) -> Option<&'static str> {
        op.survey()
    }
}

/// Entry point of every library binary: `bench-<lib> bench|survey [Bench…]`
/// or `bench-<lib> resources Bench`.
pub fn run(lib: Lib, body: impl FnOnce(&mut Harness) + Send + 'static) {
    let args: Vec<String> = std::env::args().collect();
    let (mode, resources) = match args.get(1).map(String::as_str) {
        Some("bench") => (Mode::Bench, false),
        Some("survey") => (Mode::Survey, false),
        // The resources pass: this process runs one benchmark exactly as
        // bench does, and its record adds the peak RSS of the process.
        Some("resources") if args.len() == 3 => (Mode::Bench, true),
        _ => {
            eprintln!("usage: {} bench|survey [BenchName…] | resources BenchName", args[0]);
            std::process::exit(2);
        }
    };
    let only = (args.len() > 2).then(|| args[2..].to_vec());
    let parts = if resources { "rust-res-parts" } else { "rust-parts" };
    let out_dir = PathBuf::from(std::env::var("OUT").unwrap_or_else(|_| "results".into())).join(parts);
    fs::create_dir_all(&out_dir).unwrap_or_else(|e| panic!("create OUT/{parts}: {e}"));
    let kind = match (mode, resources) {
        (_, true) => "resources",
        (Mode::Bench, _) => "bench",
        (Mode::Survey, _) => "survey",
    };
    let path = out_dir.join(format!("{kind}_{}.jsonl", lib.lib));
    // A run limited to some benchmarks (always the case for resources)
    // appends to the part file.
    let out = if only.is_some() {
        OpenOptions::new().create(true).append(true).open(&path)
    } else {
        File::create(&path)
    }
    .expect("open results file");
    let count = std::env::var("COUNT").ok().and_then(|v| v.parse().ok()).unwrap_or(3);
    let benchtime = std::env::var("BENCHTIME")
        .ok()
        .and_then(|v| v.trim_end_matches('s').parse::<f64>().ok())
        .unwrap_or(1.0);

    // Keep panics quiet: the message is reported in the results instead.
    panic::set_hook(Box::new(|info| {
        if let Ok(mut m) = PANIC_MSG.lock() {
            *m = Some(info.to_string());
        }
    }));

    if !resources {
        // run.sh prints "== resources <lib>" once for its processes.
        eprintln!("== {} {} ({}@{})", kind, lib.lib, lib.pkg, lib.version);
    }
    let mut h = Harness {
        lib,
        mode,
        resources,
        count,
        benchtime: Duration::from_secs_f64(benchtime),
        out,
        only,
        skipped: Vec::new(),
    };
    std::thread::Builder::new()
        .name("bench".into())
        .stack_size(STACK_BYTES)
        .spawn(move || body(&mut h))
        .expect("spawn worker")
        .join()
        .expect("worker panicked");
}
