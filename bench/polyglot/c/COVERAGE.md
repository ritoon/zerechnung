# Couverture : écosystème C

Banc C du protocole commun (`../PROTOCOL.md`). Un exécutable par librairie
(`build/bench-<lib>`, source `libs/<lib>/main.c`), un harnais partagé
(`harness/harness.c`, repris par le banc C++), construction CMake en `Release`
(drapeaux par défaut de CMake : `-O3 -DNDEBUG`, aucune option propre au
processeur). Compilateur : Apple clang 21.0.0, CMake 4.5.1, Ninja. Les
librairies viennent de Homebrew (macOS arm64).

**Il n'existe pas de librairie C dédiée à Factur-X, ZUGFeRD, XRechnung ou
EN 16931.** Ce banc mesure donc surtout des **planchers** : des librairies PDF
ou XML généralistes, avec quelques dizaines de lignes écrites dans
l'adaptateur. La seule fonction propre à ZUGFeRD de l'écosystème C est
`pdf_zugferd_xml()` de MuPDF, qui sait lire la pièce jointe d'un PDF hybride
et rien de plus. Aucune librairie C ne valide les règles EN 16931 : libxslt,
le moteur XSLT du monde C, ne fait que du XSLT 1.0 et ne peut pas exécuter
les XSLT officiels (XSLT 2.0). La validation de référence par les artefacts
officiels se trouve dans le banc C++ (SaxonC-HE).

