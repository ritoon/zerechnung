<?php

/*
 * PHP harness of the cross-language benchmark (see ../PROTOCOL.md).
 *
 * Mirror of python/harness.py and rust/harness/src/lib.rs: same input files,
 * the result is checked before it is measured, "unsupported" when the library
 * cannot do the operation, "fail" with the message when it returns a wrong
 * result or throws.
 *
 *     php harness.php survey    [lib…]   correctness surveys    -> $OUT/survey_php.jsonl
 *     php harness.php bench     [lib…]   benchmarks             -> $OUT/bench_php.jsonl
 *     php harness.php resources [lib…]   CPU time and peak RSS  -> $OUT/resources_php.jsonl
 *
 * Every benchmark runs in a fresh child process (first_call_ns is a true first
 * call: classes are autoloaded and the library's caches are cold), every survey
 * in one child per library and operation; a child that dies (fatal error,
 * memory exhaustion) is restarted after the file it died on. Each timed series
 * also reads the CPU time of the process (cpu_ns_per_op); "resources" runs the
 * same children and adds their peak resident memory (max_rss_bytes), read by
 * the child itself after the series. Variables: OUT,
 * COUNT (3), BENCHTIME (1, seconds), FX_ECOSYSTEM (../../ecosystem), FX_CORPUS
 * (corpus root), FX_TMP (scratch dir of the path-based APIs, default
 * <system temp>/fx-bench-php), PHP_OPTS (the -d flags given to every child,
 * set by run.sh), CHECK_TIMEOUT (30 s, the check call of a benchmark),
 * SURVEY_TIMEOUT (10 s, one survey file), CHILD_TIMEOUT (1800 s, one child).
 */

declare(strict_types=1);

namespace Bench;

const HERE = __DIR__;
const RESULT_MARK = '@@RESULT ';
const FILE_MARK = '@@FILE ';

/**
 * Libraries in the order of the runs, with the Composer environment each one
 * is loaded from: "main" is ./vendor, "invoicesuite" is ./invoicesuite/vendor
 * (horstoeko/invoicesuite declares a conflict with horstoeko/zugferd).
 */
const LIBS = [
    'horstoeko/zugferd' => 'main',
    'horstoeko/invoicesuite' => 'invoicesuite',
    'atgp/factur-x' => 'main',
    'tiime/factur-x' => 'main',
    'tiime/cross-industry-invoice' => 'main',
    'easybill/zugferd-php' => 'main',
    'easybill/e-invoicing' => 'main',
    'mahdiabderraouf/facturx-php' => 'main',
    'daniel-jorg-schuppelius/php-erechnung-toolkit' => 'main',
    'john-wink/en16931-php' => 'main',
    'invoicepress/vampire-php' => 'main',
    'intragile/php-einvoicing' => 'main',
    'dealerweb/einvoice' => 'main',
    'quittio/core' => 'main',
    'pnscripts/pn-invoice' => 'main',
    'pat-o-dev/factur-x' => 'main',
    'stann/factur-x' => 'main',
    'smalot/pdfparser' => 'main',
    'ext-dom' => 'main',
];

const BENCHES = [
    'Extract_Small', 'Extract_Large', 'Extract_Minimum', 'Extract_Extended',
    'Parse_Small', 'Parse_Lines', 'Parse_Large',
    'ParseValidate_Small', 'ParseValidate_Lines', 'ParseValidate_Large',
    'Write_Small', 'Write_Lines',
    'Embed_Small', 'EmbedClassic_Small',
    'CheckPDF_Small', 'CheckPDF_Large',
];

const BENCH_SPEC = [
    'Extract_Small' => ['extract', 'PDFSmall'],
    'Extract_Large' => ['extract', 'PDFLarge'],
    'Extract_Minimum' => ['extract', 'PDFMinimum'],
    'Extract_Extended' => ['extract', 'PDFExtended'],
    'Parse_Small' => ['parse', 'XMLSmall'],
    'Parse_Lines' => ['parse', 'XMLLines'],
    'Parse_Large' => ['parse', 'XMLLarge'],
    'ParseValidate_Small' => ['validate', 'XMLSmall'],
    'ParseValidate_Lines' => ['validate', 'XMLLines'],
    'ParseValidate_Large' => ['validate', 'XMLLarge'],
    'Write_Small' => ['write', 'XMLSmall'],
    'Write_Lines' => ['write', 'XMLLines'],
    'Embed_Small' => ['embed', 'Host'],
    'EmbedClassic_Small' => ['embed', 'HostClassic'],
    'CheckPDF_Small' => ['check_pdf', 'PDFSmall'],
    'CheckPDF_Large' => ['check_pdf', 'PDFLarge'],
];

