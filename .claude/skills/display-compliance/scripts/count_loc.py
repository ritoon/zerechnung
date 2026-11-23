#!/usr/bin/env python3
"""Count the lines of Go code of ZeRechnung per country, for the pie charts of the widget.

Usage:
  count_loc.py --data FILE [--repo PATH] [--map FILE] [--extra FILE] [--ref REF]

--data   the matrix built for this invocation, in the scratchpad; its "loc" key is rewritten
--repo   a checkout or worktree of ritoon/zerechnung (default: the git top level of the current directory)
--map    the rules that tie files to countries (default: data/loc_map.json of the skill)
--extra  more rules, in the same format, tried before those of --map: for files the map
         does not know yet, written in the scratchpad at invocation
--ref    the commit to count (default: origin/main; fetch it first)

A line of code is a non-blank line that is not a comment, in a .go file that is
not a test. Generated code is the *_gen.go files (the gen/main.go generators are
written by hand). Each file goes to the first rule that matches it; a file
that no rule matches stops the count, so that a new package is never left out
silently: give it a rule with --extra and run again.

Nothing here is a fixed list of countries: a pattern can hold {cc}, any country
code of the matrix in lower case (internal/rules/{cc}/*), and the countries of
a group come from the matrix (a norm or a feature it covers) or from the code
(the countries whose files import a package).

The script writes the "loc" key of the data file, in the canonical layout of
build_widget.py, and prints the totals on stderr. It reads the repository, it
never writes there.
"""
import argparse
import fnmatch
import json
import re
import subprocess
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from build_widget import canonical  # noqa: E402

SKILL = Path(__file__).resolve().parent.parent


def git(repo, *args):
    p = subprocess.run(["git", "-C", str(repo), *args], capture_output=True, text=True)
    if p.returncode != 0:
        raise SystemExit(f"error: git {' '.join(args)}: {p.stderr.strip()}")
    return p.stdout


def match(path, pat):
    if "/" not in pat:  # a file at the root
        return "/" not in path and fnmatch.fnmatch(path, pat)
    return fnmatch.fnmatch(path, pat)


def find_rule(path, rules, codes):
    """The first rule that matches path, as (type, target, pattern), {cc} resolved."""
    for typ, target, pats in rules:
        for pat in pats:
            if "{cc}" not in pat:
                if match(path, pat):
                    return typ, target, pat
                continue
            for cc in codes:
                p = pat.replace("{cc}", cc.lower())
                if match(path, p):
                    return typ, cc if target == "{cc}" else target, p
    return None


def sloc(text):
    n, block = 0, False
    for line in text.splitlines():
        s = line.strip()
        if block:
            if "*/" not in s:
                continue
            block, s = False, s.split("*/", 1)[1].strip()
        if not s or s.startswith("//"):
            continue
        if s.startswith("/*"):
            block = "*/" not in s
            continue
        n += 1
    return n


def group_countries(spec, countries, importers):
    if spec.get("split") is False:
        return None
    if "imports" in spec:
        return sorted(importers.get(spec["imports"], set()))
    out = []
    for c in countries:
        if "norm" in spec:
            hit = any(re.search(spec["norm"], n["label"]) and n["status"] in ("y", "p") for n in c["norms"])
        else:
            f = spec["feature"]
            st = f.get("status", ["y", "p"])
            hit = any(x["kind"] == f["kind"] and re.search(f.get("label", "."), x["label"]) and x["status"] in st
                      for x in c.get("features", []))
        if hit:
            out.append(c["code"])
    return sorted(out)


