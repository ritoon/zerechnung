# Banc PHP : librairies retenues, écartées et couverture de la norme

Banc PHP du protocole commun (`../PROTOCOL.md`), comparé au paquet Go
`github.com/ritoon/zerechnung`. Relevés du 2026-11-22.

- **Interpréteur** : PHP 8.6.0 NTS (Homebrew, `/opt/homebrew/bin/php`),
  libxml2 2.9.13, libxslt 1.1.35, Composer 2.11.2. Extensions vérifiées par
  `./run.sh setup` : dom, libxml, xml, simplexml, xmlreader, xmlwriter,
  mbstring, gd, zlib, bcmath, fileinfo, zip, iconv, ctype (exigées par les
  paquets figés), pcntl (délai des appels dans le harnais).
- **OPcache et JIT** : le CLI tel qu'installé, OPcache éteint
  (`opcache.enable_cli=0`, valeur par défaut du CLI), donc pas de JIT. C'est
  ce que reçoit un script PHP lancé en ligne de commande, et le choix est
  écrit dans le champ `runtime` de chaque ligne JSON (`PHP 8.6.0 (NTS,
  OPcache off, JIT off, zend.assertions=-1)`). Dans un processus CLI, OPcache
  n'évite pas la compilation des fichiers (le cache meurt avec le processus,
  et la compilation tombe dans `first_call_ns`) ; il apporte l'optimiseur
  d'opcodes et rend le JIT possible. Pour une campagne avec JIT (vérifié :
  `runtime` passe à `OPcache on, JIT tracing`) :
  `PHP_OPTS="-d memory_limit=4G -d zend.assertions=-1 -d display_errors=stderr -d error_reporting=6143 -d opcache.enable_cli=1 -d opcache.jit=tracing -d opcache.jit_buffer_size=64M" ./run.sh bench`.
- **Réglages forcés** par `run.sh` dans `PHP_OPTS`, transmis à chaque
  processus fils : `memory_limit=4G` (128M par défaut, pour qu'aucune
  librairie n'échoue faute de mémoire sur les gros fichiers ou pendant le
  relevé), `zend.assertions=-1` (le php.ini installé est celui de
  développement, `zend.assertions=1`, qui exécuterait les `assert()` des
  librairies), `display_errors=stderr` et `error_reporting=6143` (E_ALL sans
  les dépréciations : plusieurs librairies en déclenchent sous PHP 8.6, et
  les afficher ralentirait la mesure). Aucun de ces réglages ne change le
  code exécuté.
