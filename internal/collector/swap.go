package collector

// Caller must pass elapsed > 0. If a counter goes backwards, that rate stays zero.
func applySwapRates(prevIn, prevOut, curIn, curOut uint64, elapsed float64) (inRate, outRate float64) {
	if curIn >= prevIn {
		inRate = float64(curIn-prevIn) / elapsed
	}
	if curOut >= prevOut {
		outRate = float64(curOut-prevOut) / elapsed
	}
	return inRate, outRate
}
