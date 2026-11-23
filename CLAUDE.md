# ZeRechnung

@CONTRIBUTING.md

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
