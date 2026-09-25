package console

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// authFunc is an Authenticator the test can switch between identities.
type authFunc func(*http.Request) (Identity, error)

func (f authFunc) Identity(r *http.Request) (Identity, error) { return f(r) }

// newWrappedServer serves the API against env through wrap, which sees every
// request after the impersonation headers are set.
func newWrappedServer(t *testing.T, wrap func(*http.Request)) *Server {
	t.Helper()
	base := rest.CopyConfig(env.Config)
	base.WrapTransport = func(rt http.RoundTripper) http.RoundTripper {
		return roundTripFunc(func(r *http.Request) (*http.Response, error) {
			wrap(r)
			return rt.RoundTrip(r)
		})
	}
	s, err := NewServer(Config{ClusterName: "test"}, base)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// Catches: a reader built per request, which runs discovery on every poll.
func TestConsoleRunsDiscoveryOncePerIdentity(t *testing.T) {
	var mu sync.Mutex
	discovery := 0
	srv := newWrappedServer(t, func(r *http.Request) {
		if r.URL.Path == "/api" || r.URL.Path == "/apis" {
			mu.Lock()
			discovery++
			mu.Unlock()
		}
	})
	srv.UseAuthenticator(fakeAuth{id: Identity{Username: "alice", Expiry: time.Now().Add(time.Hour)}})
	count := func() int { mu.Lock(); defer mu.Unlock(); return discovery }

	if rec := get(t, srv, "/api/applications?namespace=a"); rec.Code != http.StatusOK {
		t.Fatalf("first request = %d %s", rec.Code, rec.Body.String())
	}
	first := count()
	if rec := get(t, srv, "/api/applications?namespace=a"); rec.Code != http.StatusOK {
		t.Fatalf("second request = %d %s", rec.Code, rec.Body.String())
	}
	t.Logf("discovery requests to /api and /apis: %d after the first request, %d after the second", first, count())
	if first == 0 {
		t.Fatal("the first request ran no discovery; the counter sees nothing")
	}
	if got := count(); got != first {
		t.Fatalf("the second request ran discovery again: %d requests, want %d", got, first)
	}
}

// Catches: a cache keyed by username alone, or groups in a way that lets two
// identities share a reader, so one user's request impersonates another.
func TestConsoleKeepsOneReaderPerIdentity(t *testing.T) {
	var mu sync.Mutex
	var sent []string
	srv := newWrappedServer(t, func(r *http.Request) {
		mu.Lock()
		sent = append(sent, r.Header.Get("Impersonate-User")+" "+strings.Join(r.Header.Values("Impersonate-Group"), ","))
		mu.Unlock()
	})
	var current Identity
	srv.UseAuthenticator(authFunc(func(*http.Request) (Identity, error) { return current, nil }))
	expiry := time.Now().Add(time.Hour)
	for _, id := range []Identity{
		{Username: "alice", Groups: []string{"team-a"}, Expiry: expiry},
		{Username: "alice", Groups: []string{"team-b"}, Expiry: expiry},
		{Username: "bob", Groups: []string{"team-b"}, Expiry: expiry},
		{Username: "alice", Groups: []string{"team-a"}, Expiry: expiry},
		{Username: "alice", Groups: []string{"team-b"}, Expiry: expiry},
	} {
		current = id
		mu.Lock()
		sent = nil
		mu.Unlock()
		get(t, srv, "/api/applications?namespace=a")
		want := id.Username + " " + strings.Join(id.Groups, ",")
		mu.Lock()
		if len(sent) == 0 {
			t.Fatalf("%+v sent no request", id)
		}
		for _, got := range sent {
			if got != want {
				t.Fatalf("a request for %q impersonated %q", want, got)
			}
		}
		mu.Unlock()
	}
	if n := srv.readers.len(); n != 3 {
		t.Fatalf("cached readers = %d, want 3", n)
	}
}

// countedReader is a distinct reader per build.
type countedReader struct {
	client.Reader
	n int
}

// newTestCache returns a cache on a clock the test moves, and the number of
// readers it has built.
func newTestCache() (c *readerCache, now *time.Time, builds *int) {
	t0 := time.Unix(1_000_000, 0)
	n := 0
	c = newReaderCache(func(Identity) (client.Reader, error) {
		n++
		return &countedReader{n: n}, nil
	})
	c.now = func() time.Time { return t0 }
	return c, &t0, &n
}

func mustGet(t *testing.T, c *readerCache, id Identity) client.Reader {
	t.Helper()
	r, err := c.get(id)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// Catches: an entry reused past five minutes, or past the session's ID token.
func TestReaderCacheExpiresEntries(t *testing.T) {
	c, now, builds := newTestCache()

	long := Identity{Username: "alice", Groups: []string{"b", "a"}, Expiry: now.Add(time.Hour)}
	r1 := mustGet(t, c, long)
	if r2 := mustGet(t, c, Identity{Username: "alice", Groups: []string{"a", "b"}, Expiry: long.Expiry}); r2 != r1 || *builds != 1 {
		t.Fatalf("the same identity with its groups reordered was rebuilt: %d builds", *builds)
	}
	*now = now.Add(readerTTL - time.Second)
	if mustGet(t, c, long) != r1 || *builds != 1 {
		t.Fatalf("rebuilt before the TTL: %d builds", *builds)
	}
	*now = now.Add(time.Second)
	if mustGet(t, c, long) == r1 || *builds != 2 {
		t.Fatalf("reused at the TTL: %d builds", *builds)
	}

	short := Identity{Username: "bob", Expiry: now.Add(time.Minute)}
	r3 := mustGet(t, c, short)
	*now = now.Add(time.Minute - time.Second)
	if mustGet(t, c, short) != r3 || *builds != 3 {
		t.Fatalf("rebuilt before the session expiry: %d builds", *builds)
	}
	*now = now.Add(time.Second)
	if mustGet(t, c, short) == r3 || *builds != 4 {
		t.Fatalf("reused at the session expiry: %d builds", *builds)
	}

	noExpiry := Identity{Username: "carol"}
	if mustGet(t, c, noExpiry) == mustGet(t, c, noExpiry) || *builds != 6 {
		t.Fatalf("an identity with no session expiry was reused: %d builds", *builds)
	}
}

// Catches: an unbounded cache, or one that evicts a live entry while an
// expired one stays.
func TestReaderCacheEvictsExpiredThenOldest(t *testing.T) {
	c, now, builds := newTestCache()
	user := func(i int) Identity {
		return Identity{Username: fmt.Sprintf("user-%d", i), Expiry: now.Add(time.Hour)}
	}
	for i := range maxReaders {
		mustGet(t, c, user(i))
	}
	mustGet(t, c, user(maxReaders))
	if n := c.len(); n != maxReaders {
		t.Fatalf("cached readers = %d, want %d", n, maxReaders)
	}
	before := *builds
	mustGet(t, c, user(1))
	if *builds != before {
		t.Fatal("the second-oldest entry was evicted")
	}
	mustGet(t, c, user(0))
	if *builds != before+1 {
		t.Fatal("the oldest entry was not evicted")
	}

	// An expired entry goes before the oldest live one.
	c, now, builds = newTestCache()
	for i := range maxReaders - 1 {
		mustGet(t, c, user(i))
	}
	mustGet(t, c, Identity{Username: "brief", Expiry: now.Add(time.Minute)})
	*now = now.Add(2 * time.Minute)
	mustGet(t, c, user(maxReaders))
	before = *builds
	mustGet(t, c, user(0))
	if *builds != before {
		t.Fatal("the oldest live entry was evicted while an expired one was cached")
	}
}

// Catches: unsynchronised map access under concurrent requests (run with -race).
func TestReaderCacheIsSafeConcurrently(t *testing.T) {
	var mu sync.Mutex
	c := newReaderCache(func(id Identity) (client.Reader, error) {
		mu.Lock()
		defer mu.Unlock()
		return &countedReader{}, nil
	})
	var wg sync.WaitGroup
	for i := range 50 {
		wg.Go(func() {
			if _, err := c.get(Identity{Username: fmt.Sprintf("user-%d", i%5), Expiry: time.Now().Add(time.Hour)}); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if n := c.len(); n != 5 {
		t.Fatalf("cached readers = %d, want 5", n)
	}
}

// Catches: simultaneous first requests by one identity each building a
// reader, so a page load's parallel API calls repeat discovery.
func TestReaderCacheBuildsOnceForSimultaneousRequests(t *testing.T) {
	var builds atomic.Int32
	release := make(chan struct{})
	c := newReaderCache(func(Identity) (client.Reader, error) {
		builds.Add(1)
		<-release
		return &countedReader{}, nil
	})
	id := Identity{Username: "alice", Expiry: time.Now().Add(time.Hour)}
	readers := make(chan client.Reader, 10)
	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			r, err := c.get(id)
			if err != nil {
				t.Error(err)
			}
			readers <- r
		})
	}
	time.Sleep(50 * time.Millisecond) // let every request reach the cache
	close(release)
	wg.Wait()
	close(readers)
	if n := builds.Load(); n != 1 {
		t.Fatalf("10 simultaneous requests built %d readers, want 1", n)
	}
	first := <-readers
	for r := range readers {
		if r != first {
			t.Fatal("simultaneous requests got different readers")
		}
	}
}
