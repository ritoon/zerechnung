## Ce que change cette pull request

<!-- Ce qui change, et pourquoi. Lien vers l'issue s'il y en a une : « Closes #… ». -->

## Références

<!--
Pour une règle ajoutée ou modifiée : artefact officiel, version, identifiant.
Pour un écart de validation : validateur de référence, version et sortie.
Sinon, supprimez cette section.
-->

## Vérifications

- [ ] `scripts/check.sh` finit par « All checks passed. » (gofmt, `go vet`, golangci-lint, `go test -race`, tests sur les artefacts officiels et règles levées).
- [ ] Un test couvre le changement.
- [ ] Le README suit le changement (couverture, limites, nombres de règles), s'il y a lieu.
- [ ] Aucune dépendance n'est ajoutée à `go.mod`.
- [ ] Les fichiers de test sont fictifs ou issus d'un corpus public, source et licence indiquées (voir `testdata/NOTICE`).
