<!-- Généré par bench/polyglot/report à partir de 2026-11-22 -->

#### Exactitude sur le corpus

Référence : les schematrons officiels (CEN EN 16931 et KoSIT) signalent 6 des 26 XML « valides » du corpus (BR-CL-25 deux fois, CII-SR-465 quatre fois).

| Librairie | Langage | XML valides signalés | XML lus | PDF extraits |
|---|---|---|---|---|
| **ZeRechnung (ce dépôt)** | Go | 6/26 | 26/26 | 104/104 |
| @attestwire/en16931 | TypeScript | 2/26 | 26/26 | 104/104 |
| @conformo/toolkit | TypeScript | 6/26 | 26/26 | 104/104 |
| @ekwo-ai/factur-x | TypeScript | 3/26 | 26/26 | 104/104 |
| @fin.cx/einvoice | TypeScript | 0/26 | 26/26 | 104/104 |
| @kontor-mcp/core | TypeScript | 2/26 | 26/26 | 104/104 |
| @lucaapp/zugferd-ts | TypeScript | 8/26 | 19/26 | 104/104 |
| @stackforge-eu/factur-x | TypeScript | 0/26 | — | 104/104 |
| @stafyniaksacha/facturx | TypeScript | 12/26 | 26/26 | 100/104 |
| @sweberdev/summand | TypeScript | 6/26 | — | 104/104 |
| en16931 | TypeScript | 6/26 | — | — |
| factur-x-kit | TypeScript | 18/26 | 13/26 | — |
| drafthorse | Python | 0/26 | 26/26 | — |
| factur-x | Python | 0/26 | 26/26 | 104/104 |
| getafix | Python | 6/26 | 20/26 | 104/104 |
| mcp-einvoicing-de | Python | 2/26 | 20/26 | 104/104 |
| pycheval | Python | — | 7/26 | 65/104 |
| saxonche-schematron | Python | 6/26 | — | — |
| zugferd-pdf | Python | — | — | 104/104 |
| core-invoice | Rust | 7/26 | 23/26 | — |
| einvoice | Rust | — | 26/26 | — |
| einvoice-validator | Rust | 2/26 | — | 104/104 |
| en16931 | Rust | 2/26 | 26/26 | 104/104 |
| erechnung-validator | Rust | 6/26 | — | — |
| factur-x | Rust | 17/26 | 9/26 | — |
| lopdf | Rust | — | — | 104/104 |
| pdf-invoice | Rust | 20/26 | 26/26 | 101/104 |
| pdfa | Rust | — | — | 32/104 |
| zugferd-rs | Rust | 7/26 | 19/26 | 104/104 |

#### Extraction

| Librairie | Langage | EN 16931, 149 Ko | EN 16931, 1,1 Mo | MINIMUM FNFE, 89 Ko | EXTENDED ZF 2.0, 157 Ko | Premier appel |
|---|---|---|---|---|---|---|
| **ZeRechnung (ce dépôt)** | Go | **31,5 µs** | **32,9 µs** | 11,1 µs | **36,0 µs** | — |
| einvoice-validator | Rust | 32,8 µs (×1,0) | 42,3 µs (×1,3) | 23,4 µs (×2,1) | 37,7 µs (×1,0) | 1,42 ms |
| lopdf | Rust | 74,0 µs (×2,3) | 97,3 µs (×3,0) | 44,0 µs (×4,0) | 66,5 µs (×1,8) | 734 µs |
| pdfa | Rust | 78,6 µs (×2,5) | échec | **504 ns** (×0,0) | échec | 137 µs |
| mcp-einvoicing-de | Python | 79,5 µs (×2,5) | 115 µs (×3,5) | 52,0 µs (×4,7) | 229 µs (×6,4) | 432 µs |
| zugferd-pdf | Python | 79,8 µs (×2,5) | 116 µs (×3,5) | 52,3 µs (×4,7) | 92,9 µs (×2,6) | 1,95 ms |
| pdf-invoice | Rust | 80,1 µs (×2,5) | 104 µs (×3,2) | 52,8 µs (×4,8) | 75,1 µs (×2,1) | 227 µs |
| zugferd-rs | Rust | 91,9 µs (×2,9) | 116 µs (×3,5) | 54,9 µs (×4,9) | 85,5 µs (×2,4) | 1,11 ms |
| @attestwire/en16931 | TypeScript | 180 µs (×5,7) | 204 µs (×6,2) | 109 µs (×9,8) | 207 µs (×5,8) | 4,33 ms |
| pycheval | Python | 200 µs (×6,3) | échec | 180 µs (×16) | échec | 998 µs |
| getafix | Python | 227 µs (×7,2) | 373 µs (×11) | 210 µs (×19) | 205 µs (×5,7) | 1,27 ms |
| factur-x | Python | 251 µs (×8,0) | 386 µs (×12) | 201 µs (×18) | 271 µs (×7,5) | 2,26 ms |
| @lucaapp/zugferd-ts | TypeScript | 258 µs (×8,2) | 297 µs (×9,0) | 123 µs (×11) | 231 µs (×6,4) | 4,48 ms |
| @ekwo-ai/factur-x | TypeScript | 273 µs (×8,7) | 321 µs (×9,8) | 140 µs (×13) | 247 µs (×6,8) | 4,90 ms |
| @conformo/toolkit | TypeScript | 279 µs (×8,8) | 10,47 ms (×318) | 136 µs (×12) | 243 µs (×6,8) | 4,80 ms |
| @stackforge-eu/factur-x | TypeScript | 283 µs (×9,0) | 330 µs (×10) | 142 µs (×13) | 253 µs (×7,0) | 4,69 ms |
| @stafyniaksacha/facturx | TypeScript | 381 µs (×12) | 3,15 ms (×96) | 175 µs (×16) | 446 µs (×12) | 7,41 ms |
| en16931 | Rust | 393 µs (×12) | 705 µs (×21) | 251 µs (×23) | 284 µs (×7,9) | 1,75 ms |
| @fin.cx/einvoice | TypeScript | 421 µs (×13) | 506 µs (×15) | 144 µs (×13) | 444 µs (×12) | 45,17 ms |
| @kontor-mcp/core | TypeScript | 708 µs (×22) | 1,49 ms (×45) | 811 µs (×73) | 764 µs (×21) | 9,26 ms |
| @sweberdev/summand | TypeScript | 941 µs (×30) | 20,68 ms (×629) | 486 µs (×44) | 1,10 ms (×31) | 3,08 ms |

En gras : le plus rapide de la colonne. (×n) : temps rapporté à celui du package Go de ce dépôt. Tri sur la colonne « EN 16931, 149 Ko » ; « Premier appel » : durée du premier appel dans le processus, même colonne (non mesurée pour Go).

#### Lecture XML → modèle

