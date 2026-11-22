<?php

/*
 * tiime/factur-x: rewrite of atgp/factur-x for PHP 8.3 (extraction with
 * smalot/pdfparser, injection with FPDI, XSD Factur-X 1.07.3). The Schematron
 * and XSLT files under schematron/ are shipped but never used by src/. The
 * extractor only looks for an attachment named factur-x.xml.
 */

declare(strict_types=1);

use Tiime\FacturX\ProfileExtractor;
use Tiime\FacturX\Reader;
use Tiime\FacturX\Writer;
use Tiime\FacturX\XsdProcessor;
use Tiime\FacturX\XsdValidationException;

return [
    'pkg' => 'tiime/factur-x',
    'notes' => [
        'extract' => 'Reader::extractXML($pdf, validateXsd: false) ; seule une pièce jointe nommée factur-x.xml est cherchée',
        'validate' => 'ProfileExtractor + XsdProcessor : XSD Factur-X 1.07.3 du profil (schematron livré mais jamais appelé)',
        'embed' => 'Writer::generate, options par défaut (XSD incluse, FPDI : la PDF est réécrite, XMP PDF/A-3)',
    ],
    'unsupported' => [
        'parse' => 'pas de modèle (le modèle est tiime/cross-industry-invoice, mesuré à part)',
        'write' => 'pas de modèle',
        'check_pdf' => 'pas de contrôle natif d\'un PDF hybride',
    ],
    'survey_notes' => [
        'validate' => 'XSD Factur-X 1.07.3 seulement',
        'extract' => 'Reader::extractXML(validateXsd: false), pièce jointe factur-x.xml seulement',
    ],

    'extract' => static fn (string $pdf, ?string $path): string => (new Reader())->extractXML($pdf, false),

    'validate' => static function (string $xml, ?string $path): array {
        $doc = new \DOMDocument();
        $doc->loadXML($xml);
        $profile = ProfileExtractor::process($doc);
        try {
            (new XsdProcessor($profile))->validate($xml);
        } catch (XsdValidationException $e) {
            $errors = $e->xmlErrors;

            return [max(1, \count($errors)), array_map(static fn ($er) => 'XSD: ' . mb_substr(trim($er instanceof \LibXMLError ? $er->message : (string) $er), 0, 80), $errors ?: ['?'])];
        }

        return [0, []];
    },

    'embed' => static fn (string $host, string $xml): string => (new Writer())->generate($host, $xml),
];
