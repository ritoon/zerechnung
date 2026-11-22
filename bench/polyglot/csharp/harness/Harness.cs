// Shared harness of the C# benchmarks, the C# mirror of bench/ecosystem/common,
// of rust/harness/src/lib.rs and of bench/polyglot/PROTOCOL.md.
//
// Each library project is a console program whose Main calls Harness.Run with
// its identity and a body that registers the operations in the protocol's
// order: Extract, Parse, Validate, Write, Embed, CheckPdf (or Unsupported for
// each one the library cannot do).
//
// `bench-<lib> bench` measures the operations and writes one JSON line per
// benchmark to $OUT/csharp-parts/bench_<lib>.jsonl; `bench-<lib> survey` runs
// the correctness surveys and writes $OUT/csharp-parts/survey_<lib>.jsonl.
// run.sh merges the parts into bench_csharp.jsonl and survey_csharp.jsonl.
// `bench-<lib> resources <Bench>` measures like bench, silently, and writes the
// record to $OUT/csharp-res-parts/run_<lib>.jsonl (truncated): `run.sh
// resources` starts one process per benchmark under /usr/bin/time -l and adds
// the peak RSS of that process to the record.
//
// Rules, as in the Go and Rust harnesses: a first call outside the measurement
// checks the result ("fail" with the message when it is wrong or throws,
// "unsupported" when the library cannot do the operation), then N is found by
// steps 1, 2, 5, 10, 20, 50... until one series of N calls lasts at least
// BENCHTIME seconds, then COUNT series of N calls are timed with
// Stopwatch.GetTimestamp(). One thread, no forced GC. cpu_ns_per_op: the
// process CPU time (user + system, every thread: GC, tiered JIT, thread pool)
// over the same window as each timed series, divided by N.

using System.Diagnostics;
using System.Globalization;
using System.Reflection;
using System.Runtime.CompilerServices;
using System.Text;
using System.Text.Encodings.Web;
using System.Text.Json;

namespace Bench;

/// <summary>Who is measured.</summary>
/// <param name="Name">Short name used in the results (zugferd-csharp).</param>
/// <param name="Pkg">The NuGet package(s) the adapter calls.</param>
/// <param name="Version">The exact version pinned in the project file.</param>
public sealed record Lib(string Name, string Pkg, string Version);

/// <summary>The six operation families of the protocol.</summary>
public enum Op { Extract, Parse, Validate, Write, Embed, CheckPdf }

/// <summary>What a validation found, read off the library's report outside the timing.</summary>
public sealed record Findings(int Errors, IReadOnlyList<string> Rules)
{
    public static Findings Of(IEnumerable<string> rules)
    {
        var list = rules.ToList();
        return new Findings(list.Count, list);
    }

    public static readonly Findings None = new(0, []);
}

/// <summary>The corpus and host PDFs of bench/ecosystem.</summary>
public static class Corpus
{
    private static string? _ecosystem;

    /// <summary>bench/ecosystem (FX_ECOSYSTEM overrides it).</summary>
    public static string Ecosystem => _ecosystem ??= FindEcosystem();

    /// <summary>Root of the ZUGFeRD/corpus checkout (FX_CORPUS overrides it, as in Go).</summary>
    public static string Root => Environment.GetEnvironmentVariable("FX_CORPUS") is { Length: > 0 } c
        ? c
        : Path.Combine(Ecosystem, "corpus");

    private static string FindEcosystem()
    {
        if (Environment.GetEnvironmentVariable("FX_ECOSYSTEM") is { Length: > 0 } e)
        {
            return e;
        }
        // Walk up from the binary to bench/, which holds ecosystem/host.pdf.
        for (var d = new DirectoryInfo(AppContext.BaseDirectory); d != null; d = d.Parent)
        {
            var candidate = Path.Combine(d.FullName, "ecosystem");
            if (File.Exists(Path.Combine(candidate, "host.pdf")))
            {
                return candidate;
            }
        }
        throw new DirectoryNotFoundException("bench/ecosystem not found: set FX_ECOSYSTEM");
    }

