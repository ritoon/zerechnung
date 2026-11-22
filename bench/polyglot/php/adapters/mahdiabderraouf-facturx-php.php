<?php

/*
 * mahdiabderraouf/facturx-php: Factur-X toolkit: XSD validation (Factur-X
 * 1.09.2, profile read from BT-24), XML builder for the MINIMUM, BASIC WL and
 * BASIC profiles, PDF/A-3b embedding with FPDI/tFPDF, and extraction by
 * running poppler's pdfdetach (twice: -list, then -save to a temp file).
 *
 * Its APIs take paths: extraction uses the sample path given by the harness
 * (or a temp copy for the embed read-back), embedding writes the host once to
 * the bench temp dir (outside the timing, cached by content). Generator::
 * generate validates the XML against the XSD before embedding (native).
 * Validation: the profile comes from BT-24 via the Profile enum, which has no
 * XRechnung identifier: Validator::validate throws on XRechnung (native).
 * Write: no reader, so a model built by hand from EN16931_Einfach, written in
 * the BASIC profile (the builder has no EN16931 profile).
 */

declare(strict_types=1);

use MahdiAbderraouf\FacturX\Builder;
use MahdiAbderraouf\FacturX\Enums\InvoiceTypeCode;
use MahdiAbderraouf\FacturX\Enums\NoteSubjectCode;
use MahdiAbderraouf\FacturX\Enums\Profile;
use MahdiAbderraouf\FacturX\Enums\VatCategory;
use MahdiAbderraouf\FacturX\Exceptions\InvalidXmlException;
use MahdiAbderraouf\FacturX\Generator;
use MahdiAbderraouf\FacturX\Models\Address;
use MahdiAbderraouf\FacturX\Models\Buyer;
use MahdiAbderraouf\FacturX\Models\Delivery;
use MahdiAbderraouf\FacturX\Models\Invoice;
use MahdiAbderraouf\FacturX\Models\Line;
use MahdiAbderraouf\FacturX\Models\Note;
use MahdiAbderraouf\FacturX\Models\Payterm;
use MahdiAbderraouf\FacturX\Models\Seller;
use MahdiAbderraouf\FacturX\Models\VatBreakdown;
use MahdiAbderraouf\FacturX\Parser;
use MahdiAbderraouf\FacturX\Validator;

/** Writes $bytes once to the bench temp dir and returns the path (cached by content). */
$asFile = static function (string $bytes, string $ext): string {
    static $paths = [];
    $key = md5($bytes) . $ext;
    if (!isset($paths[$key])) {
        $p = \Bench\tmp_dir() . '/mahdiabderraouf-' . $key;
        if (file_put_contents($p, $bytes) === false) {
            throw new \RuntimeException("cannot write $p");
        }
        $paths[$key] = $p;
    }

    return $paths[$key];
};

