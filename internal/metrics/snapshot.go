package metrics

import (
	"runtime"
	"sync/atomic"
	"time"
	"unsafe"
)

type snapshot struct {
	cpuPercent  atomic.Uint64
	memUsedMB   atomic.Uint64
	memTotalMB  atomic.Uint64
	memPercent  atomic.Uint64
	diskUsedMB  atomic.Uint64
	diskTotalMB atomic.Uint64
	diskPercent atomic.Uint64
	goroutines  atomic.Uint64
	uptime      atomic.Uint64
}

type SystemSnapshot struct {
	CPUPercent  float64 `json:"cpu_percent"`
	MemUsedMB   int64   `json:"mem_used_mb"`
	MemTotalMB  int64   `json:"mem_total_mb"`
	MemPercent  float64 `json:"mem_percent"`
	DiskUsedMB  int64   `json:"disk_used_mb"`
	DiskTotalMB int64   `json:"disk_total_mb"`
	DiskPercent float64 `json:"disk_percent"`
	Goroutines  int     `json:"goroutines"`
	UptimeSecs  float64 `json:"uptime_seconds"`
}

func storeFloat(addr *atomic.Uint64, v float64) {
	addr.Store(*(*uint64)(unsafe.Pointer(&v)))
}

func loadFloat(addr *atomic.Uint64) float64 {
	v := addr.Load()
	return *(*float64)(unsafe.Pointer(&v))
}

func (r *Registry) GetSystem() SystemSnapshot {
	return SystemSnapshot{
		CPUPercent:  loadFloat(&r.snap.cpuPercent),
		MemUsedMB:   int64(loadFloat(&r.snap.memUsedMB)),
		MemTotalMB:  int64(loadFloat(&r.snap.memTotalMB)),
		MemPercent:  loadFloat(&r.snap.memPercent),
		DiskUsedMB:  int64(loadFloat(&r.snap.diskUsedMB)),
		DiskTotalMB: int64(loadFloat(&r.snap.diskTotalMB)),
		DiskPercent: loadFloat(&r.snap.diskPercent),
		Goroutines:  int(loadFloat(&r.snap.goroutines)),
		UptimeSecs:  loadFloat(&r.snap.uptime),
	}
}

// SetSystem Override SetSystem to also update the snapshot.
func (r *Registry) SetSystem(
	cpu float64,
	memUsed, memTotal int64,
	memPct float64,
	diskUsed, diskTotal int64,
	diskPct float64,
) {
	r.cpuPercent.Set(cpu)
	r.memUsedMB.Set(float64(memUsed))
	r.memTotalMB.Set(float64(memTotal))
	r.memPercent.Set(memPct)
	r.diskUsedMB.Set(float64(diskUsed))
	r.diskTotalMB.Set(float64(diskTotal))
	r.diskPercent.Set(diskPct)
	storeFloat(&r.snap.cpuPercent, cpu)
	storeFloat(&r.snap.memUsedMB, float64(memUsed))
	storeFloat(&r.snap.memTotalMB, float64(memTotal))
	storeFloat(&r.snap.memPercent, memPct)
	storeFloat(&r.snap.diskUsedMB, float64(diskUsed))
	storeFloat(&r.snap.diskTotalMB, float64(diskTotal))
	storeFloat(&r.snap.diskPercent, diskPct)
}

// UpdateRuntime Override UpdateRuntime to also update the snapshot.
func (r *Registry) UpdateRuntime() {
	g := float64(runtime.NumGoroutine())
	u := time.Since(r.startTime).Seconds()
	r.goroutines.Set(g)
	r.uptime.Set(u)
	storeFloat(&r.snap.goroutines, g)
	storeFloat(&r.snap.uptime, u)
}
