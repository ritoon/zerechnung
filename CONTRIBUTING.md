# Contribuer à ZeRechnung

Les contributions sont bienvenues : signalement d'un bogue, écart avec un validateur de référence, règle manquante, nouvelle version d'un artefact officiel, correction de la documentation. Ce guide dit comment les proposer pour qu'elles soient intégrées sans aller-retour inutile.

En participant au projet, vous acceptez de respecter son [code de conduite](CODE_OF_CONDUCT.md).

## Avant de commencer

- **Bogue ou écart de validation** : ouvrez une [issue](https://github.com/ritoon/zerechnung/issues/new/choose) avec le modèle qui correspond. Une règle levée à tort ou manquée se démontre au mieux avec la sortie d'un validateur de référence (KoSIT, Mustang, schematron officiel exécuté par Saxon…).
- **Changement important** (nouveau profil, nouveau jeu de règles, changement d'API) : ouvrez d'abord une issue pour en discuter. L'API est encore en v0 et peut changer, mais chaque changement se décide avant d'être écrit.
- **Faille de sécurité** : pas d'issue publique, voir [SECURITY.md](SECURITY.md).
- **Traduction du site** : le site de présentation est généré depuis [site/](site/), un fichier JSON par langue. Une langue s'ajoute ou se corrige sans toucher au package ; [site/README.md](site/README.md) explique comment.

Les factures contiennent des données personnelles et commerciales. Dans une issue comme dans une pull request, joignez une facture fictive ou anonymisée, jamais une facture réelle.

## Principes du package

Une contribution qui s'en écarte ne sera pas acceptée en l'état :

- **Pur Go, bibliothèque standard uniquement.** `go.mod` n'a aucune dépendance et doit le rester : pas de CGO, pas de binaire externe, pas d'appel réseau à l'exécution. Les outils qui ont besoin d'autre chose vivent à part : la comparaison avec KoSIT et Mustang dans un module séparé, sous `conformance/parity/`, et les benchmarks face aux autres librairies dans leur propre dépôt, [ritoon/invoice-bench](https://github.com/ritoon/invoice-bench). Le serveur HTTP, qui appelle le réseau (JWKS, collecteurs de logs), est lui aussi un module à part, sous `server/`, sans dépendance hors de la bibliothèque standard.
- **Règles réécrites, pas interprétées.** Les règles des schematrons sont réécrites en Go à partir des artefacts officiels, en reprenant leurs identifiants, leurs niveaux et leurs messages, pour que les rapports correspondent aux validateurs de référence. Toute règle ajoutée ou modifiée cite sa source : artefact, version, identifiant.
- **Artefacts officiels hors du dépôt.** Les schémas sont compilés en tables Go et les catalogues de règles régénérés par `go generate`, qui télécharge les artefacts et vérifie leur SHA-256. Les fichiers `*_gen.go` ne se modifient pas à la main : on modifie le générateur, puis on relance `go generate` dans le paquet concerné.
- **Rien n'est perdu en silence.** Une valeur illisible est signalée sans arrêter la lecture ; une donnée que le profil visé n'admet pas est signalée par l'option `Strict` de `Marshal`.

## Environnement

Go 1.24 ou plus récent, la version minimale de `go.mod` ; la CI teste aussi la dernière version publiée.

```bash
git clone https://github.com/ritoon/zerechnung.git
cd zerechnung
go test ./...
```

Les tests de base n'ont besoin que du dépôt.

## Avant d'ouvrir une pull request

`main` n'accepte que des pull requests, et seulement quand les checks requis des workflows [CI](.github/workflows/ci.yml), [Conformité](.github/workflows/conformance.yml) et Docker (image Linux) passent : aucun push direct, même pour les mainteneurs.

Lancez les contrôles de ces workflows qui n'ont besoin ni de Java ni de Docker :

```bash
scripts/check.sh
```

Le script s'arrête au premier échec : gofmt, `go vet` et golangci-lint (à la version de la CI, avec les réglages de [.golangci.yml](.golangci.yml)) sur le package et le serveur, `go test -race`, budgets d'allocations, tests du site et des outils de `conformance/`, puis tous les tests sur les artefacts officiels, qui échouent si l'un d'eux est ignoré, et la vérification des règles levées. `scripts/check.sh -quick` saute les tests sur artefacts, pendant qu'on itère. `go test ./...` seul ne suffit pas : il ignore sans le dire les tests qui ont besoin des artefacts.

Si la pull request touche au serveur (`server/`), lancez aussi le test de bout en bout de l'image Docker :

```bash
docker build -f server/Dockerfile -t zerechnung:test . && server/scripts/smoke.sh zerechnung:test
```

[`conformance/fetch.sh`](conformance/fetch.sh), qu'appelle le script, récupère les artefacts à des versions figées et définit les variables `FX_*` qui activent les tests optionnels (détail dans la page [Testing](https://github.com/ritoon/zerechnung/wiki/Testing) du wiki et dans [conformance](conformance/README.md)). Le workflow [Conformité](.github/workflows/conformance.yml) les exécute à chaque pull request et échoue si l'un d'eux est ignoré. Il compare aussi le package à KoSIT et à Mustang, facture par facture : tout nouvel écart fait échouer le job. Un écart justifié s'ajoute à [conformance/parity/known.txt](conformance/parity/known.txt), avec sa justification. `conformance/parity.sh` a besoin de Java ; il n'est pas nécessaire de le lancer en local, la CI s'en charge.

## Code et tests

- Les commentaires du code sont en anglais. Chaque symbole exporté a un commentaire de documentation qui commence par son nom et se termine par un point (règles `revive` et `godot` du linter).
- Une correction vient avec un test qui échouait avant elle.
- Le workflow [Fuzzing](.github/workflows/fuzz.yml) fuzze chaque nuit toutes les cibles `Fuzz*` du dépôt, que [scripts/fuzz-targets.sh](scripts/fuzz-targets.sh) trouve seul. L'entrée qui fait échouer une cible est jointe au run (artefact `fuzz-failure-*`) : elle s'ajoute dans `testdata/fuzz/<cible>/` du paquet avec la correction, et `go test` la rejoue ensuite à chaque run.
- Une règle ajoutée ou modifiée vient avec un test qui la déclenche sur une facture modifiée pour l'enfreindre, et la facture valide qui ne la déclenche pas. Le job *Tests sur les artefacts officiels* le vérifie : une règle des catalogues qu'aucun test ne lève le fait échouer, sauf si elle est déjà dans [testdata/rule-coverage.txt](testdata/rule-coverage.txt), la liste des règles qu'aucun test ne lève encore. Un test qui lève une règle de cette liste fait échouer le job tant que la ligne n'est pas retirée (voir [conformance](conformance/README.md#règles-levées-par-la-suite)).
- Un fichier de test ajouté est fictif, ou provient d'un corpus public dont la licence permet la copie ; sa source et sa licence sont alors indiquées, comme dans [testdata/NOTICE](testdata/NOTICE).
- Quand le comportement change, le README suit : sections [Couverture](README.md#couverture) et [Limites](README.md#limites), nombres de règles, exemples.

## Profil PGO de la commande

[cmd/zerechnung/default.pgo](cmd/zerechnung/default.pgo) est un profil CPU des commandes `validate`, `embed` et `json` : `go build` et `go install` l'utilisent d'eux-mêmes pour la commande `zerechnung` (optimisation guidée par profil). Il ne change rien pour les utilisateurs de la bibliothèque. Mesuré sur un Apple M4 Pro, il rend la commande 2,9 % plus rapide sur les fichiers de `testdata` et 1,4 % sur le corpus, où la décompression des flux domine.

Régénérez-le quand le code des commandes ou des contrôles change beaucoup. Le banc `BenchmarkCommands` ([cmd/zerechnung/pgo_test.go](cmd/zerechnung/pgo_test.go)) fait en mémoire le travail des commandes sur les fichiers de `testdata`, et sur ceux du corpus quand `FX_CORPUS` est définie (`eval "$(conformance/fetch.sh)"`) :

```bash
go test ./cmd/zerechnung -run '^$' -bench Commands -benchtime 40s -cpu 1 -pgo=off -cpuprofile cmd/zerechnung/default.pgo
```

`-pgo=off` évite de profiler un binaire déjà optimisé par l'ancien profil. Comparez ensuite avec `benchstat` un binaire de test compilé sans profil et un autre avec le nouveau :

```bash
go test -c -o off.test -pgo=off ./cmd/zerechnung
go test -c -o on.test -pgo=cmd/zerechnung/default.pgo ./cmd/zerechnung
(cd cmd/zerechnung && for i in $(seq 10); do ../../off.test -test.run '^$' -test.bench Commands -test.cpu 1 >> ../../off.txt; ../../on.test -test.run '^$' -test.bench Commands -test.cpu 1 >> ../../on.txt; done)
benchstat off.txt on.txt
```

## Commits et pull requests

- Les messages de commit sont en français. Le sujet dit ce que fait le commit, au présent : « Applique les règles néerlandaises… », « Corrige la lecture de BT-24… ». Le corps explique pourquoi, et ce qui reste hors du changement.
- Une pull request traite un seul sujet. Elle est fusionnée en un seul commit (squash) : son titre devient le sujet du commit.
- Le [modèle de pull request](.github/pull_request_template.md) rappelle les vérifications attendues.

## Licence

En contribuant, vous acceptez que votre contribution soit publiée sous la [licence MIT](LICENSE) du projet.
