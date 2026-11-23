// Package ereporting reads and writes the reports of the French e-reporting
// (flux 10): the data that a company subject to VAT in France transmits to
// the tax administration, through its approved platform (plateforme agréée,
// PA), for the transactions that are not invoiced electronically in France,
// international business-to-business sales and purchases, invoice by
// invoice (flux 10.1), and the daily totals of the other transactions, such
// as the sales to consumers (flux 10.3); and for the payments received for
// the services whose VAT is due on payment (flux 10.2 and 10.4).
//
// A report follows the XML schemas of the flux 10 published by the FNFE-MPE
// in its France_RFE package 1.4.0.04 (Flux10/xsd): a root element Report
// (TB-0), in no namespace, holding the header of the transmission
// (ReportDocument, TB-1) and a report of the transactions
// (TransactionsReport, TB-2) or of the payments (PaymentsReport, TB-3).
// The model, Report, follows the schemas field by field, with the business
// terms (TT-*, TG-*) in the comments of its fields: Marshal writes all of
// it, and Parse reads all that the schemas define.
//
// Parse reads a report: the dates in the format AAAAMMJJ, the time of the
// transmission in the format AAAAMMJJHHMMSS, the amounts, rates and
// quantities as decimals that keep their decimals. A value that cannot be
// read, or an element or attribute that the schemas do not define, is
// reported as an Issue while the rest of the report is still read.
// Marshal writes a report in the order of the schemas.
//
// validate.XML recognizes a report and checks it against the schemas of the
// flux 10, compiled into the package, and for the values that Parse cannot
// read. Neither validate nor this package checks the rules of the PPF on
// the content of a report (the schematron Flux10 of France_RFE, drawn from
// annex 7 of the external specifications of the DGFiP: formats of the
// identifiers and dates, code lists, totals), nor computes a report from
// invoices: the schematrons of correspondence (BR-FR-MAP) that define the
// data of the flux 10.1 drawn from an invoice are not published yet.
// Transmitting a report is the job of the approved platform.
package ereporting
