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

Lancez les mêmes contrôles que la CI ([.github/workflows/ci.yml](.github/workflows/ci.yml)) :

```bash
gofmt -l $(git ls-files '*.go')
go vet ./...
go test -race ./...
golangci-lint run
```

`gofmt -l` ne doit rien afficher. La CI utilise golangci-lint v2.15.0, avec les réglages de [.golangci.yml](.golangci.yml).

Si la pull request touche au serveur (`server/`), lancez aussi ses tests, et le test de bout en bout de l'image Docker :

```bash
(cd server && go vet ./... && go test -race ./... && golangci-lint run --config=../.golangci.yml)
docker build -f server/Dockerfile -t zerechnung:test . && server/scripts/smoke.sh zerechnung:test
```

Si la pull request touche aux règles, aux schémas ou à la lecture et l'écriture du XML, lancez aussi les tests sur les artefacts officiels :

```bash
eval "$(conformance/fetch.sh)"
go test ./...
```

`fetch.sh` récupère les artefacts à des versions figées et définit les variables `FX_*` qui activent les tests optionnels (détail dans la section [Tests](README.md#tests) du README et dans [conformance](conformance/README.md)). Le workflow [Conformité](.github/workflows/conformance.yml) les exécute à chaque pull request et échoue si l'un d'eux est ignoré. Il compare aussi le package à KoSIT et à Mustang, facture par facture : tout nouvel écart fait échouer le job. Un écart justifié s'ajoute à [conformance/parity/known.txt](conformance/parity/known.txt), avec sa justification. `conformance/parity.sh` a besoin de Java ; il n'est pas nécessaire de le lancer en local, la CI s'en charge.

## Code et tests

- Les commentaires du code sont en anglais. Chaque symbole exporté a un commentaire de documentation qui commence par son nom et se termine par un point (règles `revive` et `godot` du linter).
- Une correction vient avec un test qui échouait avant elle.
- Une règle ajoutée ou modifiée vient avec un test qui la déclenche sur une facture modifiée pour l'enfreindre, et la facture valide qui ne la déclenche pas.
- Un fichier de test ajouté est fictif, ou provient d'un corpus public dont la licence permet la copie ; sa source et sa licence sont alors indiquées, comme dans [testdata/NOTICE](testdata/NOTICE).
- Quand le comportement change, le README suit : sections [Couverture](README.md#couverture) et [Limites](README.md#limites), nombres de règles, exemples.

## Commits et pull requests

- Les messages de commit sont en français. Le sujet dit ce que fait le commit, au présent : « Applique les règles néerlandaises… », « Corrige la lecture de BT-24… ». Le corps explique pourquoi, et ce qui reste hors du changement.
- Une pull request traite un seul sujet. Elle est fusionnée en un seul commit (squash) : son titre devient le sujet du commit.
- Le [modèle de pull request](.github/pull_request_template.md) rappelle les vérifications attendues.

## Licence

En contribuant, vous acceptez que votre contribution soit publiée sous la [licence MIT](LICENSE) du projet.