return [
    'pkg' => 'mahdiabderraouf/facturx-php',
    'notes' => [
        'extract' => 'Parser::getXml($chemin) : deux appels à pdfdetach de poppler (-list puis -save vers un fichier temporaire)',
        'validate' => 'Validator::validate : XSD Factur-X 1.09.2 du profil lu dans BT-24 (pas d\'identifiant XRechnung dans l\'énumération Profile)',
        'write' => 'Builder::build, profil BASIC (le générateur ne propose pas EN16931 ; pas de lecteur XML)',
        'embed' => 'Generator::generate($cheminHôte, $xml) : validation XSD puis PDF/A-3b par FPDI/tFPDF',
    ],
    'unsupported' => [
        'parse' => 'pas de lecteur XML (Parser::extractBaseData ne lit que la date, le vendeur et le numéro)',
        'check_pdf' => 'pas de contrôle PDF',
    ],
    'survey_notes' => [
        'validate' => 'XSD Factur-X 1.09.2 seulement ; profil lu dans BT-24, XRechnung refusé',
        'extract' => 'Parser::getXml (pdfdetach de poppler)',
    ],

    'extract' => static fn (string $pdf, ?string $path): string => Parser::getXml($path ?? $asFile($pdf, '.pdf')),

    'validate' => static function (string $xml, ?string $path): array {
        try {
            Validator::validate($xml);

            return [0, []];
        } catch (InvalidXmlException $e) {
            $errs = $e->getErrors() ?? [];
            if ($errs === []) {
                // profile refused before any schema check (e.g. XRechnung): no validation ran
                throw $e;
            }

            return [\count($errs), array_map(static fn (\LibXMLError $x) => 'XSD: ' . mb_substr(trim($x->message), 0, 80), $errs)];
        }
    },

    // EN16931_Einfach.cii.xml (XML Small) in the BASIC profile
    'hand_model' => static function (): Invoice {
        return new Invoice(
            profile: Profile::BASIC,
            number: '471102',
            typeCode: InvoiceTypeCode::COMMERCIAL_INVOICE,
            issueDate: new \DateTime('2018-03-05'),
            totalAmountWithoutVAT: 473.00,
            totalVATAmount: 56.87,
            totalAmountWithVAT: 529.87,
            amountDueForPayment: 529.87,
            buyer: new Buyer(
                name: 'Kunden AG Mitte',
                address: new Address(countryCode: 'DE', postCode: '69876', address1: 'Kundenstraße 15', city: 'Frankfurt'),
                identifiers: ['GE2020211'],
            ),
            seller: new Seller(
                name: 'Lieferant GmbH',
                vatIdentifier: 'DE123456789',
                address: new Address(countryCode: 'DE', postCode: '80333', address1: 'Lieferantenstraße 20', city: 'München'),
                identifiers: ['549910'],
            ),
            lines: [
                new Line(identifier: '1', name: 'Trennblätter A4', netPrice: 9.90, totalNetPrice: 198.00, invoicedQuantity: 20.0,
                    invoicedQuantityUnit: 'H87', vatCategory: VatCategory::STANDARD_RATE, vatRate: 19.0, grossPrice: 9.90,
                    standardIdentifier: '4012345001235', schemeIdentifier: '0160'),
                new Line(identifier: '2', name: 'Joghurt Banane', netPrice: 5.50, totalNetPrice: 275.00, invoicedQuantity: 50.0,
                    invoicedQuantityUnit: 'H87', vatCategory: VatCategory::STANDARD_RATE, vatRate: 7.0, grossPrice: 5.50,
                    standardIdentifier: '4000050986428', schemeIdentifier: '0160'),
            ],
            lineNetAmount: 473.00,
            chargesSum: 0.00,
            allowancesSum: 0.00,
            paidAmount: 0.00,
            notes: [
                new Note('Rechnung gemäß Bestellung vom 01.03.2018.'),
                new Note("Lieferant GmbH\nLieferantenstraße 20\n80333 München\nDeutschland\nGeschäftsführer: Hans Muster\nHandelsregisternummer: H A 123", NoteSubjectCode::REGULATORY_INFORMATION),
            ],
            delivery: new Delivery(actualDeliveryDate: new \DateTime('2018-03-05')),
            vatBreakdowns: [
                new VatBreakdown(vatCategoryTaxAmount: 19.25, vatCategoryTaxableAmount: 275.00, vatCategory: VatCategory::STANDARD_RATE, percentage: 7.0),
                new VatBreakdown(vatCategoryTaxAmount: 37.62, vatCategoryTaxableAmount: 198.00, vatCategory: VatCategory::STANDARD_RATE, percentage: 19.0),
            ],
            payterm: new Payterm(paymentTerms: 'Zahlbar innerhalb 30 Tagen netto bis 04.04.2018, 3% Skonto innerhalb 10 Tagen bis 15.03.2018'),
        );
    },

    'write' => static fn (Invoice $inv): string => Builder::build($inv),

    'embed' => static fn (string $host, string $xml): string => Generator::generate($asFile($host, '.pdf'), $xml),
];
