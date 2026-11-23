---
name: display-compliance
description: Calcule et affiche la matrice de conformité de ZeRechnung (ritoon/zerechnung, dépôt local factur-x) - pays par pays en Europe, les normes de facture électronique (Factur-X, ZUGFeRD, XRechnung, FatturaPA, Facturae, CIUS nationales, KSeF, ISDOC…), les fonctionnalités (PDF hybride, signature, QR code, plateforme, e-reporting), leur statut couvert, partiel, à faire ou hors périmètre, les issues GitHub liées et les lignes de code par pays, dans un widget interactif avec drapeaux. Tout est recalculé à chaque appel depuis le code de origin/main, le wiki et les issues. Utilise cette skill dès que l'utilisateur parle de la matrice, du tableau de conformité, de la couverture des normes ou des pays, de « ce qui est coché », de l'état d'un pays (France, Espagne, Pologne…), de ce qu'il reste à faire ou de la prochaine norme à attaquer dans ZeRechnung, après la fusion d'une PR de conformité, ou quand il tape /display-compliance, même s'il ne prononce pas le mot « matrice ».
---

# display-compliance

La matrice dit, pour chaque pays d'Europe, quelles normes de facture électronique
existent, quelles fonctionnalités elles imposent, et ce que ZeRechnung couvre sur
`origin/main`. L'utilisateur s'en sert comme plan de travail : il choisit le pays, la
norme ou la fonctionnalité à attaquer en voyant ce qui est coché, partiel ou non
coché. Elle doit donc dire vrai sur `origin/main`, sans anticiper le travail en revue.

**La skill ne garde aucune donnée.** Elle décrit comment calculer la matrice et
comment l'afficher ; la matrice et les lignes de code se recalculent à chaque appel,
dans le scratchpad, depuis les sources. On ne modifie la skill que pour changer son
comportement (méthode, critères, règles de rattachement, rendu), jamais pour y
noter un état : un statut, un pays, un compte de lignes ou une issue ne s'y écrivent
pas.

## Fichiers de la skill

La skill vit dans le dépôt, sous `.claude/skills/display-compliance/`.

- `references/criteria.md` : comment passer des sources à la matrice (ce qui est une
  norme ou une fonctionnalité, les statuts, les notes, le rattachement des lignes de
  code). À lire avant chaque calcul.
- `scripts/gather_state.py` : rassemble les sources en un rapport Markdown, en lecture
  seule (voir « Calculer la matrice »).
- `scripts/count_loc.py` : compte les lignes de code de `origin/main` par pays et les
  écrit dans la clé `loc` de la matrice.
- `data/loc_map.json` : la méthode de rattachement des fichiers Go à un pays ou à un
  groupe de pays (socle EN 16931, Factur-X, Peppol, PDF hybride…). Aucun pays n'y est
  figé : les pays des groupes se déduisent de la matrice ou des imports du code.
- `scripts/build_widget.py` : `check`, `fmt`, `build --out FICHIER`, `table` (tableau
  Markdown, pour un client sans widget) et `compare ANCIENNE` (deux matrices
  calculées). Tous prennent `--data`.
- `assets/widget.html` : le gabarit du widget ; `build` remplace `__MATRIX_DATA__` par
  le JSON. Pour changer l'apparence, modifier ce fichier, jamais le HTML généré.

## Format de la matrice

Un objet JSON : `repo`, `asof` (`commit`, `date`, `wiki`), `countries`, et `loc`, que
`count_loc.py` écrit. Un pays : `code` (ISO 3166-1 alpha-2, il sert aussi au drapeau),
`name`, `region` (`ue`, `aele`, `autre`), `issues` (numéros), `norms` et `features`.
Une norme : `label`, `status` (`y`, `p`, `n`, `x`), `note` facultative, `future: true`
pour une norme pas encore en vigueur. Une fonctionnalité : `kind` (`pdf`, `sig`, `qr`,
`net`, `rep`), `label`, `status`, `note`. `build_widget.py check` valide le tout.

## Calculer la matrice

À chaque appel, même pour un simple affichage. Les commandes partent de la racine du
checkout ou du worktree courant :

```bash
S=.claude/skills/display-compliance
W=<scratchpad>/wiki
M=<scratchpad>/matrix.json
```

1. **Le wiki.** `git clone -q git@github.com:ritoon/zerechnung.wiki.git "$W"`, s'il
   n'est pas déjà dans le scratchpad (le script le met à jour sinon).
2. **Les sources.** `python3 "$S/scripts/gather_state.py" --wiki "$W" > <scratchpad>/sources.md`,
   puis lire le rapport en entier. Il donne l'en-tête de la matrice, les pays du wiki
   (région, tableau des normes, notes), la section 11 du wiki (formats face aux jeux
   de règles), les faits du code de `origin/main` (paquets, jeux de règles, API de
   signature, encodeur QR, tableau du README, `site/data/compliance.json`, PR
   fusionnées récemment) et les issues ouvertes « conformité », par pays. Il refuse un
   dossier qui n'est pas un checkout de `ritoon/zerechnung`.
