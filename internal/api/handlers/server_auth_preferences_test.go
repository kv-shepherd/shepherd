package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

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
