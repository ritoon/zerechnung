// Package esig checks the certificate of an electronic signature or seal:
// what it declares (qualified certificate, signature or seal, key in a
// qualified device: ETSI EN 319 412-5), its validity at a given time, its
// chain to the roots the caller trusts, and its revocation status from the
// CRLs and OCSP responses the caller gives. Nothing is fetched from the
// network.
package esig

import (
	"crypto/x509"
	"encoding/asn1"
)

// Object identifiers of ETSI EN 319 412-5 (QCStatements) and of the
// certificate policies of ETSI EN 319 411-2.
var (
	oidQCStatements  = asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 1, 3}
	oidQcCompliance  = asn1.ObjectIdentifier{0, 4, 0, 1862, 1, 1}
	oidQcSSCD        = asn1.ObjectIdentifier{0, 4, 0, 1862, 1, 4}
	oidQcType        = asn1.ObjectIdentifier{0, 4, 0, 1862, 1, 6}
	oidQcTypeESign   = asn1.ObjectIdentifier{0, 4, 0, 1862, 1, 6, 1}
	oidQcTypeESeal   = asn1.ObjectIdentifier{0, 4, 0, 1862, 1, 6, 2}
	oidQcTypeWeb     = asn1.ObjectIdentifier{0, 4, 0, 1862, 1, 6, 3}
	oidQcCCLegislate = asn1.ObjectIdentifier{0, 4, 0, 1862, 1, 7}

	oidPolicyQCPn     = asn1.ObjectIdentifier{0, 4, 0, 194112, 1, 0}
	oidPolicyQCPl     = asn1.ObjectIdentifier{0, 4, 0, 194112, 1, 1}
	oidPolicyQCPnQSCD = asn1.ObjectIdentifier{0, 4, 0, 194112, 1, 2}
	oidPolicyQCPlQSCD = asn1.ObjectIdentifier{0, 4, 0, 194112, 1, 3}
)

// Type is the purpose a certificate is issued for.
type Type int

// Certificate purposes (QcType of ETSI EN 319 412-5 §4.2.3).
const (
	TypeUnknown   Type = iota // not declared
	TypeSignature             // electronic signature of a natural person (esign)
	TypeSeal                  // electronic seal of a legal person (eseal)
	TypeWeb                   // website authentication (web)
)

func (t Type) String() string {
	switch t {
	case TypeSignature:
		return "signature"
	case TypeSeal:
		return "seal"
	case TypeWeb:
		return "web"
	}
	return "unknown"
}

// QC is what a certificate declares in its QCStatements extension (ETSI
// EN 319 412-5) and in its certificate policies (ETSI EN 319 411-2).
type QC struct {
	// Qualified: an EU qualified certificate (QcCompliance without
	// QcCClegislation, or a QCP-n / QCP-l policy).
	Qualified bool
	// QSCD: the private key resides in a qualified signature or seal
	// creation device (QcSSCD, or a QCP-n-qscd / QCP-l-qscd policy).
	QSCD bool
	// Type: signature, seal or web. A qualified certificate without QcType
	// is a certificate for electronic signatures (EN 319 412-5, table 1).
	Type Type
}

type qcStatement struct {
	ID   asn1.ObjectIdentifier
	Info asn1.RawValue `asn1:"optional"`
}

// ParseQC reads what cert declares. A malformed extension is ignored: the
// certificate then declares nothing.
func ParseQC(cert *x509.Certificate) QC {
	var q QC
	compliance, legislation := false, false
	for _, ext := range cert.Extensions {
		if !ext.Id.Equal(oidQCStatements) {
			continue
		}
		var stmts []qcStatement
		if rest, err := asn1.Unmarshal(ext.Value, &stmts); err != nil || len(rest) != 0 {
			return QC{}
		}
		for _, s := range stmts {
			switch {
			case s.ID.Equal(oidQcCompliance):
				compliance = true
			case s.ID.Equal(oidQcSSCD):
				q.QSCD = true
			case s.ID.Equal(oidQcCCLegislate):
				legislation = true
			case s.ID.Equal(oidQcType):
				var types []asn1.ObjectIdentifier
				if _, err := asn1.Unmarshal(s.Info.FullBytes, &types); err == nil && len(types) == 1 {
					switch {
					case types[0].Equal(oidQcTypeESign):
						q.Type = TypeSignature
					case types[0].Equal(oidQcTypeESeal):
						q.Type = TypeSeal
					case types[0].Equal(oidQcTypeWeb):
						q.Type = TypeWeb
					}
				}
			}
		}
	}
	q.Qualified = compliance && !legislation
	if compliance && q.Type == TypeUnknown {
		q.Type = TypeSignature
	}
	for _, p := range cert.Policies {
		switch {
		case p.EqualASN1OID(oidPolicyQCPnQSCD):
			q.QSCD = true
			fallthrough
		case p.EqualASN1OID(oidPolicyQCPn):
			q.Qualified = true
			if q.Type == TypeUnknown {
				q.Type = TypeSignature
			}
		case p.EqualASN1OID(oidPolicyQCPlQSCD):
			q.QSCD = true
			fallthrough
		case p.EqualASN1OID(oidPolicyQCPl):
			q.Qualified = true
			if q.Type == TypeUnknown {
				q.Type = TypeSeal
			}
		}
	}
	return q
}