def label_of(path, pat, target):
    """The folder of the file, or the precise pattern a country carves out of a shared folder."""
    if "/" not in path:
        return "(racine)"
    if pat.endswith("/*") or target.startswith("g:"):
        d = path.rsplit("/", 1)[0]
        d = d[:-4] if d.endswith("/gen") else d
    else:
        d = pat
    return d[len("internal/"):] if d.startswith("internal/") else d


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--repo")
    ap.add_argument("--data", required=True)
    ap.add_argument("--map", default=str(SKILL / "data" / "loc_map.json"))
    ap.add_argument("--extra")
    ap.add_argument("--ref", default="origin/main")
    args = ap.parse_args()

    repo = args.repo or subprocess.run(["git", "rev-parse", "--show-toplevel"], capture_output=True, text=True).stdout.strip() or "."
    data = json.loads(Path(args.data).read_text(encoding="utf-8"))
    cmap = json.loads(Path(args.map).read_text(encoding="utf-8"))
    extra = json.loads(Path(args.extra).read_text(encoding="utf-8")) if args.extra else []
    if isinstance(extra, dict):
        extra = extra.get("rules", [])
    if data["repo"] not in git(repo, "remote", "-v"):
        print(f"error: {repo} is not a checkout of {data['repo']}", file=sys.stderr)
        return 1

    codes = sorted(c["code"] for c in data["countries"])
    rules = extra + cmap["rules"]
    problems = []
    for typ, target, _ in rules:
        if typ == "skip" or target == "{cc}":
            continue
        if target.startswith("g:"):
            if target[2:] not in cmap["groups"]:
                problems.append(f"rule target {target}: no such group")
        elif target not in codes:
            problems.append(f"rule target {target}: not a country of the matrix")
    if problems:
        for p in problems:
            print(f"error: {p}", file=sys.stderr)
        return 1

    commit = git(repo, "rev-parse", "--short", args.ref).strip()
    module = re.search(r"^module\s+(\S+)", git(repo, "show", f"{args.ref}:go.mod"), re.M).group(1)
    watched = {g["imports"] for g in cmap["groups"].values() if "imports" in g}
    files = [f for f in git(repo, "ls-tree", "-r", "--name-only", args.ref).split()
             if f.endswith(".go") and not f.endswith("_test.go")]
    units, unmatched, importers = {}, [], {}
    for f in files:
        rule = find_rule(f, rules, codes)
        if rule is None:
            unmatched.append(f)
            continue
        typ, target, pat = rule
        if typ == "skip":
            continue
        text = git(repo, "show", f"{args.ref}:{f}")
        u = units.setdefault((typ, target, label_of(f, pat, target)), {"code": 0, "gen": 0})
        u["gen" if f.endswith("_gen.go") else "code"] += sloc(text)
        if not target.startswith("g:"):
            for w in watched:
                if f'"{module}/{w}"' in text:
                    importers.setdefault(w, set()).add(target)
    if unmatched:
        print("error: no rule matches these files; give them one in an --extra file (see the Lignes de code section "
              "of references/criteria.md) and run again:", file=sys.stderr)
        for f in unmatched:
            print(f"  {f}", file=sys.stderr)
        return 1

    groups = {k: {"label": g["label"], "countries": group_countries(g, data["countries"], importers),
                  **({"base": True} if g.get("base") else {}),
                  **({"parts": g["parts"]} if g.get("parts") else {}),
                  **({"short": g["short"]} if g.get("short") else {})}
              for k, g in cmap["groups"].items()}
    used = {t[2:] for (_, t, _) in units if t.startswith("g:")}
    empty = sorted(k for k in used if groups[k]["countries"] == [])
    if empty:
        print("error: no country of the matrix uses these groups, though code goes to them: " + ", ".join(empty), file=sys.stderr)
        return 1
    unnamed = sorted({f"{t[2:]}: {d}" for (_, t, d) in units
                      if t.startswith("g:") and "parts" in groups[t[2:]] and d not in groups[t[2:]]["parts"]})
    if unnamed:
        print("warning: these folders have no name in the parts of their group; the widget shows their path:", file=sys.stderr)
        for x in unnamed:
            print(f"  {x}", file=sys.stderr)

    data["loc"] = {
        "commit": commit,
        "groups": groups,
        "units": [{"type": t, "target": g, "dir": d, **u} for (t, g, d), u in sorted(units.items()) if u["code"] or u["gen"]],
    }
    Path(args.data).write_text(canonical(data), encoding="utf-8")

    tot = {}
    for u in data["loc"]["units"]:
        k = (u["type"], u["target"])
        tot[k] = tot.get(k, 0) + u["code"] + u["gen"]
    names = {c["code"]: c["name"] for c in data["countries"]}
    print(f"Lignes de code au commit {commit}, écrites dans {args.data} :", file=sys.stderr)
    for typ, what in (("norm", "normes"), ("feat", "fonctionnalités")):
        rows = sorted(((v, k[1]) for k, v in tot.items() if k[0] == typ), reverse=True)
        print(f"  {what} : {sum(v for v, _ in rows)} lignes", file=sys.stderr)
        for v, t in rows:
            g = groups.get(t[2:]) if t.startswith("g:") else None
            who = f"{g['label']} ({', '.join(g['countries']) if g['countries'] else 'tous'})" if g else names[t]
            print(f"    {v:6d}  {who}", file=sys.stderr)
    return 0


if __name__ == "__main__":
    sys.exit(main())
