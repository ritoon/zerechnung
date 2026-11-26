# ZeRechnung

@CONTRIBUTING.md

## Parcourir le code

- Cherchez avec `rg` (ripgrep), pas avec `grep -r` ni `find` : il suit les `.gitignore` et saute les dossiers cachés. Depuis le checkout principal, `grep -r` parcourt aussi `conformance/.artifacts/` (les artefacts officiels de `conformance/fetch.sh`, plus de 400 Mo) et les copies du dépôt sous `.claude/worktrees/` : une recherche y prend une demi-minute et renvoie surtout des doublons, là où `rg` répond en une fraction de seconde.
- `rg -t go` limite la recherche au Go ; `rg --files -g '<motif>'` remplace `find` pour trouver un fichier.
- Les `*_gen.go` (tables des schémas, catalogues de règles, jusqu'à 550 Ko par fichier) noient les résultats : `rg -g '!*_gen.go'` les écarte quand on cherche le code écrit à la main. Pour retrouver l'origine d'une règle, gardez-les et cherchez son identifiant tel quel : `rg -F 'BR-CO-15' internal/rules`.
- Les artefacts officiels se cherchent en nommant leur dossier, que `rg` parcourt alors malgré le `.gitignore` : `rg -F 'BR-CO-15' conformance/.artifacts`.

## CodeGraph : choisir l’outil selon la question

CodeGraph n’est pas un préalable aux recherches ou aux lectures. La présence
d’un index ne rend pas son appel obligatoire. Respecter les consignes plus
spécifiques du dépôt et choisir l’outil selon le besoin :

- **Fichier, symbole connu ou texte précis :** utiliser `rg` dans un périmètre
  ciblé, puis lire la plage utile. Lire directement cette plage si son emplacement
  est déjà connu. Utiliser le LSP/`gopls` pour une définition ou des références
  si disponible. Si `rg` est indisponible, utiliser `git grep` ou la recherche
  locale équivalente.
- **Appelants, dépendances et impact d’un changement :** privilégier une requête
  de graphe ciblée (`codegraph callers <symbole>`, `codegraph callees <symbole>`
  ou `codegraph impact <symbole>`), lorsque l’index est exploitable.
- **Flux complexe entre plusieurs fichiers :** utiliser `codegraph_explore`
  ou `codegraph explore --max-files 3 "<symboles du flux>"` si les relations
  sont nécessaires à la réponse. Côté MCP, fixer `maxFiles` à 3 au départ.
  Nommer les symboles pertinents et élargir seulement pour une dépendance
  manquante. `maxFiles` limite les fichiers de source, pas tout le texte renvoyé ;
  un dossier cité dans la requête n’est pas un filtre strict de périmètre.
- **Limiter le contexte :** réutiliser les extraits déjà obtenus et encore
  valides. Éviter de relire un fichier déjà fourni par le graphe ; vérifier
  toutefois le code courant si la branche ou le fichier a changé. Ne pas lancer
  une exploration générale pour localiser une seule fonction connue.
- **Index absent, inutilisable ou décalé :** poursuivre avec `rg`, le LSP et des
  lectures ciblées. Vérifier les résultats importants sur la branche de travail
  si l’index vient d’un autre checkout. L’indexation initiale (`codegraph init`)
  reste un choix explicite de l’utilisateur, pas un prérequis automatique.
- **Index et cache :** l’index local accélère la recherche des relations ; il ne
  supprime pas les tokens du texte retourné au modèle. Le cache du modèle est
  distinct et peut aussi bénéficier aux lectures classiques. Ne pas promettre
  une économie de tokens ou de coût sans mesure du parcours complet.

## Avant chaque commit

- Lancez `scripts/check.sh` et ne commitez que s'il finit par « All checks passed. ». Il reprend les contrôles des workflows CI et Conformité qui n'ont besoin ni de Java ni de Docker, dont tous les tests sur les artefacts officiels et la vérification des règles levées (`TestRuleCoverage`).
- `go test ./...` seul n'est pas une vérification : il ignore sans le dire les tests qui ont besoin des artefacts officiels (variables `FX_*`), c'est-à-dire la plupart des tests des règles, des schémas et du XML.
- `scripts/check.sh -quick` saute ces tests pendant qu'on itère ; le commit passe par le script complet.

## Branches et pull requests

- Jamais de push sur `main` : le ruleset du dépôt n'y accepte que des pull requests dont les checks requis passent. Travaillez sur une branche et ouvrez une pull request.
- Une pull request ne se fusionne que quand tous ses checks sont verts. Un check qui échoue se corrige ; il ne se contourne pas en modifiant le ruleset ou le workflow.

## Régressions

- Une correction vient avec un test qui échouait avant elle.
- Un échec du workflow Fuzzing joint l'entrée fautive (artefact `fuzz-failure-*`) : elle va dans `testdata/fuzz/<cible>/` du paquet, dans le commit de la correction.
