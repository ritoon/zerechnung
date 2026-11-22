# Banc Python : librairies retenues, écartées et couverture de la norme

Banc Python du protocole commun (`../PROTOCOL.md`), comparé au paquet Go
`github.com/ritoon/zerechnung`. Relevés du 2026-11-17.

- **Interpréteur** : CPython 3.13.18 (Homebrew), venv `.venv/`, sans uv.
  Dépendances figées dans `requirements.txt` (sortie de `pip freeze`),
  installées en roues seulement (`--only-binary=:all:`) sauf `tagic`
  (dépendance pure Python de getafix, publiée en sdist seulement ; pyproject
  relu : backend poetry-core, pas de `setup.py`, aucun code exécuté à
  l'installation).
- **Artefacts officiels** téléchargés par `./run.sh setup` dans `.artefacts/`
  (ignoré par git), SHA-256 vérifiés dans `run.sh` :
  CEN EN 16931 CII **1.3.17** (`en16931-cii-1.3.17.zip`, release
  `validation-1.3.17` de ConnectingEurope/eInvoicing-EN16931) et KoSIT
  **XRechnung 3.0.2 / schematron 2.6.0** (`xrechnung-3.0.2-schematron-2.6.0.zip`,
  itplr-kosit/xrechnung-schematron).
- **Pas de Java** : aucun adaptateur ne lance de JVM. Le seul moteur XSLT 2.0
  est `saxonche` (SaxonC-HE compilé en bibliothèque native GraalVM :
  `saxonche.dylibs/libsaxonc-core-he.13.1.1.dylib`, 97 Mo).
- **Pas de réseau** pendant `survey` et `bench` : les seuls appels réseau
  trouvés dans le code des librairies mesurées (factur-x : schematron par un
  serveur Saxon HTTP ; mcp-einvoicing-de : validateur KoSIT distant) ne sont
  pas empruntés (voir leurs fiches).

Lancement : `./run.sh setup`, puis `./run.sh survey [lib…]` et
`./run.sh bench [lib…]` (variables `OUT`, `COUNT`, `BENCHTIME` ; aussi
`CHECK_TIMEOUT` = 30 s pour l'appel de contrôle d'un benchmark,
`SURVEY_TIMEOUT` = 10 s par fichier du relevé, `CHILD_TIMEOUT` = 1800 s par
processus fils). Chaque benchmark tourne dans un **processus fils neuf**, si
bien que `first_call_ns` est un vrai premier appel (imports déjà faits,
caches de la librairie froids). Le relevé lance un processus par librairie.

Les chemins `paquet/fichier.py:ligne` ci-dessous sont relatifs à
`.venv/lib/python3.13/site-packages/`.

## Résultats du relevé (`./run.sh survey`)

| Librairie | validate (signalés / 26) | parse (/ 26) | extract (/ 104) |
|---|---|---|---|
| factur-x 7.10 | 0 (XSD seule) | 26 | 104 (97 avec `check_xsd=True`, la valeur par défaut) |
| drafthorse 2025.2.0 | 0 (XSD seule) | 26 | — |
| pycheval 0.4.0 | — | 7 | 65 (39 boucles infinies coupées) |
| getafix 0.5.0 | 6 (six exceptions, aucune règle détectée) | 20 | 104 |
| zugferd-pdf 0.1.2 | — | — | 104 |
| erechnung 0.3.0 | — | — | — |
| mcp-einvoicing-de 0.11.4 | 2 (FX-SCH-A-000571 ×2) | 20 | 104 |
| saxonche + XSLT officiels | **6** (BR-CL-25 ×2, CII-SR-465 ×4) | — | — |

« — » : l'opération n'existe pas dans la librairie (rien n'est écrit dans
`survey_python.jsonl`).

**Référence 6/26 reproduite** par `saxonche-schematron`, et expliquée : avec
le CEN 1.3.17 seul, les quatre `XRECHNUNG_*.cii.xml` ne lèvent que
CII-SR-465, de niveau `warning` dans le schematron CEN. Le validateur KoSIT
(validator-configuration-xrechnung v2026-08-31, CHANGELOG « custom level
error for CII-SR-465 and CII-SR-466 ») relève CII-SR-465/466 au niveau
`error` et abaisse CII-SR-475/476 au niveau `information` dans le scénario
CII. L'adaptateur applique ces niveaux quand BT-24 désigne XRechnung
(`adapters/saxonche_schematron.py`, `_KOSIT_CUSTOM_LEVEL`) ; le paquet Go
compte aussi CII-SR-465 comme une erreur (`internal/rules/en16931/syntax.go`).
Les deux BR-CL-25 (schéma d'adresse électronique `EM` hors liste EAS) sont
sur `EN16931_ElektronischeAdresse` et `EN16931_RechnungsUebertragung`.
Contrôle de sensibilité : un GrandTotal modifié donne BR-CO-15 et BR-CO-16.
Le fichier `not_validating_full_invoice…` du corpus ne lève aucune erreur
fatal/error avec les XSLT officiels (ni avec les XSD Factur-X de factur-x et
drafthorse) : son nom vient du corpus.

Détail des écarts à la référence :

- **getafix 6/26** : les six signalements sont des exceptions de lecture, pas
  des règles. Quatre `XRECHNUNG_*` refusés (« not a valid Profile »),
  `EN16931_Betriebskostenabrechnung` (`ValueError` vide),
  `EN16931_Physiotherapeut` (`TypeError: ProcuringProject.__init__() missing
  … 'id' and 'name'`). Les deux BR-CL-25 ne sont pas vus (aucune règle
  BR-CL dans getafix).
- **mcp-einvoicing-de 2/26** : FX-SCH-A-000571 (équivalent FeRD de BR-CL-25)
  sur les deux mêmes fichiers que la référence. Les XRECHNUNG passent :
  CII-SR-465 reste un avertissement, et la librairie ne compte que les
  erreurs (verdict de la librairie, conforme à sa documentation).
- **factur-x, drafthorse 0/26** : validation XSD seule, et les 26 fichiers
  sont conformes au XSD.
- **parse** : pycheval refuse EXTENDED et XRECHNUNG (`pycheval/parse.py:356-361`),
  les TypeCode 204, 387, 389, 575, le SubjectCode ADU, le schéma `EM`, et
  les éléments vides (`BuyerAssignedID`, `LineTwo`,
  `AttachmentBinaryObject`). mcp-einvoicing-de refuse 5 numéros de TVA
  allemands d'exemple dont la clé de contrôle est fausse (DE123456789,
  DE234567891) et une remise négative (modèle pydantic strict). getafix :
  mêmes six fichiers que pour validate.
- **extract** : pycheval boucle sans fin dès que la première pièce jointe
  ne s'appelle pas `factur-x.xml` (`pycheval/pdf_extract.py:30-32`,
  `while doc: if doc[0] == FACTURX_FILENAME: break` sans avancer) ; le
  harnais coupe l'appel au bout de `SURVEY_TIMEOUT`. factur-x avec ses
  options par défaut (`check_xsd=True`) refuse 7 XML extraits non conformes
  au XSD.

