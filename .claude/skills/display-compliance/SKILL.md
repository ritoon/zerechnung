---
name: display-compliance
description: Met à jour et affiche la matrice de conformité de ZeRechnung (ritoon/zerechnung, dépôt local factur-x) - pays par pays en Europe, les normes de facture électronique (Factur-X, ZUGFeRD, XRechnung, FatturaPA, Facturae, CIUS nationales, KSeF, ISDOC…), les fonctionnalités (PDF hybride, signature, QR code, plateforme, e-reporting), leur statut couvert, partiel, à faire ou hors périmètre et les issues GitHub liées, dans un widget interactif avec drapeaux. Utilise cette skill dès que l'utilisateur parle de la matrice, du tableau de conformité, de la couverture des normes ou des pays, de « ce qui est coché », de l'état d'un pays (France, Espagne, Pologne…), de ce qu'il reste à faire ou de la prochaine norme à attaquer dans ZeRechnung, après la fusion d'une PR de conformité, ou quand il tape /display-compliance, même s'il ne prononce pas le mot « matrice ».
---

# display-compliance

La matrice dit, pour chaque pays d'Europe, quelles normes de facture électronique
existent, quelles fonctionnalités elles imposent, et ce que ZeRechnung couvre au
commit noté. L'utilisateur s'en sert comme plan de travail : il choisit le pays, la
norme ou la fonctionnalité à attaquer en voyant ce qui est coché, partiel ou non
coché. Elle doit donc dire vrai sur `origin/main`, sans anticiper le travail en revue.

## Fichiers de la skill

La skill vit dans le dépôt, sous `.claude/skills/display-compliance/`. Les commandes
ci-dessous partent de la racine du checkout ou du worktree courant, avec :

```bash
S=.claude/skills/display-compliance
M=<scratchpad>/matrix.json
```

- `data/matrix.json` : la matrice, versionnée. En-tête `repo` et `asof` (`commit`,
  `date`, `wiki`), puis un objet par pays : `code` (ISO 3166-1 alpha-2, il sert aussi
  au drapeau), `name`, `region` (`ue`, `aele`, `autre`), `issues` (numéros), `norms` et
  `features`. Une norme : `label`, `status` (`y`, `p`, `n`, `x`), `note` facultative,
  `future: true` pour une norme pas encore en vigueur. Une fonctionnalité : `kind`
  (`pdf`, `sig`, `qr`, `net`, `rep`), `label`, `status`, `note`. La clé `loc`, écrite
  par `count_loc.py`, porte les lignes de code par pays des camemberts.
- `data/loc_map.json` : les règles qui rattachent chaque dossier ou fichier Go à un
  pays ou à un groupe de pays (socle EN 16931, Factur-X, Peppol, PDF hybride…) pour
  les camemberts.
- `assets/widget.html` : le gabarit du widget ; `build` remplace `__MATRIX_DATA__`
  par le JSON. Pour changer l'apparence, modifier ce fichier, jamais le HTML généré.
- `scripts/gather_state.py` : rapport en lecture seule de ce qui a changé depuis
  `asof.commit` (commits, PR fusionnées, issues liées fermées, issues « conformité »
  non liées) et des faits de `origin/main` sur lesquels reposent les statuts.
- `scripts/build_widget.py` : `check`, `fmt`, `compare ANCIEN`, `build --out FICHIER`,
  `table` (tableau Markdown, pour un client sans widget). Tous prennent `--data`.
- `scripts/count_loc.py` : compte les lignes de code de `origin/main` par pays d'après
  `data/loc_map.json` et les écrit dans la clé `loc` de `--data`. Il s'arrête sur tout
  fichier qu'aucune règle ne rattache, et sur tout dossier du socle sans nom dans ses
  `parts`.
- `references/criteria.md` : les règles de statut et les décisions passées. À lire
  avant de changer un statut ou d'ajouter une norme.

## Partir de la matrice de main

