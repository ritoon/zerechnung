#!/usr/bin/env python3
"""Collect what changed in ZeRechnung since the compliance matrix was last updated.

Usage:
  gather_state.py [--repo PATH] [--data FILE] [--wiki PATH] [--no-fetch]

--repo   a checkout or worktree of ritoon/zerechnung (default: the git top level of the current directory)
--data   the matrix (default: data/matrix.json of the skill)
--wiki   a clone of the project wiki, to print its date and the section 11 table
--no-fetch  skip `git fetch origin`

Prints a Markdown report on stdout: commits and merged pull requests since the
matrix's commit, the state of every issue the matrix links, the open
compliance issues it does not link yet, and the facts of origin/main the
statuses rest on (format packages, rule sets, signing APIs, QR code users,
the README support table, site/data/compliance.json). It reads and fetches,
it never writes.
"""
import argparse
import json
import re
import subprocess
import sys
from pathlib import Path

SKILL = Path(__file__).resolve().parent.parent
REF = "origin/main"


def run(cmd, cwd=None, check=False):
    p = subprocess.run(cmd, cwd=cwd, capture_output=True, text=True)
    if check and p.returncode != 0:
        raise RuntimeError(f"{' '.join(cmd)}: {p.stderr.strip()}")
    return p.stdout if p.returncode == 0 else None


def git(repo, *args):
    return run(["git", "-C", str(repo), *args])


def section(title):
    print(f"\n## {title}\n")


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--repo")
    ap.add_argument("--data", default=str(SKILL / "data" / "matrix.json"))
    ap.add_argument("--wiki")
    ap.add_argument("--no-fetch", action="store_true")
    args = ap.parse_args()

    repo = Path(args.repo or (run(["git", "rev-parse", "--show-toplevel"]) or ".").strip())
    data = json.loads(Path(args.data).read_text(encoding="utf-8"))
    slug = data["repo"]
    remotes = git(repo, "remote", "-v") or ""
    if slug not in remotes:
        print(f"error: {repo} is not a checkout of {slug} (git remote -v does not mention it)", file=sys.stderr)
        return 1
    if not args.no_fetch:
        git(repo, "fetch", "-q", "origin")

    head = (git(repo, "log", "-1", "--format=%h %cs %s", REF) or "?").strip()
    asof = data["asof"]
    print(f"# État de {slug}\n")
    print(f"- Dépôt : `{repo}`")
    print(f"- {REF} : {head}")
    print(f"- Matrice : commit {asof['commit']} du {asof['date']}")

    section(f"Commits de {REF} depuis {asof['commit']}")
    known = git(repo, "cat-file", "-e", asof["commit"] + "^{commit}") is not None
    if not known:
        print(f"Le commit {asof['commit']} est inconnu de ce dépôt : comparer avec les PR fusionnées ci-dessous.")
    else:
        log = (git(repo, "log", "--oneline", "--no-decorate", f"{asof['commit']}..{REF}") or "").strip()
        print("\n".join(f"- {l}" for l in log.splitlines()) if log else "Aucun : la matrice est à jour avec le code.")

    # PRs merged after the matrix's commit (its committer time), or since its date when the commit is unknown.
    since = (git(repo, "log", "-1", "--format=%cI", asof["commit"]) or "").strip() if known else ""
    section("PR fusionnées depuis " + (f"le commit {asof['commit']}" if since else f"le {asof['date']}"))
    prs = run(["gh", "pr", "list", "-R", slug, "--state", "merged", "--search", f"merged:>={asof['date']}",
               "--limit", "200", "--json", "number,title,mergedAt,mergeCommit"])
    if prs is None:
        print("gh n'a pas répondu (authentification ?) : liste non disponible.")
    else:
        rows = sorted(json.loads(prs), key=lambda p: p["mergedAt"])
        if since:
            # GitHub gives mergedAt in UTC (Z); git gives the committer offset: compare as aware datetimes.
            from datetime import datetime
            t0 = datetime.fromisoformat(since)
            rows = [p for p in rows if datetime.fromisoformat(p["mergedAt"].replace("Z", "+00:00")) > t0
                    and not (p.get("mergeCommit") or {}).get("oid", "").startswith(asof["commit"])]
        print("\n".join(f"- #{p['number']} {p['title']} ({p['mergedAt'][:10]})" for p in rows) or "Aucune.")

    issues_json = run(["gh", "issue", "list", "-R", slug, "--state", "all", "--limit", "500",
                       "--json", "number,title,state,closedAt,labels"])
    linked = {i: c["name"] for c in data["countries"] for i in c.get("issues", [])}
    section("Issues liées à la matrice")
    if issues_json is None:
        print("gh n'a pas répondu : état des issues non disponible.")
        issues = []
    else:
        issues = json.loads(issues_json)
        by_num = {i["number"]: i for i in issues}
        closed = []
        for n in sorted(linked):
            i = by_num.get(n)
            if i is None:
                print(f"- #{n} ({linked[n]}) : introuvable")
            elif i["state"] != "OPEN":
                closed.append(f"- #{n} ({linked[n]}) : FERMÉE le {(i['closedAt'] or '')[:10]} — {i['title']}")
        print("\n".join(closed) if closed else "Toutes ouvertes.")
        print(f"\n({len(linked)} issues liées ; une issue fermée signale en général un statut à passer à « couvert » et une issue à retirer.)")

        section("Issues ouvertes « conformité » non liées")
        loose = [i for i in issues if i["state"] == "OPEN" and i["number"] not in linked
                 and any(l["name"] == "conformité" for l in i["labels"])]
        print("\n".join(f"- #{i['number']} {i['title']}" for i in sorted(loose, key=lambda i: i["number"])) or "Aucune.")

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
        print(f"\n`site/data/compliance.json` (au {sj.get('asof', '?')}), à confronter à la matrice :\n")
        for c in sj.get("countries", []):
            print(f"- {c['code']} : " + ", ".join(f"{n}={s}" for n, s in c.get("cover", [])))

    if args.wiki:
        w = Path(args.wiki)
        section("Wiki")
        print("Derniers commits :")
        print("\n".join(f"- {l}" for l in (git(w, "log", "-5", "--oneline", "--no-decorate") or "").splitlines()))
        page = next(w.glob("Normes-europ*-par-pays.md"), None)
        if page:
            text = page.read_text(encoding="utf-8")
            print(f"\n{page.name} : {text.splitlines()[0][:160]}…")
            m = re.search(r"## 11\..*?\n(.*?)\n## 12\.", text, re.S)
            if m:
                rows = [l for l in m.group(1).splitlines() if l.startswith("|") and not l.startswith("|---")]
                print("\nSection 11 (formats face aux jeux de règles) :\n")
                print("\n".join(rows))
    return 0


if __name__ == "__main__":
    sys.exit(main())
