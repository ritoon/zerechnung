// Copied from github.com/piglig/go-qr v1.1.0 (internal/reedsolomon), MIT
// License, Copyright (c) 2023 piglig; see LICENSE.

package qr

// Reed-Solomon codes over GF(2^8), with the field polynomial 0x11D of
// ISO/IEC 18004: the encoder side only.

var (
	gfExp [512]byte // gfExp[i] = 2^i, doubled to skip the modulo in gfMul
	gfLog [256]byte // gfLog[x] = i such that 2^i = x
)

// maxDivisorDegree is the largest number of error correction codewords per
// block in ISO/IEC 18004.
const maxDivisorDegree = 30

// rsDivisors holds the generator polynomial of each degree up to
// maxDivisorDegree, read-only after init.
var rsDivisors [maxDivisorDegree + 1][]byte

func init() {
	x := 1
	for i := range 255 {
		gfExp[i] = byte(x)
		gfLog[x] = byte(i)
		x = gfMultiplyBitwise(x, 0x02)
	}
	for i := 255; i < 512; i++ {
		gfExp[i] = gfExp[i-255]
	}
	for d := 1; d <= maxDivisorDegree; d++ {
		rsDivisors[d] = computeDivisor(d)
	}
}

// gfMultiplyBitwise multiplies two field elements without the tables, which
// it serves to build.
func gfMultiplyBitwise(x, y int) int {
	z := 0
	for i := 7; i >= 0; i-- {
		z = (z << 1) ^ ((z >> 7) * 0x11D)
		z ^= ((y >> i) & 1) * x
	}
	return z
}

// gfMul multiplies two field elements with the tables.
func gfMul(a, b byte) byte {
	if a == 0 || b == 0 {
		return 0
	}
	return gfExp[int(gfLog[a])+int(gfLog[b])]
}

// rsDivisor returns the generator polynomial of the given degree, between 1
// and maxDivisorDegree, without its leading coefficient.
func rsDivisor(degree int) []byte {
	return rsDivisors[degree]
}

// computeDivisor computes the product of (x - 2^i) for i from 0 to
// degree-1, highest coefficient first, without the leading 1.
func computeDivisor(degree int) []byte {
	res := make([]byte, degree)
	res[degree-1] = 1
	root := byte(1)
	for range degree {
		for j := range res {
			res[j] = gfMul(res[j], root)
			if j+1 < len(res) {
				res[j] ^= res[j+1]
			}
		}
		root = gfMul(root, 0x02)
	}
	return res
}

// rsRemainder returns the error correction codewords of data: the remainder
// of its division by divisor, of len(divisor) bytes.
func rsRemainder(data, divisor []byte) []byte {
	res := make([]byte, len(divisor))
	for _, b := range data {
		factor := b ^ res[0]
		copy(res, res[1:])
		res[len(res)-1] = 0
		for i := range res {
			res[i] ^= gfMul(divisor[i], factor)
		}
	}
	return res
}