La branche courante peut être en retard sur `origin/main`, ou porter une matrice en
cours de revue. Travailler donc sur une copie de celle de `origin/main`, dans le
scratchpad, et passer `--data "$M"` à chaque script :

```bash
git fetch -q origin
git show "origin/main:$S/data/matrix.json" > "$M" 2>/dev/null || cp "$S/data/matrix.json" "$M"
```

(Le repli sur la copie locale ne sert que tant que `origin/main` n'a pas la skill.)
Afficher la matrice d'une branche précise seulement si l'utilisateur le demande.

## Choisir le mode

- **Affichage seul** (« affiche la matrice », « montre-moi le tableau ») : afficher la
  copie, après un contrôle rapide de fraîcheur.
- **Mise à jour** (« mets à jour la matrice », une PR de conformité vient d'être
  fusionnée, question sur l'état actuel d'un pays, ou le contrôle de fraîcheur montre
  des commits de conformité) : suivre le déroulé complet.

Contrôle de fraîcheur :

```bash
git log --oneline "$(python3 -c 'import json,sys;print(json.load(open(sys.argv[1]))["asof"]["commit"])' "$M")"..origin/main
```

En affichage seul, s'il y a des commits, afficher quand même et le signaler en une
ligne (combien, lesquels touchent la conformité), en proposant la mise à jour.

## Mise à jour

1. **Le wiki.** Dans le scratchpad de la session :
   `git clone -q git@github.com:ritoon/zerechnung.wiki.git <scratchpad>/wiki`, ou
   `git -C <scratchpad>/wiki pull -q` s'il existe déjà.
2. **Le rapport.**
   `python3 "$S/scripts/gather_state.py" --data "$M" --wiki <scratchpad>/wiki`
   (il lit `origin/main` et refuse un dossier qui n'est pas un checkout de
   `ritoon/zerechnung`).
3. **Une sauvegarde** avant de toucher aux données : `cp "$M" <scratchpad>/matrix.before.json`
4. **Les changements.** Pour chaque PR fusionnée et chaque issue liée fermée, lire ce
   qu'elle apporte (`gh pr view N -R ritoon/zerechnung --json title,body,closingIssuesReferences`),
   puis modifier `$M` d'après `references/criteria.md` : statut, note (citer la PR,
   « PR #NN »), liste `issues` du pays (retirer une issue fermée, ajouter une issue
   ouverte qui suit un manque). Confronter aussi le rapport aux statuts actuels : jeux
   de règles, API de signature, tableau du README, section 11 du wiki,
   `site/data/compliance.json`. Un écart entre ces sources est un point à vérifier,
   pas une raison de changer un statut au hasard.
   Mettre `asof.commit` au hash court de `origin/main`, `asof.date` à la date du jour
   et `asof.wiki` à la ligne d'état de la page wiki si elle a changé.
5. **Les lignes de code.** `python3 "$S/scripts/count_loc.py" --data "$M"`, à chaque
   mise à jour : le code change à chaque commit. S'il s'arrête sur des fichiers non
   rattachés (un nouveau paquet, un nouveau jeu de règles), ajouter une règle à
   `data/loc_map.json` d'après la section « Lignes de code » de
   `references/criteria.md`, et relancer. S'il s'arrête sur un dossier du socle sans
   nom, lui en donner un dans les `parts` du groupe `socle`.
6. **Le contrôle.** `python3 "$S/scripts/build_widget.py" fmt --data "$M"` (format
   canonique, une ligne par élément), puis `check --data "$M"` (régler les
   avertissements ou savoir dire pourquoi ils restent), puis
   `compare <scratchpad>/matrix.before.json --data "$M"` pour la liste des changements.
