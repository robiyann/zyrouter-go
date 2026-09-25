package handlers

import (
	"encoding/json"
	"math"
	"net/http"
	"runtime"
	"sort"
	"time"

	"zyrouter/backend/internal/db"
	"zyrouter/backend/internal/handlerutil"
	"zyrouter/backend/internal/tracing"
)

// HandleSystemOverview returns compact, admin-only runtime telemetry for the
// dashboard overview. Values are process-local and do not expose provider
// credentials or request payloads.
func HandleSystemOverview(repo *db.Repo) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var mem runtime.MemStats
		runtime.ReadMemStats(&mem)

		spans := tracing.Recent(0)
		latency := summarizeLatency(spans)
		pools := summarizeProxyPools(repo)

		handlerutil.WriteJSON(w, http.StatusOK, map[string]any{
			"generatedAt": time.Now().UTC().Format(time.RFC3339Nano),
			"uptimeSecs":  tracing.UptimeSecs(),
			"runtime": map[string]any{
				"goVersion":  runtime.Version(),
				"os":         runtime.GOOS,
				"arch":       runtime.GOARCH,
				"cpuCores":   runtime.NumCPU(),
				"goroutines": runtime.NumGoroutine(),
			},
			"memory": map[string]any{
				"heapAllocBytes": mem.HeapAlloc,
				"heapSysBytes":   mem.HeapSys,
				"sysBytes":       mem.Sys,
				"heapAllocMb":    bytesToMB(mem.HeapAlloc),
				"heapSysMb":      bytesToMB(mem.HeapSys),
				"sysMb":          bytesToMB(mem.Sys),
				"gcCycles":       mem.NumGC,
			},
			"latency":    latency,
			"proxyPools": pools,
		})
	}
}

type latencySummary struct {
	Count      int     `json:"count"`
	Errors     int     `json:"errors"`
	AverageMs  float64 `json:"averageMs"`
	P50Ms      float64 `json:"p50Ms"`
	P95Ms      float64 `json:"p95Ms"`
	P99Ms      float64 `json:"p99Ms"`
	WindowSize int     `json:"windowSize"`
}

func summarizeLatency(spans []tracing.Span) latencySummary {
	if len(spans) == 0 {
		return latencySummary{}
	}

	durations := make([]int64, 0, len(spans))
	var total int64
	errors := 0
	for _, span := range spans {
		if span.DurationMs < 0 {
			continue
		}
		durations = append(durations, span.DurationMs)
		total += span.DurationMs
		if span.Status != "200" {
			errors++
		}
	}
	if len(durations) == 0 {
		return latencySummary{}
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	return latencySummary{
		Count:      len(durations),
		Errors:     errors,
		AverageMs:  float64(total) / float64(len(durations)),
		P50Ms:      percentileDuration(durations, 0.50),
		P95Ms:      percentileDuration(durations, 0.95),
		P99Ms:      percentileDuration(durations, 0.99),
		WindowSize: len(durations),
	}
}

func percentileDuration(sorted []int64, ratio float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	index := int(math.Ceil(float64(len(sorted))*ratio)) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return float64(sorted[index])
}

type proxyPoolSummary struct {
	TotalPools      int            `json:"totalPools"`
	ActivePools     int            `json:"activePools"`
	TotalEndpoints  int            `json:"totalEndpoints"`
	ActiveEndpoints int            `json:"activeEndpoints"`
	ByType          map[string]int `json:"byType"`
}

func summarizeProxyPools(repo *db.Repo) proxyPoolSummary {
	result := proxyPoolSummary{ByType: map[string]int{}}
	if repo == nil {
		return result
	}
	pools, err := repo.GetProxyPools()
	if err != nil {
		return result
	}
	result.TotalPools = len(pools)
	for _, pool := range pools {
		if pool == nil {
			continue
		}
		if pool.IsActive == 1 {
			result.ActivePools++
		}
		endpoints, poolType := proxyPoolEndpointCount(pool.Data)
		result.TotalEndpoints += endpoints
		if pool.IsActive == 1 {
			result.ActiveEndpoints += endpoints
		}
		result.ByType[poolType] += endpoints
	}
	return result
}

func proxyPoolEndpointCount(raw string) (int, string) {
	var data map[string]any
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return 0, "http"
	}
	typeName := "http"
	if value, ok := data["type"].(string); ok && value != "" {
		typeName = value
	}
	if urls, ok := data["urls"].([]any); ok {
		count := 0
		for _, url := range urls {
			if value, ok := url.(string); ok && value != "" {
				count++
			}
		}
		if count > 0 {
			return count, typeName
		}
	}
	if value, ok := data["proxyUrl"].(string); ok && value != "" {
		return 1, typeName
	}
	return 0, typeName
}

func bytesToMB(value uint64) float64 {
	return float64(value) / (1024 * 1024)
}
