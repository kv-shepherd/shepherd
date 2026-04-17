package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"kv-shepherd.io/shepherd/ent/platformsetting"
	entuserpreference "kv-shepherd.io/shepherd/ent/userpreference"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/api/middleware"
	"kv-shepherd.io/shepherd/internal/testutil"
)

func TestCurrentUserPreference_CRUD(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	client := testutil.OpenEntPostgres(t, "auth_preferences_crud")
	server := NewServer(ServerDeps{EntClient: client})

	user, err := client.User.Create().
		SetID("user-1").
		SetUsername("alice").
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}

	key := "admin.users.columns"

	putBody := []byte(`{"value":{"columns":["profile:department","roles","created_at"]}}`)
	putW := httptest.NewRecorder()
	putC, _ := gin.CreateTestContext(putW)
	putReq := httptest.NewRequest(http.MethodPut, "/auth/preferences/"+key, bytes.NewReader(putBody))
	putReq.Header.Set("Content-Type", "application/json")
	putReq = putReq.WithContext(middleware.SetUserContext(putReq.Context(), user.ID, user.Username, nil))
	putC.Request = putReq

	server.UpdateCurrentUserPreference(putC, key)
	if putW.Code != http.StatusOK {
		t.Fatalf("put status=%d body=%s", putW.Code, putW.Body.String())
	}

	var updated generated.UserPreference
	if err := json.Unmarshal(putW.Body.Bytes(), &updated); err != nil {
		t.Fatalf("decode updated preference: %v", err)
	}
	if updated.Key != key {
		t.Fatalf("updated key=%q, want %q", updated.Key, key)
	}

	getW := httptest.NewRecorder()
	getC, _ := gin.CreateTestContext(getW)
	getReq := httptest.NewRequest(http.MethodGet, "/auth/preferences/"+key, http.NoBody)
	getReq = getReq.WithContext(middleware.SetUserContext(getReq.Context(), user.ID, user.Username, nil))
	getC.Request = getReq

	server.GetCurrentUserPreference(getC, key)
	if getW.Code != http.StatusOK {
		t.Fatalf("get status=%d body=%s", getW.Code, getW.Body.String())
	}

	var got generated.UserPreference
	if err := json.Unmarshal(getW.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode fetched preference: %v", err)
	}
	columns, ok := got.Value["columns"].([]interface{})
	if !ok || len(columns) != 3 {
		t.Fatalf("preference columns=%#v, want 3 entries", got.Value["columns"])
	}
	if columns[0] != "profile:department" || columns[1] != "roles" || columns[2] != "created_at" {
		t.Fatalf("preference columns order=%#v, want preserved order", columns)
	}

	deleteW := httptest.NewRecorder()
	deleteC, _ := gin.CreateTestContext(deleteW)
	deleteReq := httptest.NewRequest(http.MethodDelete, "/auth/preferences/"+key, http.NoBody)
	deleteReq = deleteReq.WithContext(middleware.SetUserContext(deleteReq.Context(), user.ID, user.Username, nil))
	deleteC.Request = deleteReq

	server.DeleteCurrentUserPreference(deleteC, key)
	if deleteW.Code != http.StatusNoContent {
		t.Fatalf("delete status=%d body=%s", deleteW.Code, deleteW.Body.String())
	}

	missingW := httptest.NewRecorder()
	missingC, _ := gin.CreateTestContext(missingW)
	missingReq := httptest.NewRequest(http.MethodGet, "/auth/preferences/"+key, http.NoBody)
	missingReq = missingReq.WithContext(middleware.SetUserContext(missingReq.Context(), user.ID, user.Username, nil))
	missingC.Request = missingReq

	server.GetCurrentUserPreference(missingC, key)
	if missingW.Code != http.StatusNotFound {
		t.Fatalf("missing status=%d body=%s", missingW.Code, missingW.Body.String())
	}
}

