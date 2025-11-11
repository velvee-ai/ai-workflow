package cache

import (
	"testing"
	"time"
)

func TestCache_SetAndGet(t *testing.T) {
	c := New[string](5 * time.Minute)

	c.Set("key1", "value1")
	val, ok := c.Get("key1")
	if !ok {
		t.Fatal("expected to find key1")
	}
	if val != "value1" {
		t.Errorf("expected value1, got %s", val)
	}
}

func TestCache_Expiration(t *testing.T) {
	c := New[string](100 * time.Millisecond)

	c.Set("key1", "value1")
	time.Sleep(150 * time.Millisecond)

	_, ok := c.Get("key1")
	if ok {
		t.Error("expected key1 to be expired")
	}
}

func TestCache_CustomTTL(t *testing.T) {
	c := New[int](5 * time.Minute)

	c.SetWithTTL("short", 42, 50*time.Millisecond)
	c.SetWithTTL("long", 99, 10*time.Minute)

	time.Sleep(100 * time.Millisecond)

	if _, ok := c.Get("short"); ok {
		t.Error("expected short to be expired")
	}
	if val, ok := c.Get("long"); !ok || val != 99 {
		t.Error("expected long to still be present")
	}
}

func TestCache_Delete(t *testing.T) {
	c := New[string](5 * time.Minute)

	c.Set("key1", "value1")
	c.Delete("key1")

	_, ok := c.Get("key1")
	if ok {
		t.Error("expected key1 to be deleted")
	}
}

func TestCache_Clear(t *testing.T) {
	c := New[string](5 * time.Minute)

	c.Set("key1", "value1")
	c.Set("key2", "value2")
	c.Clear()

	if c.Len() != 0 {
		t.Errorf("expected cache to be empty, got %d entries", c.Len())
	}
}

func TestCache_Cleanup(t *testing.T) {
	c := New[string](100 * time.Millisecond)

	c.Set("key1", "value1")
	c.SetWithTTL("key2", "value2", 10*time.Minute)

	time.Sleep(150 * time.Millisecond)
	c.Cleanup()

	if c.Len() != 1 {
		t.Errorf("expected 1 entry after cleanup, got %d", c.Len())
	}

	if _, ok := c.Get("key2"); !ok {
		t.Error("expected key2 to still exist")
	}
}
