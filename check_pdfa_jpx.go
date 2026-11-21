package zerechnung

import (
	"bytes"
	"encoding/binary"

	"github.com/ritoon/zerechnung/internal/pdf"
)

// jpxSignature is the signature box that starts a file of the JP2 family
// (ISO 15444-1, I.5.1).
var jpxSignature = []byte{0, 0, 0, 0x0c, 'j', 'P', ' ', ' ', 0x0d, 0x0a, 0x87, 0x0a}

// jpxHeader holds what the JP2 header box (jp2h) of JPEG 2000 data tells,
// as veraPDF reads it for 6.2.8.3: the values are 0 for data that is not
// in the JP2 file format.
type jpxHeader struct {
	jp2      bool  // the data starts with the JP2 signature box
	channels int64 // number of components (ihdr)
	bitDepth int64 // bits per component (ihdr), 0 when they differ
	bpcc     bool  // a bits per component box: the components differ in depth
	specs    int64 // colour specification boxes (colr)
	approx   int64 // colour specification boxes with APPROX 1
	method   int64 // METH of the colour specification that applies
	enumCS   int64 // EnumCS of the colour specification that applies, -1 when none
}

// jpxBoxes calls fn with the type and content of each box of data, until
// fn returns false.
func jpxBoxes(data []byte, fn func(typ string, content []byte) bool) {
	for len(data) >= 8 {
		size := uint64(binary.BigEndian.Uint32(data))
		typ := string(data[4:8])
		head := uint64(8)
		switch size {
		case 0: // up to the end of the data
			size = uint64(len(data))
		case 1: // the size follows the type, on 8 bytes
			if len(data) < 16 {
				return
			}
			size, head = binary.BigEndian.Uint64(data[8:16]), 16
		}
		if size < head {
			return
		}
		size = min(size, uint64(len(data)))
		if !fn(typ, data[head:size]) {
			return
		}
		data = data[size:]
	}
}

// readJPX reads the header of JPEG 2000 data. The colour specification
// that applies is the first one with APPROX 1, or the only one.
func readJPX(data []byte) jpxHeader {
	h := jpxHeader{enumCS: -1}
	if !bytes.HasPrefix(data, jpxSignature) {
		return h
	}
	h.jp2 = true
	var header []byte
	jpxBoxes(data[len(jpxSignature):], func(typ string, content []byte) bool {
		if typ == "jp2h" {
			header = content
			return false
		}
		return true
	})
	firstMethod, approxMethod := int64(-1), int64(-1)
	firstEnum, approxEnum := int64(-1), int64(-1)
	jpxBoxes(header, func(typ string, b []byte) bool {
		switch typ {
		case "ihdr":
			if len(b) < 11 {
				return false
			}
			h.channels = int64(binary.BigEndian.Uint16(b[8:10]))
			h.bitDepth = int64(int8(b[10])) + 1
		case "bpcc":
			h.bpcc = true
		case "colr":
			if len(b) < 3 {
				return false
			}
			h.specs++
			meth, approx := int64(b[0]), b[2] == 1
			if firstMethod < 0 {
				firstMethod = meth
			}
			if approx {
				h.approx++
				if approxMethod < 0 {
					approxMethod = meth
				}
			}
			if meth == 1 {
				if len(b) < 7 {
					return false
				}
				enum := int64(binary.BigEndian.Uint32(b[3:7]))
				if firstEnum < 0 {
					firstEnum = enum
				}
				if approx && approxEnum < 0 {
					approxEnum = enum
				}
			}
		}
		return true
	})
	switch {
	case h.approx > 0:
		h.method, h.enumCS = max(approxMethod, 0), approxEnum
	case h.specs == 1:
		h.method, h.enumCS = max(firstMethod, 0), firstEnum
	}
	return h
}

// jpx checks the JPEG 2000 data of an image (6.2.8.3): 1, 3 or 4 colour
// channels, a single bit depth from 1 to 38 and, when the image dictionary
// has no /ColorSpace, a colour specification that applies, of method 1, 2
// or 3, not the enumerated colour space CIEJab (19). Data filtered by
// another filter before JPXDecode is not read.
func (s *pdfaScan) jpx(st *pdf.Stream, where string) {
	names, _ := s.filters(st.Dict)
	if len(names) != 1 || names[0] != "JPXDecode" {
		return
	}
	h := readJPX(st.Raw)
	if !h.jp2 {
		s.report("PDF-56", where, "the JPEG 2000 data is not in the JP2 file format: it has no header giving its colour channels and bit depth (6.2.8.3)")
		return
	}
	if h.channels != 1 && h.channels != 3 && h.channels != 4 {
		s.report("PDF-56", where, "the JPEG 2000 data has %d colour channels; PDF/A admits 1, 3 and 4 (6.2.8.3)", h.channels)
	}
	switch {
	case h.bpcc:
		s.report("PDF-56", where, "the colour channels of the JPEG 2000 data differ in bit depth (bpcc box); PDF/A requires a single bit depth (6.2.8.3)")
	case h.bitDepth < 1 || h.bitDepth > 38:
		s.report("PDF-56", where, "the JPEG 2000 data has a bit depth of %d; PDF/A admits 1 to 38 (6.2.8.3)", h.bitDepth)
	}
	if s.present(st.Dict, "ColorSpace") {
		return
	}
	switch {
	case h.specs != 1 && h.approx != 1:
		s.report("PDF-56", where, "the image has no /ColorSpace and its JPEG 2000 data has %d colour specifications, %d of them with APPROX 1; PDF/A requires one that applies (6.2.8.3)", h.specs, h.approx)
	case h.method < 1 || h.method > 3:
		s.report("PDF-56", where, "the image has no /ColorSpace and the colour specification of its JPEG 2000 data has the method %d; PDF/A admits 1, 2 and 3 (6.2.8.3)", h.method)
	case h.enumCS == 19:
		s.report("PDF-56", where, "the image has no /ColorSpace and its JPEG 2000 data uses the enumerated colour space CIEJab (19), which PDF/A forbids (6.2.8.3)")
	}
}
