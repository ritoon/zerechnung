# Couverture : écosystème C++

Banc C++ du protocole commun (`../PROTOCOL.md`). Un exécutable par librairie
(`build/bench-<lib>`, source `libs/<lib>/main.cpp`), le harnais du banc C
(`../c/harness/harness.c`, compilé en C) vu par sa façade C++
(`harness/harness.hpp`), construction CMake en `Release` (drapeaux par défaut
de CMake : `-O3 -DNDEBUG`, aucune option propre au processeur), C++20.
Compilateur : Apple clang 21.0.0, CMake 4.5.1, Ninja. Les librairies viennent
de Homebrew (macOS arm64), sauf Kura (source épinglée) et SaxonC-HE (binaire
officiel épinglé).

**L'écosystème C++ n'a pas de librairie dédiée qui lise, valide ou écrive le
CII.** Une seule librairie maintenue connaît Factur-X : **Kura**
(bentopdf-kura, AGPL-3.0, ★25, version 1.4.1 du 2026-11-08), un moteur PDF/A
qui lit, attache et contrôle la facture **au niveau du conteneur PDF**, sans
jamais analyser le XML. La validation EN 16931 n'existe en C++ que par
**SaxonC-HE**, le processeur XSLT 3.0 de Saxonica compilé en librairie native,
qui exécute les XSLT officiels du CEN et de la KoSIT : c'est le validateur de
référence du banc, pas une librairie de facture. Tout le reste est un
**plancher** : qpdf, PoDoFo et poppler-cpp pour le PDF, Xerces-C (avec le XSD
CII) et pugixml pour le XML, avec quelques dizaines de lignes écrites dans
l'adaptateur.

Les preuves citées sont des chemins dans les sources des librairies, au tag de
la version construite (dépôt GitHub, archive épinglée dans `.artefacts/`, ou
en-têtes installés par Homebrew dans `/opt/homebrew/opt/<formule>/include`),
et dans les adaptateurs de ce banc. Popularité relevée le 2026-11-22 : étoiles
GitHub, date de la version mesurée (tag), installations Homebrew sur 365 jours
(`formulae.brew.sh`, surtout comme dépendance d'autres formules). Il n'y a
pas de registre de téléchargements pour le C++.

## Recensement

- GitHub, recherche de dépôts en C++ : `zugferd`, `factur-x`, `facturx`,
  `xrechnung`, `en16931`, `e-rechnung`, `CrossIndustryInvoice`, `einvoice`,
  `e-invoice`. Recherche de code en C++ : `"urn:cen.eu:en16931:2017"`,
  `rsm:CrossIndustryInvoice`, `"factur-x.xml"`, `ZUGFeRD`. Résultats : Kura,
  les projets de la table des écartées, des applications (KDE Kraft,
  YACAppServer, UltraCanvas, Duncans-Suite, PDFE) et des exemples de SDK
  commerciaux (Datalogics, PDF-XChange).
- vcpkg (2 917 ports) et Conan Center (1 982 recettes) : aucun port de
  facture électronique ; seulement `qpdf`, `podofo`, `poppler`, `xerces-c`,
  `pugixml`, `libxslt` (et `libmupdf` dans vcpkg).
- Homebrew : aucune formule `zugferd`, `factur`, `xrechnung` ou `einvoice`.
  La formule `saxon` est Saxon-HE pour Java, pas SaxonC. Debian : aucun
  paquet source de ces noms.

## Librairies mesurées

### Kura 1.4.1 (bentopdf-kura) : dédiée, au niveau du conteneur PDF

Moteur C++ de conversion et de contrôle PDF/A, PDF/X et facture électronique,
bâti sur qpdf. Source épinglée : archive du tag `v1.4.1`
(SHA-256 `18353ceb…43fb8` dans `run.sh`), dont seul `pdfa-engine/` est
décompressé ; construit par `add_subdirectory` avec la CLI, PDFium, zxing-cpp,
QuickJS, les fuzzers et le test du SDK désactivés (`CMakeLists.txt`), lié à
la qpdf de Homebrew. Il connaît Factur-X, ZUGFeRD, XRechnung et Order-X au
niveau du PDF ; il ne lit pas le CII (profil détecté par motifs de texte,
`core/src/einvoice.cpp:92` `detectInvoice`).

- Extraction : `pdfa::readInvoice` (`core/src/einvoice_read.cpp:56`), premier
  fichier de `/Names/EmbeddedFiles` sous un des noms `factur-x.xml`,
  `zugferd-invoice.xml`, `ZUGFeRD-invoice.xml`, `xrechnung.xml`,
  `order-x.xml` (`:72-73`). Le XMP n'est pas consulté. 104 PDF sur 104.
- Injection : `kura_convert(host, "3b", &options)` avec `invoice_xml`
  (`core/include/kura/kura.h:114`, `capi/kura_c.cpp:203`), le point d'entrée
  du SDK. Il convertit tout l'hôte en PDF/A-3b (polices, espaces de couleur,
  métadonnées) et attache la facture (`attachInvoiceXml`,
  `core/src/sanitize.cpp:990`, appelé à `:1347`) avec `/AF`,
  `/AFRelationship` et le schéma d'extension XMP `fx:` ; le profil et le nom
  du fichier viennent de `detectInvoice`. Toute la conversion PDF/A est dans
  la mesure (environ 12,6 ms contre 0,1 à 0,5 ms pour les planchers, qui ne
  convertissent rien).