const SURVEY_OPS = ['validate', 'parse', 'extract'];

/** The adapter cannot perform this operation (status: unsupported). */
final class Unsupported extends \Exception
{
}

/**
 * A call that did not return in time. It extends \Error, not \Exception, so
 * that the many `catch (\Exception)` blocks of the libraries do not swallow it.
 */
final class Timeout extends \Error
{
}

// ---------------------------------------------------------------- environment

function env_float(string $name, float $default): float
{
    $v = getenv($name);

    return ($v === false || $v === '') ? $default : (float) rtrim($v, 's');
}

function env_int(string $name, int $default): int
{
    $v = getenv($name);

    return ($v === false || $v === '') ? $default : (int) $v;
}

function check_timeout(): float
{
    return env_float('CHECK_TIMEOUT', 30.0);
}

function survey_timeout(): float
{
    return env_float('SURVEY_TIMEOUT', 10.0);
}

function child_timeout(): float
{
    return env_float('CHILD_TIMEOUT', 1800.0);
}

function ecosystem(): string
{
    $v = getenv('FX_ECOSYSTEM');

    return ($v !== false && $v !== '') ? $v : \dirname(HERE, 2) . '/ecosystem';
}

function corpus_root(): string
{
    $v = getenv('FX_CORPUS');

    return ($v !== false && $v !== '') ? $v : ecosystem() . '/corpus';
}

function runtime(): string
{
    $opcache = \function_exists('opcache_get_status') && (bool) ini_get('opcache.enable_cli') && \PHP_SAPI === 'cli';
    $jit = $opcache ? (string) ini_get('opcache.jit') : 'disable';

    return sprintf(
        'PHP %s (%s, OPcache %s, JIT %s, zend.assertions=%s)',
        \PHP_VERSION,
        \PHP_ZTS ? 'ZTS' : 'NTS',
        $opcache ? 'on' : 'off',
        (!$opcache || \in_array($jit, ['', '0', 'off', 'disable'], true) || (int) ini_get('opcache.jit_buffer_size') === 0) ? 'off' : $jit,
        (string) ini_get('zend.assertions'),
    );
}

// ---------------------------------------------------------------- corpus

/**
 * Scratch directory for the libraries that need files (paths, mPDF cache):
 * FX_TMP, else <system temp>/fx-bench-php. Created on first use.
 */
function tmp_dir(): string
{
    $d = getenv('FX_TMP') ?: sys_get_temp_dir() . '/fx-bench-php';
    if (!is_dir($d) && !@mkdir($d, 0o700, true) && !is_dir($d)) {
        throw new \RuntimeException("cannot create $d");
    }

    return $d;
}

/** @return array{name: string, path: string, data: string} */
function load_file(string $path): array
{
    $data = file_get_contents($path);
    if ($data === false) {
        throw new \RuntimeException("cannot read $path");
    }

    return ['name' => basename($path), 'path' => $path, 'data' => $data];
}

/** @return array{name: string, path: string, data: string} */
function sample(string $name): array
{
    $c = corpus_root();
    $rel = match ($name) {
        'PDFSmall' => "$c/XML-Rechnung/FX/EN16931_Einfach.pdf",
        'PDFLarge' => "$c/XML-Rechnung/FX/EN16931_Elektron.pdf",
        'PDFMinimum' => "$c/ZUGFeRDv2/correct/FNFE-factur-x-examples/Facture_FR_MINIMUM.pdf",
        'PDFExtended' => "$c/ZUGFeRDv2/correct/intarsys/EXTENDED/zugferd_2p0_EXTENDED_Warenrechnung.pdf",
        'XMLSmall' => "$c/XML-Rechnung/CII/EN16931_Einfach.cii.xml",
        'XMLLines' => "$c/XML-Rechnung/CII/EN16931_Rabatte.cii.xml",
        'XMLLarge' => "$c/XML-Rechnung/CII/XRECHNUNG_Elektron.cii.xml",
        'Host' => ecosystem() . '/host.pdf',
        'HostClassic' => ecosystem() . '/host_classic.pdf',
    };

    return load_file($rel);
}

function looks_like_cii(mixed $b): bool
{
    return \is_string($b) && (str_contains($b, 'CrossIndustryInvoice') || str_contains($b, 'CrossIndustryDocument'));
}

/** The 26 valid CII samples of the surveys. @return list<string> */
function cii_files(): array
{
    $m = glob(corpus_root() . '/XML-Rechnung/CII/*.cii.xml') ?: [];
    sort($m, \SORT_STRING);

    return array_values(array_filter(
        $m,
        static fn (string $f): bool => basename($f) !== 'not_validating_full_invoice_based_onTest_EeISI_300_CENfullmodel.cii.xml',
    ));
}

