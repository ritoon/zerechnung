package bench.harness;

import java.io.IOException;
import java.io.PrintStream;
import java.io.UncheckedIOException;
import java.io.Writer;
import java.lang.management.ManagementFactory;
import java.math.BigDecimal;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.nio.file.StandardOpenOption;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.function.Consumer;
import java.util.function.Function;
import java.util.stream.Stream;

/**
 * Shared harness of the Java benchmarks, the Java mirror of
 * {@code rust/harness/src/lib.rs}, {@code bench/ecosystem/common} and
 * {@code bench/polyglot/PROTOCOL.md}.
 *
 * <p>Each library module has a {@code bench.Main} whose {@code main} calls
 * {@link #run} with its identity and a body that registers the operations in
 * the protocol's order: {@code extract}, {@code parse}, {@code validate},
 * {@code write}, {@code embed}, {@code checkPdf} (or {@code unsupported} for
 * each one the library cannot do).
 *
 * <p>{@code bench.Main bench} measures the operations and writes one JSON line
 * per benchmark to {@code $OUT/java-parts/bench_<lib>.jsonl}; {@code bench.Main
 * survey} runs the correctness surveys and writes
 * {@code $OUT/java-parts/survey_<lib>.jsonl}. {@code run.sh} merges the parts
 * into {@code bench_java.jsonl} and {@code survey_java.jsonl}. {@code bench.Main
 * resources <Bench>} measures like {@code bench}, silently, and writes the
 * record to {@code $OUT/java-res-parts/run_<lib>.jsonl} (truncated): {@code
 * run.sh resources} starts one JVM per benchmark under {@code /usr/bin/time -l}
 * and adds the peak RSS of that JVM to the record.
 *
 * <p>Rules, as in the Go and Rust harnesses: a first call outside the
 * measurement checks the result ({@code fail} with the message when it is wrong
 * or throws, {@code unsupported} when the library cannot do the operation),
 * then N is found by steps 1, 2, 5, 10, 20, 50… until one series of N calls
 * lasts at least {@code BENCHTIME} seconds, then {@code COUNT} series of N calls
 * are timed with {@link System#nanoTime()}. Every result goes to a volatile
 * sink so that the JIT cannot drop the call as dead code. One thread runs the
 * operations; the JVM's own compiler and GC threads are not under our control,
 * but their work is in {@code cpu_ns_per_op}: the process CPU time (user +
 * system, every thread) over the same window as each timed series, divided by N.
 */
public final class Harness {

    /** Stack of the thread every operation runs on: the official validation
     *  stylesheets recurse deeply (the Rust harness uses the same size). */
    private static final long STACK_BYTES = 512L * 1024 * 1024;

    /** Who is measured. {@code lib}: short name used in the results;
     *  {@code pkg}: the Maven artifact(s) the adapter calls; {@code version}:
     *  the exact version pinned in the module's pom.xml. */
    public record Lib(String lib, String pkg, String version) {
    }

    /** The six operation families of the protocol. */
    public enum Op {
        EXTRACT("extract", "Extract_Small", "Extract_Large", "Extract_Minimum", "Extract_Extended"),
        PARSE("parse", "Parse_Small", "Parse_Lines", "Parse_Large"),
        VALIDATE("validate", "ParseValidate_Small", "ParseValidate_Lines", "ParseValidate_Large"),
        WRITE(null, "Write_Small", "Write_Lines"),
        EMBED(null, "Embed_Small", "EmbedClassic_Small"),
        CHECK_PDF(null, "CheckPDF_Small", "CheckPDF_Large");

        private final String survey;
        private final List<String> benches;

        Op(String survey, String... benches) {
            this.survey = survey;
            this.benches = List.of(benches);
        }

        public List<String> benches() {
            return benches;
        }

        public String survey() {
            return survey;
        }
    }

