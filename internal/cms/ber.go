package cms

import (
	"errors"
)

// maxDepth bounds the nesting of the ASN.1 elements read by normalize.
const maxDepth = 64

var errBER = errors.New("cms: malformed BER encoding")

// normalize converts a BER encoding into DER for what encoding/asn1
// refuses: indefinite lengths become definite, lengths are written on the
// fewest bytes, and constructed OCTET STRINGs become primitive. Tools that
// stream their output (openssl cms -stream, Java's CMSSignedDataStreamGenerator)
// write an indefinite length and cut the content into chunks.
//
// The output is at most as long as the input plus a few bytes per element:
// definite lengths replace the end-of-contents octets.
func normalize(b []byte) ([]byte, error) {
	out, rest, err := normalizeElem(nil, b, 0)
	if err != nil {
		return nil, err
	}
	if len(rest) != 0 {
		return nil, errBER
	}
	return out, nil
}

// header reads the identifier and length octets at the start of b. length is
// -1 for an indefinite length.
func header(b []byte) (tag []byte, constructed bool, length int, n int, err error) {
	if len(b) < 2 {
		return nil, false, 0, 0, errBER
	}
	i := 1
	if b[0]&0x1f == 0x1f { // high tag number form
		for {
			if i >= len(b) || i > 5 {
				return nil, false, 0, 0, errBER
			}
			c := b[i]
			i++
			if c&0x80 == 0 {
				break
			}
		}
	}
	tag = b[:i]
	constructed = b[0]&0x20 != 0
	if i >= len(b) {
		return nil, false, 0, 0, errBER
	}
	l := b[i]
	i++
	switch {
	case l < 0x80:
		length = int(l)
	case l == 0x80:
		if !constructed {
			return nil, false, 0, 0, errBER
		}
		length = -1
	default:
		k := int(l & 0x7f)
		if k > 4 || i+k > len(b) {
			return nil, false, 0, 0, errBER
		}
		for _, c := range b[i : i+k] {
			length = length<<8 | int(c)
		}
		i += k
		if length < 0 {
			return nil, false, 0, 0, errBER
		}
	}
	if length > len(b)-i {
		return nil, false, 0, 0, errBER
	}
	return tag, constructed, length, i, nil
}

// normalizeElem appends the DER form of the element at the start of b to out
// and returns the bytes after the element.
func normalizeElem(out, b []byte, depth int) ([]byte, []byte, error) {
	if depth > maxDepth {
		return nil, nil, errBER
	}
	tag, constructed, length, n, err := header(b)
	if err != nil {
		return nil, nil, err
	}
	body := b[n:]
	if !constructed {
		out = appendHeader(out, tag, length)
		return append(out, body[:length]...), body[length:], nil
	}
	// The children of a constructed element, normalized.
	var kids []byte
	var rest []byte
	if length >= 0 {
		inner := body[:length]
		for len(inner) > 0 {
			if kids, inner, err = normalizeElem(kids, inner, depth+1); err != nil {
				return nil, nil, err
			}
		}
		rest = body[length:]
	} else {
		inner := body
		for {
			if len(inner) < 2 {
				return nil, nil, errBER
			}
			if inner[0] == 0 && inner[1] == 0 {
				rest = inner[2:]
				break
			}
			if kids, inner, err = normalizeElem(kids, inner, depth+1); err != nil {
				return nil, nil, err
			}
		}
	}
	if len(tag) == 1 && tag[0] == 0x24 { // constructed OCTET STRING
		content, err := concatOctets(kids)
		if err != nil {
			return nil, nil, err
		}
		out = appendHeader(out, []byte{0x04}, len(content))
		return append(out, content...), rest, nil
	}
	out = appendHeader(out, tag, len(kids))
	return append(out, kids...), rest, nil
}

// concatOctets joins the contents of a sequence of primitive OCTET STRINGs
// (already normalized).
func concatOctets(b []byte) ([]byte, error) {
	var out []byte
	for len(b) > 0 {
		tag, _, length, n, err := header(b)
		if err != nil || len(tag) != 1 || tag[0] != 0x04 {
			return nil, errBER
		}
		out = append(out, b[n:n+length]...)
		b = b[n+length:]
	}
	return out, nil
}

func appendHeader(out, tag []byte, length int) []byte {
	out = append(out, tag...)
	return appendLength(out, length)
}

func appendLength(out []byte, length int) []byte {
	if length < 0x80 {
		return append(out, byte(length))
	}
	var tmp [8]byte
	k := 0
	for l := length; l > 0; l >>= 8 {
		k++
	}
	for i := 0; i < k; i++ {
		tmp[i] = byte(length >> (8 * (k - 1 - i)))
	}
	out = append(out, 0x80|byte(k))
	return append(out, tmp[:k]...)
}
