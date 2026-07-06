package db

import (
	"testing"
	"time"
)

func TestInvitations_Lifecycle(t *testing.T) {
	db := newTestDB(t)

	// Create a pending invitation.
	if _, err := db.CreateInvitation("invitee@example.com", "tok-1", time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("CreateInvitation: %v", err)
	}

	// It resolves and lists as pending.
	email, ok := db.GetInvitation("tok-1")
	if !ok || email != "invitee@example.com" {
		t.Fatalf("GetInvitation = %q, %v; want invitee@example.com, true", email, ok)
	}
	if db.HasPendingInvitationForEmail("invitee@example.com") != true {
		t.Error("HasPendingInvitationForEmail should be true")
	}
	pending, _ := db.ListPendingInvitations()
	if len(pending) != 1 {
		t.Fatalf("pending = %d, want 1", len(pending))
	}

	// Once accepted, the token is no longer valid and not listed.
	if err := db.AcceptInvitation("tok-1"); err != nil {
		t.Fatalf("AcceptInvitation: %v", err)
	}
	if _, ok := db.GetInvitation("tok-1"); ok {
		t.Error("accepted invitation must not resolve")
	}
	if pending, _ := db.ListPendingInvitations(); len(pending) != 0 {
		t.Error("accepted invitation must not be listed as pending")
	}
}

func TestInvitations_ExpiredAndRevoked(t *testing.T) {
	db := newTestDB(t)

	// Expired invitation is invalid.
	db.CreateInvitation("old@example.com", "tok-old", time.Now().Add(-time.Hour))
	if _, ok := db.GetInvitation("tok-old"); ok {
		t.Error("expired invitation must not resolve")
	}

	// Revoked invitation disappears.
	id, _ := db.CreateInvitation("rev@example.com", "tok-rev", time.Now().Add(time.Hour))
	if err := db.RevokeInvitation(id); err != nil {
		t.Fatalf("RevokeInvitation: %v", err)
	}
	if _, ok := db.GetInvitation("tok-rev"); ok {
		t.Error("revoked invitation must not resolve")
	}
}
