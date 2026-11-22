package pt

import (
	"fmt"
	"math"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

type function struct {
	min, max int // arity; max -1 for any
	fn       func(c *ctx, f *call, args []seq) (seq, error)
}

// funcs are the functions of the XPath subset.
var funcs map[string]function

func init() {
	funcs = map[string]function{
		"not": {1, 1, func(_ *ctx, _ *call, a []seq) (seq, error) {
			b, err := ebv(a[0])
			return boolSeq(!b), err
		}},
		"true":   {0, 0, func(*ctx, *call, []seq) (seq, error) { return boolSeq(true), nil }},
		"false":  {0, 0, func(*ctx, *call, []seq) (seq, error) { return boolSeq(false), nil }},
		"exists": {1, 1, func(_ *ctx, _ *call, a []seq) (seq, error) { return boolSeq(len(a[0]) > 0), nil }},
		"empty":  {1, 1, func(_ *ctx, _ *call, a []seq) (seq, error) { return boolSeq(len(a[0]) == 0), nil }},
		"count": {1, 1, func(_ *ctx, _ *call, a []seq) (seq, error) {
			return seq{intItem(int64(len(a[0])))}, nil
		}},
		"name":             {0, 1, fnName},
		"string":           {0, 1, fnString},
		"normalize-space":  {0, 1, fnNormalizeSpace},
		"string-length":    {0, 1, fnStringLength},
		"starts-with":      {2, 2, strPred(strings.HasPrefix)},
		"ends-with":        {2, 2, strPred(strings.HasSuffix)},
		"contains":         {2, 2, strPred(strings.Contains)},
		"substring-before": {2, 2, fnSubstringBefore},
		"substring-after":  {2, 2, fnSubstringAfter},
		"substring":        {2, 3, fnSubstring},
		"concat":           {2, -1, fnConcat},
		"matches":          {2, 3, fnMatches},
		"index-of":         {2, 2, fnIndexOf},
		"distinct-values":  {1, 1, fnDistinctValues},
		"round":            {1, 1, fnRound},
		"sum":              {1, 1, fnSum},
		"xs:decimal":       {1, 1, fnDecimal},
	}
}

func intItem(i int64) item { return item{k: kInteger, r: new(big.Rat).SetInt64(i)} }

func strItem(s string) seq { return seq{{k: kString, s: s}} }

// oneAtomic atomizes an argument that allows at most one item.
func oneAtomic(s seq, f string) (item, bool, error) {
	a := atomize(s)
	switch len(a) {
	case 0:
		return item{}, false, nil
	case 1:
		return a[0], true, nil
	}
	return item{}, false, dynErr("%s: a sequence of %d items is not allowed as argument", f, len(a))
}

// strArg returns an argument as a string: "" for the empty sequence.
func strArg(s seq, f string) (string, error) {
	i, ok, err := oneAtomic(s, f)
	if err != nil || !ok {
		return "", err
	}
	return itemString(i), nil
}

// ctxOrArg returns the argument of a function that defaults to the context
// item.
func ctxOrArg(c *ctx, a []seq) seq {
	if len(a) == 0 {
		return seq{c.it}
	}
	return a[0]
}

// itemString returns the string value of an atomic item.
func itemString(i item) string {
	switch i.k {
	case kNode:
		return stringValue(i)
	case kString, kUntyped:
		return i.s
	case kBool:
		return strconv.FormatBool(i.b)
	case kInteger:
		return i.r.Num().String()
	case kDecimal:
		return decimalString(i.r)
	}
	return doubleString(i.f)
}

// decimalString returns the canonical form of an xs:decimal: no trailing
// zero, no decimal point for an integer value.
func decimalString(r *big.Rat) string {
	if r.IsInt() {
		return r.Num().String()
	}
	s := r.FloatString(divScale + 2)
	return strings.TrimRight(strings.TrimRight(s, "0"), ".")
}

// doubleString returns the canonical form of an xs:double.
func doubleString(f float64) string {
	switch {
	case math.IsNaN(f):
		return "NaN"
	case math.IsInf(f, 1):
		return "INF"
	case math.IsInf(f, -1):
		return "-INF"
	case f == 0:
		if math.Signbit(f) {
			return "-0"
		}
		return "0"
	}
	if a := math.Abs(f); a >= 1e-6 && a < 1e6 {
		return strconv.FormatFloat(f, 'f', -1, 64)
	}
	s := strconv.FormatFloat(f, 'E', -1, 64)
	mant, exp, _ := strings.Cut(s, "E")
	if !strings.Contains(mant, ".") {
		mant += ".0"
	}
	e, _ := strconv.Atoi(exp)
	return mant + "E" + strconv.Itoa(e)
}

func fnName(c *ctx, _ *call, a []seq) (seq, error) {
	s := ctxOrArg(c, a)
	if len(s) == 0 {
		return strItem(""), nil
	}
	if len(s) > 1 || s[0].k != kNode {
		return nil, dynErr("name: the argument is not a single node")
	}
	it := s[0]
	switch {
	case it.n == nil:
		return strItem(""), nil
	case it.a > 0:
		at := it.n.Attrs[it.a-1]
		if at.Prefix != "" {
			return strItem(at.Prefix + ":" + at.Name), nil
		}
		return strItem(at.Name), nil
	case it.n.Prefix != "":
		return strItem(it.n.Prefix + ":" + it.n.Name), nil
	}
	return strItem(it.n.Name), nil
}

func fnString(c *ctx, _ *call, a []seq) (seq, error) {
	s, err := strArg(ctxOrArg(c, a), "string")
	return strItem(s), err
}

// normalizeSpace implements normalize-space(): XML white space collapsed
// to single spaces and trimmed.
func normalizeSpace(s string) string {
	b := make([]byte, 0, len(s))
	sp := false
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case ' ', '\t', '\n', '\r':
			sp = len(b) > 0
			continue
		}
		if sp {
			b = append(b, ' ')
			sp = false
		}
		b = append(b, s[i])
	}
	return string(b)
}