| Librairie | Langage | 13 Ko | 19 Ko | 1,3 Mo | Premier appel |
|---|---|---|---|---|---|
| **ZeRechnung (ce dépôt)** | Go | **18,1 µs** | **31,8 µs** | 1,17 ms | — |
| zugferd-rs | Rust | 24,5 µs (×1,4) | 37,7 µs (×1,2) | échec | 1,71 ms |
| pdf-invoice | Rust | 25,6 µs (×1,4) | 39,0 µs (×1,2) | 1,72 ms (×1,5) | 90,2 µs |
| core-invoice | Rust | 45,9 µs (×2,5) | 68,5 µs (×2,2) | 3,67 ms (×3,1) | 325 µs |
| @ekwo-ai/factur-x | TypeScript | 70,8 µs (×3,9) | 115 µs (×3,6) | 26,78 ms (×23) | 1,80 ms |
| en16931 | Rust | 75,2 µs (×4,2) | 130 µs (×4,1) | 7,19 ms (×6,1) | 2,25 ms |
| @conformo/toolkit | TypeScript | 92,8 µs (×5,1) | 160 µs (×5,0) | 1,71 ms (×1,5) | 1,59 ms |
| @attestwire/en16931 | TypeScript | 136 µs (×7,5) | 234 µs (×7,4) | 3,24 ms (×2,8) | 2,26 ms |
| @lucaapp/zugferd-ts | TypeScript | 160 µs (×8,9) | 285 µs (×9,0) | échec | 2,46 ms |
| pycheval | Python | 224 µs (×12) | échec | échec | 900 µs |
| factur-x-kit | TypeScript | 273 µs (×15) | échec | échec | 7,53 ms |
| mcp-einvoicing-de | Python | 332 µs (×18) | 477 µs (×15) | **704 µs** (×0,6) | 1,47 ms |
| einvoice | Rust | 341 µs (×19) | 567 µs (×18) | 12,40 ms (×11) | 1,83 ms |
| drafthorse | Python | 601 µs (×33) | 996 µs (×31) | 1,09 ms (×0,9) | 3,40 ms |
| @stafyniaksacha/facturx | TypeScript | 945 µs (×52) | 1,68 ms (×53) | 5,61 ms (×4,8) | 8,42 ms |
| @kontor-mcp/core | TypeScript | 1,15 ms (×64) | 1,78 ms (×56) | 4,59 ms (×3,9) | 13,18 ms |
| getafix | Python | 1,21 ms (×67) | 2,10 ms (×66) | échec | 3,57 ms |
| factur-x | Python | 1,51 ms (×83) | 2,08 ms (×66) | 12,32 ms (×11) | 5,38 ms |
| @fin.cx/einvoice | TypeScript | 2,16 ms (×119) | 3,85 ms (×121) | 7,30 ms (×6,2) | 12,74 ms |
| factur-x | Rust | échec | échec | 1,67 ms (×1,4) | — |

En gras : le plus rapide de la colonne. (×n) : temps rapporté à celui du package Go de ce dépôt. Tri sur la colonne « 13 Ko » ; « Premier appel » : durée du premier appel dans le processus, même colonne (non mesurée pour Go).

#### Lecture et validation

| Librairie | Langage | Valides signalés /26 | 13 Ko | 19 Ko | 1,3 Mo | Premier appel |
|---|---|---|---|---|---|---|
| **ZeRechnung (ce dépôt)** | Go | 6/26 | **24,8 µs** | 45,4 µs | **794 µs** | — |
| pdf-invoice | Rust | 20/26 | 26,3 µs (×1,1) | **40,1 µs** (×0,9) | 1,71 ms (×2,2) | 38,2 µs |
| @ekwo-ai/factur-x | TypeScript | 3/26 | 71,1 µs (×2,9) | 115 µs (×2,5) | 27,40 ms (×35) | 1,75 ms |
| core-invoice | Rust | 7/26 | 77,2 µs (×3,1) | 121 µs (×2,7) | 5,28 ms (×6,7) | 1,17 ms |
| en16931 | Rust | 2/26 | 77,6 µs (×3,1) | 131 µs (×2,9) | 6,87 ms (×8,6) | 1,60 ms |
| @attestwire/en16931 | TypeScript | 2/26 | 202 µs (×8,1) | 331 µs (×7,3) | 6,93 ms (×8,7) | 7,98 ms |
| factur-x-kit | TypeScript | 18/26 | 294 µs (×12) | échec | échec | 10,56 ms |
| einvoice-validator | Rust | 2/26 | 483 µs (×19) | 781 µs (×17) | 3,94 ms (×5,0) | 22,63 ms |
| drafthorse | Python | 0/26 | 507 µs (×20) | 542 µs (×12) | 8,21 ms (×10) | 2,45 ms |
| factur-x | Python | 0/26 | 518 µs (×21) | 552 µs (×12) | 5,41 ms (×6,8) | 791 µs |
| @stackforge-eu/factur-x | TypeScript | 0/26 | 831 µs (×33) | 883 µs (×19) | 10,33 ms (×13) | 30,32 ms |
| @sweberdev/summand | TypeScript | 6/26 | 854 µs (×34) | 1,40 ms (×31) | 7,68 ms (×9,7) | 17,64 ms |
| saxonche-schematron | Python | 6/26 | 900 µs (×36) | 1,47 ms (×32) | 12,29 ms (×15) | 136,64 ms |
| getafix | Python | 6/26 | 1,60 ms (×64) | 2,62 ms (×58) | échec | 2,97 ms |
| zugferd-rs | Rust | 7/26 | 2,00 ms (×81) | 2,78 ms (×61) | 5,22 ms (×6,6) | 5,66 ms |
| @fin.cx/einvoice | TypeScript | 0/26 | 3,15 ms (×127) | 5,37 ms (×118) | 10,76 ms (×14) | 17,52 ms |
| en16931 | TypeScript | 6/26 | 14,81 ms (×596) | 26,27 ms (×578) | 41,09 ms (×52) | 164,49 ms |
| erechnung-validator | Rust | 6/26 | 31,86 ms (×1 282) | 55,39 ms (×1 219) | 82,79 ms (×104) | 153,04 ms |
| @kontor-mcp/core | TypeScript | 2/26 | 47,59 ms (×1 915) | 53,48 ms (×1 177) | 74,76 ms (×94) | 146,69 ms |
| @stafyniaksacha/facturx | TypeScript | 12/26 | 63,00 ms (×2 535) | 80,75 ms (×1 777) | 79,63 ms (×100) | 127,70 ms |
| @conformo/toolkit | TypeScript | 6/26 | 72,29 ms (×2 909) | 76,06 ms (×1 674) | 121,35 ms (×153) | 114,69 ms |
| mcp-einvoicing-de | Python | 2/26 | 101,48 ms (×4 084) | 105,54 ms (×2 323) | 159,17 ms (×200) | 285,41 ms |
| @lucaapp/zugferd-ts | TypeScript | 8/26 | 2,70 s (×108 660) | 9,24 s (×203 400) | 4,35 s (×5 475) | 2,91 s |
| factur-x | Rust | 17/26 | échec | échec | 1,63 ms (×2,1) | — |

En gras : le plus rapide de la colonne. (×n) : temps rapporté à celui du package Go de ce dépôt. Tri sur la colonne « 13 Ko » ; « Premier appel » : durée du premier appel dans le processus, même colonne (non mesurée pour Go).

