# Couverture : écosystème Rust

Banc Rust du protocole commun (`../PROTOCOL.md`). Un crate binaire par librairie
(`libs/<lib>`, paquet `bench-<lib>`), un harnais partagé (`harness`), versions
épinglées `=x.y.z` et `Cargo.lock`. Toolchain : rustc / cargo 1.93.1, profil
`release` par défaut.

Les preuves citées sont des chemins dans les sources des crates téléchargés par
Cargo (`~/.cargo/registry/src/index.crates.io-*/<crate>-<version>/…`).
Licence, dernière version et téléchargements viennent de l'API crates.io
(`/api/v1/crates/<nom>`, relevé du 2026-11-17 ; téléchargements : total, et sur
90 jours entre parenthèses quand ils diffèrent).

## Librairies mesurées

### en16931 + en16931-formats 0.10.1 (hupe1980)

Modèle sémantique EN 16931 et règles réimplémentées en Rust, lecteurs et
écrivains CII et UBL, extraction et injection ZUGFeRD / Factur-X (features
`cii`, `zugferd`). Aucun XSD ni XSLT exécuté : les règles sont du code.

- Artefacts : CEN `validation-1.3.17` (`en16931-0.10.1/src/lib.rs:95`, listes de
  codes générées dans `src/codes/generated.rs:6`) ; KoSIT
  `xrechnung-schematron` v2.6.0, c'est-à-dire XRechnung 3.0.x
  (`src/profiles.rs:26-31`) ; Peppol BIS Billing 3.0.21 (`src/profiles.rs:46`).
- Règles : dans `src/validation/`, 60 identifiants BR, 28 BR-CO, 23 BR-CL et
  21 BR-DEC, plus les familles des catégories de TVA que génère la macro
  `family!` (`src/validation/rules/category.rs:395`). Il y a aussi 25 BR-DE,
  14 BR-DEX et 5 BR-DE-CVD (`rules/xrechnung.rs`), ainsi que 64 identifiants
  PEPPOL (`rules/peppol.rs`).
- Couche syntaxe : 447 des 522 assertions `not(…)` CII-SR / CII-DT de CEN ; les
  75 autres demandent un moteur XPath et ne sont pas reprises
  (`en16931-formats-0.10.1/src/cii/prohibitions_generated.rs:10-13`).
  CII-SR-465 n'y figure pas.
- Factur-X : les quatre profils servent à classer le document. MINIMUM et
  BASIC WL n'exécutent que les règles de totaux ; BASIC et EXTENDED exécutent
  les règles cœur. Les règles propres à FeRD ne sont pas livrées
  (`src/profiles.rs:6-9`).
- Conteneur : `receive` sur un PDF compare le XMP (`fx:ConformanceLevel`, nom
  du fichier, `/AFRelationship`, `pdfaid:part`) avec la pièce jointe et BT-24
  (`en16931-formats-0.10.1/src/zugferd/extract.rs:22-83`). Il n'y a pas de
  contrôle ISO 19005.
- Injection : par mise à jour incrémentale (`src/zugferd/embed.rs:1-17`). Elle
  refuse un hôte qui porte déjà des métadonnées Factur-X. Les deux hôtes du
  protocole ont un XMP qui contient l'espace `fx:`, d'où le `fail` sur
  `Embed_Small` et `EmbedClassic_Small`.
- Validation mesurée : `receive(Policy::default())`, qui applique le profil
  déclaré par BT-24 (BR-DE sur les XRECHNUNG). On compte les entrées fatales
  des couches et les constats `fatal()`.
- Réseau : aucun.

### einvoice-validator 0.4.0 (+ einvoice-xsd, einvoice-pdf)

Exécute les artefacts officiels sans Java : un moteur XSD, XPath et XSLT 2.0
écrit en Rust (`einvoice-xsd`, `einvoice-xpath`, `einvoice-xslt`…).

