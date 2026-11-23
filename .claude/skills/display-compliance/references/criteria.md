# Critères de la matrice

Ce fichier fixe comment on passe des sources à la matrice, pour que deux calculs, à
une minute ou à des semaines d'écart, donnent le même résultat sur les mêmes sources.
Il ne note aucun état : tout ce que la matrice affiche vient des sources du jour. En
cas de doute, préférer le statut le plus prudent et écrire pourquoi dans la note.

## Les sources

`scripts/gather_state.py` les rassemble. Du plus fiable au moins fiable :

1. le code de `origin/main` : paquets publics, jeux de règles de `validate`, API
   `Sign*` et `Verify*`, utilisateurs de l'encodeur `internal/qr`, `doc.go` de chaque
   paquet (section *Limits*), à lire quand le rapport ne suffit pas ;
2. les PR fusionnées et les issues (`gh pr view`, `gh issue view`) ;
3. le wiki : `Normes-européennes-par-pays.md` (les pays, leurs normes et leurs dates ;
   sa section 11 place chaque format face aux jeux de règles), et au besoin
   `Ce-que-valide-ZeRechnung.md` et `Grille-de-la-norme.md` ;
4. le README (tableau « What's supported ») et `site/data/compliance.json`, souvent en
   retard d'une PR.

Le wiki dit quelles normes existent ; le code dit ce que ZeRechnung en fait. Quand
deux sources se contredisent sur ce que fait le package, le code l'emporte, et l'écart
est un point à signaler à l'utilisateur, avec la page à corriger.

## Les pays

Un pays par section « ### Nom (XX) » du wiki, dans l'ordre de la page, avec la région
que donne le rapport : `ue` pour les 27 (tableau « Union européenne » de la section 3),
`aele` pour l'EEE hors UE, la Suisse et le Royaume-Uni, `autre` pour le reste. `name`
est le nom français du wiki.

## Ce qui compte comme une norme

Une ligne de `norms` est un format ou une spécialisation qu'un émetteur doit
produire, ou qu'un destinataire doit savoir lire, pour facturer dans le pays :

- les syntaxes et profils en vigueur ou dont l'obligation est datée : Factur-X,
  ZUGFeRD, XRechnung, Peppol BIS, FatturaPA, Facturae, KSeF FA(3), ISDOC… ;
- les CIUS et extensions nationales ;
- les jeux de règles nationaux publiés séparément quand on peut les appliquer à part
  (règles BR-FR, BR-DE…) ;
- un pays sans format national qui admet EN 16931 en UBL ou en CII : une norme
  « EN 16931 » (ou « Peppol BIS 3.0 » quand c'est par Peppol).

Le libellé reprend le nom du wiki, version comprise (« Facturae 3.2.2 »,
« Peppol BIS 3.0 »). Une ancienne version reste tant que l'administration l'admet
encore ; restent dehors les versions qu'elle n'admet plus, même si le package les lit
encore, les formats d'échange privés sans base légale et les protocoles de transport.
Une norme annoncée mais pas encore en vigueur, ou dont la version finale n'est pas
publiée, porte `"future": true` ; le widget la dessine en pointillés. Elle compte dans
les totaux, comme le reste.

Les lignes du wiki qui ne sont ni un format ni une obligation autour du document (la
réforme elle-même, son calendrier, une plateforme abandonnée) ne deviennent pas des
lignes : elles servent aux notes et aux échéances.

Une fonctionnalité (`features`) est ce que la norme impose autour du document :

| `kind` | Quoi | Exemples |
|---|---|---|
| `pdf` | PDF hybride (XML embarqué dans un PDF/A) | Factur-X, ZUGFeRD |
| `sig` | Signature ou cachet électronique | PAdES, XAdES de Facturae, CAdES et XAdES de FatturaPA |
| `qr` | QR code imposé sur la facture | ATCUD, QR-facture suisse, codes KSeF |
| `net` | Plateforme, réseau ou mode d'échange | Chorus Pro, SDI, Peppol, FACe, KSeF |
| `rep` | Reporting fiscal autour de la facture | e-reporting, statuts du cycle de vie, SII, myDATA, SAF-T |

## Les statuts

ZeRechnung est un package Go pur, sans dépendance et sans appel réseau. Les statuts
mesurent ce que le package fait avec le document, hors ligne.

- `y` **couvert** : dans `origin/main`, avec une API publique ou un jeu de règles de
  `validate`, testé (tests unitaires, conformité, ou parité avec l'outil de référence
  quand il en existe un). Lire, écrire et valider pour un format ; produire et
  vérifier pour une signature, facultative ou non.
- `p` **partiel** : une partie seulement est faite, et la note dit laquelle manque.
  Cas typiques : la syntaxe est lue et les règles EN 16931 appliquées, mais pas les
  règles nationales de la CIUS ; le document est lu, écrit et validé contre son XSD,
  mais pas contre son schematron ; l'encodeur QR existe (`internal/qr`) mais aucune
  API publique ne produit le contenu national.
- `n` **à faire** : rien dans `origin/main`, ou seulement du travail en revue. Un
  élément `n` ou `p` doit être suivi par une issue listée dans `issues` du pays
  (`check` prévient sinon).
- `x` **hors périmètre** : transmission et plateformes (envoi à Chorus Pro, SDI, FACe,
  KSeF, Peppol, déclarations en temps réel aux administrations), parce qu'elles
  demandent du réseau, un compte ou un certificat d'accès ; et ce que produit
  l'intermédiaire (point d'accès, plateforme), pas l'émetteur. Une norme n'est jamais
  `x`.

Frontière entre `n` et `x` : un document que le package peut traiter hors ligne reste
dans le périmètre, même s'il sert à une plateforme. Les statuts du cycle de vie et les
rapports d'e-reporting d'une réforme sont des fichiers à lire, écrire et valider ; une
option de validation propre aux plateformes est une option du package : tous sont dans
le périmètre, alors que leur envoi est `x`. Avant de classer un reporting en `x`, se
demander s'il existe un fichier que le package pourrait produire.

Une PR en revue ne change pas la matrice : le statut reste `n` ou `p` jusqu'à sa
fusion dans `origin/main` (la note peut dire « en revue dans la PR #NN »).

## Les issues

`issues` d'un pays liste les issues ouvertes qui suivent un de ses manques, d'après
la section « Issues ouvertes » du rapport, rangées par le pays de leur titre. Une
issue fermée n'y figure jamais. Une issue générale (veille, points ouverts) ne va à
aucun pays. Un manque sans issue est un nouveau manque, à proposer à l'utilisateur.

## Notes

Une note est courte et dit ce qu'il faut savoir en survolant la pastille : ce qui
manque pour un `p`, l'API ou la PR pour un `y` (« PR #NN », d'après les PR fusionnées
du rapport ou `git log`), l'échéance pour un `n` (« B2B au 06/10/2027 »), l'issue qui
suit un manque (« (#NN) »). Les dates en JJ/MM/AAAA. Une information qu'on n'a pas pu
confirmer (contenu exact d'un QR code, forme d'un cachet) est écrite « à vérifier ».

## Lignes de code

Le widget mesure l'effort déjà fourni par pays, pas la couverture : un pays couvert à
100 % par Peppol pèse peu, un pays à format national pèse lourd. `count_loc.py` compte,
`data/loc_map.json` décrit la méthode de rattachement.

- **Ce qu'on compte** : les lignes non vides hors commentaires des fichiers `.go` hors
  tests de `origin/main`. Restent dehors les outils et ce qui ne sert pas les normes :
  `cmd`, `server`, `conformance`, `site`, les aides de test (dossiers `*test`,
  `internal/variants`, `internal/race`, `internal/initcost`). Le code généré
  (`*_gen.go`) est compté à part, pour la case qui le retire ; les générateurs
  (`gen/main.go`) sont écrits à la main et comptent comme tels.
- **Normes ou fonctionnalités** : un format et ses règles (lecture, écriture, schéma,
  jeu de règles) vont aux normes ; ce que la norme impose autour du document (PDF
  hybride, signature, QR code) va aux fonctionnalités. Un même paquet peut se
  partager : la signature d'un format national va aux fonctionnalités, le reste du
  paquet à ses normes. Les documents de reporting suivent la matrice, qui les range
  dans les fonctionnalités (`rep`) : leur paquet, leurs règles, leurs schémas et leurs
  fichiers de `validate` vont aux fonctionnalités de leur pays.
- **Un pays ou un groupe** : un code propre à un pays lui revient en entier, même si
  un voisin s'en sert. Un code commun à plusieurs pays forme un groupe (Factur-X /
  ZUGFeRD, Peppol, interpréteur schematron, PDF/A-3 hybride, PAdES, socle de
  signature, encodeur QR) : le widget le répartit à parts égales entre ses pays, ou le
  montre en tranche grise. Les pays d'un groupe ne s'écrivent jamais : ils se déduisent
  de la matrice (`norm` ou `feature` : une norme ou une fonctionnalité couverte ou
  partielle) ou du code (`imports` : les pays dont un fichier importe le paquet).
- **Le socle EN 16931** (CII, UBL, modèle `invoice`, règles et listes de codes CEN,
  schémas, `validate`, lecture XML) est la base commune européenne, marquée `base`
  dans `data/loc_map.json`. Il n'est pas réparti entre les pays : le widget le dessine
  en disque central, et chaque pays en dépasse d'une barre dont la surface est à
  l'échelle de celle du disque : un pays qui ajoute 20 % du socle a une barre de 20 %
  de la surface du disque (rayon extérieur R0·√(1 + 2πp/θ) pour une part d'angle θ).
  La barre empile les dossiers du pays, dans sa couleur, puis sa part des groupes
  partagés, en gris ; montrés à part, les groupes ont leur propre barre. Les pays
  autour du cercle sont ceux du socle (motif `norm` du groupe : une norme couverte ou
  partielle qui repose sur EN 16931). Les pays qui ajoutent au moins 1 % du socle ont
  une part trois fois plus large, pour que leurs barres restent lisibles ; à surface
  égale, une part étroite sort plus loin, et les graduations ne valent que pour les
  parts larges. Les composants du socle (`parts`) s'affichent au survol du cercle.
- **Un fichier qu'aucune règle ne rattache** arrête le comptage. Le rattacher, dans
  `<scratchpad>/loc_extra.json` passé par `--extra`, au pays dont il implémente la
  norme (un motif peut porter `{cc}`, le code du pays en minuscules), ou à un groupe
  s'il en sert plusieurs ; à défaut, au socle. Dans les dossiers qui mêlent socle et
  pays (racine, `cii`, `ubl`, `invoice`, `validate`, `internal/rules`,
  `internal/codes`, `internal/schema`), le socle et le PDF hybride nomment leurs
  fichiers un à un, sans joker : c'est ce qui fait arrêter le comptage sur un nouveau
  fichier national au lieu de le ranger dans le socle. Un code national à côté d'un
  encodeur commun va au pays, l'encodeur restant au groupe.
