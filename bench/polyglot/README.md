# Benchmarks TypeScript, Python, Rust, Java, C#, C, C++ et PHP

Ce dossier compare le package Go de ce dépôt aux librairies Factur-X / ZUGFeRD / EN 16931 des écosystèmes TypeScript, Python, Rust, Java, C# (.NET), C, C++ et PHP : temps de réponse, exactitude sur le corpus officiel ZUGFeRD, et normes réellement appliquées. Il complète [`bench/ecosystem`](../ecosystem), qui fait la même chose pour les librairies Go.

Le code de ce dossier ne fait pas partie du package `zerechnung`. Chaque langage a son propre dossier, autonome :

| Dossier | Contenu |
|---|---|
| [`PROTOCOL.md`](PROTOCOL.md) | Protocole commun : fichiers d'entrée, opérations, chronométrage, format des sorties |
| [`typescript/`](typescript) | 13 librairies npm, Node 22, harnais `.mjs` ; fiches dans [`typescript/COVERAGE.md`](typescript/COVERAGE.md) |
| [`python/`](python) | 8 librairies PyPI, CPython 3.13 ; fiches dans [`python/COVERAGE.md`](python/COVERAGE.md) |
| [`rust/`](rust) | 11 crates, rustc 1.93, un binaire par crate ; fiches dans [`rust/COVERAGE.md`](rust/COVERAGE.md) |
| [`java/`](java) | 10 librairies Maven Central (dont Saxon-HE avec les XSLT officiels et le plancher PDFBox), OpenJDK 27, un module Maven et une JVM par librairie ; fiches dans [`java/COVERAGE.md`](java/COVERAGE.md) |
| [`csharp/`](csharp) | 13 paquets NuGet (dont SaxonCS-HE avec les XSLT officiels et trois planchers), .NET 11, un projet par librairie ; fiches dans [`csharp/COVERAGE.md`](csharp/COVERAGE.md) |
| [`c/`](c) | aucune librairie C dédiée : 4 planchers (MuPDF, qpdf, libxml2, libxslt), CMake, Apple clang 21 ; fiches dans [`c/COVERAGE.md`](c/COVERAGE.md) |
| [`cpp/`](cpp) | Kura (conteneur PDF/A seulement), SaxonC-HE avec les XSLT officiels, 5 planchers (qpdf, PoDoFo, poppler-cpp, Xerces-C, pugixml) ; fiches dans [`cpp/COVERAGE.md`](cpp/COVERAGE.md) |
| [`php/`](php) | 19 paquets Packagist (dont les planchers smalot/pdfparser et ext-dom), PHP 8.6 CLI, un processus par benchmark ; fiches dans [`php/COVERAGE.md`](php/COVERAGE.md) |
| `report/` | Outil qui fusionne les sorties en tableaux Markdown |
| `results/<date>/` | Sorties brutes (`bench_*`, `survey_*`), environnement (`env.txt`) et tableaux (`tables.md`) |

## Méthode

Les opérations, les fichiers d'entrée et les règles de mesure sont ceux du banc Go : mêmes octets du corpus ZUGFeRD (commit `84f8d8f`), mêmes noms de benchmarks (`Extract_*`, `Parse_*`, `ParseValidate_*`, `Write_*`, `Embed_*`, `CheckPDF_*`), résultat vérifié avant toute mesure.

Le chronométrage reproduit `go test -bench . -count 3 -benchtime 1s` : N croît par paliers 1, 2, 5, 10… jusqu'à ce qu'une série dure 1 s, puis 3 séries de N appels ; la valeur retenue est la médiane. Les mesures se font en processus, sur un seul fil, avec l'horloge monotone de chaque langage. La colonne « Premier appel » donne la durée du tout premier appel : chargement paresseux, compilation des schematrons.

Une passe à part, `resources`, mesure le **temps CPU** par opération (utilisateur + système, tous les fils du processus, sur la même fenêtre que le chronomètre) et le **pic de mémoire résidente** d'un processus qui charge une librairie et n'exécute qu'un benchmark, runtime compris (détails dans [`PROTOCOL.md`](PROTOCOL.md)).

Pour la validation, chaque librairie applique le jeu de règles le plus complet dont elle dispose hors ligne, sans règles françaises (le corpus est allemand). Les clients d'API distantes, les librairies qui exigent une JVM hors du banc Java et les paquets vides sont écartés ; la raison est donnée dans chaque `COVERAGE.md`.

Un **plancher** est une librairie générique (PDF ou XML) qui ne connaît pas Factur-X, branchée par quelques lignes dans l'adaptateur : il donne le coût minimal d'une opération dans le langage. En C, il n'existe que des planchers.

## Utilisation

```bash
./run.sh setup     # corpus (via bench/ecosystem), npm ci, venv, cargo, Maven, dotnet, CMake, Composer, artefacts officiels
./run.sh survey    # exactitude sur le corpus, les neuf langages
./run.sh bench     # benchmarks, un langage après l'autre
./run.sh resources # temps CPU et pic mémoire, un processus par librairie et par benchmark
./run.sh report    # results/<date>/tables.md et env.txt
./run.sh all       # les cinq à la suite
```