7. **L'affichage**, comme ci-dessous.
8. **La version.** Proposer de verser la mise à jour au dépôt, et attendre l'accord :
   une branche partie de `origin/main` (`git switch -c claude/matrice-AAAA-MM-JJ origin/main`
   si l'arbre de travail est propre, sinon un worktree à part), `cp "$M" "$S/data/matrix.json"`,
   un commit en français (« Met à jour la matrice de conformité après la PR #NN »),
   puis une PR que l'utilisateur fusionne lui-même. Une PR de conformité peut aussi
   porter la mise à jour de sa propre ligne : elle prend effet à la fusion.

S'il n'y a aucun commit depuis `asof.commit` et aucune issue liée fermée, ne rien
modifier : dire que la matrice est à jour et l'afficher.

## Afficher

1. `python3 "$S/scripts/build_widget.py" build --data "$M" --out <scratchpad>/compliance-widget.html`
   (le résumé chiffré sort sur stderr).
2. Si `mcp__visualize__read_me` n'a pas encore été appelé dans la conversation,
   l'appeler avec `modules: ["interactive", "chart"]`, sans le mentionner. Le widget
   dessine, sous la matrice, les lignes de code par pays quand la matrice a une clé
   `loc` ; sans elle, la section disparaît. Pour les normes : le socle EN 16931, base
   commune européenne, en disque central, et pour chaque pays une barre qui en dépasse
   de son code propre, la surface de la barre à l'échelle de celle du disque (dossiers
   du pays dans sa couleur, part du code partagé en gris). Pour les fonctionnalités :
   un camembert par pays.
3. Lire le fichier généré et passer son contenu tel quel, sans retouche, en
   `widget_code` à `mcp__visualize__show_widget`, avec le titre
   `zerechnung_compliance_matrix` et deux ou trois messages de chargement en français.
4. Sans outil de widget (Claude Code en terminal, par exemple), afficher la sortie de
   `python3 "$S/scripts/build_widget.py" table --data "$M"`.

## Répondre

En français, court, sans redonner le tableau que le widget montre déjà :

- l'état affiché : commit et date, et les commits arrivés depuis en affichage seul ;
- les changements de cette mise à jour, d'après `compare`, en phrases (« Pologne :
  KSeF FA(3) passe de à faire à couvert (PR #NN) ») ;
- les points à vérifier : écarts entre sources, informations non confirmées ;
- les lignes de code seulement si un pays ou un groupe bouge nettement (une nouvelle
  CIUS, un nouveau format), d'après la ligne « Lignes de code » de `compare` ;
- les nouveaux manques sans issue, avec la proposition d'en créer.

## Nouveaux manques

Une norme ou une fonctionnalité à faire ou partielle sans issue : proposer de créer
l'issue, et attendre l'accord. Avec l'accord, chercher d'abord un doublon
(`gh issue list -R ritoon/zerechnung --state all --search "<pays> <norme>"`), puis
`gh issue create -R ritoon/zerechnung --label enhancement --label conformité`, en
français, sur le modèle des issues #124 à #145 (« Le besoin », user stories, « Ce que
vous proposez », « À faire », hors périmètre). Ajouter ensuite son numéro au pays.

## Faire évoluer la structure

- Un pays : code ISO à deux lettres (le drapeau vient de flag-icons 7.2.3, qui
  connaît aussi `xk` pour le Kosovo), une région, au moins une norme.
- Un type de fonctionnalité : l'ajouter à `KINDS` dans `build_widget.py` et à `FK`
  dans `assets/widget.html`, avec une icône Tabler.
- Un nouveau paquet ou jeu de règles : une règle dans `data/loc_map.json`, avant la
  règle plus générale qui l'avalerait (la première qui correspond l'emporte). Un code
  partagé par plusieurs pays : un groupe dans `groups`, avec ses pays en liste fixe
  ou déduits de la matrice. Un nouveau dossier du socle : un nom dans ses `parts`. Un
  nouveau groupe de normes : un nom court (`short`), l'étiquette de sa barre quand le
  code partagé est montré à part. Un nouveau document de reporting (comme `cdar` ou
  `ereporting`) : aux fonctionnalités de son pays, comme la matrice le range.
- Un changement de format des données : adapter `validate`, `canonical` et `compare`
  dans `build_widget.py`, et le rendu du gabarit.
