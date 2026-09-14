package repository

import (
	"strings"
	"testing"

	"ws-rat/internal/model"
)

func TestStatusUpdateQueryUpdatesLastSeenOnlyWhenOnline(t *testing.T) {
	if query := statusUpdateQuery(model.StatusOnline); !strings.Contains(query, "last_seen = NOW()") {
		t.Fatalf("online query must update last_seen: %s", query)
	}
	if query := statusUpdateQuery(model.StatusOffline); strings.Contains(query, "last_seen") {
		t.Fatalf("offline query must preserve last_seen: %s", query)
	}
}
