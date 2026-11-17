package repository

import (
	"context"
	"testing"

	"pingtai/internal/database"
)

func TestLocalAndSSOIdentitiesWithSameUsernameAreDistinctAndStable(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	users := NewUserRepository(db)
	id, err := users.Create(ctx, "test", "hashed-password", 0)
	if err != nil {
		t.Fatal(err)
	}
	user, err := users.FindByID(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	localFirst, err := users.EnsureLocalIdentity(ctx, user)
	if err != nil {
		t.Fatal(err)
	}
	localAgain, err := users.EnsureLocalIdentity(ctx, user)
	if err != nil {
		t.Fatal(err)
	}
	ssoFirst, err := users.EnsureExternalIdentity(ctx, "sso", "test", "test", "学校用户 Test")
	if err != nil {
		t.Fatal(err)
	}
	ssoAgain, err := users.EnsureExternalIdentity(ctx, "sso", "test", "test", "学校用户 Test")
	if err != nil {
		t.Fatal(err)
	}
	if localFirst.ID == ssoFirst.ID {
		t.Fatalf("local and SSO identities with the same account name must be distinct: %q", localFirst.ID)
	}
	if localFirst.ID != localAgain.ID || ssoFirst.ID != ssoAgain.ID {
		t.Fatalf("identity IDs must remain stable: local %q/%q, sso %q/%q",
			localFirst.ID, localAgain.ID, ssoFirst.ID, ssoAgain.ID)
	}
	if localFirst.Provider != "local" || ssoFirst.Provider != "sso" {
		t.Fatalf("unexpected identity providers: local=%q sso=%q", localFirst.Provider, ssoFirst.Provider)
	}
}
