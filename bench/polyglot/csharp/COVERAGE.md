# Couverture : écosystème C# / .NET

Banc C# du protocole commun (`../PROTOCOL.md`). Un projet console par librairie
(`libs/<lib>`, assemblage `bench-<lib>`), un harnais partagé (`harness`), une
solution qui les regroupe (`Bench.slnx`). Versions épinglées `[x.y.z]` et un
`packages.lock.json` par projet, restauré en mode verrouillé. Toolchain : SDK
.NET 11.0.100 (runtime Microsoft.NETCore.App 11.0.0), cible `net11.0`,
configuration Release.

Les preuves citées sont des chemins dans les sources des dépôts au tag de la
version publiée (ZUGFeRD-csharp 18.0.1, International.EInvoicing v1.1.0,
Balsoft v1.1.0, Securibox V0.6.0-alpha-3, FacturXDotNet v0.5.9-alpha,
pax.XRechnung.NET au dernier commit avant la 0.3.3). Pour OfficeIMO et
En16931 (sources non clonées) et pour Antiphon (logiciel fermé), les preuves
viennent du contenu du `.nupkg` (README, ressources, documentation XML) et de
l'API publique lue dans les métadonnées des assemblages (MetadataLoadContext,
sans exécuter de code). Licence, dernière version et téléchargements viennent de
l'API NuGet (registration et search, relevé du 2026-11-22). Les téléchargements
sont le total de toutes les versions : NuGet ne publie pas de chiffre sur
90 jours.

## Librairies mesurées

### ZUGFeRD-csharp + ZUGFeRD.PDF-csharp 18.0.1 (Stephan Stapel)

La librairie .NET la plus téléchargée du domaine. Modèle `InvoiceDescriptor`,
lecteurs et écrivains ZUGFeRD 1.0, 2.0 et 2.3 (CII), ainsi que XRechnung UBL.
Le paquet `.PDF` lit et écrit le PDF sur PDFsharp-extended 1.3.0
(`ZUGFeRD.PDF/ZUGFeRD.PDF.csproj:45`).

- Fin de maintenance annoncée dans la description NuGet : support jusqu'au
  30 novembre 2026, paquet « Legacy » à partir du 1er décembre 2026.
- Extraction : `InvoicePdfProcessor.LoadFromPdf`
  (`ZUGFeRD.PDF/InvoicePdfProcessor.cs:46`) extrait la pièce jointe et lit le
  modèle. Noms cherchés : zugferd.xml, zugferd-invoice.xml, factur-x.xml,
  xrechnung.xml (`ZUGFeRD.PDF/InvoiceDescriptorPdfLoader.cs:103`). Aucune API
  publique ne rend les octets : `Extract_*` mesure extraction et lecture. La
  vérification réécrit le modèle hors chronométrage. 103 PDF sur 104 :
  `Avoir_FR_type381_BASIC.pdf` est refusé (« No ZUGFeRD invoice reader was
  able to parse this stream »).
- Validation : `InvoiceValidator.Validate` (`ZUGFeRD/InvoiceValidator.cs:50`)
  recalcule seulement les totaux : somme des lignes (l. 69-108), BR-CO-17 et
  BR-CO-14 (l. 148-182), totaux et montant dû. Pas de XSD ni de schematron.
  Les messages mêlent la trace du calcul et les échecs. L'adaptateur garde les
  lignes d'échec : celles qui portent un identifiant BR- ou BT-, ou un écart
  « berechnet / tatsächlich ». Ce filtre est heuristique.
- Les 3 fichiers signalés (Teilrechnung, Kraftfahrversicherung,
  Sachversicherung) sont des faux positifs sur `lineTotal` et `grandTotal`.
  Balsoft fait les mêmes, pour la même raison.
- Écriture : `InvoiceDescriptor.Save(Version23, profil lu, CII)`.
- Injection : `InvoicePdfProcessor.SaveToPdf(Version23, Comfort, CII)`
  réécrit le XML depuis le modèle et recopie les pages dans un nouveau document
  PDFsharp. Il ajoute un XMP Factur-X et un OutputIntent sRGB
  (`ZUGFeRD.PDF/InvoiceDescriptorPdfSaver.cs:321-326`), et embarque les polices
  prises dans les dossiers système (`:329`, `:345-`,
  `ZUGFeRD.PDF/FontInfoProvider.cs:89-103`). Dates = `DateTime.UtcNow`
  (`InvoiceDescriptorPdfSaver.cs:185`, `:230`), non réglable.
- Pas de contrôle de PDF hybride : CheckPDF est `unsupported`. Réseau : aucun
  (pas de `HttpClient` dans les sources).

### International.EInvoicing 1.1.0 (Epic Dice Studio)

Modèle EN 16931, lecteurs et écrivains UBL, CII, Factur-X / ZUGFeRD et
XRechnung, avec un moteur Schematron maison : un sous-ensemble XPath évalué
sur les `.sch` officiels prétraités, sans XSLT. Paquets utilisés, tous en
1.1.0 : la façade, `.FacturX.PdfSharp` (PDFsharp 6.2.4, MIT), `.Countries.Germany`,
`.Validation.XRechnung` et `.Validation.Xsd`.

- EN 16931 : artefacts CEN 1.3.16
  (`src/International.EInvoicing.Validation.En16931/En16931Rules.cs:18`),
  806 assertions dans le `.sch` CII prétraité. `AddDefaults` charge UBL, CII,
  CDAR, Factur-X, les règles EN 16931 et la façade
  (`src/International.EInvoicing/EInvoicingBuilderExtensions.cs:27-38`).
- XRechnung : `ArtefactVersion = "3.0"`
  (`src/International.EInvoicing.Validation.XRechnung/XRechnungRules.cs:18`),
  52 assertions. Le schematron KoSIT a été pris sur `master` le 2026-08-29, pas
  sur une version publiée (`specs/xrechnung/PROVENANCE.md:6-7`). Les niveaux
  personnalisés de la configuration KoSIT ne s'appliquent pas : CII-SR-465
  reste un warning, d'où 2 fichiers signalés sur 26 au lieu de 6.