## Fiches : librairies retenues

### factur-x 7.10 (Akretion)

BSD, dernière version 2026-10-30, 411 142 téléchargements le mois dernier
(pypistats). La librairie de référence de l'écosystème (Odoo).

- Lecture : `parse_ubl_cii_xml` (`facturx/parse_xml.py:49`, XSD par défaut
  ligne 56) vers un dictionnaire ; écriture `generate_cii_xml`
  (`facturx/generate_xml.py:2351`) et `generate_ubl_xml` (ligne 3607).
- PDF : `get_xml_from_pdf` (`facturx/facturx.py:662`),
  `generate_from_binary` (ligne 1425, XMP PDF/A-3, sans conversion PDF/A,
  passe par un fichier temporaire).
- XSD embarqués : Factur-X **1.09** (5 profils,
  `facturx/xsd_and_schematron/facturx-*/Factur-X_*.xsd`), UBL 2.1,
  ZUGFeRD 1.0, CII D22B (extended CTC FR).
- Schematrons embarqués : Factur-X **1.09.2** (FNFE-MPE/FeRD, 5 profils ;
  le XSLT EN 16931 compte 200 identifiants BR, 14 CII-SR et 218 FX-SCH),
  BR-FR-Flux2 CII et UBL (124 identifiants BR-FR), CEN UBL **1.3.16**.
  Mais `xml_check_schematron` ne les exécute **que par un POST HTTP vers un
  serveur Saxon** (`facturx/facturx.py:495` `requests.post`, URL par défaut
  `http://localhost:5000/transform` ligne 179 ; les codedb sont servies
  depuis raw.githubusercontent.com, lignes 182-186). Ce serveur n'est pas
  fourni (il faut une JVM) : hors ligne, la validation se réduit au XSD.
