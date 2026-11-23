# Critères de la matrice

Ce fichier fixe comment on décide d'un statut, pour que deux mises à jour faites à
des semaines d'écart donnent le même résultat. En cas de doute, préférer le statut
le plus prudent et écrire pourquoi dans la note.

## Ce qui compte comme une norme

Une ligne de `norms` est un format ou une spécialisation qu'un émetteur doit
produire, ou qu'un destinataire doit savoir lire, pour facturer dans le pays :

- les syntaxes et profils en vigueur ou dont l'obligation est datée : Factur-X,
  ZUGFeRD, XRechnung, Peppol BIS, FatturaPA, Facturae, KSeF FA(3), ISDOC… ;
- les CIUS et extensions nationales (CIUS-PT, CIUS-RO, EXTENDED-CTC-FR, BT-ES…) ;
- les jeux de règles nationaux publiés séparément quand on peut les appliquer à part
  (règles BR-FR, BR-DE…).

Une ancienne version reste tant que l'administration l'admet encore (ZUGFeRD 2.0.1
à 2.5 en Allemagne). Restent dehors : les versions qu'elle n'admet plus (ZUGFeRD 1.0,
que le package lit pourtant encore, XRechnung 2.x), les formats d'échange privés sans
base légale, les protocoles de transport. Une norme annoncée mais pas encore en vigueur porte `"future": true` ; le
widget la dessine en pointillés. Elle compte dans les totaux, comme le reste.

Une fonctionnalité (`features`) est ce que la norme impose autour du document :

| `kind` | Quoi | Exemples |
|---|---|---|
| `pdf` | PDF hybride (XML embarqué dans un PDF/A) | Factur-X, ZUGFeRD |
| `sig` | Signature ou cachet électronique | PAdES, XAdES de Facturae, CAdES et XAdES de FatturaPA |
| `qr` | QR code imposé sur la facture | ATCUD (PT), QR-facture (CH), KSeF |
| `net` | Plateforme, réseau ou mode d'échange | Chorus Pro, SDI, Peppol, FACe, KSeF |
| `rep` | Reporting fiscal autour de la facture | e-reporting FR, SII, myDATA, SAF-T, statuts CDAR |

Régions : `ue` pour les 27, `aele` pour l'EEE hors UE, la Suisse et le Royaume-Uni,
`autre` pour le reste (Balkans, Turquie, Ukraine, Moldavie…).

## Les statuts

ZeRechnung est un package Go pur, sans dépendance et sans appel réseau. Les statuts
mesurent ce que le package fait avec le document, hors ligne.

- `y` **couvert** : dans `origin/main`, avec une API publique ou un jeu de règles de
  `validate`, testé (tests unitaires, conformité, ou parité avec l'outil de référence
  quand il en existe un). Lire, écrire et valider pour un format ; produire et
  vérifier pour une signature.
- `p` **partiel** : une partie seulement est faite, et la note dit laquelle manque.
  Cas typiques : la syntaxe est lue et les règles EN 16931 appliquées, mais pas les
  règles nationales de la CIUS ; l'encodeur QR existe (`internal/qr`) mais aucune API
  publique ne produit le contenu national.
- `n` **à faire** : rien dans `origin/main`, ou seulement du travail en revue. Un
  élément `n` ou `p` doit être suivi par une issue listée dans `issues` du pays
  (`check` prévient sinon).
- `x` **hors périmètre** : transmission et plateformes (envoi à Chorus Pro, SDI, FACe,
  KSeF, Peppol, déclarations en temps réel aux administrations), parce qu'elles
  demandent du réseau, un compte ou un certificat d'accès. Une norme n'est jamais `x`.