- Embarqué par défaut : l'XSLT officiel CEN EN 16931 CII 1.3.17, non modifié
  (`bundles/cen-cii/bundle.toml:9`, `bundles/cen-cii/cii/xslt/EN16931-CII-validation.xslt`,
  810 identifiants de règles), et les XSD UN/CEFACT CII D16B (`einvoice-xsd`).
  Le README affirme que les verdicts concordent avec Saxon-HE sur les corpus
  de test CEN 1.3.17.
- Hors ligne, c'est le seul jeu de règles. Factur-X 1.09.2
  (`pins/fetch-facturx.pins:1`), XRechnung 3.0.2 / schematron 2.6.0
  (`pins/fetch-xrechnung.pins`), les règles françaises et Peppol 3.0.22
  (`pins/fetch-peppol.pins:1`) se téléchargent avec la feature `fetch`
  (`Cargo.toml:42-60`, `src/rulesets/mod.rs:15`). Cette feature n'est pas
  activée ici : le client HTTPS n'est pas compilé.
- Dans l'XSLT CEN, CII-SR-465 a le drapeau `warning`. Une validation CEN pure
  signale donc 2 fichiers sur 26, pas 6.
- Pas de modèle de facture, donc Parse et Write sont `unsupported`.
  `pdf::extract` renvoie le XML, l'`/AFRelationship` et le XMP sans les
  contrôler : CheckPDF est `unsupported`.
- `rust-version = "1.99"` est déclaré dans toutes les versions publiées, mais
  le crate compile avec 1.93.1. D'où `--ignore-rust-version` dans `run.sh`.

### erechnung-validator 0.1.1 (Promaris)

Rejoue côte à côte KoSIT et Mustang avec leurs artefacts, sur un moteur
XSLT / Schematron en Rust (`oxixml-*` 0.1.2). Pas de Java.

- KoSIT : configuration XRechnung 3.0.2, scénarios
  `artefakte/kosit/scenarios-xrechnung-3.0.2.xml` (CHANGELOG au 2026-01-31),
  XSD CII D16B, CEN `EN16931-CII-validation-1.3.16.sch` (`src/kosit/szenario.rs:32`),
  `XRechnung-CII-validation.xsl` (28 BR-DE, 7 BR-DEX, 6 BR-DE-CVD). Sources
  dans `NOTICE.md`.
- Mustang 2.26.0 : XSD et Schematron Factur-X 1.09.2 des cinq profils
  (`artefakte/mustang/sch/FACTUR-X_EN16931.sch:2-5`, « Version: 1.09.2 »),
  XRechnung 3.0 côté Mustang (`artefakte/mustang/xr/XR_30`). Ce sont les
  règles de profil Factur-X les plus complètes de l'écosystème (196
  identifiants dans `FACTUR-X_EXTENDED.xslt`).
- Pas de BR-FR : la variante EXTENDED-CTC-FR n'est pas embarquée. Pas de
  Peppol.
- Validation mesurée : `beide(Szenarien::Automatisch)`. Les erreurs des deux
  outils sont additionnées, préfixées `KoSIT:` ou `Mustang:`. C'est la seule
  librairie qui retrouve la référence de 6 fichiers sur 26.
- CheckPDF : `beide_pdf` est la fonction native de la librairie, mais elle se
  limite à extraire le XML puis à le valider. `src/pdf.md` le dit : « Keine
  PDF/A- oder veraPDF-Prüfung ». La note du benchmark le précise.
- Extract : le module `pdf` est privé (`src/lib.rs:24`), donc `unsupported`.
- Réseau : aucun.

### core-invoice + core-invoice-formats 2.0.3 (proxeon)

Modèle EN 16931 et PINT avec ses règles en Rust, codecs UBL 2.1 et CII D16B.

- Règles alignées sur CEN `validation-1.3.16` (`core-invoice-2.0.3/src/codes.rs:17`).
  On compte 58 BR, 23 BR-CO, 23 BR-CL, 21 BR-DEC et 98 règles de catégories
  de TVA. Peppol BIS v3.0.20 (`codes.rs:19`, 60 identifiants PEPPOL),
  PINT 1.1.2 et PINT-MY 1.3.0.