func TestCurrentUserPreference_RejectsInvalidKey(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	client := testutil.OpenEntPostgres(t, "auth_preferences_invalid_key")
	server := NewServer(ServerDeps{EntClient: client})

	user, err := client.User.Create().
		SetID("user-1").
		SetUsername("alice").
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodGet, "/auth/preferences/../../bad", http.NoBody)
	req = req.WithContext(middleware.SetUserContext(req.Context(), user.ID, user.Username, nil))
	c.Request = req

	server.GetCurrentUserPreference(c, "../../bad")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestCurrentUserPreference_RejectsMissingValue(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	client := testutil.OpenEntPostgres(t, "auth_preferences_missing_value")
	server := NewServer(ServerDeps{EntClient: client})

	user, err := client.User.Create().
		SetID("user-1").
		SetUsername("alice").
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPut, "/auth/preferences/admin.users.columns", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(middleware.SetUserContext(req.Context(), user.ID, user.Username, nil))
	c.Request = req

	server.UpdateCurrentUserPreference(c, "admin.users.columns")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestCurrentUserPreference_SharedDirectoryLayoutUsesPlatformSetting(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	client := testutil.OpenEntPostgres(t, "auth_preferences_shared_directory_layout")
	server := NewServer(ServerDeps{EntClient: client})

	adminUser, err := client.User.Create().
		SetID("admin-1").
		SetUsername("admin-one").
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("seed admin user: %v", err)
	}
	otherUser, err := client.User.Create().
		SetID("user-2").
		SetUsername("bob").
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("seed other user: %v", err)
	}

	_, err = client.UserPreference.Create().
		SetID("pref-legacy-1").
		SetUserID(adminUser.ID).
		SetKey(sharedUserDirectoryDisplayPreferenceKey).
		SetValue(map[string]interface{}{
			"columns": []string{"profile:department", "roles", "created_at"},
		}).
		Save(t.Context())
	if err != nil {
		t.Fatalf("seed legacy user preference: %v", err)
	}

	legacyGetW := httptest.NewRecorder()
	legacyGetC, _ := gin.CreateTestContext(legacyGetW)
	legacyGetReq := httptest.NewRequest(http.MethodGet, "/auth/preferences/"+sharedUserDirectoryDisplayPreferenceKey, http.NoBody)
	legacyGetReq = legacyGetReq.WithContext(middleware.SetUserContext(legacyGetReq.Context(), adminUser.ID, adminUser.Username, nil))
	legacyGetC.Request = legacyGetReq

	server.GetCurrentUserPreference(legacyGetC, sharedUserDirectoryDisplayPreferenceKey)
	if legacyGetW.Code != http.StatusOK {
		t.Fatalf("legacy get status=%d body=%s", legacyGetW.Code, legacyGetW.Body.String())
	}

	var legacyResp generated.UserPreference
	mustDecodeJSON(t, legacyGetW.Body.Bytes(), &legacyResp)
	legacyColumns, ok := legacyResp.Value["columns"].([]interface{})
	if !ok || len(legacyColumns) != 3 {
		t.Fatalf("legacy columns=%#v, want 3 entries", legacyResp.Value["columns"])
	}
	if legacyColumns[0] != "profile:department" || legacyColumns[1] != "roles" || legacyColumns[2] != "created_at" {
		t.Fatalf("legacy columns order=%#v, want preserved order", legacyColumns)
	}

	putC, putW := newAuthedGinContext(
		t,
		http.MethodPut,
		"/auth/preferences/"+sharedUserDirectoryDisplayPreferenceKey,
		`{"value":{"columns":["profile:section","email","roles","created_at"],"merged_columns":[{"label":"Overview","column_keys":["profile:section","email"],"show_labels":true}]}}`,
		adminUser.ID,
		[]string{"user:manage"},
	)
	server.UpdateCurrentUserPreference(putC, sharedUserDirectoryDisplayPreferenceKey)
	if putW.Code != http.StatusOK {
		t.Fatalf("shared put status=%d body=%s", putW.Code, putW.Body.String())
	}

	var updated generated.UserPreference
	mustDecodeJSON(t, putW.Body.Bytes(), &updated)
	updatedColumns, ok := updated.Value["columns"].([]interface{})
	if !ok || len(updatedColumns) != 4 {
		t.Fatalf("updated columns=%#v, want 4 entries", updated.Value["columns"])
	}
	if updatedColumns[0] != "profile:section" || updatedColumns[1] != "email" || updatedColumns[2] != "roles" || updatedColumns[3] != "created_at" {
		t.Fatalf("updated columns order=%#v, want preserved order", updatedColumns)
	}

	setting, err := client.PlatformSetting.Query().
		Where(platformsetting.KeyEQ(sharedUserDirectoryDisplayPreferenceKey)).
		Only(t.Context())
	if err != nil {
		t.Fatalf("query shared platform setting: %v", err)
	}
	storedColumns, ok := setting.Value["columns"].([]interface{})
	if !ok || len(storedColumns) != 4 {
		t.Fatalf("stored platform-setting columns=%#v, want 4 entries", setting.Value["columns"])
	}
	if storedColumns[0] != "profile:section" || storedColumns[1] != "email" || storedColumns[2] != "roles" || storedColumns[3] != "created_at" {
		t.Fatalf("stored platform-setting columns order=%#v, want preserved order", storedColumns)
	}

	legacyCount, err := client.UserPreference.Query().
		Where(entuserpreference.KeyEQ(sharedUserDirectoryDisplayPreferenceKey)).
		Count(t.Context())
	if err != nil {
		t.Fatalf("count legacy user preferences: %v", err)
	}
	if legacyCount != 0 {
		t.Fatalf("legacy preference count=%d, want 0 after promoting shared layout", legacyCount)
	}

	sharedGetW := httptest.NewRecorder()
	sharedGetC, _ := gin.CreateTestContext(sharedGetW)
	sharedGetReq := httptest.NewRequest(http.MethodGet, "/auth/preferences/"+sharedUserDirectoryDisplayPreferenceKey, http.NoBody)
	sharedGetReq = sharedGetReq.WithContext(middleware.SetUserContext(sharedGetReq.Context(), otherUser.ID, otherUser.Username, nil))
	sharedGetC.Request = sharedGetReq

	server.GetCurrentUserPreference(sharedGetC, sharedUserDirectoryDisplayPreferenceKey)
	if sharedGetW.Code != http.StatusOK {
		t.Fatalf("shared get status=%d body=%s", sharedGetW.Code, sharedGetW.Body.String())
	}

	var sharedResp generated.UserPreference
	mustDecodeJSON(t, sharedGetW.Body.Bytes(), &sharedResp)
	sharedColumns, ok := sharedResp.Value["columns"].([]interface{})
	if !ok || len(sharedColumns) != 4 {
		t.Fatalf("shared columns=%#v, want 4 entries", sharedResp.Value["columns"])
	}
	if sharedColumns[0] != "profile:section" || sharedColumns[1] != "email" || sharedColumns[2] != "roles" || sharedColumns[3] != "created_at" {
		t.Fatalf("shared columns order=%#v, want preserved order", sharedColumns)
	}

	deleteC, deleteW := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/auth/preferences/"+sharedUserDirectoryDisplayPreferenceKey,
		"",
		adminUser.ID,
		[]string{"user:manage"},
	)
	server.DeleteCurrentUserPreference(deleteC, sharedUserDirectoryDisplayPreferenceKey)
	if deleteW.Code != http.StatusNoContent {
		t.Fatalf("shared delete status=%d body=%s", deleteW.Code, deleteW.Body.String())
	}

	missingSharedW := httptest.NewRecorder()
	missingSharedC, _ := gin.CreateTestContext(missingSharedW)
	missingSharedReq := httptest.NewRequest(http.MethodGet, "/auth/preferences/"+sharedUserDirectoryDisplayPreferenceKey, http.NoBody)
	missingSharedReq = missingSharedReq.WithContext(middleware.SetUserContext(missingSharedReq.Context(), otherUser.ID, otherUser.Username, nil))
	missingSharedC.Request = missingSharedReq

	server.GetCurrentUserPreference(missingSharedC, sharedUserDirectoryDisplayPreferenceKey)
	if missingSharedW.Code != http.StatusNotFound {
		t.Fatalf("shared missing status=%d body=%s", missingSharedW.Code, missingSharedW.Body.String())
	}
}

func TestCurrentUserPreference_SharedDirectoryLayoutRejectsNonAdminMutation(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	client := testutil.OpenEntPostgres(t, "auth_preferences_shared_directory_forbidden")
	server := NewServer(ServerDeps{EntClient: client})

	user, err := client.User.Create().
		SetID("user-1").
		SetUsername("alice").
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}

	putC, putW := newAuthedGinContext(
		t,
		http.MethodPut,
		"/auth/preferences/"+sharedUserDirectoryDisplayPreferenceKey,
		`{"value":{"columns":["email","roles","created_at"]}}`,
		user.ID,
		nil,
	)
	server.UpdateCurrentUserPreference(putC, sharedUserDirectoryDisplayPreferenceKey)
	if putW.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", putW.Code, putW.Body.String())
	}
	assertErrorCode(t, putW.Body.Bytes(), "FORBIDDEN")
}
