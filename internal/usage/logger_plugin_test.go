package usage

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	coreusage "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/usage"
)

func TestRequestStatisticsRecordIncludesLatency(t *testing.T) {
	stats := NewRequestStatistics()
	stats.Record(context.Background(), coreusage.Record{
		APIKey:      "test-key",
		Model:       "gpt-5.4",
		RequestedAt: time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC),
		Latency:     1500 * time.Millisecond,
		Detail: coreusage.Detail{
			InputTokens:  10,
			OutputTokens: 20,
			TotalTokens:  30,
		},
	})

	snapshot := stats.Snapshot()
	details := snapshot.APIs["test-key"].Models["gpt-5.4"].Details
	if len(details) != 1 {
		t.Fatalf("details len = %d, want 1", len(details))
	}
	if details[0].LatencyMs != 1500 {
		t.Fatalf("latency_ms = %d, want 1500", details[0].LatencyMs)
	}
}

func TestRequestStatisticsMergeSnapshotDedupIgnoresLatency(t *testing.T) {
	stats := NewRequestStatistics()
	timestamp := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	first := StatisticsSnapshot{
		APIs: map[string]APISnapshot{
			"test-key": {
				Models: map[string]ModelSnapshot{
					"gpt-5.4": {
						Details: []RequestDetail{{
							Timestamp: timestamp,
							LatencyMs: 0,
							Source:    "user@example.com",
							AuthIndex: "0",
							Tokens: TokenStats{
								InputTokens:  10,
								OutputTokens: 20,
								TotalTokens:  30,
							},
						}},
					},
				},
			},
		},
	}
	second := StatisticsSnapshot{
		APIs: map[string]APISnapshot{
			"test-key": {
				Models: map[string]ModelSnapshot{
					"gpt-5.4": {
						Details: []RequestDetail{{
							Timestamp: timestamp,
							LatencyMs: 2500,
							Source:    "user@example.com",
							AuthIndex: "0",
							Tokens: TokenStats{
								InputTokens:  10,
								OutputTokens: 20,
								TotalTokens:  30,
							},
						}},
					},
				},
			},
		},
	}

	result := stats.MergeSnapshot(first)
	if result.Added != 1 || result.Skipped != 0 {
		t.Fatalf("first merge = %+v, want added=1 skipped=0", result)
	}

	result = stats.MergeSnapshot(second)
	if result.Added != 0 || result.Skipped != 1 {
		t.Fatalf("second merge = %+v, want added=0 skipped=1", result)
	}

	snapshot := stats.Snapshot()
	details := snapshot.APIs["test-key"].Models["gpt-5.4"].Details
	if len(details) != 1 {
		t.Fatalf("details len = %d, want 1", len(details))
	}
}

func TestStatisticsPersistence_LoadsAndFlushesSnapshot(t *testing.T) {
	stats := NewRequestStatistics()
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")
	persistPath := filepath.Join(tempDir, statisticsPersistenceFileName)

	initial := statisticsPersistencePayload{
		Version:    1,
		ExportedAt: time.Now().UTC(),
		Usage: StatisticsSnapshot{
			APIs: map[string]APISnapshot{
				"loaded-key": {
					Models: map[string]ModelSnapshot{
						"gpt-5.4": {
							Details: []RequestDetail{{
								Timestamp: time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC),
								Source:    "loaded@example.com",
								AuthIndex: "0",
								Tokens: TokenStats{
									InputTokens:  10,
									OutputTokens: 5,
									TotalTokens:  15,
								},
							}},
						},
					},
				},
			},
		},
	}
	raw, errMarshal := json.Marshal(initial)
	if errMarshal != nil {
		t.Fatalf("marshal initial payload: %v", errMarshal)
	}
	if errWrite := os.WriteFile(persistPath, raw, 0o600); errWrite != nil {
		t.Fatalf("write initial payload: %v", errWrite)
	}

	if errStart := StartStatisticsPersistence(configPath, stats); errStart != nil {
		t.Fatalf("StartStatisticsPersistence() error = %v", errStart)
	}

	loaded := stats.Snapshot()
	if loaded.TotalRequests != 1 {
		_ = ShutdownStatisticsPersistence()
		t.Fatalf("loaded total_requests = %d, want 1", loaded.TotalRequests)
	}

	stats.Record(context.Background(), coreusage.Record{
		APIKey:      "loaded-key",
		Model:       "gpt-5.4",
		RequestedAt: time.Date(2026, 3, 20, 12, 5, 0, 0, time.UTC),
		Detail: coreusage.Detail{
			InputTokens:  7,
			OutputTokens: 8,
			TotalTokens:  15,
		},
	})

	if errStop := ShutdownStatisticsPersistence(); errStop != nil {
		t.Fatalf("ShutdownStatisticsPersistence() error = %v", errStop)
	}

	data, errRead := os.ReadFile(persistPath)
	if errRead != nil {
		t.Fatalf("read persisted payload: %v", errRead)
	}

	var persisted statisticsPersistencePayload
	if errUnmarshal := json.Unmarshal(data, &persisted); errUnmarshal != nil {
		t.Fatalf("unmarshal persisted payload: %v", errUnmarshal)
	}
	if persisted.Usage.TotalRequests != 2 {
		t.Fatalf("persisted total_requests = %d, want 2", persisted.Usage.TotalRequests)
	}
	details := persisted.Usage.APIs["loaded-key"].Models["gpt-5.4"].Details
	if len(details) != 2 {
		t.Fatalf("persisted details len = %d, want 2", len(details))
	}
}