3. **La matrice.** Écrire `$M` d'après `references/criteria.md` : l'en-tête tel que le
   rapport le donne, puis un pays par section du wiki, dans son ordre, avec ses normes
   et ses fonctionnalités, leurs statuts tirés des faits du code, des notes courtes et
   les issues ouvertes qui suivent ses manques. Ne rien reprendre d'une matrice
   affichée plus tôt : chaque statut se décide sur les sources du jour.
4. **Les lignes de code.** `python3 "$S/scripts/count_loc.py" --data "$M"`. S'il
   s'arrête sur des fichiers qu'aucune règle ne rattache (un nouveau paquet, un nouveau
   jeu de règles), écrire `<scratchpad>/loc_extra.json` (`{"rules": [...]}`, même
   format que `data/loc_map.json`) d'après la section « Lignes de code » de
   `references/criteria.md`, et relancer avec `--extra <scratchpad>/loc_extra.json`.
   Le dire à l'utilisateur dans la réponse ; si ces règles doivent durer, proposer de
   les verser dans `data/loc_map.json`, ce qui change le comportement de la skill.
5. **Le contrôle.** `python3 "$S/scripts/build_widget.py" fmt --data "$M"`, puis
   `check --data "$M"`. Régler les avertissements, ou savoir dire pourquoi ils restent :
   une norme ou une fonctionnalité `n` ou `p` sans issue est un nouveau manque.

Si l'utilisateur demande ce qui a changé depuis un affichage précédent de la même
conversation, garder l'ancienne matrice du scratchpad et lancer
`compare <ancienne> --data "$M"`.

## Afficher

1. `python3 "$S/scripts/build_widget.py" build --data "$M" --out <scratchpad>/compliance-widget.html`
   (le résumé chiffré sort sur stderr).
2. Si `mcp__visualize__read_me` n'a pas encore été appelé dans la conversation,
   l'appeler avec `modules: ["interactive", "chart"]`, sans le mentionner. Le widget
   dessine, sous la matrice, les lignes de code par pays. Pour les normes : le socle
   EN 16931, base commune européenne, en disque central, et pour chaque pays une barre
   qui en dépasse de son code propre, la surface de la barre à l'échelle de celle du
   disque (dossiers du pays dans sa couleur, part du code partagé en gris). Pour les
   fonctionnalités : un camembert par pays.
3. Lire le fichier généré et passer son contenu tel quel, sans retouche, en
   `widget_code` à `mcp__visualize__show_widget`, avec le titre
   `zerechnung_compliance_matrix` et deux ou trois messages de chargement en français.
4. Sans outil de widget (Claude Code en terminal, par exemple), afficher la sortie de
   `python3 "$S/scripts/build_widget.py" table --data "$M"`.

## Répondre

En français, court, sans redonner le tableau que le widget montre déjà :

- l'état calculé : commit et date de `origin/main`, date du wiki ;
- les points à vérifier : écarts entre sources (le code l'emporte, la page à corriger
  est à nommer), informations non confirmées ;
- les nouveaux manques sans issue, avec la proposition d'en créer ;
- les fichiers rattachés à l'invocation par `--extra`, s'il y en a ;
- les lignes de code seulement si un pays ou un groupe se détache nettement.

## Nouveaux manques

Une norme ou une fonctionnalité à faire ou partielle sans issue : proposer de créer
l'issue, et attendre l'accord. Avec l'accord, chercher d'abord un doublon
(`gh issue list -R ritoon/zerechnung --state all --search "<pays> <norme>"`), puis
`gh issue create -R ritoon/zerechnung --label enhancement --label conformité`, en
français : « Le besoin », user stories, « Ce que vous proposez », « À faire », hors
périmètre. Le titre commence par le pays (« Pologne : … ») : c'est ce qui la range
sous son pays dans le rapport des sources, et donc dans la matrice du prochain appel.

## Faire évoluer la skill

Seulement pour changer son comportement :

- Une façon de décider d'un statut ou de classer une norme : `references/criteria.md`.
- Un type de fonctionnalité : l'ajouter à `KINDS` dans `build_widget.py` et à `FK`
  dans `assets/widget.html`, avec une icône Tabler.
- Une source de plus : `gather_state.py` (lecture seule).
- Un rattachement durable de fichiers : une règle dans `data/loc_map.json`, avant la
  règle plus générale qui l'avalerait (la première qui correspond l'emporte). Un code
  partagé par plusieurs pays : un groupe dans `groups`, dont les pays se déduisent
  (`norm`, `feature` ou `imports`), jamais une liste de pays. Un nouveau groupe de
  normes : un nom court (`short`), l'étiquette de sa barre quand le code partagé est
  montré à part. Un nouveau dossier du socle : un nom dans ses `parts`, sans quoi le
  survol montre son chemin.
- Un changement de format des données : adapter `validate`, `canonical` et `compare`
  dans `build_widget.py`, et le rendu du gabarit.

Les drapeaux viennent de flag-icons 7.2.3, qui connaît aussi `xk` pour le Kosovo.
