package lyrics

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func syncedResult() Result {
	return Result{
		Synced: true,
		Lines: []Line{
			{At: 1 * time.Second, Text: "first"},
			{At: 63500 * time.Millisecond, Text: "second"},
		},
	}
}

func plainResult() Result {
	return Result{
		Synced: false,
		Lines: []Line{
			{At: -1, Text: "line one"},
			{At: -1, Text: "line two"},
		},
	}
}

func TestCachePutGetSynced(t *testing.T) {
	dir := t.TempDir()
	c := NewCache(dir, 7)
	in := syncedResult()
	c.Put("Artist", "Title", "Album", 100, in)

	got, hit, stale := c.Get("Artist", "Title", "Album", 100)
	if !hit || stale {
		t.Fatalf("expected hit, got hit=%v stale=%v", hit, stale)
	}
	if !got.Synced || len(got.Lines) != 2 {
		t.Fatalf("bad result: %+v", got)
	}
	if got.Lines[0].Text != "first" || got.Lines[1].Text != "second" {
		t.Fatalf("bad text: %+v", got.Lines)
	}
	if got.Lines[1].At != 63500*time.Millisecond {
		t.Fatalf("bad timestamp: %v", got.Lines[1].At)
	}
}

func TestCachePutGetPlain(t *testing.T) {
	c := NewCache(t.TempDir(), 7)
	c.Put("A", "B", "", 0, plainResult())
	got, hit, _ := c.Get("A", "B", "", 0)
	if !hit || got.Synced {
		t.Fatalf("expected plain hit, got hit=%v synced=%v", hit, got.Synced)
	}
	if len(got.Lines) != 2 || got.Lines[0].Text != "line one" {
		t.Fatalf("bad plain result: %+v", got.Lines)
	}
}

func TestCacheKeyStable(t *testing.T) {
	c := NewCache(t.TempDir(), 7)
	k1 := c.key("Artist", "Title", "Album", 100)
	k2 := c.key("  artist ", "TITLE", "album", 100)
	if k1 != k2 {
		t.Fatalf("key not normalized: %s != %s", k1, k2)
	}
	k3 := c.key("Artist", "Title", "Album", 101)
	if k1 == k3 {
		t.Fatalf("duration must change the key")
	}
}

func TestCacheFreshMiss(t *testing.T) {
	c := NewCache(t.TempDir(), 7)
	c.PutMiss("A", "B", "C", 10)
	_, hit, blocked := c.Get("A", "B", "C", 10)
	if hit {
		t.Fatal("miss must not be a hit")
	}
	if !blocked {
		t.Fatal("a fresh miss must block a refetch")
	}
}

func TestCacheStaleMiss(t *testing.T) {
	dir := t.TempDir()
	c := NewCache(dir, 7)
	c.PutMiss("A", "B", "C", 10)
	// Backdate the miss file past the recheck window.
	k := c.key("A", "B", "C", 10)
	old := time.Now().Add(-8 * 24 * time.Hour)
	if err := os.Chtimes(filepath.Join(dir, k+".miss"), old, old); err != nil {
		t.Fatal(err)
	}
	_, hit, blocked := c.Get("A", "B", "C", 10)
	if hit {
		t.Fatal("stale miss must not be a hit")
	}
	if blocked {
		t.Fatal("a stale miss must not block a refetch")
	}
}

func TestCacheNeverStale(t *testing.T) {
	dir := t.TempDir()
	c := NewCache(dir, 0) // 0 => never stale
	c.PutMiss("A", "B", "C", 10)
	k := c.key("A", "B", "C", 10)
	old := time.Now().Add(-100 * 24 * time.Hour)
	os.Chtimes(filepath.Join(dir, k+".miss"), old, old)
	_, _, blocked := c.Get("A", "B", "C", 10)
	if !blocked {
		t.Fatal("recheck 0 must always block")
	}
}

func TestCachePutClearsMiss(t *testing.T) {
	dir := t.TempDir()
	c := NewCache(dir, 7)
	c.PutMiss("A", "B", "C", 10)
	c.Put("A", "B", "C", 10, plainResult())
	k := c.key("A", "B", "C", 10)
	if _, err := os.Stat(filepath.Join(dir, k+".miss")); !os.IsNotExist(err) {
		t.Fatal("Put must remove the miss file")
	}
	_, hit, _ := c.Get("A", "B", "C", 10)
	if !hit {
		t.Fatal("expected a hit after Put")
	}
}

func TestCacheDisabled(t *testing.T) {
	c := NewCache("", 7)
	if c.Writable() {
		t.Fatal("empty dir must not be writable")
	}
	c.Put("A", "B", "C", 1, plainResult())
	_, hit, blocked := c.Get("A", "B", "C", 1)
	if hit || blocked {
		t.Fatal("disabled cache must report no hit and no block")
	}
}
