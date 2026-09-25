package console

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
	"sync"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// readerTTL is the longest the console reuses one identity's reader.
	readerTTL = 5 * time.Minute
	// maxReaders is the most identities the console keeps readers for.
	maxReaders = 1000
)

// readerCache keeps one read-only client per identity, and per token for
// token sessions, so its REST mapper
// runs discovery once rather than on every request. Each reader still
// impersonates exactly its identity; the cache only saves rebuilding it.
type readerCache struct {
	build func(Identity) (client.Reader, error)
	now   func() time.Time

	mu       sync.Mutex
	seq      uint64
	entries  map[string]readerEntry
	inflight map[string]*readerBuild
}

// readerBuild is one identity's reader under construction. Requests that
// arrive while it runs wait on done and share its result.
type readerBuild struct {
	done   chan struct{}
	reader client.Reader
	err    error
}

type readerEntry struct {
	reader  client.Reader
	expires time.Time
	seq     uint64 // insertion order, for evicting the oldest
}

func newReaderCache(build func(Identity) (client.Reader, error)) *readerCache {
	return &readerCache{build: build, now: time.Now, entries: map[string]readerEntry{}, inflight: map[string]*readerBuild{}}
}

// readerKey identifies id unambiguously by its sign-in method, username and
// sorted groups and, for a token session, the SHA-256 of its token, so two
// tokens never share a reader: each reader carries the one token it was
// built with. The key never holds the token itself.
func readerKey(id Identity) string {
	groups := slices.Clone(id.Groups)
	slices.Sort(groups)
	k := struct {
		Method   string   `json:"m"`
		Username string   `json:"u"`
		Groups   []string `json:"g"`
		Token    string   `json:"h,omitempty"`
	}{Method: MethodOIDC, Username: id.Username, Groups: groups}
	if id.IsToken() {
		sum := sha256.Sum256([]byte(id.Token.Reveal()))
		k.Method, k.Token = MethodToken, hex.EncodeToString(sum[:])
	}
	key, _ := json.Marshal(k)
	return string(key)
}

// get returns id's reader, building one when there is none or it has passed
// min(readerTTL, the session's expiry). Simultaneous requests for one identity
// share a single build. An identity without a session expiry gets a fresh
// reader that is not kept.
func (c *readerCache) get(id Identity) (client.Reader, error) {
	key := readerKey(id)
	c.mu.Lock()
	now := c.now()
	if e, ok := c.entries[key]; ok && now.Before(e.expires) {
		c.mu.Unlock()
		return e.reader, nil
	}
	if b, ok := c.inflight[key]; ok {
		c.mu.Unlock()
		<-b.done
		return b.reader, b.err
	}
	b := &readerBuild{done: make(chan struct{})}
	c.inflight[key] = b
	c.mu.Unlock()

	b.reader, b.err = c.build(id)

	c.mu.Lock()
	delete(c.inflight, key)
	expires := now.Add(readerTTL)
	if id.Expiry.Before(expires) {
		expires = id.Expiry
	}
	if b.err == nil && now.Before(expires) {
		if _, ok := c.entries[key]; !ok && len(c.entries) >= maxReaders {
			c.evict(now)
		}
		c.seq++
		c.entries[key] = readerEntry{reader: b.reader, expires: expires, seq: c.seq}
	}
	c.mu.Unlock()
	close(b.done)
	return b.reader, b.err
}

// evict drops every expired entry or, when none has expired, the oldest.
// The caller holds c.mu.
func (c *readerCache) evict(now time.Time) {
	oldest, oldestSeq := "", uint64(0)
	for k, e := range c.entries {
		if !now.Before(e.expires) {
			delete(c.entries, k)
			continue
		}
		if oldest == "" || e.seq < oldestSeq {
			oldest, oldestSeq = k, e.seq
		}
	}
	if len(c.entries) >= maxReaders {
		delete(c.entries, oldest)
	}
}

func (c *readerCache) len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}
