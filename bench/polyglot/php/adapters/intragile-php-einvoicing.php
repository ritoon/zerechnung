<?php

/*
 * intragile/php-einvoicing: EN 16931 model with CII and UBL readers/writers,
 * a Factur-X PDF reader and an incremental-update PDF embedder written from
 * scratch (no FPDI), and validation by the official Schematron rules
 * pre-compiled to PHP (packages intragile/einvoicing-rules-*), run by its own
 * XPath 2.0 evaluator, after the XSD.
 *
 * Validation: XRechnungValidator::create() for BT-24 = EN 16931 or XRechnung
 * (the KoSIT scenario table: XSD CII D16B, CEN EN 16931 1.3.16 as pinned by
 * the KoSIT configuration, plus XRechnung schematron 2.6.0 and the KoSIT
 * custom levels for XRechnung); FacturXValidator::create() for the other
 * Factur-X profiles (Factur-X 1.09.2 schematron + XSD). Both are built once,
 * outside the timing. Errors = fatal findings + schema errors + XPath dynamic
 * errors, as ValidationReport::isValid.
 */

declare(strict_types=1);

use IntrAgile\EInvoicing\Model\Invoice;
use IntrAgile\EInvoicing\Syntax\Cii\CiiReader;
use IntrAgile\EInvoicing\Syntax\Cii\CiiWriter;
use IntrAgile\EInvoicing\Syntax\FacturX\FacturXPdfReader;
use IntrAgile\EInvoicing\Syntax\FacturX\FacturXPdfWriter;
use IntrAgile\EInvoicing\Validation\DynamicError;
use IntrAgile\EInvoicing\Validation\FacturX\FacturXValidator;
use IntrAgile\EInvoicing\Validation\Finding;
use IntrAgile\EInvoicing\Validation\SchemaError;
use IntrAgile\EInvoicing\Validation\XRechnung\XRechnungValidator;

$validators = [];
$pick = static function (string $xml) use (&$validators): XRechnungValidator|FacturXValidator {
    $id = preg_match('~GuidelineSpecifiedDocumentContextParameter>\s*<(?:\w+:)?ID>([^<]+)<~', $xml, $m) ? trim($m[1]) : '';
    $kosit = $id === 'urn:cen.eu:en16931:2017' || str_contains($id, 'xrechnung');

    return $kosit
        ? ($validators['kosit'] ??= XRechnungValidator::create())
        : ($validators['facturx'] ??= FacturXValidator::create());
};

return [
    'pkg' => 'intragile/php-einvoicing',
    'notes' => [
        'extract' => 'FacturXPdfReader::read()->xml (lecteur PDF écrit dans la librairie)',
        'parse' => 'CiiReader::read()->invoice (modèle EN 16931)',
        'validate' => 'XRechnungValidator (scénarios KoSIT : XSD CII D16B + schematron CEN 1.3.16 précompilé en PHP, + XRechnung 2.6.0 et niveaux KoSIT si BT-24 XRechnung) ; FacturXValidator pour les autres profils Factur-X ; erreurs = fatal + XSD + erreurs dynamiques',
        'write' => 'CiiWriter::write du modèle lu par CiiReader',
        'embed' => 'FacturXPdfWriter::embed (mise à jour incrémentale, sans FPDI ; ne convertit pas en PDF/A)',
    ],
    'unsupported' => [
        'check_pdf' => 'pas de contrôle PDF/A (seulement des avertissements à l\'intégration)',
    ],
    'survey_notes' => [
        'validate' => 'scénarios KoSIT : XSD + CEN 1.3.16 (+ XRechnung 2.6.0 et niveaux KoSIT), règles précompilées en PHP',
        'parse' => 'CiiReader::read',
        'extract' => 'FacturXPdfReader::read',
    ],

    'extract' => static fn (string $pdf, ?string $path): string => (new FacturXPdfReader())->read($pdf)->xml,

    'parse' => static fn (string $xml, ?string $path): Invoice => (new CiiReader())->read($xml)->invoice,

    'validate' => static function (string $xml, ?string $path) use ($pick): array {
        $r = $pick($xml)->validate($xml);
        $rules = [
            ...array_map(static fn (SchemaError $e) => 'XSD: ' . mb_substr($e->message, 0, 80), $r->schemaErrors),
            ...array_map(static fn (DynamicError $e) => 'XPATH: ' . ($e->ruleId ?? $e->errorCode), $r->dynamicErrors),
            ...array_map(static fn (Finding $f) => $f->ruleId, $r->fatal()),
        ];

        return [\count($rules), $rules];
    },

    'write' => static fn (Invoice $inv): string => (new CiiWriter())->write($inv),

    'embed' => static fn (string $host, string $xml): string => (new FacturXPdfWriter())->embed($host, $xml)->pdf,
];