    /// <summary>csharp/.artefacts, where run.sh setup unpacks the official validation artefacts.</summary>
    public static string Artefacts
    {
        get
        {
            if (Environment.GetEnvironmentVariable("FX_ARTEFACTS") is { Length: > 0 } a)
            {
                return a;
            }
            for (var d = new DirectoryInfo(AppContext.BaseDirectory); d != null; d = d.Parent)
            {
                var candidate = Path.Combine(d.FullName, ".artefacts");
                if (Directory.Exists(candidate))
                {
                    return candidate;
                }
            }
            throw new DirectoryNotFoundException(".artefacts not found: run ./run.sh setup or set FX_ARTEFACTS");
        }
    }

    public static byte[] Load(string path) => File.ReadAllBytes(path);

    public static byte[] Sample(string name) => name switch
    {
        "PDFSmall" => Load(Path.Combine(Root, "XML-Rechnung/FX/EN16931_Einfach.pdf")),
        "PDFLarge" => Load(Path.Combine(Root, "XML-Rechnung/FX/EN16931_Elektron.pdf")),
        "PDFMinimum" => Load(Path.Combine(Root, "ZUGFeRDv2/correct/FNFE-factur-x-examples/Facture_FR_MINIMUM.pdf")),
        "PDFExtended" => Load(Path.Combine(Root, "ZUGFeRDv2/correct/intarsys/EXTENDED/zugferd_2p0_EXTENDED_Warenrechnung.pdf")),
        "XMLSmall" => Load(Path.Combine(Root, "XML-Rechnung/CII/EN16931_Einfach.cii.xml")),
        "XMLLines" => Load(Path.Combine(Root, "XML-Rechnung/CII/EN16931_Rabatte.cii.xml")),
        "XMLLarge" => Load(Path.Combine(Root, "XML-Rechnung/CII/XRECHNUNG_Elektron.cii.xml")),
        "Host" => Load(Path.Combine(Ecosystem, "host.pdf")),
        "HostClassic" => Load(Path.Combine(Ecosystem, "host_classic.pdf")),
        _ => throw new ArgumentException(name),
    };

    public static string InputOf(string bench) => bench switch
    {
        "Extract_Small" or "CheckPDF_Small" => "PDFSmall",
        "Extract_Large" or "CheckPDF_Large" => "PDFLarge",
        "Extract_Minimum" => "PDFMinimum",
        "Extract_Extended" => "PDFExtended",
        "Parse_Small" or "ParseValidate_Small" or "Write_Small" => "XMLSmall",
        "Parse_Lines" or "ParseValidate_Lines" or "Write_Lines" => "XMLLines",
        "Parse_Large" or "ParseValidate_Large" => "XMLLarge",
        "Embed_Small" => "Host",
        "EmbedClassic_Small" => "HostClassic",
        _ => throw new ArgumentException(bench),
    };

    /// <summary>The 26 valid CII samples of the surveys.</summary>
    public static List<string> CiiFiles()
    {
        var dir = Path.Combine(Root, "XML-Rechnung/CII");
        var files = Directory.GetFiles(dir)
            .Where(p =>
            {
                var n = Path.GetFileName(p);
                return n.EndsWith(".cii.xml", StringComparison.Ordinal)
                    && n != "not_validating_full_invoice_based_onTest_EeISI_300_CENfullmodel.cii.xml";
            })
            .ToList();
        files.Sort(StringComparer.Ordinal);
        return files;
    }

    /// <summary>
    /// The 104 valid hybrid PDFs of the surveys: every .pdf under ZUGFeRDv2/correct
    /// (recursive, sorted per directory as in Rust) and XML-Rechnung/FX/*.pdf.
    /// </summary>
    public static List<string> PdfFiles()
    {
        var output = new List<string>();
        void Walk(string dir)
        {
            if (!Directory.Exists(dir))
            {
                return;
            }
            var entries = Directory.GetFileSystemEntries(dir).ToList();
            entries.Sort(StringComparer.Ordinal);
            foreach (var p in entries)
            {
                if (Directory.Exists(p))
                {
                    Walk(p);
                }
                else if (Path.GetExtension(p) == ".pdf")
                {
                    output.Add(p);
                }
            }
        }
        Walk(Path.Combine(Root, "ZUGFeRDv2/correct"));
        var fx = Directory.GetFiles(Path.Combine(Root, "XML-Rechnung/FX"), "*.pdf").ToList();
        fx.Sort(StringComparer.Ordinal);
        output.AddRange(fx);
        return output;
    }

