package cdar

import (
	"time"

	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/internal/xmltree"
	"github.com/ritoon/zerechnung/invoice"
)

// Version is the version of the CDAR syntax of the statuses, D22B, whose
// schema France_RFE publishes.
const Version = "D22B"

// Namespace is the namespace of the root element of a CDAR message,
// rsm:CrossDomainAcknowledgementAndResponse.
const Namespace = rules.NSCDAR

// Profiles of a status (MDT-3).
const (
	// GuidelineInvoice is the profile of a status of an invoice that the
	// platforms exchange, and send to the PPF along with the other
	// recipients.
	GuidelineInvoice = "urn.cpro.gouv.fr:1p0:CDV:invoice"
	// GuidelinePPF is the profile of a status sent to the PPF alone: its
	// only recipient is the PPF (identifier 0000 of scheme 0238, role DFH),
	// and its referenced invoice carries the profile as reference type
	// (MDT-97). The business process (MDT-2) is then empty, or a code of
	// three characters at most (BR-FR-CDV-CL-01).
	GuidelinePPF = "urn.cpro.gouv.fr:1p0:CDV:einvoicingF2"
)

// Business processes of a status (MDT-2, BR-FR-CDV-CL-01).
const (
	ProcessRegulated    = "REGULATED"     // invoice between taxable persons established in France, within the reform
	ProcessNonRegulated = "NON_REGULATED" // invoice outside the obligation of electronic invoicing
	ProcessB2C          = "B2C"
	ProcessB2CInt       = "B2CINT"
	ProcessB2BInt       = "B2BINT"
	ProcessOutOfScope   = "OUTOFSCOPE"
)

// Phases of a status (MDT-77, BR-FR-CDV-09).
const (
	PhaseTransmission = "305" // statuses of the transmission of the invoice: 200 Déposée, 202 Reçue, 213 Rejetée…
	PhaseProcessing   = "23"  // statuses of its processing by the buyer: 204 Prise en charge, 205 Approuvée, 210 Refusée…
)

// Roles of the parties of a status (MDT-21, MDT-40, MDT-59, MDT-132,
// MDT-158), among those of the lists of the schematron (BR-FR-CDV-CL-02 to
// BR-FR-CDV-CL-08).
const (
	RoleSeller   = "SE"
	RoleBuyer    = "BY"
	RolePlatform = "WK"  // an approved platform
	RolePPF      = "DFH" // the public invoicing portal
)

// Identifiers of the platforms and of the PPF (MDT-19, MDT-38, MDT-57).
const (
	// SchemePlatform is the scheme of the registration numbers that the
	// administration gives to the approved platforms and to the PPF.
	SchemePlatform = "0238"
	// PPFID is the identifier of the PPF in scheme SchemePlatform.
	PPFID = "0000"
)

// Status codes (MDT-105, MDT-115), among the codes of the list of the
// schematron (BR-FR-CDV-CL-06), those of the examples of France_RFE and of
// the lists of reasons of the schematron.
const (
	StatusDeposited          = "200" // Déposée
	StatusReceived           = "202" // Reçue
	StatusMadeAvailable      = "203" // Mise à disposition
	StatusTakenOver          = "204" // Prise en charge
	StatusApproved           = "205" // Approuvée
	StatusPartiallyApproved  = "206" // Approuvée partiellement
	StatusDisputed           = "207" // En litige
	StatusSuspended          = "208" // Suspendue
	StatusRefused            = "210" // Refusée
	StatusPaymentTransmitted = "211" // Paiement transmis
	StatusCollected          = "212" // Encaissée
	StatusRejected           = "213" // Rejetée
	StatusCancelled          = "220" // Annulée
	StatusRoutingError       = "221" // Erreur de routage
	StatusInadmissible       = "501" // Irrecevable
)

// Labels of the status codes of the constants above, as the examples of
// France_RFE write them (MDT-106).
var labels = map[string]string{
	StatusDeposited:          "Déposée",
	StatusReceived:           "Reçue",
	StatusMadeAvailable:      "Mise à disposition",
	StatusTakenOver:          "Prise en charge",
	StatusApproved:           "Approuvée",
	StatusPartiallyApproved:  "Approuvée partiellement",
	StatusDisputed:           "En litige",
	StatusSuspended:          "Suspendue",
	StatusRefused:            "Refusée",
	StatusPaymentTransmitted: "Paiement transmis",
	StatusCollected:          "Encaissée",
	StatusRejected:           "Rejetée",
	StatusCancelled:          "Annulée",
	StatusRoutingError:       "Erreur de routage",
	StatusInadmissible:       "Irrecevable",
}

// Label returns the label of a status code of the constants of the package
// ("Déposée" for 200), or "" for another code.
func Label(code string) string { return labels[code] }

