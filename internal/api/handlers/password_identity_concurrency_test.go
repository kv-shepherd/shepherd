package handlers

import (
	"net/http"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/internal/service"
)

const (
	passwordIdentityRaceOldPassword = "Passw0rd!Example"
	passwordIdentityRaceNewPassword = "freshalias9"
	passwordIdentityRaceFreshEmail  = passwordIdentityRaceNewPassword + "@example.com"
)

func TestUpdateUser_RejectsPasswordAgainstFreshIdentityAfterConcurrentEmailChange(t *testing.T) {
	srv, client, authSessions := newAdminIdentityTestServerWithAuthSessions(t, "update_user_fresh_password_identity")
	ctx := t.Context()
	srv.passwordHashGenerator = passwordIdentityRaceHashGenerator

	const userID = "user-update-fresh-password-identity"
	originalHash := passwordIdentityRaceHash(t, passwordIdentityRaceOldPassword)
	if _, err := client.User.Create().
		SetID(userID).
		SetUsername("password.identity.admin").
		SetEmail("old-admin@example.com").
		SetPasswordHash(originalHash).
		SetEnabled(true).
		Save(ctx); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	beforeVersion, err := authSessions.CurrentSessionVersion(ctx, userID)
	if err != nil {
		t.Fatalf("seed session version: %v", err)
	}

	requestContext, response := newAuthedGinContext(
		t,
		http.MethodPatch,
		"/admin/users/"+userID,
		`{"password":"`+passwordIdentityRaceNewPassword+`"}`,
		"admin-1",
		[]string{"user:manage"},
	)
	release, blockerPID := holdUserMutationGuard(t, srv.pool, userID)
	done := runHandlerAsync(func() { srv.UpdateUser(requestContext, userID) })
	waitForBlockedAdvisoryCalls(t, srv.pool, blockerPID, 1)

	if err := client.User.UpdateOneID(userID).SetEmail(passwordIdentityRaceFreshEmail).Exec(ctx); err != nil {
		t.Fatalf("commit concurrent email update: %v", err)
	}
	release()
	waitForHandlerCompletion(t, done, "update user with a password validated against fresh identity")

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d body=%s", response.Code, http.StatusBadRequest, response.Body.String())
	}
	assertErrorCode(t, response.Body.Bytes(), "INVALID_REQUEST")
	assertPasswordIdentityRaceRejected(t, client, authSessions, userID, originalHash, beforeVersion)
}

func TestChangePassword_RejectsPasswordAgainstFreshIdentityAfterConcurrentEmailChange(t *testing.T) {
	srv, client, authSessions := newAdminIdentityTestServerWithAuthSessions(t, "change_password_fresh_identity")
	ctx := t.Context()

	const userID = "user-change-password-fresh-identity"
	originalHash := passwordIdentityRaceHash(t, passwordIdentityRaceOldPassword)
	if _, err := client.User.Create().
		SetID(userID).
		SetUsername("password.identity.self").
		SetEmail("old-self@example.com").
		SetPasswordHash(originalHash).
		SetEnabled(true).
		Save(ctx); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	beforeVersion, err := authSessions.CurrentSessionVersion(ctx, userID)
	if err != nil {
		t.Fatalf("seed session version: %v", err)
	}
	srv.passwordHashGenerator = passwordIdentityRaceHashGenerator

	requestContext, response := newAuthedGinContext(
		t,
		http.MethodPost,
		"/auth/change-password",
		`{"old_password":"`+passwordIdentityRaceOldPassword+`","new_password":"`+passwordIdentityRaceNewPassword+`"}`,
		userID,
		nil,
	)
	release, blockerPID := holdUserMutationGuard(t, srv.pool, userID)
	done := runHandlerAsync(func() { srv.ChangePassword(requestContext) })
	waitForBlockedAdvisoryCalls(t, srv.pool, blockerPID, 1)

	if err := client.User.UpdateOneID(userID).SetEmail(passwordIdentityRaceFreshEmail).Exec(ctx); err != nil {
		t.Fatalf("commit concurrent email update: %v", err)
	}
	release()
	waitForHandlerCompletion(t, done, "change password with a password validated against fresh identity")

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d body=%s", response.Code, http.StatusBadRequest, response.Body.String())
	}
	assertErrorCode(t, response.Body.Bytes(), "INVALID_REQUEST")
	assertPasswordIdentityRaceRejected(t, client, authSessions, userID, originalHash, beforeVersion)
}

func passwordIdentityRaceHash(t *testing.T, password string) string {
	t.Helper()
	hash, err := passwordIdentityRaceHashGenerator(password)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	return hash
}

func passwordIdentityRaceHashGenerator(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	return string(hash), err
}

func assertPasswordIdentityRaceRejected(
	t *testing.T,
	client *ent.Client,
	authSessions *service.AuthSessionManager,
	userID string,
	originalHash string,
	beforeVersion int64,
) {
	t.Helper()
	reloaded, err := client.User.Get(t.Context(), userID)
	if err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if reloaded.Email != passwordIdentityRaceFreshEmail {
		t.Fatalf("email = %q, want committed concurrent value %q", reloaded.Email, passwordIdentityRaceFreshEmail)
	}
	if reloaded.PasswordHash != originalHash {
		t.Fatal("password hash changed despite fresh-identity policy rejection")
	}
	if compareOldErr := bcrypt.CompareHashAndPassword([]byte(reloaded.PasswordHash), []byte(passwordIdentityRaceOldPassword)); compareOldErr != nil {
		t.Fatalf("old password no longer matches after rejection: %v", compareOldErr)
	}
	if compareNewErr := bcrypt.CompareHashAndPassword([]byte(reloaded.PasswordHash), []byte(passwordIdentityRaceNewPassword)); compareNewErr == nil {
		t.Fatal("new password matches after fresh-identity policy rejection")
	}
	afterVersion, err := authSessions.CurrentSessionVersion(t.Context(), userID)
	if err != nil {
		t.Fatalf("read session version after rejection: %v", err)
	}
	if afterVersion != beforeVersion {
		t.Fatalf("session version = %d, want unchanged value %d", afterVersion, beforeVersion)
	}
}
