// Package facturae reads and writes invoices in Facturae, the Spanish
// format (versions 3.2, 3.2.1 and 3.2.2 of its XML schema): the format of the
// invoices sent to the Spanish public administrations through FACe
// (Ley 25/2013), and one of the four syntaxes, with CII, UBL and EDIFACT,
// that article 7.1 of Real Decreto 238/2026 admits for the mandatory
// electronic invoices between businesses (Ley 18/2022 «Crea y Crece»).
//
// Parse turns a Facturae document into an invoice.Invoice, the model of
// packages cii and ubl, so that an invoice read from one syntax can be
// written in the others. No official mapping of Facturae to EN 16931 is
// published: the data that EN 16931 carries goes to the EN 16931 fields as
// the list below maps it, and the rest to the national blocks of the model
// (see invoice.Spain), so that cii.Marshal or ubl.Marshal write the invoice
// read, reporting the national data as lost in strict mode. Reading is
// tolerant: any namespace prefix is accepted (the namespace of the root
// element is checked and gives the version), values are trimmed, elements
// are recognized in any order, a value that cannot be read is reported as
// an Issue while the rest of the document is still read, and the elements
// that the format does not define are kept as extensions (see
// invoice.Extension). A document may hold several invoices (a batch):
// ParseAll reads them all, each with the file header and the parties.
//
// Marshal writes an invoice as a Facturae 3.2.2 document, in the order of
// the schema, from its national blocks and its EN 16931 fields: each
// Facturae value that the national blocks do not hold is derived from the
// EN 16931 fields. The EN 16931 data that Facturae cannot carry is dropped,
// or reported by a *LossError when MarshalOptions.Strict is set.
//
// # Mapping
//
// The main correspondences between EN 16931 and Facturae, by the local
// names of the elements:
//
//   - BT-1 Invoice number: InvoiceSeriesCode followed by InvoiceNumber. The
//     series is kept in the national block, so that Marshal splits the
//     number again.
//   - BT-2 Issue date: IssueDate. BT-7 Value added tax point date:
//     OperationDate.
//   - BT-3 Invoice type code: InvoiceClass and InvoiceDocumentType. A
//     corrective invoice (OR) reads as 384, a self-billed invoice (AF) as
//     389, a corrective self-billed invoice as 471, any other as 380. A credit note (381 and the other credit note
//     codes) is written as a corrective invoice with its amounts negated;
//     when the national block gives no Corrective data, the reason is 16
//     (taxable base), the method 02 (differences) for a negative total and
//     01 (full correction) otherwise, and the tax period is the invoicing
//     period, else the month of the issue date of the corrected invoice,
//     else that of the issue date.
//   - BG-3 Preceding invoice reference: InvoiceNumber, InvoiceSeriesCode
//     and InvoiceIssueDate of Corrective.
//   - BT-5, BT-6 Currencies: InvoiceCurrencyCode and TaxCurrencyCode
//     (written as the invoice currency when BT-6 is empty). The amounts in
//     euros of an invoice in another currency are EquivalentInEuros, and
//     the exchange rate ExchangeRateDetails; BT-111 is the sum of the VAT in
//     euros.
//   - BG-1 Invoice notes: InvoiceDescription (a note without subject code),
//     LegalReference (subject ABL) and InvoiceAdditionalInformation
//     (subject AAI).
//   - BT-12 Contract reference: ReceiverContractReference. BT-13 Purchase
//     order reference: ReceiverTransactionReference. BT-17 Tender or lot
//     reference: FileReference. These elements of InvoiceIssueData exist
//     since version 3.2.2.
//   - BG-4, BG-7 Seller and buyer: SellerParty and BuyerParty. The
//     TaxIdentificationNumber is the VAT identifier (BT-31, BT-48) without
//     its ES prefix, with its country prefix for an intra-Community
//     transaction only, else the legal registration identifier (BT-30,
//     BT-47), else the tax registration identifier (BT-32). PersonTypeCode
//     is F (Individual) when the national block gives the names of a
//     person, J (LegalEntity) otherwise; ResidenceTypeCode is R for Spain,
//     U for another member state, E otherwise. The name (BT-27, BT-44) is
//     the CorporateName of a
//     LegalEntity or the names of an Individual, the trading name (BT-28,
//     BT-45) TradeName, the address AddressInSpain or OverseasAddress, the
//     contact (BG-6, BG-9) ContactDetails. The administrative centres (the
//     DIR3 codes that FACe requires) stay in the national block.
//   - BG-14 Invoicing period: InvoicingPeriod.
//   - BG-16 Payment instructions: one Installment per payment means. BT-9
//     Payment due date: InstallmentDueDate of the first installment (the
//     issue date when the invoice gives none, the element being required).
//     BT-81 Payment means type code: PaymentMeans, through a table of
//     correspondence (04 transfer and 30, 02 direct debit and 49…). BT-83
//     Remittance information: PaymentReconciliationReference. BT-84, BT-86
//     IBAN and BIC: AccountToBeCredited (an eight character BIC is written
//     with XXX, as Facturae wants eleven).
//   - BG-20, BG-21 Document allowances and charges: GeneralDiscounts and
//     GeneralSurcharges.
//   - BG-22 Document totals: InvoiceTotals. BT-106 TotalGrossAmount, BT-107
//     TotalGeneralDiscounts, BT-108 TotalGeneralSurcharges, BT-109
//     TotalGrossAmountBeforeTaxes, BT-113 TotalPaymentsOnAccount; BT-110
//     is the sum of the VAT breakdown, BT-112 and BT-115 are computed as
//     EN 16931 defines them. InvoiceTotal, TotalOutstandingAmount and
//     TotalExecutableAmount, which deduct the taxes withheld, are derived on
//     writing and kept in the national block only when they differ.
//   - BG-23 VAT breakdown: TaxesOutputs, tax type 01 (IVA) for the VAT
//     categories S, Z, E, AE, K, G and O, 03 (IGIC) for L and 02 (IPSI) for
//     M. The categories that Facturae does not name are read from the rate
//     and the SpecialTaxableEvent of the lines: 01 (exempt) gives E, 02
//     (not subject) O, and a VATEX code at the start of the reason K, G or
//     AE. BT-120 Exemption reason: SpecialTaxableEventReason.
//   - BG-24 Additional supporting documents: an attachment goes to
//     RelatedDocuments of AdditionalData.
//   - BG-25 Invoice line: InvoiceLine. BT-126 SequenceNumber, BT-129
//     Quantity, BT-130 UnitOfMeasure (through a table of correspondence to
//     the UN/ECE recommendation 20 codes), BT-146 UnitPriceWithoutTax,
//     BT-131 GrossAmount, BT-153 ItemDescription, BT-155 ArticleCode,
//     BT-127 AdditionalLineItemInformation, BG-26 LineItemPeriod, BG-27 and
//     BG-28 DiscountsAndRebates and Charges, BT-151 and BT-152 the tax of
//     TaxesOutputs.
//
// The taxes withheld (IRPF), the equivalence surcharge, the subsidies, the
// reimbursable expenses, the third party issuer and the registration data
// of a company have no EN 16931 term: they stay in the national blocks.
// EN 16931 deducts no tax withheld from the amount due (BT-115), unlike
// the executable amount of Facturae.
//
// # Limits
//
// Marshal writes a document even when a value that the schema requires is
// missing from the invoice (the tax identifier of a party, for example):
// validate.XML checks a Facturae document against the schema of its
// version and reports it. The package neither creates nor verifies the
// XAdES signature that Facturae requires (a signature read is not kept),
// and it does not handle EDIFACT, the transmission of invoices (FACe, the
// private platforms, the public solution of the AEAT), the invoice states
// or VERI*FACTU.
//
// The schemas compiled into the module come from facturae.gob.es. Origen de
// los datos: Ministerio de Hacienda y Administraciones Públicas - Ministerio
// de Industria, Energía y Turismo. The ministries do not take part in,
// sponsor or support this package. No official Facturae 3.2.x example is
// published: the tests use invoices written by hand.
package facturae