- `generate_cii_xml` a `check_schematron="base"` par défaut
  (`generate_xml.py:2355`) : l'adaptateur passe `check_schematron=False`
  pour ne rien envoyer au réseau.
- Pas de contrôle natif d'un PDF hybride (`CheckPDF` non supporté).

### drafthorse 2025.2.0 (pretix)

Apache-2.0, dernière version 2025-09-15, 156 671 téléchargements/mois.

- Modèle objet complet, `Document.parse` et `Document.serialize`
  (`drafthorse/models/elements.py:70-78` : la sérialisation revalide la
  sortie contre le XSD, ce qui entre dans `Write_*`).
- XSD Factur-X **1.0.07** des 5 profils (`drafthorse/schema/Factur-X_1.0.07_*.xsd`,
  `drafthorse/utils.py:14-15`), aucun schematron.
- `attach_xml` (`drafthorse/pdf.py:47`) : XMP PDF/A-3, reprend les
  OutputIntents de l'hôte ; pas de lecture de PDF (README).

### PyCheval 0.4.0

Apache-2.0, dernière version 2026-11-16, 3 561 téléchargements/mois.

- Le README annonce Factur-X 1.09.2, le code déclare
  `FACTURX_VERSION = "1.0.07"` (`pycheval/__init__.py:19`).
- Modèles MINIMUM, BASIC WL, BASIC, EN 16931 ; EXTENDED et XRECHNUNG
  refusés à la lecture (`pycheval/parse.py:356-361`). 52 contrôles de
  structure `raise ModelError` dans `pycheval/model.py`, pas de règles BR.
- Extraction : boucle infinie décrite plus haut ; filtre aussi sur
  `/Subtype /text/xml`.
- Contrôle de l'AFRelationship selon le pays (fr, de) à la lecture d'un PDF
  (`pycheval/pdf_parse.py:85-110`) : seul contrôle de conteneur.
- Injection : l'API publique (`embed_facturx_file_in_pdf`,
  `pycheval/pdf_embed.py:77`) lit le XML depuis un fichier ; l'adaptateur
  appelle `_embed` (ligne 133) avec les mêmes options par défaut
  (relation Data) pour rester en mémoire.

### getafix 0.5.0

Apache-2.0, dernière version 2026-11-10, 2 092 téléchargements/mois.

- Modèle typé BT/BG, lecture et écriture CII (ElementTree de la stdlib).
- Profils : enum « Factur-X 1.08 / ZUGFeRD 2.4 » (`getafix/schema/types.py:6-7`),
  5 profils, pas de XRECHNUNG.
- Règles codées à la main : 90 identifiants BR-* distincts dans le paquet
  (comptés par AST), dont **89 des 223 règles BR du CEN 1.3.17** (après
  normalisation BR-05/BR-5 et BR-IG/BR-AF), **aucune BR-CL**, aucune
  CII-SR/CII-DT ; plus 16 BR-FXEXT-* (surcouche EXTENDED) et 11 GETAFIX-*
  (`getafix/rules/*.py`). Les `ValidationWarning` ne comptent pas.
- PDF : `extract_xml(pdf: Path)` (`getafix/pdf.py:92`) n'accepte qu'un
  chemin (la lecture du fichier est dans la mesure) ; `attach_xml`
  (ligne 45) ajoute une pièce jointe générique et, selon sa documentation
  (lignes 18-20), ne rend pas le document PDF/A-3.

### zugferd-pdf 0.1.2 (module `zugferd`)

MIT, dernière version 2026-08-25, 97 téléchargements/mois.

- Écriture depuis un modèle maison (`generate_facturx_xml`), extraction
  (`zugferd/extractor.py:13`), packaging PDF/A-3U
  (`zugferd/pdfa_packager.py:140` : ajoute un OutputIntent sRGB et un XMP
  PDF/A-3U, AFRelationship Alternative).
- `validate_zugferd_pdf` (`zugferd/validator.py:78`) est le seul
  « CheckPDF » natif du banc, mais il est fait maison : XMP, OutputIntent,
  /AF, liste BT-24, BR-DEC-23 et décimales de BT-112 ; pas de schematron
  malgré le README. Faux positifs : `pdfaid:part` n'est cherché que sous
  forme d'élément (ligne 124), si bien que le `EN16931_Einfach.pdf`
  officiel (XMP en attributs) est signalé « Missing pdfaid:part » et
  « Missing /OutputIntents » (ligne 184).
- Pas de lecture XML vers modèle : `Write_*` part d'un modèle construit à
  la main (`adapters/zugferd_pdf.py`, `hand_model`).

