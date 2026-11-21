package invoice

// amountScale is the number of decimals of computed amounts (EN 16931 allows
// at most two).
const amountScale = 2

var hundred = DecimalFromInt(100)

// CountsInTotals reports whether the line takes part in the document totals.
// In EXTENDED invoices, lines whose status reason code is GROUP (a group of
// sub-lines) or INFORMATION (an information-only line) do not: only plain
// lines and DETAIL sub-lines are summed.
func (l *Line) CountsInTotals() bool {
	return l.StatusReasonCode == "" || l.StatusReasonCode == "DETAIL"
}

// Calculate fills in the amounts that EN 16931 derives from other data.
//
// For each line it computes, when they are absent: the allowance and charge
// amounts from their percentage and base (BT-136, BT-141), the net price from
// the gross price and discount (BT-146 = BT-148 − BT-147), and the line net
// amount BT-131 = BT-129 × BT-146 ÷ BT-149 + charges − allowances, rounded to
// two decimals. Document level allowance and charge amounts are computed from
// their percentage in the same way.
//
// It then recomputes, overwriting previous values: the totals BT-106 to
// BT-110, BT-112 and BT-115 (which adds the charges collected on behalf of
// a third party, BG-34, as BR-FXEXT-CO-16 of EXTENDED requires), and the VAT
// breakdown, one entry per VAT category and rate found on lines, document
// level allowances and charges and logistics charges. Logistics charges
// (EXT) count as document level charges in BT-108, as BR-FXEXT-CO-12
// requires. Tax amounts are rounded half away from zero to two decimals.
// Exemption reasons and extensions already present in the breakdown are
// kept.
//
// Invoices without lines (MINIMUM, BASIC WL) keep the VAT breakdown given by
// the caller: only missing tax amounts are computed, and BT-109 is the sum of
// the taxable amounts when BT-106 is absent.
//
// BT-111 (VAT total in accounting currency) needs an exchange rate and is left
// unchanged.
func (inv *Invoice) Calculate() {
	for i := range inv.Lines {
		inv.Lines[i].calculate()
	}
	for i := range inv.AllowancesCharges {
		inv.AllowancesCharges[i].calculate()
	}

	t := &inv.Totals
	var allowances, charges Decimal
	var nAllowances, nCharges int
	for _, ac := range inv.AllowancesCharges {
		if ac.Charge {
			charges = charges.Add(ac.Amount)
			nCharges++
		} else {
			allowances = allowances.Add(ac.Amount)
			nAllowances++
		}
	}
	for _, lc := range inv.LogisticsCharges {
		charges = charges.Add(lc.Amount)
		nCharges++
	}
	t.AllowanceTotal, t.ChargeTotal = Decimal{}, Decimal{}
	if nAllowances > 0 {
		t.AllowanceTotal = allowances.Round(amountScale)
	}
	if nCharges > 0 {
		t.ChargeTotal = charges.Round(amountScale)
	}

	if len(inv.Lines) > 0 {
		lineTotal := NewDecimal(0, amountScale)
		for i := range inv.Lines {
			if inv.Lines[i].CountsInTotals() {
				lineTotal = lineTotal.Add(inv.Lines[i].NetAmount)
			}
		}
		t.LineTotal = lineTotal.Round(amountScale)
		inv.VATBreakdown = inv.computeBreakdown()
	} else {
		for i := range inv.VATBreakdown {
			b := &inv.VATBreakdown[i]
			if !b.TaxAmount.IsSet() {
				b.TaxAmount = taxAmount(b.CategoryCode, b.TaxableAmount, b.Rate)
			}
		}
	}

	if t.LineTotal.IsSet() {
		t.TaxBasisTotal = t.LineTotal.Sub(t.AllowanceTotal).Add(t.ChargeTotal).Round(amountScale)
	} else {
		basis := NewDecimal(0, amountScale)
		for _, b := range inv.VATBreakdown {
			basis = basis.Add(b.TaxableAmount)
		}
		t.TaxBasisTotal = basis.Round(amountScale)
	}
	tax := NewDecimal(0, amountScale)
	for _, b := range inv.VATBreakdown {
		tax = tax.Add(b.TaxAmount)
	}
	t.TaxTotal = tax.Round(amountScale)
	t.GrandTotal = t.TaxBasisTotal.Add(t.TaxTotal).Round(amountScale)
	t.DuePayable = t.GrandTotal.Sub(t.PaidAmount).Add(t.RoundingAmount)
	for _, c := range inv.ThirdPartyCharges {
		t.DuePayable = t.DuePayable.Add(c.Amount)
	}
	t.DuePayable = t.DuePayable.Round(amountScale)
}