`survey`, `bench` et `resources` acceptent une liste de langages (`./run.sh bench rust go`) ; chaque dossier a aussi son propre `run.sh`, qui accepte une liste de librairies. Variables : `OUT`, `COUNT` (3), `BENCHTIME` (1, en secondes), et pour `resources` `RES_COUNT` (1) et `RES_BENCHTIME` (1).

Prérequis :

- Go 1.26, Node 22 et npm, Python 3.13 (Homebrew), Rust 1.93, et pour zugferd-rs `libxml2` et `libclang` ;
- Java : JDK 21 ou plus récent (mesuré avec OpenJDK 27 de Homebrew ; `/usr/bin/java` n'est qu'un stub sous macOS, `java/run.sh` prend `JAVA_HOME` ou `/opt/homebrew/opt/openjdk`) et Maven 3 ;
- C# : SDK .NET 11 (`brew install dotnet`) ; pas de Java ;
- C et C++ : CMake, Ninja, pkg-config et `brew install mupdf qpdf libxml2 libxslt podofo poppler xerces-c pugixml freetype openjpeg jpeg-turbo little-cms2` ; SaxonC-HE (binaire officiel) et Kura (sources) sont téléchargés et épinglés par `cpp/run.sh`, pour macOS arm64 seulement ;
- PHP : PHP 8.4 ou plus récent en CLI, avec les extensions listées dans [`php/COVERAGE.md`](php/COVERAGE.md), Composer 2, et `pdfdetach` (poppler) ;
- curl, unzip, shasum et un accès réseau pendant `setup`.

Sur un Apple M4 Pro, les relevés d'exactitude durent une dizaine de minutes, les mesures de temps environ 1 h 25 (dont 17 minutes pour PHP et 16 pour TypeScript, à cause des validateurs lents) et la passe CPU et mémoire environ 55 minutes. Lancer les mesures sur une machine au repos, sur secteur ou en mode hautes performances, capot ouvert (`run.sh` empêche la veille sur inactivité avec `caffeinate -i`, pas celle du capot fermé) : un autre processus qui occupe un cœur, ou une veille, fausse les temps.

## Résultats du 22/11/2026

