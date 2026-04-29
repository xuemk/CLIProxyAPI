package tui

import (
	"strings"
	"testing"
)

func TestRenderLatencyBreakdown(t *testing.T) {
	tests := []struct {
		name         string
		modelStats   map[string]any
		wantEmpty    bool
		wantContains string
	}{
		{
			name:       "no details",
			modelStats: map[string]any{},
			wantEmpty:  true,
		},
		{
			name: "empty details",
			modelStats: map[string]any{
				"details": []any{},
			},
			wantEmpty: true,
		},
		{
			name: "details with zero latency",
			modelStats: map[string]any{
				"details": []any{
					map[string]any{
						"latency_ms": float64(0),
					},
				},
			},
			wantEmpty: true,
		},
		{
			name: "single request with latency",
			modelStats: map[string]any{
				"details": []any{
					map[string]any{
						"latency_ms": float64(1500),
					},
				},
			},
			wantEmpty:    false,
			wantContains: "avg 1500ms  min 1500ms  max 1500ms",
		},
		{
			name: "multiple requests with varying latency",
			modelStats: map[string]any{
				"details": []any{
					map[string]any{
						"latency_ms": float64(100),
					},
					map[string]any{
						"latency_ms": float64(200),
					},
					map[string]any{
						"latency_ms": float64(300),
					},
				},
			},
			wantEmpty:    false,
			wantContains: "avg 200ms  min 100ms  max 300ms",
		},
		{
			name: "mixed valid and invalid latency values",
			modelStats: map[string]any{
				"details": []any{
					map[string]any{
						"latency_ms": float64(500),
					},
					map[string]any{
						"latency_ms": float64(0),
					},
					map[string]any{
						"latency_ms": float64(1500),
					},
				},
			},
			wantEmpty:    false,
			wantContains: "avg 1000ms  min 500ms  max 1500ms",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := usageTabModel{}
			result := m.renderLatencyBreakdown(tt.modelStats)

			if tt.wantEmpty {
				if result != "" {
					t.Errorf("renderLatencyBreakdown() = %q, want empty string", result)
				}
				return
			}

			if result == "" {
				t.Errorf("renderLatencyBreakdown() = empty, want non-empty string")
				return
			}

			if tt.wantContains != "" && !strings.Contains(result, tt.wantContains) {
				t.Errorf("renderLatencyBreakdown() = %q, want to contain %q", result, tt.wantContains)
			}
		})
	}
}

func TestUsageTimeTranslations(t *testing.T) {
	prevLocale := CurrentLocale()
	t.Cleanup(func() {
		SetLocale(prevLocale)
	})

	tests := []struct {
		locale string
		want   string
	}{
		{locale: "en", want: "Time"},
		{locale: "zh", want: "时间"},
	}

	for _, tt := range tests {
		t.Run(tt.locale, func(t *testing.T) {
			SetLocale(tt.locale)
			if got := T("usage_time"); got != tt.want {
				t.Fatalf("T(usage_time) = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAuthTabVisibleFilesFiltersUnauthorized(t *testing.T) {
	model := authTabModel{
		files: []map[string]any{
			{"name": "ok.json", "status_message": ""},
			{"name": "unauth.json", "status_message": "unauthorized"},
			{"name": "quota.json", "status_message": "quota exhausted"},
		},
		showUnauthorizedOnly: true,
	}

	visible := model.visibleFiles()
	if len(visible) != 1 {
		t.Fatalf("visibleFiles len = %d, want 1", len(visible))
	}
	if got := getAnyString(visible[0], "name"); got != "unauth.json" {
		t.Fatalf("visibleFiles[0].name = %q, want %q", got, "unauth.json")
	}
}

func TestAuthTabToggleAllVisibleSelectionsHonoursUnauthorizedFilter(t *testing.T) {
	model := authTabModel{
		files: []map[string]any{
			{"name": "ok.json", "status_message": ""},
			{"name": "unauth-a.json", "status_message": "unauthorized"},
			{"name": "unauth-b.json", "status_message": "unauthorized"},
		},
		selected:             make(map[string]struct{}),
		showUnauthorizedOnly: true,
	}

	model.toggleAllVisibleSelections()
	selected := model.selectedNames()
	if len(selected) != 2 {
		t.Fatalf("selected len = %d, want 2", len(selected))
	}
	if selected[0] != "unauth-a.json" || selected[1] != "unauth-b.json" {
		t.Fatalf("selected names = %#v, want unauthorized-only entries", selected)
	}

	model.toggleAllVisibleSelections()
	if len(model.selectedNames()) != 0 {
		t.Fatalf("expected selections to be cleared on second toggle")
	}
}
