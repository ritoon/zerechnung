<?php

/*
 * john-wink/en16931-php: native PHP port of the KoSIT validation pipeline
 * (no Java, no XSLT): XSD CII D16B, then the 1276 syntax rules (CII-SR/DT,
 * UBL-CR/SR) evaluated on the DOM from resources/syntax-rules.json, then the
 * 274 business rules (EN 16931 core + XRechnung CIUS/Extension) coded in PHP
 * on its semantic model. Reference: validator-configuration-xrechnung 3.0.2.
 *
 * Rule set picked by BT-24: En16931Validator::xrechnung() when the
 * specification identifier names XRechnung, En16931Validator::en16931()
 * otherwise. Both validators are built once, outside the timing. Errors are
 * the fatal violations (ValidationResult::fatals), warnings are not counted.
 * Reader only: no CII writer, no PDF.
 */

declare(strict_types=1);

use JohnWink\En16931\En16931Validator;
use JohnWink\En16931\Model\Invoice;
use JohnWink\En16931\Reader\CiiInvoiceReader;
use JohnWink\En16931\Violation;

$validators = [];
$pick = static function (string $xml) use (&$validators): En16931Validator {
    $id = preg_match('~GuidelineSpecifiedDocumentContextParameter>\s*<(?:\w+:)?ID>([^<]+)<~', $xml, $m) ? strtolower($m[1]) : '';
    $key = str_contains($id, 'xrechnung') ? 'xrechnung' : 'en16931';

    return $validators[$key] ??= ($key === 'xrechnung' ? En16931Validator::xrechnung() : En16931Validator::en16931());
};

return [
    'pkg' => 'john-wink/en16931-php',
    'notes' => [
        'parse' => 'CiiInvoiceReader::read (DOM + XPath vers le modèle sémantique EN 16931)',
        'validate' => 'En16931Validator::validate : XSD CII D16B + 1276 règles de syntaxe KoSIT + 274 règles métier EN 16931 (+ CIUS XRechnung si BT-24 la nomme), réécrites en PHP ; référence validator-configuration-xrechnung 3.0.2 ; erreurs = violations fatales',
    ],
    'unsupported' => [
        'extract' => 'pas de PDF',
        'write' => 'lecteur seulement (pas d\'écriture CII)',
        'embed' => 'pas de PDF',
        'check_pdf' => 'pas de PDF',
    ],
    'survey_notes' => [
        'validate' => 'XSD CII D16B + syntaxe KoSIT + règles EN 16931, et CIUS XRechnung quand BT-24 la nomme (portage PHP, pas le XSLT officiel)',
        'parse' => 'CiiInvoiceReader::read',
    ],

    'parse' => static fn (string $xml, ?string $path): Invoice => (new CiiInvoiceReader())->read($xml),

    'validate' => static function (string $xml, ?string $path) use ($pick): array {
        $fatals = $pick($xml)->validate($xml)->fatals();

        return [\count($fatals), array_map(static fn (Violation $v) => $v->ruleId, $fatals)];
    },
];
