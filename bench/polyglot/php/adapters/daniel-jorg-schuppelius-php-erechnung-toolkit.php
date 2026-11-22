<?php

/*
 * daniel-jorg-schuppelius/php-erechnung-toolkit (AGPL-3.0-or-later): German
 * e-invoicing toolkit (XRechnung/ZUGFeRD model, UBL and CII parser and
 * generator, order and GAEB formats, Peppol). PDF features need the optional
 * package daniel-jorg-schuppelius/php-pdf-toolkit (AGPL too, installed here),
 * whose ZugferdReader runs poppler's pdfdetach (-list then -savefile to a
 * private temp dir).
 *
 * Validation offline: CiiSchemaValidator (XSD CII D16B) on the XML, then
 * DocumentRulesValidator on the parsed document (profile, supporting
 * documents, periods, Leitweg-ID: BR-29, BR-30, BR-52, BR-CL-24, BR-CO-19/20,
 * BR-DE-22 and TK-* checks). The EN 16931 / XRechnung Schematron goes through
 * KositValidator, which runs the bundled KoSIT jar with Java: not measured.
 * Embed: not measured: ZugferdPdfGenerator renders its own PDF (no host) and
 * ZugferdPdfAugmenter only adds supporting documents to a ZUGFeRD PDF.
 */

declare(strict_types=1);

use ERechnungToolkit\Entities\Document;
use ERechnungToolkit\Generators\ERechnungGenerator;
use ERechnungToolkit\Parsers\ERechnungParser;
use ERechnungToolkit\Parsers\ZugferdPdfParser;
use ERechnungToolkit\Validators\CiiSchemaValidator;
use ERechnungToolkit\Validators\DocumentRulesValidator;
use ERechnungToolkit\Validators\ValidationMessage;

return [
    'pkg' => 'daniel-jorg-schuppelius/php-erechnung-toolkit',
    'notes' => [
        'extract' => 'ZugferdPdfParser::extractXml($chemin) via php-pdf-toolkit (pdfdetach de poppler : -list puis -savefile)',
        'parse' => 'ERechnungParser::parse',
        'validate' => 'CiiSchemaValidator (XSD CII D16B) puis DocumentRulesValidator sur le document lu (profil, pièces jointes, périodes, Leitweg-ID : 7 règles BR et contrôles TK-*) ; schematron EN 16931/XRechnung par KositValidator (Java) non mesuré',
        'write' => 'ERechnungGenerator::generateCii du document lu par ERechnungParser',
    ],
    'unsupported' => [
        'embed' => 'pas d\'intégration dans un PDF hôte (ZugferdPdfGenerator rend son propre PDF ; ZugferdPdfAugmenter n\'ajoute que des pièces justificatives)',
        'check_pdf' => 'pas de contrôle PDF',
    ],
    'survey_notes' => [
        'validate' => 'XSD CII D16B + quelques règles du toolkit (BR-29, BR-30, BR-52, BR-CL-24, BR-CO-19/20, BR-DE-22) ; KoSIT exige Java',
        'parse' => 'ERechnungParser::parse',
        'extract' => 'ZugferdPdfParser::extractXml (pdfdetach)',
    ],

    'extract' => static function (string $pdf, ?string $path): string {
        $p = new ZugferdPdfParser();
        $xml = $path !== null ? $p->extractXml($path) : $p->extractXmlFromString($pdf);
        if ($xml === null) {
            throw new \RuntimeException('aucune facture XML extraite (pdfdetach indisponible ou pas de pièce jointe)');
        }

        return $xml;
    },

    'parse' => static fn (string $xml, ?string $path): Document => (new ERechnungParser())->parse($xml),

    'validate' => static function (string $xml, ?string $path): array {
        $rules = array_map(static fn (string $e) => 'XSD: ' . mb_substr($e, 0, 80), (new CiiSchemaValidator())->validate($xml));
        $doc = (new ERechnungParser())->parse($xml);
        foreach ((new DocumentRulesValidator())->validate($doc)->getErrors() as $m) {
            /** @var ValidationMessage $m */
            $rules[] = $m->getCode();
        }

        return [\count($rules), $rules];
    },

    'write' => static fn (Document $d): string => (new ERechnungGenerator())->generateCii($d),
];
