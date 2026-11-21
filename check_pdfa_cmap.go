package zerechnung

import (
	"bytes"
)

// This file reads how the CMap of a Type 0 font splits the strings of the
// text into codes and maps them to CIDs (ISO 32000-1, 9.7.5 and 9.7.6.2),
// for the glyph checks of check_pdfa_glyphs.go.

// codeRange is a codespace range of a CMap: the codes of n bytes whose
// bytes lie, one by one, between those of lo and hi.
type codeRange struct {
	lo, hi [4]byte
	n      int
}

// cidRange maps the codes lo to hi of n bytes to CIDs from cid.
type cidRange struct {
	lo, hi uint32
	n      int
	cid    uint32
}

// cidMap is what the glyph checks read of a CMap.
type cidMap struct {
	identity bool // Identity-H or Identity-V: codes of 2 bytes, each its own CID
	space    []codeRange
	cids     []cidRange // cidchar mappings are ranges of one code
	notdef   []cidRange
}

// identityCMap is the CMap Identity-H (or Identity-V).
var identityCMap = &cidMap{identity: true}

// next splits the first code of s as veraPDF reads it
// (CMap.getCodeFromStream), and returns its value and length: byte after
// byte, up to 4, until the bytes read lie in a codespace range of their
// length, a byte past the end of s reading as 0xFF. When no range starts
// with the bytes read, the code is 0, as long as the shortest range.
func (m *cidMap) next(s []byte) (code uint32, size int) {
	var b [4]byte
	for n := 1; n <= 4; n++ {
		b[n-1] = 0xff
		if n-1 < len(s) {
			b[n-1] = s[n-1]
		}
		if m.identity {
			if n == 2 {
				return codeValue(b[:2]), 2
			}
			continue
		}
		partial := false
		for _, r := range m.space {
			switch {
			case r.n == n && r.contains(b[:n]):
				return codeValue(b[:n]), n
			case r.n > n && r.contains(b[:n]):
				partial = true
			}
		}
		if !partial {
			return 0, max(n, m.shortest())
		}
	}
	return 0, 4
}

// shortest returns the length of the shortest codespace range.
func (m *cidMap) shortest() int {
	n := 4
	for _, r := range m.space {
		n = min(n, r.n)
	}
	return n
}

func (r codeRange) contains(b []byte) bool {
	for i, c := range b {
		if c < r.lo[i] || c > r.hi[i] {
			return false
		}
	}
	return true
}

func codeValue(b []byte) uint32 {
	var v uint32
	for _, c := range b {
		v = v<<8 | uint32(c)
	}
	return v
}

// cid returns the CID a code maps to, and whether a cidrange or cidchar
// mapping maps it; a code of a notdefrange maps to its CID, unmapped.
func (m *cidMap) cid(code uint32) (uint32, bool) {
	if m.identity {
		return code, true
	}
	for _, r := range m.cids {
		if code >= r.lo && code <= r.hi {
			return r.cid + code - r.lo, true
		}
	}
	for _, r := range m.notdef {
		if code >= r.lo && code <= r.hi {
			return r.cid, false
		}
	}
	return 0, false
}

// parseCMap reads the codespace ranges and the CID mappings of an embedded
// CMap program.
func parseCMap(data []byte) *cidMap {
	m := &cidMap{}
	for _, sec := range []struct {
		begin, end string
		tokens     int
	}{
		{"begincodespacerange", "endcodespacerange", 2},
		{"begincidrange", "endcidrange", 3},
		{"begincidchar", "endcidchar", 2},
		{"beginnotdefrange", "endnotdefrange", 3},
	} {
		rest := data
		for {
			i := bytes.Index(rest, []byte(sec.begin))
			if i < 0 {
				break
			}
			rest = rest[i+len(sec.begin):]
			j := bytes.Index(rest, []byte(sec.end))
			if j < 0 {
				j = len(rest)
			}
			toks := cmapCodes(rest[:j])
			rest = rest[j:]
			for k := 0; k+sec.tokens <= len(toks); k += sec.tokens {
				t := toks[k : k+sec.tokens]
				switch sec.begin {
				case "begincodespacerange":
					if t[0].n >= 1 && t[0].n <= 4 && t[0].n == t[1].n {
						r := codeRange{n: t[0].n}
						copy(r.lo[:], t[0].b)
						copy(r.hi[:], t[1].b)
						m.space = append(m.space, r)
					}
				case "begincidrange":
					m.cids = append(m.cids, cidRange{lo: t[0].v, hi: t[1].v, n: t[0].n, cid: t[2].v})
				case "begincidchar":
					m.cids = append(m.cids, cidRange{lo: t[0].v, hi: t[0].v, n: t[0].n, cid: t[1].v})
				case "beginnotdefrange":
					m.notdef = append(m.notdef, cidRange{lo: t[0].v, hi: t[1].v, n: t[0].n, cid: t[2].v})
				}
			}
		}
	}
	return m
}

// maxCID returns the largest CID the CMap maps a code to.
func (m *cidMap) maxCID() uint32 {
	var c uint32
	for _, r := range m.cids {
		if r.hi > r.lo {
			c = max(c, r.cid+r.hi-r.lo)
		} else {
			c = max(c, r.cid)
		}
	}
	for _, r := range m.notdef {
		c = max(c, r.cid)
	}
	return c
}

// cmapCode is a hexadecimal string (its bytes) or an integer of a CMap.
type cmapCode struct {
	b []byte
	n int // bytes of a string, 0 for an integer
	v uint32
}

// cmapCodes returns the hexadecimal strings and integers of a section of
// a CMap program.
func cmapCodes(b []byte) []cmapCode {
	var toks []cmapCode
	for i := 0; i < len(b); {
		switch c := b[i]; {
		case c == '<':
			j := bytes.IndexByte(b[i:], '>')
			if j < 0 {
				return toks
			}
			var digits []byte
			for _, h := range b[i+1 : i+j] {
				if unhex(h) >= 0 {
					digits = append(digits, h)
				}
			}
			if len(digits)%2 != 0 {
				digits = append(digits, '0')
			}
			t := cmapCode{n: len(digits) / 2}
			for k := 0; k+1 < len(digits); k += 2 {
				v := byte(unhex(digits[k])<<4 | unhex(digits[k+1]))
				t.b = append(t.b, v)
				t.v = t.v<<8 | uint32(v)
			}
			toks = append(toks, t)
			i += j + 1
		case c >= '0' && c <= '9':
			var v uint32
			for i < len(b) && b[i] >= '0' && b[i] <= '9' {
				if v < 1<<28 {
					v = v*10 + uint32(b[i]-'0')
				}
				i++
			}
			toks = append(toks, cmapCode{v: v})
		default:
			i++
		}
	}
	return toks
}

// unhex returns the value of a hexadecimal digit, or -1.
func unhex(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}
	return -1
}
