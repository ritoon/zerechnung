// Copied from github.com/piglig/go-qr v1.1.0 (encode.go, qr_code.go), MIT
// License, Copyright (c) 2023 piglig; see LICENSE.

package qr

import "fmt"

// Options are the constraints that a specification puts on a symbol. The
// zero value asks for level L and allows every version.
type Options struct {
	// Level is the error correction level, used as given.
	Level Level
	// MinVersion and MaxVersion bound the version: the smallest version
	// of the range that holds the data is chosen. Zero stands for
	// MinVersion and MaxVersion respectively.
	MinVersion, MaxVersion int
}

// versions returns the version range of o, the zero values replaced.
func (o Options) versions() (int, int) {
	lo, hi := o.MinVersion, o.MaxVersion
	if lo == 0 {
		lo = MinVersion
	}
	if hi == 0 {
		hi = MaxVersion
	}
	return lo, hi
}

// EncodeText encodes text as a single segment, in numeric mode if it holds
// digits only, in alphanumeric mode if it holds characters of that mode
// only, in byte mode (UTF-8) otherwise.
func EncodeText(text string, o Options) (*Code, error) {
	segs, err := textSegments(text)
	if err != nil {
		return nil, err
	}
	return Encode(segs, o)
}

// EncodeBytes encodes data as a single segment in byte mode, whatever the
// characters it holds.
func EncodeBytes(data []byte, o Options) (*Code, error) {
	seg, err := Bytes(data)
	if err != nil {
		return nil, err
	}
	return Encode([]Segment{seg}, o)
}

// Encode encodes segs, in this order, in the smallest version allowed by o
// that holds them, with the mask of lowest penalty.
func Encode(segs []Segment, o Options) (*Code, error) {
	lo, hi := o.versions()
	return encode(segs, o.Level, lo, hi, -1)
}

// encode encodes segs at level l in the smallest version of [minVer, maxVer]
// that holds them, with mask msk, or the mask of lowest penalty if msk is
// -1.
func encode(segs []Segment, l Level, minVer, maxVer, msk int) (*Code, error) {
	if l < Low || l > High {
		return nil, fmt.Errorf("%w: error correction level %d", ErrInvalidArgument, l)
	}
	if minVer < MinVersion || minVer > maxVer || maxVer > MaxVersion {
		return nil, fmt.Errorf("%w: versions %d to %d", ErrInvalidArgument, minVer, maxVer)
	}
	if msk < -1 || msk > 7 {
		return nil, fmt.Errorf("%w: mask %d", ErrInvalidArgument, msk)
	}

	version := minVer
	for ; ; version++ {
		capacityBits := numDataCodewords(version, l) * 8
		usedBits := totalBits(segs, version)
		if usedBits != -1 && usedBits <= capacityBits {
			break
		}
		if version >= maxVer {
			if usedBits == -1 {
				return nil, fmt.Errorf("%w: a segment holds too many characters for version %d", ErrDataTooLong, maxVer)
			}
			return nil, fmt.Errorf("%w: %d bits, version %d-%s holds %d", ErrDataTooLong, usedBits, maxVer, l, capacityBits)
		}
	}

	var bb bitBuffer
	for i := range segs {
		seg := &segs[i]
		bb.appendBits(seg.mode.bits, 4)
		bb.appendBits(seg.numChars, seg.mode.numCharCountBits(version))
		bb.appendBuffer(&seg.data)
	}
	// Terminator, padding to a byte boundary, then pad codewords.
	capacityBits := numDataCodewords(version, l) * 8
	bb.appendBits(0, min(4, capacityBits-bb.len()))
	bb.appendBits(0, (8-bb.len()%8)%8)
	for pad := 0xEC; bb.len() < capacityBits; pad ^= 0xEC ^ 0x11 {
		bb.appendBits(pad, 8)
	}
	return newCode(version, l, bb.data, msk), nil
}

// newCode lays out the data codewords in a symbol of version ver at level
// l, with mask msk, or the mask of lowest penalty if msk is -1.
func newCode(ver int, l Level, dataCodewords []byte, msk int) *Code {
	b := newBuilder(ver, l)
	b.drawFunctionPatterns()
	b.drawCodewords(b.addEccAndInterleave(dataCodewords))
	if msk == -1 {
		msk = b.chooseBestMask()
	}
	b.applyMask(msk)
	b.drawFormatBits(msk)
	return b.toCode(msk)
}