    public static string Rel(string path) => Path.GetRelativePath(Root, path);

    private static readonly byte[] Cii = "CrossIndustryInvoice"u8.ToArray();
    private static readonly byte[] Cid = "CrossIndustryDocument"u8.ToArray();

    /// <summary>The sanity check applied to every extracted or embedded XML.</summary>
    public static bool LooksLikeCii(ReadOnlySpan<byte> b) => b.IndexOf(Cii) >= 0 || b.IndexOf(Cid) >= 0;

    /// <summary>The same check on whatever an operation returned (bytes, string, stream).</summary>
    public static bool LooksLikeCii(object? o) => o switch
    {
        byte[] b => LooksLikeCii(b.AsSpan()),
        string s => s.Contains("CrossIndustryInvoice", StringComparison.Ordinal)
            || s.Contains("CrossIndustryDocument", StringComparison.Ordinal),
        MemoryStream ms => LooksLikeCii(ms.ToArray().AsSpan()),
        System.Xml.XmlNode n => LooksLikeCii(n.OuterXml),
        System.Xml.Linq.XNode x => LooksLikeCii(x.ToString()),
        _ => false,
    };
}

/// <summary>Keeps results alive so that the JIT cannot drop the measured calls.</summary>
public static class Sink
{
    private static object? _last;
    private static long _count;

    [MethodImpl(MethodImplOptions.NoInlining)]
    public static void Consume<T>(T value)
    {
        if (value is null)
        {
            _count++;
        }
        else if (typeof(T).IsValueType)
        {
            _count += value.GetHashCode() & 1;
        }
        else
        {
            _last = value;
        }
    }

    public static long Count => _count + (_last is null ? 0 : 1);
}

/// <summary>Registers the operations of one library and writes their results.</summary>
public sealed class Harness
{
    private enum Mode { Bench, Survey }

    private static readonly JsonWriterOptions JsonOptions = new()
    {
        Encoder = JavaScriptEncoder.UnsafeRelaxedJsonEscaping,
        Indented = false,
    };

    private readonly Lib _lib;
    private readonly Mode _mode;
    private readonly int _count;
    private readonly double _benchtimeNs;
    private readonly StreamWriter _out;
    private readonly List<string>? _only;
    /// <summary>resources: run.sh prints the progress line, with the peak RSS.</summary>
    private readonly bool _quiet;
    private readonly Dictionary<string, string> _skipped = new(StringComparer.Ordinal);

    private Harness(Lib lib, Mode mode, int count, double benchtimeSeconds, StreamWriter output, List<string>? only, bool quiet)
    {
        _lib = lib;
        _mode = mode;
        _count = count;
        _benchtimeNs = benchtimeSeconds * 1e9;
        _out = output;
        _only = only;
        _quiet = quiet;
    }

    /// <summary>Whether this run is the survey (to skip expensive setup in bench-only paths).</summary>
    public bool IsSurvey => _mode == Mode.Survey;

    private static string[] Benches(Op op) => op switch
    {
        Op.Extract => ["Extract_Small", "Extract_Large", "Extract_Minimum", "Extract_Extended"],
        Op.Parse => ["Parse_Small", "Parse_Lines", "Parse_Large"],
        Op.Validate => ["ParseValidate_Small", "ParseValidate_Lines", "ParseValidate_Large"],
        Op.Write => ["Write_Small", "Write_Lines"],
        Op.Embed => ["Embed_Small", "EmbedClassic_Small"],
        Op.CheckPdf => ["CheckPDF_Small", "CheckPDF_Large"],
        _ => [],
    };

    // ------------------------------------------------------------ output

    /// <summary>Message of an exception, unwrapped from reflection and aggregate wrappers.</summary>
    public static string Describe(Exception e)
    {
        while (e is TargetInvocationException or AggregateException && e.InnerException != null)
        {
            e = e.InnerException;
        }
        var msg = $"{e.GetType().Name}: {e.Message}";
        if (e.InnerException is { } inner)
        {
            msg += $" ({inner.GetType().Name}: {inner.Message})";
        }
        return msg;
    }

