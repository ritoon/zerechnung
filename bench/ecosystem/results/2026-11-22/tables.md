<!-- Généré par bench/ecosystem/report à partir de 2026-11-22 -->

#### Exactitude sur le corpus

| Librairie | XML valides rejetés | XML parsés | PDF extraits |
|---|---|---|---|
| **ZeRechnung (ce dépôt)** | 6/26 | 26/26 | 104/104 |
| DCNWS | 2/26 | 26/26 | 104/104 |
| andee | 22/26 | 26/26 | 75/104 |
| angelo | 4/26 | 26/26 | 75/104 |
| dotwave | — | 26/26 | 75/104 |
| fX | 26/26 | 26/26 | 75/104 |
| gobl.de | 20/26 | 26/26 | — |
| gopdfattach | — | — | 97/104 |
| kanjo | 9/26 | 26/26 | 75/104 |
| litho | — | — | 104/104 |
| papierleicht | 6/26 | — | 104/104 |
| pdf0 | 6/26 | — | 104/104 |
| pdfcpu | — | — | 75/104 |
| speedata | 0/26 | — | 104/104 |
| taha | 15/26 | 26/26 | 48/104 |

#### Extraction

| Librairie | EN 16931, 149 Ko | EN 16931, 1,1 Mo | MINIMUM FNFE, 89 Ko | EXTENDED ZF 2.0, 157 Ko | Mémoire/op | Allocations/op |
|---|---|---|---|---|---|---|
| **ZeRechnung (ce dépôt)** | **30,0 µs** | **32,8 µs** | **11,2 µs** | **36,7 µs** | 39 Ko | 228 |
| speedata | 72,6 µs (×2,4) | 237 µs (×7,2) | 25,7 µs (×2,3) | 82,3 µs (×2,2) | 401 Ko | 318 |
| litho | 107 µs (×3,6) | 153 µs (×4,7) | 52,5 µs (×4,7) | 115 µs (×3,1) | 585 Ko | 826 |
| papierleicht | 110 µs (×3,7) | 2,36 ms (×72,0) | 34,5 µs (×3,1) | 118 µs (×3,2) | 417 Ko | 490 |
| DCNWS | 200 µs (×6,7) | 2,35 ms (×71,5) | 104 µs (×9,3) | 182 µs (×5,0) | 184 Ko | 4 926 |
| dotwave | 323 µs (×10,8) | 2,51 ms (×76,3) | 227 µs (×20,2) | échec | 628 Ko | 3 304 |
| pdfcpu | 329 µs (×11,0) | 2,49 ms (×75,9) | 241 µs (×21,5) | échec | 613 Ko | 3 629 |
| fX | 331 µs (×11,0) | 2,53 ms (×76,9) | 233 µs (×20,8) | échec | 628 Ko | 3 305 |
| angelo | 347 µs (×11,6) | 2,52 ms (×76,9) | 246 µs (×21,9) | échec | 628 Ko | 3 303 |
| kanjo | 352 µs (×11,7) | 2,74 ms (×83,4) | 252 µs (×22,5) | échec | 628 Ko | 3 308 |
| gopdfattach | 385 µs (×12,8) | 541 µs (×16,5) | échec | 392 µs (×10,7) | 651 Ko | 3 136 |
| andee | 509 µs (×17,0) | 692 µs (×21,1) | 526 µs (×46,9) | échec | 658 Ko | 4 390 |
| taha | échec | échec | 2,74 ms (×243,9) | échec | — | — |

Mémoire et allocations : colonne « EN 16931, 149 Ko ». Tri sur cette même colonne.

#### Parsing

| Librairie | 13 Ko | 19 Ko | 1,3 Mo | Mémoire/op | Allocations/op |
|---|---|---|---|---|---|
| **ZeRechnung (ce dépôt)** | **18,2 µs** | **32,4 µs** | **1,11 ms** | 28 Ko | 104 |
| angelo | 116 µs (×6,4) | 184 µs (×5,7) | 6,75 ms (×6,1) | 56 Ko | 1 268 |
| fX | 119 µs (×6,5) | 189 µs (×5,8) | 7,35 ms (×6,6) | 53 Ko | 1 165 |
| dotwave | 138 µs (×7,6) | 201 µs (×6,2) | 6,61 ms (×5,9) | 64 Ko | 1 253 |
| taha | 138 µs (×7,6) | 224 µs (×6,9) | 8,77 ms (×7,9) | 209 Ko | 1 237 |
| DCNWS | 164 µs (×9,0) | 248 µs (×7,7) | 6,94 ms (×6,2) | 128 Ko | 2 079 |
| andee | 176 µs (×9,7) | 244 µs (×7,5) | 7,06 ms (×6,3) | 78 Ko | 1 326 |
| speedata | 207 µs (×11,4) | 341 µs (×10,5) | 7,26 ms (×6,5) | 331 Ko | 4 531 |
| kanjo | 244 µs (×13,4) | 370 µs (×11,4) | 13,40 ms (×12,0) | 99 Ko | 2 036 |
| gobl.de | 503 µs (×27,7) | 786 µs (×24,2) | 22,19 ms (×19,9) | 256 Ko | 5 246 |

