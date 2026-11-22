<?php

/*
 * pat-o-dev/factur-x: French-oriented Factur-X EN 16931 generator: CII writer
 * from its own model, PDF attacher by incremental update, XML extractor, and
 * a "curated, hand-picked" validator (structure, BR-CO-15, BR-CO-25,
 * BR-CO-5-6, and the French mapping rules BR-FR-MAP-12/14).
 *
 * Validation: InvoiceValidator::validate; the failures of the French rules
 * (BR-FR-*) are not counted (protocol: no French rules).
 * Write: no XML reader, so a model built by hand from EN16931_Einfach (unit
 * H87 has no case in UnitOfMeasureCode: C62; note subject REG has none
 * either: plain note). Embed: PdfA3Attacher::attach with the XML Small and
 * the package's XmpMetadataBuilder packet (generateHybridPdf only takes a
 * model); classic xref only, and a catalog with /AF or /Names is refused.
 */

declare(strict_types=1);

use PatODev\FacturX\Enum\InvoiceTypeCode;
use PatODev\FacturX\Enum\PartyIdentifierScheme;
use PatODev\FacturX\Enum\UnitOfMeasureCode;
use PatODev\FacturX\Enum\VatCategory;
use PatODev\FacturX\FacturXGenerator;
use PatODev\FacturX\Model\Address;
use PatODev\FacturX\Model\Identifier;
use PatODev\FacturX\Model\Invoice;
use PatODev\FacturX\Model\InvoiceLine;
use PatODev\FacturX\Model\Note;
use PatODev\FacturX\Model\Party;
use PatODev\FacturX\Pdf\EmbeddedXmlExtractor;
use PatODev\FacturX\Pdf\PdfA3Attacher;
use PatODev\FacturX\Pdf\XmpMetadataBuilder;
use PatODev\FacturX\Validation\InvoiceValidator;
use PatODev\FacturX\Validation\RuleResult;

return [
    'pkg' => 'pat-o-dev/factur-x',
    'notes' => [
        'extract' => 'EmbeddedXmlExtractor::extract (xref classique seulement)',
        'validate' => 'InvoiceValidator::validate : sélection de règles écrites à la main (structure, BR-CO-15, BR-CO-25, BR-CO-5-6), sans XSD ni schematron ; échecs BR-FR-* (règles françaises) non comptés',
        'write' => 'FacturXGenerator::generateXml (pas de lecteur XML)',
        'embed' => 'PdfA3Attacher::attach + XmpMetadataBuilder::build (mise à jour incrémentale ; xref classique seulement, refuse un catalogue avec /AF ou /Names)',
    ],
    'unsupported' => [
        'parse' => 'pas de lecteur XML',
        'check_pdf' => 'pas de contrôle PDF',
    ],
    'survey_notes' => [
        'validate' => 'quelques règles choisies à la main (structure, BR-CO-15, BR-CO-25, BR-CO-5-6), sans les règles françaises',
        'extract' => 'EmbeddedXmlExtractor::extract (xref classique seulement)',
    ],

    'extract' => static fn (string $pdf, ?string $path): string => (new EmbeddedXmlExtractor())->extract($pdf),

    'validate' => static function (string $xml, ?string $path): array {
        $fails = array_values(array_filter(
            (new InvoiceValidator())->validate($xml)->failures(),
            static fn (RuleResult $r) => !str_starts_with($r->code, 'BR-FR-'),
        ));

        return [\count($fails), array_map(static fn (RuleResult $r) => $r->code, $fails)];
    },

    // EN16931_Einfach.cii.xml (XML Small), where the model has a place
    'hand_model' => static function (): Invoice {
        $inv = new Invoice(
            number: '471102',
            issueDate: new \DateTimeImmutable('2018-03-05'),
            typeCode: InvoiceTypeCode::CommercialInvoice,
            seller: new Party(
                name: 'Lieferant GmbH',
                address: new Address(line1: 'Lieferantenstraße 20', city: 'München', postalCode: '80333', countryCode: 'DE'),
                vatNumber: 'DE123456789',
                taxNumber: '201/113/40209',
                privateIdentifiers: [new Identifier('549910'), new Identifier('4000001123452', PartyIdentifierScheme::Gln)],
            ),
            buyer: new Party(
                name: 'Kunden AG Mitte',
                address: new Address(line1: 'Kundenstraße 15', city: 'Frankfurt', postalCode: '69876', countryCode: 'DE'),
                privateIdentifiers: [new Identifier('GE2020211')],
            ),
            paymentTermsText: 'Zahlbar innerhalb 30 Tagen netto bis 04.04.2018, 3% Skonto innerhalb 10 Tagen bis 15.03.2018',
            deliveryDate: new \DateTimeImmutable('2018-03-05'),
        );
        $inv->addLine(new InvoiceLine(lineId: '1', itemName: 'Trennblätter A4', quantity: 20.0, unitCode: UnitOfMeasureCode::Piece,
            netUnitPrice: 9.9, vatCategory: VatCategory::Standard, vatRate: 19.0, grossUnitPrice: 9.9, sellerAssignedId: 'TB100A4'));
        $inv->addLine(new InvoiceLine(lineId: '2', itemName: 'Joghurt Banane', quantity: 50.0, unitCode: UnitOfMeasureCode::Piece,
            netUnitPrice: 5.5, vatCategory: VatCategory::Standard, vatRate: 7.0, grossUnitPrice: 5.5, sellerAssignedId: 'ARNR2'));
        $inv->addNote(new Note('Rechnung gemäß Bestellung vom 01.03.2018.'));
        $inv->addNote(new Note("Lieferant GmbH\nLieferantenstraße 20\n80333 München\nDeutschland\nGeschäftsführer: Hans Muster\nHandelsregisternummer: H A 123"));

        return $inv;
    },

    'write' => static fn (Invoice $inv): string => (new FacturXGenerator())->generateXml($inv),

    'embed' => static function (string $host, string $xml): string {
        $xmp = (new XmpMetadataBuilder())->build(documentTitle: 'Facture 471102');

        return (new PdfA3Attacher())->attach(pdfBytes: $host, xmlContent: $xml, xmpPacket: $xmp);
    },
];