    private static string Shorten(string s)
    {
        s = s.Replace('\n', ' ').Replace('\r', ' ');
        return s.Length > 400 ? s[..400] + "…" : s;
    }

    private void Line(Action<Utf8JsonWriter> body)
    {
        using var ms = new MemoryStream();
        using (var w = new Utf8JsonWriter(ms, JsonOptions))
        {
            w.WriteStartObject();
            w.WriteString("lang", "csharp");
            w.WriteString("lib", _lib.Name);
            w.WriteString("pkg", _lib.Pkg);
            w.WriteString("version", _lib.Version);
            body(w);
            w.WriteEndObject();
        }
        _out.WriteLine(Encoding.UTF8.GetString(ms.ToArray()));
        _out.Flush();
    }

    /// <summary>Values rounded to 0.1 ns; NaN (no CPU clock) is written as null.</summary>
    private static void WriteRounded(Utf8JsonWriter w, string name, IReadOnlyList<double> xs)
    {
        w.WriteStartArray(name);
        foreach (var x in xs)
        {
            if (double.IsNaN(x))
            {
                w.WriteNullValue();
            }
            else
            {
                w.WriteNumberValue(Math.Round(x * 10.0) / 10.0);
            }
        }
        w.WriteEndArray();
    }

    private void BenchLine(string bench, string status, IReadOnlyList<double> ns, IReadOnlyList<double> cpu, long iters, long first, string note)
    {
        Line(w =>
        {
            w.WriteString("bench", bench);
            w.WriteString("status", status);
            WriteRounded(w, "ns_per_op", ns);
            WriteRounded(w, "cpu_ns_per_op", cpu);
            w.WriteNumber("iters", iters);
            w.WriteNumber("first_call_ns", first);
            w.WriteString("note", note);
        });
        if (_quiet)
        {
            return;
        }
        var tag = status == "ok"
            ? $"{ns.Min(),14:F1} ns/op  (N={iters})"
            : $"{status,14}  {new string(Shorten(note).Take(100).ToArray())}";
        Console.Error.WriteLine($"   {bench,-22} {tag}");
    }

    private void SurveyLine(string op, int ok, int total, List<Action<Utf8JsonWriter>> failures, string note)
    {
        Console.Error.WriteLine($"   SURVEY {op}: {ok}/{total}");
        Line(w =>
        {
            w.WriteString("op", op);
            w.WriteNumber("ok", ok);
            w.WriteNumber("total", total);
            w.WriteStartArray("failures");
            foreach (var f in failures)
            {
                w.WriteStartObject();
                f(w);
                w.WriteEndObject();
            }
            w.WriteEndArray();
            w.WriteString("note", note);
        });
    }

    private bool Wanted(string bench) => _only == null || _only.Contains(bench);

    /// <summary>Whether bench is to be measured; one marked with Skip is recorded as unsupported instead.</summary>
    private bool Gate(string bench)
    {
        if (!Wanted(bench))
        {
            return false;
        }
        if (_skipped.TryGetValue(bench, out var note))
        {
            BenchLine(bench, "unsupported", [], [], 0, 0, note);
            return false;
        }
        return true;
    }

    /// <summary>Marks one benchmark of an otherwise supported family as unsupported (call before registering the family).</summary>
    public void Skip(string bench, string note) => _skipped[bench] = note;

    /// <summary>Marks an operation the library cannot perform.</summary>
    public void Unsupported(Op op, string note)
    {
        if (_mode != Mode.Bench)
        {
            return;
        }
        foreach (var b in Benches(op))
        {
            if (Wanted(b))
            {
                BenchLine(b, "unsupported", [], [], 0, 0, note);
            }
        }
    }

    // ------------------------------------------------------------ timing

    private static double ElapsedNs(long start) =>
        (Stopwatch.GetTimestamp() - start) * (1e9 / Stopwatch.Frequency);

    /// <summary>
    /// Process CPU time in ns: user + system time of every thread of the
    /// process (getrusage on Unix, µs resolution on macOS).
    /// </summary>
    private static long CpuNs() => Environment.CpuUsage.TotalTime.Ticks * 100;