### erechnung 0.3.0

MIT, dernière version 2026-08-28, 116 téléchargements/mois.

- Génération seule : PDF DIN 5008 dessiné par reportlab, XML CII EN 16931
  ou XRechnung 3.0 (`erechnung/xml.py:16-18`), puis attaché par factur-x
  (`erechnung/generator.py:7`, 72).
- Mesuré : `calculate` + `xml_bytes`, la partie XML du chemin public
  `Invoice.zugferd_file`, sur un modèle construit à la main
  (`adapters/erechnung.py`). `erechnung/validation.py` ne contrôle que les
  données d'entrée Python (IBAN, pays, dates…).

### mcp-einvoicing-de 0.11.4 (+ mcp-einvoicing-core 1.35.0)

Apache-2.0, dernière version 2026-09-12, 287 téléchargements/mois (core :
1 582). Serveur MCP ; on n'utilise que la couche bibliothèque.

- Lecture / écriture : `ZUGFeRDCIIParser`, `ZUGFeRDCIISerializer`,
  `XRechnungUBLParser/Serializer` (`mcp_einvoicing_de/serializers.py:26-95`),
  modèle pydantic strict (clé de contrôle des n° de TVA allemands). Le
  sérialiseur perd les pièces jointes (`Write_Large` impossible à comparer
  octet à octet).
- Validation locale `invoice_validate` : XSLT embarqués exécutés par
  saxonche (`mcp_einvoicing_de/validators/schematron.py:69-84`,
  `tools/invoice_validate.py:172-195`). Profil EN 16931 : XSLT FeRD
  Factur-X **1.09.2** (en-tête du fichier ; la docstring ligne 10 dit encore
  1.08). XRECHNUNG : CEN EN 16931 CII (compilé par SchXslt 1.10.1, depuis la
  configuration KoSIT 2026-08-31, donc CEN **1.3.16**) puis
  XRechnung-CII-validation.xsl « Schematron Version 2.6.0 / XRechnung
  3.0.2 », mêmes identifiants de règles que l'artefact officiel 2.6.0
  (39 BR-DE, 7 BR-DEX, 6 BR-TMP, 22 PEPPOL-EN16931) mais fichier pas
  identique octet à octet. Seules les erreurs fatal/error comptent.
- Coût : `_run_one_stylesheet` (`tools/invoice_validate.py:278`) construit
  un nouveau validateur à chaque appel, donc un nouveau `PySaxonProcessor`
  et une recompilation de la feuille (`mcp_einvoicing_core/schematron.py:381-384`) :
  ~0,1 à 0,2 s par validation, c'est le coût réel de l'API.
- Réseau : la validation KoSIT distante (`validators/kosit.py`, httpx) est
  désactivée par défaut depuis la 0.11 (`cloud_validate=False`,
  `tools/invoice_validate.py:40` lit `EINVOICING_DE_KOSIT_ENABLE`).
  L'adaptateur force cette variable à `0` avant l'import et vérifie
  `validator_used == "local_schematron"` à chaque appel.
- Pas d'usage des XSD trouvés dans le paquet DE ; pas de contrôle de PDF
  hybride (`PDFEmbedder.identify` ne lit que le XMP).

### saxonche 13.1.1 + XSLT officiels (validateur de référence)

SaxonC-HE, MPL-2.0 selon Saxonica (champ licence vide sur PyPI), dernière
version 2026-10-27, 1 978 505 téléchargements/mois. Natif, sans JVM.

- XSLT CEN EN 16931 CII 1.3.17 : 223 identifiants BR (dont 23 BR-CL et
  21 BR-DEC), 486 CII-SR, 103 CII-DT. Plus le XSLT KoSIT XRechnung 2.6.0
  quand BT-24 désigne XRechnung, avec les niveaux personnalisés KoSIT
  (voir plus haut).
- Compilation des deux feuilles au premier appel (0,15 à 0,3 s, compris
  dans `first_call_ns`), puis 1 à 15 ms par facture du banc.
- Seulement `ParseValidate_*` (validateur seul).

## Statut des benchmarks

Essai `COUNT=1 BENCHTIME=0.2 ./run.sh bench` (2 min 30 s pour les
8 librairies × 16 benchmarks ; les durées ne sont pas à retenir) :

