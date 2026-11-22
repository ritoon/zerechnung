# Couverture : librairies TypeScript / JavaScript

Banc mené selon `../PROTOCOL.md` le 2026-11-17 avec Node v22.15.0. Les versions sont figées dans `package.json` et `package-lock.json`. Les téléchargements hebdomadaires viennent de `api.npmjs.org/downloads/point/last-week`, semaine du 2026-11-09 au 2026-11-15. Les licences et les dernières versions viennent de `npm view`.

Chaque affirmation sur une règle, un artefact, le réseau ou Java a été vérifiée dans le code installé. Les chemins cités sont relatifs à `node_modules/` et correspondent aux versions figées.

## Sommaire

13 librairies sont mesurées. Les colonnes du banc donnent, pour chaque famille d'opérations :
- `ok` : l'opération est mesurée ;
- `—` : la librairie ne sait pas la faire (`unsupported`) ;
- `fail` : la librairie lève une exception ou rend un résultat faux.

Le relevé (`survey_typescript.jsonl`) compte :
- validate : les fichiers valides que la librairie signale, sur 26 (référence des schematrons officiels : 6) ;
- parse : les CII lus, sur 26 ;
- extract : les PDF dont le XML est extrait, sur 104.

| Librairie | Extract ×4 | Parse ×3 | ParseValidate ×3 | Write Small / Lines | Embed ×2 | CheckPDF ×2 | validate | parse | extract |
|---|---|---|---|---|---|---|---|---|---|
| @stackforge-eu/factur-x 1.4.7 | ok | — | ok (XSD seul) | ok (main) / — | ok | — | 0/26 | — | 104/104 |
| @e-invoice-eu/core 3.4.4 | — | — | — | ok (main) / — | — | — | — | — | — |
| node-zugferd 0.1.1-beta.1 | — | — | — | ok (main) / — | — | — | — | — | — |
| @stafyniaksacha/facturx 0.6.1 | ok | ok | ok | ok / ok | **fail** | — | 12/26 | 26/26 | 100/104 |
| factur-x-kit 0.3.2 | — | ok / **fail** ×2 | ok / **fail** ×2 | ok / **fail** | — | — | 18/26 | 13/26 | — |
| @lucaapp/zugferd-ts 0.1.0 | ok | ok ×2 / **fail** (Large) | ok | ok / ok | ok | — | 8/26 | 19/26 | 104/104 |
| @attestwire/en16931 0.14.1 | ok | ok | ok | ok / ok | — | ok | 2/26 | 26/26 | 104/104 |
| @ekwo-ai/factur-x 0.27.0 | ok | ok | ok (8 règles) | ok (main) / — | ok | — | 3/26 | 26/26 | 104/104 |
| @conformo/toolkit 0.2.4 | ok | ok | ok | ok / ok | ok | ok | **6/26** | 26/26 | 104/104 |
| @kontor-mcp/core 1.0.4 | ok | ok | ok | — | — | — | 2/26 | 26/26 | 104/104 |
| @sweberdev/summand 2.0.1 | ok | — | ok | — | — | ok | **6/26** | — | 104/104 |
| en16931 0.1.0 | — | — | ok | — | — | — | **6/26** | — | — |
| @fin.cx/einvoice 11.3.1 | ok | ok | ok | ok / ok | ok (sans XMP) | — | 0/26 | 26/26 | 104/104 |

Trois librairies retrouvent exactement la référence de 6 fichiers signalés : conformo, summand et en16931 (BR-CL-25 deux fois, CII-SR-465 sur les quatre XRECHNUNG). en16931 signale aussi CII-SR-475 et CII-SR-476 sur deux de ces fichiers : il ne lit pas les sévérités.

## Méthode propre au banc TypeScript

- **Un processus par librairie et par benchmark.** `run.sh` lance `node bench.mjs <lib> <Benchmark>` pour chaque couple, si bien que `first_call_ns` est un vrai premier appel à froid : chargement paresseux, compilation des schematrons, JIT. Le relevé tourne dans un processus par librairie (`node survey.mjs <lib>`).
- **Console neutralisée.** Plusieurs librairies écrivent sur la console à chaque appel :
  - @fin.cx/einvoice affiche quatre lignes par extraction ;
  - stackforge avertit de l'absence d'OutputIntent ;
  - node-zugferd signale que `strict` est désactivé.

  La console est donc remplacée par des fonctions vides dans les processus de mesure (`lib/harness.mjs`, `silenceConsole`). Les résultats JSONL partent directement sur stdout.
