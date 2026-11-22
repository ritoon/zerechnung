# Couverture : écosystème Java

Banc Java du protocole commun (`../PROTOCOL.md`). Un module Maven par
librairie (`libs/<lib>`), un harnais partagé (`harness`), versions épinglées
dans chaque `pom.xml`, construction par `mvn -C` (sommes de contrôle
strictes). JDK : OpenJDK 27.0.1.1 (Homebrew), cible `release 21` ; Maven
3.10.1. Chaque librairie tourne dans sa propre JVM, avec les options par
défaut.

Les preuves citées sont des chemins dans les jars de `~/.m2/repository/…` et
leurs jars de sources (`-sources.jar`), ou dans les artefacts officiels que
`./run.sh setup` télécharge dans `.artefacts/`. Popularité : étoiles GitHub
(API GitHub, relevé du 2026-11-22), dernière version et date (Maven Central,
`maven-metadata.xml`), dépendants de la version mesurée (deps.dev). Maven
Central ne publie pas de nombre de téléchargements : la colonne
« Téléchargements » du tableau donne à la place les étoiles et les dépendants.

## Librairies mesurées

### Mustang 2.26.0 (ZUGFeRD/mustangproject)

`org.mustangproject:library` et `org.mustangproject:validator`, la librairie
de référence de la communauté ZUGFeRD. Modèle de facture, lecture CII et UBL,
écriture CII de tous les profils, injection dans un PDF/A, validateur (XSD,
schematron, veraPDF).

- Validateur : `XMLValidator.java` choisit les règles d'après BT-24.
  - Profils Factur-X : XSD du profil (`schema/ZF_250/<profil>/FACTUR-X_*.xsd`,
    `XMLValidator.java:256-277`) et schematron Factur-X 1.09.2 du profil
    (`xslt/ZF_250/FACTUR-X_*.xslt`, 433 assertions pour EN16931). Les règles
    CEN y sont recodées sous des identifiants `FX-SCH-*`. BR-CL-25 y devient
    FX-SCH-A-000571 (liste de codes du `@schemeID`, `FACTUR-X_EN16931.xslt:6420`).
  - XRechnung : XSD Factur-X EN16931, puis CEN CII 1.3.16
    (`xslt/en16931schematron/EN16931-CII-validation.xslt`, 806 assertions,
    `XMLValidator.java:401-404`), puis KoSIT 2.4.0 pour XRechnung 3.0.2
    (`xslt/XR_30/XRechnung-CII-validation.xslt`, 70 assertions, `:574`), tous
    en erreur. Sur les profils BASIC et EN16931, le schematron XRechnung tourne
    aussi, mais ses constats restent des `notice`.
  - BR-FR Flux 2 V1.3.0
    (`xslt/XP_Z12_012/20260216_BR-FR-Flux2-Schematron-CII_V1.3.0.xsl`)
    seulement si le vendeur et l'acheteur sont en France (`:233`, `:393`). Ce
    n'est jamais le cas dans ce corpus.
  - Contrôle arithmétique des totaux (`:412`).
  - CII-SR-465 garde le drapeau `warning` de CEN. Mustang signale donc 2
    fichiers sur 26 (FX-SCH-A-000571 sur les deux fichiers de BR-CL-25) et ne
    signale pas les 4 XRECHNUNG.
- CheckPDF native : `ZUGFeRDValidator` sur un PDF lance veraPDF
  (`PDFValidator.java:29-34`), contrôle le XMP et la pièce jointe, puis valide
  le XML extrait.
- Lecture : `ZUGFeRDImporter` analyse aussi le XML en modèle dès l'import
  (`ZUGFeRDInvoiceImporter.java:292-313`, `355-368`). `Extract_*` comprend donc
  cette analyse, ce que dit la note. `extractInvoice` refuse 3 fichiers sur 26
  dont il ne retrouve pas le total à payer (ArithmeticException :
  `EN16931_1_Teilrechnung`, `EN16931_Kraftfahrversicherung_Bruttopreise`,
  `EN16931_Sachversicherung_berechneter_Steuersatz`).
- Écriture : `ZUGFeRD2PullProvider`, profil EN16931. UBL : lecture seulement,
  l'écriture UBL ne sert qu'à l'avis d'expédition (`UBLDAPullProvider`).