    /** What a validation found, read off the library's report outside the
     *  timing: the number of errors (0 = accepted) and their rule ids. */
    public record Findings(int errors, List<String> rules) {
        public static Findings of(List<String> rules) {
            return new Findings(rules.size(), List.copyOf(rules));
        }
    }

    /** A library call; any exception or error becomes a {@code fail}. */
    @FunctionalInterface
    public interface Fn<A, R> {
        R apply(A a) throws Exception;
    }

    /** A library call with two inputs (host PDF and XML). */
    @FunctionalInterface
    public interface Fn2<A, B, R> {
        R apply(A a, B b) throws Exception;
    }

    @FunctionalInterface
    private interface Call<T> {
        T call() throws Exception;
    }

    @FunctionalInterface
    private interface Check<T> {
        void check(T t) throws Exception;
    }

    /** A value built once, on first use, that remembers its failure: the
     *  Java counterpart of {@code OnceLock<Result<T, String>>}. */
    public static final class Lazy<T> {
        private final Call<T> init;
        private boolean done;
        private T value;
        private Exception error;

        private Lazy(Call<T> init) {
            this.init = init;
        }

        public static <T> Lazy<T> of(Fn<Void, T> init) {
            return new Lazy<>(() -> init.apply(null));
        }

        public T get() throws Exception {
            if (!done) {
                try {
                    value = init.call();
                } catch (Exception e) {
                    error = e;
                } catch (Throwable t) {
                    error = new Exception(describe(t), t);
                }
                done = true;
            }
            if (error != null) {
                throw error;
            }
            return value;
        }
    }

    // ------------------------------------------------------------ corpus

    /** {@code bench/ecosystem}, where the corpus and the host PDFs live
     *  ({@code FX_ECOSYSTEM}, default {@code ../../ecosystem} from java/). */
    public static Path ecosystem() {
        String v = System.getenv("FX_ECOSYSTEM");
        return Path.of(v != null && !v.isEmpty() ? v : "../../ecosystem").toAbsolutePath().normalize();
    }

    /** Root of the ZUGFeRD/corpus checkout ({@code FX_CORPUS} overrides it, as in Go). */
    public static Path corpus() {
        String v = System.getenv("FX_CORPUS");
        return v != null && !v.isEmpty() ? Path.of(v).toAbsolutePath().normalize() : ecosystem().resolve("corpus");
    }

    /** Where {@code run.sh setup} unpacks the official artefacts
     *  ({@code FX_ARTEFACTS}, default {@code .artefacts} from java/). */
    public static Path artefacts() {
        String v = System.getenv("FX_ARTEFACTS");
        return Path.of(v != null && !v.isEmpty() ? v : ".artefacts").toAbsolutePath().normalize();
    }

    private static byte[] load(Path p) {
        try {
            return Files.readAllBytes(p);
        } catch (IOException e) {
            throw new UncheckedIOException("load " + p, e);
        }
    }

    /** The bytes of one protocol input ({@code XMLSmall}, {@code PDFLarge}…),
     *  for adapters that build a model by hand from it. */
    public static byte[] sample(String name) {
        Path c = corpus();
        return switch (name) {
            case "PDFSmall" -> load(c.resolve("XML-Rechnung/FX/EN16931_Einfach.pdf"));
            case "PDFLarge" -> load(c.resolve("XML-Rechnung/FX/EN16931_Elektron.pdf"));
            case "PDFMinimum" -> load(c.resolve("ZUGFeRDv2/correct/FNFE-factur-x-examples/Facture_FR_MINIMUM.pdf"));
            case "PDFExtended" ->
                    load(c.resolve("ZUGFeRDv2/correct/intarsys/EXTENDED/zugferd_2p0_EXTENDED_Warenrechnung.pdf"));
            case "XMLSmall" -> load(c.resolve("XML-Rechnung/CII/EN16931_Einfach.cii.xml"));
            case "XMLLines" -> load(c.resolve("XML-Rechnung/CII/EN16931_Rabatte.cii.xml"));
            case "XMLLarge" -> load(c.resolve("XML-Rechnung/CII/XRECHNUNG_Elektron.cii.xml"));
            case "Host" -> load(ecosystem().resolve("host.pdf"));
            case "HostClassic" -> load(ecosystem().resolve("host_classic.pdf"));
            default -> throw new IllegalArgumentException(name);
        };
    }