- **Chaîne ou octets.** Le XML est passé sous la forme que prend l'API de chaque librairie :
  - texte (décodé en UTF-8 hors mesure) pour stafyniaksacha, lucaapp, ekwo, conformo, en16931, fin.cx, stackforge ;
  - octets (`Buffer`) pour attestwire, kontor, summand, factur-x-kit.

  Les PDF sont toujours passés en octets.
- **Asynchrone.** Une fonction qui rend une promesse est attendue dans la boucle (`await`). Le coût d'un tour de micro-tâche est donc inclus, comme il le serait pour un appelant réel.
- **Modèles à la main.** `Write_*` part du modèle produit par le parseur de la librairie quand il existe. Quatre librairies n'ont pas de lecteur, ou ont un lecteur dont le modèle ne s'écrit pas : stackforge, e-invoice-eu, node-zugferd et ekwo. Pour elles, le modèle de `EN16931_Einfach` est écrit à la main dans `lib/handmodels.mjs`, avec la note « modèle construit à la main ».
  - Seul `Write_Small` est mesuré ; `Write_Lines` est `unsupported`.
  - Les quatre sorties ont été passées dans summand : 0 erreur au XSD et au schematron CEN.
- **Relecture d'`Embed`.** Le PDF produit est relu avec l'extracteur de la librairie elle-même. Toutes les librairies qui injectent en ont un.
- **SaxonJS.** conformo, kontor et stafyniaksacha exécutent des schematrons compilés en SEF avec `saxon-js` 2.7, sous licence propriétaire Saxonica : gratuite, mais non libre. C'est la seule dépendance de ce type.
- **Scripts d'installation.** `npm ci --ignore-scripts`, également imposé par `.npmrc` (qui fixe aussi `legacy-peer-deps`, avec lequel le lockfile a été construit). Aucun paquet mesuré n'a besoin d'un script d'installation. Deux sont bloqués :
  - le `postinstall` de @fin.cx/einvoice, qui télécharge des schematrons depuis les branches `master` de GitHub (voir sa fiche) ;
  - celui de `xsd-schema-validator`, dépendance optionnelle de node-zugferd, qui compile une classe Java avec `javac`.

## Fiches des librairies mesurées

### @stackforge-eu/factur-x 1.4.7 (EUPL-1.2, 94 466/semaine)

Génération de XML Factur-X / ZUGFeRD / XRechnung à partir de sa propre saisie (`FacturXInvoiceInput`, `buildXml`). Elle injecte aussi en PDF/A-3 et extrait le XML, avec `pdf-lib`. Elle n'a pas de lecteur CII.

- **Validation.** `validateXsd(xml, Profile.EN16931)` : XSD seul, exécuté par `libxml2-wasm`. Le schéma est relu et recompilé à chaque appel (`@stackforge-eu/factur-x/dist/index.js:1139`). Les XSD sont ceux de Factur-X 1.09 / ZUGFeRD 2.5 pour les cinq profils (`@stackforge-eu/factur-x/schema/README.md`).
  - Pas de schematron.
  - `validateInput` contrôle la saisie, pas un XML.
- **Injection.** `embedFacturX({ pdf, xml, profile })` ajoute la pièce jointe, l'AFRelationship et le XMP PDF/A-3b. Elle n'ajoute d'OutputIntent que si l'on fournit un profil ICC (`rgbIccProfile`) ; sinon elle avertit que le PDF ne sera pas conforme PDF/A-3 (`@stackforge-eu/factur-x/dist/index.js:1473`). Le banc garde les options par défaut.
- **Réseau et Java.** Aucun appel réseau, pas de Java.

### @e-invoice-eu/core 3.4.4 (WTFPL, 63 013/semaine)

Génération seulement : un JSON calqué sur UBL (contrôlé par un schéma ajv) produit du UBL, du CII, de l'XRechnung ou du Factur-X.

- **Lecture et validation.** Aucun lecteur XML, aucune validation XML.
- **Factur-X.** Les formats `Factur-X-*` attachent le XML généré à un PDF fourni. À défaut de PDF, ils lancent LibreOffice (`spawn`, `@e-invoice-eu/core/dist/e-invoice-eu.esm.js:7103`).
- **Mesure.** `Write_Small` part d'un JSON écrit à la main, et le contrôle ajv est compris dans l'appel. `InvoiceService.generate` est lent : environ 50 ms par appel, contre 15 µs pour stackforge.

