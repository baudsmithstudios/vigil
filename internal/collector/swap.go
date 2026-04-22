package collector

// applySwapRates computes swap in/out byte rates per second from cumulative
// counters. Counter rollback is treated as unavailable data for the interval.
func applySwapRates(prevIn, prevOut, curIn, curOut uint64, elapsed float64) (inRate, outRate float64) {
	if elapsed <= 0 {
		return 0, 0
	}
	if curIn >= prevIn {
		inRate = float64(curIn-prevIn) / elapsed
	}
	if curOut >= prevOut {
		outRate = float64(curOut-prevOut) / elapsed
	}
	return inRate, outRate
}