    private static readonly long[] Steps = [1, 2, 5];

    /// <summary>Times f after the first call (already made, in firstNs) passed its check.</summary>
    private void Measure<T>(string bench, long firstNs, string note, Func<T> f)
    {
        long n = 1;
        var step = 0;
        try
        {
            while (true)
            {
                var t = Stopwatch.GetTimestamp();
                for (long i = 0; i < n; i++)
                {
                    Sink.Consume(f());
                }
                if (ElapsedNs(t) >= _benchtimeNs)
                {
                    break;
                }
                step++;
                n = Steps[step % 3] * (long)Math.Pow(10, step / 3);
            }
            var ns = new List<double>(_count);
            var cpu = new List<double>(_count);
            for (var c = 0; c < _count; c++)
            {
                // The CPU clock is read just outside the wall-clock window, so
                // that its cost stays out of ns_per_op.
                var cpu0 = CpuNs();
                var t = Stopwatch.GetTimestamp();
                for (long i = 0; i < n; i++)
                {
                    Sink.Consume(f());
                }
                ns.Add(ElapsedNs(t) / n);
                cpu.Add((double)(CpuNs() - cpu0) / n);
            }
            BenchLine(bench, "ok", ns, cpu, n, firstNs, note);
        }
        catch (Exception e)
        {
            var msg = $"during measurement (N={n}): {Shorten(Describe(e))}";
            BenchLine(bench, "fail", [], [], 0, firstNs, note.Length == 0 ? msg : $"{msg} — {note}");
        }
    }

    /// <summary>Runs the first call and either measures or records the failure.</summary>
    private void BenchOne<T>(string bench, string note, Action<T> check, Func<T> f)
    {
        var t = Stopwatch.GetTimestamp();
        T result;
        long first;
        try
        {
            result = f();
            first = (long)ElapsedNs(t);
        }
        catch (Exception e)
        {
            first = (long)ElapsedNs(t);
            Fail(bench, note, first, Describe(e));
            return;
        }
        try
        {
            check(result);
        }
        catch (Exception e)
        {
            Fail(bench, note, first, Describe(e));
            return;
        }
        Measure(bench, first, note, f);
    }

    private void Fail(string bench, string note, long first, string error)
    {
        var msg = note.Length == 0 ? Shorten(error) : $"{Shorten(error)} — {note}";
        BenchLine(bench, "fail", [], [], 0, first, msg);
    }

    private static void CheckCii(object? o, string what)
    {
        if (!Corpus.LooksLikeCii(o))
        {
            throw new InvalidDataException($"{what} bytes are not a CII invoice");
        }
    }

    // ------------------------------------------------------------ operations

    /// <summary>PDF → bytes of the invoice XML.</summary>
    public void Extract(string note, Func<byte[], byte[]> f) => Extract(note, f, x => x);

    /// <summary>
    /// PDF → the invoice, when the library's extraction returns something other
    /// than the XML bytes (a model, a string): xmlOf turns the result into the
    /// XML for the check, outside the timing.
    /// </summary>
    public void Extract<T>(string note, Func<byte[], T> f, Func<T, object?> xmlOf)
    {
        if (_mode == Mode.Bench)
        {
            foreach (var b in Benches(Op.Extract))
            {
                if (!Gate(b))
                {
                    continue;
                }
                var data = Corpus.Sample(Corpus.InputOf(b));
                BenchOne(b, note, r => CheckCii(xmlOf(r), "extracted"), () => f(data));
            }
            return;
        }
        var files = Corpus.PdfFiles();
        var (ok, failures) = (0, new List<Action<Utf8JsonWriter>>());
        foreach (var p in files)
        {
            var data = Corpus.Load(p);
            var rel = Corpus.Rel(p);
            try
            {
                if (Corpus.LooksLikeCii(xmlOf(f(data))))
                {
                    ok++;
                }
                else
                {
                    failures.Add(w => { w.WriteString("file", rel); w.WriteString("error", "extracted bytes are not a CII invoice"); });
                }
            }
            catch (Exception e)
            {
                var err = Shorten(Describe(e));
                failures.Add(w => { w.WriteString("file", rel); w.WriteString("error", err); });
            }
        }
        SurveyLine("extract", ok, files.Count, failures, note);
    }

