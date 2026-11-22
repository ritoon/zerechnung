<?php

/*
 * atgp/factur-x: extraction (smalot/pdfparser), injection (FPDI) and XSD
 * validation (Factur-X 1.09.2, five profiles). No model: Parse and Write are
 * unsupported. XRECHNUNG documents are not a Factur-X profile for
 * ProfileHandler, so their validation throws.
 */

declare(strict_types=1);

use Atgp\FacturX\Reader;
use Atgp\FacturX\Writer;
use Atgp\FacturX\XsdValidator;

return [
    'pkg' => 'atgp/factur-x',
    'notes' => [
        'extract' => 'Reader::extractXML($pdf, validateXsd: false) (par défaut l\'extraction valide aussi le XSD)',
        'validate' => 'XsdValidator::validate : XSD Factur-X 1.09.2 du profil détecté, rien d\'autre',
        'embed' => 'Writer::generate, options par défaut (XSD incluse, FPDI : la PDF est réécrite, XMP PDF/A-3)',
    ],
    'unsupported' => [
        'parse' => 'pas de modèle : la librairie ne manipule que les octets du XML',
        'write' => 'pas de modèle',
        'check_pdf' => 'pas de contrôle natif d\'un PDF hybride',
    ],
    'survey_notes' => [
        'validate' => 'XSD Factur-X 1.09.2 seulement (XsdValidator)',
        'extract' => 'Reader::extractXML(validateXsd: false)',
    ],

    'extract' => static fn (string $pdf, ?string $path): string => (new Reader())->extractXML($pdf, false),

    'validate' => static function (string $xml, ?string $path): array {
        $v = new XsdValidator();
        if ($v->validate($xml)) {
            return [0, []];
        }
        $errors = $v->getXmlErrors();

        return [\count($errors), array_map(static fn (\LibXMLError $e) => 'XSD: ' . mb_substr(trim($e->message), 0, 80), $errors)];
    },

    'embed' => static fn (string $host, string $xml): string => (new Writer())->generate($host, $xml),

    'survey_extract_extra' => static function (array $files): string {
        $ok = 0;
        foreach ($files as $f) {
            try {
                $x = (new Reader())->extractXML((string) file_get_contents($f));
                $ok += (int) (str_contains($x, 'CrossIndustryInvoice') || str_contains($x, 'CrossIndustryDocument'));
            } catch (\Throwable) {
            }
        }

        return sprintf('avec validateXsd: true (défaut) : %d/%d', $ok, \count($files));
    },
];
