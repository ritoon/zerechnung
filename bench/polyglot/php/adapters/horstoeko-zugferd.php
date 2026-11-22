<?php

/*
 * horstoeko/zugferd: the reference PHP library (reader, builder, PDF reader,
 * PDF merger, validators).
 *
 * Offline validation is the Factur-X XSD of the profile resolved from BT-24
 * (ZugferdXsdValidator, run on the document re-serialized by JMS). The
 * Schematron files shipped in src/schema/{schematron,xslt} are declared in
 * ZugferdProfiles but never run; ZugferdKositValidator and ZugferdPdfValidator
 * download the KoSIT validator or veraPDF and start Java: not used here.
 */

declare(strict_types=1);

use horstoeko\zugferd\ZugferdDocumentPdfMerger;
use horstoeko\zugferd\ZugferdDocumentPdfReader;
use horstoeko\zugferd\ZugferdDocumentReader;
use horstoeko\zugferd\ZugferdXsdValidator;

return [
    'pkg' => 'horstoeko/zugferd',
    'notes' => [
        'extract' => 'ZugferdDocumentPdfReader::getXmlFromContent (smalot/pdfparser)',
        'parse' => 'ZugferdDocumentReader::readAndGuessFromContent (JMS Serializer, profil résolu par BT-24)',
        'validate' => 'lecture + ZugferdXsdValidator : XSD Factur-X du profil sur le document resérialisé ; schematron embarqué non exécuté, KoSIT exige Java',
        'write' => 'ZugferdDocument::serializeAsXml (JMS) du modèle lu par ZugferdDocumentReader',
        'embed' => 'ZugferdDocumentPdfMerger(xml, pdf)->generateDocument()->downloadString() (FPDI : la PDF est réécrite, XMP PDF/A-3B)',
    ],
    'unsupported' => [
        'check_pdf' => 'ZugferdPdfValidator télécharge veraPDF et lance Java (exclu)',
    ],
    'survey_notes' => [
        'validate' => 'XSD Factur-X du profil (ZugferdXsdValidator) après lecture ; pas de schematron hors ligne',
        'parse' => 'ZugferdDocumentReader::readAndGuessFromContent',
        'extract' => 'ZugferdDocumentPdfReader::getXmlFromContent',
    ],

    'extract' => static fn (string $pdf, ?string $path): string => ZugferdDocumentPdfReader::getXmlFromContent($pdf),

    'parse' => static fn (string $xml, ?string $path): ZugferdDocumentReader => ZugferdDocumentReader::readAndGuessFromContent($xml),

    'validate' => static function (string $xml, ?string $path): array {
        $doc = ZugferdDocumentReader::readAndGuessFromContent($xml);
        $v = (new ZugferdXsdValidator($doc))->validate();
        $errors = $v->validationErrors();

        return [\count($errors), array_map(static fn ($e) => 'XSD: ' . mb_substr(trim((string) $e), 0, 80), $errors)];
    },

    'write' => static fn (ZugferdDocumentReader $doc): string => $doc->serializeAsXml(),

    'embed' => static fn (string $host, string $xml): string => (new ZugferdDocumentPdfMerger($xml, $host))->generateDocument()->downloadString(),
];