/** The 104 valid hybrid PDFs: every .pdf under ZUGFeRDv2/correct (recursive), then XML-Rechnung/FX/*.pdf. @return list<string> */
function pdf_files(): array
{
    $out = [];
    $it = new \RecursiveIteratorIterator(new \RecursiveDirectoryIterator(
        corpus_root() . '/ZUGFeRDv2/correct',
        \FilesystemIterator::SKIP_DOTS,
    ));
    foreach ($it as $f) {
        if ($f->isFile() && str_ends_with($f->getFilename(), '.pdf')) {
            $out[] = $f->getPathname();
        }
    }
    sort($out, \SORT_STRING);
    $fx = glob(corpus_root() . '/XML-Rechnung/FX/*.pdf') ?: [];
    sort($fx, \SORT_STRING);

    return [...$out, ...$fx];
}

/** @return list<string> */
function files_for(string $op): array
{
    return $op === 'extract' ? pdf_files() : cii_files();
}

function rel_path(string $f): string
{
    $root = rtrim(corpus_root(), '/') . '/';

    return str_starts_with($f, $root) ? substr($f, \strlen($root)) : $f;
}

// ---------------------------------------------------------------- tools

/**
 * Calls $fn with a SIGALRM guard. PHP checks the signal between two VM
 * instructions, so a loop in PHP code is interrupted; a long call inside a C
 * function (libxml, preg) only when it returns.
 */
function guarded(callable $fn, ?float $timeout = null): mixed
{
    $t = $timeout ?? check_timeout();
    $secs = max(1, (int) ceil($t));
    pcntl_async_signals(true);
    pcntl_signal(\SIGALRM, static function () use ($secs): void {
        throw new Timeout(sprintf('timeout (%d s): call interrupted (infinite loop?)', $secs));
    });
    pcntl_alarm($secs);
    try {
        return $fn();
    } finally {
        pcntl_alarm(0);
        pcntl_signal(\SIGALRM, \SIG_DFL);
    }
}

function err_text(\Throwable $e): string
{
    $cls = (new \ReflectionClass($e))->getShortName();
    $s = trim(preg_replace('/\s+/', ' ', $cls . ': ' . $e->getMessage()) ?? '');
    if ($e->getPrevious() !== null && \strlen($s) < 200) {
        $p = $e->getPrevious();
        $s .= ' <- ' . (new \ReflectionClass($p))->getShortName() . ': ' . trim(preg_replace('/\s+/', ' ', $p->getMessage()) ?? '');
    }

    // Drop the bench root from paths and the bytes of binary payloads quoted
    // by some parsers, so the note stays readable and valid UTF-8.
    // Some libraries also paste the stack trace of a wrapped exception into
    // their message: keep only the message.
    $s = str_replace(HERE . '/', '', $s);
    $s = preg_replace('/ Stack trace: #0 .*/s', '', $s) ?? $s;
    $s = mb_convert_encoding($s, 'UTF-8', 'UTF-8');
    $s = preg_replace('/[^\P{C}\t]+/u', '?', $s) ?? $s;

    return mb_strlen($s) <= 300 ? $s : mb_substr($s, 0, 297) . '...';
}

function adapter_file(string $lib): string
{
    return HERE . '/adapters/' . str_replace('/', '-', $lib) . '.php';
}

/** Loads the Composer environment of $lib, then its adapter. @return array<string, mixed> */
function load_adapter(string $lib): array
{
    $env = LIBS[$lib];
    $autoload = $env === 'main' ? HERE . '/vendor/autoload.php' : HERE . "/$env/vendor/autoload.php";
    if (!is_file($autoload)) {
        throw new \RuntimeException("$autoload missing: run ./run.sh setup");
    }
    require_once $autoload;
    $ad = require adapter_file($lib);
    if (!\is_array($ad)) {
        throw new \RuntimeException("adapter of $lib does not return an array");
    }

    return $ad;
}

/** @param array<string, mixed> $ad */
function lib_version(array $ad): string
{
    if (isset($ad['version']) && \is_callable($ad['version'])) {
        return (string) ($ad['version'])();
    }
    try {
        $v = \Composer\InstalledVersions::getPrettyVersion($ad['pkg']);
    } catch (\Throwable) {
        $v = null;
    }

    return $v === null ? '?' : ltrim($v, 'v');
}

/** @param array<string, mixed> $ad */
function has_op(array $ad, string $op): bool
{
    return isset($ad[$op]) && \is_callable($ad[$op]);
}

// ---------------------------------------------------------------- timing