    private static String inputOf(String bench) {
        return switch (bench) {
            case "Extract_Small", "CheckPDF_Small" -> "PDFSmall";
            case "Extract_Large", "CheckPDF_Large" -> "PDFLarge";
            case "Extract_Minimum" -> "PDFMinimum";
            case "Extract_Extended" -> "PDFExtended";
            case "Parse_Small", "ParseValidate_Small", "Write_Small" -> "XMLSmall";
            case "Parse_Lines", "ParseValidate_Lines", "Write_Lines" -> "XMLLines";
            case "Parse_Large", "ParseValidate_Large" -> "XMLLarge";
            case "Embed_Small" -> "Host";
            case "EmbedClassic_Small" -> "HostClassic";
            default -> throw new IllegalArgumentException(bench);
        };
    }

    private static List<Path> sorted(Path dir) {
        try (Stream<Path> s = Files.list(dir)) {
            return s.sorted().toList();
        } catch (IOException e) {
            return List.of();
        }
    }

    /** The 26 valid CII samples of the surveys. */
    public static List<Path> ciiFiles() {
        List<Path> out = new ArrayList<>();
        for (Path p : sorted(corpus().resolve("XML-Rechnung/CII"))) {
            String n = p.getFileName().toString();
            if (n.endsWith(".cii.xml")
                    && !n.equals("not_validating_full_invoice_based_onTest_EeISI_300_CENfullmodel.cii.xml")) {
                out.add(p);
            }
        }
        return out;
    }

    /** The 104 valid hybrid PDFs of the surveys: every {@code .pdf} under
     *  {@code ZUGFeRDv2/correct} (recursive) and {@code XML-Rechnung/FX/*.pdf}. */
    public static List<Path> pdfFiles() {
        List<Path> out = new ArrayList<>();
        walk(corpus().resolve("ZUGFeRDv2/correct"), out);
        for (Path p : sorted(corpus().resolve("XML-Rechnung/FX"))) {
            if (p.getFileName().toString().endsWith(".pdf") && Files.isRegularFile(p)) {
                out.add(p);
            }
        }
        return out;
    }

    private static void walk(Path dir, List<Path> out) {
        for (Path p : sorted(dir)) {
            if (Files.isDirectory(p)) {
                walk(p, out);
            } else if (p.getFileName().toString().endsWith(".pdf")) {
                out.add(p);
            }
        }
    }

    private static String rel(Path p) {
        Path c = corpus();
        return p.startsWith(c) ? c.relativize(p).toString() : p.toString();
    }

    /** The sanity check applied to every extracted or embedded XML. */
    public static boolean looksLikeCii(byte[] b) {
        return contains(b, "CrossIndustryInvoice".getBytes(StandardCharsets.US_ASCII))
                || contains(b, "CrossIndustryDocument".getBytes(StandardCharsets.US_ASCII));
    }

    private static boolean contains(byte[] hay, byte[] needle) {
        outer:
        for (int i = 0; i + needle.length <= hay.length; i++) {
            for (int j = 0; j < needle.length; j++) {
                if (hay[i + j] != needle[j]) {
                    continue outer;
                }
            }
            return true;
        }
        return false;
    }

    private static byte[] bytesOf(Object o) {
        if (o instanceof byte[] b) {
            return b;
        }
        if (o instanceof CharSequence s) {
            return s.toString().getBytes(StandardCharsets.UTF_8);
        }
        throw new IllegalStateException("unexpected result type " + (o == null ? "null" : o.getClass().getName()));
    }

    // ------------------------------------------------------------ errors