func (l *Line) calculate() {
	if !l.NetPrice.IsSet() && l.GrossPrice.IsSet() {
		l.NetPrice = l.GrossPrice.Sub(l.PriceDiscount)
	}
	for i := range l.AllowancesCharges {
		l.AllowancesCharges[i].calculate()
	}
	if l.NetAmount.IsSet() || !l.Quantity.IsSet() || !l.NetPrice.IsSet() {
		return
	}
	amount := l.Quantity.Mul(l.NetPrice)
	if l.PriceBaseQuantity.IsSet() && !l.PriceBaseQuantity.IsZero() {
		amount = amount.Div(l.PriceBaseQuantity, 6)
	}
	for _, ac := range l.AllowancesCharges {
		if ac.Charge {
			amount = amount.Add(ac.Amount)
		} else {
			amount = amount.Sub(ac.Amount)
		}
	}
	l.NetAmount = amount.Round(amountScale)
}

func (ac *AllowanceCharge) calculate() {
	if ac.Amount.IsSet() || !ac.Percentage.IsSet() || !ac.BaseAmount.IsSet() {
		return
	}
	ac.Amount = ac.BaseAmount.Mul(ac.Percentage).Div(hundred, amountScale)
}

// computeBreakdown groups line net amounts, document level allowances and
// charges and logistics charges by VAT category and rate.
func (inv *Invoice) computeBreakdown() []VATBreakdown {
	var out []VATBreakdown
	group := func(cat string, rate Decimal) *VATBreakdown {
		for i := range out {
			if out[i].CategoryCode == cat && out[i].Rate.IsSet() == rate.IsSet() && out[i].Rate.Equal(rate) {
				return &out[i]
			}
		}
		b := VATBreakdown{CategoryCode: cat, Rate: rate, TaxableAmount: NewDecimal(0, amountScale)}
		for _, old := range inv.VATBreakdown {
			if old.CategoryCode == cat && old.Rate.IsSet() == rate.IsSet() && old.Rate.Equal(rate) {
				b.ExemptionReason = old.ExemptionReason
				b.ExemptionReasonCode = old.ExemptionReasonCode
				b.Extensions = old.Extensions
				break
			}
		}
		out = append(out, b)
		return &out[len(out)-1]
	}
	for i := range inv.Lines {
		l := &inv.Lines[i]
		if !l.CountsInTotals() || l.VATCategoryCode == "" {
			continue
		}
		g := group(l.VATCategoryCode, l.VATRate)
		g.TaxableAmount = g.TaxableAmount.Add(l.NetAmount)
	}
	for _, ac := range inv.AllowancesCharges {
		if ac.VATCategoryCode == "" {
			continue
		}
		g := group(ac.VATCategoryCode, ac.VATRate)
		if ac.Charge {
			g.TaxableAmount = g.TaxableAmount.Add(ac.Amount)
		} else {
			g.TaxableAmount = g.TaxableAmount.Sub(ac.Amount)
		}
	}
	for _, lc := range inv.LogisticsCharges {
		if lc.VATCategoryCode == "" {
			continue
		}
		g := group(lc.VATCategoryCode, lc.VATRate)
		g.TaxableAmount = g.TaxableAmount.Add(lc.Amount)
	}
	for i := range out {
		out[i].TaxableAmount = out[i].TaxableAmount.Round(amountScale)
		out[i].TaxAmount = taxAmount(out[i].CategoryCode, out[i].TaxableAmount, out[i].Rate)
	}
	return out
}

// taxAmount returns base × rate ÷ 100 rounded to two decimals, or zero for
// categories that carry no VAT. The split payment (B) carries VAT that the
// buyer pays to the treasury instead of the seller, but the invoice states
// it and counts it in the totals like the standard rate: EN 16931 has no
// other rule for it than BR-CO-17, BR-CO-15 and BR-CO-16.
func taxAmount(category string, base, rate Decimal) Decimal {
	switch category {
	case VATStandard, VATCanaryIslands, VATCeutaAndMelilla, VATSplitPayment:
		return base.Mul(rate).Div(hundred, amountScale)
	}
	return NewDecimal(0, amountScale)
}