- Injection : `ZUGFeRDExporterFromPDFA` accepte un hôte PDF/A-1 ou PDF/A-3 et
  refuse les autres (`ZUGFeRDExporterFromPDFA.java:59-75`). Le fichier est
  enregistré sans compression (`ZUGFeRDExporterFromA3.java:361`). Le PDF
  produit est PDF/A-3U et passe le validateur Mustang. Il garde pourtant dans
  `/AF` la spécification `factur-x.xml` résiduelle de l'hôte à côté de la
  nouvelle (voir les notes) : Paniolo le refuse (FX-MULTIPLE-INVOICES).
- Classpath de 78 jars : Saxon-HE 12.10, PDFBox 3.0.8 et preflight, veraPDF
  validation-model-jakarta 1.30.2 (GPL-3.0 ou MPL-2.0), ph-schematron 8.0.6,
  jaxb-runtime 4.0.2.
- Réseau : aucun.

### phive 12.3.0 et phive-rules 4.7.1 (phax)

`com.helger.phive:phive-xml` et les paquets de règles
`com.helger.phive.rules:phive-rules-{en16931,xrechnung,zugferd}`. Moteur de
validation générique, avec les XSD et les XSLT officiels précompilés, exécutés
par ph-schematron 10.3.2 sur Saxon-HE 12.10. C'est un validateur seul.

- Jeux enregistrés par `EN16931Validation.initEN16931`,
  `XRechnungValidation.initXRechnung` et `ZugferdValidation.initZugferd`.
  L'adaptateur choisit le jeu d'après BT-24.
- EN 16931 : `VID_CII_1317` (`EN16931Validation.java:122`), soit le XSD CII
  D16B (`:598`) et `external/schematron/1.3.17/cii/EN16931-CII-validation.xslt`
  (810 assertions).
- XRechnung : `VID_XRECHNUNG_CII_302`, soit XSD D16B, CEN CII 1.3.16 et
  `external/schematron/3.0.2/XRechnung-CII-validation.xslt` (KoSIT 2.6.0, 74
  assertions). phive applique ses propres niveaux
  (`XRechnungValidation.java:1042-1063` : BR-CL-23 et BR-CL-21 en
  avertissement, CII-SR-452/453/454 en erreur), pas ceux de la configuration
  KoSIT. CII-SR-465 reste donc un avertissement, et les 4 XRECHNUNG passent.
- Factur-X : XSD et schematrons ZUGFeRD 2.5.2 / Factur-X 1.09.2 des cinq
  profils (`external/schemas/2.5.2/…`, `external/schematron/2.5.2/…`,
  `ZugferdValidation.java:211-223`). Aucun fichier du corpus ne les utilise.
- Paquets non chargés : `phive-rules-peppol` et `phive-rules-france` 4.7.1
  (règles françaises, exclues par le protocole), entre autres.
- ph-schematron produit son SVRL par JAXB. Le module ajoute donc
  `jaxb-runtime` 4.0.9 : sans lui, tous les fichiers échouent.
- Réseau : aucun.

### Validateur KoSIT 1.6.3 et validator-configuration-xrechnung v2026-08-31

`org.kosit:validator`, le moteur générique de la KoSIT, piloté par une
configuration de scénarios. C'est la référence de la validation XRechnung en
Allemagne.

- Configuration officielle téléchargée par `./run.sh setup` (SHA-256 vérifié),
  fichier `scenarios.xml` :
  - Scénario XRechnung CII (`:335-400`, sélection par BT-24 à `:357`) : XSD CII
    D16B (`resources/cii/16b/xsd`), CEN EN16931-CII 1.3.16 (806 assertions),
    puis KoSIT XRechnung 2.6.0 (`resources/xrechnung/3.0.2/xsl`, 74
    assertions : 25 BR-DE, 7 BR-DEX, 5 BR-DE-CVD, 5 BR-TMP, 20
    PEPPOL-EN16931).
  - Niveaux personnalisés du scénario (`:384-400`) : CII-SR-465 et CII-SR-466
    passent en erreur, CII-SR-475 et CII-SR-476 en information.
  - Le scénario « EN16931 (CII) » (`:652`, sélection à `:672`) ne prend que
    BT-24 = `urn:cen.eu:en16931:2017` et applique XSD et CEN. Aucun scénario ne
    reconnaît un profil Factur-X (MINIMUM, BASIC…) : pas de règles Factur-X.