    /** "Class: message ← Class: message" over the cause chain. */
    public static String describe(Throwable t) {
        StringBuilder sb = new StringBuilder();
        int depth = 0;
        for (Throwable c = t; c != null && depth < 4; c = c.getCause(), depth++) {
            if (depth > 0) {
                if (c.getMessage() != null && sb.indexOf(c.getMessage()) >= 0) {
                    continue;
                }
                sb.append(" ← ");
            }
            sb.append(c.getClass().getSimpleName());
            if (c.getMessage() != null && !c.getMessage().isBlank()) {
                sb.append(": ").append(c.getMessage().strip());
            }
        }
        return sb.toString();
    }

    private static String shorten(String s) {
        s = s.replace('\n', ' ').replace('\r', ' ');
        if (s.codePointCount(0, s.length()) > 400) {
            return s.substring(0, s.offsetByCodePoints(0, 400)) + "…";
        }
        return s;
    }

    /** The outcome of one guarded call. */
    private record Outcome<T>(T value, String error) {
        boolean ok() {
            return error == null;
        }
    }

    private static <T> Outcome<T> guarded(Call<T> f) {
        try {
            return new Outcome<>(f.call(), null);
        } catch (Throwable t) {
            return new Outcome<>(null, describe(t));
        }
    }

    // ------------------------------------------------------------ output

    /** Dead-code sink: every timed result is stored here. */
    @SuppressWarnings("unused")
    private static volatile Object sink;

    /** Process CPU clock: {@code getProcessCpuTime()} is the user + system time
     *  of every thread of the JVM, in ns (getrusage on macOS, µs resolution),
     *  or -1 where the platform has none. */
    private static final com.sun.management.OperatingSystemMXBean OS =
            (com.sun.management.OperatingSystemMXBean) ManagementFactory.getOperatingSystemMXBean();

    private static final double[] NONE = new double[0];

    private enum Mode { BENCH, SURVEY }

    private final Lib lib;
    private final Mode mode;
    private final int count;
    private final long benchtimeNs;
    private final Writer out;
    private final List<String> only;
    /** {@code resources}: run.sh prints the progress line, with the peak RSS. */
    private final boolean quiet;
    private final Map<String, String> skipped = new LinkedHashMap<>();
    private static final PrintStream LOG = System.err;

    private Harness(Lib lib, Mode mode, int count, long benchtimeNs, Writer out, List<String> only, boolean quiet) {
        this.lib = lib;
        this.mode = mode;
        this.count = count;
        this.benchtimeNs = benchtimeNs;
        this.out = out;
        this.only = only;
        this.quiet = quiet;
    }

    private void line(Map<String, Object> v) {
        try {
            out.write(Json.encode(v));
            out.write('\n');
            out.flush();
        } catch (IOException e) {
            throw new UncheckedIOException("write results", e);
        }
    }

    /** Values rounded to 0.1 ns; NaN (no CPU clock) becomes null. */
    private static List<Object> rounded(double[] xs) {
        List<Object> out = new ArrayList<>();
        for (double x : xs) {
            out.add(Double.isNaN(x) ? null : Math.round(x * 10.0) / 10.0);
        }
        return out;
    }

    private void benchLine(String bench, String status, double[] ns, double[] cpu, long iters, long first,
                           String note) {
        Map<String, Object> v = new LinkedHashMap<>();
        v.put("lang", "java");
        v.put("lib", lib.lib());
        v.put("pkg", lib.pkg());
        v.put("version", lib.version());
        v.put("bench", bench);
        v.put("status", status);
        v.put("ns_per_op", rounded(ns));
        v.put("cpu_ns_per_op", rounded(cpu));
        v.put("iters", iters);
        v.put("first_call_ns", first);
        v.put("note", note);
        line(v);
        if (quiet) {
            return;
        }
        String tag;
        if (status.equals("ok")) {
            double min = Arrays.stream(ns).min().orElse(Double.NaN);
            tag = String.format(Locale.ROOT, "%14.1f ns/op  (N=%d)", min, iters);
        } else {
            String s = shorten(note);
            tag = String.format("%14s  %s", status, s.length() > 100 ? s.substring(0, 100) : s);
        }
        LOG.printf(Locale.ROOT, "   %-22s %s%n", bench, tag);
    }

