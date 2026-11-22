# Conformité

Ce dossier mesure l'exactitude du package sur les artefacts officiels, à chaque push et à chaque pull request, dans le workflow [Conformité](../.github/workflows/conformance.yml). Rien ici ne fait partie du package `zerechnung`, qui reste en pur Go : Java ne sert qu'à exécuter les validateurs de référence dans la CI.

| Fichier | Rôle |
|---|---|
| [`fetch.sh`](fetch.sh) | Récupère les artefacts officiels à des versions figées et les vérifie, puis affiche les variables `FX_*` qui activent les tests optionnels |
| [`fetch-nl.sh`](fetch-nl.sh) | Récupère les seuls artefacts des règles néerlandaises (`FX_NL`, `FX_NL_TESTS`) ; `fetch.sh` l'appelle, et le job *Tests NLCIUS sur les artefacts officiels* du workflow [CI](../.github/workflows/ci.yml) l'exécute seul, en quelques secondes |
| [`verapdf.sh`](verapdf.sh) | Convertit en PDF/A-3 les fichiers PDF/A-2b et PDF/A-3b du corpus veraPDF et les PDF non conformes des tests, puis vérifie les fichiers convertis avec veraPDF |
| [`parity.sh`](parity.sh) | Exécute KoSIT et Mustang sur les factures du corpus et de la suite KoSIT, KoSIT sur les variants des tests de mutation EN 16931 et XRechnung, les XSLT de la NPa sur les factures NLCIUS, les XSLT EXTENDED-CTC-FR de la FNFE-MPE sur les factures EXTENDED-CTC-FR et le schematron CIUS-PT d'eSPap, compilé par SchXslt, sur les factures CIUS-PT, puis compare leurs règles à celles du package |
| [`parity/`](parity) | Outil Go de comparaison (module séparé) et lanceurs Java de Mustang, des XSLT de la NPa et de la FNFE-MPE, et du schematron CIUS-PT |
| [`parity/known.txt`](parity/known.txt) | Écarts connus et justifiés, règles attendues par un variant que la référence ne lève pas, et assertions NLCIUS qu'aucune facture ne déclenche ; tout autre écart fait échouer le job |
| [`qr/`](qr) | Tests Go (module séparé) qui font lire les QR codes du package `internal/qr` par gozxing, portage de ZXing, et par le décodeur de piglig/go-qr, dont l'encodeur est issu ; job *Lecture des QR codes par deux décodeurs* |
| [`signature.sh`](signature.sh) | Fait vérifier par OpenSSL, xmlsec1 et EU DSS les signatures CAdES et XAdES du package `fatturapa`, et fait vérifier par le package les fichiers que ces outils signent |
| [`signature/`](signature) | Projet Maven d'EU DSS 6.5.1 et son lanceur [`DssCheck.java`](signature/DssCheck.java), qui valide et signe |

## Artefacts

`fetch.sh` vérifie chaque dépôt par son commit et chaque fichier téléchargé par son SHA-256 ; il garde ce qui est déjà présent et vérifié. Les dépôts phive-rules, qui embarque les fichiers de test de tous ses jeux de règles (1,4 Go), et veraPDF-corpus, qui couvre aussi PDF/A-1, PDF/A-4 et PDF/UA, ne sont récupérés que pour les dossiers utiles, par un clone partiel (`fetch_git_sparse` de [`lib.sh`](lib.sh)).