- `DefaultCheck` applique les niveaux personnalisés (`DefaultCheck.java:145-162`).
  L'adaptateur compte les violations XSD qui ne sont pas des avertissements et
  les assertions de niveau effectif error ou fatal : 6 fichiers sur 26, la
  référence.
- `ProcessorProvider` interdit la résolution de ressources externes.
  `jaxb-runtime` 4.0.8 doit être fourni : c'est une dépendance optionnelle du
  POM KoSIT.
- La version 2.0.0-tp2 existe, mais c'est une préversion technique : on mesure
  la 1.6.3.
- Réseau : aucun à l'exécution.

### Saxon-HE 13.1 et XSLT officiels (plancher)

Le plancher « artefacts officiels », écrit à la main dans
`libs/saxon-schematron/src/main/java/bench/Main.java`. C'est le portage de
l'adaptateur saxonche du banc Python.

- Saxon-HE (s9api) compile `cen-1.3.17/xslt/EN16931-CII-validation.xslt` et,
  si BT-24 désigne XRechnung,
  `xrechnung-2.6.0/schematron/cii/XRechnung-CII-validation.xsl`.
- L'adaptateur compte les `svrl:failed-assert` et `svrl:successful-report` de
  drapeau fatal ou error, après les niveaux personnalisés de KoSIT v2026-08-31,
  recopiés de `scenarios.xml:384-400`.
- Pas de XSD. Résultat : 6 fichiers sur 26.

### Paniolo 1.0.0 (NaluKit/paniolo)

`io.github.nalukit:paniolo`, `paniolo-facturx` et `paniolo-pdfa`, publiés le
2026-11-18. Modèle EN 16931, lecture et écriture CII et UBL, validation par les
artefacts officiels embarqués (Saxon-HE 13.1), PDF par `paniolo-facturx`
(PDFBox 3.0.8), PDF/A par `paniolo-pdfa` (veraPDF validation-model 1.30.4,
sous MPL-2.0 d'après le POM).

- Artefacts embarqués dans `paniolo-rules-1.0.0.jar`
  (`io/github/nalukit/paniolo/rules/…`) :
  - `xrechnung-3.0.2-validator-configuration-2026-08-31.zip`, octet pour octet
    le zip officiel (même SHA-256 que dans `run.sh`), et les anciennes
    configurations de 1.2.2 à 2.3.1 ;
  - `factur-x-1.09.2-zugferd-2.5.2.zip` ;
  - `fnfe-france-rfe-1.4.0.04-cii-extended-ctc-fr.zip` ;
  - Order-X et ZUGFeRD 1.0.
- Validation (`Paniolo.validate`, `Paniolo.java:1710`), choix d'après BT-24
  (`ValidationProfile.java:573-601`) :
  - XRechnung : le scénario KoSIT avec ses niveaux, comme le validateur
    KoSIT : 6 fichiers sur 26.
  - EN 16931 simple : la même configuration KoSIT. Les constats propres à la
    CIUS (BR-DE, BR-TMP, PEPPOL-EN16931) sont ramenés à l'information
    (`Paniolo.java:531-560`, `:1590-1602`).
  - Profils Factur-X : XSD et schematrons Factur-X 1.09.2.
  - EXTENDED-CTC-FR : règles FNFE-MPE 1.4.0.04 (EXTENDED-CTC-FR et BR-FR
    Flux 2), seulement pour ce profil (`Paniolo.java:3706`). Ce profil
    n'apparaît jamais dans le corpus.
  - Toujours : un contrôle des montants (`CalculationCheck`).
- Lecture : `read` en mode STRICT (`Paniolo.java:1209`) refuse 2 fichiers sur
  26, dont le montant net d'une ligne s'écarte de quantité × prix
  (`EN16931_Kraftfahrversicherung_Bruttopreise`,
  `EN16931_Sachversicherung_berechneter_Steuersatz`).
- CheckPDF native : `validate` sur un PDF contrôle la pièce jointe, la relation
  et le XMP, le PDF/A par veraPDF, puis le XML pour le profil déclaré.
- Injection : `embedXml` (`Paniolo.java:2078-2130`) vérifie BT-24, intègre le
  XML, puis revalide le PDF produit (cette revalidation est comprise dans la
  mesure). Pas de conversion PDF/A.
  - Elle refuse un PDF qui contient déjà une facture
    (`PdfBoxFacturXSupport.java:1190-1192`) et, pour le savoir, compte aussi
    les fichiers de `/AF` (`:248-291`).
  - Les deux hôtes du protocole gardent une spécification `factur-x.xml` dans
    `/AF` : d'où le `fail` sur `Embed_Small` et `EmbedClassic_Small`.
  - Le remplacement (`replaceInvoice`) n'existe que dans la SPI.
- Réseau : aucun.

### ph-cii 10.1.0 (phax)

`com.helger.cii:ph-cii-d16b` : classes JAXB générées depuis le XSD CII D16B
(`external/schemas/d16b/…/CrossIndustryInvoice_100pD16B.xsd`,
`CCIID16B.java:36,63`) et le marshaller
`CIID16BCrossIndustryInvoiceTypeMarshaller` (`:39-40`). C'est un modèle
syntaxique, pas sémantique.

- Parse et Write : le marshaller sans schéma (`setUseSchema(false)`).
- Validation : le même marshaller avec son XSD, activé par défaut
  (`GenericJAXBMarshaller.java:93`), et `setCollectErrors`. XSD seulement,
  aucune règle métier : 0 fichier signalé sur 26.
- `jaxb-runtime` 4.0.6 doit être fourni (version du BOM de ph-commons-parent
  12.1.0).
- Pas de PDF. UBL : projet séparé (ph-ubl), non mesuré.

### invoice4j 0.5.0 (jcodeforge)

`io.github.jcodeforge:invoice4j-zugferd` : modèle, lecteur et écrivain ZUGFeRD
/ Factur-X, lecture et écriture de PDF.

- XSD Factur-X 1.09 du profil (`/xsd/cii/en16931/Factur-X_1.09_EN16931.xsd`,
  `CiiEn16931XsdValidator.java:20` ; `/xsd/{minimum,basic,basicwl,extended}/…`).
- Règles : `ZugferdBasicValidator`, 3 règles (BR-CO-15, BR-CO-26, BR-S-2),
  avec arrêt à la première erreur. L'XSLT CEN (`xsl/EN16931-CII-validation.xsl`
  dans invoice4j-base) et les schematrons Factur-X (`schematron/FACTUR-X_*.sch`)
  sont dans les jars, mais aucun code ne les référence.