/** @return \Generator<int> 1, 2, 5, 10, 20, 50… */
function ramp_steps(): \Generator
{
    $n = 1;
    while (true) {
        foreach ([1, 2, 5] as $m) {
            yield $n * $m;
        }
        $n *= 10;
    }
}

/**
 * CPU time in nanoseconds (user + system, every thread) of this process
 * ($who = 0, RUSAGE_SELF) or of its terminated, waited-for children
 * ($who = 1, RUSAGE_CHILDREN). getrusage() has microsecond resolution.
 */
function cpu_ns(int $who = 0): int
{
    $u = getrusage($who);

    return ($u['ru_utime.tv_sec'] + $u['ru_stime.tv_sec']) * 1_000_000_000
        + ($u['ru_utime.tv_usec'] + $u['ru_stime.tv_usec']) * 1_000;
}

/** Peak resident set size in bytes: ru_maxrss is in bytes on macOS, in kilobytes on Linux. */
function max_rss_bytes(int $who = 0): int
{
    $rss = (int) getrusage($who)['ru_maxrss'];

    return \PHP_OS_FAMILY === 'Darwin' ? $rss : $rss * 1024;
}

/**
 * Ramp 1, 2, 5, 10, 20, 50… until one series lasts at least $benchtime
 * seconds, then $count series of N calls. Each timed series also reads the
 * CPU time of the process, once before and once after, around the hrtime
 * reads. The last element is [CPU ns, peak RSS bytes] of the child processes
 * that ended during the timed series, null when the library starts none:
 * they are counted neither in the CPU time nor in the peak RSS of this process.
 *
 * @return array{0: int, 1: list<float>, 2: list<float>, 3: ?array{0: int, 1: int}}
 */
function measure(callable $fn, float $benchtime, int $count): array
{
    $target = (int) ($benchtime * 1e9);
    $n = 1;
    foreach (ramp_steps() as $n) {
        $t0 = hrtime(true);
        for ($i = 0; $i < $n; $i++) {
            $fn();
        }
        if (hrtime(true) - $t0 >= $target) {
            break;
        }
    }
    $res = [];
    $cpu = [];
    $chCpu0 = cpu_ns(1);
    $chRss0 = max_rss_bytes(1);
    for ($c = 0; $c < $count; $c++) {
        $c0 = cpu_ns();
        $t0 = hrtime(true);
        for ($i = 0; $i < $n; $i++) {
            $fn();
        }
        $t1 = hrtime(true);
        $c1 = cpu_ns();
        $res[] = ($t1 - $t0) / $n;
        $cpu[] = ($c1 - $c0) / $n;
    }
    $chCpu1 = cpu_ns(1);
    $chRss1 = max_rss_bytes(1);
    $children = ($chCpu1 > $chCpu0 || $chRss1 > $chRss0) ? [$chCpu1 - $chCpu0, $chRss1] : null;

    return [$n, $res, $cpu, $children];
}

/** @param array{0: int, 1: int} $children */
function children_note(array $children, int $calls): string
{
    return sprintf(
        'la librairie lance des processus enfants : leur CPU (≈ %.0f ns/op) et leur mémoire (pic %.1f Mio) ne sont comptés ni dans cpu_ns_per_op ni dans max_rss_bytes',
        $children[0] / $calls,
        $children[1] / 1048576,
    );
}

// ---------------------------------------------------------------- one benchmark

/** @param array<string, mixed> $ad @return list<string> */
function notes_for(array $ad, string $op, string $bench): array
{
    $notes = $ad['notes'] ?? [];
    $out = [];
    foreach ([$op, $bench] as $k) {
        if (isset($notes[$k]) && !\in_array($notes[$k], $out, true)) {
            $out[] = $notes[$k];
        }
    }

    return $out;
}

/**
 * Reads back a PDF produced by Embed when the library cannot extract: with
 * horstoeko/zugferd, the most used PHP library, when it is in the same
 * Composer environment.
 */
function reference_extract(string $pdf): string
{
    if (!class_exists(\horstoeko\zugferd\ZugferdDocumentPdfReader::class)) {
        throw new \RuntimeException('no reference extractor in this Composer environment');
    }

    return \horstoeko\zugferd\ZugferdDocumentPdfReader::getXmlFromContent($pdf);
}

