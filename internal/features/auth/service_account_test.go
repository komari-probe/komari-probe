package auth

import (
	"testing"

	"github.com/komari-monitor/komari/internal/platform/dbcore"
	"github.com/komari-monitor/komari/internal/platform/models"
)

func TestCheckPasswordUpgradesLegacyHashOnSuccess(t *testing.T) {
	db := dbcore.GetDBInstance()
	user, err := CreateAccountWithDB(db, "legacyuser", "irrelevant")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	defer DeleteAccountByUsername("legacyuser")

	// Overwrite with a pre-migration legacy hash, as if this row predated bcrypt.
	legacy := legacyHashPasswd("correcthorse")
	if err := db.Model(&models.User{}).Where("uuid = ?", user.UUID).Update("passwd", legacy).Error; err != nil {
		t.Fatalf("seed legacy hash: %v", err)
	}

	uuid, ok := CheckPassword("legacyuser", "correcthorse")
	if !ok || uuid != user.UUID {
		t.Fatalf("CheckPassword against legacy hash = (%q, %v), want (%q, true)", uuid, ok, user.UUID)
	}

	var reloaded models.User
	if err := db.Where("uuid = ?", user.UUID).First(&reloaded).Error; err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if !isBcryptHash(reloaded.Passwd) {
		t.Fatalf("stored hash was not upgraded to bcrypt: %q", reloaded.Passwd)
	}
	if reloaded.Passwd == legacy {
		t.Fatal("stored hash still equals the legacy hash after a successful login")
	}

	// The upgraded hash must still authenticate the same password.
	uuid, ok = CheckPassword("legacyuser", "correcthorse")
	if !ok || uuid != user.UUID {
		t.Fatalf("CheckPassword after upgrade = (%q, %v), want (%q, true)", uuid, ok, user.UUID)
	}

	// A wrong password must still be rejected, both before and after upgrade.
	if _, ok := CheckPassword("legacyuser", "wrongpassword"); ok {
		t.Fatal("CheckPassword accepted a wrong password after upgrade")
	}
}

func TestCheckPasswordRejectsWrongPasswordAgainstLegacyHash(t *testing.T) {
	db := dbcore.GetDBInstance()
	user, err := CreateAccountWithDB(db, "legacyuser2", "irrelevant")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	defer DeleteAccountByUsername("legacyuser2")

	legacy := legacyHashPasswd("correcthorse")
	if err := db.Model(&models.User{}).Where("uuid = ?", user.UUID).Update("passwd", legacy).Error; err != nil {
		t.Fatalf("seed legacy hash: %v", err)
	}

	if _, ok := CheckPassword("legacyuser2", "wrongpassword"); ok {
		t.Fatal("CheckPassword accepted a wrong password against a legacy hash")
	}

	var reloaded models.User
	if err := db.Where("uuid = ?", user.UUID).First(&reloaded).Error; err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if reloaded.Passwd != legacy {
		t.Fatal("a failed login must not upgrade or otherwise change the stored hash")
	}
}