- Le lecteur `SellerParser` (invoice4j-base,
  `cii/parser/SellerParser.java:16-31`) ne lit ni l'identifiant, ni le numéro
  de TVA, ni l'immatriculation du vendeur. Conséquences :
  - BR-CO-26 (`BrCo26Validator.java:34-47`) échoue sur les 11 fichiers lus ;
  - la sortie de Write viole BR-S-02 et BR-CO-26 au schematron CEN.
- Le lecteur refuse 15 fichiers sur 26, même sans XSD :
  - types de document 204, 387 et 575 inconnus (`DocumentTypeCode.java:118`) ;
  - schéma d'identifiant 9958 ;
  - des règles maison : BT-70 exigé, montants BT-115 et BT-116 négatifs
    refusés, compte bancaire exigé pour un prélèvement SEPA ;
  - les 4 XRECHNUNG (« Unsupported ZUGFeRD profile », `ZugferdProfileDetector`).
- PDF : il n'y a que `ZugferdPdfReader.read(File)` et
  `ZugferdPdfWriter.write(File, File)`, pas d'API sur des octets en mémoire
  (règle 4 du protocole). Extract et Embed sont donc `unsupported`.
- UBL : lecteur et écrivain dans `invoice4jbase/ubl`, non mesurés.

### Imixs e-invoice 1.0.3 (imixs/e-invoice)

`org.imixs.util:imixs-e-invoice` : enveloppe de lecture et de modification sur
le DOM d'une facture CII, UBL ou KSeF (Pologne), écrite pour le moteur de
workflow Imixs.

- `EInvoiceModelFactory.read` détecte le format et analyse le DOM, puis
  `EInvoiceModelCII.parseContent` (`:81`) lit l'en-tête, les parties et les
  lignes.
- `getContent` resérialise le DOM (indenté) : Write est une resérialisation,
  pas une génération.
- Pas de validation, pas de PDF.
- Licence : le POM déclare la GPL (URL gpl-2.0, « All the results of the imixs
  workflow project are subject to the GPL ») ; le dépôt GitHub, la MIT.

### gallop 3.0.0 (codebarista-de/gallop)

`de.codebarista:gallop` : générateur de CII XRechnung et Factur-X EN 16931
depuis un modèle fluide, sur le DOM du JDK. Pas de lecteur, pas de validation,
pas de PDF.

