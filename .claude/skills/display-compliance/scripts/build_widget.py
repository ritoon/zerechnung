#!/usr/bin/env python3
"""Validate the compliance matrix and build the widget that displays it.

Usage:
  build_widget.py check   [--data FILE]                 validate and print the summary
  build_widget.py build   [--data FILE] [--out FILE]    validate, then write the widget HTML
  build_widget.py fmt     [--data FILE]                 rewrite the data file in its canonical layout
  build_widget.py compare OLD [--data FILE]             list what changed since OLD
  build_widget.py table   [--data FILE]                 print the matrix as a Markdown table

FILE defaults to data/matrix.json next to this script's folder. build writes to
stdout when --out is omitted; the summary always goes to stderr so that stdout
stays the bare widget code.
"""
import argparse
import json
import re
import sys
from pathlib import Path

SKILL = Path(__file__).resolve().parent.parent
DEFAULT_DATA = SKILL / "data" / "matrix.json"
TEMPLATE = SKILL / "assets" / "widget.html"
PLACEHOLDER = "__MATRIX_DATA__"

STATUSES = {"y": "couvert", "p": "partiel", "n": "à faire", "x": "hors périmètre"}
KINDS = {"pdf": "PDF hybride", "sig": "Signature", "qr": "QR code", "net": "Plateforme", "rep": "E-reporting"}
REGIONS = {"ue", "aele", "autre"}


def load(path):
    with open(path, encoding="utf-8") as fh:
        return json.load(fh)


def validate(data):
    """Return (errors, warnings) as lists of strings."""
    errors, warnings = [], []
    if not isinstance(data.get("repo"), str) or "/" not in data.get("repo", ""):
        errors.append('"repo" must be "owner/name"')
    asof = data.get("asof") or {}
    if not re.fullmatch(r"[0-9a-f]{7,40}", str(asof.get("commit", ""))):
        errors.append('"asof.commit" must be a commit hash')
    if not re.fullmatch(r"\d{4}-\d{2}-\d{2}", str(asof.get("date", ""))):
        errors.append('"asof.date" must be YYYY-MM-DD')
    countries = data.get("countries")
    if not isinstance(countries, list) or not countries:
        errors.append('"countries" must be a non-empty list')
        return errors, warnings
    seen = set()
    for i, c in enumerate(countries):
        where = c.get("code") or f"countries[{i}]"
        code = c.get("code", "")
        if not re.fullmatch(r"[A-Z]{2}", code):
            errors.append(f"{where}: code must be two capital letters (ISO 3166-1 alpha-2)")
        if code in seen:
            errors.append(f"{where}: duplicate country")
        seen.add(code)
        if not c.get("name"):
            errors.append(f"{where}: missing name")
        if c.get("region") not in REGIONS:
            errors.append(f"{where}: region must be one of {sorted(REGIONS)}")
        norms = c.get("norms")
        if not isinstance(norms, list) or not norms:
            errors.append(f"{where}: norms must be a non-empty list")
            norms = []
        for n in norms:
            if not n.get("label"):
                errors.append(f"{where}: a norm has no label")
            if n.get("status") not in STATUSES:
                errors.append(f"{where}: norm {n.get('label')!r} has status {n.get('status')!r}, want y, p, n or x")
            elif n["status"] == "x":
                warnings.append(f"{where}: norm {n.get('label')!r} is out of scope (x); a format is usually y, p or n")
        for f in c.get("features", []):
            if f.get("kind") not in KINDS:
                errors.append(f"{where}: feature {f.get('label')!r} has kind {f.get('kind')!r}, want one of {sorted(KINDS)}")
            if f.get("status") not in STATUSES:
                errors.append(f"{where}: feature {f.get('label')!r} has status {f.get('status')!r}")
            if not f.get("label"):
                errors.append(f"{where}: a feature has no label")
        issues = c.get("issues", [])
        if not all(isinstance(n, int) and n > 0 for n in issues) or len(set(issues)) != len(issues):
            errors.append(f"{where}: issues must be distinct issue numbers")
        gaps = [x for x in norms + c.get("features", []) if x.get("status") in ("n", "p")]
        if gaps and not issues:
            warnings.append(f"{where}: {len(gaps)} item(s) to do or partial, but no issue")
        for x in norms + c.get("features", []):
            for ref in re.findall(r"(?<!PR )#(\d+)", x.get("note", "")):
                if x.get("status") in ("n", "p") and int(ref) not in issues:
                    warnings.append(f"{where}: {x.get('label')!r} cites #{ref}, which is not in its issues")
    return errors, warnings


def summary(data):
    cs = data["countries"]
    norms = [n for c in cs for n in c["norms"]]
    count = {s: sum(1 for n in norms if n["status"] == s) for s in STATUSES}
    full = [c["code"] for c in cs if all(n["status"] == "y" for n in c["norms"])]
    issues = sorted({i for c in cs for i in c["issues"]})
    lines = [
        f"Matrice au commit {data['asof']['commit']} du {data['asof']['date']} : {len(cs)} pays, {len(norms)} normes",
        "  normes : " + ", ".join(f"{count[s]} {STATUSES[s]}" for s in "ypn") + (f", {count['x']} hors périmètre" if count["x"] else ""),
        f"  pays couverts à 100 % : {len(full)} ({', '.join(full)})",
    ]
    for k, label in KINDS.items():
        fs = [f for c in cs for f in c.get("features", []) if f["kind"] == k]
        if fs:
            lines.append(f"  {label} : {len(fs)} pays, " + ", ".join(
                f"{sum(1 for f in fs if f['status'] == s)} {STATUSES[s]}" for s in STATUSES if any(f["status"] == s for f in fs)))
    lines.append(f"  issues liées : {len(issues)}")
    return "\n".join(lines)