- BR-DE : 25 règles derrière la feature optionnelle `xrechnung`, activée ici
  (`src/xrechnung.rs:1-6`). Le README précise « Not KoSIT Valid »
  (`README.md:57`).
- `validate_xml` n'applique ni XSD ni CII-SR
  (`core-invoice-formats-2.0.3/src/lib.rs:179-189`). Les interdictions CII-SR
  générées (`src/prohibitions_cii.rs`) ne servent qu'à l'écriture
  (`lib.rs:298`).
- Le lecteur rejette les TypeCode 204 et 387 (« malformed ») : 3 fichiers sur
  26 ne se lisent pas.
- Validation : 4 des 7 signalements viennent des règles, qui produisent des
  faux positifs (BR-32, BR-37, BR-CO-11/12/13, BR-S-08 sur des exemples
  valides). Les 3 autres sont des erreurs de lecture.
- Pas de PDF. Réseau : aucun.

### zugferd-rs 0.2.2 (philipp-berger)

Modèle, lecteurs et écrivains CII et UBL (dont une variante XRechnung CII),
PDF via lopdf 0.47, validation XSD et Schematron via libxml2.

- XSD : Factur-X 1.09.2 / ZUGFeRD 2.5.2 et 1.08, par profil
  (`assets/schemas/cii/`, `src/validator/xsd.rs:19-27`).
- Schematron : le `.sch` prétraité de CEN `validation-1.3.17`
  (`assets/schemas/schematron/PROVENANCE.md`) est évalué en XPath 1.0 par
  libxml2. Les règles qui demandent XPath 2.0 sont sautées et notées `SKIP:`,
  avec un résultat déclaré incomplet (`src/validator/schematron.rs:1-20`).
  Toute assertion en échec devient une `Error`, quel que soit son drapeau
  (`schematron.rs:315-318`). Les warnings CEN CII-SR-119 et CII-SR-465
  comptent donc comme erreurs, alors que BR-CL-25 n'est pas trouvé.
- Saxon-HE (Java) existe en option, appelé seulement si l'appelant le demande
  (`src/validator/external.rs:1-5`). Non utilisé ici : Java est absent.
- Conteneur : `validate_pdf_a` applique 21 règles tirées des profils veraPDF
  PDF/A-3b (`src/pdfa/rules_generated.rs:1-9`), mais sans contrôler le XML.
  CheckPDF est donc `unsupported`.
- Injection : `embed_xml` réécrit le PDF en PDF/A-3b (XMP Factur-X,
  OutputIntent sRGB). Il convertit l'hôte, ce que précise la note.
- Le lecteur rejette les TypeCode 204, 387, 575 et 877 : 19 fichiers lus sur
  26, et `Parse_Large` en `fail`.
- Dépendances système : libxml2 (pkg-config) et libclang (bindgen dans le
  `build.rs` de `libxml` 0.3.24). Les XSD et le `.sch` se lisent à
  l'exécution depuis `assets/schemas` : `run.sh` exporte
  `ZUGFERD_SCHEMAS_DIR`, trouvé grâce à `cargo metadata`.
- Réseau : aucun.

### factur-x 0.3.1 et pdfa 0.1.10 (cipansor/crateworks)

Deux crates sœurs, l'un pour le XML, l'autre pour le conteneur.

- factur-x : modèle EN 16931, lecture et écriture CII et UBL.
  - `validate` contrôle 16 règles arithmétiques (`src/validate.rs` : BR-01 à
    BR-06, BR-09, BR-16, BR-CO-10, BR-CO-13 à BR-CO-18, BR-S-08). Le README
    annonce « BR-CO-10 to BR-CO-18 » (`README.md:78-79`), mais BR-CO-11 et
    BR-CO-12 manquent.
  - Le profil est seulement détecté (`profile_of`).
  - Les montants à quatre décimales sont refusés (« 9.9000 ») : 9 fichiers
    lus sur 26.
