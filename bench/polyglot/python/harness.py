"""Harnais Python du banc polyglotte (voir ../PROTOCOL.md).

Calqué sur bench/ecosystem/common/{common,harness}.go : mêmes fichiers
d'entrée, contrôle du résultat avant la mesure, « unsupported » si la
librairie ne sait pas faire l'opération, « fail » si elle renvoie un résultat
faux ou lève une exception.

    python harness.py survey    [lib…]   relevés d'exactitude -> $OUT/survey_python.jsonl
    python harness.py bench     [lib…]   benchmarks           -> $OUT/bench_python.jsonl
    python harness.py resources [lib…]   CPU et pic mémoire   -> $OUT/resources_python.jsonl

Chaque benchmark tourne dans un processus neuf (first_call_ns = vrai premier
appel), chaque relevé dans un processus par librairie. Chaque série mesurée
relève aussi le temps CPU du processus (cpu_ns_per_op) ; « resources » relance
les mêmes processus et ajoute leur pic de mémoire résidente (max_rss_bytes),
lu par le processus fils lui-même à la fin. Variables : OUT, COUNT (3),
BENCHTIME (1, en secondes), FX_CORPUS (racine du corpus).
"""

from __future__ import annotations

import glob
import importlib
import importlib.metadata
import io
import json
import os
import platform
import resource
import signal
import subprocess
import sys
import time
import traceback
from dataclasses import dataclass
from pathlib import Path

HERE = Path(__file__).resolve().parent
ECOSYSTEM = HERE.parent.parent / "ecosystem"

LIBS = [
    "factur-x",
    "drafthorse",
    "pycheval",
    "getafix",
    "zugferd-pdf",
    "erechnung",
    "mcp-einvoicing-de",
    "saxonche-schematron",
]

BENCHES = [
    "Extract_Small", "Extract_Large", "Extract_Minimum", "Extract_Extended",
    "Parse_Small", "Parse_Lines", "Parse_Large",
    "ParseValidate_Small", "ParseValidate_Lines", "ParseValidate_Large",
    "Write_Small", "Write_Lines",
    "Embed_Small", "EmbedClassic_Small",
    "CheckPDF_Small", "CheckPDF_Large",
]

CHECK_TIMEOUT = float(os.environ.get("CHECK_TIMEOUT", "30"))  # s, appel de contrôle d'un benchmark
SURVEY_TIMEOUT = float(os.environ.get("SURVEY_TIMEOUT", "10"))  # s, un fichier du relevé
CHILD_TIMEOUT = float(os.environ.get("CHILD_TIMEOUT", "1800"))  # s, un processus fils
RESULT_MARK = "@@RESULT "


# ---------------------------------------------------------------- corpus

def corpus_root() -> Path:
    v = os.environ.get("FX_CORPUS")
    return Path(v) if v else ECOSYSTEM / "corpus"


@dataclass
class Sample:
    name: str
    path: str
    data: bytes


def load(*rel: str) -> Sample:
    p = corpus_root().joinpath(*rel)
    return Sample(p.name, str(p), p.read_bytes())


def load_host(name: str) -> Sample:
    p = ECOSYSTEM / name
    return Sample(p.name, str(p), p.read_bytes())


SAMPLES = {
    "PDFSmall": lambda: load("XML-Rechnung", "FX", "EN16931_Einfach.pdf"),
    "PDFLarge": lambda: load("XML-Rechnung", "FX", "EN16931_Elektron.pdf"),
    "PDFMinimum": lambda: load("ZUGFeRDv2", "correct", "FNFE-factur-x-examples", "Facture_FR_MINIMUM.pdf"),
    "PDFExtended": lambda: load("ZUGFeRDv2", "correct", "intarsys", "EXTENDED", "zugferd_2p0_EXTENDED_Warenrechnung.pdf"),
    "XMLSmall": lambda: load("XML-Rechnung", "CII", "EN16931_Einfach.cii.xml"),
    "XMLLines": lambda: load("XML-Rechnung", "CII", "EN16931_Rabatte.cii.xml"),
    "XMLLarge": lambda: load("XML-Rechnung", "CII", "XRECHNUNG_Elektron.cii.xml"),
    "Host": lambda: load_host("host.pdf"),
    "HostClassic": lambda: load_host("host_classic.pdf"),
}