func fnNormalizeSpace(c *ctx, _ *call, a []seq) (seq, error) {
	s, err := strArg(ctxOrArg(c, a), "normalize-space")
	return strItem(normalizeSpace(s)), err
}

func fnStringLength(c *ctx, _ *call, a []seq) (seq, error) {
	s, err := strArg(ctxOrArg(c, a), "string-length")
	return seq{intItem(int64(utf8.RuneCountInString(s)))}, err
}

func strPred(f func(s, t string) bool) func(*ctx, *call, []seq) (seq, error) {
	return func(_ *ctx, c *call, a []seq) (seq, error) {
		s, err := strArg(a[0], c.name)
		if err != nil {
			return nil, err
		}
		t, err := strArg(a[1], c.name)
		return boolSeq(f(s, t)), err
	}
}

func fnSubstringBefore(_ *ctx, c *call, a []seq) (seq, error) {
	s, err := strArg(a[0], c.name)
	if err != nil {
		return nil, err
	}
	t, err := strArg(a[1], c.name)
	if err != nil {
		return nil, err
	}
	before, _, ok := strings.Cut(s, t)
	if !ok {
		before = ""
	}
	return strItem(before), nil
}

func fnSubstringAfter(_ *ctx, c *call, a []seq) (seq, error) {
	s, err := strArg(a[0], c.name)
	if err != nil {
		return nil, err
	}
	t, err := strArg(a[1], c.name)
	if err != nil {
		return nil, err
	}
	_, after, ok := strings.Cut(s, t)
	if !ok {
		after = ""
	}
	return strItem(after), nil
}

// numArg returns a numeric argument as a float64 (substring()).
func numArg(s seq, f string) (float64, error) {
	i, ok, err := oneAtomic(s, f)
	if err != nil {
		return 0, err
	}
	if !ok {
		return math.NaN(), nil
	}
	n, err := toNumber(i)
	if err != nil {
		return 0, err
	}
	return toFloat(n), nil
}

// xpathRound rounds half towards positive infinity, as round() does.
func xpathRound(f float64) float64 {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return f
	}
	return math.Floor(f + 0.5)
}