- pdfa :
  - `check_factur_x` contrôle cinq points du conteneur : XMP PDF/A-3,
    `/Metadata`, nom de la pièce jointe, `/AFRelationship` et somme de
    contrôle (README, `src/conformance.rs:106`). Il ne contrôle pas le XML,
    donc CheckPDF est `unsupported`.
  - `embed` refuse les flux d'objets et les flux xref
    (`src/container.rs:273-274`) : `fail` sur `host.pdf`, `ok` sur
    `host_classic.pdf`.
  - `embedded_file` cherche le premier flux `/Type /EmbeddedFile` par
    recherche d'octets : 32 PDF sur 104.
- Dépendance unique : `xmlkit`, du même dépôt. Réseau : aucun.

### pdf-invoice 1.5.0 (PDFluent SDK)

FDF/XFDF et modèle ZUGFeRD / Factur-X CII, sur un fork de lopdf
(`pdfluent-lopdf =0.39.9`, `Cargo.toml:61-63`). Licence AGPL-3.0 ou commerciale.

- Le modèle stocke les montants en `f64`.
- `validate_en16931` couvre 14 règles (`src/validation.rs` : BR-01 à BR-08,
  BR-CL-01, BR-CL-04, BR-CO-10, BR-CO-13, BR-CO-15, BR-S-08).
- `validate` vérifie les champs exigés par le profil. Il exige notamment
  `buyer_reference` et `payment_means` en EN16931, ce qui produit des faux
  positifs sur 20 des 26 exemples valides.
- `to_xml` refuse d'écrire un modèle qui ne passe pas ces contrôles : `fail`
  sur `Write_*`.
- Extraction : un nom à la fois, dans le tableau `/Names` racine. 101 PDF sur
  104 ; « /EF not a dict » sur trois PDF Gnuaccounting.
- Injection : réécrit tout le fichier et remplace le XMP, sans conversion
  PDF/A.
- Pas de XSD, pas de UBL, pas de contrôle de conteneur. Réseau : aucun.

### zugferd 0.3.0 (FaultierSP)

Générateur seulement : un `InvoiceBuilder` produit du XML MINIMUM, BASIC WL ou
BASIC (serde + quick-xml).

- Pas de lecteur, pas de PDF.
- `validate_business_rules` (`src/components/business_rules.rs:104`) ne
  s'applique qu'au modèle construit : 17 identifiants, dont un fantaisiste,
  « BR-7890 ».
- `Write_Small` utilise un modèle construit à la main d'après
  `EN16931_Einfach`, au niveau BASIC. Le builder exige une référence de
  commande (BT-13) et une échéance absentes de l'exemple : on y met « - » et
  la date des conditions de paiement. `Write_Lines` est sauté.

### einvoice 0.1.1

Modèles CII et UBL générés pour un fork de yaserde
(`einvoice_deps_yaserde =0.0.1`).

- Les validateurs de règles sont des bouchons : 777 fonctions
  `if false { return Err(…) }` côté CII et 967 côté UBL
  (`src/cii/cii_business_rule_validator.rs`). Validate est donc
  `unsupported`.
- `validate_zugferd_pdf` prend un chemin de fichier, et l'extraction est
  privée (`src/lib.rs:146,165`).
- Mesurés : Parse (`yaserde::de::from_str`) et Write
  (`yaserde::ser::to_string`).

### lopdf 0.47.0 (plancher)

Librairie PDF généraliste, pas une librairie Factur-X. Elle sert de plancher
d'extraction : chargement complet, parcours de `/Names/EmbeddedFiles` avec
`/Kids`, puis inflate, le tout écrit à la main dans `libs/lopdf/src/main.rs`.
zugferd-rs, en16931-formats, erechnung-validator et einvoice reposent sur
lopdf, dans des versions de 0.34 à 0.47.

## Librairies écartées

