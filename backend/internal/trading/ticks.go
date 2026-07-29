package trading

import (
	"fmt"
	"math"
	"strings"
)

const (
	tickScale       = 10_000_000_000 // 1e10
	tickScaleDigits = 10
)

func parseTicks(s string) (int64, error) {
	if s == "" {
		return 0, fmt.Errorf("parseTicks: empty string")
	}

	negative := false
	rest := s
	if rest[0] == '-' || rest[0] == '+' {
		negative = rest[0] == '-'
		rest = rest[1:]
	}

	intPart, fracPart := rest, ""
	if dot := strings.IndexByte(rest, '.'); dot >= 0 {
		intPart, fracPart = rest[:dot], rest[dot+1:]
	}

	if len(fracPart) > tickScaleDigits {
		return 0, fmt.Errorf("parseTicks: more than %d fractional digits in %q", tickScaleDigits, s)
	}

	intValue, err := digitsToInt(intPart)
	if err != nil {
		return 0, fmt.Errorf("parseTicks: %w", err)
	}
	fracValue, err := digitsToInt(fracPart)
	if err != nil {
		return 0, fmt.Errorf("parseTicks: %w", err)
	}
	for i := len(fracPart); i < tickScaleDigits; i++ {
		fracValue *= 10
	}

	// Without this, an absurdly large digit string (a stray extra zero, a
	// bad paste) would silently overflow int64 during the multiply below
	// and wrap around to a garbage -- possibly negative -- tick value
	// instead of being rejected.
	if intValue > math.MaxInt64/tickScale {
		return 0, fmt.Errorf("parseTicks: %q out of range", s)
	}
	ticks := intValue*tickScale + fracValue
	if negative {
		ticks = -ticks
	}
	return ticks, nil
}

func digitsToInt(digits string) (int64, error) {
	var value int64
	for _, c := range digits {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("invalid character %q", c)
		}
		d := int64(c - '0')
		if value > (math.MaxInt64-d)/10 {
			return 0, fmt.Errorf("value out of range")
		}
		value = value*10 + d
	}
	return value, nil
}

func formatTicks(ticks int64) string {
	negative := ticks < 0
	abs := ticks
	if negative {
		abs = -abs
	}
	intPart := abs / tickScale
	fracPart := abs % tickScale
	sign := ""
	if negative {
		sign = "-"
	}
	return fmt.Sprintf("%s%d.%0*d", sign, intPart, tickScaleDigits, fracPart)
}