| Librairie | ok | unsupported | fail |
|---|---|---|---|
| factur-x | 14 | CheckPDF ×2 | — |
| drafthorse | 10 | Extract ×4, CheckPDF ×2 | — |
| pycheval | 6 | ParseValidate ×3, CheckPDF ×2 | Extract_Large, Extract_Extended (boucle infinie, coupée à 30 s), Parse_Lines et Write_Lines (`BuyerAssignedID has no text`), Parse_Large (profil XRECHNUNG refusé) |
| getafix | 12 | CheckPDF ×2 | Parse_Large, ParseValidate_Large (profil XRECHNUNG refusé) |
| zugferd-pdf | 9 | Parse ×3, ParseValidate ×3, Write_Lines | — |
| erechnung | 1 (Write_Small) | 15 | — |
| mcp-einvoicing-de | 14 | CheckPDF ×2 | — |
| saxonche-schematron | 3 (ParseValidate) | 13 | — |

Points à connaître pour lire les chiffres :

- `Write_*` part du modèle produit par le parseur de la librairie (lecture
  hors mesure). zugferd-pdf et erechnung n'ont pas de parseur : `Write_Small`
  part d'un modèle construit à la main d'après `EN16931_Einfach`
  (`note: "modèle construit à la main"`), `Write_Lines` est `unsupported`.
- `Embed_*` est relu avec la librairie elle-même quand elle sait extraire,
  sinon avec factur-x (`note: "relu avec factur-x"`, cas de drafthorse).
- factur-x `Parse_*` inclut la validation XSD (option par défaut), d'où
  `Parse_Large` plus lent que `ParseValidate_Large` (XSD seule).
- drafthorse `Write_*` revalide la sortie contre le XSD (comportement de
  `serialize`).
- getafix `Extract_*` lit le fichier sur disque dans la mesure (API par chemin, cache OS).
- mcp-einvoicing-de `ParseValidate_*` recompile la feuille XSLT à chaque
  appel (~0,12 à 0,2 s/op) ; saxonche-schematron compile une fois.
- Le seul `CheckPDF` natif est celui de zugferd-pdf, contrôles maison.

## Fiches : librairies écartées