Les preuves citées sont des chemins dans les sources des librairies, au tag de
la version construite (dépôt GitHub ou en-têtes installés par Homebrew dans
`/opt/homebrew/opt/<formule>/include`), et dans les adaptateurs de ce banc.
Popularité relevée le 2026-11-22 : étoiles GitHub, date de la version
mesurée (tag), installations Homebrew sur 365 jours (`formulae.brew.sh`,
surtout comme dépendance d'autres formules). Il n'y a pas de registre de
téléchargements pour le C.

## Recensement

- GitHub, recherche de dépôts en langage C : `zugferd`, `factur-x`,
  `facturx`, `xrechnung`, `en16931`, `e-rechnung`, `CrossIndustryInvoice`,
  `einvoice`, `e-invoice`. Un seul résultat pertinent,
  `miyako/4d-plugin-factur-x` (plugin 4D, voir les écartées).
- GitHub, recherche de code en C : `"urn:cen.eu:en16931:2017"`,
  `rsm:CrossIndustryInvoice` et `"factur-x.xml"` ne trouvent que MuPDF
  (`source/pdf/pdf-zugferd.c`) et ses copies embarquées (SumatraPDF et
  autres), ainsi que la démo `demo/pdf_a_conformance.c` de libharu et ses
  copies.
- vcpkg (2 917 ports) et Conan Center (1 982 recettes) : aucun port de
  facture électronique ; seulement les librairies généralistes (`libmupdf`,
  `qpdf`, `libxslt`, `poppler`…).
- Homebrew : aucune formule `zugferd`, `factur`, `xrechnung` ou `einvoice`
  (la formule `invoice` est un générateur de factures PDF en Go, sans
  rapport). Debian : aucun paquet source de ces noms (recherche
  `sources.debian.org`).

## Librairies mesurées

### MuPDF 1.28.6 (Artifex) : lecture ZUGFeRD de la librairie, injection en plancher

Librairie PDF généraliste (rendu, édition). Elle contient un petit module
ZUGFeRD, `source/pdf/pdf-zugferd.c` (277 lignes au tag `1.28.6`), déclaré
dans `include/mupdf/pdf/zugferd.h:28-47`.

- Extraction (fonction de la librairie) : `pdf_zugferd_xml()`
  (`pdf-zugferd.c:216-257`). Elle lit le XMP, cherche une description
  `rdf:Description` qui porte l'attribut `xmlns:zf` =
  `urn:ferd:pdfa:CrossIndustryDocument:invoice:1p0#` (ZUGFeRD 1.0,
  `pdf-zugferd.c:74`), `xmlns:fx` = `urn:zugferd:pdfa:CrossIndustryDocument:invoice:2p0#`
  (ZUGFeRD 2.x, `:113`) ou `xmlns:fx` =
  `urn:factur-x:pdfa:CrossIndustryDocument:invoice:1p0#` (Factur-X, `:155`),
  prend le nom déclaré dans `DocumentFileName`, puis charge ce fichier parmi
  les fichiers associés `/AF` du document (`:232-247`). Pas de déclaration
  XMP, ou fichier absent de `/AF` : pas de facture.
- La recherche porte sur le préfixe et non sur l'espace de noms : le PDF
  Mustang `MustangGnuaccountingBeispielRE-20190610_507.pdf` lie l'espace
  ZUGFeRD 2.0 au préfixe `zf:` (`xmlns:zf="urn:zugferd:pdfa:CrossIndustryDocument:invoice:2p0#"`),
  MuPDF ne le reconnaît pas : 103 PDF sur 104.
- `pdf_zugferd_profile()` (`:211`) ne fait que lire le profil déclaré dans le
  XMP (ZUGFeRD 1.0 COMFORT, BASIC, EXTENDED ; 2.x MINIMUM, BASIC WL ;
  XRECHNUNG). Aucun contrôle : CheckPDF est `unsupported`.
- Injection : pas de fonction de la librairie. Plancher écrit dans
  `libs/mupdf/main.c:94-183` avec les primitives de MuPDF :
  `pdf_add_embedded_file` (`:123`), insertion triée dans
  `/Names/EmbeddedFiles`, `/AF` (un fichier de facture laissé par l'hôte est
  remplacé), `/AFRelationship` `Alternative` (ou `Data` en MINIMUM et
  BASIC WL), `/Desc`, nouveau flux `/Metadata` avec un XMP Factur-X écrit par
  le harnais (`fx_facturx_xmp`), réécriture complète par
  `pdf_write_document` (options par défaut, `:164`). Pas de conversion
  PDF/A. Relu par `pdf_zugferd_xml()`, qui exige le XMP et `/AF` : la relecture
  prouve donc le conteneur.
- Contrôle du PDF produit (hors banc, `FX_DUMP_EMBED`) : `zerechnung validate`
  ne trouve aucune erreur (avertissements : pas d'output intent PDF/A, `/Info`
  sans équivalent XMP) ; `checkInvoice` de Kura le juge cohérent ;
  `qpdf --check` sans erreur.
- Pas de lecture, d'écriture ni de validation du XML.
- Réseau : aucun.

### qpdf 12.4.2, API C (plancher d'extraction)

qpdf est écrite en C++ ; elle publie une API C (`qpdf/qpdf-c.h`,
`qpdf/qpdf-c_objects.h`). Plancher d'extraction écrit dans
`libs/qpdf-c/main.c` : `qpdf_read_memory` (`:54`), parcours de
`/Names/EmbeddedFiles` avec `/Kids` (`find_in_tree`, `:18`, profondeur
bornée à 32), premier fichier sous un nom de facture standard (`/UF` puis
`/F`), flux décodé par `qpdf_oh_get_stream_data(…, qpdf_dl_generalized)`
(`:81`). 104 PDF sur 104. L'API C n'expose pas les assistants de pièces
jointes (`QPDFEmbeddedFileDocumentHelper`) : l'injection est mesurée avec
l'API C++ dans le banc C++.

### libxml2 2.15.4 (plancher XML)

La librairie XML du monde C (et de lxml). Aucune connaissance de Factur-X.

- Parse : DOM par `xmlCtxtReadMemory` avec `XML_PARSE_NONET`
  (`libs/libxml2/main.c:53-70`), un contexte de lecture par appel.
- Write : `xmlDocDumpMemory` du DOM (`:88-100`).
- ParseValidate : même lecture, puis `xmlSchemaValidateDoc` (`:146`) contre
  le XSD UN/CEFACT CII D16B (`CrossIndustryInvoice_100pD16B.xsd` et ses 3
  schémas importés) de la configuration KoSIT XRechnung 3.0.2 du 2026-08-31,
  archive épinglée par SHA-256 dans `run.sh`. Le schéma est compilé au premier
  appel (`compiled`, `:107-131` ; coût dans `first_call_ns` de
  `ParseValidate_Small`). Le schéma seulement : aucune règle métier
  EN 16931. Les 26 exemples valides passent ; un fichier corrompu à la main
  (élément `ram:Bogus`) est bien signalé.
- Réseau : aucun (`XML_PARSE_NONET`, schémas locaux).

### libxslt 1.1.45 (vérifié : ne peut pas exécuter les XSLT officiels)

libxslt n'implémente que XSLT 1.0 (avec EXSLT). Les XSLT officiels sont en
XSLT 2.0. L'exécutable `bench-libxslt` le vérifie à chaque lancement :
`xsltParseStylesheetFile` (`libs/libxslt/main.c:73`) sur
`cen-1.3.17/xslt/EN16931-CII-validation.xslt` (version `2.0`, 810 règles)
renvoie 36 erreurs de compilation, la première étant
`xsl:when : could not compile test expression 'xs:decimal(ram:LineTotalAmount) = …'` ;
sur la feuille KoSIT `XRechnung-CII-validation.xsl` (schematron 2.6.0), 170
erreurs. ParseValidate est donc `unsupported`, avec ces nombres dans le
message ; les autres opérations sont `unsupported` (« moteur XSLT
seulement »).

## Librairies écartées

| Projet | Étoiles, activité, licence | Raison |
|---|---|---|
| miyako/4d-plugin-factur-x | ★2, 2026-06-18, MIT | Plugin pour la base 4D (API `4DPluginAPI`, types `C_BLOB`, `C_TEXT`) : pas une librairie utilisable hors de 4D. Les variantes C++ (PoDoFo, QPDF) sont dans le recensement C++ |
| libharu 2.4.6 | ★2 005, 2026-03-26, Zlib | Écrit des PDF neufs seulement, ne lit ni ne modifie un PDF existant : pas d'extraction ni d'injection dans un hôte. Sa démo `demo/pdf_a_conformance.c` attache un `factur-x.xml` à un document qu'elle crée |
| PDFio | ★362, 2026-11-17, Apache-2.0 | Librairie PDF généraliste, pas de formule Homebrew ni de port vcpkg/Conan, pas d'assistant de pièces jointes : un plancher de plus, comme qpdf-c |
| Ghostscript (`lib/zugferd.ps`) | ghostpdl ★304, AGPL-3.0 | Programme PostScript pour `gs` qui produit un PDF/A-3 ZUGFeRD par re-distillation d'un fichier ; pas une API en mémoire, `gs` non installé |
| PDFium | Google, BSD-3-Clause | API d'extraction de pièces jointes (`FPDFDoc_GetAttachment`), mais ni formule Homebrew ni port de version stable ; les planchers d'extraction sont couverts par qpdf-c et MuPDF |
| Duncans-Suite (`docs/FacturX.h`) | ★2, 2026-11-21, pas de licence | Module interne d'une application Win32 (C++), voir le recensement C++ |
| SDK commerciaux (PDFlib, Datalogics APDFL, Apryse, PDF-XChange, VPE) | propriétaires | Fonctions ZUGFeRD / Factur-X annoncées par les éditeurs, non vérifiées ; licences payantes, hors protocole |

## Relevés d'exactitude (`./run.sh survey`, essai du 2026-11-22)

| Librairie | validate (signalés / 26) | parse (/ 26) | extract (/ 104) |
|---|---|---|---|
| mupdf | — | — | 103 (préfixe `zf:` sur l'espace ZUGFeRD 2.0 d'un PDF Mustang) |
| qpdf-c | — | — | 104 |
| libxml2 | 0 (XSD seulement) | 26 | — |
| libxslt | — (`unsupported`) | — | — |

Référence (PROTOCOL) : 6 sur 26. Le XSD seul ne voit aucune des six : ce
sont des règles Schematron (BR-CL-25, CII-SR-465).

## Statut des benchmarks (essai `COUNT=1 BENCHTIME=0.05`)

`o` ok, `F` fail (message dans la ligne JSONL), `-` unsupported. Colonnes dans
l'ordre :

1. Extract S/L/Min/Ext
2. Parse S/Li/La
3. ParseValidate S/Li/La
4. Write S/Li
5. Embed, EmbedClassic
6. CheckPDF S/L

| Librairie | Extract | Parse | ParseValidate | Write | Embed | CheckPDF |
|---|---|---|---|---|---|---|
| mupdf | o o o o | - - - | - - - | - - | o o (plancher) | - - |
| qpdf-c | o o o o | - - - | - - - | - - | - - | - - |
| libxml2 | - - - - | o o o | o o o (XSD) | o o | - - | - - |
| libxslt | - - - - | - - - | - - - | - - | - - | - - |

## Tableau de couverture

| Librairie | Version | Licence | Dernière version | Téléchargements | Lecture CII | Écriture CII | UBL | Extraction PDF | Injection PDF | XSD | EN 16931 (CII) | Profils Factur-X (MINIMUM, BASIC WL, EXTENDED) | BR-FR | BR-DE (XRechnung) | Peppol BIS 3 | Contrôle du conteneur PDF/A-3 | Dépendances externes |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| MuPDF | 1.28.6 | AGPL-3.0-or-later ou commerciale (Artifex) | 1.28.6 (2026-10-29) | ★3 066 ; Homebrew 16 036 / an | non | non | non | oui : `pdf_zugferd_xml` (XMP fx:/zf: puis `/AF`), 103/104 | plancher écrit à la main (pas de fonction de la librairie) | non | non | lecture du profil déclaré dans le XMP seulement | non | non | non | non | aucune au moment de l'appel (libmupdf ; freetype, harfbuzz, openjpeg, jbig2dec, gumbo… à la construction) |
| qpdf-c (plancher) | 12.4.2 | Apache-2.0 | 12.4.2 (2026-09-26) | ★5 606 ; Homebrew 85 709 / an | non | non | non | plancher écrit à la main, 104/104 | non (voir le banc C++) | non | non | non | non | non | non | non | libqpdf (C++), jpeg-turbo, openssl |
| libxml2 (plancher) | 2.15.4 | MIT | 2.15.4 (2026-09-04) | ★771 (miroir GNOME) ; Homebrew 562 672 / an | DOM seulement | sérialisation du DOM | non | non | non | oui : CII D16B (configuration KoSIT 2026-08-31) | non | non | non | non | non | non | aucune |
| libxslt | 1.1.45 | X11 | 1.1.45 (2025-11-30) | ★75 (miroir GNOME) ; Homebrew 101 579 / an | non | non | non | non | non | non | non : XSLT 1.0, les XSLT officiels (2.0) ne compilent pas | non | non | non | non | non | libxml2, libgcrypt |

## Notes de construction et de mesure

- `./run.sh setup` télécharge trois archives officielles vérifiées par
  SHA-256 (configuration KoSIT XRechnung 3.0.2 du 2026-08-31, CEN
  `validation-1.3.17` CII, KoSIT `xrechnung-schematron` 2.6.0 ; les mêmes que
  le banc Python), les décompresse dans `.artefacts/`, puis construit chaque
  cible à part. Une librairie absente est sautée à la configuration.
- libxml2 et libxslt sont keg-only dans Homebrew : `run.sh` passe leurs
  préfixes dans `CMAKE_PREFIX_PATH`. `libqpdf.pc` ajoute `-std=c++20` à ses
  `Cflags`, refusé par le compilateur C : qpdf est trouvée par `find_path` et
  `find_library`.
- Horloge : `clock_gettime(CLOCK_UPTIME_RAW)` sur macOS, `CLOCK_MONOTONIC`
  ailleurs. Sur macOS, `CLOCK_MONOTONIC` n'avance que par microsecondes
  (résolution mesurée 1 000 ns, contre 42 ns pour `CLOCK_UPTIME_RAW`, l'horloge
  de `std::time::Instant` de Rust sur cette plateforme).
- Comme dans le harnais Rust : appel de contrôle hors mesure, puis N par
  paliers 1, 2, 5, 10… jusqu'à ce qu'une série dure `BENCHTIME`, puis
  `COUNT` séries ; un seul fil, le corps de chaque exécutable tourne dans un
  fil à pile de 512 Mio ; les résultats sont libérés dans la boucle mesurée et
  un puits `volatile` empêche le compilateur de supprimer l'appel.
- Temps CPU et pic mémoire (harnais partagé avec le banc C++) :
  `getrusage(RUSAGE_SELF)`, temps utilisateur + système de tous les fils, lu
  avant et après chaque série (`cpu_ns_per_op`) ; `ru_maxrss` (octets sur
  macOS) lu à la fin de la série dans la passe `resources`, où `run.sh` lance
  directement `build/bench-<lib> resources <Bench>`, un processus par
  benchmark. Aucune des quatre librairies ne lance de fil ni de processus
  enfant.
- Une erreur de librairie devient un `fail` avec son message. Si une
  librairie tue le processus (SIGSEGV, SIGBUS, SIGABRT…), un gestionnaire de
  signal écrit la ligne `fail` (ou la ligne de relevé) préparée avant l'appel,
  puis quitte avec le code 70 : la ligne JSONL reste complète.
- Le contrôle de l'injection relit le PDF produit avec l'extraction de la
  même librairie et exige un XML CII, comme le harnais Rust. Avec
  `FX_DUMP_EMBED=<dossier>`, le PDF de cet appel de contrôle est conservé
  pour l'inspecter avec d'autres outils (c'est ainsi que les PDF de MuPDF ont
  été passés à `zerechnung validate`, à Kura et à `qpdf --check`).
- Les PDF hôtes du protocole (`host.pdf`, `host_classic.pdf`) n'ont plus de
  `/Names/EmbeddedFiles`, mais gardent dans `/AF` le `factur-x.xml` du PDF
  intarsys dont ils viennent, ainsi que son XMP Factur-X. Le plancher MuPDF
  retire de `/AF` tout fichier de facture avant d'ajouter le sien ; sans cela,
  `zerechnung validate` signale deux pièces jointes de facture (PDF-28).
