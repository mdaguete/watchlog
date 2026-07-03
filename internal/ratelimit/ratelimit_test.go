package ratelimit

import (
	"testing"
	"time"
)

func TestAllow_UnderLimit(t *testing.T) {
	l := New(3, time.Minute)
	if !l.Allow("1.2.3.4") {
		t.Fatal("should allow first attempt")
	}
}

func TestAllow_ExceedsLimit(t *testing.T) {
	l := New(3, time.Minute)
	key := "1.2.3.4"
	l.Record(key)
	l.Record(key)
	l.Record(key)
	if l.Allow(key) {
		t.Fatal("should block after 3 attempts")
	}
}

func TestAllow_DifferentKeys(t *testing.T) {
	l := New(2, time.Minute)
	l.Record("1.1.1.1")
	l.Record("1.1.1.1")
	if l.Allow("1.1.1.1") {
		t.Fatal("should block 1.1.1.1")
	}
	if !l.Allow("2.2.2.2") {
		t.Fatal("should allow 2.2.2.2")
	}
}

func TestReset(t *testing.T) {
	l := New(2, time.Minute)
	key := "1.2.3.4"
	l.Record(key)
	l.Record(key)
	if l.Allow(key) {
		t.Fatal("should be blocked")
	}
	l.Reset(key)
	if !l.Allow(key) {
		t.Fatal("should be allowed after reset")
	}
}

func TestAllow_WindowExpires(t *testing.T) {
	l := New(2, 50*time.Millisecond)
	key := "1.2.3.4"
	l.Record(key)
	l.Record(key)
	if l.Allow(key) {
		t.Fatal("should be blocked")
	}
	time.Sleep(60 * time.Millisecond)
	if !l.Allow(key) {
		t.Fatal("should be allowed after window expires")
	}
}

func TestRecord_RestartsAfterWindow(t *testing.T) {
	l := New(2, 50*time.Millisecond)
	key := "1.2.3.4"
	l.Record(key)
	l.Record(key)
	time.Sleep(60 * time.Millisecond)
	l.Record(key) // should start fresh
	if !l.Allow(key) {
		t.Fatal("should allow after window reset, only 1 attempt")
	}
}