def canonical(data):
    """The data file with one line per norm and per feature, to keep diffs readable."""
    d = lambda v: json.dumps(v, ensure_ascii=False)
    out = ["{", f' "repo": {d(data["repo"])},', f' "asof": {d(data["asof"])},', ' "countries": [']
    cs = data["countries"]
    for i, c in enumerate(cs):
        out.append(f'  {{"code": {d(c["code"])}, "name": {d(c["name"])}, "region": {d(c["region"])}, "issues": {d(c.get("issues", []))},')
        out.append('   "norms": [')
        out.append(",\n".join(f"    {d(n)}" for n in c["norms"]))
        out.append("   ],")
        fs = c.get("features", [])
        if fs:
            out.append('   "features": [')
            out.append(",\n".join(f"    {d(f)}" for f in fs))
            out.append("   ]}" + ("," if i < len(cs) - 1 else ""))
        else:
            out.append('   "features": []}' + ("," if i < len(cs) - 1 else ""))
    out += [" ]", "}", ""]
    return "\n".join(out)


def table(data):
    """The matrix as a Markdown table, for clients that cannot display the widget."""
    sym = {"y": "✅", "p": "🟡", "n": "❌", "x": "—"}
    cell = lambda t: str(t).replace("|", "\\|")
    item = lambda z: f"{sym[z['status']]} {cell(z['label'])}" + (" (à venir)" if z.get("future") else "")
    out = [f"Matrice au commit {data['asof']['commit']} du {data['asof']['date']} ; "
           "✅ couvert, 🟡 partiel, ❌ à faire, — hors périmètre (transmission).", "",
           "| Pays | Normes | Détail des normes | Fonctionnalités | Issues |", "|---|---|---|---|---|"]
    for c in sorted(data["countries"], key=lambda c: c["name"]):
        y = sum(1 for n in c["norms"] if n["status"] == "y")
        out.append(f"| {cell(c['name'])} | {y}/{len(c['norms'])} | " + ", ".join(map(item, c["norms"]))
                   + " | " + (", ".join(map(item, c.get("features", []))) or "—")
                   + " | " + (" ".join(f"#{i}" for i in c.get("issues", [])) or "—") + " |")
    return "\n".join(out)


def compare(old, new):
    lines = []
    if old["asof"] != new["asof"]:
        lines.append(f"État : {old['asof']['commit']} ({old['asof']['date']}) → {new['asof']['commit']} ({new['asof']['date']})")
    oc = {c["code"]: c for c in old["countries"]}
    nc = {c["code"]: c for c in new["countries"]}
    for code in sorted(set(oc) | set(nc), key=lambda k: (nc.get(k) or oc.get(k))["name"]):
        a, b = oc.get(code), nc.get(code)
        name = (b or a)["name"]
        if a is None:
            lines.append(f"{name} : ajouté")
            continue
        if b is None:
            lines.append(f"{name} : retiré")
            continue
        changes = []
        for key, what in (("norms", "norme"), ("features", "fonctionnalité")):
            ka = {(x.get("kind", ""), x["label"]): x for x in a.get(key, [])}
            kb = {(x.get("kind", ""), x["label"]): x for x in b.get(key, [])}
            for k in kb.keys() - ka.keys():
                changes.append(f"+ {what} {k[1]!r} ({STATUSES[kb[k]['status']]})")
            for k in ka.keys() - kb.keys():
                changes.append(f"- {what} {k[1]!r}")
            for k in ka.keys() & kb.keys():
                if ka[k]["status"] != kb[k]["status"]:
                    changes.append(f"{what} {k[1]!r} : {STATUSES[ka[k]['status']]} → {STATUSES[kb[k]['status']]}")
                elif ka[k].get("note") != kb[k].get("note"):
                    changes.append(f"{what} {k[1]!r} : note mise à jour")
        ia, ib = set(a.get("issues", [])), set(b.get("issues", []))
        if ia != ib:
            changes.append("issues " + " ".join([f"+#{i}" for i in sorted(ib - ia)] + [f"-#{i}" for i in sorted(ia - ib)]))
        if changes:
            lines.append(f"{name} : " + " ; ".join(changes))
    return "\n".join(lines) or "Aucun changement."


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("command", choices=["check", "build", "fmt", "compare", "table"])
    ap.add_argument("old", nargs="?", help="previous data file, for compare")
    ap.add_argument("--data", default=str(DEFAULT_DATA))
    ap.add_argument("--out")
    args = ap.parse_args()

    data = load(args.data)
    if args.command == "compare":
        if not args.old:
            ap.error("compare needs the previous data file")
        print(compare(load(args.old), data))
        return 0

    errors, warnings = validate(data)
    for w in warnings:
        print(f"warning: {w}", file=sys.stderr)
    if errors:
        for e in errors:
            print(f"error: {e}", file=sys.stderr)
        return 1

    if args.command == "fmt":
        Path(args.data).write_text(canonical(data), encoding="utf-8")
        print(f"{args.data} réécrit", file=sys.stderr)
        return 0

    print(summary(data), file=sys.stderr)
    if args.command == "table":
        print(table(data))
    elif args.command == "build":
        literal = json.dumps(data, ensure_ascii=False, separators=(",", ":")).replace("</", "<\\/")
        html = TEMPLATE.read_text(encoding="utf-8").replace(PLACEHOLDER, literal)
        if args.out:
            Path(args.out).write_text(html, encoding="utf-8")
            print(f"widget écrit dans {args.out} ({len(html)} caractères)", file=sys.stderr)
        else:
            sys.stdout.write(html)
    return 0


if __name__ == "__main__":
    sys.exit(main())
