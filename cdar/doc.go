// Package cdar reads, writes and builds the statuses of the life cycle of
// the French invoices (cycle de vie, CDV) of the 2026 reform: the messages
// that the approved platforms (plateformes agréées) exchange with each other
// and send to the public invoicing portal (PPF), in the UN/CEFACT Cross
// Domain Acknowledgement and Response (CDAR) syntax, version D22B, as the
// external specifications of the reform and the France_RFE package of the
// FNFE-MPE (version 1.4.0.04) define them.
//
// A Message holds the context of the status (MDT-2 business process, MDT-3
// profile), its identification and date (MDT-4, MDT-5, MDT-8), its sender,
// issuer and recipients, and one Acknowledgement or more: the phase of the
// status (MDT-77: 305 transmission, 23 processing), its date (MDT-78) and
// the Status of each referenced invoice (MDT-87 number, MDT-91 type, MDT-100
// date, MDT-105 status code, MDT-129 seller), with its Details: the reason
// (MDT-113, MDT-114), the requested action (MDT-121, MDT-122), the notes and
// the Characteristics, among which the collected amounts and their VAT rate
// that the status 212 Encaissée carries (MDT-207 MEN, MDT-215, MDT-224).
// The fields name the data of the specifications (MDT-*), as the examples
// of France_RFE comment them.
//
// Parse reads a CDAR message, as tolerantly as the readers of invoices of
// this module: any namespace prefix is accepted (the namespace of the root
// element is checked), values are trimmed, elements are recognized in any
// order, and a value that cannot be read, or an element that the model does
// not hold, is reported as an Issue while the rest of the message is still
// read. Marshal writes a message in the order of the CDAR D22B schema;
// Parse reads back what Marshal writes.
//
// Deposited and Rejected build, from an invoice, the first status of its
// life cycle that the platform of the seller emits: 200 Déposée when the
// invoice is accepted, 213 Rejetée with the error findings of its
// validation report (package validate) as reasons. BuildOptions.PPF builds
// the version of the status that a platform sends to the PPF alone (profile
// urn.cpro.gouv.fr:1p0:CDV:einvoicingF2).
//
// Package validate recognizes a CDAR message (validate.XML, validate.Tree):
// it checks it against the CDAR D22B schema of France_RFE and the rules
// BR-FR-CDV of the schematron of the FNFE-MPE. The messages that Deposited
// and Rejected build, once written by Marshal, pass both.
//
// The transmission of the statuses (the network of the platforms, the
// directory, the flows to the PPF) and the platform mode, in which a
// platform applies the life cycle as a whole, are outside the package.
package cdar
