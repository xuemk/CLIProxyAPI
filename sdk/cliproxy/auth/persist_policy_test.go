package auth

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

type countingStore struct {
	saveCount atomic.Int32
}

func (s *countingStore) List(context.Context) ([]*Auth, error) { return nil, nil }

func (s *countingStore) Save(context.Context, *Auth) (string, error) {
	s.saveCount.Add(1)
	return "", nil
}

func (s *countingStore) Delete(context.Context, string) error { return nil }

type fileStore struct {
	baseDir string
}

func (s *fileStore) List(context.Context) ([]*Auth, error) { return nil, nil }

func (s *fileStore) Save(_ context.Context, auth *Auth) (string, error) {
	if auth == nil {
		return "", nil
	}
	if err := os.MkdirAll(s.baseDir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(s.baseDir, auth.FileName)
	data, err := json.Marshal(auth.Metadata)
	if err != nil {
		return "", err
	}
	if errWrite := os.WriteFile(path, data, 0o600); errWrite != nil {
		return "", errWrite
	}
	return path, nil
}

func (s *fileStore) Delete(_ context.Context, id string) error {
	return os.Remove(filepath.Join(s.baseDir, id))
}

func TestWithSkipPersist_DisablesUpdatePersistence(t *testing.T) {
	store := &countingStore{}
	mgr := NewManager(store, nil, nil)
	auth := &Auth{
		ID:       "auth-1",
		Provider: "antigravity",
		Metadata: map[string]any{"type": "antigravity"},
	}

	if _, err := mgr.Update(context.Background(), auth); err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	if got := store.saveCount.Load(); got != 1 {
		t.Fatalf("expected 1 Save call, got %d", got)
	}

	ctxSkip := WithSkipPersist(context.Background())
	if _, err := mgr.Update(ctxSkip, auth); err != nil {
		t.Fatalf("Update(skipPersist) returned error: %v", err)
	}
	if got := store.saveCount.Load(); got != 1 {
		t.Fatalf("expected Save call count to remain 1, got %d", got)
	}
}

func TestWithSkipPersist_DisablesRegisterPersistence(t *testing.T) {
	store := &countingStore{}
	mgr := NewManager(store, nil, nil)
	auth := &Auth{
		ID:       "auth-1",
		Provider: "antigravity",
		Metadata: map[string]any{"type": "antigravity"},
	}

	if _, err := mgr.Register(WithSkipPersist(context.Background()), auth); err != nil {
		t.Fatalf("Register(skipPersist) returned error: %v", err)
	}
	if got := store.saveCount.Load(); got != 0 {
		t.Fatalf("expected 0 Save calls, got %d", got)
	}
}

func TestMarkResult_PersistsPriorityPenaltyForRestrictedStatus(t *testing.T) {
	tempDir := t.TempDir()
	store := &fileStore{baseDir: tempDir}

	mgr := NewManager(store, nil, nil)
	record := &Auth{
		ID:       "auth.json",
		FileName: "auth.json",
		Provider: "claude",
		Attributes: map[string]string{
			"priority": "3",
		},
		Metadata: map[string]any{
			"type":     "claude",
			"priority": 3,
		},
	}
	if _, errRegister := mgr.Register(context.Background(), record); errRegister != nil {
		t.Fatalf("register auth: %v", errRegister)
	}

	mgr.MarkResult(context.Background(), Result{
		AuthID:  "auth.json",
		Model:   "gpt-5.4",
		Success: false,
		Error: &Error{
			HTTPStatus: 429,
			Message:    "quota exhausted",
		},
	})

	updated, ok := mgr.GetByID("auth.json")
	if !ok || updated == nil {
		t.Fatalf("expected auth to remain registered")
	}
	if got := updated.Attributes["priority"]; got != "2" {
		t.Fatalf("runtime priority = %q, want %q", got, "2")
	}

	data, errRead := os.ReadFile(filepath.Join(tempDir, "auth.json"))
	if errRead != nil {
		t.Fatalf("read persisted auth: %v", errRead)
	}
	var persisted map[string]any
	if errUnmarshal := json.Unmarshal(data, &persisted); errUnmarshal != nil {
		t.Fatalf("unmarshal persisted auth: %v", errUnmarshal)
	}
	if got, ok := persisted["priority"].(float64); !ok || int(got) != 2 {
		t.Fatalf("persisted priority = %#v, want 2", persisted["priority"])
	}
	if got, ok := persisted[persistentPriorityOriginalKey].(float64); !ok || int(got) != 3 {
		t.Fatalf("persisted original priority = %#v, want 3", persisted[persistentPriorityOriginalKey])
	}
	if got, ok := persisted[persistentPriorityPenaltyKey].(float64); !ok || int(got) != 1 {
		t.Fatalf("persisted priority penalty = %#v, want 1", persisted[persistentPriorityPenaltyKey])
	}
}

func TestMarkResult_DoesNotPenaltyUnauthorized(t *testing.T) {
	tempDir := t.TempDir()
	store := &fileStore{baseDir: tempDir}

	mgr := NewManager(store, nil, nil)
	record := &Auth{
		ID:       "auth.json",
		FileName: "auth.json",
		Provider: "claude",
		Attributes: map[string]string{
			"priority": "3",
		},
		Metadata: map[string]any{
			"type":     "claude",
			"priority": 3,
		},
	}
	if _, errRegister := mgr.Register(context.Background(), record); errRegister != nil {
		t.Fatalf("register auth: %v", errRegister)
	}

	mgr.MarkResult(context.Background(), Result{
		AuthID:  "auth.json",
		Model:   "gpt-5.4",
		Success: false,
		Error: &Error{
			HTTPStatus: 401,
			Message:    "unauthorized",
		},
	})

	updated, ok := mgr.GetByID("auth.json")
	if !ok || updated == nil {
		t.Fatalf("expected auth to remain registered")
	}
	if got := updated.Attributes["priority"]; got != "3" {
		t.Fatalf("runtime priority = %q, want %q", got, "3")
	}

	data, errRead := os.ReadFile(filepath.Join(tempDir, "auth.json"))
	if errRead != nil {
		t.Fatalf("read persisted auth: %v", errRead)
	}
	var persisted map[string]any
	if errUnmarshal := json.Unmarshal(data, &persisted); errUnmarshal != nil {
		t.Fatalf("unmarshal persisted auth: %v", errUnmarshal)
	}
	if got, ok := persisted["priority"].(float64); !ok || int(got) != 3 {
		t.Fatalf("persisted priority = %#v, want 3", persisted["priority"])
	}
	if _, ok := persisted[persistentPriorityPenaltyKey]; ok {
		t.Fatalf("did not expect %s to be persisted for 401", persistentPriorityPenaltyKey)
	}
}

func TestParseCodexQuotaFiveHourRemaining_PreservesZeroUsedPercent(t *testing.T) {
	body := []byte(`{
		"rate_limit": {
			"primary_window": {
				"limit_window_seconds": 18000,
				"used_percent": 0
			},
			"secondary_window": {
				"limit_window_seconds": 604800,
				"used_percent": 0
			}
		}
	}`)

	remaining, ok := parseCodexQuotaFiveHourRemaining(body)
	if !ok {
		t.Fatal("parseCodexQuotaFiveHourRemaining() ok = false, want true")
	}
	if remaining != 100 {
		t.Fatalf("remaining = %v, want 100", remaining)
	}
}

func TestApplyCodexQuotaPriorityState_CyclesNonFreePriority(t *testing.T) {
	auth := &Auth{
		ID:       "codex-plus.json",
		Provider: "codex",
		Attributes: map[string]string{
			"plan_type": "plus",
			"priority":  "7",
		},
		Metadata: map[string]any{
			"type":     "codex",
			"priority": 7,
		},
	}

	if changed := applyCodexQuotaPriorityState(auth, false); !changed {
		t.Fatal("applyCodexQuotaPriorityState(exhausted) changed = false, want true")
	}
	if got := auth.Attributes["priority"]; got != "-9999" {
		t.Fatalf("exhausted priority = %q, want -9999", got)
	}
	if got, ok := anyInt(auth.Metadata[persistentPriorityOriginalKey]); !ok || got != 7 {
		t.Fatalf("original priority = %#v, want 7", auth.Metadata[persistentPriorityOriginalKey])
	}
	if got, ok := anyInt(auth.Metadata[persistentPriorityPenaltyKey]); !ok || got != 10006 {
		t.Fatalf("priority penalty = %#v, want 10006", auth.Metadata[persistentPriorityPenaltyKey])
	}

	if changed := applyCodexQuotaPriorityState(auth, true); !changed {
		t.Fatal("applyCodexQuotaPriorityState(recovered) changed = false, want true")
	}
	if got := auth.Attributes["priority"]; got != "7" {
		t.Fatalf("recovered priority = %q, want 7", got)
	}
	if _, ok := auth.Metadata[persistentPriorityPenaltyKey]; ok {
		t.Fatalf("did not expect %s after recovery", persistentPriorityPenaltyKey)
	}
}

func TestShouldPollCodexQuotaPriority_SkipsFree(t *testing.T) {
	freeAuth := &Auth{
		ID:       "codex-free.json",
		Provider: "codex",
		Attributes: map[string]string{
			"plan_type": "free",
		},
		Metadata: map[string]any{
			"account_id": "account-free",
		},
	}
	if shouldPollCodexQuotaPriority(freeAuth) {
		t.Fatal("shouldPollCodexQuotaPriority(free) = true, want false")
	}

	plusAuth := freeAuth.Clone()
	plusAuth.ID = "codex-plus.json"
	plusAuth.Attributes["plan_type"] = "plus"
	plusAuth.Metadata["account_id"] = "account-plus"
	if !shouldPollCodexQuotaPriority(plusAuth) {
		t.Fatal("shouldPollCodexQuotaPriority(plus) = false, want true")
	}
}