def looks_like_cii(b) -> bool:
    if isinstance(b, str):
        return "CrossIndustryInvoice" in b or "CrossIndustryDocument" in b
    if isinstance(b, (bytes, bytearray)):
        return b"CrossIndustryInvoice" in b or b"CrossIndustryDocument" in b
    return False


def cii_files() -> list[str]:
    m = sorted(glob.glob(str(corpus_root() / "XML-Rechnung" / "CII" / "*.cii.xml")))
    return [f for f in m if os.path.basename(f) != "not_validating_full_invoice_based_onTest_EeISI_300_CENfullmodel.cii.xml"]


def pdf_files() -> list[str]:
    out = []
    for dirpath, _dirs, files in os.walk(corpus_root() / "ZUGFeRDv2" / "correct"):
        for f in files:
            if f.endswith(".pdf"):
                out.append(os.path.join(dirpath, f))
    out.sort()
    out += sorted(glob.glob(str(corpus_root() / "XML-Rechnung" / "FX" / "*.pdf")))
    return out


# ---------------------------------------------------------------- outils

class Unsupported(Exception):
    """L'adaptateur ne sait pas faire l'opération (status: unsupported)."""


class Timeout(BaseException):
    pass


def guarded(fn, *args, timeout=None):
    """Appelle fn(*args) avec un garde-fou SIGALRM (le code pur Python est interruptible)."""
    t = CHECK_TIMEOUT if timeout is None else timeout

    def _on_alarm(*_):
        raise Timeout(f"timeout ({t:g} s) : appel interrompu (boucle infinie ?)")

    old = signal.signal(signal.SIGALRM, _on_alarm)
    signal.setitimer(signal.ITIMER_REAL, t)
    try:
        return fn(*args)
    finally:
        signal.setitimer(signal.ITIMER_REAL, 0)
        signal.signal(signal.SIGALRM, old)


def err_text(e: BaseException) -> str:
    s = f"{type(e).__name__}: {e}".replace("\n", " ").strip()
    return s if len(s) <= 300 else s[:297] + "..."


def adapter(lib: str):
    return importlib.import_module("adapters." + lib.replace("-", "_"))


def runtime() -> str:
    return f"{platform.python_implementation()} {platform.python_version()}"


def lib_version(ad) -> str:
    try:
        return importlib.metadata.version(ad.PKG)
    except importlib.metadata.PackageNotFoundError:
        return "?"


# ---------------------------------------------------------------- chronométrage

def ramp_steps():
    n = 1
    while True:
        for m in (1, 2, 5):
            yield n * m
        n *= 10


def children_usage() -> tuple[int, int]:
    """CPU (ns, utilisateur + système) et pic RSS (octets) des processus enfants terminés."""
    u = resource.getrusage(resource.RUSAGE_CHILDREN)
    return int((u.ru_utime + u.ru_stime) * 1e9), rss_bytes(u.ru_maxrss)


def rss_bytes(ru_maxrss: int) -> int:
    """ru_maxrss est en octets sous macOS, en kilo-octets sous Linux."""
    return ru_maxrss if sys.platform == "darwin" else ru_maxrss * 1024


def measure(fn, benchtime_s: float, count: int):
    """Montée en charge 1, 2, 5, 10, 20, 50… jusqu'à une série >= benchtime, puis count séries.

    Chaque série mesurée lit aussi le temps CPU du processus (utilisateur +
    système, tous les fils), une fois avant et une fois après, autour des
    lectures de l'horloge monotone. Renvoie aussi (CPU en ns, pic RSS en
    octets) des processus enfants terminés pendant les séries, None si la
    librairie n'en lance pas : ils ne sont comptés ni dans le CPU ni dans le
    pic RSS du processus mesuré.
    """
    target = int(benchtime_s * 1e9)
    clock = time.perf_counter_ns
    cpu = time.process_time_ns
    n = 1
    for n in ramp_steps():
        t0 = clock()
        for _ in range(n):
            fn()
        if clock() - t0 >= target:
            break
    res, cpu_res = [], []
    ch_cpu0, ch_rss0 = children_usage()
    for _ in range(count):
        c0 = cpu()
        t0 = clock()
        for _ in range(n):
            fn()
        t1 = clock()
        c1 = cpu()
        res.append((t1 - t0) / n)
        cpu_res.append((c1 - c0) / n)
    ch_cpu1, ch_rss1 = children_usage()
    spawned = ch_cpu1 > ch_cpu0 or ch_rss1 > ch_rss0
    return n, res, cpu_res, (ch_cpu1 - ch_cpu0, ch_rss1) if spawned else None