#### Écriture modèle → XML

| Librairie | Langage | 13 Ko | 19 Ko | Premier appel |
|---|---|---|---|---|
| **ZeRechnung (ce dépôt)** | Go | **3,3 µs** | **5,3 µs** | — |
| zugferd-rs | Rust | 7,0 µs (×2,1) | 12,0 µs (×2,3) | 59,7 µs |
| core-invoice | Rust | 10,7 µs (×3,3) | 17,1 µs (×3,3) | 46,5 µs |
| @ekwo-ai/factur-x | TypeScript | 11,1 µs (×3,4) | — | 431 µs |
| @stackforge-eu/factur-x | TypeScript | 14,1 µs (×4,3) | — | 525 µs |
| @conformo/toolkit | TypeScript | 21,6 µs (×6,6) | 37,5 µs (×7,1) | 1,55 ms |
| zugferd | Rust | 29,4 µs (×9,0) | — | 446 µs |
| einvoice | Rust | 37,5 µs (×11) | 66,1 µs (×13) | 1,91 ms |
| @attestwire/en16931 | TypeScript | 41,9 µs (×13) | 83,6 µs (×16) | 926 µs |
| en16931 | Rust | 46,6 µs (×14) | 80,5 µs (×15) | 708 µs |
| @stafyniaksacha/facturx | TypeScript | 73,4 µs (×22) | 122 µs (×23) | 1,22 ms |
| pycheval | Python | 77,3 µs (×24) | échec | 143 µs |
| @lucaapp/zugferd-ts | TypeScript | 109 µs (×33) | 194 µs (×37) | 5,09 ms |
| mcp-einvoicing-de | Python | 109 µs (×33) | 184 µs (×35) | 179 µs |
| zugferd-pdf | Python | 122 µs (×37) | — | 273 µs |
| erechnung | Python | 124 µs (×38) | — | 328 µs |
| factur-x-kit | TypeScript | 175 µs (×53) | échec | 5,69 ms |
| @fin.cx/einvoice | TypeScript | 354 µs (×108) | 486 µs (×93) | 3,45 ms |
| getafix | Python | 399 µs (×122) | 688 µs (×131) | 817 µs |
| factur-x | Python | 696 µs (×213) | 851 µs (×162) | 2,05 ms |
| node-zugferd | TypeScript | 931 µs (×284) | — | 4,91 ms |
| drafthorse | Python | 983 µs (×300) | 1,39 ms (×264) | 1,38 ms |
| @e-invoice-eu/core | TypeScript | 39,65 ms (×12 104) | — | 81,68 ms |
| factur-x | Rust | échec | échec | — |
| pdf-invoice | Rust | échec | échec | — |

En gras : le plus rapide de la colonne. (×n) : temps rapporté à celui du package Go de ce dépôt. Tri sur la colonne « 13 Ko » ; « Premier appel » : durée du premier appel dans le processus, même colonne (non mesurée pour Go).

#### Injection dans un PDF

| Librairie | Langage | hôte à xref classique | hôte à flux xref | Premier appel |
|---|---|---|---|---|
| pdfa | Rust | **88,6 µs** (×0,9) | échec | 131 µs |
| **ZeRechnung (ce dépôt)** | Go | 96,2 µs | **96,7 µs** | — |
| pdf-invoice | Rust | 164 µs (×1,7) | 180 µs (×1,9) | 180 µs |
| zugferd-rs | Rust | 206 µs (×2,1) | 221 µs (×2,3) | 218 µs |
| mcp-einvoicing-de | Python | 775 µs (×8,1) | 679 µs (×7,0) | 1,49 ms |
| zugferd-pdf | Python | 888 µs (×9,2) | 782 µs (×8,1) | 5,17 ms |
| @stackforge-eu/factur-x | TypeScript | 1,19 ms (×12) | 1,17 ms (×12) | 14,46 ms |
| @fin.cx/einvoice | TypeScript | 1,88 ms (×20) | 1,95 ms (×20) | 52,01 ms |
| getafix | Python | 2,10 ms (×22) | 2,13 ms (×22) | 2,88 ms |
| drafthorse | Python | 2,13 ms (×22) | 2,16 ms (×22) | 3,38 ms |
| @lucaapp/zugferd-ts | TypeScript | 2,15 ms (×22) | 2,03 ms (×21) | 11,96 ms |
| @ekwo-ai/factur-x | TypeScript | 2,49 ms (×26) | 2,06 ms (×21) | 12,36 ms |
| @conformo/toolkit | TypeScript | 2,50 ms (×26) | 2,54 ms (×26) | 15,69 ms |
| pycheval | Python | 2,55 ms (×27) | 2,65 ms (×27) | 3,48 ms |
| factur-x | Python | 4,32 ms (×45) | 3,98 ms (×41) | 6,58 ms |
| @stafyniaksacha/facturx | TypeScript | échec | échec | — |
| en16931 | Rust | échec | échec | — |

En gras : le plus rapide de la colonne. (×n) : temps rapporté à celui du package Go de ce dépôt. Tri sur la colonne « hôte à xref classique » ; « Premier appel » : durée du premier appel dans le processus, même colonne (non mesurée pour Go).

#### Contrôle d'un PDF hybride

| Librairie | Langage | 149 Ko | 1,1 Mo | Premier appel |
|---|---|---|---|---|
| zugferd-pdf | Python | **213 µs** (×0,3) | **269 µs** (×0,3) | 589 µs |
| @attestwire/en16931 | TypeScript | 368 µs (×0,5) | 403 µs (×0,5) | 11,29 ms |
| en16931 | Rust | 489 µs (×0,6) | 817 µs (×1,0) | 515 µs |
| **ZeRechnung (ce dépôt)** | Go | 811 µs | 856 µs | — |
| @sweberdev/summand | TypeScript | 1,90 ms (×2,3) | 21,72 ms (×25) | 18,68 ms |
| erechnung-validator | Rust | 32,88 ms (×41) | 37,11 ms (×43) | 33,75 ms |
| @conformo/toolkit | TypeScript | 75,14 ms (×93) | 95,02 ms (×111) | 118,45 ms |

En gras : le plus rapide de la colonne. (×n) : temps rapporté à celui du package Go de ce dépôt. Tri sur la colonne « 149 Ko » ; « Premier appel » : durée du premier appel dans le processus, même colonne (non mesurée pour Go).

#### Allocations par appel

