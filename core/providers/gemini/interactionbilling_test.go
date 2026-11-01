package gemini

import (
	"fmt"
	"testing"
	"time"
)

func TestInteractionBillingSetClaim(t *testing.T) {
	s := newInteractionBillingSet(2, time.Hour)
	if !s.claim("k", "a") || s.claim("k", "a") {
		t.Fatal("first claim bills, second does not")
	}
	if !s.claim("other", "a") {
		t.Fatal("the same id on another key is a different run")
	}
	if !s.claim("k", "") || !s.claim("k", "") {
		t.Fatal("an empty id cannot be deduplicated and is always billed")
	}
}

func TestInteractionBillingSetEvictsOldest(t *testing.T) {
	s := newInteractionBillingSet(2, time.Hour)
	s.claim("k", "a")
	s.claim("k", "b")
	s.claim("k", "c") // evicts a
	if s.order.Len() != 2 || len(s.entries) != 2 {
		t.Fatalf("set holds %d/%d entries, want 2", s.order.Len(), len(s.entries))
	}
	if !s.claim("k", "a") {
		t.Fatal("an evicted run is billed again")
	}
	if s.claim("k", "c") {
		t.Fatal("a retained run is not billed again")
	}
}

func TestInteractionBillingSetExpires(t *testing.T) {
	now := time.Unix(0, 0)
	s := newInteractionBillingSet(10, time.Hour)
	s.now = func() time.Time { return now }
	s.claim("k", "a")
	now = now.Add(59 * time.Minute)
	if s.claim("k", "a") {
		t.Fatal("still within the ttl")
	}
	now = now.Add(2 * time.Minute)
	if !s.claim("k", "a") {
		t.Fatal("past the ttl the entry no longer suppresses billing")
	}
}

func BenchmarkInteractionBillingSetClaim(b *testing.B) {
	s := newInteractionBillingSet(interactionBillingCapacity, interactionBillingTTL)
	ids := make([]string, 1024)
	for i := range ids {
		ids[i] = fmt.Sprintf("int_%d", i)
	}
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		s.claim("key", ids[i%len(ids)])
		i++
	}
}