# ---------------------------------------------------------------- un benchmark

BENCH_SPEC = {
    "Extract_Small": ("extract", "PDFSmall"),
    "Extract_Large": ("extract", "PDFLarge"),
    "Extract_Minimum": ("extract", "PDFMinimum"),
    "Extract_Extended": ("extract", "PDFExtended"),
    "Parse_Small": ("parse", "XMLSmall"),
    "Parse_Lines": ("parse", "XMLLines"),
    "Parse_Large": ("parse", "XMLLarge"),
    "ParseValidate_Small": ("validate", "XMLSmall"),
    "ParseValidate_Lines": ("validate", "XMLLines"),
    "ParseValidate_Large": ("validate", "XMLLarge"),
    "Write_Small": ("write", "XMLSmall"),
    "Write_Lines": ("write", "XMLLines"),
    "Embed_Small": ("embed", "Host"),
    "EmbedClassic_Small": ("embed", "HostClassic"),
    "CheckPDF_Small": ("check_pdf", "PDFSmall"),
    "CheckPDF_Large": ("check_pdf", "PDFLarge"),
}


def notes_for(ad, op: str, bench: str) -> list[str]:
    notes = getattr(ad, "NOTES", {})
    out = []
    for k in (op, bench):
        if k in notes and notes[k] not in out:
            out.append(notes[k])
    return out


def children_note(children: tuple[int, int], calls: int) -> str:
    cpu_ns, rss = children
    return (f"la librairie lance des processus enfants : leur CPU (≈ {cpu_ns / calls:.0f} ns/op) "
            f"et leur mémoire (pic {rss / 2**20:.1f} Mio) ne sont comptés ni dans cpu_ns_per_op ni dans max_rss_bytes")


def reference_extract(pdf: bytes):
    """Relecture d'un PDF produit par Embed quand la librairie ne sait pas extraire."""
    import facturx  # autre librairie du même langage
    name, xml = facturx.get_xml_from_pdf(pdf, check_xsd=False)
    if not xml:
        raise ValueError("aucune pièce jointe XML Factur-X relue par factur-x")
    return xml


def run_one(lib: str, bench: str, benchtime: float, count: int) -> dict:
    ad = adapter(lib)
    op, sample_name = BENCH_SPEC[bench]
    rec = {
        "lang": "python", "lib": lib, "pkg": ad.PKG, "version": lib_version(ad), "bench": bench,
        "status": "ok", "ns_per_op": [], "cpu_ns_per_op": [], "iters": 0, "first_call_ns": None, "note": "",
        "runtime": runtime(),
    }
    notes = notes_for(ad, op, bench)

    def finish(status, msg=None):
        rec["status"] = status
        if msg:
            notes.append(msg)
        rec["note"] = " ; ".join(n for n in notes if n)
        return rec

    impl = getattr(ad, op, None)
    if impl is None:
        return finish("unsupported", getattr(ad, "UNSUPPORTED", {}).get(op, "opération non proposée par la librairie"))

    s = SAMPLES[sample_name]()
    clock = time.perf_counter_ns
    try:
        if op == "write":
            x = s.data
            if getattr(ad, "parse", None) is not None:
                model = guarded(ad.parse, x, s.path)
            elif getattr(ad, "hand_model", None) is not None and sample_name == "XMLSmall":
                model = ad.hand_model()
                notes.append("modèle construit à la main")
            else:
                return finish("unsupported", "pas de parseur ; modèle à la main seulement pour EN16931_Einfach (protocole)")

            def fn():
                return impl(model)
        elif op == "embed":
            xml = SAMPLES["XMLSmall"]().data
            host = s.data

            def fn():
                return impl(host, xml)
        else:
            data, path = s.data, s.path

            def fn():
                return impl(data, path)

        # 1. vérifier avant de mesurer (premier appel du processus)
        t0 = clock()
        try:
            out = guarded(fn)
        except Unsupported as e:
            return finish("unsupported", str(e))
        rec["first_call_ns"] = clock() - t0

        if op == "extract" or op == "write":
            if not looks_like_cii(out):
                return finish("fail", "le résultat ne contient pas CrossIndustryInvoice")
        elif op == "embed":
            if not isinstance(out, (bytes, bytearray)) or not out.startswith(b"%PDF"):
                return finish("fail", "le résultat n'est pas un PDF")
            # relecture avec la librairie elle-même, à défaut avec factur-x
            back_fn = getattr(ad, "extract", None)
            try:
                back = guarded(back_fn, bytes(out), None) if back_fn else reference_extract(bytes(out))
            except BaseException as e:  # noqa: BLE001
                return finish("fail", "le PDF produit ne se relit pas : " + err_text(e))
            if not looks_like_cii(back):
                return finish("fail", "le PDF produit ne se relit pas (pas de facture extraite)")
            if not back_fn:
                notes.append("relu avec factur-x")
        elif op in ("validate", "check_pdf"):
            if not (isinstance(out, tuple) and isinstance(out[0], int)):
                return finish("fail", f"résultat inattendu : {out!r}"[:200])

        # 2. mesurer
        n, res, cpu_res, children = measure(fn, benchtime, count)
        rec["iters"] = n
        rec["ns_per_op"] = [round(v, 1) for v in res]
        rec["cpu_ns_per_op"] = [round(v, 1) for v in cpu_res]
        if children is not None:
            notes.append(children_note(children, n * count))
        return finish("ok")
    except BaseException as e:  # noqa: BLE001
        if isinstance(e, KeyboardInterrupt):
            raise
        return finish("fail", err_text(e))