| Librairie | Langage | Extraction, PDF de 149 Ko | Lecture, XML de 13 Ko | Validation, XML de 13 Ko | Validation, XML de 1,3 Mo | Écriture, XML de 13 Ko | Injection, hôte à flux xref | Contrôle, PDF de 149 Ko |
|---|---|---|---|---|---|---|---|---|
| **ZeRechnung (ce dépôt)** | Go | 39 Ko · 228 | **28 Ko · 111** | **23 Ko · 82** | 1,3 Mo · 159 | **8 Ko · 1** | **245 Ko · 456** | 700 Ko · 2 383 |
| @attestwire/en16931 | TypeScript | 397 Ko | 278 Ko | 425 Ko | 3,0 Mo | 174 Ko | — | 755 Ko |
| @conformo/toolkit | TypeScript | 483 Ko | 336 Ko | ≥ 60,9 Mo | ≥ 83,2 Mo | 112 Ko | 1,3 Mo | ≥ 62,2 Mo |
| @e-invoice-eu/core | TypeScript | — | — | — | — | ≥ 184,7 Mo | — | — |
| @ekwo-ai/factur-x | TypeScript | 485 Ko | 164 Ko | 164 Ko | 1,2 Mo | 49 Ko | 1,0 Mo | — |
| @fin.cx/einvoice | TypeScript | 1,1 Mo | 7,4 Mo | 11,8 Mo | 15,1 Mo | 773 Ko | 1,2 Mo | — |
| @kontor-mcp/core | TypeScript | 2,1 Mo | 4,5 Mo | 39,0 Mo | 54,7 Mo | — | — | — |
| @lucaapp/zugferd-ts | TypeScript | 493 Ko | 431 Ko | ≥ 13,2 Go | ≥ 20,3 Go | 419 Ko | 1,1 Mo | — |
| @stackforge-eu/factur-x | TypeScript | 484 Ko | — | 112 Ko | 120 Ko | 44 Ko | 2,4 Mo | — |
| @stafyniaksacha/facturx | TypeScript | 501 Ko | 1,0 Mo | ≥ 145,5 Mo | ≥ 164,5 Mo | 118 Ko | échec | — |
| @sweberdev/summand | TypeScript | 235 Ko | — | 3,1 Mo | 6,2 Mo | — | — | 3,4 Mo |
| en16931 | TypeScript | — | — | ≥ 100,4 Mo | ≥ 118,7 Mo | — | — | — |
| factur-x-kit | TypeScript | — | 638 Ko | 696 Ko | échec | 385 Ko | — | — |
| node-zugferd | TypeScript | — | — | — | — | 2,4 Mo | — | — |
| drafthorse | Python | — | 416 Ko · 5 034 | 819 Ko · 6 235 | 164,9 Mo · 6 716 | 1,3 Mo · 12 116 | 5,3 Mo · 34 504 | — |
| erechnung | Python | — | — | — | — | 222 Ko · 2 166 | — | — |
| factur-x | Python | 545 Ko · 3 108 | 2,2 Mo · 24 916 | 795 Ko · 6 359 | 3,6 Mo · 6 505 | 1,1 Mo · 9 710 | 7,0 Mo · 52 592 | — |
| getafix | Python | 623 Ko · 2 723 | 489 Ko · 5 741 | 787 Ko · 9 188 | échec | 505 Ko · 5 306 | 5,4 Mo · 35 125 | — |
| mcp-einvoicing-de | Python | 186 Ko · 1 553 | 572 Ko · 6 040 | 5,0 Mo · 41 334 | 48,8 Mo · 58 413 | 190 Ko · 1 898 | 3,4 Mo · 5 847 | — |
| pycheval | Python | 544 Ko · 2 637 | 374 Ko · 3 302 | — | — | 122 Ko · 1 254 | 6,0 Mo · 39 854 | — |
| saxonche-schematron | Python | — | — | 321 Ko · 749 | 12,7 Mo · 1 332 | — | — | — |
| zugferd-pdf | Python | 187 Ko · 1 552 | — | — | — | 207 Ko · 1 804 | 3,7 Mo · 5 582 | **466 Ko · 4 008** |
| core-invoice | Rust | — | 120 Ko · 113 | 201 Ko · 141 | 3,9 Mo · 191 | 24 Ko · 396 | — | — |
| einvoice | Rust | — | 409 Ko · 8 465 | — | — | 79 Ko · 981 | — | — |
| einvoice-validator | Rust | 94 Ko · 255 | — | 397 Ko · 7 188 | 1,7 Mo · 8 030 | — | — | — |
| en16931 | Rust | 1,3 Mo · 2 199 | 130 Ko · 1 312 | 131 Ko · 1 330 | 3,5 Mo · 1 656 | 80 Ko · 539 | échec | 1,5 Mo · 3 834 |
| erechnung-validator | Rust | — | — | 151,1 Mo · 850 719 | 317,3 Mo · 1 941 234 | — | — | 151,9 Mo · 851 698 |
| factur-x | Rust | — | échec | échec | 2,6 Mo · 1 133 | échec | — | — |
| lopdf | Rust | 529 Ko · 415 | — | — | — | — | — | — |
| pdf-invoice | Rust | 363 Ko · 447 | 82 Ko · 64 | 84 Ko · 87 | **102 Ko · 86** | échec | 949 Ko · 615 | — |
| pdfa | Rust | **36 Ko · 18** | — | — | — | — | échec | — |
| zugferd | Rust | — | — | — | — | 86 Ko · 529 | — | — |
| zugferd-rs | Rust | 544 Ko · 503 | 81 Ko · 77 | 204 Ko · 2 943 | 221 Ko · 2 945 | 28 Ko · 196 | 1,6 Mo · 979 | — |

Octets alloués · nombre d'allocations, pour un appel, après le même contrôle du résultat et un échauffement. Go : `go test -benchmem`, médiane des exécutions. Rust : allocateur global du harnais, qui compte chaque `alloc` et `realloc`. Python : `PYTHONMALLOC=malloc` et un compteur inséré dans le processus, qui compte chaque `malloc`, objets Python et extensions C comprises (libxml2, qpdf). TypeScript : croissance du tas V8 et des ArrayBuffer pendant la série d'appels ; V8 ne compte pas ses allocations, d'où l'absence de nombre, et « ≥ » quand un ramasse-miettes a tourné pendant la série. En gras : le moins d'octets alloués de la colonne.

#### Notes des harnais

Appels mesurés et particularités, pour les opérations réussies ou en échec (les raisons des opérations non prises en charge sont dans les fichiers `COVERAGE.md`).