### node-zugferd 0.1.1-beta.1 (MIT, 62 514/semaine)

Génération par profil (MINIMUM, BASIC WL, BASIC, EN16931, EXTENDED) à partir d'une saisie typée. `embedInPdf` n'attache que le XML que la librairie vient de générer.

- **Validation.** En mode `strict`, seule la XSD Factur-X 1.07.3 (`node-zugferd/dist/Factur-X_1.07.3_*.xsd`) est contrôlée, par `xsd-schema-validator`, qui exige Java (`node-zugferd/dist/profile.js:40`).
  - Java n'étant pas installé, la mesure se fait avec `strict: false`.
  - Pas de schematron.
- **Versions.** Le tag `latest` npm est 0.1.1-beta.1, marqué déprécié au profit de 1.0.0-beta.1 (parue le 2026-11-16 sous le tag `beta`, non mesurée).

### @stafyniaksacha/facturx 0.6.1 (MIT, 7 339/semaine)

Portage en TypeScript d'akretion/factur-x (Python). Elle lit et écrit le CII vers un modèle typé (`xmlToInvoice` / `invoiceToXml`), extrait et injecte avec `pdf-lib`. Elle prend aussi en charge Order-X.

- **Validation.** `check({ xml, level, schematron: true })` exécute :
  - la XSD Factur-X 1.09 du profil, par `libxml2-wasm` ;
  - le schematron officiel Factur-X 1.09 du profil, compilé en SEF et exécuté par SaxonJS.

  Il existe un SEF par profil, de MINIMUM à EXTENDED (`@stafyniaksacha/facturx/dist/shared/xsd/facturx/<profil>/FACTUR-X_<PROFIL>.sef.json.gz`).
  - Ce schematron est celui du paquet Factur-X : il reprend les règles CEN et y ajoute `PEPPOL-EN16931-R008` (éléments vides) et la liste de codes `FACTUR-X_EN16931_codedb.xml`.
  - Les résultats ne portent aucune sévérité : toute assertion en échec compte. D'où 12/26 au relevé :
    - R008 sur 6 fichiers ;
    - la liste de codes sur 6 fichiers : schéma d'adresse électronique (`URIID`, l'équivalent de BR-CL-25) sur 2, BT-24 XRechnung absent de la liste Factur-X sur les 4 XRECHNUNG, en plus de CII-SR-465.
  - Le niveau est forcé à `en16931`, car la détection automatique lève « Unknown level: kosit » sur les XRECHNUNG.
- **Injection.** `generate({ pdf, xml })` échoue sur les deux hôtes : « Not implemented yet: Document ID already set ». L'erreur est levée dès que la remorque du PDF hôte a un `/ID` (`@stafyniaksacha/facturx/dist/shared/facturx.BQpPzQjc.mjs:184`), ce qui est le cas de la plupart des PDF réels.
- **Extraction.** Ne reconnaît que `factur-x.xml` et `zugferd-invoice.xml`, d'où l'échec sur les 4 PDF XRECHNUNG (`xrechnung.xml`).

### factur-x-kit 0.3.2 (MIT, 5 067/semaine)

Lecture et écriture CII vers un modèle zod, pour les profils MINIMUM, BASIC WL, BASIC et EN16931. EXTENDED est replié sur EN16931 ; il n'y a pas de profil XRechnung.

- **Validation.** `validate()` applique le schéma zod du profil et les règles BR réécrites en TypeScript : 131 identifiants `BR-*` distincts sont cités dans `factur-x-kit/dist/index.js`, plus des règles maison `BR-OWN-*`. Pas de XSD ni de schematron.
- **Lecture.** Le lecteur échoue sur 13 des 26 CII :
  - « Received Invalid XML Input » sur 5 fichiers, dont `EN16931_Rabatte` (XML Lines) ;
  - « Received Invalid Data Value » sur 2 fichiers ;
  - un problème de structure des données lues sur 2 fichiers ;
  - « Profile not yet implemented » sur les 4 XRECHNUNG.
