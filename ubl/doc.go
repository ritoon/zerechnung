// Package ubl reads and writes invoices and credit notes in the OASIS UBL 2.1
// syntax, following the EN 16931 syntax binding of CEN/TS 16931-3-2. UBL is
// accepted by the French e-invoicing reform (base profile and
// EXTENDED-CTC-FR in pure UBL), by XRechnung, by Peppol BIS Billing 3.0 and by
// Peppol BIS Self-Billing 3.0.
//
// Parse turns an Invoice or CreditNote document into an invoice.Invoice, the
// same model as package cii, so that an invoice read from one syntax can be
// written in the other. Reading is tolerant: any namespace prefix is accepted
// (the namespaces are checked), values are trimmed, elements are recognized
// in any order, and a value that cannot be read (a malformed amount, date or
// indicator) is reported as an Issue while the rest of the document is still
// read. ParseTree does the same from a tree already parsed with
// internal/xmltree.
//
// Marshal writes an invoice as a UBL Invoice, or as a CreditNote when its type
// code (BT-3) is a credit note code. It writes the EN 16931 data set, in the
// order of the UBL 2.1 schema; data that the binding cannot carry (the
// EXTENDED fields of the model, extensions) is dropped, or reported as a
// *LossError when MarshalOptions.Strict is set.
//
// Two extensions of the binding are read and written when the specification
// identifier declares them: the Dutch G-account extension
// (invoice.GAccountExtension), and the Italian extension of the domestic
// invoice (invoice.ItalianExtension, EXT-IT), whose UBL extensions and
// elements carry FatturaPA data into the national blocks of the model
// (invoice.Italy). The Croatian CIUS and its extension (CIUS-HR and EXT-HR,
// invoice.URNCIUSHR) are read and written too: the UBL extension
// HRFISK20Data and the elements outside the binding that the Croatian
// specification uses go to the national blocks invoice.Croatia,
// DocumentReference.Croatia, AllowanceCharge.Croatia and Line.Croatia.
package ubl
