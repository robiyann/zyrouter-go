package handlers

import (
	"testing"

	"zyrouter/backend/internal/tracing"
)

func TestSummarizeLatency(t *testing.T) {
	result := summarizeLatency([]tracing.Span{
		{DurationMs: 100, Status: "200"},
		{DurationMs: 200, Status: "200"},
		{DurationMs: 900, Status: "502"},
		{DurationMs: 400, Status: "200"},
	})
	if result.Count != 4 || result.Errors != 1 {
		t.Fatalf("unexpected latency counts: %+v", result)
	}
	if result.AverageMs != 400 || result.P50Ms != 200 || result.P95Ms != 900 || result.P99Ms != 900 {
		t.Fatalf("unexpected latency summary: %+v", result)
	}
}

func TestProxyPoolEndpointCount(t *testing.T) {
	count, poolType := proxyPoolEndpointCount(`{"type":"socks5","urls":["http://one","http://two"]}`)
	if count != 2 || poolType != "socks5" {
		t.Fatalf("unexpected multi-endpoint pool: count=%d type=%q", count, poolType)
	}

	count, poolType = proxyPoolEndpointCount(`{"proxyUrl":"https://relay.example","type":"vercel"}`)
	if count != 1 || poolType != "vercel" {
		t.Fatalf("unexpected single-endpoint pool: count=%d type=%q", count, poolType)
	}
}