| Paquet | Raison |
|---|---|
| en16931 0.2 (b2brouter) | UBL seulement ; validation par l'API distante validex.net |
| zugferd 0.1.1, xrechnung 0.1.2 | clients de l'API distante xrechnung.dev |
| beliq 0.7.3, factora 0.1.2, invowerk 1.0.19, facturx-engine 0.1.0 | clients d'API distantes ou de conteneurs Docker |
| python-invoice-lint 1.0.0 | enveloppe de saxonche + CEN téléchargé à l'exécution dans `~/.cache` (doublon du validateur de référence), API par chemin, recompile à chaque appel, URL de projet « your-org » laissées en place, licence « Other/Proprietary » ; 26 téléchargements/mois |
| pyfactx 0.7.0 | génération seule, épingle `saxonche<13`, autovalidation par XSLT Factur-X 1.07.2 ; 20/mois |
| facturxlib 1.0.1 | génération seule, le modèle simplifié ne gère qu'un taux de TVA (EN16931_Einfach en a deux) ; 25/mois ; installé pour essai puis retiré |
| pydantic-factur-x 0.2.0 | génération seule, dernière version 2023-12 ; 70/mois |
| factur-x-nolog 1.7.3 | fourche ancienne de factur-x |
| pyschematron 1.2.5 | moteur Schematron générique pur Python (GPLv3+, 7 401/mois), pas une librairie Factur-X. Essai hors banc avec le `.sch` prétraité du CEN 1.3.17 : verdicts corrects (BR-CL-25 trouvé), mais 12 s pour `EN16931_Einfach`, 33 s pour `EN16931_ElektronischeAdresse` et 97 s pour le fichier CEN complet, contre quelques ms pour saxonche : inutilisable pour ce banc |
| einvoice 0.0.1 | QR code de facture indienne |
| invoice2data | extraction de texte de PDF par gabarits, pas de XML |
| python-ubl, peppol-py | UBL/Peppol seulement, ou transport AS4 |
| Liaisons Rust/C | aucune trouvée sur PyPI (`core-invoice`, `zugferd-rs`, `facturx-rs`… : 404 sur l'API JSON de PyPI ; rien dans les recherches web et GitHub) |

## Couverture de la norme

| Librairie | Version | Licence | Dernière version | Téléchargements | Lecture CII | Écriture CII | UBL | Extraction PDF | Injection PDF | XSD | EN 16931 (CII) | Profils Factur-X (MINIMUM, BASIC WL, EXTENDED) | BR-FR | BR-DE (XRechnung) | Peppol BIS 3 | Contrôle du conteneur PDF/A-3 | Dépendances externes |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| factur-x | 7.10 | BSD | 2026-11-19 | 411 142/mois | oui (dictionnaire) | oui | oui (lecture, écriture ; XSLT CEN UBL 1.3.16 via serveur Saxon) | oui | oui (XMP PDF/A-3, sans conversion) | oui (Factur-X 1.09, 5 profils) | XSLT FNFE-MPE Factur-X 1.09.2 embarqué, exécuté seulement par un serveur Saxon HTTP ; hors ligne : non | XSD des 5 profils ; schematron des 5 profils via serveur Saxon | XSLT BR-FR-Flux2 (124 règles) via serveur Saxon | non | non | non | lxml, pypdf, requests ; serveur Saxon HTTP (JVM) pour tout schematron |
| drafthorse | 2025.2.0 | Apache-2.0 | 2025-09-15 | 156 671/mois | oui | oui (revalidée par XSD) | non | non | oui (XMP PDF/A-3) | oui (Factur-X 1.0.07, 5 profils) | non | XSD des 5 profils | non | non | non | non | lxml, pypdf |
| pycheval | 0.4.0 | Apache-2.0 | 2026-11-16 | 3 561/mois | partiel (7/26 : refuse EXTENDED, XRECHNUNG, plusieurs TypeCode) | oui | non | partiel (65/104, boucle infinie) | oui (API publique par fichier) | non | non (52 contrôles de structure) | MINIMUM, BASIC WL, BASIC, EN 16931 ; pas EXTENDED | non | non (XRECHNUNG refusé) | non | partiel (AFRelationship selon le pays) | pypdf |
| getafix | 0.5.0 | Apache-2.0 | 2026-11-10 | 2 092/mois | oui (20/26) | oui | non | oui (chemin seulement) | partiel (pièce jointe sans XMP ni PDF/A-3) | non | partiel (89 des 223 règles BR du CEN, aucune BR-CL ni CII-SR) | 5 profils (Factur-X 1.08) ; EXTENDED : 16 règles BR-FXEXT | non | non | non | non | tagic, pypdf |
| zugferd-pdf | 0.1.2 | MIT | 2026-08-25 | 97/mois | non | oui (modèle maison) | non | oui | oui (ajoute OutputIntent sRGB et XMP PDF/A-3U) | non | non (BR-DEC-23, décimales BT-112) | liste BT-24 seulement | non | non | non | partiel (contrôles maison, faux positifs sur XMP en attributs) | pikepdf, pypdf, lxml, reportlab |
| erechnung | 0.3.0 | MIT | 2026-08-28 | 116/mois | non | oui (EN 16931, XRechnung 3.0) | non | non | non (dessine son PDF, attaché par factur-x) | non | non (données d'entrée seulement) | non | non | non | non | non | reportlab, factur-x, pikepdf, lxml, pillow |
| mcp-einvoicing-de | 0.11.4 | Apache-2.0 | 2026-09-12 | 287/mois (core 1 582) | oui (20/26, modèle strict) | oui (avec pertes) | oui (parseur, sérialiseur, XSLT CEN UBL et XRechnung UBL) | oui | oui (pikepdf, XMP pdfaid, OutputIntent sRGB) | non | XSLT FeRD Factur-X 1.09.2 (EN 16931) ou CEN CII 1.3.16 (XRECHNUNG) via saxonche, hors ligne | XSLT FeRD des 5 profils via saxonche | non | XSLT KoSIT XRechnung 3.0.2, schematron 2.6.0 via saxonche | non (22 règles PEPPOL-EN16931 reprises par XRechnung) | non | saxonche (97 Mo natif), pikepdf, pydantic, httpx, fastmcp, cryptography ; KoSIT distant optionnel, désactivé |
| saxonche + XSLT officiels (référence) | 13.1.1 ; CEN 1.3.17 ; KoSIT 2.6.0 | MPL-2.0 | 2026-10-27 | 1 978 505/mois | — | — | non (XSLT CII seulement) | — | — | non (le CEN ne fournit pas de XSD CII) | XSLT officiel CEN 1.3.17 via Saxon (223 BR, 589 CII-SR/CII-DT) | non | non | XSLT officiel KoSIT 2.6.0 via Saxon, niveaux KoSIT | non (22 règles PEPPOL-EN16931 via XRechnung) | non | saxonche natif (sans JVM) ; artefacts téléchargés par `run.sh setup` |

Téléchargements : pypistats, 30 derniers jours au 2026-11-17. Licences et
dates : API JSON de PyPI.
