#!/usr/bin/env python3
"""Collect the sources the compliance matrix is built from, at each invocation of the skill.

Usage:
  gather_state.py --wiki PATH [--repo PATH] [--no-fetch]

--wiki      a clone of the project wiki (git@github.com:ritoon/zerechnung.wiki.git)
--repo      a checkout or worktree of ritoon/zerechnung (default: the git top level of the current directory)
--no-fetch  skip `git fetch origin`

The skill keeps no matrix: it builds one in the scratchpad each time, from what
this script prints on stdout, in Markdown:

- the header of the matrix (repo, origin/main commit and date, the wiki's date);
- the countries of the wiki page "Normes européennes par pays", with their
  region and the rows of their table (norm or system, scope, status, dates) and
  their notes, and its section 11, which sets each format against the rule sets;
- the facts of origin/main the statuses rest on: public packages, rule sets,
  signing APIs, users of the QR encoder, the README support table,
  site/data/compliance.json, the pull requests merged lately;
- the open compliance issues, by country.

It reads and fetches, it never writes.
"""
import argparse
import json
import re
import subprocess
import sys
from pathlib import Path

REPO = "ritoon/zerechnung"
REF = "origin/main"
# Outside the EU, the EEA, Switzerland and the United Kingdom are region "aele"; every other country is "autre".
AELE = {"IS", "LI", "NO", "CH", "GB"}
NOTE_MAX = 320


def run(cmd, cwd=None):
    p = subprocess.run(cmd, cwd=cwd, capture_output=True, text=True)
    return p.stdout if p.returncode == 0 else None


def git(repo, *args):
    return run(["git", "-C", str(repo), *args])


def section(title):
    print(f"\n## {title}\n")


def cells(line):
    return [c.strip() for c in line.strip().strip("|").split("|")]