# ---------------------------------------------------------------- relevés

def survey_lib(lib: str) -> list[dict]:
    ad = adapter(lib)
    lines = []
    base = {"lang": "python", "lib": lib, "pkg": ad.PKG, "version": lib_version(ad)}
    survey_notes = getattr(ad, "SURVEY_NOTES", {})

    if getattr(ad, "validate", None) is not None:
        flagged, total, failures = 0, 0, []
        for f in cii_files():
            data = Path(f).read_bytes()
            total += 1
            try:
                n, rules = guarded(ad.validate, data, f, timeout=SURVEY_TIMEOUT)
                if n > 0:
                    flagged += 1
                    failures.append({"file": os.path.basename(f), "errors": n, "rules": sorted(set(rules))})
            except BaseException as e:  # noqa: BLE001
                flagged += 1
                failures.append({"file": os.path.basename(f), "error": err_text(e)})
        lines.append({**base, "op": "validate", "ok": flagged, "total": total, "failures": failures,
                      "note": survey_notes.get("validate", "")})

    if getattr(ad, "parse", None) is not None:
        ok, total, failures = 0, 0, []
        for f in cii_files():
            data = Path(f).read_bytes()
            total += 1
            try:
                guarded(ad.parse, data, f, timeout=SURVEY_TIMEOUT)
                ok += 1
            except BaseException as e:  # noqa: BLE001
                failures.append({"file": os.path.basename(f), "error": err_text(e)})
        lines.append({**base, "op": "parse", "ok": ok, "total": total, "failures": failures,
                      "note": survey_notes.get("parse", "")})

    if getattr(ad, "extract", None) is not None:
        ok, total, failures = 0, 0, []
        for f in pdf_files():
            data = Path(f).read_bytes()
            total += 1
            rel = os.path.relpath(f, corpus_root())
            try:
                x = guarded(ad.extract, data, f, timeout=SURVEY_TIMEOUT)
                if looks_like_cii(x):
                    ok += 1
                else:
                    failures.append({"file": rel, "error": "pas de XML CII extrait"})
            except BaseException as e:  # noqa: BLE001
                failures.append({"file": rel, "error": err_text(e)})
        note = survey_notes.get("extract", "")
        extra = getattr(ad, "survey_extract_extra", None)
        if extra is not None:
            note = (note + " ; " if note else "") + extra(pdf_files())
        lines.append({**base, "op": "extract", "ok": ok, "total": total, "failures": failures, "note": note})
    return lines


# ---------------------------------------------------------------- orchestration

def child(argv: list[str]) -> None:
    sys.path.insert(0, str(HERE))
    kind = argv[0]
    if kind in ("bench", "resources"):
        _, lib, bench, benchtime, count = argv
        out = run_one(lib, bench, float(benchtime), int(count))
        if kind == "resources" and out["status"] == "ok":
            # pic de mémoire résidente de ce processus, celui qui a exécuté le benchmark
            out["max_rss_bytes"] = rss_bytes(resource.getrusage(resource.RUSAGE_SELF).ru_maxrss)
        print(RESULT_MARK + json.dumps(out, ensure_ascii=False), flush=True)
    elif kind == "survey":
        for line in survey_lib(argv[1]):
            print(RESULT_MARK + json.dumps(line, ensure_ascii=False), flush=True)


