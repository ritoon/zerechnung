<?php

/*
 * smalot/pdfparser: general PDF parser, not a Factur-X library. Extraction
 * floor, written here: parse the whole PDF, then take the first /Filespec
 * whose /F or /UF names a known invoice attachment and return its /EF /F
 * stream (decoded by the parser). horstoeko/zugferd, atgp/factur-x and
 * tiime/factur-x all extract with this parser.
 */

declare(strict_types=1);

use Smalot\PdfParser\Parser;

const SMALOT_NAMES = ['factur-x.xml', 'zugferd-invoice.xml', 'xrechnung.xml', 'zugferd_invoice.xml'];

return [
    'pkg' => 'smalot/pdfparser',
    'notes' => [
        'extract' => 'plancher écrit à la main : Parser::parseContent, puis le premier /Filespec nommé factur-x.xml, zugferd-invoice.xml ou xrechnung.xml, flux /EF /F',
    ],
    'unsupported' => [
        'parse' => 'librairie PDF généraliste',
        'validate' => 'librairie PDF généraliste',
        'write' => 'librairie PDF généraliste',
        'embed' => 'lecture seule',
        'check_pdf' => 'librairie PDF généraliste',
    ],
    'survey_notes' => [
        'extract' => 'plancher : Parser::parseContent + /Filespec /EF /F',
    ],

    'extract' => static function (string $pdf, ?string $path): string {
        $doc = (new Parser())->parseContent($pdf);
        foreach ($doc->getObjectsByType('Filespec') as $spec) {
            $name = '';
            foreach (['UF', 'F'] as $k) {
                if ($spec->has($k)) {
                    $name = strtolower(trim((string) $spec->get($k)->getContent()));
                    if ($name !== '') {
                        break;
                    }
                }
            }
            if (!\in_array($name, SMALOT_NAMES, true) || !$spec->has('EF')) {
                continue;
            }
            $ef = $spec->get('EF');
            if (!$ef->has('F')) {
                continue;
            }
            $xml = $ef->get('F')->getContent();
            if (\is_string($xml) && $xml !== '') {
                return $xml;
            }
        }
        throw new \RuntimeException('aucune pièce jointe XML de facture trouvée');
    },
];
