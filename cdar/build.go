package cdar

import (
	"errors"
	"time"

	"github.com/ritoon/zerechnung/invoice"
	"github.com/ritoon/zerechnung/validate"
)

// ReasonSemantic is the reason of a status 213 Rejetée for an invoice that
// breaks the rules of its format or of the reform (REJ_SEMAN, list of
// BR-FR-CDV-CL-09_MDT-113_213): the reason that Rejected gives to each
// error of the validation report.
const ReasonSemantic = "REJ_SEMAN"

// Status codes of the transmission phase (MDT-88, BR-FR-CDV-CL-05) of the
// statuses that Deposited and Rejected build, as in the examples of
// France_RFE.
const (
	statusCodeDeposited = "10"
	statusCodeRejected  = "8"
)

// BuildOptions are the options of Deposited and Rejected.
type BuildOptions struct {
	// ID is the identifier of the message (MDT-4). When empty, it is made
	// of the invoice number, the status code, the time of reception, the
	// invoice type and the invoice date, as in the examples of France_RFE:
	// F202500003_200_20250701151000#380_20250701.
	ID string
	// Time is the time of creation of the message (MDT-8): time.Now, to the
	// second, when zero. Marshal writes its wall clock in its location.
	Time time.Time
	// Received is the time at which the platform received the invoice
	// (MDT-95), which is also the time of the status (MDT-78): Time when
	// zero.
	Received time.Time
	// BusinessProcess is the business process of the invoice (MDT-2):
	// ProcessRegulated when empty. A status for the PPF alone has none.
	BusinessProcess string
	// Platform is the approved platform that emits the status, the sender
	// and the issuer of the message (role WK), with its registration number
	// (scheme 0238) and its name. When it has no identifier and no name,
	// the sender and the issuer only carry the role, as in the examples of
	// France_RFE.
	Platform Party
	// ToBuyer adds the buyer to the recipients of the status, with its
	// SIREN, name and electronic address: the platform of the buyer
	// informs it of the rejection of an invoice it received.
	ToBuyer bool
	// PPF builds the version of the status that the platform sends to the
	// PPF alone (profile GuidelinePPF): the PPF is its only recipient, and
	// the business process is left out.
	PPF bool
}

// Deposited builds the status 200 Déposée of an invoice, which the platform
// of the seller emits once it accepted the invoice: a status of the
// transmission phase (305) sent to the seller and to the PPF, or to the PPF
// alone (BuildOptions.PPF).
func Deposited(inv *invoice.Invoice, opts *BuildOptions) (*Message, error) {
	if inv == nil {
		return nil, errors.New("cdar: nil invoice")
	}
	return build(inv, opts, StatusDeposited, statusCodeDeposited, nil), nil
}

// Rejected builds the status 213 Rejetée of an invoice from its validation
// report: a status of the transmission phase (305), sent to the seller and
// to the PPF, or to the PPF alone (BuildOptions.PPF), with one detail per
// error of the report, in the order of the report, whose reason is
// REJ_SEMAN (ReasonSemantic) and whose label gives the rule and its
// message. It fails when the report holds no error.
func Rejected(inv *invoice.Invoice, rep *validate.Report, opts *BuildOptions) (*Message, error) {
	if inv == nil {
		return nil, errors.New("cdar: nil invoice")
	}
	var details []Detail
	if rep != nil {
		for _, f := range rep.Findings {
			if f.Severity != validate.Error {
				continue
			}
			details = append(details, Detail{
				ReasonCode: ReasonSemantic,
				Reason:     f.Rule + " : " + f.Message,
				Sequence:   len(details) + 1,
			})
		}
	}
	if len(details) == 0 {
		return nil, errors.New("cdar: the validation report holds no error to reject the invoice for")
	}
	return build(inv, opts, StatusRejected, statusCodeRejected, details), nil
}

// build builds a status of the transmission phase of inv.
func build(inv *invoice.Invoice, opts *BuildOptions, code, statusCode string, details []Detail) *Message {
	var o BuildOptions
	if opts != nil {
		o = *opts
	}
	if o.Time.IsZero() {
		o.Time = time.Now().Truncate(time.Second)
	}
	if o.Received.IsZero() {
		o.Received = o.Time
	}
	m := &Message{
		ID:        o.ID,
		IssueTime: o.Time,
		Sender:    platform(o.Platform),
		Issuer:    platform(o.Platform),
	}
	if m.ID == "" {
		m.ID = inv.Number + "_" + code + "_" + o.Received.Format(dateTimeFormat) + "#" + inv.TypeCode + "_" + inv.IssueDate.Format102()
	}
	ppf := Party{IDs: []invoice.Identifier{{ID: PPFID, Scheme: SchemePlatform}}, Name: "PPF", Role: RolePPF}
	st := Status{
		InvoiceID:   inv.Number,
		StatusCode:  statusCode,
		InvoiceType: inv.TypeCode,
		ReceiptTime: o.Received,
		InvoiceDate: inv.IssueDate,
		Code:        code,
		Label:       Label(code),
		Seller:      &Party{IDs: legalIDs(&inv.Seller)},
		Details:     details,
	}
	if o.PPF {
		m.Guideline = GuidelinePPF
		m.Recipients = []Party{ppf}
		st.ReferenceType = GuidelinePPF
	} else {
		m.Guideline = GuidelineInvoice
		m.BusinessProcess = o.BusinessProcess
		if m.BusinessProcess == "" {
			m.BusinessProcess = ProcessRegulated
		}
		m.Recipients = append(m.Recipients, recipient(&inv.Seller, RoleSeller))
		if o.ToBuyer {
			m.Recipients = append(m.Recipients, recipient(&inv.Buyer, RoleBuyer))
		}
		m.Recipients = append(m.Recipients, ppf)
	}
	m.Acknowledgements = []Acknowledgement{{TypeCode: PhaseTransmission, IssueTime: o.Received, Invoices: []Status{st}}}
	return m
}

// platform returns the sender or issuer of a status: the platform p, with
// the role WK.
func platform(p Party) *Party {
	c := p
	c.IDs = append([]invoice.Identifier(nil), p.IDs...)
	c.Role = RolePlatform
	return &c
}

// recipient returns a party of the invoice as a recipient of its status:
// its legal identifiers, name and electronic address, with role.
func recipient(p *invoice.Party, role string) Party {
	return Party{IDs: legalIDs(p), Name: p.Name, Role: role, Address: p.ElectronicAddress}
}

// legalIDs returns the identifiers of a party of the invoice that a status
// gives: its legal registration identifier (BT-30, BT-47, the SIREN of
// scheme 0002 in France), else its identifiers of scheme 0002.
func legalIDs(p *invoice.Party) []invoice.Identifier {
	if p.LegalRegistration.ID != "" {
		return []invoice.Identifier{p.LegalRegistration}
	}
	var ids []invoice.Identifier
	for _, id := range p.IDs {
		if id.Scheme == invoice.SchemeSIREN {
			ids = append(ids, id)
		}
	}
	return ids
}