Mémoire et allocations : colonne « 13 Ko ». Tri sur cette même colonne.

#### Validation

| Librairie | Valides rejetés /26 | 13 Ko | 19 Ko | 1,3 Mo | Mémoire/op | Allocations/op |
|---|---|---|---|---|---|---|
| **ZeRechnung (ce dépôt)** | 6/26 | **24,6 µs** | **44,1 µs** | **757 µs** | 23 Ko | 75 |
| fX | 26/26 | 121 µs (×4,9) | 192 µs (×4,3) | 6,83 ms (×9,0) | 58 Ko | 1 224 |
| pdf0 | 6/26 | 128 µs (×5,2) | 203 µs (×4,6) | 7,18 ms (×9,5) | 67 Ko | 1 462 |
| angelo | 4/26 | 141 µs (×5,7) | 214 µs (×4,9) | 6,70 ms (×8,8) | 91 Ko | 1 912 |
| taha | 15/26 | 141 µs (×5,7) | 227 µs (×5,1) | 8,65 ms (×11,4) | 209 Ko | 1 246 |
| andee | 22/26 | 203 µs (×8,2) | 293 µs (×6,6) | 7,16 ms (×9,5) | 100 Ko | 2 166 |
| kanjo | 9/26 | 281 µs (×11,4) | 409 µs (×9,3) | 13,46 ms (×17,8) | 120 Ko | 2 106 |
| DCNWS | 2/26 | 752 µs (×30,5) | 1,24 ms (×28,1) | 7,41 ms (×9,8) | 624 Ko | 15 279 |
| speedata | 0/26 | 1,06 ms (×43,0) | 1,94 ms (×43,9) | 8,35 ms (×11,0) | 1,9 Mo | 46 459 |
| gobl.de | 20/26 | 1,64 ms (×66,5) | 2,57 ms (×58,3) | 23,96 ms (×31,7) | 981 Ko | 15 543 |
| papierleicht | 6/26 | 13,71 ms (×556,3) | 24,28 ms (×550,4) | 14,55 ms (×19,2) | 65,6 Mo | 254 560 |

Mémoire et allocations : colonne « 13 Ko ». Tri sur cette même colonne.

#### Écriture

| Librairie | 13 Ko | 19 Ko | Mémoire/op | Allocations/op |
|---|---|---|---|---|
| **ZeRechnung (ce dépôt)** | **3,3 µs** | **5,4 µs** | 8 Ko | 1 |
| fX | 20,3 µs (×6,2) | 36,0 µs (×6,7) | 32 Ko | 175 |
| dotwave | 20,4 µs (×6,2) | 33,4 µs (×6,2) | 34 Ko | 361 |
| speedata | 27,1 µs (×8,2) | 49,0 µs (×9,2) | 44 Ko | 988 |
| DCNWS | 36,0 µs (×10,9) | 70,9 µs (×13,2) | 72 Ko | 1 130 |
| kanjo | 101 µs (×30,6) | 175 µs (×32,6) | 547 Ko | 826 |
| andee | 300 µs (×91,1) | 531 µs (×99,1) | 165 Ko | 2 980 |
| taha | 653 µs (×198,3) | 1,29 ms (×241,3) | 812 Ko | 341 |

Mémoire et allocations : colonne « 13 Ko ». Tri sur cette même colonne.

#### Injection

| Librairie | hôte à xref classique | hôte à flux xref | Mémoire/op | Allocations/op |
|---|---|---|---|---|
| **ZeRechnung (ce dépôt)** | **96,2 µs** | **98,7 µs** | 249 Ko | 438 |
| kanjo | 340 µs (×3,5) | échec | 739 Ko | 3 213 |
| DCNWS | 406 µs (×4,2) | 1,70 ms (×17,2) | 2,5 Mo | 6 157 |
| pdfcpu | 428 µs (×4,4) | 451 µs (×4,6) | 1,5 Mo | 3 840 |
| pdf0 | 453 µs (×4,7) | 529 µs (×5,4) | 912 Ko | 9 390 |
| andee | 565 µs (×5,9) | échec | 2,0 Mo | 6 913 |
| gopdfattach | 1,08 ms (×11,2) | 1,18 ms (×11,9) | 5,2 Mo | 7 992 |
| dotwave | 1,10 ms (×11,4) | 1,15 ms (×11,7) | 4,9 Mo | 8 797 |
| litho | 35,09 ms (×364,6) | 35,41 ms (×358,7) | 34,7 Mo | 394 045 |