    private boolean wanted(String bench) {
        return only == null || only.contains(bench);
    }

    /** Whether {@code bench} is to be measured; a bench marked with
     *  {@link #skip} is recorded as unsupported instead. */
    private boolean gate(String bench) {
        if (!wanted(bench)) {
            return false;
        }
        String note = skipped.get(bench);
        if (note != null) {
            benchLine(bench, "unsupported", NONE, NONE, 0, 0, note);
            return false;
        }
        return true;
    }

    /** Marks one benchmark of an otherwise supported family as unsupported
     *  (call before registering the family). */
    public void skip(String bench, String note) {
        skipped.put(bench, note);
    }

    /** Marks an operation the library cannot perform. */
    public void unsupported(Op op, String note) {
        if (mode == Mode.BENCH) {
            for (String b : op.benches()) {
                if (wanted(b)) {
                    benchLine(b, "unsupported", NONE, NONE, 0, 0, note);
                }
            }
        }
    }

    /** Whether this run is the survey (to skip expensive setup in bench-only paths). */
    public boolean isSurvey() {
        return mode == Mode.SURVEY;
    }

    private static long step(int step) {
        long n = new long[] {1, 2, 5}[step % 3];
        for (int i = 0; i < step / 3; i++) {
            n *= 10;
        }
        return n;
    }

    /** Times {@code f} after the first call (already made, {@code first} ns) passed. */
    private <T> void measure(String bench, long first, String note, Call<T> f) {
        try {
            long n = 1;
            int step = 0;
            while (true) {
                long t = System.nanoTime();
                for (long i = 0; i < n; i++) {
                    sink = f.call();
                }
                if (System.nanoTime() - t >= benchtimeNs) {
                    break;
                }
                step++;
                n = step(step);
            }
            double[] ns = new double[count];
            double[] cpu = new double[count];
            for (int k = 0; k < count; k++) {
                // The CPU clock is read just outside the wall-clock window, so
                // that its cost stays out of ns_per_op.
                long c = OS.getProcessCpuTime();
                long t = System.nanoTime();
                for (long i = 0; i < n; i++) {
                    sink = f.call();
                }
                ns[k] = (double) (System.nanoTime() - t) / n;
                long used = OS.getProcessCpuTime() - c;
                cpu[k] = c < 0 ? Double.NaN : (double) used / n;
            }
            benchLine(bench, "ok", ns, cpu, n, first, note);
        } catch (Throwable e) {
            String msg = "during measurement: " + shorten(describe(e));
            benchLine(bench, "fail", NONE, NONE, 0, first, note.isEmpty() ? msg : msg + " — " + note);
        }
    }

    /** Runs the first call and either measures or records the failure. */
    private <T> void benchOne(String bench, String note, Check<T> check, Call<T> f) {
        long t = System.nanoTime();
        Outcome<T> r = guarded(f);
        long first = System.nanoTime() - t;
        String err = r.error();
        if (err == null) {
            Outcome<Object> c = guarded(() -> {
                check.check(r.value());
                return null;
            });
            err = c.error();
        }
        if (err == null) {
            measure(bench, first, note, f);
        } else {
            String msg = note.isEmpty() ? shorten(err) : shorten(err) + " — " + note;
            benchLine(bench, "fail", NONE, NONE, 0, first, msg);
        }
    }

    private void surveyLine(String op, int ok, int total, List<Object> failures, String note) {
        LOG.printf("   SURVEY %s: %d/%d%n", op, ok, total);
        Map<String, Object> v = new LinkedHashMap<>();
        v.put("lang", "java");
        v.put("lib", lib.lib());
        v.put("pkg", lib.pkg());
        v.put("version", lib.version());
        v.put("op", op);
        v.put("ok", ok);
        v.put("total", total);
        v.put("failures", failures);
        v.put("note", note);
        line(v);
    }

