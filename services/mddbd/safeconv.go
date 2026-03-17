package main

import "math"

// safeInt32 converts an int to int32 with overflow clamping.
func safeInt32(v int) int32 {
	if v > math.MaxInt32 {
		return math.MaxInt32
	}
	if v < math.MinInt32 {
		return math.MinInt32
	}
	return int32(v) // #nosec G115 -- bounds checked above
}

// safeUint16 converts an int to uint16 with overflow clamping.
func safeUint16(v int) uint16 {
	if v > math.MaxUint16 {
		return math.MaxUint16
	}
	if v < 0 {
		return 0
	}
	return uint16(v) // #nosec G115 -- bounds checked above
}