- Particularité relevée : les hôtes du protocole gardent dans `/AF` le
  `factur-x.xml` du PDF intarsys dont ils viennent. Kura réinscrit ce fichier
  orphelin dans `/Names/EmbeddedFiles` (`sanitize.cpp:825-870`), puis
  `attachAssociatedFile` (`:934`) ajoute la nouvelle facture sans remplacer
  l'ancienne : le PDF produit contient deux `factur-x.xml`, et
  `readInvoice` relit le premier, l'ancien (la même facture en CRLF, 13 396
  octets contre 13 153). `zerechnung validate` signale une seule erreur sur ce
  PDF (PDF-28, deux pièces jointes de facture), aucun avertissement. Le banc
  l'accepte, car la relecture trouve un CII, comme le harnais Rust ; la note
  de la ligne JSONL le dit.
- CheckPDF : `kura_read_invoice` (`kura.h:123`, `kura_c.cpp:268`), qui appelle
  `pdfa::checkInvoice` (`core/src/einvoice_check.cpp:8-95`) : URN de la
  spécification, présence de l'identifiant, du code de type et de la date,
  plus d'un XML attaché, avertissement MINIMUM / BASIC WL, nom de la pièce
  jointe, `/AFRelationship`, présence dans `/AF`, et dans le XMP
  `DocumentFileName`, `ConformanceLevel`, `DocumentType`. Ces contrôles
  s'appuient sur `std::regex` (`einvoice_check.cpp:31`), d'où environ 1,4 ms
  par appel. Pas une validation PDF/A : une incohérence trouvée fait partie
  du rapport, pas d'un échec.
- Pas de lecture, d'écriture ni de validation du XML.
- Réseau : aucun.

### SaxonC-HE 13.1.1 (Saxonica) : validateur de référence

Saxon-HE compilé en librairie native par GraalVM `native-image` (pas de JVM),
avec une API C++ (`saxonc/SaxonProcessor.h`, `Xslt30Processor.h`). Binaire
officiel pour macOS arm64 (`SaxonCHE-macos-arm64-13-1-1.zip`, fichiers datés
du 2026-10-26). Saxonica ne publie pas de somme de contrôle : celle de
`run.sh` (`8f164115…8630281`) a été relevée au premier téléchargement. C'est
la même librairie que `saxonche`, la liaison Python utilisée par
`../python/adapters/saxonche_schematron.py`, avec les mêmes artefacts et le
même décompte.

