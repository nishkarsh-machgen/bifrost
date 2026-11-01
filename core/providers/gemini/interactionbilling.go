package gemini

import (
	"container/list"
	"sync"
	"time"
)

// Interactions keep their usage on the stored resource, so every read of a finished interaction (a
// repeated GET poll, a stream resumed with last_event_id) reports the run's full usage again. Only
// the first read that reports it is billed; later ones keep the record but drop the usage.
//
// The set is per process: a cluster can still bill a re-read that lands on another node once more.
// Entries are keyed by provider key id and interaction id (well under 200 B each), capped at
// interactionBillingCapacity with least-recently-billed eviction, and expire after
// interactionBillingTTL, so the set never holds more than a few MB.
const (
	interactionBillingCapacity = 10000
	interactionBillingTTL      = 24 * time.Hour
)

var interactionsBilled = newInteractionBillingSet(interactionBillingCapacity, interactionBillingTTL)

type interactionBillingEntry struct {
	key      string
	billedAt time.Time
}

// interactionBillingSet records which interactions have already had their usage billed.
type interactionBillingSet struct {
	mu       sync.Mutex
	capacity int
	ttl      time.Duration
	now      func() time.Time
	order    *list.List // front = most recently billed
	entries  map[string]*list.Element
}

func newInteractionBillingSet(capacity int, ttl time.Duration) *interactionBillingSet {
	return &interactionBillingSet{
		capacity: capacity,
		ttl:      ttl,
		now:      time.Now,
		order:    list.New(),
		entries:  make(map[string]*list.Element),
	}
}

// claim reports whether the caller is the first to bill this interaction's usage, recording it if
// so. An empty interaction id cannot be deduplicated and is always billed.
func (s *interactionBillingSet) claim(keyID, interactionID string) bool {
	if interactionID == "" {
		return true
	}
	key := keyID + "\x00" + interactionID
	now := s.now()

	s.mu.Lock()
	defer s.mu.Unlock()
	if el, ok := s.entries[key]; ok {
		entry := el.Value.(*interactionBillingEntry)
		if now.Sub(entry.billedAt) < s.ttl {
			return false
		}
		// Expired: bill again and treat it as a fresh entry.
		entry.billedAt = now
		s.order.MoveToFront(el)
		return true
	}
	s.entries[key] = s.order.PushFront(&interactionBillingEntry{key: key, billedAt: now})
	for s.order.Len() > s.capacity {
		oldest := s.order.Back()
		s.order.Remove(oldest)
		delete(s.entries, oldest.Value.(*interactionBillingEntry).key)
	}
	return true
}