| Artefact | Version | Vérification | Variable |
|---|---|---|---|
| [ZUGFeRD/corpus](https://github.com/ZUGFeRD/corpus) | commit `84f8d8f` | commit | `FX_CORPUS` |
| XML des PDF valides du corpus, extraits par `pdfdetach` (poppler-utils) | — | — | `FX_CORPUS_XML` |
| [itplr-kosit/xrechnung-testsuite](https://github.com/itplr-kosit/xrechnung-testsuite) | `v2026-08-31` (XRechnung 3.0.2), commit `44c3905` | commit | `FX_KOSIT` |
| [Configuration du validateur KoSIT](https://github.com/itplr-kosit/validator-configuration-xrechnung/releases/tag/v2026-08-31) XRechnung 3.0.2 | release 2026-08-31 | SHA-256 | `FX_KOSIT_CONF` |
| [fnfempe/France_RFE](https://github.com/fnfempe/France_RFE) | `v1.4.0.04`, commit `97ba0f3` | commit | `FX_FNFE` |
| [OpenPEPPOL/peppol-bis-invoice-3](https://github.com/OpenPEPPOL/peppol-bis-invoice-3) | `3.0.22`, commit `38d693d` | commit | `FX_PEPPOL` |
| [peppolautoriteit-nl/validation](https://github.com/peppolautoriteit-nl/validation) (artefacts de l'autorité Peppol néerlandaise, licence MIT) | release 2026-05-21 (SI-UBL 2.0.3.13, NLCIUS-CII 1.0.3.13, extension G-rekening 1.0.2.13), commit `4e93ae8` | commit | `FX_NL` |
| Fichiers de test SimplerInvoicing de [phax/phive-rules](https://github.com/phax/phive-rules) (`phive-rules-simplerinvoicing/src/test/resources/external/test-files/simplerinvoicing`, Apache-2.0) | commit `bab0629` | commit (clone partiel de ce seul dossier) | `FX_NL_TESTS` |
| [veraPDF/veraPDF-corpus](https://github.com/veraPDF/veraPDF-corpus) (CC BY 4.0) | branche `staging`, commit `bb75f4f` | commit (clone partiel de `PDF_A-2b` et `PDF_A-3b`) | `FX_VERAPDF` |
| Paquet OASIS [UBL-2.1.zip](https://docs.oasis-open.org/ubl/os-UBL-2.1/UBL-2.1.zip) | 2.1 | SHA-256 | `FX_UBL_ZIP` |
| XSD Factur-X : BASIC, EN 16931 et EXTENDED du paquet FeRD ZUGFeRD 2.5.2, MINIMUM et BASIC WL du paquet FNFE-MPE 1.09 | 1.09.2 | SHA-256 de chacun des 20 fichiers | `FX_XSD`, `FX_XSD_DIR` |
| Schematrons Factur-X de BASIC, EN 16931 et EXTENDED et leurs listes de codes (`FACTUR-X_<profil>.sch`, `FACTUR-X_<profil>_codedb.xml`) du paquet FeRD ZUGFeRD 2.5.2 | 1.09.2 | SHA-256 de chacun des 6 fichiers | `FX_FACTURX_SCH` |
| Six exemples FatturaPA 1.2 de l'Agenzia delle Entrate ([fatturapa.gov.it](https://www.fatturapa.gov.it/it/norme-e-regole/documentazione-fattura-elettronica/formato-fatturapa/), `IT01234567890_FPA01.xml` à `FPA03` et `FPR01` à `FPR03`), que leurs conditions d'utilisation interdisent de copier dans le dépôt | 1.2 | SHA-256 de chacun des 6 fichiers | `FX_FATTURAPA` |
| Schémas XML FatturaPA de l'Agenzia delle Entrate ([fatturapa.gov.it](https://www.fatturapa.gov.it/it/norme-e-regole/documentazione-fattura-elettronica/formato-fatturapa/), `Schema_VFPA12_v1.2.4.xsd` dont `internal/schema/fatturapa_tables_gen.go` est généré, et `Schema_VFPA12_V1.2.3.xsd`, en vigueur jusqu'au 10/01/2027) et schéma xmldsig du W3C ([`xmldsig-core-schema.xsd`](https://www.w3.org/TR/2002/REC-xmldsig-core-20020212/xmldsig-core-schema.xsd)) qu'ils importent ; xmllint (libxml2-utils) y vérifie les mêmes documents que le paquet | 1.2.4 et 1.2.3 | SHA-256 de chacun des 3 fichiers | `FX_FATTURAPA_XSD` |
| Schémas XML Facturae 3.2, 3.2.1 et 3.2.2 de [facturae.gob.es](https://www.facturae.gob.es/) (`Facturaev3_2.xml`, `Facturaev3_2_1.xml`, `Facturaev3_2_2.xml`), dont `internal/schema/facturae_tables_gen.go` est généré ; origine des données : Ministerio de Hacienda y Administraciones Públicas - Ministerio de Industria, Energía y Turismo | 3.2.2 | SHA-256 de chacun des 3 fichiers | `FX_FACTURAE_XSD` |
| Exemples CIUS-PT d'eSPap (page [Normas](https://www.espap.gov.pt/spfin/normas/Paginas/normas.aspx), document 254 : 13 factures, une note de débit et une note de crédit), que leurs conditions d'utilisation, non publiées, ne permettent pas de copier dans le dépôt | 2.1.1 | SHA-256 de l'archive | `FX_PT` |
| Schematron CIUS-PT d'eSPap (même page, document 267), dont `internal/rules/pt/rules_gen.go` est généré par `internal/rules/pt/gen`, qui vérifie la même somme | 2.1.1 | SHA-256 de l'archive | `FX_PT_SCH` |

FeRD et FNFE-MPE ne distribuent leurs paquets qu'au travers d'un formulaire : les 20 XSD et les 6 fichiers de schematron viennent de deux copies publiques de ces paquets sous Apache-2.0 ([LandrixSoftware/validator-configuration-zugferd](https://github.com/LandrixSoftware/validator-configuration-zugferd), [backoffice-plus/e-invoice-validator](https://github.com/backoffice-plus/e-invoice-validator)), à des commits figés. Chaque XSD est vérifié par le SHA-256 du fichier officiel, celui-là même dont `internal/schema/tables_gen.go` a été généré. Les schematrons et leurs listes de codes sont ceux que Mustang 2.26.0 embarque (`schematron/ZF_250`) et que contient le paquet officiel, aux fins de ligne près (CRLF dans le paquet et dans Mustang, LF dans la copie git) : leur SHA-256, sur le texte en LF, est vérifié par `fetch.sh` et par le générateur `internal/rules/facturx/gen`, dont `codedb_gen.go` est la sortie.

Les validateurs de référence, téléchargés par `parity.sh` depuis leurs publications GitHub, et Saxon et SchXslt depuis Maven Central, sont vérifiés de la même façon ; KoSIT s'exécute avec la configuration XRechnung 3.0.2 du 2026-08-31 récupérée par `fetch.sh`, Saxon avec les XSLT du dépôt de la NPa (`FX_NL`), avec les XSLT EXTENDED-CTC-FR du paquet France_RFE (`FX_FNFE`) et avec celle que SchXslt compile du schematron CIUS-PT (`FX_PT_SCH`) :

| Validateur | Fichier | SHA-256 |
|---|---|---|
| KoSIT validator 1.6.3 | `validator-1.6.3-standalone.jar` | `799e64be…` |
| Mustang 2.26.0 | `Mustang-CLI-2.26.0.jar` | `42d7868c…` |
| Saxon-HE 12.10 | `Saxon-HE-12.10.jar` | `b571af28…` |
| XML Resolver 5.3.3, que Saxon 12 demande | `xmlresolver-5.3.3.jar`, `xmlresolver-5.3.3-data.jar` | `1fe4d5b9…`, `b0c487ad…` |
| SchXslt 1.10.1, qui compile le schematron CIUS-PT en XSLT | `schxslt-1.10.1.jar` | `4f4f21ed…` |

## Tests sur les artefacts

Le job *Tests sur les artefacts officiels* lance `go test ./...` avec toutes les variables `FX_*` définies (voir la section Tests du [README](../README.md#tests)) et échoue si un test est ignoré : chaque test optionnel tourne donc à chaque pull request.

### Règles levées par la suite

Le job compile aussi la suite avec l'étiquette `ruletrace` : chaque binaire de test écrit alors dans `rules.txt` les règles des findings qu'il rapporte (`ZERECHNUNG_RULE_TRACE`, voir [`internal/rules/trace_ruletrace.go`](../internal/rules/trace_ruletrace.go)), et `TestRuleCoverage` ([`rulecoverage_test.go`](../rulecoverage_test.go)) vérifie ensuite que chaque règle des catalogues (`validate.Catalog` de tous les jeux de règles et `ContainerCatalog`), plus `XML`, `CII` et les règles de `fatturapa.VerifySignature`, est levée par au moins un test. C'est la version dynamique des tests `TestCatalogComplete`, qui lisent les sources : une règle écrite qu'aucun test ne fait échouer peut être inatteignable, ou fausse sans qu'aucun test ne le voie.

La suite lève 2 038 des 3 098 règles. Les 1 060 autres sont listées, par famille, dans [`testdata/rule-coverage.txt`](../testdata/rule-coverage.txt). Le test échoue sur une règle qu'aucun test ne lève et que la liste ne cite pas, comme sur une règle de la liste qu'un test lève : la liste ne peut que raccourcir. Les règles du schéma XSD, du conteneur PDF et de tous les jeux nationaux (BR-FR, BR-DE, Chorus Pro, Peppol, BR-NL, BR-IT) sont toutes levées. Les manques sont dans :

| Jeu de règles | Non levées | Familles |
|---|---|---|
| EN 16931 | 686 sur 1 566 | règles de syntaxe CII-SR (455) et CII-DT (95) du schematron CEN, UBL-CR (30), UBL-SR (19) et UBL-DT (2), catégories de TVA autres que S et E (55), décimales BR-DEC (16), listes de codes BR-CL (10), motif des remises et majorations de ligne (BR-42, BR-44, BR-CO-23, BR-CO-24) |
| Factur-X | 339 sur 415 | listes de codes du schematron Factur-X 1.09.2 (FX-SCH-A, 304), règles EXTENDED BR-FXEXT (25), IGIC et IPSI (10) |
| CIUS-PT | 34 sur 372 | formats DT-CIUS-PT (29), BR-AA, BR-CO-07, BR-CO-08, UBL-SR-38 |
| FatturaPA (signature) | 1 | IT-SIG-06, que seul le job d'interopérabilité des signatures lève |

```bash
eval "$(conformance/fetch.sh)"
ZERECHNUNG_RULE_TRACE=$PWD/rules.txt go test -tags ruletrace ./...
go test -run '^TestRuleCoverage$' . -rule-coverage rules.txt
```

## veraPDF sur les PDF convertis

Le job *veraPDF sur les PDF convertis* exécute [`verapdf.sh`](verapdf.sh), qui vérifie avec veraPDF 1.30.3 les fichiers qu'écrit `ConvertPDFA`. veraPDF tourne depuis l'image Docker du consortium veraPDF, figée par son empreinte (`verapdf/cli:v1.30.3@sha256:d74b7026…`) : le package reste en pur Go. Le script :

1. récupère le corpus veraPDF au commit de `fetch.sh` (`FX_VERAPDF`) ;
2. lance `TestConvertCorpus`, `TestConvertPDFA` et `TestEmbedConvertPDFA` avec `FX_PDFA_OUT` : les 998 fichiers PDF/A-2b et PDF/A-3b du corpus, valides et invalides, sont convertis (ou refusés), comme les PDF non conformes que génèrent les tests de [`convert_test.go`](../convert_test.go) (actions et JavaScript, LZW, états graphiques, annotations sans le drapeau Print, ressources héritées, fichiers incorporés incomplets, métadonnées refusées par PDF/A…) et la facture hybride de `testdata` ; les fichiers convertis sont écrits dans `conformance/.artifacts/pdfa/out` ;
3. exécute veraPDF sur ce dossier, qui détecte la conformité déclarée (PDF/A-3B) ;
4. relance les mêmes tests avec `FX_PDFA_REPORT` : chaque fichier converti doit passer veraPDF, sauf les échecs connus que liste `convertKnownFailures` dans [`convert_corpus_test.go`](../convert_corpus_test.go). Un échec non listé, un fichier listé qui passe ou n'est plus converti, un fichier absent du rapport font échouer le job.

### Résultats de la conversion

| Fichiers | Convertis | Refusés | Convertis qui passent veraPDF | Échecs connus |
|---|---|---|---|---|
| Corpus veraPDF, PDF/A-2b et PDF/A-3b (998) | 836 | 162 | 835 | 1 |
| PDF non conformes générés par les tests (10) | 10 | — | 10 | 0 |

Des 162 refus, un seul porte sur un fichier valide du corpus : un document signé, que la réécriture invaliderait (`convertRefusedValid`). Les autres sont des fichiers invalides que la conversion ne sait pas réparer sans moteur PDF : police non incorporée, glyphe absent du programme de police ou de largeur différente, glyphe `.notdef`, nom de glyphe hors de l'Adobe Glyph List, valeur de CID hors limites, données JPEG 2000 refusées par PDF/A, DeviceCMYK sans profil CMYK, annotation masquée ou sans apparence, opérateur inconnu, profil ICC de l'intention de sortie inutilisable, chiffrement…

Le seul échec connu porte sur ce que la conversion ne lit pas : un type de valeur d'un schéma d'extension XMP sans `pdfaType:field`.

## Parité avec KoSIT et Mustang

Le job *Parité avec KoSIT, Mustang, la NPa et eSPap* exécute les deux validateurs de référence et le package sur le même jeu de 282 factures :

- corpus ZUGFeRD : `XML-Rechnung/CII`, `XML-Rechnung/UBL`, `XML-Rechnung/FX`, `ZUGFeRDv2/correct` et `ZUGFeRDv2/fail` (factures invalides à dessein) ;
- suite de tests KoSIT : `src/test` (cas métier et cas techniques, toutes valides).

Sont laissés de côté ZUGFeRD 1.0 (Mustang lui applique un schematron dont les assertions n'ont pas d'identifiant : seules les erreurs de schéma seraient comparables), les factures Peppol de 25 et 61 Mo, fatturaPA et les dossiers non triés du corpus.

KoSIT reçoit les factures XML et ne vérifie que celles qu'un scénario de sa configuration reconnaît (XRechnung, et EN 16931 en CII et en UBL) ; Mustang reçoit toutes les factures, XML et PDF. Les deux s'exécutent chacun dans une seule JVM ; Mustang par [`parity/MustangBatch.java`](parity/MustangBatch.java), qui appelle `ZUGFeRDValidator` sans notices, comme l'option `--no-notices` de sa ligne de commande.

Pour chaque facture et chaque référence, l'outil compare les règles levées **en erreur** de part et d'autre, celles qui décident du verdict :

- les règles des schematrons officiels, sous leur identifiant : EN 16931 (BR, BR-CO, BR-CL, BR-DEC, règles par catégorie de TVA, CII-SR, CII-DT, UBL-CR, UBL-SR, UBL-DT), Factur-X (FX-SCH, BR-FXEXT), XRechnung (BR-DE, BR-DEX, BR-TMP) et Peppol (dont les règles que reprennent les schematrons XRechnung) ;
- les assertions du schematron Factur-X 1.09.2 d'EN 16931 qui vérifient une liste de codes (codedb), que Mustang applique à un XML EN 16931 seul, rapprochées de la règle BR-CL du CEN sur le même élément, que le package lui applique comme KoSIT, et celles sur BT-24, rapprochées de FX-PROFILE (table `equivalents` de [`parity/compare.go`](parity/compare.go)) ;
- les erreurs de schéma XSD, dont les messages diffèrent d'un validateur à l'autre, qui comptent pour une seule règle, `SCHEMA`.

Les niveaux sont ceux que donne chaque validateur ; pour KoSIT, ceux que fixe le scénario (`customLevel`, CII-SR-465 en erreur et BR-CL-21 en avertissement pour XRechnung par exemple), comme dans son évaluation du document. Une règle en erreur d'un côté et en avertissement de l'autre est un écart.

Restent hors de la comparaison : le conteneur PDF (Mustang vérifie PDF/A avec veraPDF, le package a ses propres contrôles PDF-xx), les contrôles propres à chacun (calculs de Mustang, FR-PROFILE du package) et les règles BR-FR, que Mustang 2.26.0 applique en version 1.3.0 aux factures entre deux parties françaises, quand le package applique la version 1.4.0.04 sur option.

Le job écrit le tableau de résultats dans le résumé du run et dépose les rapports bruts (KoSIT, Mustang, JSON) en artefact. Un écart absent de [`parity/known.txt`](parity/known.txt) le fait échouer.

### Résultats

| Référence | Factures comparées | Mêmes erreurs | Écarts justifiés |
|---|---|---|---|
| KoSIT validator 1.6.3, configuration XRechnung 3.0.2 du 2026-08-31 | 128 | 128 | 0 |
| Mustang 2.26.0 | 282 | 239 | 43 |

Avec KoSIT, toutes les factures comparées ont les mêmes erreurs.

Les écarts avec Mustang viennent de différences entre les références elles-mêmes : Mustang n'applique pas Peppol BIS 3.0 aux factures UBL Peppol du corpus, ni la configuration KoSIT aux factures XRechnung (niveaux des règles, et XSD CII D16B complet là où Mustang garde le XSD Factur-X EN 16931), et embarque le schematron XRechnung 2.4.0, quand KoSIT et le package appliquent la version 2.6.0. Enfin, le package range dans le profil XRECHNUNG les factures qui déclarent XRechnung 1.2 ou 2.1, et leur applique les règles de XRechnung 3.0.2, règles Peppol des schematrons XRechnung comprises, que Mustang n'y lève pas ; KoSIT refuse ces factures sans les évaluer.

Le package applique les schematrons Factur-X 1.09.2 comme Mustang à BASIC, à EXTENDED et à EN 16931 dans un PDF Factur-X : mêmes erreurs sur toutes ces factures du corpus, les assertions sur BT-24 étant rapprochées de FX-PROFILE. À un XML EN 16931 seul, il applique les règles CEN 1.3.17 comme KoSIT, là où Mustang garde le schematron Factur-X (voir [Factur-X ou CEN](../README.md#factur-x-ou-cen)) ; sur le corpus, la table `equivalents` rapproche les deux constats (FX-SCH-A-000571 et BR-CL-25 sur les deux exemples au schéma 9958), sans écart.

### Cas négatifs

Le tableau *Cas négatifs officiels* du rapport donne, pour chaque facture invalide à dessein du corpus, les erreurs levées par le package et par chaque référence. La suite de tests KoSIT ne contient que des factures valides.

Le test `TestOfficialNegativeCases` ([`negative_test.go`](../negative_test.go)), exécuté par le job *Tests sur les artefacts officiels*, fige ces verdicts : chacune des 24 factures de `ZUGFeRDv2/fail` et des 2 exemples `not_validating` doit lever les règles que lèvent les références (sous le même identifiant ou, pour le même défaut, sous celui du package). Le défaut PDF/A que Mustang relève avec veraPDF sur `ZUGFeRD_2_fully_compliant_complete.pdf`, un CIDSet de police incomplet, est relevé par PDF-72. Sept factures n'ont pas d'erreur, pour une raison donnée dans le test : les cinq exemples FNFE-MPE que le corpus a déplacés sans dire ce qui est faux, où Mustang ne trouve pas d'erreur non plus, un prix net incohérent qu'aucune règle EN 16931 ne contrôle, et un exemple où aucune référence ne trouve d'erreur.

### Variants des tests de mutation EN 16931 et XRechnung

Le corpus et la suite KoSIT contiennent surtout des factures valides : sur ces 128 factures, KoSIT ne lève que 17 de ses 1 936 assertions. Pour comparer aussi les chemins d'erreur, le job fait passer à KoSIT les documents des tests de mutation de trois paquets de règles, que leur test `TestVariants` écrit avec `-variants DIR` (paquet [`internal/variants`](../internal/variants)) :

```bash
go test ./internal/rules/de -run '^TestVariants$' -variants DIR
```

| Paquet | Variants | Documents |
|---|---|---|
| `internal/rules/de` | 293 | les cas de mutation des factures XRechnung CII et UBL du dépôt, factures de départ comprises ; ceux dont le BT-24 n'est retenu par aucun scénario de la configuration KoSIT (BR-DE-21, extension sur un avoir) sont laissés de côté |
| `internal/rules/en16931` | 55 | les cas des règles CEN en CII sans profil Factur-X, au BT-24 CEN ou XRechnung |
| `internal/rules/ubl` | 64 | les cas du contrôle des factures UBL, sauf ceux de l'extension G-rekening |

Chaque variant casse une ou plusieurs règles d'une facture valide (308 variants), ou vérifie qu'un cas limite n'en casse aucune (104) ; le fichier `expected.tsv` que le test écrit à côté donne les règles attendues. KoSIT seul les reçoit. L'outil compare les règles à tous les niveaux, erreurs, avertissements et informations, car beaucoup des règles cassées sont des avertissements. Il vérifie aussi que KoSIT lève chaque règle attendue : sinon l'écart `?RÈGLE` signale un variant qui ne teste plus sa règle chez la référence, et fait échouer le job comme les autres.

Deux comportements de KoSIT demandent un traitement à part :

- il n'exécute pas les schematrons sur un document que le XSD rejette : seules les règles qu'il lève, le schéma, sont alors comparées ;
- quand un schematron s'arrête sur une erreur dynamique XPath (plusieurs valeurs là où le test en attend une), son rapport XML montre une étape valide, sans message ; seul le tableau final de `kosit.log` indique `UNDEFINED`. L'outil lit ce tableau, et ne compare pour ces documents que les règles que KoSIT lève.

Le rapport donne enfin, pour chaque XSLT de la configuration KoSIT, la part des assertions levées au moins une fois, à n'importe quel niveau, sur le corpus ou sur les variants, par famille de règles, et la liste de celles qui ne le sont jamais.

### Résultats des variants

| Référence | Variants comparés | Mêmes règles | Écarts justifiés | Même verdict |
|---|---|---|---|---|
| KoSIT validator 1.6.3, configuration XRechnung 3.0.2 du 2026-08-31 | 412 | 398 | 14 | 410 |

| XSLT de la configuration KoSIT | Assertions | Levées sur le corpus et la suite KoSIT | Levées sur les variants | Levées au total |
|---|---|---|---|---|
| `EN16931-CII-validation.xsl` (CEN 1.3.16) | 806 | 10 | 66 | 70 (8,7 %) |
| `EN16931-UBL-validation.xsl` (CEN 1.3.16) | 979 | 4 | 70 | 70 (7,2 %) |
| `XRechnung-CII-validation.xsl` | 74 | 2 | 70 | 70 (94,6 %) |
| `XRechnung-UBL-validation.xsl` | 77 | 1 | 73 | 73 (94,8 %) |
| Total | 1 936 | 17 | 279 | 283 (14,6 %) |

Les variants portent les assertions XRechnung exercées de 3 à 143 sur 151, dont les 43 assertions Peppol (22 en CII, 21 en UBL), et celles du CEN de 14 à 140 sur 1 785. Restent jamais levées en CII BR-DE-21 (un BT-24 qu'aucun scénario ne retient n'est pas évalué), BR-DE-23-a, BR-DEX-05 et BR-TMP-CVD-01, en UBL BR-DE-21, BR-DE-1, BR-DE-2 et BR-TMP-6. Ces chiffres mesurent ce que la référence a confirmé, pas la couverture des tests du package : les familles CII-SR et UBL-CR (482 et 678 assertions) interdisent chacune un élément du schéma, et presque aucune n'est levée ; les règles BR, BR-CO et de catégorie de TVA restent en majorité à exercer par de nouveaux variants.

Les écarts justifiés de [`parity/known.txt`](parity/known.txt) viennent de deux causes :

- le BT-24 `urn:cen.eu:en16931:2017`, qui est aussi celui du profil Factur-X EN 16931 : le package valide ces factures contre le XSD de ce profil, plus strict que le XSD CII D16B complet de KoSIT (7 variants, qui lèvent par ailleurs de part et d'autre la règle qu'ils testent) ;
- la version CEN 1.3.16 de la configuration KoSIT, quand le package applique la 1.3.17 : règles IGIC renommées de BR-AF en BR-IP, CII-SR-476 comptée par document, contexte de CII-SR-036 corrigé, CII-DT-105 ajoutée, préfixe de BR-CO-09 entouré d'espaces (7 variants).

Deux variants n'ont pas le même verdict : un variant EN 16931 avec `ram:TestIndicator`, que le XSD Factur-X EN 16931 n'admet pas, et le variant de CII-SR-477, que KoSIT n'a pas pu évaluer.

Le package applique aux factures XRechnung les assertions Peppol que les schematrons XRechnung exécutent avant les leurs (`internal/rules/de`, voir sa documentation) : leurs contextes, leurs tests et leurs niveaux diffèrent par endroits de Peppol BIS 3.0 (élément vide de livraison en CII ou de référence de commande en UBL admis, R120 en avertissement, tolérance de 0,5 pour le forint). Leurs variants couvrent chaque assertion et ses cas limites (règle qui s'applique en premier dans un motif, tolérance, quantité de base du prix brut) ; KoSIT lève les mêmes règles sur chacun, sauf sur ceux que son XSD rejette (indicateur `False` ou vide), où seul le schéma est comparé.

Ces variants ont aussi révélé deux écarts, corrigés. En UBL, BR-CO-14 comparait la TVA de chaque `cac:TaxTotal` à la somme des ventilations de tous, alors que le schematron UBL la compare à celle de ses propres `cac:TaxSubtotal` (voir `taxTotal` dans [`business.go`](../internal/rules/ubl/business.go)). En UBL encore, les règles BR-S-01, BR-AE-01, BR-E-01, BR-G-01, BR-IC-01, BR-IG-01, BR-IP-01, BR-O-01 et BR-Z-01 suivaient les tests du schematron CII, alors que ceux du schematron UBL diffèrent. En UBL, une ventilation S exige une ligne, remise ou charge S, et inversement ; il en va de même pour L et M, sur les seules catégories de type TVA ; pour les autres catégories, dès que la catégorie apparaît, il faut exactement une ventilation (voir `categoryPresentUBL` dans [`vat.go`](../internal/rules/en16931/vat.go)).

## Parité NLCIUS avec les XSLT de la NPa

Le même job exécute les XSLT que l'autorité Peppol néerlandaise (NPa) compile de ses schematrons, `xsl/nlcius-cii-1.0.xsl` (NLCIUS-CII 1.0.3.13), `xsl/si-ubl-2.0.xsl` (SI-UBL 2.0.3.13) et `xsl/si-ubl-2.0-ext-gaccount.xsl` (extension G-rekening 1.0.2.13) du dépôt `FX_NL`, sur 290 fichiers :

- les 107 fichiers de test de SimplerInvoicing pour SI-UBL 2.0.3.2, NLCIUS-CII 1.0.3 et l'extension G-rekening 1.0, repris par phive-rules (`FX_NL_TESTS`) ;
- les 3 factures NLCIUS du dépôt (`internal/rules/nl/testdata` : CII, UBL et G-rekening) ;
- 180 variantes de ces factures : les documents des tests de mutation de `internal/rules/nl`, que `go test ./internal/rules/nl -run '^TestVariants$' -variants DIR` écrit, et qui enfreignent ou frôlent chacun une règle (condition sur le pays du fournisseur, valeur normalisée, code de type comparé comme un nombre, élément vide, montant illisible…).

Chaque fichier reçoit la XSLT qu'un validateur choisit : NLCIUS-CII pour une facture CII ; pour une facture UBL, celle de l'extension G-rekening quand le `cbc:CustomizationID` enfant de la racine la déclare, celle de SI-UBL 2.0 sinon. [`parity/NPaBatch.java`](parity/NPaBatch.java) les exécute avec Saxon-HE 12.10 dans une seule JVM, chaque XSLT compilée une fois, et écrit le rapport SVRL de chaque fichier. Le Saxon-HE 9.6 que fournit le dépôt de la NPa n'est pas utilisé : il s'arrête sur une erreur interne en comparant une valeur non typée à un nombre (`" 0389 " = 380`).

Pour chaque fichier, l'outil compare les règles levées **en erreur et en avertissement**, sous leur identifiant :

- côté NPa, chaque assertion échouée du rapport SVRL, au niveau de son `flag` (`fatal` : erreur, `warning` : avertissement) : celles des motifs NLCIUS (`nlcius`, `SI-UBL-VERSION`, `g-account-extension`) et les règles EN 16931 que chaque XSLT inclut (CEN 1.3.16), syntaxe CII et UBL comprise ;
- côté package, les constats des jeux `EN16931` et `Netherlands` de `validate.XML` avec `Rules: validate.Default | validate.Netherlands`, comme si le validateur avait été choisi ; ses constats de schéma XSD, de profil Factur-X et Peppol, que les XSLT n'ont pas, restent hors de la comparaison.

Une règle levée d'un seul côté, ou à des niveaux différents, est un écart. Quand la transformation s'arrête sur une erreur dynamique (une valeur qui ne se convertit pas en `xs:decimal`, plusieurs valeurs là où le test n'en attend qu'une), le rapport ne contient que l'erreur et sa ligne dans la XSLT : l'outil retrouve l'assertion dont le test l'a levée, la tient pour échouée, comme le package (voir la documentation de `internal/rules/nl`), et ne compare qu'elle.

L'outil liste enfin les assertions des motifs NLCIUS qu'aucun fichier ne fait échouer, chacune désignée par son identifiant et son message (deux assertions BR-NL-8 en UBL, trois au message de BR-NL-34 sous les identifiants de BR-NL-32) : chacune doit être justifiée dans `known.txt` par une ligne `npa syntaxe/motif ~RÈGLE raison`, sans quoi le job échoue.

### Résultats NLCIUS

| Référence | Fichiers comparés | Mêmes règles | Écarts justifiés |
|---|---|---|---|
| XSLT de la NPa (Saxon-HE 12.10) : NLCIUS-CII 1.0.3.13, SI-UBL 2.0.3.13, G-rekening 1.0.2.13 | 290 | 283 | 7 |

Sur chacun des 290 fichiers, le package et les XSLT lèvent les mêmes règles BR-NL, BR-GA, SI-V20-INV-R000, BII2-T10-R001 et de contrôle des éléments vides, au même niveau, et rendent le même verdict. Les 7 écarts justifiés portent sur des règles EN 16931 :

- UBL-SR-57 (une seule `cac:PartyLegalEntity` par partie), ajoutée par les artefacts CEN 1.3.17 que le package applique, absente de la version 1.3.16 des XSLT : 2 fichiers de test, qui lèvent par ailleurs de part et d'autre la règle qu'ils testent ;
- 5 arrêts de la XSLT sur une erreur de type XPath dans une règle CEN (BR-06, BR-07, BR-DEC-18, BR-CO-16, BR-61), sur un élément répété que le schéma n'admet qu'une fois : le package lit la première valeur et signale la répétition par sa règle de syntaxe (UBL-SR-09, UBL-SR-15, CII-SR-487) ou par le schéma XSD, et la règle NLCIUS du cas.

Les 5 autres arrêts sont rendus à l'identique : BR-NL-7 sur un code de type `38O`, BR-GA-3 sur un montant `3.025E2` ou répété, BR-CO-16 sur un montant dû `1e3`, en CII comme en UBL. Les 8 assertions NLCIUS qu'aucun fichier ne déclenche sont celles qu'écarte l'ordre des règles, que le package n'écrit pas : BR-NL-9 et BR-NL-31 en CII ; en UBL, BR-NL-32-2, BR-NL-32-3 et les trois assertions au message de BR-NL-34 ; et SI-INV-R000, qui vérifie que la racine est une `Invoice` ou une `CreditNote`, les seuls documents qu'un validateur donne à la XSLT SI-UBL 2.0.

## Parité EXTENDED-CTC-FR avec les XSLT de la FNFE-MPE

Le même job exécute les XSLT que la FNFE-MPE compile de ses schematrons EXTENDED-CTC-FR, `FNFE_RFE_INVOICE/CII/EXTENDED-CTC-FR/2xslt/EXTENDED-CTC-FR-CII.xslt` et `FNFE_RFE_INVOICE/UBL/EXTENDED-CTC-FR/2xslt/EXTENDED-CTC-FR-UBL.xslt` du paquet France_RFE 1.4.0.04 (`FX_FNFE`), sur 224 fichiers :

- les 10 exemples EXTENDED-CTC-FR du paquet (`Z.example/TEST/CII/EXTENDED-CTC-FR` et `Z.example/TEST/UBL/EXTENDED-CTC-FR`), qui déclarent le profil ;
- les 4 factures EXTENDED-CTC-FR du dépôt (`internal/rules/fr/testdata`, en CII et en UBL), parmi les factures françaises de ce répertoire, celles qui déclarent le profil ;
- 210 variantes : les documents des tests de mutation EXTENDED-CTC-FR de `internal/rules/en16931` (106, en CII) et de `internal/rules/ubl` (104, en UBL), que leur test `TestCTCFRVariants` écrit avec `-variants DIR` (paquet [`internal/variants`](../internal/variants)), et qui enfreignent ou frôlent chacun une règle du schematron (sous-lignes DETAIL, GROUP et INFORMATION, tolérances des totaux et des ventilations de TVA, listes de codes, règles de syntaxe…).

Chaque fichier reçoit la XSLT de sa syntaxe, que [`parity/NPaBatch.java`](parity/NPaBatch.java) exécute avec Saxon-HE 12.10, comme celles de la NPa. Pour chaque fichier, l'outil compare les règles levées **en erreur et en avertissement**, sous leur identifiant : côté FNFE-MPE, chaque assertion échouée du rapport SVRL, au niveau de son `flag` ; côté package, les constats du jeu `EN16931` de `validate.XML` avec les jeux par défaut, qui portent les règles EXTENDED-CTC-FR pour une facture qui déclare le profil ; ses constats de schéma XSD et des autres jeux, que les XSLT n'ont pas, restent hors de la comparaison. Un arrêt de la transformation sur une erreur dynamique est traité comme pour la NPa : seule l'assertion dont le test l'a levée est comparée. Comme pour les variants KoSIT, l'outil vérifie aussi que la XSLT lève chaque règle que le test de mutation d'une variante attend (`expected.tsv`) : sinon l'écart `?RÈGLE` fait échouer le job.

### Résultats EXTENDED-CTC-FR

| Référence | Fichiers comparés | Mêmes règles | Écarts justifiés |
|---|---|---|---|
| XSLT EXTENDED-CTC-FR de la FNFE-MPE, France_RFE 1.4.0.04 (Saxon-HE 12.10) | 224 | 223 | 1 |

Le seul écart justifié est un arrêt de la XSLT CII sur une erreur de type XPath dans le test de BR-FREXT-CO-10, sur une variante qui répète `ram:LineTotalAmount` : le package lit la première valeur et lève CII-SR-477, la règle que la variante teste.

## Parité CIUS-PT avec le schematron d'eSPap

Le validateur de l'administration publique portugaise applique aux factures CIUS-PT le schematron d'eSPap (`FX_PT_SCH`), que le package reproduit dans `internal/rules/pt` avec un interpréteur du sous-ensemble de XPath 2.0 qu'il emploie. Le même job compile ce schematron (`urn_feap.gov.pt_CIUS-PT_2.1.1.sch` et les motifs qu'il inclut) en XSLT avec SchXslt 1.10.1, dont Saxon exécute le pipeline `xslt/2.0/pipeline-for-svrl.xsl`, puis [`parity/CiusPtBatch.java`](parity/CiusPtBatch.java) exécute cette XSLT avec Saxon-HE 12.10, dans une seule JVM et sur un fil par processeur, sur 5 783 fichiers :

- les 15 exemples d'eSPap (`FX_PT`), lus dans l'archive : 13 factures, une note de débit et une note de crédit ;
- les 3 factures CIUS-PT du dépôt (`internal/rules/pt/testdata` : une facture, une note de crédit et une facture plus complète) ;
- les 5 765 variantes de ces factures que `go test ./internal/rules/pt -run '^TestVariants$' -variants DIR` écrit, avec leur liste (`manifest.tsv`, déposée en artefact : document de base, élément remplacé et remplacement) ; chacune supprime, répète, vide ou remplace un élément ou un attribut (nombres voisins, codes d'autres listes, notes structurées mal formées, éléments que la syntaxe UBL interdit).

Le lanceur ne garde du rapport SVRL que les assertions échouées et les rapports déclenchés, avec leur identifiant, leur `flag` et leur emplacement. Pour chaque fichier, l'outil compare les règles levées **en erreur et en avertissement**, chacune **sur l'élément où elle est levée** :

- côté Saxon, chaque assertion échouée au niveau de son `flag` (`fatal` : erreur, `warning` : avertissement), sur l'élément que donne son attribut `location` (`/Q{…}Invoice[1]/Q{…}InvoiceLine[2]/…`) ;
- côté package, les constats de `validate.XML` avec `Profile: invoice.ProfileCIUSPT` et `Rules: validate.EN16931 | validate.Portugal`, sur l'élément que donne leur `Path` : ceux du jeu EN 16931, que le schematron remplace pour une facture CIUS-PT, et ceux du jeu `Portugal`, sauf les contrôles propres au package (PT-NIF, PT-SYNTAX). Le profil est imposé : une variante peut retirer ou changer BT-24, et Saxon applique le schematron à tout fichier qu'il reçoit.

Les deux emplacements sont ramenés à la même forme, un pas par élément avec son nom local et son rang parmi ses frères de même nom (`/Invoice[1]/InvoiceLine[2]/ID[1]`). Une règle levée d'un seul côté, à un autre niveau ou sur un autre élément est un écart, noté `+RÈGLE@ÉLÉMENT` (package seul, ou niveau plus élevé) ou `-RÈGLE@ÉLÉMENT` dans le rapport et dans `known.txt`.

Quand Saxon s'arrête sur une erreur de type XPath dans le test d'une assertion (plusieurs valeurs là où le test n'en attend qu'une, `XPTY0004` ; une valeur qui ne se convertit pas en nombre, `FORG0001`), son rapport ne contient que l'erreur et la ligne de la XSLT où elle survient. L'outil retrouve l'assertion dont l'`xsl:if` occupe cette ligne ; le package tient cette assertion pour échouée (voir la documentation de `internal/rules/pt`), et seule elle est comparée, sans son élément, que Saxon ne donne pas. L'arrêt n'est pas un écart quand le package lève l'assertion ; il en est un quand le package ne la lève pas, ou quand la ligne ne désigne aucune assertion. Une erreur dans le contexte d'une règle n'arrête pas Saxon, qui considère que l'élément ne correspond pas à la règle (XSLT 3.0) : le package fait de même.

### Résultats CIUS-PT

| Fichiers | Comparés | Mêmes règles, aux mêmes éléments | Arrêts de Saxon, assertion tenue pour échouée par le package | Écarts |
|---|---|---|---|---|
| Exemples d'eSPap (`FX_PT`) | 15 | 15 (aucune règle levée) | 0 | 0 |
| Factures du dépôt (`internal/rules/pt/testdata`) | 3 | 3 (aucune règle levée) | 0 | 0 |
| Variantes de ces factures | 5 765 | 5 462 | 303 | 0 |

Des 5 462 variantes que Saxon mène à terme, 3 878 lèvent au moins une règle : 10 546 constats, 8 699 en erreur et 1 847 en avertissement, chacun sur le même élément de part et d'autre. Les 303 arrêts portent sur 31 assertions : 171 `FORG0001`, une valeur vide ou textuelle convertie en `xs:decimal` ou en `xs:double` (règles de calcul et de tolérance des totaux DT-CIUS-PT-157 à DT-CIUS-PT-175, BR-27, BR-28, BR-S-06, BR-E-05…), et 132 `XPTY0004`, un élément répété dont le test attend une seule valeur (BR-61, BR-AA-01 à BR-AA-04, BR-E-02 à BR-E-04, DT-CIUS-PT-157 à DT-CIUS-PT-175) ; le package lève chaque fois l'assertion. Le rapport du job les compte par erreur et par assertion. Pendant le développement, environ 18 000 autres variantes ont été comparées de la même façon, sans écart.

Les 1 077 cas de `internal/rules/pt/cases_gen_test.go`, chacun une variante et les règles que Saxon lève pour elle, figent en plus ce résultat dans `go test`, sans Java (1 105 des 1 154 identifiants du schematron) ; `go test ./internal/rules/pt -run '^TestUpdateCases$' -variants DIR` les régénère à partir des rapports SVRL des variantes. Les 49 autres identifiants n'ont pas de variante à une seule modification qui les lève : ils demandent deux modifications à la fois (BR-CO-03 : date et code de date d'exigibilité de la TVA ; BR-42, BR-44, BR-CO-23, BR-CO-24 : motif et code de motif d'une remise ou d'une charge de ligne), ou portent sur un élément qu'aucune des trois factures ne contient (devise de TVA BT-6 et notes structurées `#SOURCECURRENCYCODE#` et `#CALCULATIONRATE#`, répétitions que vérifient certaines règles de syntaxe UBL-SR). `cases_gen_test.go` les liste.

## Interopérabilité des signatures FatturaPA

Le job *Interopérabilité des signatures FatturaPA* exécute [`signature.sh`](signature.sh), qui croise les signatures du package `fatturapa` (`SignCAdES`, `SignXAdES`, `Unwrap`, `VerifySignature`) avec trois implémentations de référence, dans les deux sens. Aucun certificat réel n'intervient : `TestWriteInteropFiles`, avec `FX_SIG_OUT`, génère une PKI de test (racine, autorité intermédiaire, certificats de signature qualifiés RSA et ECDSA) valide à la date du jour.

| Outil | Vérifie les fichiers du package | Signe des fichiers que le package vérifie |
|---|---|---|
| OpenSSL 3.5 (runner Ubuntu 26.04) | `openssl cms -verify -cades`, qui exige l'attribut signing-certificate-v2 ; contenu identique à la facture signée | `openssl cms -sign` avec et sans `-cades`, RSA, RSA-PSS (sel maximal, en PEM), ECDSA en BER (`-stream`) |
| xmlsec1 1.3.9 (paquet Ubuntu) | `xmlsec1 --verify`, RSA, RSA-PSS et ECDSA (xmlsec1 1.2 ne connaît pas RSA-PSS : le script passe alors ce fichier) | `xmlsec1 --sign` : C14N exclusive, C14N 1.1 avec ECDSA, transformation XPath et C14N avec commentaires |
| EU DSS 6.5.1 (Maven Central) | `DssCheck.java verify` : format CAdES-BASELINE-B ou XAdES-BASELINE-B, structure XSD, condensats, valeur de signature, référence signée au certificat, chaîne jusqu'à la racine de test, blocs FC, ISC, VCI, CV et SAV de la validation ETSI EN 319 102-1 à PASSED | `DssCheck.java sign` : XAdES Baseline B enveloppée (transformation XPath Filter 2.0) et CAdES Baseline B enveloppante, RSA, RSA-PSS et ECDSA |

La conclusion de DSS reste INDETERMINATE, faute de données de révocation pour la PKI de test : seule la validation du certificat (bloc XCV) en dépend, et le script ne la contrôle pas.

`TestForeignSignatures`, avec `FX_SIG_FOREIGN`, vérifie enfin chacun des 13 fichiers signés par les outils : `Unwrap` doit rendre la facture (à l'octet près en CAdES, à l'identique une fois lue en XAdES, que les outils réécrivent) et `VerifySignature` ne lever que les règles attendues de l'outil (`foreignExpect` dans [`interop_test.go`](../fatturapa/interop_test.go)) : aucune pour DSS et `openssl cms -sign -cades`, IT-SIG-06 pour une signature CMS sans signing-certificate-v2 ou une signature XML sans propriétés XAdES, 00103 pour une signature sans heure de signature (riferimento temporale). Tous lèvent IT-SIG-05, faute de données de révocation.

## En local

```bash
eval "$(conformance/fetch.sh)"
go test ./...
```

```bash
conformance/parity.sh
```

```bash
conformance/signature.sh
```

`fetch.sh` demande git, curl et `pdfdetach` (poppler-utils) ; `parity.sh` demande en plus Java 17 ou suivant et unzip, et exécute KoSIT, Mustang, les XSLT de la NPa et de la FNFE-MPE et le schematron CIUS-PT ; `signature.sh` demande Go, OpenSSL 3, xmlsec1, Java 21 et Maven. Les artefacts vont dans `conformance/.artifacts`, ignoré par git.