- `Write_Small` utilise un modèle construit à la main d'après `EN16931_Einfach`
  (`einfach()` dans `libs/gallop/src/main/java/bench/Main.java`).
  - gallop écrit toujours une partie livrée BG-13
    (`CIIXMLEInvoiceWriter.java:261-270`) et un moyen de paiement BG-16
    (`:297`, `:392-395`). Le modèle y met l'adresse de l'acheteur et le code 1,
    sans quoi BR-57 et BR-CL-16 échouent.
  - Pas de champ pour les GTIN.
  - La sortie passe le schematron CEN 1.3.17.
- `Write_Lines` est sauté.
- Formats : `EInvoiceFormat.XRECHNUNG` et `EN16931_CORE`, dont `ZUGFERD` et
  `FACTURX` sont des alias.

### PDFBox 3.0.8 (plancher)

Librairie PDF généraliste d'Apache, celle de Mustang, de Paniolo et
d'invoice4j. Elle sert de plancher d'extraction, écrit à la main dans
`libs/pdfbox/src/main/java/bench/Main.java` :

- `Loader.loadPDF`, puis parcours de l'arbre `/EmbeddedFiles` avec `/Kids` ;
- premier fichier sous un nom standard, casse ignorée (`factur-x.xml`,
  `zugferd-invoice.xml`, `xrechnung.xml`, `order-x.xml`, `cii.xml`), comme le
  plancher lopdf du banc Rust ;
- flux `/UF`, sinon `/F`.

## Librairies écartées

| Librairie | Version | Raison |
|---|---|---|
| Konik | 1.3.2 | Archivée, ZUGFeRD 1 seulement, dernier commit en 2022 (★60) |
| iText pdfInvoice (`com.itextpdf:zugferd`) | 2.0.1 (2018-08-29) | Archivée, ZUGFeRD 1.0, AGPL (★6) |
| SRSoftware document.zugferd | 1.0.5 | Greffon du moteur de documents SRSoftware (`DocumentFactory`, `render(Map)`) : génère depuis un gabarit et injecte par une copie de l'`A3Exporter` de Mustang ; ni lecture ni validation |
| codinux e-invoice-jvm | 0.8.1 | Surcouche Kotlin de Mustang 2.15.1 (Mustang est mesuré directement) ; licence propriétaire « codinux GmbH & Co. KG » |
| Yona einvoice-java (`com.useyona`) | 0.2.3 | Client de l'API distante Yona (exclu par le protocole) |
| en16931-basics (phax) | 1.1.0 | Listes de codes et utilitaires partagés, pas d'entrée-sortie de facture (★1) |
| en16931-cii2ubl (phax) | — | Convertisseur CII → UBL ; sa lecture CII est celle de ph-cii, déjà mesuré (★48) |
| OpenXRechnungToolbox | — | Application graphique sous GPL-3.0, pas une librairie (★172) |
| veraPDF | 1.30.x | Validateur PDF/A généraliste, mesuré à travers Mustang et Paniolo (★352) |

Toutes les librairies retenues sont en Java pur, sans dépendance native, et
aucune n'appelle le réseau à l'exécution.

## Relevés d'exactitude (`./run.sh survey`)

Relevé fait avec un `OUT` temporaire, hors de `results/`.

| Librairie | validate (signalés / 26) | parse (/ 26) | extract (/ 104) |
|---|---|---|---|
| mustang | 2 (FX-SCH-A-000571 ×2, BR-CL-25 recodé ; CII-SR-465 reste un avertissement) | 23 (3 ArithmeticException sur le total à payer) | 104 |
| phive | 2 (BR-CL-25 ×2 ; niveaux propres à phive, CII-SR-465 en avertissement) | — | — |
| kosit-validator | **6** (BR-CL-25 ×2, CII-SR-465 ×4 sur les XRECHNUNG) | — | — |
| saxon-schematron | **6** (idem) | — | — |
| paniolo | **6** (idem) | 24 (2 écarts de montant net de ligne, mode STRICT) | 104 |
| ph-cii | 0 (XSD seulement) | 26 | — |
| invoice4j | 26 (11 faux positifs BR-CO-26, 15 refus de lecture) | 11 | — |
| imixs-e-invoice | — | 26 | — |
| gallop | — | — | — |
| pdfbox | — | — | 104 |

