<?php

/*
 * horstoeko/invoicesuite: successor of horstoeko/zugferd by the same author,
 * one DTO for many formats (ZUGFeRD/Factur-X, XRechnung CII and UBL, Peppol,
 * FatturaPA, ...). It declares a Composer conflict with horstoeko/zugferd, so
 * it lives in its own environment (invoicesuite/vendor).
 *
 * Offline validation is XSD only (InvoiceSuiteXsdDocumentValidator, schema of
 * the format provider recognised in the content). The two other validators are
 * not measured: InvoiceSuiteKositDocumentValidator downloads the KoSIT jar and
 * runs Java, InvoiceSuiteDocuflairDocumentValidator posts the invoice to a
 * remote web service.
 */

declare(strict_types=1);

use horstoeko\invoicesuite\InvoiceSuiteDocumentReader;
use horstoeko\invoicesuite\InvoiceSuitePdfDocumentBuilder;
use horstoeko\invoicesuite\InvoiceSuitePdfDocumentReader;
use horstoeko\invoicesuite\utils\InvoiceSuiteMessageBagItem;
use horstoeko\invoicesuite\validators\InvoiceSuiteXsdDocumentValidator;

return [
    'pkg' => 'horstoeko/invoicesuite',
    'notes' => [
        'extract' => 'InvoiceSuitePdfDocumentReader::createFromContent (prinsfrank/pdfparser ; le lecteur désérialise aussi le XML trouvé) puis getInvoiceDocumentAttachment()->getAttachmentContent()',
        'parse' => 'InvoiceSuiteDocumentReader::createFromContent (fournisseur de format deviné, JMS Serializer)',
        'validate' => 'InvoiceSuiteXsdDocumentValidator::createFromContent()->validate() : XSD du fournisseur de format reconnu (Factur-X EN16931 pour XRechnung CII) ; validateurs KoSIT (Java) et Docuflair (service distant) non mesurés',
        'write' => 'InvoiceSuiteDocumentReader::copyToBuilder()->getContent() (DTO puis sérialisation)',
        'embed' => 'InvoiceSuitePdfDocumentBuilder::createFromDocumentContentAndPdfContent()->generatePdfDocumentAndGetContent() (FPDI/TCPDF)',
    ],
    'unsupported' => [
        'check_pdf' => 'pas de contrôle PDF/A',
    ],
    'survey_notes' => [
        'validate' => 'XSD seulement (KoSIT exige Java, Docuflair est un service distant)',
        'parse' => 'InvoiceSuiteDocumentReader::createFromContent',
        'extract' => 'InvoiceSuitePdfDocumentReader::createFromContent (échoue si le XML trouvé ne correspond à aucun fournisseur de format)',
    ],

    'extract' => static function (string $pdf, ?string $path): string {
        $att = InvoiceSuitePdfDocumentReader::createFromContent($pdf)->getInvoiceDocumentAttachment();
        if ($att === null) {
            throw new \RuntimeException('aucune pièce jointe de facture');
        }

        return $att->getAttachmentContent();
    },

    'parse' => static fn (string $xml, ?string $path): InvoiceSuiteDocumentReader => InvoiceSuiteDocumentReader::createFromContent($xml),

    'validate' => static function (string $xml, ?string $path): array {
        $v = InvoiceSuiteXsdDocumentValidator::createFromContent($xml)->validate();
        $msgs = array_merge($v->getErrorMessagesInMessageBag(), $v->getInternalErrorMessagesInMessageBag());

        return [\count($msgs), array_map(static fn (InvoiceSuiteMessageBagItem $m) => 'XSD: ' . mb_substr($m->getMessageContent(), 0, 80), $msgs)];
    },

    'write' => static fn (InvoiceSuiteDocumentReader $r): string => $r->copyToBuilder()->getContent(),

    'embed' => static fn (string $host, string $xml): string => InvoiceSuitePdfDocumentBuilder::createFromDocumentContentAndPdfContent($xml, $host)->generatePdfDocumentAndGetContent(),
];
