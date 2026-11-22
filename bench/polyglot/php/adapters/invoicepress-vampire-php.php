<?php

/*
 * invoicepress/vampire-php (VAMPIRE PHP): the official Schematron packages
 * (CEN EN 16931 1.3.17, XRechnung 3.0.2, Factur-X 1.09.2, Peppol, ...)
 * pre-compiled to JSON and run by an XPath 2.0 interpreter written in PHP,
 * after the XSD. The scenario (schema + rule sets + KoSIT custom levels) is
 * chosen from rulesets/scenarios.json, as the KoSIT validator does: XRechnung
 * CII gets EN16931-CII + XRechnung-CII with the KoSIT custom levels, plain
 * EN 16931 CII gets EN16931-CII, Factur-X profiles get their own rule sets.
 *
 * The Validator is built once, outside the timing (README usage). Errors are
 * the messages whose flag blocks acceptance (fatal), as Result::isAccepted.
 * Validation only: no model, no PDF.
 */

declare(strict_types=1);

use EinvoiceValidator\Pipeline\ScenarioResolver;
use EinvoiceValidator\Result\Message;
use EinvoiceValidator\Validator;

$validator = null;
$get = static function () use (&$validator): Validator {
    if ($validator === null) {
        $pkg = \dirname((new \ReflectionClass(Validator::class))->getFileName(), 2);
        $validator = new Validator(
            scenarioResolver: ScenarioResolver::fromJsonFile($pkg . '/rulesets/scenarios.json'),
            baseDir: $pkg,
        );
    }

    return $validator;
};

return [
    'pkg' => 'invoicepress/vampire-php',
    'notes' => [
        'validate' => 'Validator::validateString, scénario KoSIT de rulesets/scenarios.json : XSD puis schematron officiels précompilés (EN16931-CII 1.3.17, + XRechnung-CII 3.0.2 et niveaux KoSIT pour XRechnung), interprète XPath 2.0 en PHP ; erreurs = messages bloquants (fatal)',
    ],
    'unsupported' => [
        'extract' => 'validation seulement',
        'parse' => 'validation seulement (pas de modèle)',
        'write' => 'validation seulement',
        'embed' => 'validation seulement',
        'check_pdf' => 'validation XML seulement',
    ],
    'survey_notes' => [
        'validate' => 'scénarios KoSIT : XSD + schematron CEN 1.3.17 (+ XRechnung 3.0.2 et niveaux KoSIT), interprète XPath 2.0 maison',
    ],

    'validate' => static function (string $xml, ?string $path) use ($get): array {
        $r = $get()->validateString($xml);
        $bad = array_values(array_filter($r->messages(), static fn (Message $m) => $m->flag->blocksAcceptance()));
        if ($bad === [] && !$r->isAccepted()) {
            // a failed stage without a fatal message (no scenario, no verdict)
            return [1, ['VAMPIRE: ' . ($r->hasVerdict() ? 'stage failed' : 'no verdict')]];
        }

        return [\count($bad), array_map(static fn (Message $m) => $m->ruleId ?? $m->stage->name, $bad)];
    },
];