    /// <summary>XML → the library's model.</summary>
    public void Parse<M>(string note, Func<byte[], M> f)
    {
        if (_mode == Mode.Bench)
        {
            foreach (var b in Benches(Op.Parse))
            {
                if (!Gate(b))
                {
                    continue;
                }
                var data = Corpus.Sample(Corpus.InputOf(b));
                BenchOne(b, note, m =>
                {
                    if (m is null)
                    {
                        throw new InvalidDataException("parser returned null");
                    }
                }, () => f(data));
            }
            return;
        }
        var files = Corpus.CiiFiles();
        var (ok, failures) = (0, new List<Action<Utf8JsonWriter>>());
        foreach (var p in files)
        {
            var data = Corpus.Load(p);
            var rel = Corpus.Rel(p);
            try
            {
                if (f(data) is null)
                {
                    throw new InvalidDataException("parser returned null");
                }
                ok++;
            }
            catch (Exception e)
            {
                var err = Shorten(Describe(e));
                failures.Add(w => { w.WriteString("file", rel); w.WriteString("error", err); });
            }
        }
        SurveyLine("parse", ok, files.Count, failures, note);
    }

    /// <summary>
    /// XML → complete validation. f is timed; summarize reads the number of
    /// errors and their rule ids off its result, outside the timing.
    /// </summary>
    public void Validate<R>(string note, Func<byte[], R> f, Func<R, Findings> summarize)
    {
        if (_mode == Mode.Bench)
        {
            foreach (var b in Benches(Op.Validate))
            {
                if (!Gate(b))
                {
                    continue;
                }
                var data = Corpus.Sample(Corpus.InputOf(b));
                BenchOne(b, note, r => summarize(r), () => f(data));
            }
            return;
        }
        var files = Corpus.CiiFiles();
        var (flagged, failures) = (0, new List<Action<Utf8JsonWriter>>());
        foreach (var p in files)
        {
            var data = Corpus.Load(p);
            var rel = Corpus.Rel(p);
            try
            {
                var fd = summarize(f(data));
                if (fd.Errors == 0)
                {
                    continue;
                }
                flagged++;
                failures.Add(w =>
                {
                    w.WriteString("file", rel);
                    w.WriteNumber("errors", fd.Errors);
                    w.WriteStartArray("rules");
                    foreach (var r in fd.Rules)
                    {
                        w.WriteStringValue(r);
                    }
                    w.WriteEndArray();
                });
            }
            catch (Exception e)
            {
                flagged++;
                var err = Shorten(Describe(e));
                failures.Add(w =>
                {
                    w.WriteString("file", rel);
                    w.WriteNumber("errors", 0);
                    w.WriteStartArray("rules");
                    w.WriteEndArray();
                    w.WriteString("error", err);
                });
            }
        }
        SurveyLine("validate", flagged, files.Count, failures, note);
    }

    /// <summary>Model → CII XML, the model built once outside the timing by parse.</summary>
    public void Write<M, O>(string note, Func<byte[], M> parse, Func<M, O> write)
    {
        if (_mode != Mode.Bench)
        {
            return;
        }
        foreach (var b in Benches(Op.Write))
        {
            if (!Gate(b))
            {
                continue;
            }
            var data = Corpus.Sample(Corpus.InputOf(b));
            M model;
            try
            {
                model = parse(data);
            }
            catch (Exception e)
            {
                BenchLine(b, "fail", [], [], 0, 0, $"parse: {Shorten(Describe(e))}" + (note.Length == 0 ? "" : $" — {note}"));
                continue;
            }
            BenchOne(b, note, o => CheckCii(o, "written"), () => write(model));
        }
    }

