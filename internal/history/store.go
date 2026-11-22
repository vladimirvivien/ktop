package history

import (
	"math"
	"sort"
	"sync"
	"time"

	"github.com/vladimirvivien/ktop/internal/snapshot"
)

// DefaultMaxSamples stores up to 1 hour of samples at 2-second intervals.
const DefaultMaxSamples = 1800

// SamplePoint captures one timestamped cluster and per-node telemetry point.
type SamplePoint struct {
	Timestamp  time.Time
	CPUPct     float64
	MemPct     float64
	NetRxBps   float64
	NetTxBps   float64
	DiskRdBps  float64
	DiskWrBps  float64
	NodeCPUPct map[string]float64
	NodeMemPct map[string]float64
}

// SeriesSet holds extracted chart series and peaks for a requested time window.
type SeriesSet struct {
	CPU      []float64
	Mem      []float64
	NetRx    []float64
	NetTx    []float64
	DiskRd   []float64
	DiskWr   []float64
	PeakCPU  float64
	PeakMem  float64
	PeakRx   float64
	PeakTx   float64
	PeakRd   float64
	PeakWr   float64
	NodeCPU  map[string][]float64
	NodeMem  map[string][]float64
	TopNodes []string
}

// Store is a thread-safe, renderer-agnostic bounded history buffer fed by Snapshots.
type Store struct {
	mu         sync.RWMutex
	maxSamples int
	points     []SamplePoint
}

// NewStore creates a bounded Store keeping at most maxSamples entries.
func NewStore(maxSamples int) *Store {
	if maxSamples <= 0 {
		maxSamples = DefaultMaxSamples
	}
	return &Store{
		maxSamples: maxSamples,
		points:     make([]SamplePoint, 0, min(maxSamples, 256)),
	}
}

// Push appends a SamplePoint, evicting the oldest sample when capacity is reached.
func (s *Store) Push(pt SamplePoint) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.points) >= s.maxSamples {
		copy(s.points, s.points[1:])
		s.points[len(s.points)-1] = pt
		return
	}
	s.points = append(s.points, pt)
}

// PushSnapshot converts an immutable Snapshot into a SamplePoint and appends it.
func (s *Store) PushSnapshot(snap *snapshot.Snapshot) {
	if snap == nil {
		return
	}
	ts := snap.Timestamp
	if ts.IsZero() {
		ts = time.Now()
	}
	pt := SamplePoint{
		Timestamp:  ts,
		CPUPct:     ratioPct(float64(snap.Summary.UsageCPUMillis), float64(snap.Summary.AllocatableCPUMillis)),
		MemPct:     ratioPct(float64(snap.Summary.UsageMemoryBytes), float64(snap.Summary.AllocatableMemoryBytes)),
		NetRxBps:   snap.Summary.NetworkRxRateBytesPerS,
		NetTxBps:   snap.Summary.NetworkTxRateBytesPerS,
		DiskRdBps:  snap.Summary.DiskReadRateBytesPerS,
		DiskWrBps:  snap.Summary.DiskWriteRateBytesPerS,
		NodeCPUPct: make(map[string]float64, len(snap.Nodes)),
		NodeMemPct: make(map[string]float64, len(snap.Nodes)),
	}
	for _, n := range snap.Nodes {
		pt.NodeCPUPct[n.Name] = ratioPct(float64(n.UsageCPUMillis), float64(n.AllocatableCPUMillis))
		pt.NodeMemPct[n.Name] = ratioPct(float64(n.UsageMemoryBytes), float64(n.AllocatableMemoryBytes))
	}
	s.Push(pt)
}

// Len returns the number of buffered samples.
func (s *Store) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.points)
}

// WindowSlice returns a copy of samples within the trailing duration window.
func (s *Store) WindowSlice(window time.Duration) []SamplePoint {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.points) == 0 {
		return nil
	}
	if window <= 0 {
		out := make([]SamplePoint, len(s.points))
		copy(out, s.points)
		return out
	}
	latest := s.points[len(s.points)-1].Timestamp
	cutoff := latest.Add(-window)
	idx := sort.Search(len(s.points), func(i int) bool {
		return !s.points[i].Timestamp.Before(cutoff)
	})
	if idx >= len(s.points) {
		idx = len(s.points) - 1
	}
	out := make([]SamplePoint, len(s.points)-idx)
	copy(out, s.points[idx:])
	return out
}

// Extract builds ready-to-render series slices and peaks for the requested window.
func (s *Store) Extract(window time.Duration, maxTopNodes int) SeriesSet {
	pts := s.WindowSlice(window)
	out := SeriesSet{
		CPU:     make([]float64, len(pts)),
		Mem:     make([]float64, len(pts)),
		NetRx:   make([]float64, len(pts)),
		NetTx:   make([]float64, len(pts)),
		DiskRd:  make([]float64, len(pts)),
		DiskWr:  make([]float64, len(pts)),
		NodeCPU: make(map[string][]float64),
		NodeMem: make(map[string][]float64),
	}
	if len(pts) == 0 {
		return out
	}

	if maxTopNodes <= 0 {
		maxTopNodes = 4
	}
	latest := pts[len(pts)-1]
	type nodeRank struct {
		name  string
		score float64
	}
	ranks := make([]nodeRank, 0, len(latest.NodeCPUPct))
	for name, cpu := range latest.NodeCPUPct {
		ranks = append(ranks, nodeRank{name: name, score: cpu + latest.NodeMemPct[name]*0.5})
	}
	sort.Slice(ranks, func(i, j int) bool {
		if ranks[i].score != ranks[j].score {
			return ranks[i].score > ranks[j].score
		}
		return ranks[i].name < ranks[j].name
	})
	limit := min(maxTopNodes, len(ranks))
	out.TopNodes = make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		name := ranks[i].name
		out.TopNodes = append(out.TopNodes, name)
		out.NodeCPU[name] = make([]float64, len(pts))
		out.NodeMem[name] = make([]float64, len(pts))
	}

	for i, pt := range pts {
		out.CPU[i] = pt.CPUPct
		out.Mem[i] = pt.MemPct
		out.NetRx[i] = pt.NetRxBps
		out.NetTx[i] = pt.NetTxBps
		out.DiskRd[i] = pt.DiskRdBps
		out.DiskWr[i] = pt.DiskWrBps

		out.PeakCPU = math.Max(out.PeakCPU, pt.CPUPct)
		out.PeakMem = math.Max(out.PeakMem, pt.MemPct)
		out.PeakRx = math.Max(out.PeakRx, pt.NetRxBps)
		out.PeakTx = math.Max(out.PeakTx, pt.NetTxBps)
		out.PeakRd = math.Max(out.PeakRd, pt.DiskRdBps)
		out.PeakWr = math.Max(out.PeakWr, pt.DiskWrBps)

		for _, name := range out.TopNodes {
			out.NodeCPU[name][i] = pt.NodeCPUPct[name]
			out.NodeMem[name][i] = pt.NodeMemPct[name]
		}
	}
	return out
}

func ratioPct(used, total float64) float64 {
	if total <= 0 {
		return 0
	}
	return math.Max(0, math.Min(100, (used/total)*100))
}
