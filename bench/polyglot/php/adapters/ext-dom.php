<?php

/*
 * ext-dom (DOMDocument::schemaValidate, libxml2): XSD-only validation floor,
 * written here. Schema: the official Factur-X 1.09.2 XSD of the profile named
 * by BT-24 (EN16931 for XRechnung and plain EN 16931), the files shipped in
 * vendor/atgp/factur-x/xsd (the PHP code of that package is not called).
 */

declare(strict_types=1);

const EXTDOM_XSD_DIR = __DIR__ . '/../vendor/atgp/factur-x/xsd/factur-x';

return [
    'pkg' => 'ext-dom',
    'version' => static fn (): string => \PHP_VERSION . ' (libxml ' . \LIBXML_DOTTED_VERSION . ')',
    'notes' => [
        'validate' => 'plancher écrit à la main : DOMDocument::loadXML + schemaValidate, XSD Factur-X 1.09.2 du profil (EN16931 pour XRechnung), libxml2 ' . \LIBXML_DOTTED_VERSION,
    ],
    'unsupported' => [
        'extract' => 'extension XML seulement',
        'parse' => 'pas de modèle (DOM seul)',
        'write' => 'pas de modèle',
        'embed' => 'extension XML seulement',
        'check_pdf' => 'extension XML seulement',
    ],
    'survey_notes' => [
        'validate' => 'XSD Factur-X 1.09.2 seulement (libxml2 ' . \LIBXML_DOTTED_VERSION . ')',
    ],

    'validate' => static function (string $xml, ?string $path): array {
        $id = preg_match('~GuidelineSpecifiedDocumentContextParameter>\s*<(?:\w+:)?ID>([^<]+)<~', $xml, $m) ? strtolower($m[1]) : '';
        $profile = match (true) {
            str_contains($id, 'extended') => 'extended/Factur-X_1.09.2_EXTENDED.xsd',
            str_contains($id, 'basicwl') => 'basic-wl/Factur-X_1.09.2_BASICWL.xsd',
            str_contains($id, 'basic') => 'basic/Factur-X_1.09.2_BASIC.xsd',
            str_contains($id, 'minimum') => 'minimum/Factur-X_1.09.2_MINIMUM.xsd',
            default => 'en16931/Factur-X_1.09.2_EN16931.xsd',
        };
        $prev = libxml_use_internal_errors(true);
        libxml_clear_errors();
        try {
            $doc = new \DOMDocument();
            if (!$doc->loadXML($xml, \LIBXML_NONET)) {
                return [1, ['XML: not well-formed']];
            }
            if ($doc->schemaValidate(EXTDOM_XSD_DIR . '/' . $profile)) {
                return [0, []];
            }
            $errors = libxml_get_errors();

            return [\count($errors), array_map(static fn (\LibXMLError $e) => 'XSD: ' . mb_substr(trim($e->message), 0, 80), $errors)];
        } finally {
            libxml_clear_errors();
            libxml_use_internal_errors($prev);
        }
    },
];