// CharacteristicCollected is the type of the characteristic that carries an
// amount collected and its VAT rate in a status 212 Encaissée (MDT-207,
// BR-FR-CDV-14).
const CharacteristicCollected = "MEN"

// Message is a CDAR message: one status or more of invoices.
//
// The dates and times of format 204 (CCYYMMDDHHMMSS) have no time zone:
// Parse reads them in UTC, Marshal writes the wall clock of a time in its
// own location.
type Message struct {
	Test            bool   // TestIndicator: a test message
	BusinessProcess string // MDT-2 business process (cadre de facturation), see ProcessRegulated
	Guideline       string // MDT-3 profile: GuidelineInvoice or GuidelinePPF

	ID        string    // MDT-4 identifier of the message
	Name      string    // MDT-5 name of the message
	IssueTime time.Time // MDT-8 date and time of creation of the message

	Sender     *Party  // MDT-19 to MDT-21: the platform that sends the message
	Issuer     *Party  // MDT-38 to MDT-40: the party that issues the status
	Recipients []Party // MDT-57 to MDT-73: its recipients, the PPF among them

	Acknowledgements []Acknowledgement // the statuses, usually one
}

// Party is a party of a status: its sender, issuer or a recipient, or the
// seller or a recipient of a referenced invoice.
type Party struct {
	// IDs are the global identifiers of the party, with their scheme
	// (MDT-19, MDT-38, MDT-57, MDT-129): 0002 for a SIREN, 0238 for a
	// platform or the PPF (SchemePlatform).
	IDs  []invoice.Identifier
	Name string // MDT-20, MDT-39, MDT-58
	Role string // MDT-21, MDT-40, MDT-59, MDT-132, MDT-158, see RoleSeller
	// Address is the electronic address of a recipient (MDT-73), with its
	// scheme: 0225 for the directory of the reform.
	Address invoice.Identifier
}

// Acknowledgement is a status in a message: its phase, its date and the
// statuses of the invoices it references.
type Acknowledgement struct {
	MultipleReferences bool      // MDT-74: the status references several invoices
	TypeCode           string    // MDT-77 phase: PhaseTransmission or PhaseProcessing
	IssueTime          time.Time // MDT-78 date and time of deposit of the status
	Invoices           []Status  // the statuses of the referenced invoices
}

// Status is the status of a referenced invoice.
type Status struct {
	InvoiceID     string       // MDT-87 invoice number (BT-1)
	StatusCode    string       // MDT-88 status code of the phase (BR-FR-CDV-CL-05)
	InvoiceType   string       // MDT-91 invoice type code (BT-3)
	ReceiptTime   time.Time    // MDT-95 date and time of reception of the invoice
	ReferenceType string       // MDT-97 reference type: GuidelinePPF in a status sent to the PPF alone
	InvoiceDate   invoice.Date // MDT-100 invoice issue date (BT-2)
	Code          string       // MDT-105 status code, see StatusDeposited
	Label         string       // MDT-106 label of the status
	Seller        *Party       // MDT-129, MDT-132: the seller, issuer of the invoice
	Recipients    []Party      // MDT-158: recipients of the invoice
	Details       []Detail     // details of the status
}

// Detail is a detail of a status: a reason, a requested action, notes and
// characteristics.
type Detail struct {
	ReasonCode string // MDT-113 reason code (BR-FR-CDV-CL-09)
	Reason     string // MDT-114 label of the reason
	Code       string // MDT-115 status code of the detail
	Label      string // label of the status of the detail
	ActionCode string // MDT-121 requested action code (BR-FR-CDV-CL-10)
	Action     string // MDT-122 requested action
	// Sequence is the incremental number of the detail (MDT-124-2), from
	// 1; 0 is an absent number.
	Sequence        int
	Notes           []Note
	Characteristics []Characteristic
}

// Note is a note of a detail.
type Note struct {
	Content string // MDT-126
	Subject string // MDT-127 subject code (UNTDID 4451)
}

// Characteristic is a data of the invoice that a detail gives: an amount
// collected, or a value in dispute and the expected one.
type Characteristic struct {
	ID       string          // MDT-206 identifier of the data: a business term (BT-152)
	TypeCode string          // MDT-207 type of the data (BR-FR-CDV-CL-11), see CharacteristicCollected
	Changed  bool            // MDT-209 the value is changed
	Name     string          // MDT-211 name of the data
	Location string          // MDT-213 location of the data in the XML of the invoice
	Amount   invoice.Decimal // MDT-215 amount, the collected amount of a status 212
	Currency string          // currency of the amount
	Date     invoice.Date    // MDT-219 date (format 102)
	Percent  invoice.Decimal // MDT-224 percentage, the VAT rate of a collected amount
}

// IsCDAR reports whether root is the root element of a CDAR message:
// rsm:CrossDomainAcknowledgementAndResponse in namespace Namespace.
func IsCDAR(root *xmltree.Node) bool { return rules.IsCDAR(root) }