- ParseValidate (`libs/saxonc-he/main.cpp:155-187`) : feuilles compilées une
  fois au premier appel (`compile`, `:59-87`, coût dans `first_call_ns` de
  `ParseValidate_Small`, environ 140 ms), puis à chaque appel
  `parseXmlFromString` et `transformToString` :
  - XSLT officiel CEN `EN16931-CII-validation.xslt` de `validation-1.3.17`
    (ConnectingEurope/eInvoicing-EN16931, XSLT 2.0, 810 règles : CII-SR 484,
    CII-DT 103, BR 58, BR-CL 23, BR-CO 23, BR-DEC 21, BR-IC 12, BR-O 14,
    BR-S, BR-Z, BR-E, BR-AE, BR-IG, BR-IP, BR-G 10 chacune, BR-B 2 ; 339
    marquées `fatal`, 471 `warning`) pour chaque facture ;
  - en plus, quand BT-24 contient `xrechnung` (`is_xrechnung`, `:90-110`,
    même test que l'adaptateur Python), la feuille KoSIT
    `XRechnung-CII-validation.xsl` de `xrechnung-schematron` 2.6.0
    (XRechnung 3.0.2, 74 règles : 31 BR-DE, 7 BR-DEX, 7 BR-DE-CVD, 22
    PEPPOL-EN16931-R, 7 transitoires), comme le scénario CII de la
    configuration KoSIT.
- Décompte (`errors`, `:119-149`) : chaque `failed-assert` ou
  `successful-report` du SVRL marqué `fatal` ou `error`, après les niveaux
  propres à la configuration KoSIT du 2026-08-31 dans le scénario XRechnung
  (CII-SR-465 et CII-SR-466 relevées en `error`, CII-SR-475 et CII-SR-476
  abaissées en `information`), appliqués aux deux rapports comme le fait
  l'adaptateur Python. Le SVRL est lu avec pugixml dans l'appel mesuré.
- Résultat : 6 factures signalées sur 26 (BR-CL-25 ×2, CII-SR-465 ×4), la
  référence du protocole. Sur un fichier corrompu à la main (montant total
  faux), BR-CO-15 et BR-CO-16 sont bien signalées.
- Pas de XSD (absent des artefacts CII du CEN) ; aucune règle française ;
  pas d'UBL ni de Peppol BIS mesurés (les XSLT UBL du CEN ne sont pas chargés).
- Réseau : aucun (feuilles locales ; `setcwd` vers `.artefacts`).

### qpdf 12.4.2 (plancher PDF)

La librairie PDF « qui préserve le contenu » du monde C++, sans connaissance
de Factur-X.

- Extraction (`libs/qpdf/main.cpp:37-53`) : `processMemoryFile`,
  `QPDFEmbeddedFileDocumentHelper::getEmbeddedFiles()` (tout l'arbre
  `/Names/EmbeddedFiles`), premier fichier sous un nom de facture standard,
  flux décodé par `getStreamData(qpdf_dl_generalized)`. 104 PDF sur 104.
- Injection (`:55-100`) : `QPDFEFStreamObjectHelper::createEFStream`,
  `QPDFFileSpecObjectHelper::createFileSpec`, `replaceEmbeddedFile` ; à la
  main `/AFRelationship` (`Alternative`, ou `Data` en MINIMUM et BASIC WL,
  `:69`), `/AF` reconstruit sans le fichier de facture laissé par l'hôte
  (`:75-86`), nouveau `/Metadata` avec le XMP Factur-X du harnais (`:91-93`) ;
  écriture complète par `QPDFWriter` en mémoire avec
  `setNewlineBeforeEndstream(true)` (`:97`, exigé par PDF/A). Pas de
  conversion PDF/A.
- Contrôle du PDF produit (hors banc, `FX_DUMP_EMBED`) : `zerechnung
  validate` sans erreur (avertissements : pas d'output intent PDF/A, `/Info`
  sans équivalent XMP), `checkInvoice` de Kura le juge cohérent,
  `qpdf --check` sans erreur.

### PoDoFo 1.1.3 (plancher PDF)

Librairie PDF généraliste en C++17, sans connaissance de Factur-X.

- Extraction (`libs/podofo/main.cpp:24-45`) :
  `PdfMemDocument::LoadFromBuffer`, arbre `PdfEmbeddedFiles` parcouru par
  `ToDictionary` (avec les `/Kids`), premier fichier sous un nom de facture
  standard, `PdfFileSpec::GetEmbeddedData`. 104 PDF sur 104.
- Injection (`:47-104`) : `CreateFileSpec` et `SetEmbeddedData`, ajout dans
  l'arbre `PdfEmbeddedFiles` (`:71`) ; à la main `/AFRelationship` par profil
  (`:56`), `/AF` reconstruit sans le fichier de facture de l'hôte
  (`:77-90`), nouveau `/Metadata` XMP Factur-X (`:94-97`) ;
  `PdfMemDocument::Save` avec `NoMetadataUpdate` (`:102`), pour que PoDoFo
  garde ce XMP et n'y écrive pas l'heure courante. Pas de conversion PDF/A.
  Mêmes contrôles hors banc que qpdf, mêmes résultats.

### poppler-cpp 26.11.0 (plancher d'extraction)

Façade C++ de la librairie de rendu Poppler. Extraction seulement
(`libs/poppler-cpp/main.cpp:22-38`) : `document::load_from_raw_data`,
`embedded_files()`, premier fichier sous un nom de facture standard,
`embedded_file::data()`. 104 PDF sur 104. poppler-cpp n'a pas d'API
d'écriture : Embed est `unsupported`. C'est le plus lent des extracteurs
(0,4 à 0,7 ms contre 15 à 45 µs) : l'ouverture d'un document Poppler prépare
le rendu.

### Xerces-C++ 3.3.0 (plancher XML validant)

L'analyseur XML validant d'Apache, sans connaissance de Factur-X. Les deux
analyseurs vivent tout le processus et sont réutilisés, comme dans un service
de longue durée.

- Parse (`libs/xerces-c/main.cpp:90-110`) : `XercesDOMParser` avec espaces de
  noms, sans validation (`Val_Never`), sur un `MemBufInputSource` ; le DOM est
  adopté puis libéré à chaque appel.
- ParseValidate (`:112-136` et `:164-175`) : second analyseur avec validation XML Schema
  (`setDoSchema`, `Val_Always`) contre le XSD UN/CEFACT CII D16B
  (`CrossIndustryInvoice_100pD16B.xsd` et ses 3 schémas importés) de la
  configuration KoSIT XRechnung 3.0.2 du 2026-08-31, chargé une fois par
  `loadGrammar` et mis en cache (`useCachedGrammarInParse`, `:133`) ; erreurs
  comptées par un `ErrorHandler`. Le schéma seulement : aucune règle métier.
  Les 26 exemples valides passent ; un élément inconnu (`ram:Bogus`) ajouté
  à la main donne 2 erreurs de schéma.
- Write (`:138-162`) : `DOMLSSerializer` du DOM vers un
  `MemBufFormatTarget` en UTF-8.
- Réseau : aucun (`setLoadExternalDTD(false)`, schémas locaux).

### pugixml 1.16 (plancher XML)

Le DOM en mémoire le plus rapide du monde C++, sans validation ni
connaissance de Factur-X. Parse : `xml_document::load_buffer` avec
`parse_default` (`libs/pugixml/main.cpp:18`). Write : `xml_document::save`
avec `format_default` dans une chaîne (`:33`). ParseValidate est
`unsupported`.

## Librairies écartées

| Projet | Étoiles, activité, licence | Raison |
|---|---|---|
| LASTRADA-Software/XRechnung | ★3, dernier commit 2024-10-16, MIT | Modèle XRechnung en C++20 qui n'écrit que de l'UBL (`XRechnungSerializer.cpp:2101`, `ubl:Invoice` ; aucun élément `ram:`) ; pas de lecture ; `XRechnungValidator.hpp` n'est qu'une interface abstraite. Sans activité depuis deux ans, aucune opération CII du protocole |
| dragotin/kraft (KDE Kraft) | ★80, v2.1.0 du 2026-10-22, GPL-2.0 | Application de devis et factures (Qt, KDE) : `src/exportxrechnung.cpp` remplit un gabarit UBL XRechnung (`reports/xrechnung.xrtmpl`). Pas une librairie, pas de CII |
| classix/zugferd-cpp | ★0, 2025-01-31, pas de licence | Projet de formation (IHK) : outil en ligne de commande qui affiche le XML embarqué avec une copie de PoDoFo dans le dépôt. Pas de licence, pas une librairie ; l'opération est mesurée avec PoDoFo |
| miyako/4d-plugin-factur-x, -PoDoFo, -QPDF | ★2, ★0, ★0, 2026-11-10, MIT | Plugins pour la base 4D (`4DPluginAPI`, `C_BLOB`), qui assemblent un Factur-X avec PoDoFo ou qpdf. Pas utilisables hors de 4D ; les opérations PDF sont mesurées directement avec PoDoFo et qpdf |
| Neox365/Duncans-Suite (`docs/FacturX.h`) | ★2, 2026-11-21, pas de licence | Module d'une application Win32 qui écrit un CII EN 16931 depuis sa propre structure de facture et l'insère dans le PDF qu'elle génère ; son en-tête précise qu'il n'est pas validé par le validateur KoSIT. Pas de licence, pas une librairie |
| jw23578/YACAppServer (`xrechnunggenerator.cpp`) | ★0, 2025-11-19, pas de licence | Générateur XRechnung CII (pugixml) interne à un serveur d'applications. Pas de licence, pas une librairie |
| ULTRA-OS-Project/UltraCanvas (UltraFIBU) | ★4, 2026-11-22, MIT | Rapport PDF de facture d'une application de démonstration ; son en-tête dit explicitement ne pas faire de ZUGFeRD |
| ipswitch8/PDFE-public | ★0, 2026-11-20, AGPL-3.0 | Éditeur PDF pour Windows (Qt6) ; ZUGFeRD n'apparaît que dans ses tests de fonctionnalités PDF 2.0. Application, pas une librairie |
| aspose-pdf-foss/Aspose.PDF-FOSS-for-Cpp | ★2, 2026-10-22, MIT | Librairie PDF généraliste récente (C++20) ; `ZUGFeRD` n'est qu'une valeur de l'énumération `PdfFormat` (`include/aspose/pdf/pdf_format.hpp:33`) reprise d'Aspose.PDF, sans code Factur-X dans le dépôt. Ni formule Homebrew ni port : un plancher de plus |
| ksef_fa3 | ★0, 2026-04-08 | Facture électronique polonaise KSeF FA(3), pas EN 16931 |
| FriNagy/flex2pdf | ★0, 2026-02-26, pas de licence | Outil Windows en ligne de commande au-dessus du SDK commercial VPE |
| PDFium | Google, BSD-3-Clause | API de pièces jointes (`FPDFDoc_GetAttachment`, `FPDFDoc_AddAttachment`), mais ni formule Homebrew ni version stable empaquetée (l'option PDFium de Kura est aussi désactivée ici). Les planchers PDF sont couverts par qpdf, PoDoFo et poppler-cpp |
| MuPDF | ★3 066, AGPL-3.0 | API C : mesurée dans le banc C (`../c/COVERAGE.md`) |
| SDK commerciaux (Datalogics APDFL, PDFlib, Apryse, PDF-XChange, VPE) | propriétaires | Exemples ZUGFeRD et Factur-X publiés par les éditeurs (`ConvertToZUGFeRD.cpp`, `ConvertToFactur-X.cpp` chez Datalogics), non vérifiés ; licences payantes, hors protocole |

## Relevés d'exactitude (`./run.sh survey`, essai du 2026-11-22)

| Librairie | validate (signalés / 26) | parse (/ 26) | extract (/ 104) |
|---|---|---|---|
| kura | — | — | 104 |
| qpdf | — | — | 104 |
| podofo | — | — | 104 |
| poppler-cpp | — | — | 104 |
| xerces-c | 0 (XSD seulement) | 26 | — |
| pugixml | — | 26 | — |
| saxonc-he | 6 : BR-CL-25 ×2, CII-SR-465 ×4 | — | — |

Référence (PROTOCOL) : 6 sur 26. SaxonC-HE la retrouve exactement, avec les
mêmes règles que le banc Python. Le XSD seul ne voit aucune des six : ce
sont des règles Schematron.

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
| kura | o o o o | - - - | - - - | - - | o o (conversion PDF/A-3b) | o o |
| qpdf | o o o o | - - - | - - - | - - | o o (plancher) | - - |
| podofo | o o o o | - - - | - - - | - - | o o (plancher) | - - |
| poppler-cpp | o o o o | - - - | - - - | - - | - - | - - |
| xerces-c | - - - - | o o o | o o o (XSD) | o o | - - | - - |
| pugixml | - - - - | o o o | - - - | o o | - - | - - |
| saxonc-he | - - - - | - - - | o o o | - - | - - | - - |

## Tableau de couverture

| Librairie | Version | Licence | Dernière version | Téléchargements | Lecture CII | Écriture CII | UBL | Extraction PDF | Injection PDF | XSD | EN 16931 (CII) | Profils Factur-X (MINIMUM, BASIC WL, EXTENDED) | BR-FR | BR-DE (XRechnung) | Peppol BIS 3 | Contrôle du conteneur PDF/A-3 | Dépendances externes |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| Kura (bentopdf-kura) | 1.4.1 | AGPL-3.0 ou commerciale | 1.4.1 (2026-11-08) | ★25 ; pas de paquet | non (profil détecté par motifs) | non | non | oui : `readInvoice`, 104/104 | oui : `kura_convert` niveau 3b, conversion complète en PDF/A-3b (garde le `factur-x.xml` laissé par l'hôte dans `/AF`) | non | non | profil déduit de l'URN pour le XMP ; avertissement MINIMUM / BASIC WL | non | non | non | partiel : cohérence charge utile, nom, `/AFRelationship`, `/AF`, XMP (`checkInvoice`) ; pas une validation PDF/A | qpdf, freetype, jpeg-turbo, openjpeg, little-cms2, zlib |
| SaxonC-HE (référence) | 13.1.1 | MPL-2.0 (artefacts CEN : EUPL-1.2 ; KoSIT : Apache-2.0) | 13.1.1 (2026-10-26) | binaire Saxonica, pas de paquet | non | non | non (XSLT UBL non chargés) | non | non | non | oui : XSLT officiels CEN 1.3.17, 810 règles | non | non | oui : schematron KoSIT 2.6.0 (XRechnung 3.0.2) si BT-24 XRechnung | non (règles PEPPOL-EN16931-R de la feuille KoSIT seulement) | non | aucune (GraalVM natif embarqué) ; macOS arm64 épinglé |
| qpdf (plancher) | 12.4.2 | Apache-2.0 | 12.4.2 (2026-09-26) | ★5 606 ; Homebrew 85 709 / an | non | non | non | plancher, 104/104 | plancher écrit à la main | non | non | non | non | non | non | non | jpeg-turbo, openssl |
| PoDoFo (plancher) | 1.1.3 | LGPL-2.0-or-later ou MPL-2.0 | 1.1.3 (2026-11-16) | ★616 ; Homebrew 918 / an | non | non | non | plancher, 104/104 | plancher écrit à la main | non | non | non | non | non | non | non | freetype, fontconfig, jpeg-turbo, libpng, libtiff, openssl |
| poppler-cpp (plancher) | 26.11.0 | GPL-2.0-only ou GPL-3.0-only | 26.11.0 (novembre 2026) | dépôt freedesktop.org (pas sur GitHub) ; Homebrew 800 191 / an | non | non | non | plancher, 104/104 | non (pas d'API d'écriture) | non | non | non | non | non | non | non | cairo, glib, freetype, fontconfig, nss, gpgme… |
| Xerces-C++ (plancher) | 3.3.0 | Apache-2.0 | 3.3.0 (2024-10-14) | ★161 ; Homebrew 40 081 / an | DOM seulement | sérialisation du DOM | non | non | non | oui : CII D16B (configuration KoSIT 2026-08-31) | non | non | non | non | non | non | aucune |
| pugixml (plancher) | 1.16 | MIT | 1.16 (2026-06-15) | ★4 674 ; Homebrew 34 961 / an | DOM seulement | sérialisation du DOM | non | non | non | non | non | non | non | non | non | non | aucune |

## Notes de construction et de mesure

- `./run.sh setup` télécharge les mêmes trois archives officielles que le banc
  C, vérifiées par SHA-256 (configuration KoSIT XRechnung 3.0.2 du
  2026-08-31, CEN `validation-1.3.17` CII, KoSIT `xrechnung-schematron`
  2.6.0), plus SaxonC-HE 13.1.1 pour macOS arm64 (ailleurs, `saxonc-he` est
  sauté) et l'archive du tag Kura `v1.4.1`, dont seul `pdfa-engine/` est
  extrait. Puis il construit chaque cible à part ; une librairie absente est
  sautée à la configuration.
- SaxonC-HE est importé à la main dans `CMakeLists.txt` (bibliothèques et
  en-têtes de l'archive), sans les fichiers de paquet CMake qu'elle contient :
  la configuration n'exécute aucun de ses scripts.
- Kura est construit par `add_subdirectory` de sa source épinglée : la
  configuration CMake exécute donc son `CMakeLists.txt`. Les dépendances
  (qpdf, freetype, jpeg-turbo, openjpeg, little-cms2, zlib) sont vérifiées
  avant, car le `CMakeLists.txt` de Kura arrête toute la configuration s'il en
  manque une ; ses parties optionnelles (CLI, PDFium, zxing-cpp, QuickJS,
  fuzzers, test du SDK) sont désactivées.
- Le harnais est celui du banc C (`../c/harness/harness.c`), avec le même
  protocole : appel de contrôle hors mesure, N par paliers 1, 2, 5, 10…,
  `COUNT` séries, un seul fil (pile de 512 Mio), libération dans la boucle
  mesurée, puits `volatile`. Horloge `CLOCK_UPTIME_RAW` sur macOS (celle de
  `Instant` en Rust ; `CLOCK_MONOTONIC` n'y avance que par microsecondes).
  Une exception C++ devient un `fail` avec son message ; un signal fatal
  (SIGSEGV, SIGABRT…) écrit la ligne `fail` préparée avant l'appel.
- Temps CPU et pic mémoire, par le même harnais : `getrusage(RUSAGE_SELF)`,
  temps utilisateur + système de tous les fils, lu avant et après chaque
  série (`cpu_ns_per_op`) ; `ru_maxrss` (octets sur macOS) lu à la fin de la
  série dans la passe `resources`, où `run.sh` lance directement
  `build/bench-<lib> resources <Bench>`, un processus par benchmark. Aucune
  librairie ne lance de processus enfant. SaxonC-HE garde son isolat GraalVM
  dans le processus, avec ses propres fils (cinq fils au total pendant la
  validation) : leur travail et le tas GraalVM sont comptés ; le temps CPU
  reste à environ 100 % du temps réel. Kura ne lance des fils que pour
  convertir les couleurs d'images d'au moins 2 Mpx (`colorx.cpp`,
  `colorconv.cpp`), ce que les PDF hôtes ne demandent pas.
- Le contrôle de l'injection relit le PDF produit avec l'extraction de la
  même librairie et exige un XML CII, comme le harnais Rust. Avec
  `FX_DUMP_EMBED=<dossier>`, le PDF de l'appel de contrôle est conservé ; c'est
  ainsi que les PDF de qpdf, PoDoFo et Kura ont été passés à `zerechnung
  validate`, à `checkInvoice` de Kura et à `qpdf --check`.
- Les planchers d'injection (qpdf, PoDoFo, et MuPDF dans le banc C) écrivent
  `/AFRelationship /Alternative`, ou `/Data` pour MINIMUM et BASIC WL
  (Factur-X 1.0.07, § 6.2.2), retirent de `/AF` tout fichier de facture
  laissé par l'hôte et remplacent son XMP par un paquet Factur-X écrit par le
  harnais. Aucun ne convertit en PDF/A (pas d'output intent) ; Kura est le
  seul à le faire, d'où l'écart d'un facteur 25 à 100 sur Embed.
- SaxonC-HE démarre un isolat GraalVM : le premier appel (environ 140 ms)
  comprend ce démarrage et la compilation des deux feuilles. PoDoFo a montré
  une fois un premier appel de 141 ms (initialisation unique), non reproduit
  ensuite.
