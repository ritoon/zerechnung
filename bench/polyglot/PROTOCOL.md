# Protocole commun : bancs TypeScript, Python, Rust, Java, C#, C, C++ et PHP

Ce dossier mesure les librairies Factur-X / ZUGFeRD / EN 16931 des écosystèmes TypeScript, Python, Rust, Java, C# (.NET), C, C++ et PHP avec **le même protocole** que le banc Go de `bench/ecosystem` (voir `bench/ecosystem/common/common.go` et `harness.go`), pour comparer leurs temps à ceux du package Go de ce dépôt.

Un sous-dossier par langage : `typescript/`, `python/`, `rust/`, `java/`, `csharp/`, `c/`, `cpp/`, `php/`. Chacun est autonome (ses dépendances, son harnais) et ne touche à rien d'autre dans le dépôt.

## Fichiers d'entrée

Chemins relatifs à `bench/ecosystem/` (le corpus est récupéré par `bench/ecosystem/run.sh setup`, commit ZUGFeRD/corpus `84f8d8f`). Utiliser ces octets-là, sans copie ni modification.

| Nom | Fichier |
|---|---|
| PDF Small (149 Ko, EN 16931) | `corpus/XML-Rechnung/FX/EN16931_Einfach.pdf` |
| PDF Large (1,1 Mo, EN 16931) | `corpus/XML-Rechnung/FX/EN16931_Elektron.pdf` |
| PDF Minimum (89 Ko, FNFE) | `corpus/ZUGFeRDv2/correct/FNFE-factur-x-examples/Facture_FR_MINIMUM.pdf` |
| PDF Extended (157 Ko, ZUGFeRD 2.0) | `corpus/ZUGFeRDv2/correct/intarsys/EXTENDED/zugferd_2p0_EXTENDED_Warenrechnung.pdf` |
| XML Small (13 Ko) | `corpus/XML-Rechnung/CII/EN16931_Einfach.cii.xml` |
| XML Lines (19 Ko, remises et charges) | `corpus/XML-Rechnung/CII/EN16931_Rabatte.cii.xml` |
| XML Large (1,3 Mo, pièce jointe base64) | `corpus/XML-Rechnung/CII/XRECHNUNG_Elektron.cii.xml` |
| Hôte à flux xref | `host.pdf` (143 Ko, sans pièce jointe) |
| Hôte à table xref classique | `host_classic.pdf` (147 Ko) |

Corpus des relevés d'exactitude :

- **26 XML CII valides** : `corpus/XML-Rechnung/CII/*.cii.xml`, sauf `not_validating_full_invoice_based_onTest_EeISI_300_CENfullmodel.cii.xml`.
- **104 PDF hybrides valides** : tous les `.pdf` sous `corpus/ZUGFeRDv2/correct/` (récursif) et `corpus/XML-Rechnung/FX/*.pdf`.

## Opérations mesurées

Mêmes noms que le banc Go :

| Benchmark | Entrée | Mesure |
|---|---|---|
| `Extract_Small`, `Extract_Large`, `Extract_Minimum`, `Extract_Extended` | PDF | PDF → octets du XML de la facture |
| `Parse_Small`, `Parse_Lines`, `Parse_Large` | XML | XML → modèle de la librairie |
| `ParseValidate_Small`, `ParseValidate_Lines`, `ParseValidate_Large` | XML | XML → validation complète, nombre d'erreurs |
| `Write_Small`, `Write_Lines` | XML | modèle → XML CII ; le modèle est construit **une fois hors mesure** (par le parseur de la librairie ; à défaut, à la main à partir du contenu de `EN16931_Einfach`, avec `note: "modèle construit à la main"`) |
| `Embed_Small` | `host.pdf` + XML Small | PDF hôte + XML → PDF Factur-X |
| `EmbedClassic_Small` | `host_classic.pdf` + XML Small | idem, hôte à xref classique |
| `CheckPDF_Small`, `CheckPDF_Large` | PDF | contrôle complet d'un PDF hybride par une fonction **native** de la librairie (conteneur + XML) ; pas de composition extraction + validation |

Règles, identiques au banc Go :

1. **Vérifier avant de mesurer.** Un premier appel hors mesure contrôle le résultat : le XML extrait contient `CrossIndustryInvoice` ou `CrossIndustryDocument` ; le PDF produit par `Embed` se relit et la facture s'en extrait (avec la librairie elle-même, ou à défaut une autre librairie du même langage). Si la librairie ne sait pas faire l'opération : `status: "unsupported"`. Si elle renvoie un résultat faux ou lève une exception : `status: "fail"` avec le message.
2. **Validation** : le jeu de règles le plus complet que la librairie applique hors ligne à ces factures allemandes, **sans** règles nationales françaises (le corpus est allemand). Noter lequel dans `note`.
3. **Injection** : options par défaut, date de modification fixe si la librairie la demande. Si la librairie convertit aussi en PDF/A ou refuse un hôte non PDF/A, le noter.
4. Mesure **en processus**, sur les octets déjà en mémoire (lecture des fichiers hors mesure).

## Méthode de chronométrage