Apple M4 Pro sur batterie, mode hautes performances (`powermode 2`), Docker Desktop actif en tâche de fond (environ 17 % d'un cœur). Go 1.27.4, Node 22.15.0, CPython 3.13.18, rustc 1.93.1, OpenJDK 27.0.1.1, .NET 11.0.0, Apple clang 21, PHP 8.6.0 sans OPcache ni JIT ; versions complètes dans [`results/2026-11-22/env.txt`](results/2026-11-22/env.txt). Package Go au commit `8d728bd`. Tableaux complets : [`results/2026-11-22/tables.md`](results/2026-11-22/tables.md). Relevés et mesures de temps : 1 h 33 pour les neuf langages, le package Go et 85 librairies ; passe CPU et mémoire : 53 minutes. Ces résultats remplacent ceux du 17/11 (TypeScript, Python et Rust), que garde l'historique git.

### Temps de réponse

Le plus rapide des autres langages pour chaque opération. Un plancher (librairie PDF ou XML générique) donne le coût minimal de l'opération dans son langage ; la librairie Factur-X la plus rapide est donnée à côté quand elle diffère.

| Opération | ZeRechnung (Go) | Plus rapide ailleurs | Écart |
|---|---|---|---|
| Extraction du XML (PDF de 149 Ko) | 29,9 µs | qpdf (C, plancher) : 27,9 µs ; Kura (C++) : 28,9 µs | ×0,93 ; ×0,97 |
| Extraction du XML (PDF de 1,1 Mo) | 33,4 µs | qpdf (C, plancher) : 32,8 µs ; Kura (C++) : 34,3 µs | ×0,98 ; ×1,03 |
| Lecture XML → modèle (13 Ko) | 18,8 µs | pugixml (C++, plancher, arbre DOM sans modèle de facture) : 5,6 µs ; zugferd-rs (Rust) : 24,5 µs | ×0,30 ; ×1,3 |
| Lecture XML → modèle (1,3 Mo, pièce jointe base64) | 1,32 ms | pugixml (C++, plancher) : 238 µs ; john-wink/en16931-php (PHP) : 646 µs | ×0,18 ; ×0,49 |
| Validation, toutes librairies (13 Ko) | 26,4 µs | pdf-invoice (Rust) : 26,1 µs, 14 règles, 20 faux positifs sur 26 | ×0,99 |
| Validation EN 16931 complète (13 Ko) | 26,4 µs | XSLT officiels CEN et KoSIT par Saxon-HE (Java) : 419 µs | ×16 |
| Écriture modèle → XML (13 Ko) | 3,5 µs | pugixml (C++, plancher) : 3,7 µs ; zugferd-rs (Rust) : 6,9 µs | ×1,1 ; ×2,0 |
| Injection dans un PDF à xref classique | 101 µs | pdfa (Rust) : 87,1 µs, échoue sur un hôte à flux xref | ×0,87 |
| Injection dans un PDF à flux xref | 105 µs | MuPDF (C, plancher) : 123 µs ; pdf-invoice (Rust) : 183 µs | ×1,2 ; ×1,7 |
| Contrôle complet d'un PDF hybride (149 Ko) | 863 µs | zugferd-pdf (Python) : 212 µs, contrôles du conteneur seulement | ×0,25 |

Validation EN 16931 complète (schematrons CEN, avec ou sans KoSIT), la plus rapide de chaque langage, sur le XML de 13 Ko :

| Langage | Librairie | Temps | Premier appel | Valides signalés /26 |
|---|---|---|---|---|
| Go | **ZeRechnung (ce dépôt)**, règles compilées | 26,4 µs | — | 6 |
| Java | Saxon-HE + XSLT officiels | 419 µs (×16) | 629 ms | 6 |
| Rust | einvoice-validator (XSLT CEN exécuté en Rust) | 483 µs (×18) | 28 ms | 2 |
| C# | SaxonCS-HE + XSLT officiels | 528 µs (×20) | 707 ms | 6 |
| C++ | SaxonC-HE + XSLT officiels | 862 µs (×33) | 269 ms | 6 |
| TypeScript | @sweberdev/summand | 908 µs (×34) | 17 ms | 6 |
| Python | saxonche + XSLT officiels | 915 µs (×35) | 304 ms | 6 |
| PHP | dealerweb/einvoice (XSLT CEN compilés en PHP) | 8,03 ms (×304) | 24 ms | 6 |

- Les planchers qpdf (C, C++) et pugixml (C++) font mieux que le package Go sur l'extraction, la lecture et l'écriture (à égalité sur l'écriture de 19 Ko) : ils ne font que la partie générique, trouver le flux ou construire un arbre DOM.
- Parmi les librairies Factur-X, le package Go est le plus rapide sur 6 colonnes sur 16 : extraction de 1,1 Mo, lecture de 13 et 19 Ko, écriture, injection dans un hôte à flux xref. Il est à moins de 5 % du premier sur 2 autres : extraction de 149 Ko (Kura, 28,9 µs) et validation de 13 Ko (pdf-invoice, 26,1 µs, 14 règles).
- Les autres colonnes :
  - extraction : Kura (C++) lit le PDF EXTENDED en 28,2 µs (×0,76) ; pdfa (Rust) lit le PDF MINIMUM en 495 ns en cherchant le premier flux `/EmbeddedFile` sans lire la structure du PDF, ce qui rate 72 PDF du corpus ;
  - lecture du XML de 1,3 Mo : le package Go décode la pièce jointe base64 (3,6 Mo alloués par lecture), john-wink/en16931-php (PHP) la lit en 646 µs ;
  - validation : pdf-invoice (Rust, 14 règles) sur 19 Ko, pat-o-dev/factur-x (PHP, quelques règles écrites à la main) sur 1,3 Mo ;
  - injection dans un hôte à xref classique : pdfa (Rust), qui échoue sur un hôte à flux xref ;
  - contrôle d'un PDF (point suivant).
- **Le contrôle d'un PDF hybride est passé de 75 µs (17/11) à 863 µs.** Depuis le commit 3ab8263, `Check` vérifie aussi le contenu des pages et les polices pour PDF/A : le profil CPU montre le temps passé à décompresser et lire les programmes de polices TrueType (`pdfaScan.font`), et 10 fois plus d'allocations (718 Ko par appel au lieu de 71 Ko). Les librairies plus rapides ne contrôlent que le conteneur et le XMP.
- Hors contrôle PDF, le package Go est de 0 à 19 % plus lent que le 17/11, avec les mêmes allocations, sauf l'injection (+137 allocations). L'environnement explique la plus grande part : Go 1.27.4 au lieu de 1.26.4 et Docker actif.
- Les validateurs qui exécutent le schematron officiel coûtent de 0,4 ms à 110 ms par facture. Les plus lents : securibox (C#) 38 ms, mustang (Java) 11 ms, vampire-php (PHP) 64 ms, mcp-einvoicing-de (Python) 108 ms. Leur premier appel compile les feuilles de style : de 17 ms (summand) à 2,1 s (validateur KoSIT). Les règles du package Go sont du code compilé : il n'y a rien à charger.
- Injection : Mustang (Java) met 5,2 ms (`ZUGFeRDExporterFromPDFA`, PDF enregistré sans compression) ; plusieurs librairies refusent les hôtes du banc ou y laissent deux `factur-x.xml` (voir « Particularités connues »).

### Temps CPU et mémoire

Passe `resources` du même jour, de 12 h 17 à 13 h 10, sur secteur : un processus par librairie et par benchmark, une série d'une seconde. Dans chaque langage, le temps réel médian de cette passe est celui de la passe `bench` à 5 % près. Tableaux complets par opération dans `tables.md` (« Temps CPU : … » et « Pic mémoire : … »).

| Langage | Librairies | Processus | Pic mémoire min. | médian | max. | CPU / temps réel médian | max. |
|---|---|---|---|---|---|---|---|
| Go | 1 | 16 | 16,8 Mio | 20,9 Mio | 25,5 Mio | 163 % | 206 % |
| TypeScript | 13 | 119 | 65,4 Mio | 174 Mio | 509 Mio | 102 % | 433 % |
| Python | 8 | 69 | 39,8 Mio | 59,5 Mio | 387 Mio | 100 % | 100 % |
| Rust | 11 | 74 | 1,9 Mio | 7,2 Mio | 503 Mio | 100 % | 111 % |
| Java | 10 | 63 | 259 Mio | 602 Mio | 2 129 Mio | 125 % | 399 % |
| C# | 13 | 111 | 69,6 Mio | 90,0 Mio | 509 Mio | 100 % | 506 % |
| C | 3 | 18 | 2,2 Mio | 5,8 Mio | 11,7 Mio | 100 % | 100 % |
| C++ | 7 | 40 | 1,7 Mio | 9,1 Mio | 278 Mio | 100 % | 100 % |
| PHP | 19 | 135 | 32,0 Mio | 43,0 Mio | 1 173 Mio | 100 % | 100 % |

Sans la série Go à `GOMAXPROCS=1` ni les deux librairies PHP qui lancent des processus enfants. Pic mémoire : mémoire résidente maximale (max RSS) du processus, runtime compris. CPU / temps réel : 100 % = un cœur occupé pendant la série chronométrée.

Par opération, le package Go et la librairie Factur-X la plus rapide de chaque langage (un plancher s'il n'y en a pas) ; pour la validation, la plus rapide qui applique le schematron EN 16931 complet. Dans chaque case : temps CPU par opération, sa part du temps réel, pic mémoire du processus.

| Langage | Extraction (149 Ko) | Lecture XML → modèle (13 Ko) | Validation EN 16931 complète (13 Ko) | Injection (hôte à flux xref) | Contrôle d'un PDF (149 Ko) |
|---|---|---|---|---|---|
| **Go** | 46,9 µs (152 %), 21,1 Mio | 32,0 µs (174 %), 19,2 Mio | 34,5 µs (141 %), 18,7 Mio | 193 µs (201 %), 23,9 Mio | 1,15 ms (133 %), 22,8 Mio |
| **Go, `GOMAXPROCS=1`** | 33,0 µs (99 %), 17,4 Mio | 22,3 µs (99 %), 16,4 Mio | 29,2 µs (99 %), 16,7 Mio | 106 µs (98 %), 18,6 Mio | 892 µs (100 %), 19,9 Mio |
| TypeScript | @attestwire/en16931 : 194 µs (102 %), 142 Mio | @ekwo-ai/factur-x : 72,2 µs (100 %), 104 Mio | @sweberdev/summand : 1,00 ms (111 %), 154 Mio | @stackforge-eu/factur-x : 1,34 ms (109 %), 185 Mio | @attestwire/en16931 : 377 µs (102 %), 157 Mio |
| Python | zugferd-pdf : 79,2 µs (100 %), 40,1 Mio | pycheval : 226 µs (100 %), 42,5 Mio | saxonche + XSLT : 893 µs (100 %), 214 Mio | mcp-einvoicing-de : 667 µs (100 %), 96,8 Mio | zugferd-pdf : 211 µs (100 %), 40,3 Mio |
| Rust | einvoice-validator : 32,6 µs (100 %), 3,1 Mio | zugferd-rs : 24,4 µs (100 %), 6,6 Mio | einvoice-validator : 479 µs (100 %), 26,7 Mio | pdf-invoice : 181 µs (104 %), 9,0 Mio | en16931 : 479 µs (100 %), 5,5 Mio |
| Java | Paniolo : 119 µs (100 %), 272 Mio | imixs-e-invoice : 72,1 µs (100 %), 536 Mio | Saxon-HE + XSLT : 1,96 ms (399 %), 341 Mio | Mustang : 8,66 ms (154 %), 295 Mio | Paniolo : 19,16 ms (165 %), 612 Mio |
| C# | Antiphon : 78,6 µs (100 %), 73,6 Mio | Antiphon : 27,9 µs (100 %), 74,3 Mio | SaxonCS-HE + XSLT : 527 µs (100 %), 142 Mio | Balsoft : 1,07 ms (115 %), 86,8 Mio | OfficeIMO : 229 µs (100 %), 98,0 Mio |
| C | qpdf (plancher) : 28,5 µs (100 %), 3,6 Mio | libxml2 (plancher) : 53,9 µs (100 %), 2,8 Mio | — | MuPDF (plancher) : 127 µs (100 %), 11,5 Mio | — |
| C++ | Kura : 28,3 µs (100 %), 4,4 Mio | pugixml (plancher) : 5,3 µs (100 %), 1,7 Mio | SaxonC-HE + XSLT : 823 µs (100 %), 201 Mio | Kura : 13,03 ms (100 %), 12,1 Mio | Kura : 1,54 ms (100 %), 4,7 Mio |
| PHP | quittio/core : 72,2 µs (100 %), 39,2 Mio | tiime/cross-industry-invoice : 226 µs (100 %), 40,5 Mio | dealerweb/einvoice : 7,61 ms (100 %), 51,4 Mio | dealerweb/einvoice : 2,03 ms (100 %), 54,4 Mio | — |

- **Le package Go occupe plus d'un cœur** : de 125 à 206 % du temps réel, 163 % en médiane. Le fil mesuré est seul, mais le ramasse-miettes concurrent de Go travaille sur d'autres cœurs (`GOMAXPROCS` vaut 14 sur cette machine). Limité à un cœur, le package consomme de 5 à 47 % de CPU en moins pour un temps réel de 4 à 38 % plus long : 33 µs par extraction, 22 µs par lecture, 29 µs par validation, 4,4 µs par écriture, 107 µs par injection. Il reste alors le plus rapide des librairies Factur-X sur la lecture de 13 Ko, l'écriture et l'injection dans un hôte à flux xref.
- **Les runtimes à un seul fil restent à 100 %** : Python, PHP, C, C++ et Rust ; dans Rust, lopdf garde un fil rayon à côté du fil mesuré (104 à 111 % pour zugferd-rs et pdf-invoice).
- **JVM** : 125 % en médiane, jusqu'à 399 % (Saxon-HE sur la validation, 231 % pour imixs-e-invoice sur une lecture). Dans une JVM neuve, la compilation JIT et le ramasse-miettes G1 tournent encore pendant la série d'une seconde ; dans la passe `bench`, JVM chaude, PDFBox est à 101 ou 102 %.
- **.NET** : 100 % en médiane. ZUGFeRD-csharp monte à 438 et 506 % sur l'injection : à chaque appel, une vingtaine de fils du pool de .NET cherchent des polices dans les dossiers du système.
- **Node** : 102 % en médiane ; @stafyniaksacha/facturx monte à 433 % sur l'écriture, avec les fils de V8.
- **Pic mémoire du package Go : 17 à 26 Mio** selon l'opération, runtime compris (16 à 24 Mio à un cœur). C'est 2 à 10 fois plus que Rust, C et C++ (2 à 13 Mio, hors validateurs et XML de 1,3 Mo), moins que Python (au moins 40 Mio, 60 en médiane) et PHP (au moins 32 Mio, 43 en médiane), et bien moins que Node (65 à 509 Mio), .NET (70 à 509 Mio) et la JVM (259 Mio à 2,1 Gio).
- **Java** : le pic mesure surtout la taille du tas G1, pas les données vivantes. Avec les options par défaut, le tas maximal vaut un quart de la mémoire de la machine (6 Gio ici) et G1 le fait grossir sous la pression des allocations : PDFBox garde environ 2 Mo vivants dans un tas de 448 Mo. Le validateur KoSIT atteint 2,1 Gio sur le XML de 1,3 Mo, Mustang 1,8 Gio sur une lecture.
- **Valider avec les XSLT officiels coûte de 140 à 340 Mio** quel que soit le langage (Saxon en Java, C#, C++ et Python), contre 27 Mio pour einvoice-validator (Rust) et 19 Mio pour le package Go.
- **Autres pics** : quittio/core (PHP) 1,1 Gio sur la lecture du XML de 1,3 Mo, En16931 (C#) et @stafyniaksacha/facturx (TS) 509 Mio, erechnung-validator (Rust) 503 Mio, mcp-einvoicing-de (Python) 387 Mio.
- **Processus enfants** : mahdiabderraouf/facturx-php et php-erechnung-toolkit lancent `pdfdetach` (poppler) à chaque extraction ; leur CPU (environ 22 et 26 ms par appel) et leur mémoire ne sont pas comptés (marqués † dans `tables.md`).
- **Un processus neuf ne va pas toujours à la même vitesse** que la passe `bench` : en .NET, plusieurs librairies sont de 2 à 4 fois plus lentes dans un processus d'une seconde (compilation à plusieurs niveaux inachevée) ; Kura, seul dans son processus, injecte et contrôle deux fois plus vite.

### Exactitude sur le corpus

Sur les 26 XML CII valides du corpus, les schematrons officiels (CEN EN 16931 1.3.17 et KoSIT XRechnung) en signalent 6 : BR-CL-25 deux fois, CII-SR-465 quatre fois sur les exemples XRECHNUNG. CII-SR-465 est un avertissement dans les artefacts du CEN ; la configuration KoSIT l'élève au rang d'erreur. Un validateur fidèle au CEN seul en signale donc 2.

| Signalés | Librairies |
|---|---|
| 6, comme la référence | **ZeRechnung (Go)** ; TypeScript : @conformo/toolkit, @sweberdev/summand, en16931 (sans distinguer avertissements et erreurs) ; Python : saxonche + XSLT officiels ; getafix aussi, mais par 6 exceptions de lecture ; Rust : erechnung-validator ; Java : validateur KoSIT, Saxon-HE + XSLT officiels, Paniolo ; C# : SaxonCS-HE + XSLT officiels, pax.XRechnung.NET et Securibox, mais pour d'autres raisons (pax.XRechnung.NET : une règle FeRD sur 2 fichiers et le refus des 4 XRECHNUNG ; Securibox : CII-SR-465 et des contrôles de listes de codes au lieu de BR-CL-25) ; C++ : SaxonC-HE ; PHP : dealerweb/einvoice, intragile/php-einvoicing, vampire-php, quittio/core |
| 2, comme le CEN seul | @attestwire/en16931, @kontor-mcp/core, mcp-einvoicing-de, einvoice-validator, en16931 (Rust), Mustang, phive, International.EInvoicing, Antiphon, En16931 (C#), pat-o-dev/factur-x |
| 1 à 5 | php-erechnung-toolkit (1), @ekwo-ai/factur-x (3, des clés IBAN), ZUGFeRD-csharp (3), atgp/factur-x, tiime/factur-x et facturx-php (4, XSD Factur-X seul), Balsoft (5) |
| 7 à 26, faux positifs | core-invoice (7), zugferd-rs (7), @lucaapp/zugferd-ts (8), @stafyniaksacha/facturx (12), FacturXDotNet (15), factur-x Rust (17), factur-x-kit (18), pdf-invoice (20), OfficeIMO (23), invoice4j (26) |
| 0 | @stackforge-eu/factur-x, @fin.cx/einvoice, factur-x (Python), drafthorse, ph-cii, horstoeko/zugferd, horstoeko/invoicesuite, easybill/zugferd-php, john-wink/en16931-php, stann/factur-x, pn-invoice ; les planchers XSD (System.Xml.Schema, libxml2, Xerces-C, ext-dom) |

Les 104 PDF hybrides sont tous extraits par le package Go et par la plupart des librairies qui savent extraire, planchers compris. Les autres : ZUGFeRD-csharp et MuPDF 103, @stafyniaksacha/facturx, atgp/factur-x et facturx-php 100, pdf-invoice 101, horstoeko/invoicesuite 88, Securibox 74, FacturXDotNet et tiime/factur-x 70, pycheval 65, pdfa 32, pat-o-dev/factur-x 1.

### Normes appliquées

Relevé dans le code des librairies, pas dans leur README (fichiers et lignes dans chaque `COVERAGE.md`). Une sélection : les plus téléchargées et les plus complètes de chaque langage.

| Librairie | Langage | XSD | EN 16931 (CII) | Profils Factur-X | BR-FR | BR-DE (XRechnung) | Peppol BIS 3 en CII | Conteneur PDF/A-3 | UBL |
|---|---|---|---|---|---|---|---|---|---|
| **ZeRechnung (ce dépôt)** | Go | Factur-X 1.09.2, CII D16B (XRECHNUNG), UBL 2.1 | 793 assertions, CEN 1.3.17 | 544 règles (schematrons 1.09 et 1.09.2) | FNFE-MPE 1.4.0.04 | KoSIT 3.0.2, Leitweg-ID et GEBA | 3.0.21 (90 assertions) | PDF-01 à PDF-72, pages et polices PDF/A comprises | lecture, écriture, validation EN 16931, BR-FR, BR-DE, Peppol |
| @stackforge-eu/factur-x (94 000/sem.) | TS | Factur-X 1.09 | non | XSD seul | non | non | non | non | non |
| @e-invoice-eu/core (63 000/sem.) | TS | non | non (génération) | génération | non | génération | non | non | écriture |
| @conformo/toolkit | TS | oui | CEN 1.3.17 (SaxonJS) | XSD seul | Flux 2, si `country: "FR"` | KoSIT 3.0.2 | UBL seulement | partiel | oui |
| @sweberdev/summand | TS | oui | CEN 1.3.16 | XMP comparé à BT-24 | non | KoSIT 2.6.0 | non | partiel | validation |
| @stafyniaksacha/facturx | TS | Factur-X 1.09 | via le schematron Factur-X 1.09 (SaxonJS) | oui, sans gravités | non | non | non | non | non |
| factur-x, Akretion (411 000/mois) | Python | Factur-X 1.09 | seulement par un serveur Saxon HTTP | XSD ; schematron par serveur | 124 règles, par serveur | non | non | non | lecture, écriture |
| drafthorse (157 000/mois) | Python | Factur-X 1.0.07 | non | XSD seul | non | non | non | non | non |
| mcp-einvoicing-de | Python | non | FeRD 1.09.2 ou CEN 1.3.16 (saxonche) | oui (saxonche) | non | KoSIT 3.0.2 | non | non | oui |
| en16931 + en16931-formats (9 700 tél.) | Rust | non | CEN 1.3.17 réécrit, 447 des 522 CII-SR et CII-DT | partiel | non | KoSIT 2.6.0 | oui, 3.0.21 | partiel | oui |
| einvoice-validator | Rust | oui | XSLT CEN 1.3.17 exécuté en Rust | seulement avec la feature `fetch` (téléchargement) | idem | idem | idem | non | validation |
| erechnung-validator | Rust | oui | CEN 1.3.16 | Factur-X 1.09.2, 5 profils | non | KoSIT 3.0.2 | non | non | validation |
| zugferd-rs | Rust | Factur-X 1.09.2 | partiel (XPath 1.0) | XSD | non | non | non | 21 règles veraPDF | oui |
| Mustang (Java) | Java | Factur-X 1.09.2 | à travers le schematron Factur-X EN16931 | 5 profils (Factur-X 1.09.2) | Flux 2 1.3.0, entre entreprises françaises | KoSIT 2.4.0 | non | veraPDF, XMP, pièce jointe | lecture |
| phive + phive-rules | Java | CII D16B, Factur-X 1.09.2 | XSLT CEN 1.3.17 | 5 profils (ZUGFeRD 2.5.2) | paquet séparé | KoSIT 2.6.0, niveaux KoSIT | paquet séparé | non | validation |
| validateur KoSIT | Java | CII D16B | XSLT CEN 1.3.16 | non | non | KoSIT 2.6.0, niveaux KoSIT | non | non | validation |
| Paniolo | Java | CII D16B, Factur-X 1.09.2 | XSLT CEN 1.3.16 (configuration KoSIT) | 5 profils | profil EXTENDED-CTC-FR seulement | configuration KoSIT XRechnung 3.0.2 | non | veraPDF, XMP, relation | lecture, écriture |
| ZUGFeRD-csharp (1,8 M tél.) | C# | non | recalcul des totaux | lecture et écriture, sans règles | non | non | non | non | XRechnung UBL |
| International.EInvoicing | C# | CII D16B | CEN 1.3.16, moteur maison | non hors ligne | non chargé | partiel (52 assertions) | paquet séparé | non | oui |
| Antiphon (commercial) | C# | CII D16B | CEN 1.3.16 précompilé | génération seulement | non | KoSIT 2.6.0, sans niveaux | paquet séparé | non | paquets séparés |
| Kura | C++ | non | non | profil déduit pour le XMP | non | non | non | partiel (charge utile, /AF, XMP) | non |
| horstoeko/zugferd (1,25 M/mois) | PHP | Factur-X, 5 profils | non hors ligne (KoSIT par Java) | XSD seul | non | KoSIT par Java seulement | non | veraPDF par Java seulement | non |
| dealerweb/einvoice | PHP | CII D16B, UBL | XSLT CEN 1.3.16 compilés en PHP | schematron FeRD 1.09.2 | FNFE-MPE 1.4.0 (option) | XRechnung 3.0.2, niveaux KoSIT | BIS 3.0.21 réécrit | non | oui |
| intragile/php-einvoicing | PHP | CII D16B | schematron CEN 1.3.16 précompilé en PHP | schematron Factur-X 1.09.2 | non | XRechnung 2.6.0, niveaux KoSIT | non installé | non | oui |

Aucune librairie, dans aucun des huit autres langages, ne réunit les règles EN 16931 complètes, les règles de profil Factur-X, les règles françaises, les règles allemandes, Peppol en CII et le contrôle du conteneur. Les plus proches :

- **dealerweb/einvoice** (PHP) : CEN, FeRD, KoSIT, BR-FR en option, Peppol réécrit, UBL ; pas de contrôle du conteneur PDF ; 8 ms par validation.
- **Paniolo** (Java) : CEN, KoSIT, schematrons Factur-X, veraPDF pour le conteneur ; BR-FR pour le seul profil EXTENDED-CTC-FR, pas de Peppol ; 0,8 ms par validation, 12 ms par contrôle de PDF.
- **@conformo/toolkit** (TS) : CEN, KoSIT et BR-FR, UBL, création de PDF/A-3 ; pas de schematron de profil Factur-X, Peppol en UBL seulement, dépend de SaxonJS (licence Saxonica) ; 72 ms par validation.

Les librairies les plus téléchargées ne valident pas les règles métier : @stackforge-eu/factur-x, factur-x (Python), atgp/factur-x et horstoeko/zugferd hors ligne s'arrêtent au XSD ; ZUGFeRD-csharp recalcule les totaux ; @e-invoice-eu/core et node-zugferd génèrent seulement.

Le package convertit aussi un PDF quelconque en **PDF/A-3** (conversion partielle, depuis le commit fe8db5e), comme zugferd-rs, @lucaapp/zugferd-ts et @conformo/toolkit ; mcp-einvoicing-de ajoute seulement un OutputIntent sRGB. Mustang et Paniolo confient le contrôle PDF/A à **veraPDF**, l'outil de référence ; le package Go a ses propres contrôles (PDF-01 à PDF-72). Il lit, écrit et valide aussi la syntaxe **UBL**, comme plusieurs de ces librairies, mais le banc ne mesure que le CII.

## Particularités connues

- **pdfa (Rust)** extrait par recherche d'octets : 495 ns sur le PDF MINIMUM, mais 32 PDF sur 104 seulement.
- **Securibox (C#)** échoue sur `Extract_Extended` dans la passe `bench` (« Schema namespace URI and prefix mismatch », exception XMP) mais réussit dans la passe `resources`, seul dans son processus : l'échec dépend des benchmarks déjà passés dans le processus.
- **erechnung-validator** apparaît dans `CheckPDF` avec `beide_pdf`, sa propre fonction, qui extrait et valide le XML sans contrôler le conteneur.
- **en16931 (Rust)** refuse d'injecter dans les deux PDF hôtes, parce que leur XMP contient déjà des propriétés `fx:`. **@stafyniaksacha/facturx** échoue sur tout hôte dont le trailer a un `/ID`.
- **Modèles construits à la main** pour `Write_Small` quand la librairie ne sait pas lire le CII : @stackforge-eu/factur-x, @e-invoice-eu/core, node-zugferd, @ekwo-ai/factur-x, zugferd-pdf, erechnung, zugferd (Rust). Leur `Write_Lines` n'est pas mesuré.
- **Un processus par benchmark** en TypeScript, en Python et en PHP : « Premier appel » y est un appel à froid. En Rust, Java, C#, C et C++, un processus par librairie : en Java et en C#, le premier appel comprend donc le chargement des classes et la compilation par le JIT.
- **Les PDF hôtes ne sont pas vides.** `host.pdf` et `host_classic.pdf` n'ont plus d'arbre `/EmbeddedFiles`, mais leur catalogue garde dans `/AF` la spécification de l'ancien `factur-x.xml` (avec son flux) et un XMP Factur-X. Le contrôle d'`Embed` ne vérifie que la relecture d'un CII ; les effets observés :
  - refus d'injecter une seconde facture : Paniolo (Java), intragile/php-einvoicing et pat-o-dev/factur-x (PHP) ;
  - PDF produit avec deux `factur-x.xml` dans `/AF` : Mustang (Java), Kura (C++), dont le PDF est signalé PDF-28 par `zerechnung validate` ;
  - les planchers C et C++, OfficeIMO (C#) et les autres injecteurs retirent ou remplacent l'ancienne entrée.
- **Java** : le `Extract` de Mustang comprend la lecture du modèle (pas d'API d'extraction brute). Mustang et phive appliquent leurs propres niveaux de gravité et laissent CII-SR-465 en avertissement. Aucune option de JVM : la pile de 512 Mio vient d'un fil créé par le harnais.
- **C#** : les programmes tournent par `dotnet <dll>` (l'apphost ne trouve pas un .NET Homebrew sans `DOTNET_ROOT`) ; culture invariante fixée dans le code (`InvariantGlobalization` casse SaxonCS) ; pile de 256 Mio pour les XSLT officiels. Antiphon est un logiciel commercial fermé, mesuré en licence d'évaluation.
- **C et C++** : horloge `CLOCK_UPTIME_RAW` sous macOS (celle de `Instant` en Rust), parce que `CLOCK_MONOTONIC` y avance par pas de 1 µs. Les planchers n'ajoutent pas d'OutputIntent et ne convertissent pas en PDF/A ; Kura convertit en PDF/A-3b, d'où ses 12,6 ms (hôte à flux xref) et 25 ms (xref classique) par injection. libxslt ne fait que XSLT 1.0 : les XSLT officiels (XSLT 2.0) n'y compilent pas.
- **PHP** : PHP CLI tel qu'installé, OPcache éteint (`opcache.enable_cli=0`), donc sans JIT ; `memory_limit=4G` et `zend.assertions=-1` passés par `PHP_OPTS`. Aucun moteur XSLT 2.0 n'est utilisable en PHP 8.6 (ext-xsl est en XSLT 1.0, l'extension SaxonC 13.1.1 ne compile pas avec PHP 8.6) : les validateurs complets sont des réécritures en PHP. Le parseur gratuit de FPDI ne lit pas les flux xref, d'où les échecs d'`Embed_Small` des injecteurs FPDI ou mPDF. L'installation passe `--no-scripts --no-plugins`.
- **Installation** : `npm ci --ignore-scripts` (le postinstall de @fin.cx/einvoice télécharge des schematrons non figés), wheels Python seulement (sauf `tagic`, pur Python, relu), `cargo build --ignore-rust-version` pour einvoice-validator, `RAYON_NUM_THREADS=1` pour lopdf.
- Le contrôle complet d'un PDF par le package Go mesure 863 µs (2 376 allocations) au commit `8d728bd`, contre 75,0 µs (504 allocations) en v0.1.0 le 17/11 : depuis le commit 3ab8263, `zerechnung.Check` vérifie aussi le contenu des pages et les polices pour PDF/A, et décompresse pour cela les programmes de polices.

## Licences

Le code de ce dossier est sous licence MIT, comme le dépôt. Les dépendances sont installées par `run.sh setup` et ne sont pas importées par le package `zerechnung`. Certaines ont des licences restrictives :

- AGPL-3.0 : pdf-invoice (Rust), MuPDF (C), Kura (C++), php-erechnung-toolkit et php-pdf-toolkit (PHP) ;
- GPL : poppler (C++), vampire-php (PHP), veraPDF (GPL-3.0 ou MPL-2.0, tiré par Mustang et Paniolo en Java) ;
- LGPL : PoDoFo (C++), smalot/pdfparser (PHP) ;
- EUPL-1.2 : @stackforge-eu/factur-x, En16931 (C#), intragile/einvoicing-rules-en16931 (PHP), artefacts CEN ;
- MPL-2.0 : saxonche, Saxon-HE (Java), SaxonCS-HE (C#), SaxonC-HE (C++) ; licence Saxonica : SaxonJS, utilisé par @conformo, @kontor-mcp et @stafyniaksacha ;
- licence commerciale fermée : Antiphon (C#), mesuré en licence d'évaluation (gratuite, sans clé, « pour évaluer avant achat ») ; le banc C# épingle PDFsharp 6.2.4 et 7.0.0-preview-1 (International.EInvoicing, Balsoft, Antiphon, Securibox, plancher), versions MIT : PDFsharp 6.5.0 et suivantes, et 7.0.0-preview-2, sont en AGPL-3.0 ;
- sans licence déclarée : stann/factur-x (PHP).

Les artefacts officiels téléchargés par les bancs Python, Java, C# et C++ (CEN 1.3.17, KoSIT schematron 2.6.0 et configuration XRechnung) sont vérifiés par SHA-256 et ne sont pas versionnés.
