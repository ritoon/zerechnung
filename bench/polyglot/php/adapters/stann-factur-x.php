<?php

/*
 * stann/factur-x: Factur-X generator for Chorus Pro: a header-only CII model
 * (document, seller, buyer, totals, references: no lines, no VAT breakdown),
 * XSD validation against the Factur-X 1.0.06 schemas it ships, and a PDF/A-3
 * writer with mPDF (pages re-imported by FPDI) that takes the XML as a string.
 *
 * Validation: FacturX::validate with the schema of the profile named by BT-24
 * (EN16931 for XRechnung). The library leaves libxml's error buffer filled
 * when it throws (libxml_use_internal_errors(false) is only reached on
 * success), so the adapter clears it before each call: without that, the
 * errors of one invoice would be counted again for the next ones.
 * Write: not measured, the model cannot hold the lines nor the VAT breakdown
 * of EN16931_Einfach. Embed: mPDF writes its cache in vendor/mpdf/mpdf/tmp
 * (FacturX::createFile gives no way to set tempDir).
 */

declare(strict_types=1);

use Stann\FacturX\Exception\InvalidXMLSchema;
use Stann\FacturX\FacturX;
use Stann\FacturX\Schema;

$schemaFor = static function (string $xml): Schema {
    $id = preg_match('~GuidelineSpecifiedDocumentContextParameter>\s*<(?:\w+:)?ID>([^<]+)<~', $xml, $m) ? strtolower($m[1]) : '';

    return match (true) {
        str_contains($id, 'extended') => Schema::EXTENDED,
        str_contains($id, 'basicwl') => Schema::BASIC_WL,
        str_contains($id, 'basic') => Schema::BASIC,
        str_contains($id, 'minimum') => Schema::MINIMUM,
        default => Schema::EN16931,
    };
};

return [
    'pkg' => 'stann/factur-x',
    'notes' => [
        'validate' => 'FacturX::validate : XSD Factur-X 1.0.06 du profil choisi par l\'adaptateur d\'après BT-24 (EN16931 pour XRechnung) ; tampon d\'erreurs libxml vidé avant chaque appel',
        'embed' => 'FacturX::createFile($hôte, $xml) : pages réimportées par FPDI dans mPDF (PDFA 3-B)',
    ],
    'unsupported' => [
        'extract' => 'pas de lecture (README : « coming soon »)',
        'parse' => 'pas de lecture (README : « coming soon »)',
        'write' => 'modèle d\'en-tête seulement (ni lignes ni ventilation TVA) : EN16931_Einfach n\'est pas représentable',
        'check_pdf' => 'pas de contrôle PDF',
    ],
    'survey_notes' => [
        'validate' => 'XSD Factur-X 1.0.06 seulement',
    ],

    'validate' => static function (string $xml, ?string $path) use ($schemaFor): array {
        libxml_clear_errors();
        try {
            (new FacturX())->validate($xml, $schemaFor($xml));

            return [0, []];
        } catch (InvalidXMLSchema $e) {
            return [\count($e->errors), array_map(static fn (\LibXMLError $x) => 'XSD: ' . mb_substr(trim($x->message), 0, 80), $e->errors)];
        } finally {
            libxml_clear_errors();
            libxml_use_internal_errors(false);
        }
    },

    'embed' => static fn (string $host, string $xml): string => (new FacturX())->createFile($host, $xml),
];