Référence (PROTOCOL) : 6 sur 26.

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
| mustang | o o o o | o o o | o o o | o o | o o | o o |
| phive | - - - - | - - - | o o o | - - | - - | - - |
| kosit-validator | - - - - | - - - | o o o | - - | - - | - - |
| saxon-schematron | - - - - | - - - | o o o | - - | - - | - - |
| paniolo | o o o o | o o o | o o o | o o | F F | o o |
| ph-cii | - - - - | o o o | o o o | o o | - - | - - |
| invoice4j | - - - - | o o F | o o F | o o | - - | - - |
| imixs-e-invoice | - - - - | o o o | - - - | o o | - - | - - |
| gallop | - - - - | - - - | - - - | o (sauté) | - - | - - |
| pdfbox | o o o o | - - - | - - - | - - | - - | - - |

## Tableau de couverture

| Librairie | Version | Licence | Dernière version | Téléchargements | Lecture CII | Écriture CII | UBL | Extraction PDF | Injection PDF | XSD | EN 16931 (CII) | Profils Factur-X (MINIMUM, BASIC WL, EXTENDED) | BR-FR | BR-DE (XRechnung) | Peppol BIS 3 | Contrôle du conteneur PDF/A-3 | Dépendances externes |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| Mustang (library + validator) | 2.26.0 | Apache-2.0 (veraPDF : GPL-3.0 ou MPL-2.0) | 2.26.0 (2026-08-25) | ★497 ; dépendants : library 3, validator 1 | partiel (refuse 3/26 dont il ne retrouve pas le total) | oui (tous les profils, XRechnung) | lecture ; écriture : avis d'expédition seulement | oui (analyse aussi en modèle) | oui (hôte PDF/A-1 ou A-3 exigé, PDF/A-3U produit ; garde le `/AF` résiduel de l'hôte) | oui : Factur-X 1.09.2 par profil | oui : règles CEN recodées dans le schematron Factur-X EN16931 ; XSLT CEN CII 1.3.16 en plus pour XRechnung | oui : schematrons Factur-X 1.09.2 des 5 profils | oui : BR-FR Flux 2 V1.3.0, seulement si vendeur et acheteur sont français | oui : XRechnung 3.0.2, schematron KoSIT 2.4.0 (70 assertions) ; en notice sur BASIC et EN16931 | non | oui : veraPDF, XMP, pièce jointe | aucune (Java pur : PDFBox, Saxon-HE 12.10, veraPDF) |
| phive + phive-rules | 12.3.0 / 4.7.1 | Apache-2.0 (phive-rules : « Apache 2 » au POM, NOASSERTION sur GitHub ; artefacts CEN EUPL-1.2) | 12.3.0 (2026-11-15) / 4.7.1 (2026-11-17) | ★79 / ★55 ; dépendants : non indexé | non (validateur seul) | non | validation (jeux UBL EN 16931 et XRechnung) | non | non | oui : CII D16B ; Factur-X 1.09.2 par profil | oui : XSLT officiel CEN 1.3.17 (810 assertions) via ph-schematron et Saxon-HE | oui : ZUGFeRD 2.5.2 / Factur-X 1.09.2, 5 profils | non chargé (paquet séparé phive-rules-france) | oui : XRechnung 3.0.2, schematron KoSIT 2.6.0 (74 assertions), niveaux propres à phive (CII-SR-465 en avertissement) | non chargé (paquet séparé phive-rules-peppol) | non | aucune (Java pur ; jaxb-runtime à fournir) |
| Validateur KoSIT + configuration XRechnung | 1.6.3 / v2026-08-31 | Apache-2.0 (moteur et configuration) | 1.6.3 (2026-08-20) ; 2.0.0-tp2 en préversion | ★199 (configuration ★120) ; dépendants : 0 | non (validateur seul) | non | validation (scénarios UBL de la configuration) | non | non | oui : CII D16B (configuration) | oui : XSLT CEN CII 1.3.16 (806 assertions) de la configuration, via Saxon-HE 12.9 | non (aucun scénario Factur-X) | non | oui : schematron KoSIT 2.6.0 et niveaux personnalisés (25 BR-DE, 7 BR-DEX, 5 BR-DE-CVD) | non (20 règles PEPPOL-EN16931 reprises par XRechnung) | non | configuration officielle téléchargée par `setup` (SHA-256) ; jaxb-runtime à fournir |
| Saxon-HE + XSLT officiels (plancher) | 13.1 | MPL-2.0 (artefacts CEN EUPL-1.2, KoSIT Apache-2.0) | 13.1 (2026-10-15) | ★178 ; dépendants : 30 | non | non | non | non | non | non | XSLT officiel CEN CII 1.3.17 via Saxon | non | non | XSLT officiel KoSIT 2.6.0 via Saxon, niveaux KoSIT v2026-08-31 | non | non | artefacts officiels téléchargés par `setup` (SHA-256) |
| Paniolo (+ facturx, pdfa) | 1.0.0 | Apache-2.0 (veraPDF validation-model : MPL-2.0) | 1.0.0 (2026-11-18) | ★1 ; dépendants : non indexé | partiel (STRICT refuse 2/26 : écarts de montant de ligne) | oui | oui (lecture, écriture) | oui | partiel : pas de conversion PDF/A, revalide la sortie ; refuse un hôte qui porte déjà une facture (les deux hôtes du protocole) | oui : CII D16B (configuration KoSIT), Factur-X 1.09.2 par profil | oui : XSLT CEN CII 1.3.16 de la configuration KoSIT v2026-08-31 embarquée | oui : schematrons Factur-X 1.09.2 / ZUGFeRD 2.5.2 | oui pour le profil EXTENDED-CTC-FR seulement (FNFE-MPE 1.4.0.04, BR-FR Flux 2) | oui : configuration KoSIT XRechnung 3.0.2 v2026-08-31 avec ses niveaux ; constats ramenés à l'information hors XRechnung | non | oui : veraPDF (paniolo-pdfa), pièce jointe, relation, XMP | aucune (Java pur : Saxon-HE 13.1, PDFBox 3.0.8, veraPDF 1.30.4) |
| ph-cii (d16b) | 10.1.0 | Apache-2.0 | 10.1.0 (2026-11-14) | ★15 ; dépendants : 0 | oui (JAXB, modèle syntaxique D16B) | oui (JAXB) | non (projet ph-ubl séparé) | non | non | oui : CII D16B | non | non | non | non | non | non | aucune (jaxb-runtime à fournir) |
| invoice4j-zugferd | 0.5.0 | Apache-2.0 | 0.5.0 (2026-09-18) | ★4 ; dépendants : 0 | partiel (11/26 : types 204/387/575, schéma 9958, règles maison, XRechnung refusés) | partiel (identifiants du vendeur perdus à la lecture) | oui (invoice4j-base, non mesuré) | partiel (API sur fichiers seulement) | partiel (API sur fichiers seulement) | oui : Factur-X 1.09 par profil | partiel (3 règles : BR-CO-15, BR-CO-26, BR-S-2) ; XSLT CEN embarqué jamais exécuté | XSD seulement (schematrons Factur-X embarqués jamais exécutés) | non | non (profil XRechnung refusé) | non | non | aucune (PDFBox 3.0.8) |
| Imixs e-invoice | 1.0.3 | GPL au POM, MIT sur GitHub | 1.0.3 (2026-09-28) | ★4 ; dépendants : 0 | oui (enveloppe DOM, champs principaux) | partiel (resérialise le DOM lu ou modifié) | oui (lecture ; KSeF aussi) | non | non | non | non | non | non | non | non | non | aucune |
| gallop | 3.0.0 | MIT | 3.0.0 (2026-09-25) | ★0 ; dépendants : 0 | non | oui (EN 16931 et XRechnung depuis un modèle fluide ; écrit toujours BG-13 et BG-16) | non | non | non | non | non | non (EN 16931 seulement) | non | écriture XRechnung seulement | non | non | aucune (DOM du JDK) |
| PDFBox (plancher) | 3.0.8 | Apache-2.0 | 3.0.8 (2026-07-08) | ★3 143 ; dépendants : 392 (119 directs) | non | non | non | plancher écrit à la main | non | non | non | non | non | non | non | non | aucune |