Mémoire et allocations : colonne « hôte à xref classique ». Tri sur cette même colonne.

#### Contrôle

| Librairie | 149 Ko | 1,1 Mo | Mémoire/op | Allocations/op |
|---|---|---|---|---|
| **ZeRechnung (ce dépôt)** | **832 µs** | **860 µs** | 701 Ko | 2 376 |
| DCNWS | 1,11 ms (×1,3) | 3,43 ms (×4,0) | 896 Ko | 24 617 |
| pdf0 | 2,22 ms (×2,7) | 2,56 ms (×3,0) | 1,8 Mo | 7 536 |
| papierleicht | 14,03 ms (×16,9) | 17,04 ms (×19,8) | 66,0 Mo | 255 047 |
| litho | 20,20 ms (×24,3) | 21,40 ms (×24,9) | 16,7 Mo | 298 499 |

Mémoire et allocations : colonne « 149 Ko ». Tri sur cette même colonne.

#### Planchers

| Librairie | RawToken 13 Ko | RawToken 19 Ko | RawToken 1,3 Mo | memcpy 149 Ko | Mémoire/op | Allocations/op |
|---|---|---|---|---|---|---|
| plancher | **96,3 µs** | **150 µs** | **6,93 ms** | **1,8 µs** | 31 Ko | 711 |

Mémoire et allocations : colonne « RawToken 13 Ko ». Tri sur cette même colonne.

#### Allocations par appel

| Librairie | Extraction, PDF de 149 Ko | Lecture, XML de 13 Ko | Validation, XML de 13 Ko | Validation, XML de 1,3 Mo | Écriture, XML de 13 Ko | Injection, hôte à flux xref | Contrôle, PDF de 149 Ko |
|---|---|---|---|---|---|---|---|
| **ZeRechnung (ce dépôt)** | **39 Ko · 228** | **28 Ko · 104** | **23 Ko · 75** | **1,3 Mo · 159** | **8 Ko · 1** | **245 Ko · 456** | **701 Ko · 2 376** |
| andee | 658 Ko · 4 390 | 78 Ko · 1 326 | 100 Ko · 2 166 | 7,8 Mo · 2 284 | 165 Ko · 2 980 | — | — |
| angelo | 628 Ko · 3 303 | 56 Ko · 1 268 | 91 Ko · 1 912 | 4,1 Mo · 2 133 | — | — | — |
| DCNWS | 184 Ko · 4 926 | 128 Ko · 2 079 | 624 Ko · 15 279 | 7,1 Mo · 15 945 | 72 Ko · 1 130 | 3,7 Mo · 44 347 | 896 Ko · 24 617 |
| dotwave | 628 Ko · 3 304 | 64 Ko · 1 253 | — | — | 34 Ko · 361 | 5,0 Mo · 8 855 | — |
| fX | 628 Ko · 3 305 | 53 Ko · 1 165 | 58 Ko · 1 224 | 4,1 Mo · 1 506 | 32 Ko · 175 | — | — |
| gobl.de | — | 256 Ko · 5 246 | 981 Ko · 15 543 | 11,1 Mo · 17 829 | — | — | — |
| gopdfattach | 651 Ko · 3 136 | — | — | — | — | 5,3 Mo · 8 043 | — |
| kanjo | 628 Ko · 3 308 | 99 Ko · 2 036 | 120 Ko · 2 106 | 8,1 Mo · 2 535 | 547 Ko · 826 | — | — |
| litho | 585 Ko · 826 | — | — | — | — | 35,1 Mo · 394 412 | 16,7 Mo · 298 499 |
| papierleicht | 417 Ko · 490 | — | 65,6 Mo · 254 560 | 24,1 Mo · 54 181 | — | — | 66,0 Mo · 255 047 |
| pdf0 | — | — | 67 Ko · 1 462 | 6,6 Mo · 1 842 | — | 1,3 Mo · 9 593 | 1,8 Mo · 7 536 |
| pdfcpu | 613 Ko · 3 629 | — | — | — | — | 1,5 Mo · 3 896 | — |
| speedata | 401 Ko · 318 | 331 Ko · 4 531 | 1,9 Mo · 46 459 | 11,4 Mo · 59 403 | 44 Ko · 988 | — | — |
| taha | — | 209 Ko · 1 237 | 209 Ko · 1 246 | 20,3 Mo · 1 562 | 812 Ko · 341 | — | — |
| *plancher : jetons `encoding/xml`* |  | *31 Ko · 711* |  |  |  |  |  |

Octets alloués · nombre d'allocations, pour un appel (`go test -benchmem`, médiane des exécutions). « — » : opération non proposée, ou en échec. En gras : le moins d'octets alloués de la colonne.