    private static Map<String, Object> obj(Object... kv) {
        Map<String, Object> m = new LinkedHashMap<>();
        for (int i = 0; i < kv.length; i += 2) {
            m.put((String) kv[i], kv[i + 1]);
        }
        return m;
    }

    /** PDF → bytes of the invoice XML. */
    public void extract(String note, Fn<byte[], byte[]> f) {
        if (mode == Mode.BENCH) {
            for (String b : Op.EXTRACT.benches()) {
                if (!gate(b)) {
                    continue;
                }
                byte[] data = sample(inputOf(b));
                Check<byte[]> check = x -> {
                    if (x == null || !looksLikeCii(x)) {
                        throw new IllegalStateException("extracted bytes are not a CII invoice");
                    }
                };
                benchOne(b, note, check, () -> f.apply(data));
            }
        } else {
            List<Path> files = pdfFiles();
            int ok = 0;
            List<Object> failures = new ArrayList<>();
            for (Path p : files) {
                byte[] data = load(p);
                Outcome<byte[]> r = guarded(() -> f.apply(data));
                if (r.ok() && r.value() != null && looksLikeCii(r.value())) {
                    ok++;
                } else if (r.ok()) {
                    failures.add(obj("file", rel(p), "error", "extracted bytes are not a CII invoice"));
                } else {
                    failures.add(obj("file", rel(p), "error", shorten(r.error())));
                }
            }
            surveyLine("extract", ok, files.size(), failures, note);
        }
    }

    /** XML → the library's model. */
    public <M> void parse(String note, Fn<byte[], M> f) {
        if (mode == Mode.BENCH) {
            for (String b : Op.PARSE.benches()) {
                if (!gate(b)) {
                    continue;
                }
                byte[] data = sample(inputOf(b));
                benchOne(b, note, m -> {
                    if (m == null) {
                        throw new IllegalStateException("no model returned");
                    }
                }, () -> f.apply(data));
            }
        } else {
            List<Path> files = ciiFiles();
            int ok = 0;
            List<Object> failures = new ArrayList<>();
            for (Path p : files) {
                byte[] data = load(p);
                Outcome<M> r = guarded(() -> f.apply(data));
                if (r.ok() && r.value() != null) {
                    ok++;
                } else {
                    failures.add(obj("file", rel(p), "error", r.ok() ? "no model returned" : shorten(r.error())));
                }
            }
            surveyLine("parse", ok, files.size(), failures, note);
        }
    }

    /** XML → complete validation. {@code f} is timed; {@code summarize} reads
     *  the number of errors and their rule ids off its result, outside the timing. */
    public <R> void validate(String note, Fn<byte[], R> f, Function<R, Findings> summarize) {
        if (mode == Mode.BENCH) {
            for (String b : Op.VALIDATE.benches()) {
                if (!gate(b)) {
                    continue;
                }
                byte[] data = sample(inputOf(b));
                benchOne(b, note, r -> summarize.apply(r), () -> f.apply(data));
            }
        } else {
            List<Path> files = ciiFiles();
            int flagged = 0;
            List<Object> failures = new ArrayList<>();
            for (Path p : files) {
                byte[] data = load(p);
                Outcome<Findings> r = guarded(() -> summarize.apply(f.apply(data)));
                if (r.ok() && r.value().errors() == 0) {
                    continue;
                }
                flagged++;
                if (r.ok()) {
                    failures.add(obj("file", rel(p), "errors", r.value().errors(), "rules", r.value().rules()));
                } else {
                    failures.add(obj("file", rel(p), "errors", 0, "rules", List.of(), "error", shorten(r.error())));
                }
            }
            surveyLine("validate", flagged, files.size(), failures, note);
        }
    }