## Notes de construction et de mesure

- Un module Maven par librairie : les classpaths restent séparés (Mustang tire
  Saxon-HE 12.10, KoSIT 12.9, Paniolo 13.1). `run.sh` lance une JVM par
  librairie, avec les options par défaut : ni `-Xss` ni `-Xmx`.
- Le harnais exécute les opérations dans un fil à pile de 512 Mio créé par le
  programme (`new Thread(…, 512L << 20)`), comme le banc Rust. Aucun validateur
  n'a demandé d'option de JVM.
- `mvn -B -C -q package` : `-C` fait échouer la construction sur une somme de
  contrôle invalide. La phase `package` écrit le classpath de chaque module
  dans `target/classpath.txt` (`maven-dependency-plugin:build-classpath`), que
  `run.sh` passe à `java -cp`. Seuls des plugins Apache Maven sont utilisés,
  aux versions épinglées dans le POM parent.
- JAXB : KoSIT, ph-cii et phive (par ph-schematron) demandent une
  implémentation de Jakarta XML Binding que leurs POM ne tirent pas.
  `org.glassfish.jaxb:jaxb-runtime` est donc ajouté en portée `runtime`, à la
  version que gère chaque projet : 4.0.8 pour KoSIT, 4.0.6 pour ph-cii, 4.0.9
  pour phive.
