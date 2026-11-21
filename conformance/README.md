# Conformité

Ce dossier mesure l'exactitude du package sur les artefacts officiels, à chaque push et à chaque pull request, dans le workflow [Conformité](../.github/workflows/conformance.yml). Rien ici ne fait partie du package `zerechnung`, qui reste en pur Go : Java ne sert qu'à exécuter les validateurs de référence dans la CI.

| Fichier | Rôle |
|---|---|
| [`fetch.sh`](fetch.sh) | Récupère les artefacts officiels à des versions figées et les vérifie, puis affiche les variables `FX_*` qui activent les tests optionnels |
| [`fetch-nl.sh`](fetch-nl.sh) | Récupère les seuls artefacts des règles néerlandaises (`FX_NL`, `FX_NL_TESTS`) ; `fetch.sh` l'appelle, et le job *Tests NLCIUS sur les artefacts officiels* du workflow [CI](../.github/workflows/ci.yml) l'exécute seul, en quelques secondes |
| [`verapdf.sh`](verapdf.sh) | Convertit en PDF/A-3 les fichiers PDF/A-2b et PDF/A-3b du corpus veraPDF et les PDF non conformes des tests, puis vérifie les fichiers convertis avec veraPDF |
| [`parity.sh`](parity.sh) | Exécute KoSIT et Mustang sur les factures du corpus et de la suite KoSIT, et les XSLT de la NPa sur les factures NLCIUS, puis compare leurs règles à celles du package |
| [`parity/`](parity) | Outil Go de comparaison (module séparé) et lanceurs Java de Mustang et des XSLT de la NPa |
| [`parity/known.txt`](parity/known.txt) | Écarts connus et justifiés, et assertions NLCIUS qu'aucune facture ne déclenche ; tout autre écart fait échouer le job |

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

FeRD et FNFE-MPE ne distribuent leurs paquets qu'au travers d'un formulaire : les 20 XSD et les 6 fichiers de schematron viennent de deux copies publiques de ces paquets sous Apache-2.0 ([LandrixSoftware/validator-configuration-zugferd](https://github.com/LandrixSoftware/validator-configuration-zugferd), [backoffice-plus/e-invoice-validator](https://github.com/backoffice-plus/e-invoice-validator)), à des commits figés. Chaque XSD est vérifié par le SHA-256 du fichier officiel, celui-là même dont `internal/schema/tables_gen.go` a été généré. Les schematrons et leurs listes de codes sont ceux que Mustang 2.26.0 embarque (`schematron/ZF_250`) et que contient le paquet officiel, aux fins de ligne près (CRLF dans le paquet et dans Mustang, LF dans la copie git) : leur SHA-256, sur le texte en LF, est vérifié par `fetch.sh` et par le générateur `internal/rules/facturx/gen`, dont `codedb_gen.go` est la sortie.

Les validateurs de référence, téléchargés par `parity.sh` depuis leurs publications GitHub, et Saxon depuis Maven Central, sont vérifiés de la même façon ; KoSIT s'exécute avec la configuration XRechnung 3.0.2 du 2026-08-31 récupérée par `fetch.sh`, Saxon avec les XSLT du dépôt de la NPa (`FX_NL`) :

| Validateur | Fichier | SHA-256 |
|---|---|---|
| KoSIT validator 1.6.3 | `validator-1.6.3-standalone.jar` | `799e64be…` |
| Mustang 2.26.0 | `Mustang-CLI-2.26.0.jar` | `42d7868c…` |
| Saxon-HE 12.10 | `Saxon-HE-12.10.jar` | `b571af28…` |
| XML Resolver 5.3.3, que Saxon 12 demande | `xmlresolver-5.3.3.jar`, `xmlresolver-5.3.3-data.jar` | `1fe4d5b9…`, `b0c487ad…` |

## Tests sur les artefacts

Le job *Tests sur les artefacts officiels* lance `go test ./...` avec toutes les variables `FX_*` définies (voir la section Tests du [README](../README.md#tests)) et échoue si un test est ignoré : chaque test optionnel tourne donc à chaque pull request.

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

Le job *Parité avec KoSIT, Mustang et la NPa* exécute les deux validateurs de référence et le package sur le même jeu de 282 factures :

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
| Mustang 2.26.0 | 282 | 241 | 41 |

Avec KoSIT, toutes les factures comparées ont les mêmes erreurs.

Les écarts avec Mustang viennent de différences entre les références elles-mêmes : Mustang n'applique pas Peppol BIS 3.0 aux factures UBL Peppol du corpus, ni la configuration KoSIT aux factures XRechnung (niveaux des règles, et XSD CII D16B complet là où Mustang garde le XSD Factur-X EN 16931), et embarque le schematron XRechnung 2.4.0, quand KoSIT et le package appliquent la version 2.6.0.

Le package applique les schematrons Factur-X 1.09.2 comme Mustang à BASIC, à EXTENDED et à EN 16931 dans un PDF Factur-X : mêmes erreurs sur toutes ces factures du corpus, les assertions sur BT-24 étant rapprochées de FX-PROFILE. À un XML EN 16931 seul, il applique les règles CEN 1.3.17 comme KoSIT, là où Mustang garde le schematron Factur-X (voir [Factur-X ou CEN](../README.md#factur-x-ou-cen)) ; sur le corpus, la table `equivalents` rapproche les deux constats (FX-SCH-A-000571 et BR-CL-25 sur les deux exemples au schéma 9958), sans écart.

### Cas négatifs

Le tableau *Cas négatifs officiels* du rapport donne, pour chaque facture invalide à dessein du corpus, les erreurs levées par le package et par chaque référence. La suite de tests KoSIT ne contient que des factures valides.

Le test `TestOfficialNegativeCases` ([`negative_test.go`](../negative_test.go)), exécuté par le job *Tests sur les artefacts officiels*, fige ces verdicts : chacune des 24 factures de `ZUGFeRDv2/fail` et des 2 exemples `not_validating` doit lever les règles que lèvent les références (sous le même identifiant ou, pour le même défaut, sous celui du package). Le défaut PDF/A que Mustang relève avec veraPDF sur `ZUGFeRD_2_fully_compliant_complete.pdf`, un CIDSet de police incomplet, est relevé par PDF-72. Sept factures n'ont pas d'erreur, pour une raison donnée dans le test : les cinq exemples FNFE-MPE que le corpus a déplacés sans dire ce qui est faux, où Mustang ne trouve pas d'erreur non plus, un prix net incohérent qu'aucune règle EN 16931 ne contrôle, et un exemple où aucune référence ne trouve d'erreur.

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

## En local

```bash
eval "$(conformance/fetch.sh)"
go test ./...
```

```bash
conformance/parity.sh
```

`fetch.sh` demande git, curl et `pdfdetach` (poppler-utils) ; `parity.sh` demande en plus Java 17 ou suivant et unzip, et exécute KoSIT, Mustang et les XSLT de la NPa. Les artefacts vont dans `conformance/.artifacts`, ignoré par git.