- **@attestwire/en16931** (TypeScript) : extractFacturX [Extract_Small, Extract_Large, Extract_Minimum, Extract_Extended] ; parseCiiInvoice [Parse_Small, Parse_Lines, Parse_Large] ; parseCiiInvoice hors mesure, puis generateCii(invoice) [Write_Small, Write_Lines] ; validate(octets du PDF) : extraction + règles sur le XML + constats du conteneur (AW-PDF-*, avertissements seulement) ; erreurs de sévérité fatal ; résultat : 0 erreur(s) [CheckPDF_Small, CheckPDF_Large] ; validate(octets) : règles EN 16931 + CIUS XRechnung (profil lu dans BT-24) + codes BR-CL, réécrites en TypeScript (pas de schematron, pas de XSD) ; erreurs de sévérité fatal ; résultat : 0 erreur(s) [ParseValidate_Small, ParseValidate_Lines, ParseValidate_Large]
- **@conformo/toolkit** (TypeScript) : @conformo/parse detectXmlFormat (lecture CII/UBL vers le modèle EN 16931 de @conformo/core) [Parse_Small, Parse_Lines, Parse_Large] ; @conformo/parse extractEmbeddedXml [Extract_Small, Extract_Large, Extract_Minimum, Extract_Extended] ; @conformo/toolkit validate(octets du PDF) : extraction + XSD + schematrons + notices du conteneur (XMP, nom de fichier ; n’affectent pas valid) ; résultat : 0 erreur(s) [CheckPDF_Small, CheckPDF_Large] ; @conformo/validate validate(xml) (défauts) : XSD CII D16B (xmllint-wasm) + schematron CEN EN 16931 CII 1.3.17, et KoSIT XRechnung 3.0.2 si BT-24 le déclare, compilés en SEF et exécutés par SaxonJS ; sévérités error/fatal après les re-niveaux de la configuration KoSIT ; BR-FR non activé (country non fourni) ; résultat : 0 erreur(s) [ParseValidate_Small, ParseValidate_Lines] ; @conformo/validate validate(xml) (défauts) : XSD CII D16B (xmllint-wasm) + schematron CEN EN 16931 CII 1.3.17, et KoSIT XRechnung 3.0.2 si BT-24 le déclare, compilés en SEF et exécutés par SaxonJS ; sévérités error/fatal après les re-niveaux de la configuration KoSIT ; BR-FR non activé (country non fourni) ; résultat : 1 erreur(s) (CII-SR-465) [ParseValidate_Large] ; detectXmlFormat hors mesure, puis @conformo/toolkit serialize(invoice, "cii") [Write_Small, Write_Lines] ; recette de createPdf appliquée à l’hôte : PDFDocument.load (pdf-lib) + @conformo/pdf finalizePDFA (profil en16931, ICC sRGB du toolkit, date fixe) + save({ useObjectStreams: false }) [Embed_Small, EmbedClassic_Small]
- **@e-invoice-eu/core** (TypeScript) : new InvoiceService(logger).generate(json façon UBL de la librairie, { format: "CII", lang: "de-de" }), contrôle ajv du JSON inclus ; modèle construit à la main [Write_Small]
- **@ekwo-ai/factur-x** (TypeScript) : @ekwo-ai/factur-x/pdf embedFacturX(pdf, xml, { profile: "en16931" }) : pdf-lib, pièce jointe + XMP [Embed_Small, EmbedClassic_Small] ; @ekwo-ai/factur-x/pdf extractFacturX [Extract_Small, Extract_Large, Extract_Minimum, Extract_Extended] ; generateCiiXml(invoice, { profile: "en16931" }) ; le modèle lu par readCii n’est pas celui de l’écriture, d’où un modèle Invoice à la main (une seule note, pas de GTIN d’article) ; modèle construit à la main [Write_Small] ; readCii(xml) [Parse_Small, Parse_Lines, Parse_Large] ; readCii(xml).violations : anomalies de lecture (dates, IBAN…) + 8 contrôles arithmétiques BR-CO-10..17 (checkReceived) ; pas de XSD ni de schematron ; résultat : 0 erreur(s) [ParseValidate_Small, ParseValidate_Lines, ParseValidate_Large]
- **@fin.cx/einvoice** (TypeScript) : EInvoice.fromXml [Parse_Small, Parse_Lines, Parse_Large] ; EInvoice.fromXml hors mesure, puis exportXml("facturx") [Write_Small, Write_Lines] ; EInvoice.fromXml puis validate(BUSINESS, { featureFlags: [EN16931_BUSINESS_RULES, CODE_LIST_VALIDATION] }) : règles EN 16931 / XRechnung / Factur-X réécrites en TypeScript (pas de XSD ; la couche schematron dépend de fichiers téléchargés au postinstall et n’est pas utilisée par validate) ; résultat : 0 erreur(s) [ParseValidate_Small, ParseValidate_Lines, ParseValidate_Large] ; new PDFEmbedder().embedXml(pdf, xml, "factur-x.xml", …) : pdf-lib attach + save, sans XMP ni métadonnées PDF/A [Embed_Small, EmbedClassic_Small] ; new PDFExtractor().extractXml (une instance réutilisée ; ses console.log sont neutralisés) [Extract_Small, Extract_Large, Extract_Minimum, Extract_Extended]
- **@kontor-mcp/core** (TypeScript) : extractEmbeddedXml [Extract_Small, Extract_Large, Extract_Minimum, Extract_Extended] ; parseInvoice(octets) [Parse_Small, Parse_Lines, Parse_Large] ; validateInvoice(octets) (défauts) : XSD CII D16B (xmllint-wasm) + schematron CEN EN 16931 CII 1.3.16, plus KoSIT XRechnung 3.0.2 (schematron 2.5.0) si BT-24 le déclare, compilés en SEF et exécutés par SaxonJS ; erreurs error/fatal hors couche « plausibility » (règles maison KONTOR-PLAUS-*, que la librairie exclut elle-même de valid) ; résultat : 0 erreur(s) [ParseValidate_Small, ParseValidate_Lines, ParseValidate_Large]
- **@lucaapp/zugferd-ts** (TypeScript) : embedXml(pdf, xml) (défauts : factur-x.xml, surcouche PDF/A-3b) [Embed_Small, EmbedClassic_Small] ; extractXml [Extract_Small, Extract_Large, Extract_Minimum, Extract_Extended] ; parseCiiXml [Parse_Small, Parse_Lines] ; parseCiiXml ; CII parse: unknown ExchangedDocument.TypeCode "877" [Parse_Large] ; parseCiiXml puis detectProfile hors mesure, serializeToXml(modèle, profil) [Write_Small, Write_Lines] ; validateXml (défauts) : XSD Factur-X 1.08 du profil détecté (xmllint-wasm) + schematron CEN EN 16931 CII 1.3.16 (node-schematron, JS pur), issues de sévérité error ; la librairie classe toute assertion en échec en error (les flag="warning" du CEN ne sont pas lus) ; résultat : 0 erreur(s) [ParseValidate_Small] ; validateXml (défauts) : XSD Factur-X 1.08 du profil détecté (xmllint-wasm) + schematron CEN EN 16931 CII 1.3.16 (node-schematron, JS pur), issues de sévérité error ; la librairie classe toute assertion en échec en error (les flag="warning" du CEN ne sont pas lus) ; résultat : 1 erreur(s) (BR-S-08) [ParseValidate_Lines] ; validateXml (défauts) : XSD Factur-X 1.08 du profil détecté (xmllint-wasm) + schematron CEN EN 16931 CII 1.3.16 (node-schematron, JS pur), issues de sévérité error ; la librairie classe toute assertion en échec en error (les flag="warning" du CEN ne sont pas lus) ; résultat : 3 erreur(s) (CII-SR-465, CII-SR-475, CII-SR-476) [ParseValidate_Large]
- **@stackforge-eu/factur-x** (TypeScript) : buildXml(saisie FacturXInvoiceInput, Profile.EN16931), sans lecteur CII ; modèle construit à la main [Write_Small] ; embedFacturX({ pdf, xml, profile: EN16931 }) : pdf-lib, pièce jointe AFRelationship + XMP PDF/A-3b [Embed_Small, EmbedClassic_Small] ; extractXml [Extract_Small, Extract_Large, Extract_Minimum, Extract_Extended] ; validateXsd(xml, Profile.EN16931) : XSD Factur-X EN16931 seul (libxml2-wasm ; schéma relu et compilé à chaque appel par la librairie) ; pas de schematron EN 16931 ; résultat : 0 erreur(s) [ParseValidate_Small, ParseValidate_Lines, ParseValidate_Large]
- **@stafyniaksacha/facturx** (TypeScript) : check({ xml, level: "en16931", schematron: true }) : XSD Factur-X 1.09 EN16931 (libxml2-wasm) + schematron officiel Factur-X 1.09 EN16931 compilé en SEF, exécuté par SaxonJS ; le résultat ne porte pas de sévérité, toute assertion en échec compte ; niveau forcé à en16931 car la détection automatique lève « Unknown level: kosit » sur les XRECHNUNG ; résultat : 0 erreur(s) [ParseValidate_Small] ; check({ xml, level: "en16931", schematron: true }) : XSD Factur-X 1.09 EN16931 (libxml2-wasm) + schematron officiel Factur-X 1.09 EN16931 compilé en SEF, exécuté par SaxonJS ; le résultat ne porte pas de sévérité, toute assertion en échec compte ; niveau forcé à en16931 car la détection automatique lève « Unknown level: kosit » sur les XRECHNUNG ; résultat : 2 erreur(s) (CII-SR-465, codedb ID) [ParseValidate_Large] ; check({ xml, level: "en16931", schematron: true }) : XSD Factur-X 1.09 EN16931 (libxml2-wasm) + schematron officiel Factur-X 1.09 EN16931 compilé en SEF, exécuté par SaxonJS ; le résultat ne porte pas de sévérité, toute assertion en échec compte ; niveau forcé à en16931 car la détection automatique lève « Unknown level: kosit » sur les XRECHNUNG ; résultat : 4 erreur(s) (PEPPOL-EN16931-R008) [ParseValidate_Lines] ; extract({ pdf }) [Extract_Small, Extract_Large, Extract_Minimum, Extract_Extended] ; generate({ pdf, xml }) (options par défaut) ; Not implemented yet: Document ID already set [Embed_Small, EmbedClassic_Small] ; xmlToInvoice [Parse_Small, Parse_Lines, Parse_Large] ; xmlToInvoice hors mesure, puis invoiceToXml + toString (le XmlDocument libxml2-wasm est libéré après chaque appel) [Write_Small, Write_Lines]
- **@sweberdev/summand** (TypeScript) : extractInvoiceXml [Extract_Small, Extract_Large, Extract_Minimum, Extract_Extended] ; validateInvoice(octets du PDF) : extraction, comparaison du profil XMP, XSD et schematrons sur le XML ; résultat : 0 erreur(s) [CheckPDF_Small, CheckPDF_Large] ; validateInvoice(octets) (défauts) : XSD CII D16B (validateur XSD maison) + schematron CEN EN 16931 CII 1.3.16, et KoSIT XRechnung 2.6.0 si BT-24 le déclare, avec les sévérités de la configuration KoSIT ; moteur XPath 2.0 maison, sans Java ; résultat : 0 erreur(s) [ParseValidate_Small, ParseValidate_Lines] ; validateInvoice(octets) (défauts) : XSD CII D16B (validateur XSD maison) + schematron CEN EN 16931 CII 1.3.16, et KoSIT XRechnung 2.6.0 si BT-24 le déclare, avec les sévérités de la configuration KoSIT ; moteur XPath 2.0 maison, sans Java ; résultat : 1 erreur(s) (CII-SR-465) [ParseValidate_Large]
- **en16931** (TypeScript) : new SchematronRunner(loadCiiSchematron()) une fois (au premier appel), puis runner.validate(xml) : schematron CEN EN 16931 CII 1.3.16 seul (pas de XSD, pas de XRechnung) ; le résultat ne distingue pas les sévérités, toute assertion déclenchée compte ; résultat : 0 erreur(s) [ParseValidate_Small, ParseValidate_Lines] ; new SchematronRunner(loadCiiSchematron()) une fois (au premier appel), puis runner.validate(xml) : schematron CEN EN 16931 CII 1.3.16 seul (pas de XSD, pas de XRechnung) ; le résultat ne distingue pas les sévérités, toute assertion déclenchée compte ; résultat : 3 erreur(s) (CII-SR-465, CII-SR-475, CII-SR-476) [ParseValidate_Large]
- **factur-x-kit** (TypeScript) : FacturX.fromXML hors mesure, puis getXML() [Write_Small] ; FacturX.fromXML hors mesure, puis getXML() ; Received Invalid XML Input [Write_Lines] ; FacturX.fromXML(octets) [Parse_Small] ; FacturX.fromXML(octets) ; Profile not yet implemented: urn:cen.eu:en16931:2017#compliant#urn:xeinkauf.de:kosit:xrechnung_3.0 [Parse_Large] ; FacturX.fromXML(octets) ; Received Invalid XML Input [Parse_Lines] ; FacturX.fromXML(octets) puis validate() : schéma zod du profil détecté et règles BR-* réécrites en TypeScript (MINIMUM à EN16931) ; pas de XSD ni de schematron ; Profile not yet implemented: urn:cen.eu:en16931:2017#compliant#urn:xeinkauf.de:kosit:xrechnung_3.0 [ParseValidate_Large] ; FacturX.fromXML(octets) puis validate() : schéma zod du profil détecté et règles BR-* réécrites en TypeScript (MINIMUM à EN16931) ; pas de XSD ni de schematron ; Received Invalid XML Input [ParseValidate_Lines] ; FacturX.fromXML(octets) puis validate() : schéma zod du profil détecté et règles BR-* réécrites en TypeScript (MINIMUM à EN16931) ; pas de XSD ni de schematron ; résultat : 0 erreur(s) [ParseValidate_Small]
- **node-zugferd** (TypeScript) : zugferd({ profile: EN16931, strict: false }).create(saisie) hors mesure, puis toXML() ; strict: false car la validation XSD de la librairie appelle Java ; modèle construit à la main [Write_Small]
- **drafthorse** (Python) : Document.serialize(schema='FACTUR-X_EN16931'), revalide la sortie contre le XSD [Write_Small, Write_Lines] ; XSD Factur-X 1.0.07 EN16931 (drafthorse.utils.validate_xml, arrêt à la première erreur) [ParseValidate_Small, ParseValidate_Lines, ParseValidate_Large] ; attach_xml, options par défaut (profil détecté, XMP PDF/A-3) ; relu avec factur-x [Embed_Small, EmbedClassic_Small]
- **erechnung** (Python) : erechnung.calculation.calculate + erechnung.xml.xml_bytes (l'API publique ne produit que le PDF complet) ; modèle construit à la main [Write_Small]
- **factur-x** (Python) : XSD Factur-X 1.09.2 seulement (le schematron exige un serveur Saxon HTTP) [ParseValidate_Small, ParseValidate_Lines, ParseValidate_Large] ; check_xsd=False (par défaut l'extraction valide aussi le XML contre le XSD) [Extract_Small, Extract_Large, Extract_Minimum, Extract_Extended] ; generate_cii_xml depuis le dictionnaire de parse_ubl_cii_xml, XSD incluse (défaut), schematron désactivé [Write_Small, Write_Lines] ; generate_from_binary, options par défaut (XSD incluse, passe par un fichier temporaire, XMP PDF/A-3) [Embed_Small, EmbedClassic_Small] ; parse_ubl_cii_xml, XSD incluse (défaut) [Parse_Small, Parse_Lines, Parse_Large]
- **getafix** (Python) : Document.from_xml(xml.etree.ElementTree.fromstring) [Parse_Small, Parse_Lines] ; Document.from_xml(xml.etree.ElementTree.fromstring) ; ValueError: 'urn:cen.eu:en16931:2017#compliant#urn:xeinkauf.de:kosit:xrechnung_3.0' is not a valid Profile [Parse_Large] ; Document.to_xml().render() depuis le modèle lu [Write_Small, Write_Lines] ; attach_xml(bytes, bytes) : pièce jointe générique, pas de XMP Factur-X ni de PDF/A-3 (documenté) [Embed_Small, EmbedClassic_Small] ; extract_xml(Path) : l'API ne prend qu'un chemin, la lecture du fichier (cache OS) est dans la mesure [Extract_Small, Extract_Large, Extract_Minimum, Extract_Extended] ; lecture + règles de getafix (89 des 223 règles BR du CEN, sans BR-CL, + 16 BR-FXEXT) ; avertissements non comptés [ParseValidate_Small, ParseValidate_Lines] ; lecture + règles de getafix (89 des 223 règles BR du CEN, sans BR-CL, + 16 BR-FXEXT) ; avertissements non comptés ; ValueError: 'urn:cen.eu:en16931:2017#compliant#urn:xeinkauf.de:kosit:xrechnung_3.0' is not a valid Profile [ParseValidate_Large]
- **mcp-einvoicing-de** (Python) : ZUGFeRDCIIParser (modèle pydantic EN 16931) [Parse_Small, Parse_Lines, Parse_Large] ; ZUGFeRDCIISerializer depuis le modèle lu (modèle avec pertes : pièces jointes non reprises) [Write_Small, Write_Lines] ; _extract_xml_from_pdf du module invoice_parse (pikepdf, un passage par nom de fichier connu) [Extract_Small, Extract_Large, Extract_Minimum, Extract_Extended] ; invoice_validate local : XSLT FeRD Factur-X 1.09.2 (EN 16931) ou CEN + KoSIT XRechnung 3.0.2 via saxonche, recompilé à chaque appel ; avertissements non comptés [ParseValidate_Small, ParseValidate_Lines, ParseValidate_Large] ; utils.pdf.embed_xml_in_pdf (pikepdf, AFRelationship Alternative, XMP Factur-X) [Embed_Small, EmbedClassic_Small]
- **pycheval** (Python) : extract_facturx_from_pdf(BytesIO), renvoie une str [Extract_Small, Extract_Minimum] ; extract_facturx_from_pdf(BytesIO), renvoie une str ; Timeout: timeout (30 s) : appel interrompu (boucle infinie ?) [Extract_Large, Extract_Extended] ; fonction interne _embed (l'API publique embed_facturx_file_in_pdf lit le XML depuis un fichier), relation Data (défaut) [Embed_Small, EmbedClassic_Small] ; generate_xml depuis le modèle de parse_xml [Write_Small] ; generate_xml depuis le modèle de parse_xml ; InvalidXMLError: Element ./{urn:un:unece:uncefact:data:standard:ReusableAggregateBusinessInformationEntity:100}BuyerAssignedID has no text [Write_Lines] ; parse_xml(BytesIO) [Parse_Small] ; parse_xml(BytesIO) ; InvalidXMLError: Element ./{urn:un:unece:uncefact:data:standard:ReusableAggregateBusinessInformationEntity:100}BuyerAssignedID has no text [Parse_Lines] ; parse_xml(BytesIO) ; UnsupportedProfileError: Unsupported profile: urn:cen.eu:en16931:2017#compliant#urn:xeinkauf.de:kosit:xrechnung_3.0 [Parse_Large]
- **saxonche-schematron** (Python) : XSLT officiels CEN EN 16931 CII 1.3.17 (+ KoSIT XRechnung 3.0.2 schematron 2.6.0 si BT-24 XRechnung) via saxonche, compilés au premier appel ; erreurs fatal/error avec les niveaux KoSIT [ParseValidate_Small, ParseValidate_Lines, ParseValidate_Large]
- **zugferd-pdf** (Python) : generate_facturx_xml (totaux et TVA recalculés depuis les lignes) ; modèle construit à la main [Write_Small] ; package_zugferd_pdfa3u : ajoute OutputIntent sRGB et XMP PDF/A-3U, date de création = maintenant [Embed_Small, EmbedClassic_Small] ; validate_zugferd_pdf : contrôles maison du conteneur (XMP, OutputIntent, /AF) + BT-24 + 2 contrôles de décimales, pas de schematron [CheckPDF_Small, CheckPDF_Large]
- **core-invoice** (Rust) : cii::write_unchecked, modèle lu par read [Write_Small, Write_Lines] ; read → Invoice (modèle sémantique) [Parse_Small, Parse_Lines, Parse_Large] ; validate_xml(profil d'après BT-24) : règles EN 16931 réimplémentées en Rust (alignées sur CEN validation-1.3.16), listes de devises, syntaxe Peppol si profil Peppol ; BR-DE (feature `xrechnung`) sur les XRECHNUNG ; pas de XSD ni de règles CII-SR [ParseValidate_Small, ParseValidate_Lines, ParseValidate_Large]
- **einvoice** (Rust) : yaserde::de::from_str::<CrossIndustryInvoice> (modèle généré du XSD, celui de validate_invoice) [Parse_Small, Parse_Lines, Parse_Large] ; yaserde::ser::to_string, modèle lu par from_str [Write_Small, Write_Lines]
- **einvoice-validator** (Rust) : Validator::cen_cii() : XSD UN/CEFACT CII D16B + XSLT officiel CEN EN 16931 CII 1.3.17 exécuté en Rust ; seule règle hors ligne intégrée (XRechnung, Factur-X et Peppol demandent la feature `fetch` et un téléchargement) [ParseValidate_Small, ParseValidate_Lines, ParseValidate_Large] ; pdf::extract (einvoice-pdf, Limits par défaut) [Extract_Small, Extract_Large, Extract_Minimum, Extract_Extended]
- **en16931** (Rust) : read → Invoice (modèle sémantique EN 16931) [Parse_Small, Parse_Lines, Parse_Large] ; receive sur le PDF : couche conteneur (XMP, /AF, arbre EmbeddedFiles, accord avec BT-24) + validation complète du XML ; pas de contrôle PDF/A-3 au sens ISO 19005 [CheckPDF_Small, CheckPDF_Large] ; receive(Policy::default()) : couches syntaxe et schéma (règles CII-SR/CII-DT générées des artefacts CEN) + règles métier EN 16931 réimplémentées en Rust, profil déclaré par BT-24 (XRechnung 3.0 : BR-DE ; ZUGFeRD EXTENDED/BASIC : règles CEN cœur) ; CEN validation-1.3.17, KoSIT xrechnung-schematron v2.6.0 [ParseValidate_Small, ParseValidate_Lines, ParseValidate_Large] ; the PDF already carries invoice metadata in the namespace "urn:factur-x:pdfa:CrossIndustryDocument:invoice:1p0#"; embedding a second invoice is refused — adaptateur : read + Validated::<En16931>::new + zugferd::embed (la librairie réécrit le XML depuis son modèle ; exige un hôte PDF/A-3 sans métadonnées Factur-X, ne convertit pas en PDF/A) [Embed_Small, EmbedClassic_Small] ; write_unchecked(Syntax::Cii), modèle lu par read [Write_Small, Write_Lines] ; zugferd::extract (Limits par défaut) [Extract_Small, Extract_Large, Extract_Minimum, Extract_Extended]
- **erechnung-validator** (Rust) : Validator::beide(Szenarien::Automatisch) : KoSIT (XSD CII D16B, CEN EN 16931 1.3.16 en Schematron, XRechnung 3.0.2 / schematron 2.6.0 pour les XRECHNUNG, configuration KoSIT, CHANGELOG au 2026-01-31) + Mustang 2.26.0 (XSD et Schematron Factur-X du profil déclaré, XR 3.0 pour les XRECHNUNG) ; erreurs des deux outils additionnées [ParseValidate_Small, ParseValidate_Lines, ParseValidate_Large] ; Validator::beide_pdf : extraction du XML puis KoSIT + Mustang ; aucun contrôle du conteneur PDF/A-3 (partie PDF de Mustang/veraPDF non reprise) [CheckPDF_Small, CheckPDF_Large]
- **factur-x** (Rust) : adaptateur : parse + validate(&Invoice) ; 16 règles sur le modèle (BR-01 à BR-06, BR-09, BR-16, BR-CO-10, BR-CO-13 à BR-CO-18, BR-S-08) ; pas de XSD, pas de règles de profil ni nationales [ParseValidate_Large] ; amount has an invalid value "3.3333" — adaptateur : parse + validate(&Invoice) ; 16 règles sur le modèle (BR-01 à BR-06, BR-09, BR-16, BR-CO-10, BR-CO-13 à BR-CO-18, BR-S-08) ; pas de XSD, pas de règles de profil ni nationales [ParseValidate_Lines] ; amount has an invalid value "3.3333" — parse(Syntax::Cii) → Invoice [Parse_Lines] ; amount has an invalid value "9.9000" — adaptateur : parse + validate(&Invoice) ; 16 règles sur le modèle (BR-01 à BR-06, BR-09, BR-16, BR-CO-10, BR-CO-13 à BR-CO-18, BR-S-08) ; pas de XSD, pas de règles de profil ni nationales [ParseValidate_Small] ; amount has an invalid value "9.9000" — parse(Syntax::Cii) → Invoice [Parse_Small] ; parse(Syntax::Cii) → Invoice [Parse_Large] ; parse: amount has an invalid value "3.3333" [Write_Lines] ; parse: amount has an invalid value "9.9000" [Write_Small]
- **lopdf** (Rust) : plancher : extraction écrite à la main sur lopdf (load_mem complet, arbre /Names/EmbeddedFiles avec /Kids, inflate) ; pas une librairie Factur-X [Extract_Small, Extract_Large, Extract_Minimum, Extract_Extended]
- **pdf-invoice** (Rust) : ZugferdInvoice::from_xml (montants en f64) [Parse_Small, Parse_Lines, Parse_Large] ; adaptateur : Document::load_mem (pdfluent-lopdf) + extract_xml_attachment essayé pour 5 noms standard [Extract_Small, Extract_Large, Extract_Minimum, Extract_Extended] ; adaptateur : Document::load_mem + embed_xml_attachment (Alternative) + add_zugferd_xmp + save_to ; réécrit tout le fichier, remplace le XMP sans conversion PDF/A [Embed_Small, EmbedClassic_Small] ; adaptateur : from_xml + validate (exigences du profil) + validate_en16931 (14 règles : BR-01 à BR-08, BR-CL-01, BR-CL-04, BR-CO-10, BR-CO-13, BR-CO-15, BR-S-08) ; pas de XSD [ParseValidate_Small, ParseValidate_Lines, ParseValidate_Large] ; profile validation: buyer_reference is required for EN16931/Extended; payment_means.type_code is required for EN16931/Extended — to_xml (valide d'abord le profil et les 14 règles EN 16931, refuse d'écrire sinon), modèle lu par from_xml [Write_Small, Write_Lines]
- **pdfa** (Rust) : embed (Level::En16931, horodatage fixe) : réécrit tout le fichier avec une table xref classique et un nouveau XMP PDF/A-3B ; refuse les hôtes à flux d'objets / flux xref [EmbedClassic_Small] ; embedded_file : premier flux /Type /EmbeddedFile trouvé par recherche d'octets [Extract_Small, Extract_Minimum] ; no embedded file read — embedded_file : premier flux /Type /EmbeddedFile trouvé par recherche d'octets [Extract_Large, Extract_Extended] ; the document has no the trailer dictionary — embed (Level::En16931, horodatage fixe) : réécrit tout le fichier avec une table xref classique et un nouveau XMP PDF/A-3B ; refuse les hôtes à flux d'objets / flux xref [Embed_Small]
- **zugferd** (Rust) : modèle construit à la main (contenu de EN16931_Einfach, niveau BASIC : le crate ne génère pas EN 16931) ; Invoice::to_xml_string (serde + quick-xml) [Write_Small]
- **zugferd-rs** (Rust) : embed_xml (profil EN 16931) : réécrit le PDF en PDF/A-3b (XMP Factur-X, OutputIntent sRGB) ; le XML est injecté tel quel [Embed_Small, EmbedClassic_Small] ; extract_xml (lopdf) [Extract_Small, Extract_Large, Extract_Minimum, Extract_Extended] ; invalid invoice type code: 877 — parse_cii_xml → Invoice [Parse_Large] ; parse_cii_xml → Invoice [Parse_Small, Parse_Lines] ; serialize_to_xml(ZugferdProfile::En16931), modèle lu par parse_cii_xml [Write_Small, Write_Lines] ; validate_xml (défaut) : XSD Factur-X 1.09.2 du profil déclaré + Schematron CEN EN 16931 évalué par libxml2 (XPath 1.0 seulement : les règles XPath 2.0 sont sautées, résultat déclaré incomplet) ; pas de BR-DE [ParseValidate_Small, ParseValidate_Lines, ParseValidate_Large]
