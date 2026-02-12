package handlers

import (
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestHashPassword_UsesConfiguredCost(t *testing.T) {
	hash, err := HashPassword("Passw0rd!Example")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}

	cost, err := bcrypt.Cost([]byte(hash))
	if err != nil {
		t.Fatalf("bcrypt.Cost() error = %v", err)
	}

	if cost != passwordHashCost {
		t.Fatalf("bcrypt cost = %d, want %d", cost, passwordHashCost)
	}
}
