package collector

import (
	"log"
	"time"

	"github.com/shirou/gopsutil/v3/net"
)

type netCollector struct {
	prev     map[string]net.IOCountersStat
	prevTime time.Time
}

func newNetCollector() *netCollector {
	return &netCollector{prev: make(map[string]net.IOCountersStat)}
}

func (c *netCollector) collect() []NetSnapshot {
	counters, err := net.IOCounters(true) // per-interface
	if err != nil {
		log.Printf("network collection error: %v", err)
		return nil
	}
	now := time.Now()
	elapsed := now.Sub(c.prevTime).Seconds()

	var out []NetSnapshot
	for _, iface := range counters {
		if !shouldCollectInterface(iface.Name) {
			continue
		}
		snap := NetSnapshot{
			Interface: iface.Name,
			BytesSent: iface.BytesSent,
			BytesRecv: iface.BytesRecv,
			ErrIn:     iface.Errin,
			ErrOut:    iface.Errout,
			DropIn:    iface.Dropin,
			DropOut:   iface.Dropout,
		}
		if prev, ok := c.prev[iface.Name]; ok && elapsed > 0 {
			applyRates(&snap, prev, iface, elapsed)
		}
		out = append(out, snap)
		c.prev[iface.Name] = iface
	}
	c.prevTime = now
	return out
}

// applyRates writes per-second send/recv/err/drop rates into snap from counter deltas.
func applyRates(snap *NetSnapshot, prev, curr net.IOCountersStat, elapsed float64) {
	if curr.BytesSent >= prev.BytesSent {
		snap.SendRate = float64(curr.BytesSent-prev.BytesSent) / elapsed
	}
	if curr.BytesRecv >= prev.BytesRecv {
		snap.RecvRate = float64(curr.BytesRecv-prev.BytesRecv) / elapsed
	}
	currDrops := curr.Dropin + curr.Dropout
	prevDrops := prev.Dropin + prev.Dropout
	if currDrops >= prevDrops {
		snap.DropRate = float64(currDrops-prevDrops) / elapsed
	}
	currErrors := curr.Errin + curr.Errout
	prevErrors := prev.Errin + prev.Errout
	if currErrors >= prevErrors {
		snap.ErrRate = float64(currErrors-prevErrors) / elapsed
	}
}
