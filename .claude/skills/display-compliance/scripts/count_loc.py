#!/usr/bin/env python3
"""Count the lines of Go code of ZeRechnung per country, for the pie charts of the widget.

Usage:
  count_loc.py [--repo PATH] [--data FILE] [--map FILE] [--ref REF]

--repo  a checkout or worktree of ritoon/zerechnung (default: the git top level of the current directory)
--data  the matrix (default: data/matrix.json of the skill); its "loc" key is rewritten
--map   the rules that tie files to countries (default: data/loc_map.json of the skill)
--ref   the commit to count (default: origin/main; fetch it first)

A line of code is a non-blank line that is not a comment, in a .go file that is
not a test. Generated code is the *_gen.go files (the gen/main.go generators are
written by hand). Each file goes to the first rule of the map that matches it;
a file that no rule matches stops the count, so that a new package is never
left out silently: add a rule to the map and run again.

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


def group_countries(spec, countries):
    if spec.get("split") is False:
        return None
    if "countries" in spec:
        return list(spec["countries"])
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
    ap.add_argument("--data", default=str(SKILL / "data" / "matrix.json"))
    ap.add_argument("--map", default=str(SKILL / "data" / "loc_map.json"))
    ap.add_argument("--ref", default="origin/main")
    args = ap.parse_args()

    repo = args.repo or subprocess.run(["git", "rev-parse", "--show-toplevel"], capture_output=True, text=True).stdout.strip() or "."
    data = json.loads(Path(args.data).read_text(encoding="utf-8"))
    cmap = json.loads(Path(args.map).read_text(encoding="utf-8"))
    if data["repo"] not in git(repo, "remote", "-v"):
        print(f"error: {repo} is not a checkout of {data['repo']}", file=sys.stderr)
        return 1

    codes = {c["code"] for c in data["countries"]}
    groups = {k: {"label": g["label"], "countries": group_countries(g, data["countries"]), **({"base": True} if g.get("base") else {}),
                  **({"parts": g["parts"]} if g.get("parts") else {}),
                  **({"short": g["short"]} if g.get("short") else {})}
              for k, g in cmap["groups"].items()}
    problems = []
    for typ, target, _ in cmap["rules"]:
        if typ == "skip":
            continue
        if target.startswith("g:"):
            g = groups.get(target[2:])
            if g is None:
                problems.append(f"rule target {target}: no such group")
            elif g["countries"] == []:
                problems.append(f"group {target[2:]}: no country of the matrix uses it")
            elif g["countries"] and not set(g["countries"]) <= codes:
                problems.append(f"group {target[2:]}: unknown countries {sorted(set(g['countries']) - codes)}")
        elif target not in codes:
            problems.append(f"rule target {target}: not a country of the matrix")
    if problems:
        for p in problems:
            print(f"error: {p}", file=sys.stderr)
        return 1

    commit = git(repo, "rev-parse", "--short", args.ref).strip()
    files = [f for f in git(repo, "ls-tree", "-r", "--name-only", args.ref).split()
             if f.endswith(".go") and not f.endswith("_test.go")]
    units, unmatched = {}, []
    for f in files:
        rule = next(((t, g, p) for t, g, pats in cmap["rules"] for p in pats if match(f, p)), None)
        if rule is None:
            unmatched.append(f)
            continue
        typ, target, pat = rule
        if typ == "skip":
            continue
        u = units.setdefault((typ, target, label_of(f, pat, target)), {"code": 0, "gen": 0})
        u["gen" if f.endswith("_gen.go") else "code"] += sloc(git(repo, "show", f"{args.ref}:{f}"))
    unnamed = sorted({f"{t[2:]}: {d}" for (_, t, d) in units
                      if t.startswith("g:") and "parts" in groups[t[2:]] and d not in groups[t[2:]]["parts"]})
    if unnamed:
        print("error: these folders have no name in the parts of their group in data/loc_map.json:", file=sys.stderr)
        for x in unnamed:
            print(f"  {x}", file=sys.stderr)
        return 1
    if unmatched:
        print("error: no rule of the map matches these files; add one to data/loc_map.json:", file=sys.stderr)
        for f in unmatched:
            print(f"  {f}", file=sys.stderr)
        return 1

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
