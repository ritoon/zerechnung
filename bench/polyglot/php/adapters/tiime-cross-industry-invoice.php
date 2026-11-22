<?php

/*
 * tiime/cross-industry-invoice: typed CII model per profile (Minimum, BasicWL,
 * Basic, EN16931, plus a French "Flux1" model) with fromXML/toXML on DOM, and
 * Twig templates to render it. The EN 16931 semantic model and its converter
 * are in tiime/en-16931 (dependency). The XSD and Schematron files under xsd/
 * are only used by the package's tests: no validation API.
 */

declare(strict_types=1);

use Tiime\CrossIndustryInvoice\EN16931\CrossIndustryInvoice;

return [
    'pkg' => 'tiime/cross-industry-invoice',
    'notes' => [
        'parse' => 'DOMDocument::loadXML + EN16931\\CrossIndustryInvoice::fromXML (XPath sur les préfixes rsm/ram)',
        'write' => 'EN16931\\CrossIndustryInvoice::toXML()->saveXML() du modèle lu par fromXML',
    ],
    'unsupported' => [
        'extract' => 'pas de PDF (voir tiime/factur-x)',
        'validate' => 'pas d\'API de validation (XSD et schematron livrés pour les tests seulement)',
        'embed' => 'pas de PDF (voir tiime/factur-x)',
        'check_pdf' => 'pas de PDF',
    ],
    'survey_notes' => [
        'parse' => 'EN16931\\CrossIndustryInvoice::fromXML',
    ],

    'parse' => static function (string $xml, ?string $path): CrossIndustryInvoice {
        $doc = new \DOMDocument();
        if (!$doc->loadXML($xml)) {
            throw new \RuntimeException('XML illisible');
        }

        return CrossIndustryInvoice::fromXML($doc);
    },

    'write' => static fn (CrossIndustryInvoice $m): string => (string) $m->toXML()->saveXML(),
];