/** @return array<string, mixed> */
function run_one(string $lib, string $bench, float $benchtime, int $count): array
{
    $ad = load_adapter($lib);
    [$op, $sampleName] = BENCH_SPEC[$bench];
    $rec = [
        'lang' => 'php', 'lib' => $lib, 'pkg' => $ad['pkg'], 'version' => lib_version($ad), 'bench' => $bench,
        'status' => 'ok', 'ns_per_op' => [], 'cpu_ns_per_op' => [], 'iters' => 0, 'first_call_ns' => null, 'note' => '',
        'runtime' => runtime(),
    ];
    $notes = notes_for($ad, $op, $bench);
    $finish = static function (string $status, ?string $msg = null) use (&$rec, &$notes): array {
        $rec['status'] = $status;
        if ($msg !== null && $msg !== '') {
            $notes[] = $msg;
        }
        $rec['note'] = implode(' ; ', array_filter($notes, static fn ($n) => $n !== ''));

        return $rec;
    };

    if (isset($ad['skip'][$bench])) {
        return $finish('unsupported', $ad['skip'][$bench]);
    }
    if (!has_op($ad, $op)) {
        return $finish('unsupported', $ad['unsupported'][$op] ?? 'opération non proposée par la librairie');
    }
    $impl = $ad[$op];

    try {
        $s = sample($sampleName);
        if ($op === 'write') {
            if (has_op($ad, 'parse')) {
                $data = $s['data'];
                $path = $s['path'];
                $model = guarded(static fn () => ($ad['parse'])($data, $path));
            } elseif (has_op($ad, 'hand_model') && $sampleName === 'XMLSmall') {
                $model = guarded(static fn () => ($ad['hand_model'])());
                $notes[] = 'modèle construit à la main';
            } else {
                return $finish('unsupported', 'pas de parseur ; modèle à la main seulement pour EN16931_Einfach (protocole)');
            }
            $fn = static fn () => $impl($model);
        } elseif ($op === 'embed') {
            $xml = sample('XMLSmall')['data'];
            $host = $s['data'];
            $fn = static fn () => $impl($host, $xml);
        } else {
            $data = $s['data'];
            $path = $s['path'];
            $fn = static fn () => $impl($data, $path);
        }

        // 1. check before measuring (first call of the process)
        $t0 = hrtime(true);
        try {
            $out = guarded($fn);
        } catch (Unsupported $e) {
            return $finish('unsupported', $e->getMessage());
        }
        $rec['first_call_ns'] = hrtime(true) - $t0;

        if ($op === 'extract' || $op === 'write') {
            if (!looks_like_cii($out)) {
                return $finish('fail', 'le résultat ne contient pas CrossIndustryInvoice');
            }
        } elseif ($op === 'embed') {
            if (!\is_string($out) || !str_starts_with($out, '%PDF')) {
                return $finish('fail', "le résultat n'est pas un PDF");
            }
            // read back with the library itself, else with horstoeko/zugferd
            $own = has_op($ad, 'extract');
            try {
                $back = guarded(static fn () => $own ? ($ad['extract'])($out, null) : reference_extract($out));
            } catch (\Throwable $e) {
                return $finish('fail', 'le PDF produit ne se relit pas : ' . err_text($e));
            }
            if (!looks_like_cii($back)) {
                return $finish('fail', 'le PDF produit ne se relit pas (pas de facture extraite)');
            }
            if (!$own) {
                $notes[] = 'relu avec horstoeko/zugferd';
            }
        } elseif ($op === 'validate' || $op === 'check_pdf') {
            if (!(\is_array($out) && \is_int($out[0] ?? null) && \is_array($out[1] ?? null))) {
                return $finish('fail', 'résultat inattendu : ' . mb_substr(var_export($out, true), 0, 200));
            }
        }

        // 2. measure
        [$n, $res, $cpu, $children] = measure($fn, $benchtime, $count);
        $rec['iters'] = $n;
        $rec['ns_per_op'] = array_map(static fn (float $v) => round($v, 1), $res);
        $rec['cpu_ns_per_op'] = array_map(static fn (float $v) => round($v, 1), $cpu);
        if ($children !== null) {
            $notes[] = children_note($children, $n * $count);
        }

        return $finish('ok');
    } catch (\Throwable $e) {
        return $finish('fail', err_text($e));
    }
}

// ---------------------------------------------------------------- surveys

/**
 * One survey file: validate (flagged = at least one error, or an exception),
 * parse (read without exception), extract (CII bytes extracted).
 *
 * @param array<string, mixed> $ad
 * @return array<string, mixed>
 */
function survey_file(array $ad, string $op, string $f): array
{
    $data = file_get_contents($f);
    $name = $op === 'extract' ? rel_path($f) : basename($f);
    try {
        if ($op === 'validate') {
            $r = guarded(static fn () => ($ad['validate'])($data, $f), survey_timeout());
            [$n, $rules] = $r;
            if ($n > 0) {
                $rules = array_values(array_unique(array_map('strval', $rules)));
                sort($rules, \SORT_STRING);

                return ['file' => $name, 'flagged' => true, 'errors' => $n, 'rules' => $rules];
            }

            return ['file' => $name, 'flagged' => false];
        }
        if ($op === 'parse') {
            guarded(static fn () => ($ad['parse'])($data, $f), survey_timeout());

            return ['file' => $name, 'ok' => true];
        }
        $x = guarded(static fn () => ($ad['extract'])($data, $f), survey_timeout());

        return looks_like_cii($x) ? ['file' => $name, 'ok' => true] : ['file' => $name, 'ok' => false, 'error' => 'pas de XML CII extrait'];
    } catch (\Throwable $e) {
        return $op === 'validate'
            ? ['file' => $name, 'flagged' => true, 'error' => err_text($e)]
            : ['file' => $name, 'ok' => false, 'error' => err_text($e)];
    }
}