def spawn(args: list[str]) -> tuple[list[dict], str]:
    cmd = [sys.executable, "-I", str(Path(__file__).resolve()), "_child", *args]
    try:
        p = subprocess.run(cmd, capture_output=True, text=True, timeout=CHILD_TIMEOUT, cwd=str(HERE))
    except subprocess.TimeoutExpired:
        return [], f"processus fils interrompu après {CHILD_TIMEOUT:g} s"
    res = [json.loads(l[len(RESULT_MARK):]) for l in p.stdout.splitlines() if l.startswith(RESULT_MARK)]
    if p.returncode == 0:
        return res, "aucune opération relevée (ni validate, ni parse, ni extract)"
    tail = (p.stderr or "").strip().splitlines()[-3:]
    return res, f"code {p.returncode} : " + " | ".join(tail)


def out_dir() -> Path:
    d = Path(os.environ.get("OUT") or (HERE.parent / "results" / time.strftime("%Y-%m-%d")))
    d.mkdir(parents=True, exist_ok=True)
    return d


def merge_jsonl(path: Path, libs: list[str], new: list[dict]) -> None:
    """Remplace dans le fichier les lignes des librairies relancées, garde les autres."""
    keep = []
    if path.exists():
        for line in path.read_text(encoding="utf-8").splitlines():
            try:
                if json.loads(line).get("lib") in libs:
                    continue
            except json.JSONDecodeError:
                continue
            keep.append(line)
    keep += [json.dumps(r, ensure_ascii=False) for r in new]
    path.write_text("\n".join(keep) + ("\n" if keep else ""), encoding="utf-8")


def main(argv: list[str]) -> int:
    if argv and argv[0] == "_child":
        child(argv[1:])
        return 0
    if not argv or argv[0] not in ("survey", "bench", "resources"):
        print(__doc__)
        return 2
    cmd, libs = argv[0], argv[1:] or LIBS
    for lib in libs:
        if lib not in LIBS:
            print(f"librairie inconnue : {lib} (connues : {', '.join(LIBS)})", file=sys.stderr)
            return 2
    out = out_dir()
    print(f"== {runtime()} ({sys.executable})")
    if cmd == "survey":
        path = out / "survey_python.jsonl"
        for lib in libs:
            print(f"== survey {lib}", flush=True)
            res, diag = spawn(["survey", lib])
            if not res:
                print(f"   (aucun résultat : {diag})")
            for r in res:
                print(f"   {r['op']}: {r['ok']}/{r['total']}" + (f"  ({r['note']})" if r.get("note") else ""))
            merge_jsonl(path, [lib], res)
        print(f"== {path}")
    else:
        # bench et resources : un processus fils par benchmark ; pour resources,
        # le fils ajoute son propre pic RSS (python -I harness.py _child resources …).
        count = int(os.environ.get("COUNT", "3"))
        benchtime = float(os.environ.get("BENCHTIME", "1").rstrip("s"))
        path = out / f"{cmd}_python.jsonl"
        for lib in libs:
            print(f"== {cmd} {lib}", flush=True)
            res_lib = []
            for bench in BENCHES:
                res, diag = spawn([cmd, lib, bench, str(benchtime), str(count)])
                if not res:
                    ad_pkg = lib
                    res = [{"lang": "python", "lib": lib, "pkg": ad_pkg, "version": "?", "bench": bench,
                            "status": "fail", "ns_per_op": [], "cpu_ns_per_op": [], "iters": 0, "first_call_ns": None,
                            "note": "le processus de mesure a échoué : " + diag, "runtime": runtime()}]
                r = res[0]
                if r["status"] == "ok":
                    med = sorted(r["ns_per_op"])[len(r["ns_per_op"]) // 2]
                    if cmd == "resources":
                        cpu_med = sorted(r["cpu_ns_per_op"])[len(r["cpu_ns_per_op"]) // 2]
                        print(f"   {bench:<20} {med:>14.0f} ns/op {cpu_med:>14.0f} cpu-ns/op "
                              f"{r['max_rss_bytes'] / 2**20:>8.1f} MiB")
                    else:
                        print(f"   {bench:<20} {med:>14.0f} ns/op  x{r['iters']}")
                elif r["status"] == "fail":
                    print(f"   {bench:<20} FAIL {r['note']}")
                else:
                    print(f"   {bench:<20} unsupported")
                res_lib.append(r)
            merge_jsonl(path, [lib], res_lib)
        print(f"== {path}")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
