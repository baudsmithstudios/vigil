package collector

// Caller must pass elapsed > 0. A counter rollback leaves that rate at zero.
func applySwapRates(prevIn, prevOut, curIn, curOut uint64, elapsed float64) (inRate, outRate float64) {
	if curIn >= prevIn {
		inRate = float64(curIn-prevIn) / elapsed
	}
	if curOut >= prevOut {
		outRate = float64(curOut-prevOut) / elapsed
	}
	return inRate, outRate
}