func fnSubstring(_ *ctx, c *call, a []seq) (seq, error) {
	s, err := strArg(a[0], c.name)
	if err != nil {
		return nil, err
	}
	start, err := numArg(a[1], c.name)
	if err != nil {
		return nil, err
	}
	length := math.Inf(1)
	if len(a) == 3 {
		if length, err = numArg(a[2], c.name); err != nil {
			return nil, err
		}
	}
	from := xpathRound(start)
	to := from + xpathRound(length)
	var b strings.Builder
	p := 1.0
	for _, r := range s {
		if p >= from && p < to {
			b.WriteRune(r)
		}
		p++
	}
	return strItem(b.String()), nil
}

func fnConcat(_ *ctx, c *call, a []seq) (seq, error) {
	var b strings.Builder
	for _, s := range a {
		v, err := strArg(s, c.name)
		if err != nil {
			return nil, err
		}
		b.WriteString(v)
	}
	return strItem(b.String()), nil
}

func fnMatches(_ *ctx, c *call, a []seq) (seq, error) {
	s, err := strArg(a[0], c.name)
	if err != nil {
		return nil, err
	}
	m := c.re
	if m == nil {
		pat, err := strArg(a[1], c.name)
		if err != nil {
			return nil, err
		}
		flags := ""
		if len(a) == 3 {
			if flags, err = strArg(a[2], c.name); err != nil {
				return nil, err
			}
		}
		if m, err = compileRegex(pat, flags); err != nil {
			return nil, dynErr("matches: %v", err)
		}
	}
	return boolSeq(m.MatchString(s)), nil
}

// eqValue compares two atomic values with eq, an untyped value as a string
// (index-of(), distinct-values()). Values that cannot be compared are not
// equal.
func eqValue(x, y item) bool {
	if x.k == kUntyped {
		x.k = kString
	}
	if y.k == kUntyped {
		y.k = kString
	}
	ok, err := compareValues("=", x, y, false)
	return err == nil && ok
}

func fnIndexOf(_ *ctx, c *call, a []seq) (seq, error) {
	search, ok, err := oneAtomic(a[1], c.name)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, dynErr("index-of: empty search value")
	}
	var out seq
	for i, x := range atomize(a[0]) {
		if eqValue(x, search) {
			out = append(out, intItem(int64(i+1)))
		}
	}
	return out, nil
}

func fnDistinctValues(_ *ctx, _ *call, a []seq) (seq, error) {
	var out seq
next:
	for _, x := range atomize(a[0]) {
		for _, y := range out {
			if eqValue(x, y) {
				continue next
			}
		}
		out = append(out, x)
	}
	return out, nil
}

func fnRound(_ *ctx, c *call, a []seq) (seq, error) {
	i, ok, err := oneAtomic(a[0], c.name)
	if err != nil || !ok {
		return nil, err
	}
	n, err := toNumber(i)
	if err != nil {
		return nil, err
	}
	switch n.k {
	case kDouble:
		return seq{{k: kDouble, f: xpathRound(n.f)}}, nil
	case kInteger:
		return seq{n}, nil
	}
	// floor(x + 0.5)
	h := new(big.Rat).Add(n.r, big.NewRat(1, 2))
	q := new(big.Int).Div(h.Num(), h.Denom()) // Euclidean: floor for a positive denominator
	return seq{{k: kDecimal, r: new(big.Rat).SetInt(q)}}, nil
}

func fnSum(_ *ctx, _ *call, a []seq) (seq, error) {
	vals := atomize(a[0])
	if len(vals) == 0 {
		return seq{intItem(0)}, nil
	}
	total, err := toNumber(vals[0])
	if err != nil {
		return nil, err
	}
	for _, v := range vals[1:] {
		s, err := arith("+", seq{total}, seq{v})
		if err != nil {
			return nil, err
		}
		total = s[0]
	}
	return seq{total}, nil
}

// reDecimal is the lexical space of xs:decimal.
var reDecimal = regexp.MustCompile(`^[+-]?([0-9]+(\.[0-9]*)?|\.[0-9]+)$`)

// reDouble is the lexical space of xs:double, but INF, -INF and NaN.
var reDouble = regexp.MustCompile(`^[+-]?([0-9]+(\.[0-9]*)?|\.[0-9]+)([eE][+-]?[0-9]+)?$`)