/** @param list<array<string, mixed>> $per @return array{0: int, 1: list<array<string, mixed>>} */
function survey_tally(string $op, array $per): array
{
    $ok = 0;
    $failures = [];
    foreach ($per as $r) {
        if ($op === 'validate') {
            if ($r['flagged']) {
                $ok++;
                $failures[] = isset($r['error'])
                    ? ['file' => $r['file'], 'error' => $r['error']]
                    : ['file' => $r['file'], 'errors' => $r['errors'], 'rules' => $r['rules']];
            }
        } elseif ($r['ok']) {
            $ok++;
        } else {
            $failures[] = ['file' => $r['file'], 'error' => $r['error']];
        }
    }

    return [$ok, $failures];
}

// ---------------------------------------------------------------- children

function emit(string $mark, array $v): void
{
    fwrite(\STDOUT, $mark . json_encode($v, \JSON_UNESCAPED_UNICODE | \JSON_UNESCAPED_SLASHES | \JSON_INVALID_UTF8_SUBSTITUTE) . "\n");
    fflush(\STDOUT);
}

/** @param list<string> $argv */
function child(array $argv): int
{
    $kind = $argv[0] ?? '';
    if ($kind === 'bench' || $kind === 'resources') {
        [, $lib, $bench, $benchtime, $count] = $argv;
        $rec = run_one($lib, $bench, (float) $benchtime, (int) $count);
        if ($kind === 'resources' && $rec['status'] === 'ok') {
            // peak resident memory of this process, the one that ran the benchmark
            $rec['max_rss_bytes'] = max_rss_bytes();
        }
        emit(RESULT_MARK, $rec);

        return 0;
    }
    if ($kind === 'info') {
        $ad = load_adapter($argv[1]);
        $ops = array_values(array_filter(SURVEY_OPS, static fn ($op) => has_op($ad, $op)));
        emit(RESULT_MARK, [
            'pkg' => $ad['pkg'], 'version' => lib_version($ad), 'ops' => $ops,
            'survey_notes' => $ad['survey_notes'] ?? (object) [],
        ]);

        return 0;
    }
    if ($kind === 'survey') {
        [, $lib, $op, $start] = $argv;
        $ad = load_adapter($lib);
        $files = files_for($op);
        for ($i = (int) $start; $i < \count($files); $i++) {
            emit(FILE_MARK, ['idx' => $i] + survey_file($ad, $op, $files[$i]));
        }
        if ($op === 'extract' && isset($ad['survey_extract_extra']) && \is_callable($ad['survey_extract_extra'])) {
            emit(RESULT_MARK, ['extra' => (string) ($ad['survey_extract_extra'])($files)]);
        }

        return 0;
    }
    fwrite(\STDERR, "unknown child command\n");

    return 2;
}

/** @return list<string> */
function php_opts(): array
{
    $v = getenv('PHP_OPTS');

    return ($v === false || trim($v) === '') ? [] : (preg_split('/\s+/', trim($v)) ?: []);
}

/**
 * Runs `php harness.php _child …` and returns its stdout lines, the tail of
 * its stderr, its exit code and whether it was killed after CHILD_TIMEOUT.
 *
 * @param list<string> $args
 * @return array{lines: list<string>, stderr: string, code: int, timedout: bool}
 */