- Journaux : il n'y a pas de fournisseur SLF4J, d'où un avertissement NOP au
  démarrage. Dans le module Mustang, le JDK 27 avertit aussi qu'un champ final
  est modifié par réflexion (JAXB sur des classes veraPDF). Ni l'un ni l'autre
  ne change les résultats.
- `first_call_ns` comprend la compilation paresseuse des XSLT (Mustang, phive,
  KoSIT, Saxon, Paniolo) et le chargement des classes : de 0,6 à 2 s pour un
  validateur.
- Temps CPU (`cpu_ns_per_op`) : `OperatingSystemMXBean.getProcessCpuTime()`
  (`com.sun.management`), soit `getrusage` sur macOS, tous les fils de la JVM,
  lu juste avant et juste après chaque série. Aucune librairie ne crée de fil
  ni de processus. Un relevé `jcmd Thread.print` pendant la mesure de chaque
  librairie ne montre que le fil `bench`, `main` et ceux de la JVM : 2 à 4
  compilateurs C1/C2, jusqu'à 11 fils parallèles et 3 concurrents de G1, Java2D
  Disposer chez Mustang (injection). Le CPU dépasse donc le temps réel quand le
  JIT compile encore ou quand G1 collecte sur plusieurs fils. Dans la passe
  `bench`, PDFBox est à +1 ou 2 %. Dans une JVM neuve (passe `resources`), il
  passe à ×3,5 avec `BENCHTIME=0.2` et revient à +2 % avec `BENCHTIME=1` ; à
  1 s, Saxon et Mustang vont encore de ×1,1 à ×3,4 selon le benchmark.
- `./run.sh resources` : une JVM par librairie et par benchmark
  (`bench.Main resources <Bench>`), même ligne de commande et mêmes options que
  `bench`, lancée directement sous `/usr/bin/time -l` (pic de RSS en octets).
  Avec les options par défaut (G1, tas initial 8 Mio, tas maximal 6 Gio, le
  quart des 24 Gio de la machine), ce pic suit surtout le dimensionnement du
  tas par G1 sous forte allocation, pas les données vivantes de la librairie.
  Exemple : PDFBox, 2 Mio vivants dans un tas de 448 Mio, 300 à 570 Mio de RSS ;
  Mustang, jusqu'à 2 Gio.
- Les sorties du harnais sont formatées avec `Locale.ROOT`.
- Sorties de Write relues avec le plancher Saxon (CEN 1.3.17) sur
  `EN16931_Einfach` et `EN16931_Rabatte` : Mustang, Paniolo, ph-cii, Imixs et
  gallop ne lèvent aucune erreur ; invoice4j viole BR-S-02 et BR-CO-26.
- Réseau : aucun à l'exécution. `./run.sh setup` télécharge par HTTPS, vérifie
  par SHA-256 et décompresse dans `.artefacts/` les trois artefacts officiels :
  CEN `validation-1.3.17` (CII), KoSIT `xrechnung-schematron` v2.6.0 et KoSIT
  `validator-configuration-xrechnung` v2026-08-31. Maven télécharge les jars.
- Hôtes : `host.pdf` et `host_classic.pdf` n'ont plus d'arbre
  `/EmbeddedFiles`. Leur catalogue garde pourtant dans `/AF` une spécification
  `factur-x.xml` (`/AFRelationship /Alternative`) avec son flux `/UF`, ainsi
  qu'un XMP Factur-X en PDF/A-3. Conséquences :
  - Paniolo refuse d'y injecter une facture ;
  - Mustang produit un PDF qui contient deux `factur-x.xml` dans `/AF`.