Calquée sur `go test -bench . -count 3 -benchtime 1s` :

- Pour chaque benchmark : trouver N par paliers croissants (1, 2, 5, 10, 20, 50, 100…) jusqu'à ce qu'une série de N appels dure au moins 1 s (`BENCHTIME`, en secondes). Cette montée en charge sert d'échauffement (JIT de V8, caches).
- Faire ensuite `COUNT` séries (3 par défaut) de N appels, chronométrées avec l'horloge monotone (`process.hrtime.bigint()`, `time.perf_counter_ns()`, `std::time::Instant`, `System.nanoTime()`, `Stopwatch.GetTimestamp()`, `clock_gettime(CLOCK_MONOTONIC)`, `std::chrono::steady_clock`, `hrtime(true)`). `ns_per_op` = durée de la série / N.
- Enregistrer aussi `first_call_ns` : la durée du tout premier appel de l'opération dans le processus (compilation de règles, chargement paresseux…).
- Pas de GC forcé, pas de parallélisme : un seul fil.
- Rust : profil `release` par défaut de Cargo, sans option de CPU spécifique. Node : la version installée (`node --version`), sans option. Python : CPython, version notée.
- Java : un processus JVM par librairie, options par défaut de la JVM (une option de pile ou de tas n'est ajoutée que si un validateur l'exige, et elle est notée). C# : build `Release`, réglages par défaut du runtime .NET (compilation par paliers). C et C++ : build CMake `Release` (`-O3 -DNDEBUG`), sans option de CPU spécifique. PHP : le PHP CLI installé, réglages notés dans `php/COVERAGE.md`.

## Temps CPU et mémoire

Deux mesures complètent le temps réel, dans tous les langages :

- **Temps CPU par opération** (`cpu_ns_per_op`, en regard de `ns_per_op`) : pour chaque série chronométrée, le temps CPU du processus (utilisateur + système, **tous les fils** : ramasse-miettes, JIT, fils du runtime) consommé pendant la même fenêtre que l'horloge monotone, divisé par N. Horloge lue une fois avant et une fois après la série : `process.cpuUsage()`, `time.process_time_ns()`, `getrusage(RUSAGE_SELF)` (Rust, C, C++, PHP, Go), `OperatingSystemMXBean.getProcessCpuTime()`, `Environment.CpuUsage`. Le rapport CPU / temps réel vaut 100 % pour un cœur occupé ; au-delà, du travail tourne en parallèle du fil mesuré.
- **Pic de mémoire** (`max_rss_bytes`) : la mémoire résidente maximale (max RSS) d'un processus qui charge la librairie et n'exécute **qu'un** benchmark (vérification, premier appel, calibrage, séries), runtime compris. Lue en fin de processus par `getrusage(RUSAGE_SELF).ru_maxrss`, ou mesurée de l'extérieur par `/usr/bin/time -l` sur le processus lui-même (jamais sur un lanceur comme npm, mvn, `dotnet run` ou `cargo run`). Pour Go, chaque benchmark tourne dans son propre binaire de test (`go test -c`), `FX_CPU=1` fait rapporter `cpu-ns/op` par les aides de `bench/ecosystem/common`. Une seconde série Go tourne avec `GOMAXPROCS=1` (ligne « ZeRechnung, GOMAXPROCS=1 ») : le ramasse-miettes concurrent de Go occupe sinon d'autres cœurs, et cette série montre le package limité à un cœur.

Ces deux mesures viennent d'une passe à part, `resources` : un processus par librairie et par benchmark, `COUNT=1` et `BENCHTIME=1` par défaut (`RES_COUNT`, `RES_BENCHTIME` dans `run.sh` à la racine). Les temps des tableaux principaux restent ceux de la passe `bench`. Le travail d'un processus enfant lancé par une librairie (une JVM, un outil en ligne de commande) n'est pas compté ; le harnais le signale dans `note`.

## Allocations

`run.sh alloc` mesure ce qu'alloue **un appel** de chaque opération : les octets demandés à l'allocateur et, quand le langage les compte, le nombre d'allocations. C'est toute la mémoire que l'appel demande, y compris ce qu'il libère avant de rendre la main ; pas la mémoire qu'occupe le processus.