function spawn(array $args): array
{
    $cmd = [\PHP_BINARY, ...php_opts(), __FILE__, '_child', ...$args];
    $proc = proc_open($cmd, [0 => ['file', '/dev/null', 'r'], 1 => ['pipe', 'w'], 2 => ['pipe', 'w']], $pipes, HERE);
    if (!\is_resource($proc)) {
        return ['lines' => [], 'stderr' => 'proc_open failed', 'code' => -1, 'timedout' => false];
    }
    stream_set_blocking($pipes[1], false);
    stream_set_blocking($pipes[2], false);
    $out = '';
    $err = '';
    $deadline = microtime(true) + child_timeout();
    $timedout = false;
    $code = -1;
    while (true) {
        $r = [$pipes[1], $pipes[2]];
        $w = $e = null;
        $left = $deadline - microtime(true);
        if ($left <= 0) {
            $timedout = true;
            proc_terminate($proc, 9);
            break;
        }
        if (@stream_select($r, $w, $e, (int) min(1, $left), 200000) === false) {
            break;
        }
        foreach ($r as $p) {
            $chunk = fread($p, 1 << 16);
            if ($chunk !== false && $chunk !== '') {
                if ($p === $pipes[1]) {
                    $out .= $chunk;
                } else {
                    $err .= $chunk;
                    if (\strlen($err) > 1 << 20) {
                        $err = substr($err, -(1 << 16));
                    }
                }
            }
        }
        $st = proc_get_status($proc);
        if (!$st['running']) {
            $code = $st['exitcode'];
            // drain what is left
            $out .= stream_get_contents($pipes[1]) ?: '';
            $err .= stream_get_contents($pipes[2]) ?: '';
            break;
        }
    }
    fclose($pipes[1]);
    fclose($pipes[2]);
    $closed = proc_close($proc);
    if ($code === -1) {
        $code = $closed;
    }
    $lines = array_values(array_filter(explode("\n", $out), static fn ($l) => $l !== ''));
    $tail = \array_slice(array_values(array_filter(array_map('trim', explode("\n", $err)))), -3);

    return ['lines' => $lines, 'stderr' => implode(' | ', $tail), 'code' => $code, 'timedout' => $timedout];
}

/** @param list<string> $lines @return list<array<string, mixed>> */
function marked(array $lines, string $mark): array
{
    $out = [];
    foreach ($lines as $l) {
        if (str_starts_with($l, $mark)) {
            $v = json_decode(substr($l, \strlen($mark)), true);
            if (\is_array($v)) {
                $out[] = $v;
            }
        }
    }

    return $out;
}

function diag(array $r): string
{
    if ($r['timedout']) {
        return sprintf('processus fils interrompu après %g s', child_timeout());
    }

    return sprintf('code %d : %s', $r['code'], $r['stderr'] !== '' ? $r['stderr'] : '(pas de message)');
}

// ---------------------------------------------------------------- orchestration

function out_dir(): string
{
    $v = getenv('OUT');
    $d = ($v !== false && $v !== '') ? $v : \dirname(HERE) . '/results/' . date('Y-m-d');
    if (!is_dir($d) && !mkdir($d, 0o777, true) && !is_dir($d)) {
        throw new \RuntimeException("cannot create $d");
    }

    return realpath($d) ?: $d;
}

/**
 * Replaces in the file the lines of the libraries that were run again and
 * keeps the others.
 *
 * @param list<string> $libs
 * @param list<array<string, mixed>> $new
 */
function merge_jsonl(string $path, array $libs, array $new): void
{
    $keep = [];
    if (is_file($path)) {
        foreach (file($path, \FILE_IGNORE_NEW_LINES | \FILE_SKIP_EMPTY_LINES) ?: [] as $line) {
            $v = json_decode($line, true);
            if (!\is_array($v) || \in_array($v['lib'] ?? null, $libs, true)) {
                continue;
            }
            $keep[] = $line;
        }
    }
    foreach ($new as $r) {
        $keep[] = json_encode($r, \JSON_UNESCAPED_UNICODE | \JSON_UNESCAPED_SLASHES | \JSON_INVALID_UTF8_SUBSTITUTE);
    }
    file_put_contents($path, $keep === [] ? '' : implode("\n", $keep) . "\n");
}

/** @return list<array<string, mixed>> */
function survey_lib(string $lib): array
{
    $info = spawn(['info', $lib]);
    $meta = marked($info['lines'], RESULT_MARK)[0] ?? null;
    if ($meta === null) {
        echo '   (aucun résultat : ' . diag($info) . ")\n";

        return [];
    }
    $base = ['lang' => 'php', 'lib' => $lib, 'pkg' => $meta['pkg'], 'version' => $meta['version']];
    $lines = [];
    foreach ($meta['ops'] as $op) {
        $files = files_for($op);
        $per = [];
        $extra = '';
        $start = 0;
        $crashes = [];
        while ($start < \count($files)) {
            $r = spawn(['survey', $lib, $op, (string) $start]);
            $got = marked($r['lines'], FILE_MARK);
            foreach ($got as $g) {
                $per[$g['idx']] = $g;
            }
            foreach (marked($r['lines'], RESULT_MARK) as $x) {
                $extra = $x['extra'] ?? $extra;
            }
            $next = $got === [] ? $start : max(array_column($got, 'idx')) + 1;
            if ($next >= \count($files)) {
                break;
            }
            // the child died on file $next: record it and go on after it
            $f = $files[$next];
            $msg = 'le processus du relevé est mort sur ce fichier : ' . diag($r);
            $name = $op === 'extract' ? rel_path($f) : basename($f);
            $per[$next] = $op === 'validate'
                ? ['idx' => $next, 'file' => $name, 'flagged' => true, 'error' => $msg]
                : ['idx' => $next, 'file' => $name, 'ok' => false, 'error' => $msg];
            $crashes[] = $name;
            $start = $next + 1;
        }
        ksort($per);
        [$ok, $failures] = survey_tally($op, array_values($per));
        $note = (string) ($meta['survey_notes'][$op] ?? '');
        if ($extra !== '') {
            $note = ($note !== '' ? "$note ; " : '') . $extra;
        }
        if ($crashes !== []) {
            $note = ($note !== '' ? "$note ; " : '') . \count($crashes) . ' processus morts (erreur fatale), relancés au fichier suivant';
        }
        $lines[] = $base + ['op' => $op, 'ok' => $ok, 'total' => \count($files), 'failures' => $failures, 'note' => $note];
    }
    if ($lines === []) {
        echo "   (aucune opération relevée : ni validate, ni parse, ni extract)\n";
    }

    return $lines;
}