- XSD : UN/CEFACT CII D16B
  (`src/International.EInvoicing.Validation.Xsd/XsdBuilderExtensions.cs:37`).
- Configuration mesurée : `AddDefaults().AddGermany().AddXRechnungRules().AddCiiSchema()`,
  c'est-à-dire `GermanEInvoicing.Create`
  (`src/International.EInvoicing.Countries.Germany/GermanEInvoicing.cs:42-43`)
  plus le XSD. Les règles XRechnung ne s'appliquent que si BT-24 le déclare.
- Hors ligne, rien d'autre. Les règles Factur-X (FeRD) ne sont pas livrées et
  se chargent depuis un dossier (`AddFacturXRulesFrom`,
  `src/International.EInvoicing.FacturX/FacturXRules.cs:43`). Les règles
  françaises viennent d'`AddFrance`
  (`src/International.EInvoicing.Countries.France/FranceServiceCollectionExtensions.cs:18`)
  et Peppol d'un paquet séparé (`International.EInvoicing.Peppol`). Aucun des
  trois n'est chargé : aucun exemple du corpus ne déclare Peppol.
- Extraction : `PdfSharpAttachmentReader.FindAttachment` avec
  `FacturXAttachment.KnownFileNames`
  (`src/International.EInvoicing.FacturX/Pdf/FacturXAttachment.cs:21-`) et la
  limite par défaut de 64 Mio. Il rend les octets bruts.
- Injection : `PdfSharpAttachmentWriter.Attach` ajoute factur-x.xml en fichier
  associé et ajoute ou complète le XMP Factur-X. ModDate =
  `DateTimeOffset.UtcNow`
  (`src/International.EInvoicing.FacturX.PdfSharp/PdfSharpAttachmentWriter.cs:59`).
- Pas de CheckPDF : la lecture d'un PDF compare le XMP au XML (avertissements),
  sans PDF/A.
- Réseau : aucun dans les paquets utilisés. Le seul `HttpClient` des sources est
  dans `web/Playground`, qui n'est pas publié.

### Balsoft.Hive.EInvoice + Balsoft.Hive.EInvoice.Pdf 1.1.0 (Balsoft GmbH)

Modèle EN 16931 écrit à la main, lecteurs et écrivains CII et UBL, validateur
codé en C# sur le modèle, création de PDF hybride sur PDFsharp 6.2.4.

- Règles : 115 identifiants BR distincts dans
  `src/Balsoft.Hive.EInvoice/Validation/InvoiceValidator.cs`, dont 26 BR-DE,
  ainsi que 4 règles PEPPOL-EN16931. Pas de XSD ni de schematron.
- Le modèle ne garde pas les totaux déclarés. `ReadResult.TotalsDiscrepancies`
  (`src/Balsoft.Hive.EInvoice/Reading/ReadResult.cs`) les compare aux totaux
  recalculés, et l'adaptateur ajoute ces écarts (identifiants BT) aux erreurs.
  Résultat : BR-CL-25 sur les 2 bons fichiers, et des écarts BT-106, BT-109,
  BT-112 et BT-115 sur 3 exemples valides. Ce sont les mêmes faux positifs que
  ZUGFeRD-csharp.
- Écriture : `CiiWriter.Write` valide avant d'écrire par défaut
  (`src/Balsoft.Hive.EInvoice/Cii/CiiWriter.cs:12-22`). L'adaptateur passe
  `ValidateBeforeWriting=false`, pour ne pas mesurer deux fois la validation.
- Extraction : `HybridPdf.ExtractXml`
  (`src/Balsoft.Hive.EInvoice.Pdf/HybridPdf.cs:127`) cherche factur-x.xml,
  xrechnung.xml, zugferd-invoice.xml, sinon le premier `.xml`.
- Injection : `HybridPdf.Create` (`HybridPdf.cs:83`) recopie les pages dans un
  nouveau document et ajoute factur-x.xml en fichier associé et un OutputIntent
  sRGB. Les polices manquantes sont prises dans les dossiers système. Le XMP est
  remplacé par mise à jour incrémentale. Horodatage = `DateTimeOffset.Now` par
  défaut.
- Pas de CheckPDF. Réseau : aucun.

### Securibox.FacturX 0.6.0-alpha-3 (Securibox)

Import, export et validation Factur-X. Un modèle `XmlSerializer` par profil,
généré depuis les XSD Factur-X 1.09. Les XSD et schematrons FeRD Factur-X 1.09
des cinq profils sont embarqués (`Securibox.FacturX/Resources/`, 424 assertions
dans `Factur-X_1.09_EN16931.sch`). Moteur Schematron maison : le `.sch` est
désérialisé et les assertions évaluées par XPath2 1.1.5, sans XSLT. PDF sur
PDFsharp 7.0.0-preview-1 (MIT ; la preview-2 est AGPL), XMP sur XmpCore
6.1.10.1.

- Le csproj du tag déclare `0.6.0-alpha-2`
  (`Securibox.FacturX/Securibox.FacturX.csproj:8`), alors que le tag et le
  paquet sont en 0.6.0-alpha-3.
- Validation mesurée : `FacturxXsdValidator.ValidateXml` +
  `FacturxSchematronValidator.ValidateXml` au profil détecté.
  L'adaptateur détecte le profil comme le fait `FacturxImporter`
  (`FacturxImporter.cs:574-627`) : XRechnung et EN 16931 vont tous deux au
  profil EN 16931. Le `.sch` est relu et désérialisé à chaque appel
  (`FacturxSchematronValidator.cs:11-60`), d'où environ 100 ms par validation.
- Les assertions FeRD n'ont pas d'`@id`, donc `BusinessRuleCode` est vide
  (`Schematron/Helpers/ValidationReportMapper.cs`). L'adaptateur prend
  l'identifiant au début du message (« [CII-SR-465] »). Pour les assertions de
  listes de codes, qui n'en ont pas, il note `codedb clN`.