- **Extraction et injection.** `FacturXPdf` n'est pas exporté : `FacturX.fromPDF` enchaîne extraction et lecture (et ne cherche que `factur-x.xml`). `getPDF` réécrit le XML depuis le modèle. Extract et Embed sont donc `unsupported`.
- **Réseau.** Le seul `fetch` sert à décoder une URL `data:` d'image d'en-tête (`factur-x-kit/dist/index.js:5106`).

### @lucaapp/zugferd-ts 0.1.0 (Apache-2.0, 4 395/semaine)

TypeScript pur : génération, injection PDF/A-3b, lecture CII et UBL, validation.

- **Validation.** `validateXml` contrôle deux choses :
  - la XSD Factur-X 1.08 du profil, par `xmllint-wasm` (`@lucaapp/zugferd-ts/schemas/cii/Factur-X_1.08_*.xsd`) ;
  - le schematron CEN EN 16931 CII 1.3.16, daté du 2026-04-10. Ses 806 `assert` (`@lucaapp/zugferd-ts/schemas/schematron/en16931-cii.sch`) sont exécutés par `node-schematron` (fontoxpath).

  Points à connaître :
  - La librairie classe toute assertion en échec en `error`, sans lire les `flag="warning"` du CEN (`@lucaapp/zugferd-ts/dist/index.js:5828`). Les CII-SR-465/475/476 comptent donc comme des erreurs.
  - Faux positifs propres au moteur : BR-CO-16 sur les deux Betriebskostenabrechnung, BR-S-08 sur Rabatte. Ni en16931 (même schematron, décimaux exacts) ni Saxon ne les lèvent.
  - Lenteur : 3,5 à 10 s par validation. Le schéma est compilé une seule fois (`getSchema`, en cache), c'est l'évaluation qui est lente.
  - UBL : seulement la XSD UBL 2.1 (`validateUblXml`). Pas de schematron KoSIT (`serializeCiiXrechnung` écrit l'XRechnung sans la valider).
- **Contrôle du conteneur.** `validatePdfA` applique 21 règles (`PDFA_RULES`) au conteneur seul, sans le XML, d'où CheckPDF `unsupported`.
- **Lecture.** Échoue sur 7 CII dont le type de document (BT-3) est 204, 387, 575 ou 877 (`@lucaapp/zugferd-ts/dist/index.js:4740`), dont XRECHNUNG_Elektron (Parse_Large).

### @attestwire/en16931 0.14.1 (MIT, 2 538/semaine)

Aucune dépendance : parseur XML, lecteur PDF et décodeur DEFLATE maison. Elle lit le CII et l'UBL, écrit le CII (`generateCii`) et ne sait pas écrire de PDF.

- **Règles.** Réécrites à la main en TypeScript, sans schematron ni XSD. Le README annonce « 294 rules » (`@attestwire/en16931/README.md:75`). Identifiants distincts relevés dans `dist/` :
  - EN 16931 : 58 BR-nn, 19 BR-CO, 23 BR-CL, 96 règles de catégorie de TVA, 21 BR-DEC ;
  - XRechnung : 24 BR-DE ;
  - Peppol : 27 PEPPOL-EN16931-R, appliquées seulement à l'UBL (`dist/rules.js:143`) ;
  - 27 règles maison ATW-* (IBAN, BIC, SIREN/SIRET, Leitweg-ID…).
- **PDF hybride.** `validate(pdf)` ajoute six familles de constats `AW-PDF-*` sur le conteneur : pièce jointe, /AF, AFRelationship, type MIME, XMP, profil XMP / BT-24 (`dist/facturx-findings.js`). Ce sont des avertissements, qui ne comptent pas comme erreurs.
- **Mesure.** La validation est rapide (0,4 ms sur Small). Au relevé, 2/26 (BR-CL-25) : elle ne signale pas CII-SR-465, un avertissement CEN.

### @ekwo-ai/factur-x 0.27.0 (MIT, 521/semaine)

Génération CII depuis un objet simple, pour cinq profils. Elle a un lecteur CII (`readCii`), une injection et une extraction avec `pdf-lib` (point d'entrée `/pdf`).

- **Validation.** `readCii(xml).violations` réunit :
  - les anomalies de lecture : dates, IBAN dont la clé est fausse (`@ekwo-ai/factur-x/dist/cii-read.js:161`) ;
  - 8 contrôles arithmétiques BR-CO-10 à BR-CO-17 (`dist/received-checks.js`).

  Au relevé, 3/26 : des IBAN d'exemple à clé invalide.
- **Écriture.** Le modèle lu par `readCii` (`ReceivedInvoice`) n'est pas celui de `generateCiiXml` (`Invoice`). `Write_Small` part donc d'un `Invoice` écrit à la main, qui n'a qu'une note et pas de GTIN d'article.

### @conformo/toolkit 0.2.4 et ses paquets (Apache-2.0, 497/semaine)

Téléchargements des autres paquets : validate 528, parse 345, pdf 160, mcp-server 560. La librairie est répartie ainsi :
- `@conformo/parse` : lecture CII et UBL vers le modèle EN 16931, extraction avec `pdf-lib` et `pdfjs-dist` ;
- `@conformo/validate` : XSD par `xmllint-wasm`, schematrons compilés en SEF et exécutés par SaxonJS ;
- `@conformo/pdf` : finalisation PDF/A-3 ;
- `@conformo/toolkit` : façade, `serialize` CII et UBL, `validate` d'un XML ou d'un PDF.

Le CLI et le serveur MCP ne sont que des enveloppes.

- **Artefacts** (`@conformo/validate/artefacts/rulesets.json`) :
  - CEN EN 16931 1.3.17 (CII et UBL) ;
  - KoSIT validator-configuration-xrechnung 2026-08-31 (XRechnung 3.0.2), avec un correctif `iban-mod97-decimal` ;
  - Peppol BIS Billing 3.0.22 (UBL seulement) ;
  - BR-FR « French CTC Flux 2 », source « factur-x 6.8 (FNFE-MPE) ».
- **XSD** : CII D16B, Factur-X MINIMUM / BASIC WL / EXTENDED, UBL 2.1 (`artefacts/xsd/`).
- **Choix des jeux de règles** (`@conformo/validate/src/engine.js:7`) :
  - EN 16931 sauf pour MINIMUM, BASIC WL et EXTENDED ;
  - XRechnung si BT-24 le déclare ;
  - BR-FR seulement si `country: "FR"`, non fourni ici.
- **Sévérités.** Celles de la configuration KoSIT (`levelOverrides`) : CII-SR-465 y est relevé en `error`, CII-SR-475/476 en `information`. D'où exactement la référence 6/26.
- **Conteneur.** `validate(pdf)` ajoute des notices XMP / nom de fichier / profil, sans effet sur `valid` (`@conformo/toolkit/src/container.js`). Pas de règles PDF/A.
- **Injection.** Recette de `createPdf` appliquée à l'hôte : `finalizePDFA` avec l'ICC sRGB du toolkit et une date fixe.
- **Réseau.** `@conformo/validate/src/page.js` charge les SEF par `fetch`, mais seulement dans le navigateur ; ce code n'est pas utilisé sous Node.

### @kontor-mcp/core 1.0.4 (Apache-2.0, 337/semaine ; @kontor-mcp/rules 554)

Bibliothèque du serveur MCP Kontor, sans MCP : détection, lecture CII et UBL, validation, conversion, génération de PDF.

- **Validation.** XSD CII D16B et UBL 2.1 par `xmllint-wasm`, puis les schematrons compilés en SEF et exécutés par SaxonJS (`@kontor-mcp/rules/artifacts/MANIFEST.json`) :
  - CEN EN 16931 1.3.16 ;
  - XRechnung 3.0.2 (schematron 2.5.0, configuration KoSIT 2026-01-31), avec le correctif D-019 sur BR-DE-19.

  Pas de Peppol ni de BR-FR. La couche maison `plausibility` (22 règles `KONTOR-PLAUS-*`) est exclue du compte, comme la librairie l'exclut elle-même de `valid`.
- **Écriture et injection.** `modelToCii` n'est pas exporté (exports limités à `.`), d'où Write `unsupported`. `generateZugferdPdf` dessine son propre PDF à partir d'une saisie, il n'injecte pas dans un hôte.
- **Mesure.** Au relevé, 2/26 : elle ne relève pas CII-SR-465.

### @sweberdev/summand 2.0.1 (MIT, 323/semaine)

Validateur sans aucune dépendance : validateur XSD, moteur XPath 2.0 et moteur Schematron maison. La lecture et l'écriture sont réservées à « Summand Pro ».

- **Artefacts** (`@sweberdev/summand/NOTICE.md`, `src/rules/`) :
  - CEN 1.3.16 converti en JSON, 806 asserts dans `src/rules/en16931-cii.json` ;
  - XRechnung 2.6.0, 52 asserts dans `src/rules/xrechnung-cii.json` ;
  - XRechnung 2.5.0 fourni, mais pas appliqué par défaut ;
  - XSD CII et UBL compilées (`src/schemas/`).

  Sévérités de la configuration KoSIT : la librairie retrouve exactement la référence 6/26.
- **PDF hybride.** `validateInvoice(pdf)` extrait le XML, compare le profil XMP au BT-24, puis valide le XML. C'est CheckPDF natif ; il n'y a pas de règles PDF/A.

### en16931 0.1.0 (MIT AND EUPL-1.2, 360/semaine)

Exécute les schematrons CEF sans modification, sur `fontoxpath` en décimaux exacts et `slimdom`. Validation seulement, CII et UBL.

- **Artefacts.** Copiés du commit `b6c9e06` de la branche `validation-1.3.16` (`en16931/cef/VENDORED.md`). Les XSD et `codelist/` ont été retirés, puisque le schematron prétraité inclut les listes.
- **Mesure.** Le résultat (`fired`) ne porte pas de sévérité, si bien que les CII-SR-475/476 (avertissements CEN) comptent. La compilation (`new SchematronRunner(loadCiiSchematron())`, environ 170 ms) est faite au premier appel et tombe dans `first_call_ns`.

### @fin.cx/einvoice 11.3.1 (MIT, 323/semaine)

Lecture et écriture CII et UBL (Factur-X, ZUGFeRD, XRechnung, UBL…), avec `@xmldom/xmldom`, `xpath` et `pdf-lib`.

- **Validation.** `EInvoice.validate(BUSINESS, { featureFlags })` n'exécute que des validateurs TypeScript (`@fin.cx/einvoice/dist_ts/einvoice.js:456`). Identifiants relevés dans `dist_ts/` : 65 BR-nn, 23 BR-CO, 26 BR-CL, 84 règles de TVA, 12 BR-DE, 2 PEPPOL.
  - Au relevé, 0/26.
  - Pas de XSD.
- **Couche schematron et réseau.** `IntegratedValidator` est séparé de `validate()` et dépend de fichiers que le `postinstall` (`dist_ts_install/index.js`) télécharge depuis les branches `master` de GitHub, sans version figée (`dist_ts/formats/validation/schematron.downloader.js`). Ce postinstall n'est pas exécuté et cette couche n'est pas mesurée.
- **Injection.** `PDFEmbedder.embedXml` ne fait que `pdf-lib` `attach` (AFRelationship Alternative), sans XMP ni métadonnées PDF/A (`dist_ts/formats/pdf/pdf.embedder.js:25`).
- **Écriture.** Sur XRECHNUNG_Elektron, `exportXml('facturx')` perd la pièce jointe base64 : la sortie fait 6,5 Ko pour 1,3 Mo en entrée.

## Écartées

### Clients d'API distantes ou enveloppes MCP

Toutes ces entrées envoient la facture à un service et ne peuvent pas être mesurées hors ligne.

| Paquet | Téléchargements/semaine | Raison |
|---|---|---|
| @invoicexml/zugferd-xrechnung 1.4.0 | 48 | Client HTTP de `https://api.invoicexml.com` (`dist/index.js:51`). |
| @facturxapi/sdk | 412 | Client d'API. |
| @invowerk-dev/sdk | 67 | Client d'API. |
| zugferd-client | 13 | Client de l'« E-Rechnung API ». |
| e-invoice-api | 2 480 | Client d'API. |
| xrechnung-check | 15 | Client d'API. |
| @normapi/client 0.11.1 | 616 | Valide « via the NormAPI service ». |
| @conformo/cli, @conformo/mcp-server | 419, 560 | Enveloppes de @conformo/toolkit, mesuré. |

### Installés et lus, puis écartés

| Paquet | Téléchargements/semaine | Raison |
|---|---|---|
| zugferd-ts 0.1.0 (MIT) | 776 | Générateur seul, depuis sa propre saisie : CII EXTENDED, injection du XML qu'il génère, règles TS sur la saisie. Pas de lecture, d'extraction ni de validation d'un XML. Il ne fait que doubler stackforge et node-zugferd. |
| factur-x 0.0.2 | 242 | Paquet non construit (`main: src/index.ts`), profil MINIMUM seulement (« unknown profile » sinon), « under development ». |
| @e-rechnung-inbox/xrechnung-core 0.1.1 | 33 | Contrôles structurels maison. Extraction par « simple substring scanner » (README), sans lecteur PDF. |
| @normwerk/einvoice-cii / -pdfa 0.1.0 | 21 / 93 | Sérialiseur et assemblage PDF/A depuis leur propre modèle. Très peu utilisés, et doublent les générateurs mesurés. |
| zugferd-generator 2.1.0 | 613 | Générateur depuis sa propre saisie (pdf-lib). Doublon des générateurs mesurés. |
| pdf-to-einvoice 0.3.0 | 746 | Application (CLI, MCP, navigateur) construite sur @e-invoice-eu/core, mesuré. |

### Non installés : peu utilisés, hors plafond d'une dizaine de librairies

Ces paquets n'ont pas été lus, sauf leur fiche npm. Ils restent à regarder si le banc s'élargit.

| Paquet | Téléchargements/semaine | Raison et remarque |
|---|---|---|
| @jasy/e-invoice 1.0.0-beta.4 | 720 | Générateur CII/UBL + PDF/A-3. |
| @crewhaus/tool-einvoice | 396 | Fichiers financiers à format fixe, pas une librairie Factur-X. |
| e-rechnung-parser 0.1.0 | 283 | Lecteur seul (fast-xml-parser). |
| facturx-sdk 1.10.0 | 177 | Annonce des règles BR-FR intégrées : à vérifier en priorité si l'on cherche du BR-FR hors schematron. |
| xrechnung-generator 2.1.0 | 153 | Générateur UBL XRechnung seul. |
| @aifind/mustangjs 0.4.0 | 142 | Portage de Mustang avec SaxonJS. |
| @itegro/einvoice 0.1.0 | 119 | Génération + schematron SaxonJS + @react-pdf. |
| @financica/facturx 0.2.0 | 89 | Lecture/écriture/injection. |
| @audoora-ext/einvoice 1.2.2 | 20 | GPL-3.0, lecteur. |
| einvoice-kit, factur-x-ts, @canopynexus/factur-x, @openbim/xrechnung… | < 20 | Quasi inutilisés. |

Aucun paquet exigeant Java n'a été mesuré. La seule dépendance Java rencontrée est `xsd-schema-validator`, optionnelle chez node-zugferd et contournée par `strict: false`.

## Tableau de couverture

Légende des colonnes « règles » :
- « réécrit » : règles écrites à la main dans le langage de la librairie, et non le schematron officiel ;
- « SEF/SaxonJS » : schematron officiel compilé, exécuté par SaxonJS.

| Librairie | Version | Licence | Dernière version | Téléchargements | Lecture CII | Écriture CII | UBL | Extraction PDF | Injection PDF | XSD | EN 16931 (CII) | Profils Factur-X (MINIMUM, BASIC WL, EXTENDED) | BR-FR | BR-DE (XRechnung) | Peppol BIS 3 | Contrôle du conteneur PDF/A-3 | Dépendances externes |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| @stackforge-eu/factur-x | 1.4.7 | EUPL-1.2 | 1.4.7 (2026-11-19) | 94 466 | non | oui (saisie propre) | non | oui | oui (XMP PDF/A-3b) | oui, Factur-X 1.09, 5 profils (libxml2-wasm) | non | XSD seulement | non | non | non | non | libxml2-wasm (WASM) |
| @e-invoice-eu/core | 3.4.4 | WTFPL | 3.4.4 (2026-11-16) | 63 013 | non | oui (JSON façon UBL) | écriture | non | partiel (XML généré seulement) | non | non | génération seulement | non | génération seulement | non | non | LibreOffice si pas de PDF fourni |
| node-zugferd | 0.1.1-beta.1 | MIT | 0.1.1-beta.1 (`latest`) ; 1.0.0-beta.1 (`beta`, 2026-11-16) | 62 514 | non | oui (saisie par profil) | non | non | partiel (XML généré seulement) | Factur-X 1.07.3, via Java | non | génération 5 profils + XSD (Java) | non | non | non | non | Java pour la validation (optionnel) |
| @stafyniaksacha/facturx | 0.6.1 | MIT | 0.6.1 (2026-08-28) | 7 339 | oui | oui | non | oui (sauf `xrechnung.xml`) | échoue si l'hôte a un /ID | oui, Factur-X 1.09, 5 profils | schematron officiel Factur-X 1.09 (règles CEN), SEF/SaxonJS, sans sévérités | oui (XSD + schematron par profil) | non | non | non (R008 seulement) | non | SaxonJS (licence Saxonica), libxml2-wasm |
| factur-x-kit | 0.3.2 | MIT | 0.3.2 (2026-07-31) | 5 067 | partiel (13/26) | oui | non | partiel (dans `fromPDF`) | partiel (réécrit le XML) | non | partiel, réécrit (131 id BR cités, zod) | MINIMUM, BASIC WL ; EXTENDED replié sur EN16931 | non | non | non | non | aucune |
| @lucaapp/zugferd-ts | 0.1.0 | Apache-2.0 | 0.1.0 (2026-06-13) | 4 395 | partiel (19/26) | oui | oui (lecture, écriture, XSD) | oui | oui (PDF/A-3b) | oui, Factur-X 1.08 + UBL 2.1 (xmllint-wasm) | oui, CEN 1.3.16, 806 asserts (node-schematron, sévérités ignorées) | XSD par profil | non | non (écrit l'XRechnung sans règles KoSIT) | non | partiel (21 règles, conteneur seul) | xmllint-wasm (WASM) |
| @attestwire/en16931 | 0.14.1 | MIT | 0.14.1 (2026-10-04) | 2 538 | oui | oui | oui (lecture) | oui (lecteur maison) | non | non | réécrit (≈ 217 id EN 16931 ; README : 294 règles en tout) | partiel (AW-PROFILE-*) | non (SIREN/SIRET seulement) | réécrit (24 BR-DE) | UBL seulement, réécrit (27 R) | partiel (6 familles AW-PDF-*, avertissements) | aucune |
| @ekwo-ai/factur-x | 0.27.0 | MIT | 0.27.0 (2026-11-19) | 521 | oui (`readCii`) | oui (modèle propre) | non | oui | oui (XMP) | non | partiel (8 BR-CO) | génération 5 profils | non | non | non | non | aucune |
| @conformo/toolkit (+ validate, parse, pdf) | 0.2.4 | Apache-2.0 | 0.2.4 (2026-11-12) | 497 (validate 528) | oui | oui | oui | oui | oui (PDF/A-3, ICC) | oui, CII D16B, Factur-X MINIMUM/BASIC WL/EXTENDED, UBL 2.1 (xmllint-wasm) | oui, CEN 1.3.17, SEF/SaxonJS | XSD seulement (pas de schematron, à dessein) | oui, FNFE-MPE factur-x 6.8, Flux 2 (si `country: "FR"`) | oui, KoSIT 3.0.2 (2026-08-31) | UBL seulement (3.0.22) | partiel (notices XMP / nom de fichier) | SaxonJS (licence Saxonica), xmllint-wasm, pdfjs-dist |
| @kontor-mcp/core | 1.0.4 | Apache-2.0 | 1.0.4 (2026-08-26) | 337 (rules 554) | oui | partiel (`convertInvoice`) | oui | oui | non (dessine son PDF) | oui, CII D16B + UBL 2.1 (xmllint-wasm) | oui, CEN 1.3.16, SEF/SaxonJS | non | non | oui, XRechnung 3.0.2 (schematron 2.5.0) | non | non | SaxonJS (licence Saxonica), xmllint-wasm |
| @sweberdev/summand | 2.0.1 | MIT | 2.0.1 (2026-10-13) | 323 | non (version Pro) | non (version Pro) | oui (validation) | oui | non | oui, CII + UBL (validateur maison) | oui, CEN 1.3.16, 806 asserts (moteur maison) | partiel (profil XMP comparé au BT-24) | non | oui, XRechnung 2.6.0 (52 asserts) | non | partiel (profil XMP) | aucune |
| en16931 | 0.1.0 | MIT AND EUPL-1.2 | 0.1.0 (2026-07-28) | 360 | non | non | oui (validation) | non | non | non | oui, CEN 1.3.16 copié (fontoxpath, décimaux exacts, sans sévérités) | non | non | non | non | non | aucune |
| @fin.cx/einvoice | 11.3.1 | MIT | 11.3.1 (2026-09-30) | 323 | oui | oui | oui | oui | partiel (pièce jointe sans XMP) | non | partiel, réécrit (≈ 200 id) | non | non | partiel, réécrit (12 BR-DE) | non (2 id) | non | postinstall réseau (non exécuté) |
