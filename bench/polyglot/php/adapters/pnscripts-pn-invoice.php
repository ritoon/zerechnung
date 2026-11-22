<?php

/*
 * pnscripts/pn-invoice: EN 16931 core model with UBL and CII writers, and a
 * validator: XSD (UBL 2.1 / CII D16B), then 104 EN 16931 business rules
 * written in PHP (BR-nn, BR-CO, BR-S/Z/E/AE; no BR-CL, BR-DEC, no K/G/O/L/M
 * rules, no syntax rules), against artefacts 1.3.16 semantics. The Schematron
 * layer is an optional external command (Saxon on Java): not measured.
 * No XML reader (README "What is not covered"), so Write uses a model built
 * by hand from EN16931_Einfach; Write_Lines is not measured (protocol).
 * No PDF (Factur-X embedding is announced for a commercial "Pro" package).
 */

declare(strict_types=1);

use Pnscripts\Invoice\Model\Address;
use Pnscripts\Invoice\Model\Identifier;
use Pnscripts\Invoice\Model\Invoice;
use Pnscripts\Invoice\Model\Line;
use Pnscripts\Invoice\Model\Party;
use Pnscripts\Invoice\Model\Specification;
use Pnscripts\Invoice\Model\TaxCategory;
use Pnscripts\Invoice\Validation\InvoiceValidator;
use Pnscripts\Invoice\Validation\Violation;
use Pnscripts\Invoice\Writer\CiiWriter;

$validator = null;

return [
    'pkg' => 'pnscripts/pn-invoice',
    'notes' => [
        'validate' => 'InvoiceValidator::validateXml : XSD CII D16B + 104 règles métier EN 16931 en PHP (pas BR-CL, BR-DEC, ni règles de syntaxe) ; couche schematron externe (Saxon/Java) non branchée ; erreurs = ValidationResult::errors()',
        'write' => 'CiiWriter::write (pas de lecteur XML)',
    ],
    'unsupported' => [
        'extract' => 'pas de PDF',
        'parse' => 'pas de lecteur XML (README : « No XML reader »)',
        'embed' => 'pas de PDF (annoncé pour une offre commerciale « Pro »)',
        'check_pdf' => 'pas de PDF',
    ],
    'survey_notes' => [
        'validate' => 'XSD CII D16B + 104 règles EN 16931 en PHP (sans BR-CL ni BR-DEC)',
    ],

    'validate' => static function (string $xml, ?string $path) use (&$validator): array {
        $errors = ($validator ??= new InvoiceValidator())->validateXml($xml)->errors();

        return [\count($errors), array_map(static fn (Violation $v) => $v->ruleId, $errors)];
    },

    // EN16931_Einfach.cii.xml (XML Small), field by field where the model has a place
    'hand_model' => static function (): Invoice {
        $s19 = TaxCategory::standard('19.00');
        $s7 = TaxCategory::standard('7.00');

        return new Invoice(
            number: '471102',
            issueDate: new \DateTimeImmutable('2018-03-05'),
            currency: 'EUR',
            seller: new Party(
                name: 'Lieferant GmbH',
                address: new Address(countryCode: 'DE', line1: 'Lieferantenstraße 20', city: 'München', postalCode: '80333'),
                vatId: 'DE123456789',
                taxRegistrationId: '201/113/40209',
                identifiers: [new Identifier('549910'), new Identifier('4000001123452', '0088')],
            ),
            buyer: new Party(
                name: 'Kunden AG Mitte',
                address: new Address(countryCode: 'DE', line1: 'Kundenstraße 15', city: 'Frankfurt', postalCode: '69876'),
                identifiers: [new Identifier('GE2020211')],
            ),
            lines: [
                new Line(id: '1', itemName: 'Trennblätter A4', quantity: '20.0000', unitCode: 'H87', netPrice: '9.9000', taxCategory: $s19,
                    grossPrice: '9.9000', sellersItemId: 'TB100A4', standardItemId: new Identifier('4012345001235', '0160')),
                new Line(id: '2', itemName: 'Joghurt Banane', quantity: '50.0000', unitCode: 'H87', netPrice: '5.5000', taxCategory: $s7,
                    grossPrice: '5.5000', sellersItemId: 'ARNR2', standardItemId: new Identifier('4000050986428', '0160')),
            ],
            specification: Specification::en16931(),
            notes: [
                'Rechnung gemäß Bestellung vom 01.03.2018.',
                "Lieferant GmbH\nLieferantenstraße 20\n80333 München\nDeutschland\nGeschäftsführer: Hans Muster\nHandelsregisternummer: H A 123",
            ],
            deliveryDate: new \DateTimeImmutable('2018-03-05'),
            paymentTerms: 'Zahlbar innerhalb 30 Tagen netto bis 04.04.2018, 3% Skonto innerhalb 10 Tagen bis 15.03.2018',
        );
    },

    'write' => static fn (Invoice $inv): string => (new CiiWriter())->write($inv),
];