def wiki_countries(text):
    """The countries of the page, in order: (code, name, region, rows, notes)."""
    eu = set()
    m = re.search(r"### Union européenne\n(.*?)\n### ", text, re.S)
    if m:
        eu = {a.rsplit("-", 1)[1].upper() for a in re.findall(r"\]\(#([^)]+)\)", m.group(1))}
    out = []
    for m in re.finditer(r"^### ([^\n]+?) \(([A-Z]{2})\)\n(.*?)(?=^##)", text, re.S | re.M):
        name, code, body = m.groups()
        rows, notes = [], []
        for line in body.splitlines():
            if line.startswith("|") and not line.startswith("|---"):
                c = cells(line)
                if c[0] != "Norme ou système":
                    rows.append(c[:4])
            elif line.startswith("- "):
                notes.append(line[2:] if len(line) <= NOTE_MAX else line[2:NOTE_MAX] + "…")
        intro = next((l for l in body.splitlines() if l and not l.startswith(("|", "-"))), "")
        region = "ue" if code in eu else "aele" if code in AELE else "autre"
        out.append((code, name, region, intro, rows, notes))
    return out


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--wiki", required=True)
    ap.add_argument("--repo")
    ap.add_argument("--no-fetch", action="store_true")
    args = ap.parse_args()

    repo = Path(args.repo or (run(["git", "rev-parse", "--show-toplevel"]) or ".").strip())
    if REPO not in (git(repo, "remote", "-v") or ""):
        print(f"error: {repo} is not a checkout of {REPO} (git remote -v does not mention it)", file=sys.stderr)
        return 1
    wiki = Path(args.wiki)
    page = next(wiki.glob("Normes-europ*-par-pays.md"), None)
    if page is None:
        print(f"error: {wiki} has no page Normes-européennes-par-pays.md; clone the wiki there", file=sys.stderr)
        return 1
    if not args.no_fetch:
        git(repo, "fetch", "-q", "origin")
        git(wiki, "pull", "-q")
    text = page.read_text(encoding="utf-8")

    commit, date, subject = (git(repo, "log", "-1", "--format=%h%x09%cs%x09%s", REF) or "?\t?\t?").strip().split("\t")
    m = re.match(r">\s*(État au [^.]+)\.", text)
    print(f"# Sources de la matrice de {REPO}\n")
    print("En-tête de la matrice :\n")
    print(f'- "repo": "{REPO}"')
    print(f'- "asof": {{"commit": "{commit}", "date": "{date}", "wiki": "Normes européennes par pays, '
          + (m.group(1)[0].lower() + m.group(1)[1:] if m else "état inconnu") + '"}')
    print(f"\n{REF} : {commit} du {date}, {subject}")

    section("Pays du wiki (Normes européennes par pays)")
    print("Une ligne par pays de la matrice, dans cet ordre de lecture. Colonnes des tableaux : norme ou système | périmètre | statut | dates.")
    for code, name, region, intro, rows, notes in wiki_countries(text):
        print(f"\n### {code} {name} (région {region})\n")
        if intro:
            print(intro)
        for r in rows:
            print("| " + " | ".join(r) + " |")
        for n in notes:
            print(f"- {n}")

    m = re.search(r"## 11\..*?\n(.*?)\n## 12\.", text, re.S)
    if m:
        section("Wiki, section 11 : les formats face aux jeux de règles")
        print("\n".join(l for l in m.group(1).splitlines() if l.startswith("|") and not l.startswith("|---")))

    section(f"Faits du code ({REF})")
    tree = git(repo, "ls-tree", "-r", "--name-only", REF) or ""
    files = set(tree.splitlines())
    pkgs = sorted({f.split("/")[0] for f in files if re.fullmatch(r"[a-z0-9]+/doc\.go", f)})
    print("Paquets publics avec doc.go : " + ", ".join(f"`{p}`" for p in pkgs))

    vg = git(repo, "show", f"{REF}:validate/validate.go") or ""
    m = re.search(r"Schema\s+RuleSet = 1 << iota(.*?)\n\s*Default\s*=", vg, re.S)
    if m:
        print("\nJeux de règles (`validate`) :")
        first = re.search(r"Schema\s+RuleSet = 1 << iota\s*//\s*(.*)", vg)
        if first:
            print(f"- `Schema` : {first.group(1).strip()}")
        for name, comment in re.findall(r"^\s*([A-Z]\w+)\s*//\s*(.*)$", m.group(1), re.M):
            print(f"- `{name}` : {comment.strip()}")

    apis = git(repo, "grep", "-n", "-E", r"^func (Sign[A-Za-z0-9_]*|Verify[A-Za-z0-9_]*|Unwrap|ReadSignature)\(", REF, "--",
               "*.go", ":!*_test.go", ":!internal/*", ":!server/*") or ""
    print("\nAPI publiques de signature :")
    print("\n".join(f"- {l.split(':', 1)[1]}" for l in apis.splitlines()) or "- aucune")

    qr = git(repo, "grep", "-l", "zerechnung/internal/qr", REF, "--", "*.go", ":!internal/qr/*", ":!conformance/*") or ""
    print("\nCode qui utilise l'encodeur `internal/qr` (hors tests de conformité) : "
          + (", ".join(f"`{l.split(':', 1)[1]}`" for l in qr.splitlines()) or "aucun, pas d'API QR publique"))

    readme = git(repo, "show", f"{REF}:README.md") or ""
    m = re.search(r"## What's supported\n(.*?)\n## ", readme, re.S)
    if m:
        rows = [l for l in m.group(1).splitlines() if l.startswith("|") and not l.startswith("|---")]
        print("\nTableau « What's supported » du README :\n")
        print("\n".join(rows))

    site = git(repo, "show", f"{REF}:site/data/compliance.json")
    if site:
        sj = json.loads(site)
        print(f"\n`site/data/compliance.json` (au {sj.get('asof', '?')}), souvent en retard d'une PR :\n")
        for c in sj.get("countries", []):
            print(f"- {c['code']} : " + ", ".join(f"{n}={s}" for n, s in c.get("cover", [])))

    log = git(repo, "log", "--format=%h %cs %s", "-60", REF) or ""
    merged = [l for l in log.splitlines() if re.search(r"\(#\d+\)$", l)]
    print("\nPR fusionnées récemment (à citer « PR #NN » dans les notes) :\n")
    print("\n".join(f"- {l}" for l in merged[:40]) or "- aucune")

    section("Issues ouvertes « conformité », par pays")
    issues = run(["gh", "issue", "list", "-R", REPO, "--state", "open", "--label", "conformité", "--limit", "300",
                  "--json", "number,title"])
    if issues is None:
        print("gh n'a pas répondu (authentification ?) : issues non disponibles, la matrice ne peut pas les lier.")
    else:
        by = {}
        for i in json.loads(issues):
            who = i["title"].split(" : ", 1)[0] if " : " in i["title"] else "(sans pays)"
            who = re.sub(r"^Prendre en charge (la |le |les |l')?", "", who)
            by.setdefault(who, []).append(i)
        for who in sorted(by):
            print(f"- {who} : " + " ; ".join(f"#{i['number']} {i['title'].split(' : ', 1)[-1]}"
                                             for i in sorted(by[who], key=lambda i: i["number"])))
    return 0


if __name__ == "__main__":
    sys.exit(main())