    /** Model → CII XML, the model built once outside the timing by {@code parse}
     *  ({@code write} returns {@code byte[]} or a {@code CharSequence}). */
    public <M, O> void write(String note, Fn<byte[], M> parse, Fn<M, O> write) {
        if (mode != Mode.BENCH) {
            return;
        }
        for (String b : Op.WRITE.benches()) {
            if (!gate(b)) {
                continue;
            }
            byte[] data = sample(inputOf(b));
            Outcome<M> model = guarded(() -> parse.apply(data));
            if (!model.ok()) {
                benchLine(b, "fail", NONE, NONE, 0, 0, "parse: " + shorten(model.error()));
                continue;
            }
            M m = model.value();
            benchOne(b, note, o -> {
                if (!looksLikeCii(bytesOf(o))) {
                    throw new IllegalStateException("written bytes are not a CII invoice");
                }
            }, () -> write.apply(m));
        }
    }

    /** Host PDF + XML Small → Factur-X PDF; the result must read back (with
     *  {@code extract}) and yield a CII invoice. */
    public void embed(String note, Fn2<byte[], byte[], byte[]> f, Fn<byte[], byte[]> extract) {
        if (mode != Mode.BENCH) {
            return;
        }
        byte[] xml = sample("XMLSmall");
        for (String b : Op.EMBED.benches()) {
            if (!gate(b)) {
                continue;
            }
            byte[] host = sample(inputOf(b));
            benchOne(b, note, out -> {
                Outcome<byte[]> x = guarded(() -> extract.apply(out));
                if (!x.ok()) {
                    throw new IllegalStateException("embedded PDF does not round-trip: " + x.error());
                }
                if (x.value() == null || !looksLikeCii(x.value())) {
                    throw new IllegalStateException("embedded PDF does not round-trip: no CII invoice read back");
                }
            }, () -> f.apply(host, xml));
        }
    }

    /** Native complete check of a hybrid PDF (container + XML). */
    public <R> void checkPdf(String note, Fn<byte[], R> f) {
        if (mode != Mode.BENCH) {
            return;
        }
        for (String b : Op.CHECK_PDF.benches()) {
            if (!gate(b)) {
                continue;
            }
            byte[] data = sample(inputOf(b));
            benchOne(b, note, r -> {
                if (r == null) {
                    throw new IllegalStateException("no report returned");
                }
            }, () -> f.apply(data));
        }
    }

    // ------------------------------------------------------------ entry point

    /** Entry point of every library module: {@code bench.Main bench|survey|resources [Bench…]}. */
    public static void run(Lib lib, String[] args, Consumer<Harness> body) {
        String kind = args.length > 0 ? args[0] : "";
        Mode mode = switch (kind) {
            case "bench", "resources" -> Mode.BENCH;
            case "survey" -> Mode.SURVEY;
            default -> null;
        };
        if (mode == null) {
            LOG.println("usage: bench.Main bench|survey|resources [BenchName…]");
            System.exit(2);
            return;
        }
        boolean resources = kind.equals("resources");
        List<String> only = args.length > 1 ? List.of(args).subList(1, args.length) : null;
        String outEnv = System.getenv("OUT");
        Path root = Path.of(outEnv != null && !outEnv.isEmpty() ? outEnv : "results");
        // resources: one (lib, bench) per JVM, read back by run.sh right after.
        Path dir = root.resolve(resources ? "java-res-parts" : "java-parts");
        Path path = dir.resolve(resources ? "run_" + lib.lib() + ".jsonl" : kind + "_" + lib.lib() + ".jsonl");
        int count = parseInt(System.getenv("COUNT"), 3);
        double benchtime = parseSeconds(System.getenv("BENCHTIME"), 1.0);
        int[] status = {0};
        try {
            Files.createDirectories(dir);
            StandardOpenOption[] opts = only != null && !resources
                    ? new StandardOpenOption[] {StandardOpenOption.CREATE, StandardOpenOption.APPEND}
                    : new StandardOpenOption[] {StandardOpenOption.CREATE, StandardOpenOption.TRUNCATE_EXISTING,
                            StandardOpenOption.WRITE};
            try (Writer w = Files.newBufferedWriter(path, StandardCharsets.UTF_8, opts)) {
                if (!resources) {
                    LOG.printf("== %s %s (%s@%s)%n", kind, lib.lib(), lib.pkg(), lib.version());
                }
                Harness h = new Harness(lib, mode, count, (long) (benchtime * 1e9), w, only, resources);
                // First read of the CPU clock (native method binding) outside any measurement.
                OS.getProcessCpuTime();
                Thread worker = new Thread(null, () -> {
                    try {
                        body.accept(h);
                    } catch (Throwable t) {
                        LOG.println("== " + lib.lib() + ": harness body failed: " + describe(t));
                        t.printStackTrace(LOG);
                        status[0] = 1;
                    }
                }, "bench", STACK_BYTES);
                worker.start();
                worker.join();
            }
        } catch (IOException | InterruptedException e) {
            LOG.println("== " + lib.lib() + ": " + describe(e));
            status[0] = 1;
        }
        // Some libraries leave non-daemon threads behind (thread pools,
        // timers): exit explicitly once the results are written.
        System.exit(status[0]);
    }

