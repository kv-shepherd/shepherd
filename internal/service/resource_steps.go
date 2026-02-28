package service

import "math"

const halfStepEpsilon = 1e-9

// IsHalfStep reports whether v is a positive multiple of 0.5.
// Valid examples: 0.5, 1.0, 1.5, 2.0.
func IsHalfStep(v float64) bool {
	if v <= 0 {
		return false
	}
	doubled := v * 2
	return math.Abs(doubled-math.Round(doubled)) < halfStepEpsilon
}