/** @param list<string> $argv */
function main(array $argv): int
{
    if (($argv[0] ?? '') === '_child') {
        return child(\array_slice($argv, 1));
    }
    $cmd = $argv[0] ?? '';
    if ($cmd !== 'survey' && $cmd !== 'bench' && $cmd !== 'resources') {
        fwrite(\STDERR, "usage: php harness.php survey|bench|resources [lib…]\n");

        return 2;
    }
    $libs = \array_slice($argv, 1) ?: array_keys(LIBS);
    foreach ($libs as $lib) {
        if (!isset(LIBS[$lib])) {
            fwrite(\STDERR, "unknown library: $lib (known: " . implode(', ', array_keys(LIBS)) . ")\n");

            return 2;
        }
    }
    $out = out_dir();
    echo '== ' . runtime() . ' (' . \PHP_BINARY . ")\n";
    if ($cmd === 'survey') {
        $path = "$out/survey_php.jsonl";
        foreach ($libs as $lib) {
            echo "== survey $lib\n";
            $res = survey_lib($lib);
            foreach ($res as $r) {
                echo "   {$r['op']}: {$r['ok']}/{$r['total']}" . ($r['note'] !== '' ? "  ({$r['note']})" : '') . "\n";
            }
            merge_jsonl($path, [$lib], $res);
        }
        echo "== $path\n";

        return 0;
    }
    // bench and resources: one child per benchmark (PHP_BINARY with PHP_OPTS,
    // no wrapper); for resources the child adds its own peak RSS.
    $count = env_int('COUNT', 3);
    $benchtime = env_float('BENCHTIME', 1.0);
    $path = "$out/{$cmd}_php.jsonl";
    $median = static function (array $v): float {
        sort($v);

        return (float) $v[intdiv(\count($v), 2)];
    };
    foreach ($libs as $lib) {
        echo "== $cmd $lib\n";
        $resLib = [];
        foreach (BENCHES as $bench) {
            $r = spawn([$cmd, $lib, $bench, (string) $benchtime, (string) $count]);
            $res = marked($r['lines'], RESULT_MARK);
            if ($res === []) {
                $res = [[
                    'lang' => 'php', 'lib' => $lib, 'pkg' => $lib, 'version' => '?', 'bench' => $bench,
                    'status' => 'fail', 'ns_per_op' => [], 'cpu_ns_per_op' => [], 'iters' => 0, 'first_call_ns' => null,
                    'note' => 'le processus de mesure a échoué : ' . diag($r), 'runtime' => runtime(),
                ]];
            }
            $x = $res[0];
            if ($x['status'] === 'ok' && $cmd === 'resources') {
                printf(
                    "   %-20s %14.0f ns/op %14.0f cpu-ns/op %8.1f MiB\n",
                    $bench,
                    $median($x['ns_per_op']),
                    $median($x['cpu_ns_per_op']),
                    $x['max_rss_bytes'] / 1048576,
                );
            } elseif ($x['status'] === 'ok') {
                printf("   %-20s %14.0f ns/op  x%d\n", $bench, $median($x['ns_per_op']), $x['iters']);
            } elseif ($x['status'] === 'fail') {
                printf("   %-20s FAIL %s\n", $bench, $x['note']);
            } else {
                printf("   %-20s unsupported\n", $bench);
            }
            $resLib[] = $x;
        }
        merge_jsonl($path, [$lib], $resLib);
    }
    echo "== $path\n";

    return 0;
}

exit(main(\array_slice($argv, 1)));
