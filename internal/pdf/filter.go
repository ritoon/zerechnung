package pdf

import (
	"errors"
	"fmt"
)

// UnsupportedFilterError is returned when a stream uses a filter the reader
// does not implement.
type UnsupportedFilterError struct {
	Filter Name
}

func (e *UnsupportedFilterError) Error() string {
	return fmt.Sprintf("pdf: unsupported stream filter /%s (supported: FlateDecode, ASCIIHexDecode, ASCII85Decode)", e.Filter)
}

// Decode returns the decoded data of a stream. Unfiltered data aliases the
// input and must not be modified.
func (r *Reader) Decode(s *Stream) ([]byte, error) { return r.decode(s, 0, nil) }

// DecodeSize is Decode with a hint of the decoded size, used to size the
// output buffer.
func (r *Reader) DecodeSize(s *Stream, hint int) ([]byte, error) { return r.decode(s, hint, nil) }

// DecodeScratch is DecodeSize with the buffers of FlateDecode lent by sc:
// the data it returns is valid until sc.Release.
func (r *Reader) DecodeScratch(s *Stream, hint int, sc *Scratch) ([]byte, error) {
	return r.decode(s, hint, sc)
}

func (r *Reader) decode(s *Stream, hint int, sc *Scratch) ([]byte, error) {
	f := r.Resolve(s.Dict.Get("Filter"))
	if f == nil {
		return s.Raw, nil
	}
	var (
		filters []Name
		parms   []Object
	)
	switch v := f.(type) {
	case Name:
		filters = []Name{v}
		parms = []Object{r.Resolve(s.Dict.Get("DecodeParms"))}
	case Array:
		dp, _ := r.Resolve(s.Dict.Get("DecodeParms")).(Array)
		for i, x := range v {
			n, ok := r.Resolve(x).(Name)
			if !ok {
				return nil, errors.New("pdf: bad /Filter entry")
			}
			filters = append(filters, n)
			var p Object
			if i < len(dp) {
				p = r.Resolve(dp[i])
			}
			parms = append(parms, p)
		}
	default:
		return nil, errors.New("pdf: bad /Filter")
	}
	data := s.Raw
	var err error
	for i, name := range filters {
		h := 0
		if i == len(filters)-1 {
			h = hint
		}
		parm, _ := parms[i].(Dict)
		switch name {
		case "FlateDecode", "Fl":
			data, err = inflateIn(data, h, sc)
			if err != nil {
				return data, fmt.Errorf("pdf: FlateDecode: %w", err)
			}
			if data, err = r.unpredict(data, parm); err != nil {
				return nil, err
			}
		case "LZWDecode", "LZW":
			early := true
			if v, ok := AsInt(r.Resolve(parm.Get("EarlyChange"))); ok {
				early = v != 0
			}
			data, err = lzwDecode(data, early)
			if err != nil {
				return data, err
			}
			if data, err = r.unpredict(data, parm); err != nil {
				return nil, err
			}
		case "RunLengthDecode", "RL":
			if data, err = runLengthDecode(data); err != nil {
				return nil, err
			}
		case "ASCIIHexDecode", "AHx":
			data = asciiHexDecode(data)
		case "ASCII85Decode", "A85":
			if data, err = ascii85Decode(data); err != nil {
				return nil, err
			}
		case "Crypt":
			if n, _ := parm.Name("Name"); n != "" && n != "Identity" {
				return nil, &UnsupportedFilterError{name}
			}
		default:
			return nil, &UnsupportedFilterError{name}
		}
	}
	return data, nil
}

