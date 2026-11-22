package xmp

import (
	"regexp"
	"testing"
)

// The regular expressions the value checks of types.go replace: the
// oracle of TestValueTypes and FuzzValueTypes.
var valueRegexps = []struct {
	name string
	re   *regexp.Regexp
	fn   func(string) bool
}{
	{"Integer", regexp.MustCompile(`^[+-]?[0-9]+$`), isInteger},
	{"Real", regexp.MustCompile(`^[+-]?([0-9]+\.?[0-9]*|\.[0-9]+)$`), isReal},
	{"Rational", regexp.MustCompile(`^[+-]?[0-9]+/[+-]?[0-9]+$`), isRational},
	{"Date", regexp.MustCompile(`^[0-9]{4}(-[0-9]{2}(-[0-9]{2}(T[0-9]{2}:[0-9]{2}(:[0-9]{2}(\.[0-9]+)?)?(Z|[+-][0-9]{2}:[0-9]{2})?)?)?)?$`), isDate},
	{"GPSCoordinate", regexp.MustCompile(`^[0-9]{1,3},[0-9]{1,2}(,[0-9]{1,2}|\.[0-9]+)[NSEW]$`), isGPS},
}

var headerRegexps = []struct {
	name string
	re   *regexp.Regexp
}{
	{"bytes", regexp.MustCompile(`\sbytes\s*=`)},
	{"encoding", regexp.MustCompile(`\sencoding\s*=`)},
}

var valueSamples = []string{
	"", "+", "-", "0", "7", "-12", "+12", "1a", "a1", "12 ", " 12", "--1", "+-1", "١٢",
	".", "1.", ".5", "-.5", "+1.25", "1.2.3", "1..2", "..1", "1e5", "-1.", "1.-2",
	"1/2", "-1/-2", "+1/+2", "1/", "/2", "1//2", "1/2/3", "a/b", "1/2 ", "1/2a",
	"2026", "202", "20261", "2026-", "2026-1", "2026-11", "2026-11-", "2026-11-2", "2026-11-22",
	"2026-11-22T", "2026-11-22T10", "2026-11-22T10:", "2026-11-22T10:3", "2026-11-22T10:30",
	"2026-11-22T10:30:", "2026-11-22T10:30:5", "2026-11-22T10:30:59", "2026-11-22T10:30:59.",
	"2026-11-22T10:30:59.1", "2026-11-22T10:30:59.123456", "2026-11-22T10:30:59Z",
	"2026-11-22T10:30:59.5Z", "2026-11-22T10:30Z", "2026-11-22T10:30+01:00", "2026-11-22T10:30:59-05:30",
	"2026-11-22T10:30+0100", "2026-11-22T10:30+01:0", "2026-11-22T10:30+01:00Z", "2026-11-22Z",
	"2026-11-22T10:30:59.Z", "2026-11-22t10:30", "2026/11/22", "2026-11-22T10:30:59.5+01:00",
	"0,0,0N", "1,2,3S", "12,34,56E", "123,45,67W", "1234,5,6N", "1,234,5N", "1,2,345N",
	"1,2.5N", "12,34.567E", "1,2.N", "1,2N", "1,2,3", "1,2,3NN", "1,2,3n", ",1,2N", "1,,2N",
	"1,2.3.4N", "1,2,3.4N",
}

func TestValueTypes(t *testing.T) {
	for _, v := range valueRegexps {
		for _, s := range valueSamples {
			if got, want := v.fn(s), v.re.MatchString(s); got != want {
				t.Errorf("%s(%q) = %v, the regular expression says %v", v.name, s, got, want)
			}
		}
	}
}

func FuzzValueTypes(f *testing.F) {
	for _, s := range valueSamples {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		for _, v := range valueRegexps {
			if got, want := v.fn(s), v.re.MatchString(s); got != want {
				t.Errorf("%s(%q) = %v, the regular expression says %v", v.name, s, got, want)
			}
		}
	})
}

var headerSamples = []string{
	`<?xpacket begin="" id="W5M0MpCehiHzreSzNTczkc9d"?>`,
	`<?xpacket begin="" id="x" bytes="1234"?>`,
	`<?xpacket begin="" id="x" encoding="UTF-8"?>`,
	"<?xpacket\tbytes = '1'?>", "<?xpacket\nencoding\r\n=?>", `<?xpacket xbytes="1"?>`,
	`<?xpacket bytes ?>`, `<?xpacket bytes`, `bytes=`, ` bytes=`, "\fencoding\f=", "\vbytes=",
	`<?xpacket begin="bytes=" ?>`, ` bytesbytes=`, `  bytes  bytes =`,
}

func TestHeaderAttr(t *testing.T) {
	for _, h := range headerRegexps {
		for _, s := range headerSamples {
			if got, want := headerAttr([]byte(s), h.name), h.re.MatchString(s); got != want {
				t.Errorf("headerAttr(%q, %q) = %v, the regular expression says %v", s, h.name, got, want)
			}
		}
	}
}

func FuzzHeaderAttr(f *testing.F) {
	for _, s := range headerSamples {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		for _, h := range headerRegexps {
			if got, want := headerAttr(b, h.name), h.re.Match(b); got != want {
				t.Errorf("headerAttr(%q, %q) = %v, the regular expression says %v", b, h.name, got, want)
			}
		}
	})
}

func TestValueTypesAllocs(t *testing.T) {
	n := testing.AllocsPerRun(100, func() {
		for _, v := range valueRegexps {
			for _, s := range valueSamples {
				v.fn(s)
			}
		}
		headerAttr([]byte(headerSamples[1]), "bytes")
	})
	if n != 0 {
		t.Errorf("the value checks allocate %v times, want 0", n)
	}
}