Frontière entre `n` et `x` : un document que le package peut traiter hors ligne reste
dans le périmètre, même s'il sert à une plateforme. Les statuts de cycle de vie CDAR
et le flux d'e-reporting français sont des fichiers à lire, écrire et valider : ils
sont `n` avec leurs issues (#109, #110), alors que leur envoi aux plateformes agréées
est `x`. Avant de classer un reporting en `x`, se demander s'il existe un fichier que
le package pourrait produire.

## Ce qui fait changer un statut

- **Une PR fusionnée** qui apporte le format, le jeu de règles ou l'API : passer à `y`
  (ou `p` si la PR laisse une partie de côté, à écrire dans la note), citer
  « PR #NN » dans la note.
- **Une issue liée fermée** : retrouver la PR qui l'a fermée et vérifier qu'elle est
  dans `origin/main`. Si oui, passer l'élément à `y` et retirer l'issue de la liste du
  pays. Une issue fermée sans PR (abandon, doublon) ne change pas le statut : chercher
  ou proposer une autre issue.
- **Une PR en revue** ne change pas la matrice de `main` : le statut reste `n` ou `p`
  jusqu'à la fusion (la note peut dire « en revue dans la PR #NN »). La PR peut porter
  la mise à jour de sa propre ligne de `data/matrix.json`, qui prend effet à la fusion.
- **Une nouvelle norme nationale** (loi, décret, calendrier publié) : l'ajouter en `n`,
  avec `future` si elle n'est pas en vigueur, et proposer une issue.
- **Une norme abandonnée ou remplacée** : la retirer, en le disant dans la réponse.

## Où regarder

Dans l'ordre, du plus fiable au moins fiable :

1. le code de `origin/main` : paquets publics, `validate/validate.go` (jeux de
   règles), API `Sign*` et `Verify*`, `doc.go` de chaque paquet (section *Limits*) ;
2. les PR fusionnées et les issues (`gh pr view`, `gh issue view`) ;
3. le wiki : `Normes-européennes-par-pays.md` (la matrice rédigée, section 11 pour
   les formats face aux jeux de règles), `Ce-que-valide-ZeRechnung.md`,
   `Grille-de-la-norme.md` ;
4. le README (tableau « What's supported ») et `site/data/compliance.json`, souvent
   en retard d'une PR.

Quand deux sources se contredisent, le code l'emporte ; l'écart est un point à
signaler à l'utilisateur, avec la page à corriger.

## Notes

Une note est courte et dit ce qu'il faut savoir en survolant la pastille : ce qui
manque pour un `p`, l'API ou la PR pour un `y`, l'échéance pour un `n` (« B2B au
06/10/2027 »). Les dates en JJ/MM/AAAA. Une information qu'on n'a pas pu confirmer
(contenu exact d'un QR code, forme d'un cachet) est écrite « à vérifier ».

## Décisions passées

- **EXTENDED-CTC-FR (France)** : `p` tant que seules les règles EN 16931 étaient
  appliquées, `y` après la PR #116 (schématron FNFE en CII et UBL, parité Saxon sur
  223 fichiers sur 224, un écart justifié).
- **PAdES (France, Allemagne et tout PDF hybride)** : `zerechnung.Sign`, PAdES B-B,
  dans le cœur du package depuis la PR #122 ; noté `y` là où la signature est
  facultative.
- **XAdES-EPES de Facturae (Espagne)** : `n` avec l'issue #121, puis `y` après la
  PR #123 (fusion 9b9f489) ; #121 fermée et retirée de la liste.
- **CDAR et e-reporting (France)** : passés de `x` à `n` quand les issues #109 et
  #110 ont posé qu'il s'agit de documents à traiter hors ligne.
- **Mode plateforme agréée (France)** : `n` avec #111, c'est une option de `validate`,
  pas une transmission.
- **ATCUD + QR code (Portugal)**, **Swiss QR Code (Suisse)** : `p`, l'encodeur
  `internal/qr` existe mais aucune API publique ne produit le contenu.
- **CIUS-RO (Roumanie)** : `p` avec l'issue #101, puis `y` après la PR qui applique
  les règles du schematron 1.0.9 de l'ANAF (jeu `validate.Romania`) ; #101 fermée et
  retirée de la liste.
- **Peppol BIS SK (Slovaquie)** : `p` avec l'issue #102, puis `y` après la PR qui
  applique les exigences de la transposition 1.13 de la Finančná správa au-delà de
  Peppol BIS (jeu `validate.Slovakia`, SK-01 à SK-15) ; #102 fermée et retirée de la
  liste. Le TDD SK reste `x` : les points d'accès le produisent.
- **CIUS-HR + extension (Croatie)** : `p` avec l'issue #103, puis `y` après la PR qui
  applique les règles du schematron de la Porezna uprava (jeu `validate.Croatia`) et lit
  et écrit l'extension en UBL ; #103 fermée et retirée de la liste.
