package sessions_test

import (
	"context"
	"testing"

	"github.com/oliverhaas/djangogo/sessions"
)

func TestSessionSetGet(t *testing.T) {
	s := (&sessions.SignedCookieStore{}).New()
	if _, ok := s.Get("missing"); ok {
		t.Fatal("Get on empty session reported present")
	}
	if s.Modified() {
		t.Fatal("fresh session is modified")
	}
	s.Set("uid", 7)
	if !s.Modified() {
		t.Fatal("Set did not mark session modified")
	}
	v, ok := s.Get("uid")
	if !ok || v != 7 {
		t.Fatalf("Get(uid) = %v, %v; want 7, true", v, ok)
	}
}

func TestSessionDelete(t *testing.T) {
	s := (&sessions.SignedCookieStore{}).New()
	s.Set("a", 1)
	fresh := (&sessions.SignedCookieStore{}).New()
	fresh.Delete("nope")
	if fresh.Modified() {
		t.Fatal("Delete of absent key marked session modified")
	}
	s.Delete("a")
	if _, ok := s.Get("a"); ok {
		t.Fatal("key still present after Delete")
	}
}

func TestSessionPop(t *testing.T) {
	s := (&sessions.SignedCookieStore{}).New()
	s.Set("a", "x")
	if _, ok := s.Pop("absent"); ok {
		t.Fatal("Pop of absent key reported present")
	}
	v, ok := s.Pop("a")
	if !ok || v != "x" {
		t.Fatalf("Pop(a) = %v, %v; want x, true", v, ok)
	}
	if _, ok := s.Get("a"); ok {
		t.Fatal("key still present after Pop")
	}
}

func TestSessionClearAndDataCopy(t *testing.T) {
	s := (&sessions.SignedCookieStore{}).New()
	s.Set("a", 1)
	s.Set("b", 2)
	d := s.Data()
	d["a"] = 99 // mutating the copy must not affect the session
	if v, _ := s.Get("a"); v != 1 {
		t.Fatalf("Data() returned a live map; session value changed to %v", v)
	}
	s.Clear()
	if !s.Modified() {
		t.Fatal("Clear did not mark session modified")
	}
	if len(s.Data()) != 0 {
		t.Fatal("Clear did not wipe data")
	}
}

func TestContextRoundTrip(t *testing.T) {
	if _, ok := sessions.FromContext(context.Background()); ok {
		t.Fatal("FromContext on empty context reported present")
	}
	s := (&sessions.SignedCookieStore{}).New()
	ctx := sessions.NewContext(context.Background(), s)
	got, ok := sessions.FromContext(ctx)
	if !ok || got != s {
		t.Fatalf("FromContext = %v, %v; want the same session", got, ok)
	}
}
