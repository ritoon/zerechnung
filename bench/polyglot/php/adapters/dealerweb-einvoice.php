<?php

/*
 * dealerweb/einvoice: EN 16931 model read through the KoSIT syntax-to-model
 * XSLT compiled to PHP, writer for every profile (CII, UBL), its own PDF layer
 * (reads xref tables and streams, writes the document anew), and validation by
 * the official Schematron stylesheets compiled to PHP from Saxon's execution
 * plan, with the KoSIT validator scenarios and message levels.
 *
 * Validation: Validator::validate($xml) with the default frenchRules: false;
 * the scenario comes from BT-24 (README table): a stand-alone CII with the
 * plain EN 16931 identifier gets the KoSIT scenario "EN16931 (CII)" (XSD CII
 * D16B + CEN 1.3.16), XRechnung gets CEN + XRechnung 3.0.2 with the KoSIT
 * levels. Errors = Report::errors() (fatal and error).
 *
 * Write and embed use Generator with validate: false: Invoice::toXml() and
 * toPdf() also run the validator on their output, which would make the write
 * bench a validation bench. Profile from the BT-24 of the model.
 */

declare(strict_types=1);

use Dealerweb\EInvoice\Generation\Generator;
use Dealerweb\EInvoice\Invoice;
use Dealerweb\EInvoice\Profile;
use Dealerweb\EInvoice\Validation\Message;
use Dealerweb\EInvoice\Validation\Validator;

$profileOf = static function (?string $id): Profile {
    $id = strtolower(trim((string) $id));

    return match (true) {
        str_contains($id, 'xrechnung') => Profile::XRechnung,
        str_contains($id, 'extended') => Profile::Extended,
        str_contains($id, 'basicwl') => Profile::BasicWl,
        str_contains($id, 'basic') => Profile::Basic,
        str_contains($id, 'minimum') => Profile::Minimum,
        default => Profile::En16931,
    };
};

$validator = null;

return [
    'pkg' => 'dealerweb/einvoice',
    'notes' => [
        'extract' => 'Invoice::xmlFromPdf (couche PDF propre à la librairie ; XML renvoyé octet pour octet)',
        'parse' => 'Invoice::fromXml (transformation KoSIT syntaxe → modèle, XSLT compilé en PHP)',
        'validate' => 'Validator::validate (frenchRules: false) : scénarios KoSIT, XSD CII D16B + schematron CEN 1.3.16 (+ XRechnung 3.0.2 et niveaux KoSIT si BT-24 XRechnung), XSLT officiels compilés en PHP ; erreurs = Report::errors()',
        'write' => 'new Generator(profil de BT-24, validate: false)->xml du modèle lu par Invoice::fromXml (toXml() validerait aussi la sortie)',
        'embed' => 'new Generator(Profile::En16931, validate: false)->pdf($xml, $hôte) : document réécrit en une révision par la couche PDF de la librairie',
    ],
    'unsupported' => [
        'check_pdf' => 'pas de contrôle du PDF lui-même (README : « No check of the PDF itself »)',
    ],
    'survey_notes' => [
        'validate' => 'scénarios KoSIT : XSD + CEN 1.3.16 (+ XRechnung 3.0.2 et niveaux KoSIT), XSLT officiels compilés en PHP, sans règles françaises',
        'parse' => 'Invoice::fromXml',
        'extract' => 'Invoice::xmlFromPdf',
    ],

    'extract' => static fn (string $pdf, ?string $path): string => Invoice::xmlFromPdf($pdf),

    'parse' => static fn (string $xml, ?string $path): Invoice => Invoice::fromXml($xml),

    'validate' => static function (string $xml, ?string $path) use (&$validator): array {
        $errors = ($validator ??= new Validator())->validate($xml)->errors();

        return [\count($errors), array_map(static fn (Message $m) => $m->code, $errors)];
    },

    'write' => static fn (Invoice $inv): string => (new Generator($profileOf($inv->specification), validate: false))->xml($inv),

    'embed' => static fn (string $host, string $xml): string => (new Generator(Profile::En16931, validate: false))->pdf($xml, $host),
];