- **Deux environnements Composer**, versions figées exactement et
  `composer.lock` versionnés : `composer.json` (96 paquets installés) et
  `invoicesuite/composer.json` (36 paquets) pour horstoeko/invoicesuite, qui
  déclare un conflit avec horstoeko/zugferd. Installation par
  `composer install --no-scripts --no-plugins --ignore-platform-req=php+
  --optimize-autoloader` : aucun script ni plugin tiers ne tourne à
  l'installation ; `php+` n'ignore que les bornes hautes
  (php-erechnung-toolkit, php-pdf-toolkit, les dschuppelius/* et
  mahdiabderraouf/facturx-php déclarent `php <8.6`). `vendor/` pèse 516 Mo
  (dont 120 Mo de règles précompilées de vampire-php), `invoicesuite/vendor/`
  134 Mo.
- **Pas de Java** : aucun adaptateur ne lance de JVM. Les validateurs qui en
  demandent une ne sont pas mesurés : KoSIT et veraPDF de horstoeko/zugferd,
  KoSIT de horstoeko/invoicesuite, de quittio et de php-erechnung-toolkit,
  couche Schematron de pn-invoice (voir les fiches).
- **Pas de réseau** pendant `survey` et `bench`. Code réseau trouvé et non
  emprunté : téléchargement du validateur KoSIT et de veraPDF
  (horstoeko/zugferd, horstoeko/invoicesuite), envoi de la facture au
  service Docuflair (horstoeko/invoicesuite), téléchargement des règles
  Peppol BIS (intragile), recherche SMP et transport Peppol
  (php-erechnung-toolkit).
- **Outil externe** : `pdfdetach` de poppler (`/opt/homebrew/bin`), lancé par
  mahdiabderraouf/facturx-php et par php-pdf-toolkit (extraction de
  php-erechnung-toolkit). Le lancement des processus entre dans la mesure.
- **XSLT 2.0 impossible** : ext-xsl (libxslt 1.1.35) refuse les XSLT
  officiels. `XSLTProcessor::importStylesheet` renvoie `false` sur
  `EN16931-CII-validation.xslt` du CEN 1.3.17 comme sur
  `horstoeko/zugferd/src/schema/xslt/FACTUR-X_EN16931.xslt`, avec
  « xsl:version: only 1.1 features are supported » puis « Invalid
  expression ». Le seul moteur XSLT 2.0 pour PHP est l'extension SaxonC-HE :
  l'archive 13.1.1 macOS arm64 de Saxonica (sans brew) ne compile pas avec
  PHP 8.6, avec 20 erreurs dans `php8_SchemaValidator.cpp` (`XtOffsetOf` non
  défini, `WRONG_PARAM_COUNT` retiré de l'API Zend). L'extension n'a pas été
  corrigée : aucun banc PHP n'exécute les XSLT officiels. Les validateurs
  complets mesurés ici les remplacent par des réécritures en PHP (interprète
  XPath 2.0 de vampire-php, règles compilées en PHP d'intragile et de
  dealerweb).

Lancement : `./run.sh setup`, puis `./run.sh survey [lib…]` et
`./run.sh bench [lib…]` (variables `OUT`, `COUNT`, `BENCHTIME` ; aussi `PHP`,
`COMPOSER`, `PHP_OPTS`, `FX_TMP` = dossier des fichiers temporaires des API
par chemin, `<temp système>/fx-bench-php` par défaut, `CHECK_TIMEOUT` = 30 s
pour l'appel de contrôle d'un benchmark, `SURVEY_TIMEOUT` = 10 s par fichier
du relevé, `CHILD_TIMEOUT` = 1800 s par processus fils). Chaque benchmark
tourne dans un **processus fils neuf** (`proc_open`), si bien que
`first_call_ns` est un vrai premier appel (autoloader Composer chargé,
classes de la librairie pas encore chargées, caches froids). Le relevé lance
un processus par librairie et par opération ; un fils qui meurt (erreur
fatale, mémoire) est relancé après le fichier sur lequel il est mort. La
mesure suit le protocole : N = 1, 2, 5, 10, 20, 50… jusqu'à une série d'au
moins `BENCHTIME`, puis `COUNT` séries, `hrtime(true)`, un seul fil.

Les chemins `paquet/fichier.php:ligne` ci-dessous sont relatifs à `vendor/`,
sauf ceux de horstoeko/invoicesuite et de prinsfrank/pdfparser, relatifs à
`invoicesuite/vendor/`.

## Résultats du relevé (`./run.sh survey`)

| Librairie | validate (signalés / 26) | parse (/ 26) | extract (/ 104) |
|---|---|---|---|
| horstoeko/zugferd 1.0.135 | 0 (XSD seule) | 26 | 104 |
| horstoeko/invoicesuite 0.0.44 | 0 (XSD seule) | 26 | 88 |
| atgp/factur-x 3.5.0 | 4 (XRECHNUNG refusés, XSD seule) | — | 100 (90 avec `validateXsd: true`, la valeur par défaut) |
| tiime/factur-x 0.3.2 | 4 (XRECHNUNG refusés, XSD seule) | — | 70 |
| tiime/cross-industry-invoice 0.3.11.1 | — | 23 | — |
| easybill/zugferd-php 6.0.0 | 0 (XSD seule) | 26 | — |
| easybill/e-invoicing 0.4.0 | — | 24 | — |
| mahdiabderraouf/facturx-php 2.4.0 | 4 (XRECHNUNG refusés, XSD seule) | — | 100 |
| php-erechnung-toolkit 0.23.1 | 1 (TK-ATT-EMPTY) | 26 | 104 |
| john-wink/en16931-php 0.4.1 | 0 | 26 | — |
| invoicepress/vampire-php 1.3.0 | **6** (BR-CL-25 ×2, CII-SR-465 ×4) | — | — |
| intragile/php-einvoicing 1.0.2 | **6** (BR-CL-25 ×2, CII-SR-465 ×4) | 26 | 104 |
| dealerweb/einvoice 1.2.3 | **6** (BR-CL-25 ×2, CII-SR-465 ×4) | 26 | 104 |
| quittio/core 1.1.0 | 6, pas les mêmes (QUITTIO-IBAN ×3, BR-CL-25 ×2, DE-USTG-03) | 26 | 104 |
| pnscripts/pn-invoice 0.2.1 | 0 | — | — |
| pat-o-dev/factur-x 0.8.0 | 2 (has-optional-party-name) | — | 1 |
| stann/factur-x 2.0.0 | 0 (XSD seule) | — | — |
| smalot/pdfparser 2.13.1 (plancher) | — | — | 104 |
| ext-dom (plancher XSD) | 0 (XSD seule) | — | — |

« — » : l'opération n'existe pas dans la librairie (rien n'est écrit dans
`survey_php.jsonl`). Une exception pendant la validation compte comme un
signalement, comme dans les autres bancs.

**Référence 6/26 reproduite** par trois librairies, sur les mêmes fichiers
et avec les mêmes règles : BR-CL-25 sur `EN16931_ElektronischeAdresse` et
`EN16931_RechnungsUebertragung`, CII-SR-465 sur les quatre `XRECHNUNG_*`.
Toutes trois appliquent la table de scénarios du validateur KoSIT, avec les
niveaux personnalisés qui font de CII-SR-465 une erreur pour XRechnung :
vampire-php (schematron CEN 1.3.17), intragile et dealerweb (CEN 1.3.16, la
version qu'épingle la configuration KoSIT ; intragile embarque la table
`scenarios-2026-08-31`).

Détail des écarts à la référence :

- **quittio 6/26, mais pas les mêmes** : les deux BR-CL-25 sont vus ; les
  quatre CII-SR-465 ne le sont pas (pas de schematron hors ligne, le niveau
  L5 est le validateur KoSIT en Java). Les quatre autres signalements
  viennent de contrôles propres à quittio, hors EN 16931 : QUITTIO-IBAN
  (clé de contrôle d'IBAN, niveau L7 de plausibilité,
  `quittio/core/src/Validation/Rules/PlausibilityRules.php:33`) sur
  `EN16931_AbweichenderZahlungsempf`, `EN16931_Innergemeinschaftliche_Lieferungen`
  et `EN16931_OEPNV` (IBAN d'exemple du corpus), et DE-USTG-03 (date de
  livraison exigée par le § 14 UStG,
  `quittio/country-de/src/Rules/UstgRules.php:82`) sur
  `EN16931_Physiotherapeut`.
- **john-wink 0/26** : BR-CL-25 est codé au niveau avertissement
  (`john-wink/en16931-php/src/RuleSets.php:227-228`, `Severity::Warning`),
  et CII-SR-465 manque dans `resources/syntax-rules.json` (la référence de la
  librairie est validator-configuration-xrechnung 3.0.2 de 2025-03-21,
  antérieure au niveau personnalisé). Seules les violations fatales sont
  comptées (verdict de la librairie).
- **atgp, tiime/factur-x, mahdiabderraouf 4/26** : les quatre `XRECHNUNG_*`
  sont refusés (exception « Invalid Factur-X URN » ou « invalid profile
  found »), l'identifiant XRechnung n'étant pas un profil Factur-X pour ces
  librairies. Les 22 autres passent le XSD.
- **php-erechnung-toolkit 1/26** : TK-ATT-EMPTY (pièce jointe vide, contrôle
  du toolkit) sur `EN16931_Betriebskostenabrechnung`.
- **pat-o-dev 2/26** : has-optional-party-name (règle de structure de la
  librairie) sur les deux `*_Betriebskostenabrechnung`.
- **horstoeko/zugferd, invoicesuite, easybill/zugferd-php, stann, ext-dom
  0/26** : XSD seule, et les 26 fichiers sont conformes au XSD. pn-invoice
  0/26 : ses 104 règles métier ne couvrent ni BR-CL ni CII-SR.
- **parse** : tiime/cross-industry-invoice refuse le schéma `EM` (« Wrong
  schemeID », 2 fichiers) et un `SpecifiedProcuringProject` sans nom
  (`EN16931_Physiotherapeut`). easybill/e-invoicing refuse le code EAS 9958,
  absent de son énumération (les deux mêmes fichiers que BR-CL-25).
- **extract** : atgp et mahdiabderraouf ne cherchent pas la pièce jointe
  `xrechnung.xml` (les quatre `FX/XRECHNUNG_*.pdf`) ; tiime/factur-x ne
  cherche que `factur-x.xml` (`tiime/factur-x/src/Reader.php:44`, 34 échecs) ;
  atgp avec ses options par défaut valide aussi le XSD et refuse 10 XML de
  plus. invoicesuite (prinsfrank/pdfparser) échoue sur 16 PDF : flux
  `gzuncompress` illisibles, racine introuvable, et XRechnung embarqué dont
  aucun fournisseur de format ne reconnaît le contenu. pat-o-dev n'en lit
  qu'un : 62 renvois /Names ou /AF non résolus, 38 `/Length` indirects,
  3 Filespec sans /EF (la librairie annonce une xref classique seulement).

## Fiches : librairies retenues

### horstoeko/zugferd 1.0.135

MIT, dernière version 2026-11-08, 10 006 092 téléchargements au total,
1 252 800 le mois dernier. La librairie PHP de référence (ZUGFeRD 1 et 2,
Factur-X, XRechnung CII).

- Lecture `ZugferdDocumentReader::readAndGuessFromContent`
  (`horstoeko/zugferd/src/ZugferdDocumentReader.php:306`), modèle JMS généré
  depuis les XSD ; écriture `ZugferdDocument::serializeAsXml`.
- PDF : `ZugferdDocumentPdfReader::getXmlFromContent`
  (`src/ZugferdDocumentPdfReader.php:97`, smalot/pdfparser) ; injection
  `ZugferdDocumentPdfMerger` (`src/ZugferdDocumentPdfMerger.php:56`) : FPDI
  réimporte les pages, XMP PDF/A-3B.
- Validation hors ligne : `ZugferdXsdValidator`
  (`src/ZugferdXsdValidator.php:66`, sur le document resérialisé lignes
  164-182) avec le XSD Factur-X du profil ; le profil XRechnung pointe vers
  le XSD EN16931 (`src/ZugferdProfiles.php:260-266`).
  `ZugferdDocumentValidator` (`src/ZugferdDocumentValidator.php:67`) ne
  vérifie que des contraintes Symfony (groupe `xsd_rules`).
- Schematron et XSLT livrés (`src/schema/schematron`, `src/schema/xslt`),
  déclarés dans `src/ZugferdProfiles.php:109-157` mais exécutés par aucun
  code ; ext-xsl refuse d'ailleurs le XSLT (2.0).
- `ZugferdKositValidator` télécharge le validateur KoSIT 1.5.0 et la
  configuration XRechnung 3.0.2 (`src/ZugferdKositValidator.php:96`, `:103`)
  et lance `java` (`:786`, `:999`) ; `ZugferdPdfValidator` télécharge veraPDF
  (`src/ZugferdPdfValidator.php:158`) et lance Java (`:636`). Non mesurés.

### horstoeko/invoicesuite 0.0.44

MIT, dernière version 2026-11-20, 21 563 au total, 15 658 le mois dernier.
Successeur de horstoeko/zugferd par le même auteur : un DTO pour de nombreux
formats (ZUGFeRD/Factur-X, XRechnung CII et UBL, Peppol, FatturaPA…).
Environnement Composer séparé (conflit déclaré avec horstoeko/zugferd).

- Lecture `InvoiceSuiteDocumentReader::createFromContent` (fournisseur de
  format deviné, JMS) ; écriture `copyToBuilder()->getContent()`.
- PDF : `InvoiceSuitePdfDocumentReader::createFromContent`
  (`horstoeko/invoicesuite/src/InvoiceSuitePdfDocumentReader.php:108`) lit
  le PDF avec prinsfrank/pdfparser 4.4.0 et **désérialise aussi le XML
  trouvé** (compris dans `Extract_*`) ; les flux mal compressés échouent dans
  `prinsfrank/pdfparser/src/Document/Filter/Decode/FlateDecode.php:136`.
  Injection `InvoiceSuitePdfDocumentBuilder` (FPDI/TCPDF).
- Validation hors ligne : `InvoiceSuiteXsdDocumentValidator`
  (`src/validators/InvoiceSuiteXsdDocumentValidator.php:200` choix du XSD,
  `:236` et `:248` `schemaValidate`). Les deux autres validateurs ne sont pas
  mesurés : `InvoiceSuiteKositDocumentValidator` télécharge le jar KoSIT
  1.6.2 (`src/validators/InvoiceSuiteKositDocumentValidator.php:54`) et lance
  Java ; `InvoiceSuiteDocuflairDocumentValidator` envoie la facture à
  `https://invoice.docuflair.com` (`:39`, curl lignes 132 et 181).

### atgp/factur-x 3.5.0

MIT, dernière version 2026-08-18, 1 122 993 au total, 61 110 le mois
dernier. Outil Factur-X sans modèle : extraction, injection, XSD.

- `Reader::extractXML` (`atgp/factur-x/src/Reader.php:40`, smalot/pdfparser),
  appelé avec `validateXsd: false` (par défaut l'extraction valide aussi le
  XSD) ; `Writer::generate` (`src/Writer.php:86`) avec FPDI, XMP PDF/A-3,
  XSD incluse par défaut.
- `XsdValidator` (`src/XsdValidator.php:21-28`) : XSD Factur-X **1.09.2**
  des cinq profils, rien d'autre ; XRechnung n'est pas un profil reconnu.

### tiime/factur-x 0.3.2

MIT, dernière version 2026-10-07, 280 508 au total, 4 784 le mois dernier.
Réécriture d'atgp/factur-x pour PHP 8.3.

- `Reader::extractXML` (`tiime/factur-x/src/Reader.php:20`) ne cherche que
  `factur-x.xml` (`:44`) ; `Writer::generate` avec FPDI.
- XSD Factur-X **1.07.3** (`src/XsdProcessor.php:7-13`). Le schematron livré
  sous `schematron/` n'est appelé par aucun code de `src/`.

### tiime/cross-industry-invoice 0.3.11.1

MIT, dernière version 2025-10-27, 269 684 au total, 808 le mois dernier.

- Modèle CII typé par profil (Minimum, BasicWL, Basic, EN16931 et un modèle
  français « Flux1 »), `CrossIndustryInvoice::fromXML` et `toXML`
  (`tiime/cross-industry-invoice/src/EN16931/CrossIndustryInvoice.php:77`,
  `:42`). Le modèle sémantique EN 16931 est dans tiime/en-16931 0.10.5
  (dépendance).
- Les XSD et schematrons sous `xsd/` ne servent qu'aux tests du paquet :
  aucune API de validation. Pas de PDF.

### easybill/zugferd-php 6.0.0

MIT, dernière version 2026-03-30, 407 329 au total, 22 919 le mois dernier.

- Modèle JMS de ZUGFeRD 1 et de ZUGFeRD 2 / Factur-X : `Reader::transform`
  (`easybill/zugferd-php/src/zugferd2/Reader.php:15`), `Builder::transform`
  (`src/zugferd2/Builder.php:22`).
- `Validator::validateAgainstXsd` (`src/zugferd2/Validator.php:11-15`) :
  XSD Factur-X **1.08.0**, schéma passé par l'appelant (l'adaptateur le
  choisit d'après BT-24, EN16931 pour XRechnung). Pas de PDF.

### easybill/e-invoicing 0.4.0

MIT, dernière version 2026-09-07, 198 338 au total, 9 426 le mois dernier.

- Modèles JMS de CII et d'UBL (EN 16931 et ses CIUS) : `Reader::read`
  (`easybill/e-invoicing/src/Reader.php:16`), `Transformer::transformToXml`
  (`src/Transformer.php:16`). Ni validation ni PDF.

### mahdiabderraouf/facturx-php 2.4.0

MIT, dernière version 2026-09-11, 12 121 au total, 1 708 le mois dernier.

- Extraction par `pdfdetach` de poppler, lancé deux fois par `exec`
  (`mahdiabderraouf/facturx-php/src/Helpers/XmlExtractor.php:40` `-list`,
  `:74` `-save` vers un fichier temporaire) : API par chemin, le lancement des
  processus est mesuré.
- Générateur XML pour MINIMUM, BASIC WL et BASIC seulement (`readme.md:24`),
  sans lecteur : `Write_Small` part d'un modèle construit à la main d'après
  `EN16931_Einfach`, écrit au profil BASIC.
- `Validator::validate` (`src/Validator.php:35`) : XSD Factur-X **1.09.2**
  du profil lu dans BT-24 ; l'énumération `Profile` n'a pas de cas
  XRechnung, d'où le refus des `XRECHNUNG_*`.
- `Generator::generate` valide le XML contre le XSD avant d'injecter
  (`src/Generator.php:88`), puis PDF/A-3b par FPDI/tFPDF (`:94`).

### daniel-jorg-schuppelius/php-erechnung-toolkit 0.23.1 (+ php-pdf-toolkit 0.33.0)

AGPL-3.0-or-later (les deux paquets), dernière version 2026-11-14,
7 295 au total, 1 468 le mois dernier (php-pdf-toolkit : 7 671 et 1 555).
Boîte à outils allemande : XRechnung/ZUGFeRD, UBL et CII, Order-X, GAEB,
Peppol.

- Lecture `ERechnungParser::parse`, écriture `ERechnungGenerator::generateCii`.
- Extraction : `ZugferdPdfParser::extractXml`
  (`daniel-jorg-schuppelius/php-erechnung-toolkit/src/Parsers/ZugferdPdfParser.php:98`)
  délègue à php-pdf-toolkit, dont `ZugferdReader`
  (`daniel-jorg-schuppelius/php-pdf-toolkit/src/Readers/ZugferdReader.php:73`,
  `:103`) lance `pdfdetach` (ou pdftk). API par chemin.
- Validation hors ligne : `CiiSchemaValidator`
  (`src/Validators/CiiSchemaValidator.php:63`, XSD CII D16B) puis
  `DocumentRulesValidator` (`src/Validators/DocumentRulesValidator.php:36`) :
  BR-29, BR-30, BR-52, BR-CL-24, BR-CO-19, BR-CO-20, BR-DE-22 et des
  contrôles TK-* du toolkit. Le schematron EN 16931 / XRechnung passe par
  `KositValidator` (`src/Validators/KositValidator.php:95`, `Java::execute`
  ligne 160, commande dans `src/Helper/KositCommandHelper.php:22-56`) : le
  README le dit (`README.md:436`, `:487` : XSLT 2.0, donc Java). Non mesuré.
- Pas d'injection dans un PDF hôte (le toolkit dessine son propre PDF).
  La librairie écrit sur stderr des erreurs de configuration (ocrmypdf
  absent) au chargement : sans effet sur les résultats.

### john-wink/en16931-php 0.4.1

MIT, dernière version 2026-10-16, 1 292 au total, 579 le mois dernier.
Portage natif en PHP de la chaîne de validation KoSIT, sans Java ni XSLT.

- `En16931Validator::xrechnung()` ou `::en16931()`
  (`john-wink/en16931-php/src/En16931Validator.php:51`, `:59`), `validate`
  (`:105`) : XSD CII D16B (`src/Syntax/SchemaValidator.php:65`), puis
  1276 règles de syntaxe (CII-SR/DT, UBL-CR/SR) lues dans
  `resources/syntax-rules.json`, puis 274 règles métier EN 16931 et CIUS
  XRechnung codées en PHP (`COVERAGE.md:15-16` du paquet). Référence
  annoncée : validator-configuration-xrechnung 3.0.2.
- Écarts constatés : BR-CL-25 en avertissement, CII-SR-465 absent (voir le
  relevé).
- Lecture `CiiInvoiceReader::read` (et `UblInvoiceReader`) ; ni écriture ni
  PDF.

### invoicepress/vampire-php 1.3.0

GPL-2.0-or-later, dernière version 2026-10-26, 377 au total, 373 le mois
dernier.

- Les schematrons officiels précompilés en JSON (`rulesets/` : EN16931-CII
  et UBL **1.3.17**, XRechnung CII et UBL **3.0.2**, Factur-X **1.09.2** des
  cinq profils, Peppol EN16931 UBL **3.0.21**, CIUS-RO, CIUS-HR…, 120 Mo),
  exécutés par un interprète XPath 2.0 écrit en PHP (`README.md:7`), après le
  XSD.
- Le scénario est choisi comme le fait le validateur KoSIT
  (`rulesets/scenarios.json`, 22 scénarios ; XRechnung CII avec les niveaux
  personnalisés, dont CII-SR-465 en erreur). `Validator::validateString`
  (`src/Validator.php:123`) ; erreurs = messages qui bloquent l'acceptation
  (`src/Result/Result.php:248`).
- Validation seule. Le plus lent des validateurs complets (66 à 107 ms par
  facture du banc).

### intragile/php-einvoicing 1.0.2

MIT (règles : rules-en16931 EUPL-1.2, rules-xrechnung et rules-facturx
Apache-2.0), dernière version 2026-11-17, 69 au total, 69 le mois dernier
(paquet publié en novembre 2026).

- Modèle EN 16931, lecteurs et écrivains CII et UBL : `CiiReader::read`
  (`intragile/php-einvoicing/src/Syntax/Cii/CiiReader.php:58`),
  `CiiWriter::write` (`src/Syntax/Cii/CiiWriter.php:74`).
- PDF écrit sans FPDI : `FacturXPdfReader::read`
  (`src/Syntax/FacturX/FacturXPdfReader.php:60`) ; `FacturXPdfWriter::embed`
  (`src/Syntax/FacturX/FacturXPdfWriter.php:68`) par mise à jour
  incrémentale, sans conversion PDF/A, qui refuse un PDF déclarant déjà une
  facture (`refuseExistingInvoice`, ligne 172).
- Validation : schematrons officiels précompilés en PHP (paquets
  `intragile/einvoicing-rules-*` : CEN 1.3.16 et 1.3.17, XRechnung 2.6.0,
  Factur-X 1.09.2 des cinq profils), évaluateur XPath 2.0 maison.
  `XRechnungValidator` reprend les scénarios KoSIT
  (`src/Validation/XRechnung/XRechnungValidator.php:110-130`, CEN `1.3.16`
  ligne 117, table `scenarios-2026-08-31`) ; `FacturXValidator`
  (`src/Validation/FacturX/FacturXValidator.php:72`, `:102`) pour les autres
  profils Factur-X.
- Les règles Peppol BIS se téléchargent depuis
  `raw.githubusercontent.com/OpenPEPPOL/peppol-bis-invoice-3/3.0.22`
  (`src/Validation/Peppol/PeppolSources.php:18`) : non installées, non
  mesurées.

### dealerweb/einvoice 1.2.3

MIT, dernière version 2026-10-30, 200 au total, 148 le mois dernier.

- Modèle EN 16931 lu par la transformation KoSIT syntaxe → modèle compilée
  en PHP : `Invoice::fromXml` ; `Invoice::xmlFromPdf`
  (`dealerweb/einvoice/src/Model/Behavior/InvoiceBehavior.php:78`) ;
  écriture de tous les profils, CII et UBL.
- Validation : XSLT officiels compilés en PHP depuis le plan d'exécution de
  Saxon (`README.md:426` : CEN 1.3.16, XRechnung 3.0.2, FeRD 1.09.2), avec
  les scénarios et niveaux du validateur KoSIT (`README.md:358`).
  `Validator` a `frenchRules = false` par défaut
  (`src/Validation/Validator.php:49`) : les règles BR-FR (FNFE-MPE France_RFE
  1.4.0.04, `resources/france/SOURCE.md`) ne tournent pas. Règles Peppol
  BIS 3.0 réécrites pour UBL et CII (`resources/peppol/SOURCE.md`).
- `toXml()` et `toPdf()` (`InvoiceBehavior.php:137`, `:155`) revalident leur
  sortie : l'adaptateur passe par `Generator` avec `validate: false`
  (`src/Generation/Generator.php:89`) pour que `Write_*` ne mesure pas une
  validation.
- Couche PDF propre (xref classiques et flux), le document est réécrit en
  une révision ; pas de contrôle PDF/A (`README.md:524`).

### quittio/core 1.1.0 (+ quittio/country-de 1.1.0)

MIT, dernière version 2026-11-17, 361 au total, 319 le mois dernier
(country-de : 320 et 301). Cadre de facturation complet : modèle,
calcul, lecteurs et écrivains CII et UBL, rendu ZUGFeRD/Factur-X avec mPDF.

- `Quittio::create` assemble les validateurs (`quittio/core/src/Quittio.php:183-240`) :
  `PhpRulesDriver` (`:210`, niveaux L1 à L3 et L7), `XsdValidator` (`:215`,
  XSD CII D16B, `src/Validation/Drivers/XsdValidator.php:113`),
  `KositValidator` (`:219`, seulement si configuré : `java -jar`,
  `src/Validation/Drivers/KositValidator.php:145`) et `VeraPdfValidator`
  (`:231`, Docker ou binaire). Niveaux décrits dans
  `src/Validation/ValidationLevel.php:12-25`.
- Règles en PHP : 68 identifiants BR dans le cœur, 23 BR-DE dans
  country-de (`quittio/country-de/src/Rules/XRechnungRules.php`), plus des
  contrôles de plausibilité (IBAN, identifiants de TVA, Leitweg-ID) et du
  droit allemand (§ 14 UStG).
- Extraction `PdfEmbeddedFileExtractor::extractFile`
  (`src/Pdf/PdfEmbeddedFileExtractor.php:72`) ; injection
  `MpdfPostProcessor::process` (`src/Pdf/PostProcessing/MpdfPostProcessor.php:34`),
  étape du rendu qui réimporte les pages par FPDI dans mPDF.

### pnscripts/pn-invoice 0.2.1

MIT, dernière version 2026-10-16, 187 au total, 184 le mois dernier.

- Modèle EN 16931 et écrivains CII et UBL ; pas de lecteur XML
  (`README.md:168`) : `Write_Small` part d'un modèle construit à la main
  d'après `EN16931_Einfach`, `Write_Lines` est `unsupported`.
- `InvoiceValidator` (`src/Validation/InvoiceValidator.php:28`, `:36`) :
  XSD (UBL 2.1, CII D16B) puis 104 règles métier écrites en PHP
  (`README.md:114` ; ni BR-CL, ni BR-DEC, ni règles de syntaxe). La couche
  schematron est une commande externe (Saxon sur Java, `README.md:138`) :
  non branchée.
- Pas de PDF (annoncé pour un paquet commercial).

### pat-o-dev/factur-x 0.8.0

MIT, dernière version 2026-07-31, 84 au total, 1 le mois dernier.
Générateur Factur-X EN 16931 orienté France.

- Écriture `FacturXGenerator::generateXml` depuis son modèle, sans lecteur :
  `Write_Small` part d'un modèle construit à la main (unité C62, H87
  n'existant pas dans son énumération ; note sans sujet REG).
- `InvoiceValidator` : sélection de règles « choisies à la main »
  (structure, BR-CO-15, BR-CO-25, BR-CO-5-6) et règles françaises
  BR-FR-MAP-12/14 (`src/Validation/InvoiceValidator.php:118`, `:123`), non
  comptées ici. Ni XSD ni schematron.
- PDF : xref classique seulement (`README.md:31`) ; `PdfA3Attacher::attach`
  (`src/Pdf/PdfA3Attacher.php:35`) par mise à jour incrémentale, qui refuse
  un catalogue portant déjà /AF ou /Names ; `EmbeddedXmlExtractor::extract`
  (`src/Pdf/EmbeddedXmlExtractor.php:27`).

### stann/factur-x 2.0.0

Aucune licence déclarée (champ vide sur Packagist), dernière version
2025-12-23, 6 078 au total, 564 le mois dernier. Générateur Factur-X pour
Chorus Pro.

- Modèle d'en-tête seulement (`stann/factur-x/src/CrossIndustryInvoice.php:13-22` :
  ni lignes ni ventilation de TVA) : `Write_*` non mesuré.
- `FacturX::validate` (`src/FacturX.php:57`) : XSD Factur-X **1.0.06**
  (`src/Schema.php:9-13`). La librairie laisse le tampon d'erreurs libxml
  rempli quand elle lève une exception : l'adaptateur le vide avant chaque
  appel, sinon les erreurs d'une facture compteraient pour les suivantes.
- `FacturX::createFile` (`src/FacturX.php:20`) : mPDF en PDF/A-3B (`:26`),
  pages réimportées par FPDI ; mPDF écrit son cache dans
  `vendor/mpdf/mpdf/tmp` (pas d'option `tempDir`).

### smalot/pdfparser 2.13.1 (plancher d'extraction)

LGPL-3.0, dernière version 2026-10-13, 60 612 162 au total, 6 173 943 le
mois dernier. Analyseur PDF généraliste, utilisé par horstoeko/zugferd,
atgp et tiime pour extraire. Plancher écrit ici (`adapters/smalot-pdfparser.php`) :
`Parser::parseContent`, puis le premier /Filespec nommé `factur-x.xml`,
`zugferd-invoice.xml` ou `xrechnung.xml`, flux /EF /F. 104/104.

### ext-dom (plancher de validation XSD)

Extension de PHP 8.6.0 (libxml2 2.9.13). Plancher écrit ici
(`adapters/ext-dom.php`) : `DOMDocument::loadXML` puis `schemaValidate` avec
le XSD Factur-X 1.09.2 du profil, fichiers pris dans
`vendor/atgp/factur-x/xsd` (le code PHP d'atgp n'est pas appelé).

## Statut des benchmarks

Essai `COUNT=1 BENCHTIME=0.05 ./run.sh bench` (19 librairies × 16
benchmarks, 304 lignes ; les durées ne sont pas à retenir) :

| Librairie | ok | unsupported | fail |
|---|---|---|---|
| horstoeko/zugferd | 13 | CheckPDF ×2 | Embed_Small (FPDI) |
| horstoeko/invoicesuite | 12 | CheckPDF ×2 | Extract_Large (prinsfrank : `gzuncompress`), Embed_Small (FPDI) |
| atgp/factur-x | 7 | Parse ×3, Write ×2, CheckPDF ×2 | ParseValidate_Large (profil XRechnung refusé), Embed_Small (FPDI) |
| tiime/factur-x | 6 | Parse ×3, Write ×2, CheckPDF ×2 | Extract_Extended (pièce jointe `zugferd-invoice.xml`), ParseValidate_Large (profil XRechnung refusé), Embed_Small (FPDI) |
| tiime/cross-industry-invoice | 5 | Extract ×4, ParseValidate ×3, Embed ×2, CheckPDF ×2 | — |
| easybill/zugferd-php | 8 | Extract ×4, Embed ×2, CheckPDF ×2 | — |
| easybill/e-invoicing | 5 | Extract ×4, ParseValidate ×3, Embed ×2, CheckPDF ×2 | — |
| mahdiabderraouf/facturx-php | 8 | Parse ×3, Write_Lines, CheckPDF ×2 | ParseValidate_Large (profil XRechnung refusé), Embed_Small (FPDI) |
| php-erechnung-toolkit | 12 | Embed ×2, CheckPDF ×2 | — |
| john-wink/en16931-php | 6 | Extract ×4, Write ×2, Embed ×2, CheckPDF ×2 | — |
| invoicepress/vampire-php | 3 (ParseValidate) | 13 | — |
| intragile/php-einvoicing | 12 | CheckPDF ×2 | Embed_Small, EmbedClassic_Small (`InvoiceAlreadyEmbedded`) |
| dealerweb/einvoice | 14 | CheckPDF ×2 | — |
| quittio/core | 13 | CheckPDF ×2 | Embed_Small (FPDI par mPDF) |
| pnscripts/pn-invoice | 4 (ParseValidate ×3, Write_Small) | Extract ×4, Parse ×3, Write_Lines, Embed ×2, CheckPDF ×2 | — |
| pat-o-dev/factur-x | 4 (ParseValidate ×3, Write_Small) | Parse ×3, Write_Lines, CheckPDF ×2 | Extract ×4 (renvois /Names ou /AF non résolus), Embed_Small (flux de xref), EmbedClassic_Small (catalogue avec /AF) |
| stann/factur-x | 4 (ParseValidate ×3, EmbedClassic_Small) | Extract ×4, Parse ×3, Write ×2, CheckPDF ×2 | Embed_Small (FPDI par mPDF) |
| smalot/pdfparser | 4 (Extract) | 12 | — |
| ext-dom | 3 (ParseValidate) | 13 | — |

Aucun `CheckPDF` natif hors ligne en PHP : les seuls contrôles de conteneur
trouvés lancent veraPDF (Java, Docker ou binaire externe).

Points à connaître pour lire les chiffres :

- **FPDI et les flux de xref** : `host.pdf` (`Embed_Small`) a une table de
  références en flux (PDF 1.5) ; l'analyseur gratuit livré avec FPDI ne la
  lit pas (« compression technique which is not supported by the free
  parser »). Tous les injecteurs à base de FPDI ou de mPDF (horstoeko,
  invoicesuite, atgp, tiime, mahdiabderraouf, quittio, stann) échouent sur
  `Embed_Small` et réussissent sur `host_classic.pdf`. Seul dealerweb (sa
  couche PDF) réussit les deux.
- **Hôtes avec /AF résiduel** : `host.pdf` et `host_classic.pdf` gardent
  dans leur catalogue `/AF` vers un Filespec `factur-x.xml` (pdfcpu
  `RemoveAttachments` n'a retiré que l'arbre des noms). intragile le voit et
  refuse d'ajouter une seconde facture (`InvoiceAlreadyEmbedded`, choix de
  la librairie) ; pat-o-dev refuse un catalogue qui a déjà /AF. Les
  injecteurs qui réécrivent le document ne s'en aperçoivent pas. À corriger
  dans `../../ecosystem` (hors de ce banc) si l'on veut mesurer ces deux
  librairies.
- `Embed_*` est relu avec la librairie elle-même quand elle sait extraire,
  sinon avec horstoeko/zugferd (`note: "relu avec horstoeko/zugferd"`, cas
  de stann).
- `Write_*` part du modèle produit par le parseur de la librairie (lecture
  hors mesure). Sans parseur, `Write_Small` part d'un modèle construit à la
  main d'après `EN16931_Einfach` (`note: "modèle construit à la main"`) et
  `Write_Lines` est `unsupported` : pn-invoice, pat-o-dev, et
  mahdiabderraouf (écrit au profil BASIC, le générateur n'ayant pas
  EN16931). Ces trois `Write_Small` ne sont pas comparables aux autres.
- dealerweb `Write_*` et `Embed_*` passent `validate: false` à `Generator` ;
  avec `toXml()` ou `toPdf()`, la sortie serait revalidée.
- mahdiabderraouf `EmbedClassic_Small` comprend la validation XSD que
  `Generator::generate` fait avant d'injecter (comportement natif).
- invoicesuite `Extract_*` comprend la désérialisation du XML extrait (le
  lecteur PDF la fait toujours).
- atgp et tiime `EmbedClassic_Small` gardent leurs options par défaut
  (validation XSD du XML incluse).
- mahdiabderraouf et php-erechnung-toolkit `Extract_*` lancent `pdfdetach`
  (deux processus par appel, environ 25 à 40 ms) : leur temps est surtout
  celui du lancement de processus. Leurs API prennent un chemin : le
  harnais donne le chemin de l'échantillon, et pour la relecture d'`Embed`
  un fichier temporaire de `FX_TMP`.
- vampire-php `ParseValidate_*` : 66 à 107 ms par facture (interprète
  XPath 2.0 en PHP), contre 8 à 16 ms pour intragile et dealerweb, qui
  vérifient les mêmes règles.
- stann : le tampon libxml est vidé par l'adaptateur avant chaque
  validation (voir la fiche) ; mPDF écrit son cache dans `vendor/`.
- quittio et mPDF : le cache de mPDF et les fichiers temporaires de quittio
  vont dans `FX_TMP`.

## Fiches : librairies écartées

| Paquet | Raison |
|---|---|
| josemmo/einvoicing 0.3.1 | UBL seulement (pas de CII) ; 575 090 au total, 67 004/mois |
| num-num/ubl-invoice 2.4.1, cleverit, invoiceninja/ubl_invoice | UBL seulement ; num-num : 1 188 255 au total, 75 659/mois |
| doards/xrechnung-validator 1.0.1 | exige l'extension SaxonC pour PHP (qui ne compile pas sous PHP 8.6), UBL et Peppol seulement ; 60 au total |
| silarhi/zugferd-factur-x 2.0.0, pyrexx/zugferd-php 1.0.6 | fourches anciennes d'easybill/zugferd-php (2024-02 et 2019-06) |
| ephenyxdigital/einvoice 2.2.7, charkoulis/einvoicing 1.0.0 | fourches de josemmo/einvoicing ; 44 et 6 téléchargements au total |
| code4business/zugferd-php, einfacharchiv/zugferd, lianhua/phacturx, inwendo/zugferd | abandonnés (marqués sur Packagist), PHP 7 |
| kinulab/facturx 1.0.13, tualo/zugferd 1.0.44, youniwemi/digital-invoice 0.4.3 | enveloppes minces des librairies mesurées : kinulab d'atgp/factur-x ^1.0, tualo d'easybill/zugferd-php, youniwemi d'easybill/zugferd-php, atgp/factur-x et josemmo/einvoicing (branche dev-master) |
| horstoeko/zugferd-laravel, intragile/laravel-zugferd, quittio/quittio, tmonier/sylius-facturx-plugin | intégrations Laravel ou Sylius des librairies mesurées |
| horstoeko/zugferdublbridge, horstoeko/zugferdvisualizer, horstoeko/zugferdmail | conversion CII ↔ UBL, rendu HTML/PDF, lecture de courriels : hors du protocole |
| horstoeko/ubl 1.0.2 | ancien, PHP 7, dernière version 2021-02 |
| invoicepress/vampire-zugferd, invoicepress/vampire-invoicesuite | ponts entre vampire-php et horstoeko (doublons des mesures) |
| vinelabs-de/xrechnung-kit, vineethkrishnan/xrechnung-kit-core 2.1.0 | générateur de gabarits XRechnung UBL + XSD ; 100 et 16 au total |
| d3/xrechnungproject | application (module OXID), GPL-3.0, 22 téléchargements |
| invoiceninja/einvoice 1.1.0 | schémas sous licence Elastic-2.0, modèle généré pour Invoice Ninja |
| tiime/en-16931 0.10.5 | modèle sémantique EN 16931 sans XML, dépendance de tiime/cross-industry-invoice et tiime/factur-x, mesuré à travers elles |
| scell/sdk, logalysm/valide-facture, factpulse/sdk, xmlvalid, ventnet/xinvoice-client, einvoicing/sdk, invoicexml/*, facturino, peppol-sh, selectpdf, gotenberg, e-invoice/e-invoice-api | clients d'API distantes ou de conteneurs : la validation ou la génération se fait ailleurs |
| genkgo/xsl 1.7.1 | transpileur XSLT 2.0 vers XSLT 1.0 générique (pas une librairie Factur-X), déclare `php ~8.3 \|\| ~8.4 \|\| ~8.5`, non essayé |
| milo/schematron 1.0.1 | moteur Schematron générique en XPath 1.0 (dernière version 2014-04) : ne peut pas lire les schematrons officiels (XPath 2.0) |
| jdomenechb/xslt2processor | moteur XSLT 2.0 partiel, abandonné |
| Extension SaxonC-HE 13.1.1 | seul moteur XSLT 2.0 natif pour PHP, ne compile pas sous PHP 8.6 (voir plus haut) |

## Couverture de la norme

| Librairie | Version | Licence | Dernière version | Téléchargements | Lecture CII | Écriture CII | UBL | Extraction PDF | Injection PDF | XSD | EN 16931 (CII) | Profils Factur-X (MINIMUM, BASIC WL, EXTENDED) | BR-FR | BR-DE (XRechnung) | Peppol BIS 3 | Contrôle du conteneur PDF/A-3 | Dépendances externes |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| horstoeko/zugferd | 1.0.135 | MIT | 2026-11-08 | 10 006 092 ; 1 252 800/mois | oui | oui | non (pont zugferdublbridge à part) | oui (104/104) | oui (FPDI, XMP PDF/A-3B ; pas de flux de xref) | oui (Factur-X, 5 profils ; XRechnung sur EN16931) | non hors ligne (KoSIT par Java) | XSD des 5 profils ; schematron livré, jamais exécuté | non | KoSIT par Java seulement | non | veraPDF par Java seulement | smalot/pdfparser, setasign/fpdi, jms/serializer, symfony/validator ; Java pour KoSIT et veraPDF (téléchargés) |
| horstoeko/invoicesuite | 0.0.44 | MIT | 2026-11-20 | 21 563 ; 15 658/mois | oui | oui | oui (XRechnung UBL, Peppol, autres formats) | partiel (88/104) | oui (FPDI/TCPDF ; pas de flux de xref) | oui (XSD du format reconnu) | non hors ligne (KoSIT par Java, Docuflair distant) | XSD | non | KoSIT par Java seulement | non | non | prinsfrank/pdfparser, setasign/fpdi, mpdf, jms/serializer ; environnement Composer séparé |
| atgp/factur-x | 3.5.0 | MIT | 2026-08-18 | 1 122 993 ; 61 110/mois | non | non | non | oui (100/104 ; pas `xrechnung.xml`) | oui (FPDI, XMP PDF/A-3 ; pas de flux de xref) | oui (Factur-X 1.09.2, 5 profils) | non | XSD des 5 profils (XRechnung refusé) | non | non | non | non | smalot/pdfparser, setasign/fpdi |
| tiime/factur-x | 0.3.2 | MIT | 2026-10-07 | 280 508 ; 4 784/mois | non | non | non | partiel (70/104 : `factur-x.xml` seulement) | oui (FPDI ; pas de flux de xref) | oui (Factur-X 1.07.3) | non (schematron livré, jamais appelé) | XSD des 5 profils (XRechnung refusé) | non | non | non | non | smalot/pdfparser, setasign/fpdi, tiime/en-16931 |
| tiime/cross-industry-invoice | 0.3.11.1 | MIT | 2025-10-27 | 269 684 ; 808/mois | oui (23/26) | oui | non | non | non | non (XSD des tests seulement) | non | modèles MINIMUM, BASIC WL, BASIC, EN 16931 ; pas EXTENDED | modèle « Flux1 », sans règles | non | non | non | tiime/en-16931, twig |
| easybill/zugferd-php | 6.0.0 | MIT | 2026-03-30 | 407 329 ; 22 919/mois | oui | oui | non | non | non | oui (Factur-X 1.08.0, schéma choisi par l'appelant) | non | XSD des 5 profils ; ZUGFeRD 1 | non | non | non | non | jms/serializer |
| easybill/e-invoicing | 0.4.0 | MIT | 2026-09-07 | 198 338 ; 9 426/mois | oui (24/26) | oui | oui (lecture, écriture) | non | non | non | non | non | non | non | non | non | jms/serializer |
| mahdiabderraouf/facturx-php | 2.4.0 | MIT | 2026-09-11 | 12 121 ; 1 708/mois | non | partiel (MINIMUM, BASIC WL, BASIC) | non | oui (100/104, par `pdfdetach`) | oui (PDF/A-3b par FPDI/tFPDF, XSD avant ; pas de flux de xref) | oui (Factur-X 1.09.2) | non | XSD des 5 profils (XRechnung refusé) | non | non | non | non | setasign/fpdi, fpdf ; `pdfdetach` (poppler) ; déclare `php <8.6` |
| daniel-jorg-schuppelius/php-erechnung-toolkit | 0.23.1 (+ php-pdf-toolkit 0.33.0) | AGPL-3.0-or-later | 2026-11-14 | 7 295 ; 1 468/mois | oui | oui | oui (parseur, générateur, XSD UBL) | oui (104/104, par `pdfdetach`) | non (dessine son propre PDF) | oui (CII D16B, UBL) | partiel hors ligne (BR-29, BR-30, BR-52, BR-CL-24, BR-CO-19/20) ; schematron par KoSIT (Java) | profils MINIMUM à EXTENDED dans le modèle, sans règles propres | non | BR-DE-22 hors ligne ; le reste par KoSIT (Java) | non (client SMP et transport Peppol, réseau) | non | php-pdf-toolkit (AGPL : dompdf, tcpdf, fpdi), `pdfdetach` ; Java pour KoSIT ; déclare `php <8.6` |
| john-wink/en16931-php | 0.4.1 | MIT | 2026-10-16 | 1 292 ; 579/mois | oui | non | oui (lecture, validation) | non | non | oui (CII D16B, UBL 2.1) | oui, réécrit en PHP (274 règles métier, 1276 règles de syntaxe) ; BR-CL-25 en avertissement, CII-SR-465 absent | non | non | oui (CIUS XRechnung en PHP) | non | non | aucune (bcmath) |
| invoicepress/vampire-php | 1.3.0 | GPL-2.0-or-later | 2026-10-26 | 377 ; 373/mois | — | — | oui (validation CEN, XRechnung, Peppol, CIUS-RO, CIUS-HR…) | — | — | oui (XSD du scénario) | oui (schematron CEN 1.3.17 précompilé, XPath 2.0 en PHP) | schematron Factur-X 1.09.2 des 5 profils | non | oui (XRechnung 3.0.2, niveaux KoSIT) | oui (PEPPOL-EN16931 3.0.21, UBL seulement) | non | aucune (bcmath) ; 120 Mo de règles |
| intragile/php-einvoicing | 1.0.2 | MIT ; règles EUPL-1.2 et Apache-2.0 | 2026-11-17 | 69 ; 69/mois | oui | oui | oui (lecture, écriture, schematron CEN UBL) | oui (104/104) | partiel (mise à jour incrémentale sans PDF/A ; refuse un PDF qui déclare déjà une facture) | oui (CII D16B) | oui (schematron CEN 1.3.16 et 1.3.17 précompilés en PHP ; 1.3.16 par les scénarios KoSIT) | schematron Factur-X 1.09.2 des 5 profils | non | oui (XRechnung 2.6.0, niveaux KoSIT 2026-08-31) | non installé (règles OpenPEPPOL 3.0.22 téléchargées depuis GitHub) | non | brick/math, intragile/einvoicing-rules-* |
| dealerweb/einvoice | 1.2.3 | MIT | 2026-10-30 | 200 ; 148/mois | oui | oui (tous profils) | oui (lecture, écriture, validation) | oui (104/104) | oui (couche PDF propre, document réécrit ; xref classique et flux) | oui (CII D16B, UBL) | oui (XSLT CEN 1.3.16 compilés en PHP, scénarios KoSIT) | schematron FeRD 1.09.2 des 5 profils | oui (FNFE-MPE XP Z12-012 1.4.0), option `frenchRules`, éteinte ici | oui (XRechnung 3.0.2, niveaux KoSIT) | oui (règles BIS 3.0.21 réécrites, UBL et CII) | non | dompdf |
| quittio/core | 1.1.0 (+ country-de 1.1.0) | MIT | 2026-11-17 | 361 ; 319/mois (country-de 320 ; 301/mois) | oui | oui | oui (lecture, écriture) | oui (104/104) | oui (mPDF + FPDI ; pas de flux de xref) | oui (CII D16B) | partiel (68 règles BR en PHP, pas de schematron hors ligne ; CII-SR-465 non vu) | profils MINIMUM à EXTENDED | non | partiel (23 BR-DE en PHP ; schematron par KoSIT, Java) | partiel (profil PeppolBis3, quelques règles en PHP) | veraPDF (Docker ou binaire) seulement | mpdf, setasign/fpdi, brick/money, nesbot/carbon, league/commonmark ; Java pour KoSIT |
| pnscripts/pn-invoice | 0.2.1 | MIT | 2026-10-16 | 187 ; 184/mois | non | oui | oui (écriture, XSD UBL 2.1) | non | non | oui (CII D16B, UBL 2.1) | partiel (104 règles BR en PHP ; ni BR-CL, ni BR-DEC, ni syntaxe) | non | non | non | non | non | aucune (bcmath) ; Saxon sur Java pour la couche schematron facultative |
| pat-o-dev/factur-x | 0.8.0 | MIT | 2026-07-31 | 84 ; 1/mois | non | oui (EN 16931) | non | partiel (1/104 : xref classique seulement) | partiel (mise à jour incrémentale ; refuse /AF, /Names et les flux de xref) | non | partiel (structure, BR-CO-15, BR-CO-25, BR-CO-5-6) | EN 16931 seulement | partiel (BR-FR-MAP-12/14, non comptées) | non | non | non | aucune |
| stann/factur-x | 2.0.0 | aucune déclarée | 2025-12-23 | 6 078 ; 564/mois | non | partiel (en-tête seulement) | non | non | oui (mPDF + FPDI, PDF/A-3B ; pas de flux de xref) | oui (Factur-X 1.0.06, 5 profils) | non | XSD des 5 profils | non | non | non | non | mpdf (cache dans `vendor/`) |
| smalot/pdfparser (plancher) | 2.13.1 | LGPL-3.0 | 2026-10-13 | 60 612 162 ; 6 173 943/mois | — | — | — | oui (104/104) | non | — | — | — | — | — | — | non | symfony/polyfill-mbstring |
| ext-dom (plancher) | PHP 8.6.0, libxml2 2.9.13 | PHP-3.01 | — | — | non | non | non | non | non | oui (XSD Factur-X 1.09.2 d'atgp) | non | XSD des 5 profils | non | non | non | non | aucune |

Téléchargements : API Packagist (`downloads.total` et `downloads.monthly`,
30 derniers jours), relevés le 2026-11-22 ; licences et dates : métadonnées
de la dernière version publiée sur Packagist.
