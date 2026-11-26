package qrcode

import "github.com/ritoon/zerechnung/internal/qr"

// Errors returned by Encode, to be tested with errors.Is.
var (
	// ErrInvalidArgument reports an error correction level or a version
	// range out of range.
	ErrInvalidArgument = qr.ErrInvalidArgument
	// ErrDataTooLong reports data that no allowed version can hold at the
	// requested error correction level.
	ErrDataTooLong = qr.ErrDataTooLong
)

// Level is the error correction level of a symbol.
type Level int

// The four error correction levels.
const (
	Low      = Level(qr.Low)      // L: about 7% of the codewords can be restored.
	Medium   = Level(qr.Medium)   // M: about 15% of the codewords can be restored.
	Quartile = Level(qr.Quartile) // Q: about 25% of the codewords can be restored.
	High     = Level(qr.High)     // H: about 30% of the codewords can be restored.
)

// String returns the letter of the level: L, M, Q or H.
func (l Level) String() string { return qr.Level(l).String() }

// Options are the constraints that a specification puts on a symbol. The
// zero value asks for level L and allows every version.
type Options struct {
	// Level is the error correction level, used as given.
	Level Level
	// MinVersion and MaxVersion bound the version, from 1 to 40: the
	// smallest version of the range that holds the data is chosen. Zero
	// stands for 1 and 40 respectively.
	MinVersion, MaxVersion int
}

// Encode encodes data in byte mode, as a single segment, in the smallest
// version allowed by o that holds it, with the mask of lowest penalty.
func Encode(data []byte, o Options) (*Symbol, error) {
	c, err := qr.EncodeBytes(data, qr.Options{Level: qr.Level(o.Level), MinVersion: o.MinVersion, MaxVersion: o.MaxVersion})
	if err != nil {
		return nil, err
	}
	return &Symbol{c: c}, nil
}

// Symbol is a QR Code symbol, without its quiet zone. It is immutable.
type Symbol struct {
	c *qr.Code
}

// Version returns the version of the symbol, from 1 to 40.
func (s *Symbol) Version() int { return s.c.Version() }

// Size returns the number of modules on each side: 17 + 4 × version.
func (s *Symbol) Size() int { return s.c.Size() }

// Level returns the error correction level of the symbol.
func (s *Symbol) Level() Level { return Level(s.c.Level()) }

// Module reports whether the module in column x and row y is dark, (0, 0)
// being the top left corner. Modules outside the symbol are light.
func (s *Symbol) Module(x, y int) bool { return s.c.Module(x, y) }
