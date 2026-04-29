package config

import "testing"

func TestPasswordPolicyValidatePassword_RejectsCommonPassword(t *testing.T) {
	t.Parallel()

	err := (PasswordPolicy{}).ValidatePassword("password123")
	if err == nil {
		t.Fatal("ValidatePassword() expected common-password rejection")
	}
}

func TestPasswordPolicyValidatePassword_RejectsIdentifierReuse(t *testing.T) {
	t.Parallel()

	err := (PasswordPolicy{}).ValidatePassword("engineer1!", "engineer1!")
	if err == nil {
		t.Fatal("ValidatePassword() expected identifier reuse rejection")
	}
}

func TestPasswordPolicyValidatePassword_EnforcesLegacyComposition(t *testing.T) {
	t.Parallel()

	policy := PasswordPolicy{
		Mode:             "legacy",
		RequireUppercase: true,
		RequireLowercase: true,
		RequireDigit:     true,
		RequireSpecial:   true,
	}

	if err := policy.ValidatePassword("password1!"); err == nil {
		t.Fatal("ValidatePassword() expected uppercase requirement failure")
	}
	if err := policy.ValidatePassword("Password1!"); err != nil {
		t.Fatalf("ValidatePassword() unexpected error = %v", err)
	}
}