- Même contrôle du résultat que pour les temps (premier appel, `first_call_ns`), puis un échauffement : au moins 3 appels, jusqu'à 100 appels ou 0,2 s.
- Puis une série comptée d'environ 0,2 s, de 1 à 1 000 appels. Valeur retenue : la moyenne par appel.
- **Go** : `go test -benchmem` (B/op, allocs/op) donne les chiffres avec les benchmarks ; `run.sh alloc go` ne fait rien.
- **Rust** : le harnais installe un `#[global_allocator]` qui enveloppe `System`. Pendant la série, chaque `alloc`, `alloc_zeroed` et `realloc` compte pour une allocation de la taille demandée ; hors série, l'allocateur ne lit qu'un indicateur atomique.
- **Python** : le processus de mesure tourne avec `PYTHONMALLOC=malloc` (chaque objet passe par `malloc`) et charge [`python/alloccount.c`](python/alloccount.c) par `DYLD_INSERT_LIBRARIES` (macOS) ou `LD_PRELOAD` (Linux). Ce compteur intercepte `malloc`, `calloc`, `realloc`, `posix_memalign` et `aligned_alloc` de tout le processus, extensions C comprises (libxml2, qpdf, pydantic-core). La mémoire gérée par `mmap` directement (tas GraalVM de saxonche) n'est pas vue. Le processus est lancé avec `-s -P` et sans les autres variables `PYTHON*` (`-I` ignorerait `PYTHONMALLOC`) ; un contrôle préalable vérifie que le compteur voit bien une allocation de 4 Ko.
- **TypeScript** : V8 ne compte pas ses allocations. Le processus tourne avec `--expose-gc` et une jeune génération de 64 Mo (`--min-semi-space-size=64 --max-semi-space-size=64`). Octets = croissance de `heapUsed` et de `arrayBuffers` pendant la série + ce que chaque passage du ramasse-miettes a libéré (`v8.GCProfiler`). La série est dimensionnée pour allouer au plus 32 Mo ; si un ramasse-miettes passe quand même, elle est réduite (jusqu'à 1 appel), puis la valeur est notée comme un minimum (`gcs` > 0, « ≥ » dans les tableaux). Pas de nombre d'allocations (`allocs: null`). Un processus par benchmark.
- **Java, C#, C, C++, PHP** : pas encore de compteur ; `run.sh alloc` à la racine les saute.

## Sorties

`run.sh` dans chaque dossier, avec les mêmes sous-commandes que `bench/ecosystem/run.sh` :

```bash
./run.sh setup              # dépendances (npm, venv, cargo, Maven, dotnet, CMake, Composer)
./run.sh survey    [lib…]   # relevés d'exactitude
./run.sh bench     [lib…]   # benchmarks
./run.sh alloc     [lib…]   # allocations par appel (TypeScript, Python, Rust)
./run.sh resources [lib…]   # temps CPU et pic mémoire, un processus par benchmark
```

Variables : `OUT` (défaut `../results/$(date +%F)`), `COUNT` (3), `BENCHTIME` (1). Fichiers produits dans `$OUT` :

`bench_<lang>.jsonl`, une ligne par librairie et benchmark :

```json
{"lang":"python","lib":"factur-x","pkg":"factur-x","version":"7.9","bench":"Extract_Small","status":"ok","ns_per_op":[51234.1,50988.0,51100.7],"cpu_ns_per_op":[51190.4,50950.2,51080.9],"iters":20000,"first_call_ns":812000,"note":""}
```

`resources_<lang>.jsonl` : les mêmes champs, une série par défaut, plus `max_rss_bytes` (octets).

`alloc_<lang>.jsonl`, une ligne par librairie et benchmark (`gcs` en TypeScript seulement) :

```json
{"lang":"rust","lib":"en16931","pkg":"en16931-formats","version":"0.10.1","bench":"Extract_Small","status":"ok","allocs":2199.0,"bytes":1390014.0,"calls":370,"first_call_ns":2006333,"note":""}
```

`survey_<lang>.jsonl`, une ligne par librairie et opération (`validate` : XML valides signalés sur 26 ; `parse` : XML lus sur 26 ; `extract` : PDF extraits sur 104) :

```json
{"lang":"python","lib":"factur-x","op":"extract","ok":104,"total":104,"failures":[{"file":"x.pdf","error":"..."}],"note":""}
```

Pour `validate`, `ok` = nombre de fichiers **signalés** (au moins une erreur, ou exception), `failures` = la liste de ces fichiers avec le nombre d'erreurs et les identifiants de règles. Référence : les schematrons officiels (CEN EN 16931 CII + KoSIT pour les XRECHNUNG) signalent 6 fichiers sur 26 (BR-CL-25 deux fois, CII-SR-465 quatre fois sur les exemples XRECHNUNG).

## Couverture de la norme

`<lang>/COVERAGE.md` : pour chaque librairie retenue ou écartée, une fiche, puis un tableau avec ces colonnes :

| Librairie | Version | Licence | Dernière version | Téléchargements | Lecture CII | Écriture CII | UBL | Extraction PDF | Injection PDF | XSD | EN 16931 (CII) | Profils Factur-X (MINIMUM, BASIC WL, EXTENDED) | BR-FR | BR-DE (XRechnung) | Peppol BIS 3 | Contrôle du conteneur PDF/A-3 | Dépendances externes |

Valeurs attendues : `oui`, `non`, `partiel (n règles)`, `XSLT officiel via Saxon`, etc. **Vérifier dans le code** (fichier et ligne, ou version des artefacts embarqués), pas seulement dans le README : nombre de règles réellement implémentées, version des artefacts officiels (CEN 1.3.x, Factur-X 1.0x, XRechnung 3.0.x, FNFE-MPE 1.x), appels réseau, besoin de Java.

Écarter (avec la raison dans `COVERAGE.md`) : les clients d'API distantes, les librairies qui exigent une JVM hors du banc Java (le banc Java mesure ces librairies-là directement), les paquets vides ou de réservation de nom.