| Crate | Version | Raison |
|---|---|---|
| facturx | 0.1.1 (28 tél.) | Détection du profil seulement, pas de lecture, d'écriture ni de validation de facture |
| invx | 0.1.1 (27) | Pas de CII |
| xrechnung | 0.1.0 (567) | Génère des factures UBL XRechnung pour freelances ; pas de CII ni de lecture |
| openbim-xrechnung | 0.1.0 (11) | Squelette sans implémentation |
| faktura | 0.0.0 (261) | Les 15 versions publiées sont retirées (yanked) |
| stanzaapi-factur-x-validator | 1.0.0 (12) | Client d'une API distante (exclu par le protocole) |
| en16931-cli, core-invoice-cli, einvoice-cli | 0.10.1 / 2.0.3 / 0.4.0 | Interfaces en ligne de commande de librairies mesurées |
| einvoice-xsd, einvoice-pdf, einvoice-schematron | 0.4.0 | Composants d'einvoice-validator : einvoice-xsd et einvoice-pdf sont mesurés à travers lui ; einvoice-schematron ne sert qu'à la feature `fetch` |
| justpdf-special | 0.1.2 (87) | Volet ZUGFeRD d'une boîte à outils PDF (OCR, codes-barres), lourdes dépendances de rendu ; moins de 100 téléchargements |
| cius-ro | 0.1.0 (14) | CIUS roumaine (BR-RO-*, `efactura.xml`), publiée le 2026-11-15 ; hors sujet pour un corpus allemand Factur-X / XRechnung |
| tpt-doc-ubl | 0.1.0 (21) | UBL / Peppol seulement |
| invoice-kit | 2.0.0 (735) | Facturation de grand livre ; pas d'E/S de facture électronique |
| schematron-engine | 0.5.0 (4 866) | Moteur Schematron générique, sans artefacts de facture |
| zugferd-code-lists | 0.1.19 (2 513) | Énumérations de listes de codes |
| billing | 0.16.0 (3 782) | Arithmétique EN 16931 (dépendance de développement d'en16931-formats) |

Aucun candidat n'exige Java. Le seul usage de Java est l'option Saxon de
zugferd-rs, laissée inactive.

## Relevés d'exactitude (`./run.sh survey`, `results/2026-11-17/survey_rust.jsonl`)

| Librairie | validate (signalés / 26) | parse (/ 26) | extract (/ 104) |
|---|---|---|---|
| en16931 | 2 (BR-CL-25 ×2) | 26 | 104 |
| einvoice-validator | 2 (BR-CL-25 ×2) | — | 104 |
| erechnung-validator | **6** (KoSIT BR-CL-25 ×2, avec Mustang FX-SCH-A-000571 ; KoSIT CII-SR-465 ×4 sur les XRECHNUNG) | — | — |
| core-invoice | 7 (3 erreurs de lecture, 4 faux positifs BR-32 / BR-CO-1x / BR-S-08, dont un BR-CL-25 juste) | 23 | — |
| zugferd-rs | 7 (CII-SR-465 ×4, CII-SR-119 ×3, warnings CEN comptés en erreur) | 19 | 104 |
| factur-x | 17 (erreurs de lecture des montants à 4 décimales) | 9 | — |
| pdfa | — | — | 32 |
| pdf-invoice | 20 (exigences de profil, BR-03, BR-08) | 26 | 101 |
| einvoice | — | 26 | — |
| lopdf | — | — | 104 |
| zugferd | — | — | — |

Référence (PROTOCOL) : 6 sur 26.

## Statut des benchmarks (essai `COUNT=1 BENCHTIME=0.2`)

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
| en16931 | o o o o | o o o | o o o | o o | F F | o o |
| einvoice-validator | o o o o | - - - | o o o | - - | - - | - - |
| erechnung-validator | - - - - | - - - | o o o | - - | - - | o o |
| core-invoice | - - - - | o o o | o o o | o o | - - | - - |
| zugferd-rs | o o o o | o o F | o o o | o o | o o | - - |
| factur-x | - - - - | F F o | F F o | F F | - - | - - |
| pdfa | o F o F | - - - | - - - | - - | F o | - - |
| pdf-invoice | o o o o | o o o | o o o | F F | o o | - - |
| zugferd | - - - - | - - - | - - - | o (sauté) | - - | - - |
| einvoice | - - - - | o o o | - - - | o o | - - | - - |
| lopdf | o o o o | - - - | - - - | - - | - - | - - |

## Tableau de couverture

| Librairie | Version | Licence | Dernière version | Téléchargements | Lecture CII | Écriture CII | UBL | Extraction PDF | Injection PDF | XSD | EN 16931 (CII) | Profils Factur-X (MINIMUM, BASIC WL, EXTENDED) | BR-FR | BR-DE (XRechnung) | Peppol BIS 3 | Contrôle du conteneur PDF/A-3 | Dépendances externes |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| en16931 (+formats) | 0.10.1 | MIT OR Apache-2.0 | 0.10.1 (2026-11-07) | 9 983 (9 534) ; formats 9 730 | oui | oui | oui (lecture, écriture) | oui | oui (incrémentale ; refuse un hôte déjà Factur-X) | non (447/522 interdictions CII-SR/DT en code) | partiel : règles CEN 1.3.17 réimplémentées en Rust (BR, BR-CO, BR-CL, BR-DEC, catégories TVA) | partiel : classement des 4 profils, règles de totaux ou cœur ; pas les règles FeRD | non | oui : KoSIT schematron v2.6.0, XRechnung 3.0 (25 BR-DE, 14 BR-DEX, CVD) | oui : 3.0.21 (64 règles) | partiel : XMP fx: et pdfaid:part comparés à la pièce jointe et à BT-24, /AFRelationship ; pas d'ISO 19005 | aucune (Rust pur) |
| einvoice-validator | 0.4.0 | MIT AND EUPL-1.2 AND OASIS / UN/CEFACT / W3C | 0.4.0 (2026-11-10) | 62 | non (pas de modèle) | non | validation seulement (CEN UBL 1.3.17) | oui (einvoice-pdf) | non | oui : CII D16B, moteur XSD en Rust | oui : XSLT officiel CEN 1.3.17 exécuté en Rust (810 règles) | hors ligne non ; Factur-X 1.09.2 via la feature `fetch` (téléchargement) | hors ligne non ; 14 jeux via `fetch` | hors ligne non ; XRechnung 3.0.2 via `fetch` | hors ligne non ; 3.0.22 via `fetch` | non (XMP et /AFRelationship renvoyés, pas contrôlés) | aucune ; réseau seulement avec `fetch` (non activée) |
| erechnung-validator | 0.1.1 | MIT OR Apache-2.0 (artefacts EUPL-1.2 / Apache-2.0) | 0.1.1 (2026-10-16) | 29 | non (validation seule) | non | validation (KoSIT UBL) | interne seulement (module privé) | non | oui : CII D16B (KoSIT) et XSD Factur-X par profil (Mustang) | oui : Schematron CEN 1.3.16 (KoSIT) et règles CEN dans les schematrons Mustang | oui : Schematron Factur-X 1.09.2 des 5 profils (Mustang 2.26.0) | non | oui : configuration KoSIT XRechnung 3.0.2 (schematron 2.6.0 : 28 BR-DE, 7 BR-DEX, 6 CVD) | non | non (`beide_pdf` = extraction + validation XML) | aucune (moteur oxixml en Rust) |
| core-invoice (+formats) | 2.0.3 | MIT OR Apache-2.0 | 2.0.3 (2026-09-04) | 293 ; formats 254 | partiel (rejette TypeCode 204/387) | oui | oui | non | non | non | partiel : règles CEN 1.3.16 réimplémentées (~223) ; pas de CII-SR à la validation | non | non | partiel : 25 BR-DE, feature `xrechnung`, « Not KoSIT Valid » | oui : v3.0.20 (60 règles) ; PINT 1.1.2, PINT-MY 1.3.0 | non | aucune |
| zugferd-rs | 0.2.2 | Apache-2.0 | 0.2.2 (2026-11-11) | 92 | partiel (rejette TypeCode 204/387/575/877) | oui | oui | oui | oui (convertit en PDF/A-3b) | oui : Factur-X 1.09.2 et 1.08 par profil (libxml2) | partiel : .sch CEN 1.3.17 en XPath 1.0, règles XPath 2.0 sautées, drapeaux ignorés ; Saxon (Java) en option | XSD seulement | non | non (écriture XRechnung CII seulement) | non | partiel : 21 règles veraPDF PDF/A-3b (`validate_pdf_a`), séparées du XML | libxml2 système, libclang à la compilation ; Java optionnel |
| factur-x | 0.3.1 | Apache-2.0 | 0.3.1 (2026-10-30) | 131 | partiel (refuse les montants à 4 décimales) | oui | oui | non | non | non | partiel (16 règles arithmétiques) | détection du profil seulement | non | non | non | non | aucune |
| pdfa | 0.1.10 | Apache-2.0 | 0.1.10 (2026-10-30) | 159 | non | non | non | partiel (recherche d'octets, 32/104) | partiel (xref classique seulement ; nouveau XMP PDF/A-3B) | non | non | non | non | non | non | partiel (5 contrôles : XMP PDF/A-3, /Metadata, nom, /AFRelationship, somme de contrôle) | aucune |
| pdf-invoice | 1.5.0 | AGPL-3.0-only OR commerciale | 1.5.0 (2026-11-19) | 72 | oui (montants f64) | partiel (refuse si les contrôles de profil échouent) | non | oui (101/104) | oui (réécriture, XMP remplacé, pas de PDF/A) | non | partiel (14 règles) | partiel (champs exigés par profil) | non | non | non | non | aucune (fork pdfluent-lopdf) |
| zugferd | 0.3.0 | Apache-2.0 | 0.3.0 (2026-11-11) | 6 013 (168) | non | oui (MINIMUM, BASIC WL, BASIC, depuis un builder) | non | non | non | non | non (17 règles sur le modèle construit seulement) | génération MINIMUM / BASIC WL / BASIC ; pas EN16931 ni EXTENDED | non | non | non | non | aucune |
| einvoice | 0.1.1 | MIT | 0.1.1 (2025-01-18) | 1 965 (12) | oui (yaserde) | oui | oui (modèle) | privée (chemin de fichier) | non | non | non (777 bouchons `if false`) | non | non | non | non | non | aucune (fork yaserde) |
| lopdf (plancher) | 0.47.0 | MIT | 0.47.0 (2026-11-04) | 30 787 114 (14 860 187) | non | non | non | plancher écrit à la main | non | non | non | non | non | non | non | non | aucune |

## Notes de construction et de mesure

- Chaque paquet se construit à part (`cargo build -p bench-<lib>`), pour que
  chaque binaire n'ait que les features de ses propres dépendances.
  `--locked --ignore-rust-version` : voir einvoice-validator.
- `RAYON_NUM_THREADS=1` : lopdf, avec ses features par défaut (zugferd-rs,
  pdf-invoice, einvoice), analyse les objets avec rayon. Le protocole demande
  un seul fil.
- Le corps de chaque binaire tourne dans un fil à pile de 512 Mio. Les
  stylesheets officielles récursent profondément : erechnung-validator
  demande 64 Mio.
- `first_call_ns` inclut la compilation paresseuse des stylesheets :
  `Validator::cen_cii()` dans un `OnceLock` (einvoice-validator) et le cache
  d'erechnung-validator.
- Revue de sécurité avant construction : `build.rs` et proc-macros des crates
  peu téléchargés (`einvoice_deps_yaserde_derive`, `multiversion_no_op`,
  `libxml`, `oxixml-*`, `einvoice-*`). Aucun accès réseau, aucun fichier
  hors du répertoire de build. Seul `libxml` interroge pkg-config et lance
  bindgen sur les en-têtes système.
- Dans les sources compilées, les seules occurrences de `std::net` analysent
  des adresses IPv6 dans les IRI (`oxixml-iri`, `einvoice-xsd`). Le client
  HTTPS d'einvoice-validator n'est pas compilé (feature `fetch` inactive) ;
  le module Saxon de zugferd-rs (`validator::external`) est compilé mais
  jamais appelé.