func fnDecimal(_ *ctx, c *call, a []seq) (seq, error) {
	i, ok, err := oneAtomic(a[0], c.name)
	if err != nil || !ok {
		return nil, err
	}
	switch i.k {
	case kInteger, kDecimal:
		return seq{{k: kDecimal, r: i.r}}, nil
	case kDouble:
		if math.IsNaN(i.f) || math.IsInf(i.f, 0) {
			return nil, dynErr("cannot cast %s to xs:decimal", doubleString(i.f))
		}
		r, _ := new(big.Rat).SetString(strconv.FormatFloat(i.f, 'g', -1, 64))
		return seq{{k: kDecimal, r: r}}, nil
	case kBool:
		return seq{{k: kDecimal, r: new(big.Rat).SetInt64(int64(boolInt(i.b)))}}, nil
	}
	s := strings.Trim(i.s, " \t\n\r")
	if !reDecimal.MatchString(s) {
		return nil, dynErr("cannot cast %q to xs:decimal", i.s)
	}
	r, ok := new(big.Rat).SetString(strings.TrimSuffix(strings.TrimPrefix(s, "+"), "."))
	if !ok {
		return nil, dynErr("cannot cast %q to xs:decimal", i.s)
	}
	return seq{{k: kDecimal, r: r}}, nil
}

// matcher is a compiled XPath regular expression.
type matcher interface{ MatchString(string) bool }

// lengthMatcher matches ^(.{min,max})$ for a max beyond the repetition
// limit of package regexp: a string of min to max characters, none a line
// feed or a carriage return unless dotAll (flag s).
type lengthMatcher struct {
	min, max int
	dotAll   bool
}

func (m lengthMatcher) MatchString(s string) bool {
	if !m.dotAll && strings.ContainsAny(s, "\n\r") {
		return false
	}
	n := utf8.RuneCountInString(s)
	return n >= m.min && n <= m.max
}

var reBoundedAny = regexp.MustCompile(`^\^\(\.\{(\d+),(\d+)\}\)\$$`)

// compileRegex compiles an XPath regular expression with its flags (s, i,
// m). The syntax of XPath is translated where it differs from package
// regexp: . matches no line feed nor carriage return without the flag s, \d
// any decimal digit of Unicode. The constructs package regexp lacks are
// refused.
func compileRegex(pat, flags string) (matcher, error) {
	dotAll := false
	prefix := ""
	for _, f := range flags {
		switch f {
		case 's':
			dotAll = true
		case 'i', 'm':
			prefix += string(f)
		default:
			return nil, fmt.Errorf("regular expression %q: flag %c is not supported", pat, f)
		}
	}
	if m := reBoundedAny.FindStringSubmatch(pat); m != nil && prefix == "" {
		lo, _ := strconv.Atoi(m[1])
		hi, _ := strconv.Atoi(m[2])
		if hi > 1000 {
			return lengthMatcher{lo, hi, dotAll}, nil
		}
	}
	if prefix != "" {
		prefix = "(?" + prefix + ")"
	}
	var b strings.Builder
	class := false
	for i := 0; i < len(pat); i++ {
		c := pat[i]
		switch {
		case c == '\\':
			if i+1 >= len(pat) {
				return nil, fmt.Errorf("regular expression %q: trailing backslash", pat)
			}
			i++
			switch e := pat[i]; e {
			case 'd':
				b.WriteString(`\p{Nd}`)
			case 'D':
				if class {
					return nil, fmt.Errorf("regular expression %q: \\D in a class", pat)
				}
				b.WriteString(`\P{Nd}`)
			case 'i', 'I', 'c', 'C', 'w', 'W':
				return nil, fmt.Errorf("regular expression %q: \\%c is not supported", pat, e)
			default:
				b.WriteByte('\\')
				b.WriteByte(e)
			}
		case c == '[' && !class:
			class = true
			b.WriteByte(c)
		case c == ']' && class:
			class = false
			b.WriteByte(c)
		case c == '.' && !class && dotAll:
			b.WriteString(`(?s:.)`)
		case c == '.' && !class:
			b.WriteString(`[^\n\r]`)
		default:
			b.WriteByte(c)
		}
	}
	re, err := regexp.Compile(prefix + b.String())
	if err != nil {
		return nil, fmt.Errorf("regular expression %q: %v", pat, err)
	}
	return re, nil
}
