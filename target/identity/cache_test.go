// SPDX-License-Identifier: GPL-3.0-only

package identity_test

import (
	"testing"

	"scampi.dev/scampi/target/identity"
)

func TestUserRoundTrip(t *testing.T) {
	c := identity.New()

	if _, _, ok := c.UID("alice"); ok {
		t.Fatal("fresh cache must miss")
	}

	c.SetUser("alice", 1000)

	uid, exists, ok := c.UID("alice")
	if !ok || !exists || uid != 1000 {
		t.Fatalf("UID(alice) = (%d, %v, %v), want (1000, true, true)", uid, exists, ok)
	}

	name, ok := c.UserName(1000)
	if !ok || name != "alice" {
		t.Fatalf("UserName(1000) = (%q, %v), want (alice, true)", name, ok)
	}
}

func TestGroupRoundTrip(t *testing.T) {
	c := identity.New()
	c.SetGroup("staff", 50)

	gid, exists, ok := c.GID("staff")
	if !ok || !exists || gid != 50 {
		t.Fatalf("GID(staff) = (%d, %v, %v), want (50, true, true)", gid, exists, ok)
	}

	name, ok := c.GroupName(50)
	if !ok || name != "staff" {
		t.Fatalf("GroupName(50) = (%q, %v), want (staff, true)", name, ok)
	}
}

func TestAbsentUserCached(t *testing.T) {
	c := identity.New()
	c.MarkUserAbsent("ghost")

	uid, exists, ok := c.UID("ghost")
	if !ok || exists || uid != 0 {
		t.Fatalf("UID(ghost) = (%d, %v, %v), want (0, false, true)", uid, exists, ok)
	}
}

func TestInvalidateUserClearsBothDirections(t *testing.T) {
	c := identity.New()
	c.SetUser("alice", 1000)

	c.InvalidateUser("alice")

	if _, _, ok := c.UID("alice"); ok {
		t.Error("UID(alice) still hit after invalidate")
	}
	if _, ok := c.UserName(1000); ok {
		t.Error("UserName(1000) still hit after invalidate")
	}
}

func TestInvalidateGroupClearsBothDirections(t *testing.T) {
	c := identity.New()
	c.SetGroup("staff", 50)

	c.InvalidateGroup("staff")

	if _, _, ok := c.GID("staff"); ok {
		t.Error("GID(staff) still hit after invalidate")
	}
	if _, ok := c.GroupName(50); ok {
		t.Error("GroupName(50) still hit after invalidate")
	}
}

func TestInvalidateAbsentUserDropsNegativeEntry(t *testing.T) {
	c := identity.New()
	c.MarkUserAbsent("ghost")
	c.InvalidateUser("ghost")

	if _, _, ok := c.UID("ghost"); ok {
		t.Error("absent entry survived invalidate")
	}
}

func TestNilCacheInvalidateIsNoOp(_ *testing.T) {
	var c *identity.Cache
	c.InvalidateUser("alice")  // must not panic
	c.InvalidateGroup("staff") // must not panic
}

func TestSetUserUpdatesBothDirections(t *testing.T) {
	c := identity.New()
	c.SetUser("alice", 1000)

	// Reassign alice to a different uid (e.g. usermod -u).
	c.SetUser("alice", 1500)

	uid, _, _ := c.UID("alice")
	if uid != 1500 {
		t.Errorf("UID(alice) = %d, want 1500", uid)
	}

	// The new mapping is reachable; old uid->name still points to alice
	// because we don't track the previous uid. This is fine: callers
	// invalidate explicitly on mutations, and a stray uid->alice hit
	// would only matter if alice's old uid got recycled to a different
	// user, which is contrived.
	name, ok := c.UserName(1500)
	if !ok || name != "alice" {
		t.Errorf("UserName(1500) = (%q, %v), want (alice, true)", name, ok)
	}
}