    private static int parseInt(String v, int def) {
        try {
            return v == null || v.isEmpty() ? def : Integer.parseInt(v.trim());
        } catch (NumberFormatException e) {
            return def;
        }
    }

    private static double parseSeconds(String v, double def) {
        try {
            if (v == null || v.isEmpty()) {
                return def;
            }
            v = v.trim();
            if (v.endsWith("s")) {
                v = v.substring(0, v.length() - 1);
            }
            return Double.parseDouble(v);
        } catch (NumberFormatException e) {
            return def;
        }
    }

    /** Minimal JSON encoder for the result lines (no dependency). */
    static final class Json {
        private Json() {
        }

        static String encode(Object o) {
            StringBuilder sb = new StringBuilder();
            write(sb, o);
            return sb.toString();
        }

        private static void write(StringBuilder sb, Object o) {
            if (o == null) {
                sb.append("null");
            } else if (o instanceof String s) {
                string(sb, s);
            } else if (o instanceof Double d) {
                if (d.isNaN() || d.isInfinite()) {
                    sb.append("null");
                } else {
                    String s = BigDecimal.valueOf(d).toPlainString();
                    sb.append(s.contains(".") ? s : s + ".0");
                }
            } else if (o instanceof Number || o instanceof Boolean) {
                sb.append(o);
            } else if (o instanceof Map<?, ?> m) {
                sb.append('{');
                boolean first = true;
                for (Map.Entry<?, ?> e : m.entrySet()) {
                    if (!first) {
                        sb.append(',');
                    }
                    first = false;
                    string(sb, String.valueOf(e.getKey()));
                    sb.append(':');
                    write(sb, e.getValue());
                }
                sb.append('}');
            } else if (o instanceof Iterable<?> it) {
                sb.append('[');
                boolean first = true;
                for (Object x : it) {
                    if (!first) {
                        sb.append(',');
                    }
                    first = false;
                    write(sb, x);
                }
                sb.append(']');
            } else {
                string(sb, o.toString());
            }
        }

        private static void string(StringBuilder sb, String s) {
            sb.append('"');
            for (int i = 0; i < s.length(); i++) {
                char c = s.charAt(i);
                switch (c) {
                    case '"' -> sb.append("\\\"");
                    case '\\' -> sb.append("\\\\");
                    case '\n' -> sb.append("\\n");
                    case '\r' -> sb.append("\\r");
                    case '\t' -> sb.append("\\t");
                    default -> {
                        if (c < 0x20) {
                            sb.append(String.format(Locale.ROOT, "\\u%04x", (int) c));
                        } else {
                            sb.append(c);
                        }
                    }
                }
            }
            sb.append('"');
        }
    }
}
