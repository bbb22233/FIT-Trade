package domain

import (
	"errors"
	"math/big"
	"regexp"
	"strings"
)

var decimalPattern = regexp.MustCompile(`^-?(?:0|[1-9][0-9]{0,17})(?:\.[0-9]{0,17}[1-9])?$`)

// Decimal is an exact, canonical wire decimal. It deliberately has no
// conversion to float.
type Decimal struct {
	coefficient big.Int
	scale       int
	text        string
}

func ParseDecimal(s string) (Decimal, error) {
	if len(s) > 38 || !decimalPattern.MatchString(s) || s == "-0" {
		return Decimal{}, errors.New("decimal is not canonical")
	}
	negative := strings.HasPrefix(s, "-")
	unsigned := strings.TrimPrefix(s, "-")
	parts := strings.SplitN(unsigned, ".", 2)
	scale := 0
	digits := parts[0]
	if len(parts) == 2 {
		scale = len(parts[1])
		digits += parts[1]
	}
	var coefficient big.Int
	if _, ok := coefficient.SetString(digits, 10); !ok {
		return Decimal{}, errors.New("invalid decimal")
	}
	if negative {
		coefficient.Neg(&coefficient)
	}
	return Decimal{coefficient: coefficient, scale: scale, text: s}, nil
}

func (d Decimal) String() string { return d.text }
func (d Decimal) Sign() int      { return d.coefficient.Sign() }

func (d Decimal) EqualAbs(other Decimal) bool {
	var left, right big.Int
	left.Abs(&d.coefficient)
	right.Abs(&other.coefficient)
	if d.scale < other.scale {
		left.Mul(&left, pow10(other.scale-d.scale))
	} else if other.scale < d.scale {
		right.Mul(&right, pow10(d.scale-other.scale))
	}
	return left.Cmp(&right) == 0
}

func pow10(n int) *big.Int {
	return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(n)), nil)
}
