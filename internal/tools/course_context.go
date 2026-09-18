package tools

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"sync"
	"time"

	"github.com/jaketmoon/hdu-station/internal/campusauth"
)

// CourseContexts retains only sanitized public course evidence, never user
// prompts, credentials, personal schedules or favorite snapshots.
// Entries are bounded, account-bound and process-local; deletion forgets them.
type CourseContexts struct {
	mu      sync.Mutex
	entries map[string]courseContextEntry
}
type courseEvidence struct {
	Offerings  *OfferingResult `json:"offerings"`
	Collection *FavoriteResult `json:"lastCollectionOperation,omitempty"`
}
type courseContextEntry struct {
	account [32]byte
	expires time.Time
	data    []byte
}

const courseContextTTL = 30 * time.Minute
const courseContextBytes = 256 << 10

func (s *CampusSession) accountKey(ctx context.Context) ([32]byte, bool) {
	if s.client == nil || s.client.credentials == nil {
		return [32]byte{}, false
	}
	token, err := s.client.credentials.AccessTokenFor(ctx, campusauth.CourseScope)
	if err != nil || token == "" {
		return [32]byte{}, false
	}
	return sha256.Sum256([]byte(token)), true
}
func (c *CourseContexts) Delete(id string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, id)
}
func (c *CourseContexts) Restore(ctx context.Context, id string, s *CampusSession) string {
	if c == nil || id == "" {
		return ""
	}
	account, ok := s.accountKey(ctx)
	if !ok {
		c.Delete(id)
		return ""
	}
	c.mu.Lock()
	entry, found := c.entries[id]
	if found && (entry.account != account || time.Now().After(entry.expires)) {
		delete(c.entries, id)
		found = false
	}
	c.mu.Unlock()
	if !found {
		return ""
	}
	var evidence courseEvidence
	if json.Unmarshal(entry.data, &evidence) != nil || (evidence.Offerings == nil || !evidence.Offerings.Term.valid()) {
		return ""
	}
	offerings := evidence.Offerings
	s.lastOfferings = offerings
	s.previousCollection = evidence.Collection
	s.contextAccount = account
	s.contextExpires = entry.expires
	// Rebuild only deterministic parsing; no cached personal schedule is restored.
	if offerings != nil {
		for i := range offerings.Queries {
			q := &offerings.Queries[i]
			for j := range q.Classes {
				o := &q.Classes[j]
				o.Times, o.TimeComplete = parseCourseTimes(o.ClassTime)
			}
			for j := range q.Candidates {
				for k := range q.Candidates[j].Classes {
					o := &q.Candidates[j].Classes[k]
					o.Times, o.TimeComplete = parseCourseTimes(o.ClassTime)
				}
			}
		}
	}
	return string(entry.data)
}
func (c *CourseContexts) Save(ctx context.Context, id string, s *CampusSession) {
	if c == nil || id == "" || s.lastOfferings == nil {
		return
	}
	account, ok := s.accountKey(ctx)
	if !ok || (s.contextAccount != [32]byte{} && account != s.contextAccount) {
		c.Delete(id)
		return
	}
	// Do not extend the lifetime of old evidence just by following up.
	expires := s.contextExpires
	if expires.IsZero() {
		expires = time.Now().Add(courseContextTTL)
	}
	var compact *OfferingResult
	data, err := json.Marshal(s.lastOfferings)
	if err != nil {
		return
	}
	if json.Unmarshal(data, &compact) != nil {
		return
	}
	if compact != nil {
		for i := range compact.Queries {
			q := &compact.Queries[i]
			for j := range q.Classes {
				q.Classes[j].Times = nil
			}
			for j := range q.Candidates {
				for k := range q.Candidates[j].Classes {
					q.Candidates[j].Classes[k].Times = nil
				}
			}
		}
	}
	collection := s.previousCollection
	for i := len(s.favoriteReceipts) - 1; i >= 0; i-- {
		r := s.favoriteReceipts[i]
		if r.Action != "read" && r.Action != "rank" {
			collection = &r
			break
		}
	}
	if collection != nil {
		copy := *collection
		copy.Courses = append([]Offering(nil), collection.Courses...)
		for i := range copy.Courses {
			copy.Courses[i].Times = nil
		}
		collection = &copy
	}
	data, err = json.Marshal(courseEvidence{Offerings: compact, Collection: collection})
	if err != nil || len(data) > courseContextBytes {
		c.Delete(id)
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = map[string]courseContextEntry{}
	}
	for key, entry := range c.entries {
		if time.Now().After(entry.expires) {
			delete(c.entries, key)
		}
	}
	if _, exists := c.entries[id]; !exists && len(c.entries) >= 64 {
		var oldest string
		var expiry time.Time
		for key, entry := range c.entries {
			if oldest == "" || entry.expires.Before(expiry) {
				oldest, expiry = key, entry.expires
			}
		}
		delete(c.entries, oldest)
	}
	c.entries[id] = courseContextEntry{account: account, expires: expires, data: data}
}
