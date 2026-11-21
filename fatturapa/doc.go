// Package fatturapa reads and writes invoices in the Italian FatturaPA
// format (FatturaElettronica 1.2: FPA12 for public administrations, FPR12
// between private parties), the format that the Sistema di Interscambio
// (SDI) exchanges, as specified by the technical specifications 1.9.1
// (schema 1.2.3) and 1.10 (schema 1.2.4, from 11 January 2027).
//
// Parse turns a FatturaPA document into an invoice.Invoice, the model of
// packages cii and ubl, so that an invoice read from one syntax can be
// written in the others. The data that EN 16931 carries goes to the
// EN 16931 fields, as appendix 4 of the Italian technical rules (Regole
// tecniche, art. 3 d.lgs. 148/2018) maps it with the conventions of the
// CIUS-IT, and the rest to the national blocks of the model (see
// invoice.Italy): the invoice read has the CIUS-IT profile, and cii.Marshal
// or ubl.Marshal write it as a CIUS-IT invoice, reporting the national data
// as lost in strict mode. Reading is tolerant: any namespace prefix is
// accepted (the namespace of the root element is checked), values are
// trimmed, elements are recognized in any order, a value that cannot be
// read is reported as an Issue while the rest of the document is still read,
// and the elements that the format does not define are kept as extensions
// (see invoice.Extension). A document may hold several invoices, one per
// FatturaElettronicaBody (a lotto): ParseAll reads them all.
//
// Marshal writes an invoice as a FatturaPA document, in the order of the
// FatturaPA schema, from its national blocks and its EN 16931 fields: each
// FatturaPA value that the national blocks do not hold is derived from the
// EN 16931 fields, as appendix 4 maps them. The EN 16931 data that
// FatturaPA cannot carry is dropped, or reported by a *LossError when
// MarshalOptions.Strict is set.
//
// The package neither validates a document against the FatturaPA schema nor
// applies the checks of the SDI; it does not read signed documents (.p7m
// envelopes), and the XAdES signature of a document is not kept.
package fatturapa