- Relevé : 6 fichiers signalés, les mêmes que la référence, mais pas pour les
  mêmes raisons :
  - CII-SR-465 sur les 4 XRECHNUNG : l'assertion FeRD n'a pas de drapeau et
    compte donc comme erreur ;
  - `codedb cl1` sur ces mêmes 4 fichiers : la liste 1 du profil EN 16931 ne
    contient que `urn:cen.eu:en16931:2017`, et rattacher XRechnung à ce profil
    crée l'erreur ;
  - `codedb cl17` sur les 2 fichiers BR-CL-25 (liste des schémas
    d'identifiants).
- Extraction : aucune API publique ne rend les octets.
  `ImportDataWithDeserialization` (`FacturxImporter.cs:143`) appelle
  `IsFacturXValid`, ignore le résultat, puis désérialise. La vérification
  resérialise hors chronométrage. Résultat : 74 PDF sur 104. 29 échouent sur
  « XmpException: Schema namespace URI and prefix mismatch » (XMP de PDF
  ZUGFeRD 2.0, `Extract_Extended` compris), 1 sur « Page has no MediaBox ».
- Écriture : aucune API publique ne transforme un modèle en XML sans PDF.
  L'adaptateur reprend `XmlSerializer.Serialize` avec les espaces de noms de
  `FacturxExporter`.
- Injection : `FacturxExporter.CreateFacturXStream` valide d'abord et échoue
  en cas d'erreur (`FacturxExporter.cs:222-`). Il recopie ensuite les pages et
  écrit le XMP.
- CheckPDF : `IsFacturXValid` (`FacturxImporter.cs:97-140`) contrôle le XMP,
  le XML embarqué, le XSD et le schematron. Erreurs et warnings comptent
  (`:119`). Pas de contrôle PDF/A.
- Réseau : aucun.

### OfficeIMO.Invoicing + .Validation + .Pdf 3.6.2 (Evotec)

Modèle de facture typé, lecteurs et écrivains CII et UBL bornés (EN 16931
1.3.16, XRechnung 3.0.2, Factur-X 1.09.2 / ZUGFeRD 2.5.2, Peppol BIS 3.0.21),
contrôles du modèle (sémantique, arithmétique), et OfficeIMO.Pdf, un moteur PDF
en C#.

- Validation : `OfficeIMO.Invoicing.Validation` exécute l'étape XSD en .NET,
  depuis l'archive KoSIT `validator-configuration-xrechnung` v2026-08-31
  qu'elle épingle (`run.sh setup` la télécharge). L'étape des règles métier
  exige Java et SaxonJ-HE 12.10 (README du paquet, l. 23 et 77-95 :
  `SaxonInvoiceRulesRunner("tools/saxon/saxon-he-12.10.jar")`). Elle est
  écartée, puisqu'il n'y a pas de JVM. Ce qui est mesuré : contrôles du modèle
  (`InvoiceModelValidator.ValidateSource`) et XSD (CII D16B, ou XRechnung
  3.0.2 si BT-24 le déclare).
- Les contrôles du modèle sont stricts et ne sont pas des règles EN 16931. Ils
  signalent 23 fichiers sur 26 : INV-WHITESPACE (55 occurrences, espaces en
  début ou fin de texte), INV-PRICE (8), INV-ADJUSTMENT-BASE (6),
  INV-DECLARED-AMOUNT, INV-ATTACHMENT-URI… L'étape XSD ne signale rien.
- Écriture : `InvoiceReadResult.Write` refuse d'écrire un modèle qui échoue à
  ces contrôles. `Write_Small` et `Write_Lines` sont donc `fail`
  (INV-WHITESPACE).
- Extraction : `PdfDocument.Load` + `ExtractAttachments` (`/Names/EmbeddedFiles`
  et `/AF`, flux décodés).
- Injection : les aides Factur-X (fichier associé et XMP) n'existent que sur
  `PdfCreateOptions`, pour les PDF qu'OfficeIMO génère. Les classes que la
  documentation XML mentionne (`PdfAttachmentExtractor`,
  `PdfComplianceAnalyzer`) sont internes : l'API publique a été lue dans les
  métadonnées. Pour un PDF existant, l'adaptateur passe par
  `Attachments.Edit`, avec un fichier associé générique, sans XMP Factur-X.
  - Sur `host.pdf`, le résultat est `PdfMutationBlockedException` : flux xref
    et flux d'objets non réécrits, écriture incrémentale non implémentée pour
    les pièces jointes.
  - Sur `host_classic.pdf`, l'injection réussit.
- CheckPDF : `AssessCompliance(PdfComplianceProfile.FacturX)` relit les
  exigences PDF/A-3 et Factur-X du fichier (XMP, fichier associé, OutputIntent,
  polices…). Le XML n'est pas validé. C'est le seul contrôle de conteneur de
  l'écosystème qui regarde le PDF/A.
- Réseau : aucun (l'archive KoSIT est lue en local).

### FacturXDotNet 0.5.9-alpha (FacturX-NET)

Documents Factur-X (PDF, XMP et factur-x.xml). Le modèle CII se limite à
l'en-tête du profil MINIMUM : vendeur, acheteur, totaux. Lecteur TurboXml,
écrivain, règles en C#, PDF sur le fork FacturXDotNet.PDFsharp
6.2.0-preview-3. API asynchrone : l'adaptateur attend chaque tâche. Dernière
version le 2025-04-21.

- Règles : 19 règles CII
  (`src/FacturXDotNet/Validation/BusinessRules/CII/` : BR-01 à BR-09, BR-13 à
  BR-15, BR-CO-09, BR-CO-26, BR-DEC-12, 13, 14 et 18, BR-S-01) et 16 règles
  hybrides (BR-HYBRID-01 à 15, BR-HYBRID-FR-01).
- Le lecteur refuse 15 des 26 exemples. Pour 11, l'attribut `schemeID` de
  l'identifiant fiscal du vendeur (« FC ») sort de son énumération. Pour 4,
  c'est BT-24 XRechnung (`ArgumentOutOfRangeException`). `Parse`,
  `ParseValidate`, `Write` et `Embed` échouent donc sur les exemples du
  protocole.
- Extraction : factur-x.xml seulement, le nom par défaut
  (`FacturXDocument.cs:83`). Résultat : 70 PDF sur 104. Les 34 autres
  (ZUGFeRD 2.0, XRechnung, Gnuaccounting) nomment autrement leur pièce jointe,
  `Extract_Extended` compris.
- CheckPDF : `FacturXValidator`, règles hybrides et CII, avec BR-HYBRID-FR-01
  (règle française) écartée. Le XML de `EN16931_Einfach.pdf` ne se lit pas,
  d'où `fail` sur `CheckPDF_Small`. `CheckPDF_Large` passe.
- Réseau : aucun.

### pax.XRechnung.NET 0.3.3 (Philipp Hetzner)

Modèle, écrivain et validateurs XRechnung UBL. Pour le CII, la seule fonction
est `ZugferdXmlValidator`, un validateur local ZUGFeRD 2.3.3 / Factur-X
1.07.3 EN 16931. Il applique le XSD Factur-X 1.07.3 EN16931
(`src/pax.XRechnung.NET/ZugferdXmlValidator.cs:111`), puis l'XSLT FeRD
`FACTUR-X_EN16931.xslt` (`:134`), exécuté par SaxonCS-HE 13.0.0. Les deux sont
embarqués.

- N'accepte que BT-24 = `urn:cen.eu:en16931:2017` (`:17`, `:44`). Les 4
  XRECHNUNG sont refusés avec un message (le relevé les compte comme
  signalés), et `ParseValidate_Large` est sauté.
- Signalés : FX-SCH-A-000031 sur les 2 fichiers BR-CL-25.
- Pas de lecteur ni d'écrivain CII, pas de PDF.
- Licence : MIT sur NuGet, GPL-3.0 dans le dépôt (`LICENSE`).
- Réseau : le client du serveur de validation KoSIT
  (`KositValidator/InvoiceKositValidator.cs`, `HttpClient`) n'est pas appelé.

### En16931 0.1.0 (Jonas Fassbender)

Modèle sémantique EN 16931 en C#, lu et écrit en CII et UBL via une
représentation intermédiaire. Les XSLT `IR/cii2ir.xslt` et `IR/ir2cii.xslt`
sont livrés en `contentFiles`. La validation exécute les XSLT officiels avec
SaxonCS-HE 13.1.0 (README du paquet, l. 11-16,
`contentFiles/any/any/En16931.Resources.Extern/`) :

- CEN 1.3.16 ;
- KoSIT XRechnung 3.0.2 / schematron 2.6.0, avec les variantes extension et
  CVD ;
- FeRD Factur-X 1.09.2 BASIC et EN 16931.

Les XSD CII D16B et D22B et UBL 2.1 sont livrés aussi.

- Spécification choisie par BT-24 : `FacturXEn16931` pour
  `urn:cen.eu:en16931:2017` (règles CEN et Factur-X EN 16931), `XRechnung`
  pour XRechnung 3.0. Pas de MINIMUM, BASIC WL ni EXTENDED. Pas de PDF.
- Lecture : 18 fichiers sur 26.
  - « XmlException: 'Element' is an invalid XmlNodeType » (3), dont
    `EN16931_Rabatte`, d'où `Parse_Lines` et `Write_Lines` en `fail`.
  - « ReadContentAsString … not supported » (3).
  - `ValidationException` (2) : `Parser.Parse` valide d'abord.
- Validation : FX-SCH-A-000571 sur les 2 fichiers BR-CL-25. Les XRECHNUNG
  passent par la spécification XRechnung sans les niveaux personnalisés KoSIT :
  CII-SR-465 reste un warning.
- Publié le 2026-11-05 (une seule version, 72 téléchargements).
- Réseau : aucun.

### Antiphon.Invoicing 1.0.0 (Clockwork Otter Foundry)

Logiciel commercial et fermé. La licence d'évaluation est gratuite et complète,
sans clé ni limite de durée (`Antiphon-Evaluation-License-Notice.md` du
paquet), et réservée à l'évaluation avant achat. Toutes les versions sont
publiées le 2026-09-18. Pas de sources : les preuves viennent de l'API
publique lue dans les métadonnées et de la documentation XML des paquets.

- Découpage : `Antiphon.Invoicing` ne contient que le modèle (30 types).
  L'adaptateur prend aussi :
  - `Parsing.Cii` (`CiiInvoiceParser.Parse`) et `Generation.Cii`
    (`CiiInvoiceGenerator.GenerateXml`) ;
  - `Validation.Pipeline` (`LayeredValidator`), `Rules.Cen.Cii`,
    `Rules.XRechnung.Cii` et `Xsd.Cii` ;
  - `Pdf` (`FacturXExtractor`, `FacturXEmbedder`, sur PDFsharp 6.2.4).

  `Rules.Peppol.Cii` et les paquets UBL ne servent pas ici.
- Validation : bien-formé, puis XSD CII D16B, puis les jeux Schematron que
  `FormatDetector` déduit de BT-24 (`cii-en16931` → `cen-cii`, `cii-xrechnung`
  → `cen-cii` et `xrechnung-cii`). Les jeux sont livrés précompilés dans une
  représentation intermédiaire et évalués par un évaluateur XPath écrit en C#,
  sans XSLT ni Java. Ils sont désérialisés une fois, au premier appel.
- Jeux livrés : CEN `validation-1.3.16` (806 assertions) et KoSIT XRechnung
  schematron v2.6.0 (74 assertions), selon les constantes `RuleSetInfo`. Il
  n'y a pas de jeu Factur-X (FeRD) ni de jeu français. Les niveaux personnalisés
  KoSIT ne s'appliquent pas : CII-SR-465 reste un warning, d'où 2 fichiers
  signalés sur 26 (BR-CL-25 ×2).
- Erreurs comptées : `OfficialFatalViolations`, c'est-à-dire les violations
  fatales du XSD et des jeux officiels.
- Lecture : 26 sur 26. Les éléments non repris sont rendus à part
  (`UnmappedElements`) plutôt que perdus.
- Écriture : `GenerateXml(Invoice, Comfort)` depuis le modèle lu.
- Extraction : `FacturXExtractor.ExtractBytes`, noms factur-x.xml,
  zugferd-invoice.xml, ZUGFeRD-invoice.xml, xrechnung.xml et order-x.xml, puis
  sans casse. 104 PDF sur 104.
- Injection : `FacturXEmbedder.Embed`. Le contrat documenté exige un hôte déjà
  PDF/A : la librairie n'ajoute que la couche pièce jointe, `/AF` et XMP
  PDF/A-3 + Factur-X, et ne vérifie pas l'hôte. Elle réussit sur les deux
  hôtes.
- Pas de contrôle de conteneur : CheckPDF est `unsupported`.
- Licence : seuls les points d'entrée simples sont appelés, pas les variantes
  `*WithLicense`, et aucune clé n'est lue.
- Réseau : aucune référence à `System.Net.Http`, `HttpClient` ou aux sockets
  dans les assemblages. Seul `Antiphon.Xsd.Core` référence `System.Net`, pour la
  propriété `Credentials` de son `XmlResolver`, qui lit les XSD embarqués.

### SaxonCS-HE 13.1.0 (Saxonica), référence

SaxonJ-HE transpilé en C# : ni Java, ni IKVM. C'est la référence XSLT 2.0+
hors ligne hors Java, l'équivalent de saxonche dans le banc Python.

- XSLT officiels CEN EN 16931 CII 1.3.17 pour toutes les factures. On y ajoute
  KoSIT XRechnung 3.0.2 schematron 2.6.0 quand BT-24 déclare XRechnung, avec
  les niveaux personnalisés de `validator-configuration-xrechnung`
  v2026-08-31 : CII-SR-465 et 466 passent en erreur, CII-SR-475 et 476 en
  information.
- Résultat : 6 fichiers sur 26 (BR-CL-25 ×2, CII-SR-465 ×4), identique à la
  référence du protocole.
- Artefacts téléchargés par `run.sh setup`, SHA-256 contrôlés, mêmes archives
  que le banc Python. Les stylesheets sont compilées au premier appel, ce qui
  est compris dans `first_call_ns`.
- `InvariantGlobalization` casse SaxonCS. La culture invariante est donc fixée
  dans le code du harnais.

### System.Xml.Schema (plancher XSD)

`XmlSchemaSet` de la BCL, compilé une fois, et un `XmlReader` validant, contre
les XSD UN/CEFACT CII D16B de la configuration KoSIT v2026-08-31
(`resources/cii/16b/xsd`). Pas une librairie Factur-X : il montre le coût de
la couche schéma que la plupart des librairies placent devant leurs règles.
Aucun des 26 exemples n'est signalé.

### PdfPig 0.1.16 (plancher)

Lecteur PDF en C# le plus téléchargé de NuGet. L'extraction est écrite dans
`libs/pdfpig/Program.cs` : `PdfDocument.Open`, puis
`Advanced.TryGetEmbeddedFiles` (arbre `/Names/EmbeddedFiles` avec `/Kids`,
flux décodés). On prend le premier fichier qui porte un nom de facture
standard, avec la même liste de noms que le plancher Rust.

### PDFsharp 6.2.4 (plancher)

La librairie PDF sous ZUGFeRD.PDF-csharp (fork PDFsharp-extended), Balsoft,
International.EInvoicing, FacturXDotNet (fork) et Securibox (7.0 preview). Le
parcours `/Root/Names/EmbeddedFiles` avec `/Kids` est écrit à la main dans
`libs/pdfsharp/Program.cs`. 6.2.4 est la dernière version MIT : 6.5.0
(2026-10-14) et 7.0.0-preview-2 sont sous AGPL-3.0-only.

## Librairies écartées

| Paquet | Version (téléchargements) | Raison |
|---|---|---|
| ZUGFeRD.NET | 1.0.1 (22 533) | Bibliothèque Java Konik compilée par IKVM |
| Saxon-HE | 10.9.0 (2 203 102) | Saxon Java sur IKVM, pour .NET Framework |
| SaxonHE10Net31, SaxonHE12s9apiExtensions | 10.9.21 (101 120), 12.10.1 (103 595) | Saxon Java compilé par IKVM ; SaxonCS-HE couvre le besoin sans Java |
| Lefty.Schematron | 0.0.16 (1 635) | Outil Schematron qui s'appuie sur Saxon Java (IKVM) |
| Eunormia.Sdk | 1.0.1 (489) | Embarque un JRE |
| OfficeIMO.Invoicing.Validation, étape des règles | 3.6.2 | Java et SaxonJ-HE 12.10 ; seule l'étape XSD est mesurée |
| FactoorSharp.FacturX (+PDF, Validator) | 21.0.0 (8 140) | Commerciale ; la licence contacte un serveur |
| Facturexa.* | 7.6.1 (700) | Commerciale : clé de licence ou essai de 30 jours |
| AloahaZUGFeRDToolkit | 11.0.0.16 (100 402) | Commerciale, sans licence d'évaluation utilisable hors ligne |
| itext7.pdfinvoice | 2.0.1 (10 700) | Commerciale (iText), clé de licence ; dernière version en 2018 |
| Access.It.Pdf.Facturx | 10.12.1 (12 006) | Aucune licence déclarée, donc aucun droit d'usage |
| FacturX-csharp | 1.0.6 (9 319) | Dérivé de ZUGFeRD-csharp, déjà mesuré |
| PROBIS-ZUGFeRD (+PDF, CDAR) | 19.0.4 (307) | Fork de ZUGFeRD-csharp, déjà mesuré |
| Lombiq.EInvoiceValidator | 2.0.1 (6 435) | Exige Node.js |
| InvoiceXml.FacturX, InvoiceXml.Zugferd.XRechnung | 1.5.0 (738, 769) | Clients d'une API distante (exclus par le protocole) |
| StanzaApi.FacturXValidator, StanzaApi.PeppolValidator | 1.0.0 (141, 139) | Clients d'une API distante |
| Xingen.Sdk, Fakturnaut.Sdk, EInvoice.Sdk, Falco.Api | 0.2.0 (370), 1.0.1 (214), 0.1.0 (178), 1.0.4 (1 186) | Clients d'une API distante |
| Recommand.Client, EConnect.Psb | 0.4.2 (4 882), 0.9.35 (30 168) | Clients d'API Peppol distantes |
| SW-ZUGFeRD | 1.0.0 (430) | Paquet vide (net461) |
| csv2zugferd | 1.3.1 (218) | Outil en ligne de commande |
| PdfPinata.EInvoice | 0.6.1-alpha.26 (1 274) | Injection seulement, dans un document PdfPinata, en préversion alpha publiée le jour du relevé ; ni lecture ni validation |
| Schematron, XSchematron, Xsl3, Bosak.Xslt, OutSmart.DAXon, XPath2 | 1.0.0 (28 247), 0.2.3 (942)… | Moteurs génériques sans artefacts de facture (XPath2 est mesuré à travers Securibox) |
| RemitExtract | 0.1.1 (294) | Extraction de données de factures PDF et QR-factures, hors sujet |
| Paquets nationaux (OioUbl, ZATCA, FatturaPA…) et UBL seuls | — | Hors sujet pour un corpus Factur-X / XRechnung CII |
| Antiphon.Invoicing.Rules.Peppol.Cii, paquets UBL d'Antiphon | 1.0.0 (130 à 210) | Composants d'Antiphon que le corpus n'utilise pas : aucun exemple ne déclare Peppol, et le corpus est en CII |

Le seul usage de Java concerne les candidats écartés et l'étape des règles
d'OfficeIMO. SaxonCS-HE, utilisé par pax.XRechnung.NET, En16931 et la
référence, est du C#.

## Relevés d'exactitude (`./run.sh survey`)

| Librairie | validate (signalés / 26) | parse (/ 26) | extract (/ 104) |
|---|---|---|---|
| zugferd-csharp | 3 (faux positifs sur les totaux : Teilrechnung, Kraftfahrversicherung, Sachversicherung) | 26 | 103 (avec lecture du modèle) |
| international-einvoicing | 2 (BR-CL-25 ×2) | 26 | 104 |
| balsoft | 5 (BR-CL-25 ×2 ; écarts BT-106/109/112/115 sur 3 exemples valides) | 26 | 104 |
| securibox | **6** (mêmes fichiers que la référence : CII-SR-465 et `codedb cl1` ×4 sur les XRECHNUNG, `codedb cl17` ×2) | 26 | 74 (XmpException ×29) |
| officeimo | 23 (contrôles du modèle : INV-WHITESPACE, INV-PRICE… ; XSD sans erreur) | 26 | 104 |
| facturxdotnet | 15 (erreurs de lecture : schemeID « FC », BT-24 XRechnung) | 11 | 70 (factur-x.xml seulement) |
| pax-xrechnung | 6 (FX-SCH-A-000031 ×2 ; 4 XRECHNUNG refusés) | — | — |
| en16931 | 2 (FX-SCH-A-000571 ×2) | 18 | — |
| antiphon | 2 (BR-CL-25 ×2) | 26 | 104 |
| saxoncs (référence) | **6** (BR-CL-25 ×2, CII-SR-465 ×4) | — | — |
| xsd (plancher) | 0 | — | — |
| pdfpig (plancher) | — | — | 104 |
| pdfsharp (plancher) | — | — | 104 |

Référence (PROTOCOL) : 6 sur 26. SaxonCS-HE la retrouve exactement. Securibox
signale les mêmes fichiers, mais par artefact (voir sa fiche).

## Statut des benchmarks (essai `COUNT=1 BENCHTIME=0.05`)

`o` ok, `F` fail (message dans la note de la ligne JSONL), `-` unsupported.
Colonnes dans l'ordre :

1. Extract S/L/Min/Ext
2. Parse S/Li/La
3. ParseValidate S/Li/La
4. Write S/Li
5. Embed, EmbedClassic
6. CheckPDF S/L

| Librairie | Extract | Parse | ParseValidate | Write | Embed | CheckPDF |
|---|---|---|---|---|---|---|
| zugferd-csharp | o o o o | o o o | o o o | o o | o o | - - |
| international-einvoicing | o o o o | o o o | o o o | o o | o o | - - |
| balsoft | o o o o | o o o | o o o | o o | o o | - - |
| securibox | o o o F | o o o | o o o | o o | o o | o o |
| officeimo | o o o o | o o o | o o o | F F | F o | o o |
| facturxdotnet | o o o F | F F F | F F F | F F | F F | F o |
| pax-xrechnung | - - - - | - - - | o o - (sauté) | - - | - - | - - |
| en16931 | - - - - | o F o | o o o | o F | - - | - - |
| antiphon | o o o o | o o o | o o o | o o | o o | - - |
| saxoncs | - - - - | - - - | o o o | - - | - - | - - |
| xsd | - - - - | - - - | o o o | - - | - - | - - |
| pdfpig | o o o o | - - - | - - - | - - | - - | - - |
| pdfsharp | o o o o | - - - | - - - | - - | - - | - - |

Les deux hôtes du protocole portent déjà un `/AF` vers un factur-x.xml
(Filespec du 2022-02-18). OfficeIMO refusait d'ajouter un second fichier du
même nom : l'adaptateur le remplace (`Replace`). Les autres librairies
réécrivent ou complètent l'entrée.

## Tableau de couverture

| Librairie | Version | Licence | Dernière version | Téléchargements | Lecture CII | Écriture CII | UBL | Extraction PDF | Injection PDF | XSD | EN 16931 (CII) | Profils Factur-X (MINIMUM, BASIC WL, EXTENDED) | BR-FR | BR-DE (XRechnung) | Peppol BIS 3 | Contrôle du conteneur PDF/A-3 | Dépendances externes |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| ZUGFeRD-csharp (+PDF) | 18.0.1 | Apache-2.0 | 18.0.1 (2026-10-09) ; fin de maintenance le 2026-11-30 | 1 842 187 ; PDF 359 076 | oui (ZUGFeRD 1.0, 2.0, 2.3) | oui (tous profils) | oui (XRechnung UBL, lecture et écriture) | oui (103/104, avec lecture du modèle) | oui (réécriture, XMP, OutputIntent, polices système) | non | partiel : recalcul des totaux (lignes, BR-CO-14, BR-CO-17…) | lecture et écriture de tous les profils ; pas de règles de profil | non | non (écriture XRechnung seulement) | non | non | PDFsharp-extended 1.3.0 ; polices système à l'injection |
| International.EInvoicing | 1.1.0 | MIT | 1.1.0 (2026-09-07) | 10 807 ; FacturX.PdfSharp 5 014 | oui | oui | oui | oui (104/104) | oui (fichier associé, XMP ajouté ou complété) | oui : CII D16B | oui : `.sch` CEN 1.3.16 prétraité (806 assertions), moteur maison | hors ligne non ; règles FeRD à charger depuis un dossier | non (`AddFrance`, non chargé) | partiel : schematron KoSIT « 3.0 » pris sur `master` (52 assertions), sans niveaux personnalisés | paquet séparé, non chargé | non (XMP comparé au XML) | PDFsharp 6.2.4 (MIT) |
| Balsoft.Hive.EInvoice (+Pdf) | 1.1.0 | Apache-2.0 | 1.1.0 (2026-10-23) | 626 ; Pdf 563 | oui | oui | oui | oui (104/104) | oui (réécriture, OutputIntent, XMP incrémental) | non | partiel : règles codées en C# (115 identifiants BR), totaux recalculés | partiel : profils contrôlés en C# | non | partiel : 26 BR-DE codées en C# | partiel : 4 règles PEPPOL-EN16931 | non | PDFsharp 6.2.4 ; polices système à l'injection |
| Securibox.FacturX | 0.6.0-alpha-3 | MIT (fichier de licence) | 0.6.0-alpha-3 (2026-09-04) | 20 644 | oui (XmlSerializer, un modèle par profil) | partiel (sérialisation publique seulement à travers le PDF) | non | partiel (74/104 ; validation incluse) | oui (réécriture, validation préalable) | oui : XSD Factur-X 1.09 par profil | oui : à travers les schematrons FeRD 1.09 | oui : schematrons FeRD 1.09 des 5 profils (424 assertions en EN 16931) | non | non | non | partiel : `IsFacturXValid` (XMP et XML), pas de PDF/A | PDFsharp 7.0.0-preview-1, XmpCore 6.1.10.1 (licence Adobe), XPath2 1.1.5 (MS-PL) |
| OfficeIMO.Invoicing (+Validation, +Pdf) | 3.6.2 | MIT | 3.6.2 (2026-11-18) | 2 524 ; Validation 1 766 ; Pdf 213 774 | oui (borné) | partiel (refuse un modèle qui échoue à ses contrôles) | oui | oui (104/104) | partiel (fichier associé générique sans XMP ; refusé sur `host.pdf`) | oui : XSD de l'archive KoSIT v2026-08-31 | non sans Java ; contrôles du modèle seulement | non sans Java | non | non sans Java | non sans Java | partiel : `AssessCompliance(FacturX)` (XMP, fichier associé, OutputIntent, polices), sans le XML | archive KoSIT téléchargée ; Java et SaxonJ-HE pour les règles (non utilisés) |
| FacturXDotNet | 0.5.9-alpha | MIT | 0.5.9-alpha (2025-04-21) | 6 411 | partiel (en-tête MINIMUM, 11/26) | partiel (MINIMUM) | non | partiel (factur-x.xml seulement, 70/104) | oui (XMP généré depuis le XML) | non | partiel (19 règles du profil MINIMUM) | MINIMUM seulement | partiel (BR-HYBRID-FR-01, écartée ici) | non | non | partiel : 15 règles hybrides (XMP, nom et relation du fichier), pas de PDF/A | fork FacturXDotNet.PDFsharp 6.2.0-preview-3 |
| pax.XRechnung.NET | 0.3.3 | MIT (NuGet) ; GPL-3.0 (dépôt) | 0.3.3 (2026-10-10) | 3 143 | non | non | oui (modèle XRechnung UBL) | non | non | oui : XSD Factur-X 1.07.3 EN16931 | oui : à travers l'XSLT FeRD 1.07.3 EN16931 | EN 16931 seulement (BT-24 `urn:cen.eu:en16931:2017` exigé) | non | non pour le CII | non | non | SaxonCS-HE 13.0.0 (MPL-2.0), ICU4N |
| En16931 | 0.1.0 | EUPL-1.2 | 0.1.0 (2026-11-05) | 72 | partiel (18/26) | partiel | oui (via la représentation intermédiaire) | non | non | XSD livrés (CII D16B et D22B, UBL 2.1) | oui : XSLT officiel CEN 1.3.16 | partiel : FeRD 1.09.2 BASIC et EN 16931 | non | oui : KoSIT 3.0.2 / 2.6.0 (et extension, CVD), sans niveaux personnalisés | non | non | SaxonCS-HE 13.1.0 (MPL-2.0) |
| Antiphon.Invoicing (+ Parsing.Cii, Generation.Cii, Validation.Pipeline, Rules.Cen.Cii, Rules.XRechnung.Cii, Xsd.Cii, Pdf) | 1.0.0 | commerciale, fermée ; évaluation gratuite sans clé | 1.0.0 (2026-09-18) | 202 ; sous-paquets 130 à 144 | oui (éléments non repris signalés) | oui (Comfort, Basic, BasicWithoutLines, Minimum) | oui (paquets séparés, non mesurés) | oui (104/104) | oui (fichier associé, XMP PDF/A-3 ; hôte PDF/A supposé) | oui : CII D16B | oui : Schematron CEN 1.3.16 précompilé (806 assertions), évaluateur maison | génération des profils ; pas de règles FeRD | non | oui : KoSIT schematron v2.6.0 (74 assertions), sans niveaux personnalisés | paquet séparé (Rules.Peppol.Cii), non chargé | non | PDFsharp 6.2.4 (MIT), System.Security.Cryptography.Pkcs (licence) |
| SaxonCS-HE (référence) | 13.1.0 | MPL-2.0 | 13.1.0 (2026-10-15) | 69 586 | non | non | non | non | non | non | oui : XSLT officiel CEN 1.3.17 | non | non | oui : KoSIT 2.6.0 et niveaux v2026-08-31 | non | non | artefacts officiels téléchargés |
| System.Xml.Schema (plancher) | BCL .NET 11.0.0 | MIT | — | — | non | non | non | non | non | oui : CII D16B (archive KoSIT) | non | non | non | non | non | non | aucune |
| PdfPig (plancher) | 0.1.16 | Apache-2.0 | 0.1.16 (2026-08-22) ; préversions 0.1.17-alpha | 47 810 678 | non | non | non | plancher écrit à la main | non | non | non | non | non | non | non | non | aucune |
| PDFsharp (plancher) | 6.2.4 | MIT (6.5.0 et suivantes : AGPL-3.0-only) | 6.5.0 (2026-10-14, AGPL) ; 6.2.4 (2026-01-06), dernière MIT | 85 920 430 | non | non | non | plancher écrit à la main | non | non | non | non | non | non | non | non | aucune |

## Notes de construction et de mesure

- Un projet par librairie, restauré (`dotnet restore --locked-mode`) et
  construit (`dotnet build -c Release`) à part. Ainsi un projet qui ne se
  construit pas n'arrête pas les autres, et chaque processus ne charge que ses
  propres dépendances. Les sorties vont dans `artifacts/`
  (`UseArtifactsOutput`), les artefacts officiels dans `.artefacts/`.
- Exécution par le muxer (`dotnet artifacts/bin/<lib>/release/bench-<lib>.dll`)
  plutôt que par l'apphost, qui ne trouve pas un .NET installé par Homebrew
  sans `DOTNET_ROOT`.
- Réglages du runtime : ceux de .NET par défaut (compilation étagée, PGO
  dynamique, GC station de travail concurrent). Seul écart : la culture
  invariante est fixée dans le code du harnais (`harness/Harness.cs:743-746`).
  `InvariantGlobalization` aurait le même effet sur les nombres, mais casse
  SaxonCS.
- Le corps de chaque programme tourne dans un fil à pile de 256 Mio
  (`harness/Harness.cs:726`) : les stylesheets officielles récursent
  profondément. Un seul fil de mesure, pas de GC forcé. Chaque résultat passe
  par `Sink.Consume`, ce qui empêche l'élimination du code mort.
- Temps CPU (`cpu_ns_per_op`) : `Environment.CpuUsage.TotalTime` (`getrusage`
  sur macOS, tous les fils du processus), lu juste avant et juste après chaque
  série. Relevé court de la passe `bench` (`COUNT=2 BENCHTIME=0.5`) : le CPU
  reste dans ±20 % du temps réel. Écarts jusqu'à ×1,6 (Securibox, En16931,
  Balsoft…), dus à la compilation par paliers en arrière-plan et au GC. Seule
  exception nette, ZUGFeRD-csharp sur `Embed_Small` et `EmbedClassic_Small` :
  ×7 à ×8. À chaque appel, une vingtaine de fils du pool (`.NET TP Worker`)
  parcourent et ouvrent en parallèle des fichiers des dossiers système, à la
  recherche des polices à embarquer (`open`, `lstat`, `fstat`, `flock`,
  `pread` dans un relevé `sample`). Ce travail compte dans le CPU du
  processus, pas dans celui du fil mesuré.
- `./run.sh resources` : un processus par librairie et par benchmark,
  `dotnet artifacts/bin/<lib>/release/bench-<lib>.dll resources <Bench>`. C'est
  la commande de `bench`, lancée directement sous `/usr/bin/time -l` (pic de
  RSS en octets). Le muxer charge le runtime dans son propre processus
  (hostfxr, hostpolicy, coreclr). Le `dotnet` de Homebrew est un script qui
  fait `exec` du muxer : le PID mesuré est donc celui du runtime. Pas de
  processus enfant. Deux assemblages seulement référencent `ProcessStartInfo` :
  OfficeIMO.Invoicing.Validation, pour l'exécuteur Java de son étape des
  règles, que l'adaptateur ne configure pas, et PdfSharp.Quality, pour des
  aides de développement. Ni l'un ni l'autre n'est appelé.
- `first_call_ns` comprend la compilation JIT et les compilations paresseuses :
  stylesheets (SaxonCS-HE, pax, En16931), schémas (`XmlSchemaSet`, OfficeIMO)
  et règles (International.EInvoicing, Antiphon).
- Revue de sécurité avant construction : cibles MSBuild importées par les
  paquets (`obj/*.nuget.g.targets`).
  - Cibles de compatibilité Microsoft ;
    `Microsoft.Extensions.Configuration.Binder` ; `ICU4N.Resources.targets`
    (dépendance de pax).
  - Ce dernier importe `CopyPatch.targets`, une tâche en ligne
    RoslynCodeTaskFactory. Elle copie les DLL de ressources satellites et met à
    jour `.deps.json` avec le Newtonsoft.Json qu'elle embarque. Pas d'accès
    réseau ; avec `SatelliteResourceLanguages=en`, elle ne copie rien.
  - Le dossier `build/` de DotNet.Bundle ne s'active que sur une cible
    `BundleApp` explicite.
  - Aucun autre paquet n'exécute de code à la construction.
- Les deux hôtes du protocole (`host.pdf`, `host_classic.pdf`) ont déjà un
  `/AF` vers un factur-x.xml. `PROTOCOL.md` décrit `host.pdf` comme « sans
  pièce jointe ». La vérification d'`Embed_*` relit l'XML injecté et le
  compare au profil CII, pas au contenu exact : une librairie qui laisserait
  l'ancien fichier en place passerait.