// unpredict reverses a TIFF or PNG predictor (ISO 32000-1, 7.4.4.4), in
// place.
func (r *Reader) unpredict(data []byte, parm Dict) ([]byte, error) {
	if parm == nil {
		return data, nil
	}
	get := func(k Name, def int64) int64 {
		if v, ok := AsInt(r.Resolve(parm.Get(k))); ok {
			return v
		}
		return def
	}
	pred := get("Predictor", 1)
	if pred <= 1 {
		return data, nil
	}
	colors, bpc, columns := get("Colors", 1), get("BitsPerComponent", 8), get("Columns", 1)
	if colors < 1 || colors > 32 || columns < 1 || columns > 1<<24 {
		return nil, errors.New("pdf: bad predictor parameters")
	}
	switch bpc {
	case 1, 2, 4, 8, 16:
	default:
		return nil, errors.New("pdf: bad predictor parameters")
	}
	bpp := int((colors*bpc + 7) / 8)
	rowLen := int((colors*bpc*columns + 7) / 8)
	if pred == 2 {
		if bpc != 8 {
			return nil, errors.New("pdf: TIFF predictor only supported for 8 bits per component")
		}
		for row := 0; row < len(data); row += rowLen {
			end := min(row+rowLen, len(data))
			for i := row + bpp; i < end; i++ {
				data[i] += data[i-bpp]
			}
		}
		return data, nil
	}
	if pred < 10 {
		return nil, fmt.Errorf("pdf: unsupported predictor %d", pred)
	}
	// PNG: each row starts with its filter type byte.
	out := data[:0]
	var prev []byte // previous decoded row
	for in := 0; in < len(data); in += rowLen + 1 {
		typ := data[in]
		end := min(in+1+rowLen, len(data))
		cur := data[in+1 : end]
		o := len(out)
		out = append(out, cur...) // moves the row back by one byte per row
		row := out[o:]
		switch typ {
		case 0:
		case 1: // Sub
			for i := bpp; i < len(row); i++ {
				row[i] += row[i-bpp]
			}
		case 2: // Up
			for i := 0; i < len(row) && i < len(prev); i++ {
				row[i] += prev[i]
			}
		case 3: // Average
			for i := range row {
				var left, up int
				if i >= bpp {
					left = int(row[i-bpp])
				}
				if i < len(prev) {
					up = int(prev[i])
				}
				row[i] += byte((left + up) / 2)
			}
		case 4: // Paeth
			for i := range row {
				var a, b, c int
				if i >= bpp {
					a = int(row[i-bpp])
				}
				if i < len(prev) {
					b = int(prev[i])
					if i >= bpp {
						c = int(prev[i-bpp])
					}
				}
				row[i] += paeth(a, b, c)
			}
		default:
			return nil, fmt.Errorf("pdf: bad PNG predictor row type %d", typ)
		}
		prev = row
	}
	return out, nil
}

func paeth(a, b, c int) byte {
	p := a + b - c
	pa, pb, pc := abs(p-a), abs(p-b), abs(p-c)
	switch {
	case pa <= pb && pa <= pc:
		return byte(a)
	case pb <= pc:
		return byte(b)
	}
	return byte(c)
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// asciiHexDecode decodes ASCIIHexDecode data up to the closing '>'.
func asciiHexDecode(src []byte) []byte {
	out := make([]byte, 0, len(src)/2)
	var hi byte
	half := false
	for _, c := range src {
		if c == '>' {
			break
		}
		if !isHex(c) {
			continue
		}
		if half {
			out = append(out, hi<<4|unhex(c))
		} else {
			hi = unhex(c)
		}
		half = !half
	}
	if half {
		out = append(out, hi<<4)
	}
	return out
}

// ascii85Decode decodes ASCII85Decode data up to the closing "~>".
func ascii85Decode(src []byte) ([]byte, error) {
	out := make([]byte, 0, len(src)*4/5+4)
	var v uint64
	n := 0
	for i := 0; i < len(src); i++ {
		c := src[i]
		switch {
		case c == '~':
			i = len(src)
			continue
		case class[c] == white:
			continue
		case c == 'z' && n == 0:
			out = append(out, 0, 0, 0, 0)
			continue
		case c < '!' || c > 'u':
			return nil, errors.New("pdf: bad ASCII85 data")
		}
		v = v*85 + uint64(c-'!')
		n++
		if n == 5 {
			if v > 0xffffffff {
				return nil, errors.New("pdf: bad ASCII85 data")
			}
			out = append(out, byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
			v, n = 0, 0
		}
	}
	if n == 1 {
		return nil, errors.New("pdf: bad ASCII85 data")
	}
	if n > 1 {
		for k := n; k < 5; k++ {
			v = v*85 + 84
		}
		b := []byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)}
		out = append(out, b[:n-1]...)
	}
	return out, nil
}
