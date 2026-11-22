# Benchmarks de l'écosystème Go Factur-X

Ce dossier mesure les librairies Go existantes sur le corpus officiel ZUGFeRD : temps de réponse, mémoire, et exactitude (extraction, parsing, validation, injection). Les résultats et leur analyse sont publiés sur le wiki :

- [Benchmarks](https://github.com/ritoon/zerechnung/wiki/Benchmarks)
- [Grille de la norme](https://github.com/ritoon/zerechnung/wiki/Grille-de-la-norme)

Le code de ce dossier ne fait pas partie du package `zerechnung` : chaque sous-dossier est un module Go indépendant.

## Prérequis

- Go 1.26 ou plus récent. Avec `GOTOOLCHAIN=auto` (positionné par le script), Go télécharge 1.27.0 pour DCNWS et pdf0, et 1.27.3 pour litho.
- git et un accès réseau, pour récupérer le corpus, les dépendances et le clone de solarpush/fX.

## Utilisation

```bash
./run.sh setup     # corpus ZUGFeRD (sparse), clone de solarpush/fX, PDF hôtes
./run.sh survey    # exactitude sur tout le corpus
./run.sh bench     # benchmarks (COUNT=3, BENCHTIME=1s par défaut)
./run.sh report    # tableaux Markdown dans results/<date>/tables.md
./run.sh all       # les quatre à la suite
```

Chaque commande accepte une liste de modules (`./run.sh bench pdf0 litho`). Les variables `OUT`, `COUNT` et `BENCHTIME` changent le dossier de sortie, le nombre d'exécutions et la durée de chaque benchmark. Une campagne complète prend une dizaine de minutes sur un Apple M4 Pro.

## Organisation

| Dossier | Contenu |
|---|---|
| `common/` | Fichiers d'entrée partagés et fonctions de mesure (`Extract`, `Bytes`, `Write`, `Embed`, `Survey*`) |
| `floor/` | Planchers : découpage en jetons par `encoding/xml`, copie mémoire |
| `<librairie>/` | Un module par librairie : benchmarks, `TestSurvey`, parfois `TestDetail` |
| `report/` | Outil qui produit les tableaux du wiki à partir des sorties brutes |
| `results/<date>/` | Sorties brutes (`bench_*.txt`, `survey_*.txt`) et tableaux (`tables.md`) |
| `corpus/`, `.deps/`, `host*.pdf` | Récupérés ou générés par `./run.sh setup`, non versionnés |

Un seul module pour toutes les librairies est impossible : pdfcpu a cassé son API entre les versions 0.11 et 0.16, et ces librairies en utilisent des versions différentes.

## Opérations mesurées

| Benchmark | Mesure |
|---|---|
| `Extract_*` | PDF → XML de la facture |
| `Parse_*` | XML → modèle Go |
| `ParseValidate_*` | XML → parsing et validation complète |
| `Write_*` | modèle Go → XML CII (parsing fait une fois, hors mesure) |
| `Embed_Small` | PDF hôte à flux xref + XML → PDF Factur-X, puis relecture du résultat |
| `EmbedClassic_Small` | idem avec l'hôte à table xref classique |
| `CheckPDF_*` | contrôle complet d'un PDF Factur-X |

Chaque mesure vérifie d'abord le résultat (le XML extrait est bien un CII, le PDF produit se relit). Si la librairie ne sait pas faire l'opération, le benchmark est ignoré ; si elle renvoie un résultat faux, il échoue.

Les relevés d'exactitude (`TestSurvey`) portent sur 26 XML CII valides (`XML-Rechnung/CII`) et 104 PDF hybrides valides (`ZUGFeRDv2/correct` et `XML-Rechnung/FX`).

## Appel de chaque librairie

| Module | Librairie | Appels |
|---|---|---|
| `speedata` | speedata/einvoice v0.0.23 + pdfdisassembler v0.0.9 | `einvoice.ParseReader`, `inv.Validate`, `inv.Write` ; extraction via `pdf.Open` → `EmbeddedFiles` |
| `andee` | andeedotnet/go-xinvoice + go-xinvoice-pdf v0.1.4 | `xinvoicepdf.ExtractXML`, `xinvoice.ParseXML`, `xinvoice.Validate`, `inv.ToXML`, `xinvoicepdf.Embed` |
| `dotwave` | dotwavehq/go-einvoice v0.7.1 | `zugferd.ExtractXML`, `einvoice.Parse`, `cii.NewSerializer().Serialize`, `zugferd.EmbedXML` (via fichiers temporaires) |
| `goblde` | invopop/gobl.de.xinvoice v0.0.10 | `xinvoice.Parse`, `Envelope.Validate`, `xinvoice.Convert` |
| `angelo` | angelodlfrtr/go-invoice-generator v0.8.0 | `facturx.Extract`, `facturx.Parse`, `Parsed.Verify` |
| `papierleicht` | papierleicht/go-e-rechnung-validator v0.1.0 | `validator.ExtractXML`, `Validator.Validate`, `Validator.ValidatePDF` |
| `dcnws` | DCNWS/invoicekit.go v0.2.13 | `facturx.Extract`, `einvoice.Parse`, `einvoice.Validate`, `einvoice.GenerateCII`, `facturx.Embed`, `hybrid.Check` |
| `pdf0` | mgilbir/pdf0 v0.9.1 + formalis v0.3.1 | `pdf0.Read` + `pdf0.ValidateFacturX`, `formalis.Validate`, `pdf0.EmbedFacturX` + `Document.Write` |
| `litho` | gotenberg/litho v0.0.18 | `pdf.Open` + `EmbeddedFiles`, `ValidateFacturXReport`, `Writer.RemediateFacturX` |
| `kanjo` | cyprienbrisset/kanjo v0.3.2 | `pdfa.ExtractInvoiceXML`, `read.ReadBytes`, `rules.Validate(doc, "en16931")`, `write.WriteBytes`, `pdfa.EmbedXML` |
| `taha` | tahayassine/facturx-go v0.1.0 | `facturx.ExtractXML`, `cii.Decode`, `en16931.Validate`, `cii.Encode` |
| `fx` | solarpush/fX v1.4.0 + 6 commits (`1314f3d`) | `pdf.ExtractXML`, `cii.Parse`, `invoice.ValidateReport`, `cii.Generate` |
| `gopdfattach` | MarlinKuhn/gopdfattach v0.0.3 | `gopdfattach.Extract`, `gopdfattach.AttachFacturX` |
| `pdfcpu` | pdfcpu v0.16.4 | `api.ExtractAttachmentsRaw` ; injection par une recette bas niveau écrite pour ce banc (FileSpec, `/AF`, XMP) |
| `facturx` | ce dépôt (par `replace` vers la racine) | `facturx.Extract`, `cii.Parse`, `validate.XML`, `cii.Marshal`, `facturx.Embed`, `facturx.Check` |

## Particularités connues

- **`Embed_Small` échoue pour andee et kanjo, et c'est voulu.** Sur l'hôte à flux d'objets, ces librairies renvoient un PDF de 25 Ko qui ne se relit pas (l'hôte en fait 144). `TestEmbedOutputSize` affiche les tailles d'entrée et de sortie. Le script continue et enregistre le reste des mesures.
- **fX se compile depuis un clone local.** L'archive de module de solarpush/fX est refusée par `go get` (un nom de fichier contient un retour chariot) ; `run.sh setup` clone le dépôt dans `.deps/` et `fx/go.mod` le référence par `replace`.
- **pdf0 n'expose pas d'extraction seule.** Le XML n'est accessible que par `ValidateFacturX`, qui contrôle tout le PDF : pdf0 apparaît donc dans `CheckPDF`, pas dans `Extract`.
- **kanjo est restreint au jeu de règles `en16931`.** Le jeu par défaut inclut les règles françaises et rejette les 26 exemples allemands.
- **pdfcpu écrit un `config.yml`** et des certificats dans le répertoire de configuration de l'utilisateur (`~/Library/Application Support/pdfcpu` sur macOS), sauf si `model.ConfigPath = "disable"`. Chaque module qui dépend de pdfcpu le positionne dans un `init()` (`noconfig_test.go`). gopdfattach tente de le faire lui-même, mais avec la valeur `"disabled"`, que pdfcpu ne reconnaît pas.
- **papierleicht compile ses règles au premier appel** (environ une seconde) dans un dossier de cache temporaire ; le benchmark crée le validateur une seule fois.
- `TestDetail` (dans plusieurs modules) liste les règles qui rejettent des XML valides.

## Résultats du 22/11/2026

Apple M4 Pro sur batterie, mode hautes performances (`powermode 2`), Go 1.27.4 pour tous les modules. Le package de ce dépôt est mesuré au commit `3a3b11a`, en 6 exécutions ; les autres librairies en 3. litho passe de v0.0.13 à v0.0.18. Les tableaux sont dans [`results/2026-11-22/tables.md`](results/2026-11-22/tables.md), avec, pour chaque opération, les octets et le nombre d'allocations d'un appel (`go test -benchmem`) ; l'analyse est sur la page [Benchmarks](https://github.com/ritoon/zerechnung/wiki/Benchmarks) du wiki.

- Le package est le plus rapide sur chaque opération, et c'est lui qui alloue le moins sur chacune : 28 Ko en 104 allocations pour lire une facture de 13 Ko, quand le seul découpage en jetons d'`encoding/xml` en demande 31 Ko et 711 ; une seule allocation de 8 Ko pour l'écrire ; 245 Ko pour l'injecter dans un PDF, contre 1,3 Mo pour pdf0.
- Le contrôle complet d'un PDF prend 832 µs, contre 54,8 µs le 16/11 : depuis la PR #73, il vérifie les exigences PDF/A sur tout le fichier (chaque objet, contenu des pages, polices, images JPEG 2000). DCNWS, le suivant, met 1,11 ms.

## Résultats du 16/11/2026

Les fichiers `bench_*.txt` de `results/2026-11-16/` ont été produits avant le déplacement du harnais dans ce dépôt : le chemin de module qu'ils affichent est `example.local/fxbench/…`, le code mesuré est le même. Les fichiers `survey_*.txt` ont été régénérés depuis ce dossier et donnent les mêmes chiffres. Pour angelo, kanjo, fX et gopdfattach, les mesures datent d'avant la désactivation du `config.yml` de pdfcpu ; une nouvelle mesure de `Extract_Small` après correction diffère de moins de 7 %, sans changer le classement.

Les fichiers `bench_facturx.txt` et `survey_facturx.txt` mesurent la première version du package de ce dépôt, sur la même machine (Apple M4 Pro) et le même jour. Son validateur applique par défaut le schéma, les règles EN 16931 et les règles de profil Factur-X, sans les règles françaises ; les 6 XML qu'il rejette sont ceux que rejette aussi papierleicht, qui exécute les schematrons officiels (CEN et KoSIT) : BR-CL-25 deux fois, CII-SR-465 quatre fois (sur les exemples XRECHNUNG).

## Ajouter une librairie

1. Créer un dossier avec un `go.mod` qui dépend de `github.com/ritoon/zerechnung/bench/ecosystem/common` par `replace … => ../common`.
2. Écrire des fonctions `extract`, `parse`, `validate`, `write` ou `embed` selon ce que la librairie sait faire, et les passer aux fonctions de `common`.
3. Ajouter un `TestSurvey`, puis le nom du dossier dans `MODULES` de `run.sh`.

## Licences

Le code de ce dossier est sous licence MIT, comme le dépôt. Les modules téléchargent des librairies tierces sous leur propre licence, dont AGPL-3.0 (dotwave, litho, fX), BSL 1.1 (kanjo) et MPL-2.0 (papierleicht) ; elles ne sont utilisées que par ces benchmarks et ne sont pas importées par le package `zerechnung`. Le corpus ZUGFeRD (Apache-2.0) est téléchargé, pas versionné ici.