    /// <summary>
    /// Host PDF + XML Small → Factur-X PDF; the result must read back (with
    /// extract, a library of the same language) and yield a CII invoice.
    /// </summary>
    public void Embed(string note, Func<byte[], byte[], byte[]> f, Func<byte[], object?> extract)
    {
        if (_mode != Mode.Bench)
        {
            return;
        }
        var xml = Corpus.Sample("XMLSmall");
        foreach (var b in Benches(Op.Embed))
        {
            if (!Gate(b))
            {
                continue;
            }
            var host = Corpus.Sample(Corpus.InputOf(b));
            BenchOne(b, note, output =>
            {
                object? back;
                try
                {
                    back = extract(output);
                }
                catch (Exception e)
                {
                    throw new InvalidDataException($"embedded PDF does not round-trip: {Describe(e)}");
                }
                if (!Corpus.LooksLikeCii(back))
                {
                    throw new InvalidDataException("embedded PDF does not round-trip: no CII invoice read back");
                }
            }, () => f(host, xml));
        }
    }

    /// <summary>Native complete check of a hybrid PDF (container + XML).</summary>
    public void CheckPdf<R>(string note, Func<byte[], R> f)
    {
        if (_mode != Mode.Bench)
        {
            return;
        }
        foreach (var b in Benches(Op.CheckPdf))
        {
            if (!Gate(b))
            {
                continue;
            }
            var data = Corpus.Sample(Corpus.InputOf(b));
            BenchOne(b, note, _ => { }, () => f(data));
        }
    }

    // ------------------------------------------------------------ entry point

    /// <summary>Stack of the worker thread every operation runs on: XSLT engines recurse deeply.</summary>
    private const int StackBytes = 256 * 1024 * 1024;

    /// <summary>Entry point of every library program: bench-&lt;lib&gt; bench|survey|resources [Bench…].</summary>
    public static int Run(Lib lib, string[] args, Action<Harness> body)
    {
        Mode mode;
        var kind = args.FirstOrDefault() ?? "";
        switch (kind)
        {
            case "bench" or "resources": mode = Mode.Bench; break;
            case "survey": mode = Mode.Survey; break;
            default:
                Console.Error.WriteLine($"usage: bench-{lib.Name} bench|survey|resources [BenchName…]");
                return 2;
        }
        var resources = kind == "resources";
        // Invariant culture for the whole process: results must not depend on
        // the machine's locale (number formats in messages, culture-sensitive
        // parsing in the libraries).
        CultureInfo.DefaultThreadCurrentCulture = CultureInfo.InvariantCulture;
        CultureInfo.DefaultThreadCurrentUICulture = CultureInfo.InvariantCulture;
        CultureInfo.CurrentCulture = CultureInfo.InvariantCulture;
        CultureInfo.CurrentUICulture = CultureInfo.InvariantCulture;
        var only = args.Length > 1 ? args.Skip(1).ToList() : null;
        // resources: one (lib, bench) per process, read back by run.sh right after.
        var outDir = Path.Combine(Environment.GetEnvironmentVariable("OUT") is { Length: > 0 } o ? o : "results",
            resources ? "csharp-res-parts" : "csharp-parts");
        Directory.CreateDirectory(outDir);
        var path = Path.Combine(outDir, resources ? $"run_{lib.Name}.jsonl" : $"{kind}_{lib.Name}.jsonl");
        var count = int.TryParse(Environment.GetEnvironmentVariable("COUNT"), out var c) ? c : 3;
        var benchtime = double.TryParse(
            Environment.GetEnvironmentVariable("BENCHTIME")?.TrimEnd('s'),
            System.Globalization.NumberStyles.Float,
            System.Globalization.CultureInfo.InvariantCulture,
            out var bt) ? bt : 1.0;

        using var output = new StreamWriter(
            new FileStream(path, only != null && !resources ? FileMode.Append : FileMode.Create, FileAccess.Write),
            new UTF8Encoding(false));
        if (!resources)
        {
            Console.Error.WriteLine($"== {kind} {lib.Name} ({lib.Pkg}@{lib.Version})");
        }
        var h = new Harness(lib, mode, count, benchtime, output, only, resources);
        // First read of the CPU clock (P/Invoke binding) outside any measurement.
        _ = CpuNs();
        Exception? crash = null;
        var worker = new Thread(() =>
        {
            try
            {
                body(h);
            }
            catch (Exception e)
            {
                crash = e;
            }
        }, StackBytes)
        { Name = "bench" };
        worker.Start();
        worker.Join();
        if (crash != null)
        {
            Console.Error.WriteLine($"== {lib.Name}: harness body failed: {Describe(crash)}");
            return 1;
        }
        return 0;
    }
}
