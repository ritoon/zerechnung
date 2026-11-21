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
// Package validate checks a FatturaPA document against the FatturaPA schema
// 1.2.4 (validate.XML); the documents that Marshal writes are valid against
// it. Neither package applies the checks of the SDI on the content of a
// document.
//
// # Signatures
//
// The SDI accepts a file signed in CAdES (a .xml.p7m envelope) or in XAdES
// (an enveloped ds:Signature, the last child of FatturaElettronica), and
// requires a signature for an invoice to a public administration (FPA12)
// (technical specifications 1.8.4, §2.1). SignCAdES and SignXAdES sign a
// document in CAdES or XAdES Baseline B with a crypto.Signer, so that the
// key may stay in a smart card, an HSM or a remote signing service, and
// FileName names the result (§2.2). Unwrap opens a signed file, a .p7m in
// DER, BER or base64 included, checks every signature against the document
// and returns the document; Parse reads a document signed in XAdES as an
// unsigned one, without keeping the signature in the invoice read.
// VerifySignature checks a signed file as the SDI does on reception
// (§5.1.1) and reports each problem under the error code of the SDI: the
// trust in the certificate and its revocation are checked against the
// roots, CRLs and OCSP responses the caller gives, since the package does
// not access the network. CMS, BER, XML canonicalization and XML signatures
// are written in Go, without dependencies.
package fatturapa
