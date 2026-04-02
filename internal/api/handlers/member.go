package handlers

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqljson"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/predicate"
	"kv-shepherd.io/shepherd/ent/resourcerolebinding"
	entrole "kv-shepherd.io/shepherd/ent/role"
	"kv-shepherd.io/shepherd/ent/rolebinding"
	entuser "kv-shepherd.io/shepherd/ent/user"
	"kv-shepherd.io/shepherd/ent/userdirectoryprofile"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/api/middleware"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
)

// ListUsers handles GET /admin/users.
func (s *Server) ListUsers(c *gin.Context, params generated.ListUsersParams) {
	ctx, _, ok := requireActorWithAnyGlobalPermission(c, "user:manage", "rbac:read", "rbac:manage")
	if !ok {
		return
	}

	page, perPage := defaultPagination(params.Page, params.PerPage)
	search := strings.TrimSpace(params.Search)
	profileFieldCatalog, err := listUserProfileFieldCatalog(ctx, s.client)
	if err != nil {
		logger.Error("failed to load user profile field catalog", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	query := s.client.User.Query()
	query = applyUserSearch(query, search, profileFieldCatalog.SearchableFields)
	userList, err := listUsersPage(ctx, query, page, perPage, profileFieldCatalog.SearchableFields)
	if err != nil {
		logger.Error("failed to list users", zap.Error(err), zap.Int("page", page))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	userList.ProfileFields = profileFieldCatalog.Descriptors

	c.JSON(http.StatusOK, userList)
}

// ListSystemMemberCandidates handles GET /systems/{system_id}/member-candidates.
func (s *Server) ListSystemMemberCandidates(
	c *gin.Context,
	systemID generated.SystemID,
	params generated.ListSystemMemberCandidatesParams,
) {
	ctx := c.Request.Context()
	if !requireGlobalPermission(c, "rbac:manage") {
		return
	}
	if _, ok := s.requireSystemRole(c, systemID, "manage_members"); !ok {
		return
	}

	page, perPage := defaultPagination(params.Page, params.PerPage)
	search := strings.TrimSpace(params.Search)

	memberBindings, err := s.client.ResourceRoleBinding.Query().
		Where(
			resourcerolebinding.ResourceTypeEQ("system"),
			resourcerolebinding.ResourceIDEQ(systemID),
		).
		All(ctx)
	if err != nil {
		if isRequestContextCanceled(err) {
			logger.Debug("request canceled while listing system member candidates", zap.Error(err), zap.String("system_id", systemID))
			return
		}
		logger.Error("failed to list system member candidates", zap.Error(err), zap.String("system_id", systemID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	existingMemberUserIDs := make([]string, 0, len(memberBindings))
	seenMemberUserIDs := make(map[string]struct{}, len(memberBindings))
	for _, binding := range memberBindings {
		if _, seen := seenMemberUserIDs[binding.UserID]; seen {
			continue
		}
		seenMemberUserIDs[binding.UserID] = struct{}{}
		existingMemberUserIDs = append(existingMemberUserIDs, binding.UserID)
	}

	query := s.client.User.Query()
	if len(existingMemberUserIDs) > 0 {
		query = query.Where(entuser.Not(entuser.IDIn(existingMemberUserIDs...)))
	}
	query = applyUserSearch(query, search, nil)

	userList, err := listUsersPage(ctx, query, page, perPage, nil)
	if err != nil {
		if isRequestContextCanceled(err) {
			logger.Debug("request canceled while querying system member candidates", zap.Error(err), zap.String("system_id", systemID))
			return
		}
		logger.Error(
			"failed to query system member candidates",
			zap.Error(err),
			zap.String("system_id", systemID),
			zap.Int("page", page),
			zap.String("search", search),
		)
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	c.JSON(http.StatusOK, userList)
}

// ListSystemMembers handles GET /systems/{system_id}/members.
func (s *Server) ListSystemMembers(c *gin.Context, systemID generated.SystemID) {
	ctx := c.Request.Context()
	if !requireGlobalPermission(c, "system:read") {
		return
	}
	if _, ok := s.requireSystemRole(c, systemID, "view"); !ok {
		return
	}

	bindings, err := s.client.ResourceRoleBinding.Query().
		Where(
			resourcerolebinding.ResourceTypeEQ("system"),
			resourcerolebinding.ResourceIDEQ(systemID),
		).
		Order(ent.Asc(resourcerolebinding.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		if isRequestContextCanceled(err) {
			logger.Debug("request canceled while listing system members", zap.Error(err), zap.String("system_id", systemID))
			return
		}
		logger.Error("failed to list system members", zap.Error(err), zap.String("system_id", systemID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	userIDs := make([]string, 0, len(bindings))
	seen := make(map[string]struct{}, len(bindings))
	for _, b := range bindings {
		if _, ok := seen[b.UserID]; ok {
			continue
		}
		seen[b.UserID] = struct{}{}
		userIDs = append(userIDs, b.UserID)
	}

	userByID := make(map[string]*ent.User, len(userIDs))
	if len(userIDs) > 0 {
		users, err := s.client.User.Query().Where(entuser.IDIn(userIDs...)).All(ctx)
		if err != nil {
			if isRequestContextCanceled(err) {
				logger.Debug("request canceled while querying users for members", zap.Error(err), zap.String("system_id", systemID))
				return
			}
			logger.Error("failed to query users for members", zap.Error(err), zap.String("system_id", systemID))
			c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
			return
		}
		for _, u := range users {
			userByID[u.ID] = u
		}
	}

	items := make([]generated.SystemMember, 0, len(bindings))
	for _, b := range bindings {
		items = append(items, toSystemMember(b, userByID[b.UserID]))
	}

	c.JSON(http.StatusOK, generated.SystemMemberList{Items: items})
}

// AddSystemMember handles POST /systems/{system_id}/members.
func (s *Server) AddSystemMember(c *gin.Context, systemID generated.SystemID) {
	ctx := c.Request.Context()
	if !requireGlobalPermission(c, "rbac:manage") {
		return
	}
	actor, ok := s.requireSystemRole(c, systemID, "manage_members")
	if !ok {
		return
	}

	var req generated.SystemMemberCreateRequest
	if !bindAndValidateJSON(c, &req) {
		return
	}

	roleName := string(req.Role)
	if !isValidMemberRole(roleName) {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_ROLE"})
		return
	}

	userEnt, err := s.client.User.Get(ctx, req.UserId)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "USER_NOT_FOUND"})
			return
		}
		logger.Error("failed to get user for member add", zap.Error(err), zap.String("user_id", req.UserId))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	id, _ := uuid.NewV7()
	member, err := s.client.ResourceRoleBinding.Create().
		SetID(id.String()).
		SetUserID(req.UserId).
		SetResourceType("system").
		SetResourceID(systemID).
		SetRole(resourcerolebinding.Role(roleName)).
		SetCreatedBy(actor).
		Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			c.JSON(http.StatusConflict, generated.Error{Code: "MEMBER_ALREADY_EXISTS"})
			return
		}
		logger.Error("failed to add system member",
			zap.Error(err),
			zap.String("system_id", systemID),
			zap.String("user_id", req.UserId),
		)
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if s.audit != nil {
		_ = s.audit.LogAction(ctx, "system.member.add", "system", systemID, actor, map[string]interface{}{
			"user_id": req.UserId,
			"role":    roleName,
		})
	}

	c.JSON(http.StatusCreated, toSystemMember(member, userEnt))
}

// UpdateSystemMemberRole handles PATCH /systems/{system_id}/members/{user_id}.
func (s *Server) UpdateSystemMemberRole(c *gin.Context, systemID generated.SystemID, userID generated.UserID) {
	ctx := c.Request.Context()
	if !requireGlobalPermission(c, "rbac:manage") {
		return
	}
	actor, ok := s.requireSystemRole(c, systemID, "manage_members")
	if !ok {
		return
	}

	var req generated.SystemMemberRoleUpdateRequest
	if !bindAndValidateJSON(c, &req) {
		return
	}

	roleName := string(req.Role)
	if !isValidMemberRole(roleName) {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_ROLE"})
		return
	}

	existing, err := s.client.ResourceRoleBinding.Query().
		Where(
			resourcerolebinding.UserIDEQ(userID),
			resourcerolebinding.ResourceTypeEQ("system"),
			resourcerolebinding.ResourceIDEQ(systemID),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "MEMBER_NOT_FOUND"})
			return
		}
		logger.Error("failed to query member for role update",
			zap.Error(err),
			zap.String("system_id", systemID),
			zap.String("user_id", userID),
		)
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	updated, err := s.client.ResourceRoleBinding.UpdateOneID(existing.ID).
		SetRole(resourcerolebinding.Role(roleName)).
		Save(ctx)
	if err != nil {
		logger.Error("failed to update member role",
			zap.Error(err),
			zap.String("system_id", systemID),
			zap.String("user_id", userID),
		)
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	userEnt, err := s.client.User.Get(ctx, userID)
	if err != nil && !ent.IsNotFound(err) {
		logger.Error("failed to get user after role update", zap.Error(err), zap.String("user_id", userID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if s.audit != nil {
		_ = s.audit.LogAction(ctx, "system.member.update_role", "system", systemID, actor, map[string]interface{}{
			"user_id":  userID,
			"old_role": existing.Role.String(),
			"new_role": roleName,
		})
	}

	c.JSON(http.StatusOK, toSystemMember(updated, userEnt))
}

// DeleteSystemMember handles DELETE /systems/{system_id}/members/{user_id}.
func (s *Server) DeleteSystemMember(c *gin.Context, systemID generated.SystemID, userID generated.UserID) {
	ctx := c.Request.Context()
	if !requireGlobalPermission(c, "rbac:manage") {
		return
	}
	actor, ok := s.requireSystemRole(c, systemID, "manage_members")
	if !ok {
		return
	}

	member, err := s.client.ResourceRoleBinding.Query().
		Where(
			resourcerolebinding.UserIDEQ(userID),
			resourcerolebinding.ResourceTypeEQ("system"),
			resourcerolebinding.ResourceIDEQ(systemID),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "MEMBER_NOT_FOUND"})
			return
		}
		logger.Error("failed to query member for delete",
			zap.Error(err),
			zap.String("system_id", systemID),
			zap.String("user_id", userID),
		)
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if member.Role == resourcerolebinding.RoleOwner {
		ownerCount, err := s.client.ResourceRoleBinding.Query().
			Where(
				resourcerolebinding.ResourceTypeEQ("system"),
				resourcerolebinding.ResourceIDEQ(systemID),
				resourcerolebinding.RoleEQ(resourcerolebinding.RoleOwner),
			).
			Count(ctx)
		if err != nil {
			logger.Error("failed to count system owners", zap.Error(err), zap.String("system_id", systemID))
			c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
			return
		}
		if ownerCount <= 1 {
			c.JSON(http.StatusConflict, generated.Error{
				Code:    "LAST_OWNER_CANNOT_BE_REMOVED",
				Message: "system must have at least one owner",
			})
			return
		}
	}

	if err := s.client.ResourceRoleBinding.DeleteOneID(member.ID).Exec(ctx); err != nil {
		logger.Error("failed to delete member",
			zap.Error(err),
			zap.String("system_id", systemID),
			zap.String("user_id", userID),
		)
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if s.audit != nil {
		_ = s.audit.LogAction(ctx, "system.member.remove", "system", systemID, actor, map[string]interface{}{
			"user_id": userID,
			"role":    member.Role.String(),
		})
	}

	c.Status(http.StatusNoContent)
}

func (s *Server) requireSystemRole(c *gin.Context, systemID, action string) (string, bool) {
	ctx := c.Request.Context()
	actor := middleware.GetUserID(ctx)
	if actor == "" {
		c.JSON(http.StatusUnauthorized, generated.Error{Code: "UNAUTHORIZED"})
		return "", false
	}

	if _, err := s.client.System.Get(ctx, systemID); err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "SYSTEM_NOT_FOUND"})
			return "", false
		}
		if isRequestContextCanceled(err) {
			logger.Debug("request canceled while loading system for member operation", zap.Error(err), zap.String("system_id", systemID))
			return "", false
		}
		logger.Error("failed to get system for member operation", zap.Error(err), zap.String("system_id", systemID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return "", false
	}

	if hasPlatformAdmin(c) {
		return actor, true
	}

	checker := middleware.NewResourceRoleChecker(s.client)
	bindingRole, found, err := checker.CheckResourceRole(ctx, actor, "system", systemID)
	if err != nil {
		if isRequestContextCanceled(err) {
			logger.Debug("request canceled while checking system role", zap.Error(err), zap.String("system_id", systemID), zap.String("actor", actor))
			return "", false
		}
		logger.Error("failed to check system role",
			zap.Error(err),
			zap.String("system_id", systemID),
			zap.String("actor", actor),
		)
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return "", false
	}
	if !found || !middleware.RoleCanPerform(bindingRole, action) {
		c.JSON(http.StatusForbidden, generated.Error{Code: "FORBIDDEN"})
		return "", false
	}

	return actor, true
}

func hasPlatformAdmin(c *gin.Context) bool {
	perms, exists := c.Get("permissions")
	if !exists {
		return false
	}
	permList, ok := perms.([]string)
	if !ok {
		return false
	}
	return slices.Contains(permList, "platform:admin")
}

func isValidMemberRole(roleName string) bool {
	switch roleName {
	case resourcerolebinding.RoleOwner.String(),
		resourcerolebinding.RoleAdmin.String(),
		resourcerolebinding.RoleMember.String(),
		resourcerolebinding.RoleViewer.String():
		return true
	default:
		return false
	}
}

func listUsersPage(
	ctx context.Context,
	query *ent.UserQuery,
	page,
	perPage int,
	profileFields []string,
) (generated.UserList, error) {
	offset := (page - 1) * perPage

	total, err := query.Clone().Count(ctx)
	if err != nil {
		return generated.UserList{}, err
	}

	users, err := query.
		Offset(offset).
		Limit(perPage).
		Order(ent.Asc(entuser.FieldUsername)).
		WithRoleBindings(func(q *ent.RoleBindingQuery) {
			q.WithRole()
		}).
		WithDirectoryProfile().
		All(ctx)
	if err != nil {
		return generated.UserList{}, err
	}

	items := make([]generated.User, 0, len(users))
	for _, userEnt := range users {
		items = append(items, toGeneratedUser(userEnt, profileFields))
	}

	totalPages := (total + perPage - 1) / perPage
	return generated.UserList{
		Items: items,
		Pagination: generated.Pagination{
			Page:       page,
			PerPage:    perPage,
			Total:      total,
			TotalPages: totalPages,
		},
	}, nil
}

func applyUserSearch(query *ent.UserQuery, search string, searchableProfileFields []string) *ent.UserQuery {
	terms := parseUserSearchTerms(search)
	if len(terms) == 0 {
		return query
	}
	normalizedProfileFields := make(map[string]string, len(searchableProfileFields))
	for _, field := range searchableProfileFields {
		normalizedProfileFields[normalizeUserSearchField(field)] = field
	}

	predicates := make([]predicate.User, 0, len(terms))
	for _, term := range terms {
		predicateForTerm := buildUserSearchPredicate(term, normalizedProfileFields, searchableProfileFields)
		if predicateForTerm == nil {
			continue
		}
		predicates = append(predicates, predicateForTerm)
	}
	if len(predicates) == 0 {
		return query
	}
	return query.Where(entuser.And(predicates...))
}

func toGeneratedUser(userEnt *ent.User, profileFields []string) generated.User {
	roleSet := make(map[string]struct{})
	for _, rb := range userEnt.Edges.RoleBindings {
		if rb == nil || rb.Edges.Role == nil {
			continue
		}
		roleSet[rb.Edges.Role.Name] = struct{}{}
	}

	roles := make([]string, 0, len(roleSet))
	for roleName := range roleSet {
		roles = append(roles, roleName)
	}
	sort.Strings(roles)

	profileAttributes := make(map[string]interface{}, len(profileFields))
	if profile := userEnt.Edges.DirectoryProfile; profile != nil {
		for _, field := range profileFields {
			value := stringifyUserDirectoryAttribute(profile.Attributes[field])
			if value == "" {
				continue
			}
			profileAttributes[field] = value
		}
	}

	return generated.User{
		Id:                userEnt.ID,
		Username:          userEnt.Username,
		Email:             userEnt.Email,
		DisplayName:       userEnt.DisplayName,
		Enabled:           userEnt.Enabled,
		Roles:             roles,
		ProfileAttributes: profileAttributes,
		CreatedAt:         userEnt.CreatedAt,
	}
}

type userProfileFieldCatalog struct {
	SearchableFields []string
	Descriptors      []generated.UserProfileField
}

type userSearchTerm struct {
	Field string
	Value string
}

var userSearchFieldSplitPattern = regexp.MustCompile(`^([^:\s]+):(.*)$`)

func listUserProfileFieldCatalog(ctx context.Context, client *ent.Client) (userProfileFieldCatalog, error) {
	profiles, err := client.UserDirectoryProfile.Query().All(ctx)
	if err != nil {
		return userProfileFieldCatalog{}, err
	}

	observedSet := make(map[string]struct{})
	observedFields := make([]string, 0)
	for _, profile := range profiles {
		for key := range profile.Attributes {
			normalizedKey := strings.TrimSpace(key)
			if normalizedKey == "" {
				continue
			}
			if _, exists := observedSet[normalizedKey]; exists {
				continue
			}
			observedSet[normalizedKey] = struct{}{}
			observedFields = append(observedFields, normalizedKey)
		}
	}
	sort.Strings(observedFields)

	searchableFields := make([]string, 0, len(observedFields))
	descriptors := make([]generated.UserProfileField, 0, len(observedFields))
	for _, field := range observedFields {
		searchableFields = append(searchableFields, field)
		descriptors = append(descriptors, generated.UserProfileField{
			Key:        field,
			Label:      humanizeUserProfileFieldLabel(field),
			Searchable: true,
		})
	}
	return userProfileFieldCatalog{
		SearchableFields: searchableFields,
		Descriptors:      descriptors,
	}, nil
}

func parseUserSearchTerms(search string) []userSearchTerm {
	tokens := splitUserSearchTokens(strings.TrimSpace(search))
	terms := make([]userSearchTerm, 0, len(tokens))
	for _, token := range tokens {
		if token == "" {
			continue
		}
		matches := userSearchFieldSplitPattern.FindStringSubmatch(token)
		if len(matches) == 3 {
			field := strings.TrimSpace(matches[1])
			value := unquoteUserSearchTermValue(strings.TrimSpace(matches[2]))
			if field != "" && value != "" {
				terms = append(terms, userSearchTerm{Field: field, Value: value})
				continue
			}
		}
		terms = append(terms, userSearchTerm{Value: unquoteUserSearchTermValue(token)})
	}
	return terms
}

func splitUserSearchTokens(search string) []string {
	if strings.TrimSpace(search) == "" {
		return nil
	}
	tokens := make([]string, 0)
	var current strings.Builder
	inQuotes := false
	escaped := false

	flush := func() {
		token := strings.TrimSpace(current.String())
		if token != "" {
			tokens = append(tokens, token)
		}
		current.Reset()
	}

	for _, r := range search {
		switch {
		case escaped:
			current.WriteRune(r)
			escaped = false
		case r == '\\' && inQuotes:
			current.WriteRune(r)
			escaped = true
		case r == '"':
			current.WriteRune(r)
			inQuotes = !inQuotes
		case unicode.IsSpace(r) && !inQuotes:
			flush()
		default:
			current.WriteRune(r)
		}
	}
	flush()
	return tokens
}

func unquoteUserSearchTermValue(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 && strings.HasPrefix(value, `"`) && strings.HasSuffix(value, `"`) {
		if unquoted, err := strconv.Unquote(value); err == nil {
			return strings.TrimSpace(unquoted)
		}
		return strings.Trim(strings.TrimSpace(value), `"`)
	}
	return value
}

func buildUserSearchPredicate(
	term userSearchTerm,
	normalizedProfileFields map[string]string,
	searchableProfileFields []string,
) predicate.User {
	if strings.TrimSpace(term.Value) == "" {
		return nil
	}
	if term.Field != "" {
		switch normalizeUserSearchField(term.Field) {
		case "username":
			return entuser.UsernameEqualFold(term.Value)
		case "display_name", "displayname", "name":
			return entuser.DisplayNameEqualFold(term.Value)
		case "email", "mail":
			return entuser.EmailEqualFold(term.Value)
		case "status", "enabled":
			if enabled, ok := parseUserEnabledSearchValue(term.Value); ok {
				return entuser.EnabledEQ(enabled)
			}
			return impossibleUserSearchPredicate()
		case "role", "roles":
			return userHasRoleNameEqualFold(term.Value)
		default:
			if field, ok := normalizedProfileFields[normalizeUserSearchField(term.Field)]; ok {
				return userProfileAttributeEqualFold(field, term.Value)
			}
			return impossibleUserSearchPredicate()
		}
	}

	predicates := []predicate.User{
		entuser.IDContainsFold(term.Value),
		entuser.UsernameContainsFold(term.Value),
		entuser.DisplayNameContainsFold(term.Value),
		entuser.EmailContainsFold(term.Value),
		userHasRoleNameContainsFold(term.Value),
	}
	for _, field := range searchableProfileFields {
		predicates = append(predicates, userProfileAttributeContains(field, term.Value))
	}
	return entuser.Or(predicates...)
}

func impossibleUserSearchPredicate() predicate.User {
	return predicate.User(func(s *sql.Selector) {
		s.Where(sql.P(func(builder *sql.Builder) {
			builder.WriteString("1 = 0")
		}))
	})
}

func userProfileAttributeContains(field, value string) predicate.User {
	field = strings.TrimSpace(field)
	value = strings.TrimSpace(value)
	return entuser.HasDirectoryProfileWith(predicate.UserDirectoryProfile(func(s *sql.Selector) {
		s.Where(sql.P(func(builder *sql.Builder) {
			builder.WriteString("LOWER(")
			builder.Join(sqljson.ValuePath(userdirectoryprofile.FieldAttributes, sqljson.Path(field), sqljson.Unquote(true)))
			builder.WriteString(") LIKE ")
			builder.Arg("%" + strings.ToLower(value) + "%")
		}))
	}))
}

func userProfileAttributeEqualFold(field, value string) predicate.User {
	field = strings.TrimSpace(field)
	value = strings.TrimSpace(value)
	return entuser.HasDirectoryProfileWith(predicate.UserDirectoryProfile(func(s *sql.Selector) {
		s.Where(sql.P(func(builder *sql.Builder) {
			builder.WriteString("LOWER(")
			builder.Join(sqljson.ValuePath(userdirectoryprofile.FieldAttributes, sqljson.Path(field), sqljson.Unquote(true)))
			builder.WriteString(") = ")
			builder.Arg(strings.ToLower(value))
		}))
	}))
}

func userHasRoleNameContainsFold(value string) predicate.User {
	return entuser.HasRoleBindingsWith(
		rolebinding.HasRoleWith(entrole.Or(
			entrole.NameContainsFold(value),
			entrole.DisplayNameContainsFold(value),
			entrole.IDContainsFold(value),
		)),
	)
}

func userHasRoleNameEqualFold(value string) predicate.User {
	return entuser.HasRoleBindingsWith(
		rolebinding.HasRoleWith(entrole.Or(
			entrole.NameEqualFold(value),
			entrole.DisplayNameEqualFold(value),
			entrole.IDEqualFold(value),
		)),
	)
}

func parseUserEnabledSearchValue(value string) (enabled, ok bool) {
	switch normalizeUserSearchField(value) {
	case "enabled", "active", "true", "yes", "y", "1", "on":
		return true, true
	case "disabled", "inactive", "false", "no", "n", "0", "off":
		return false, true
	default:
		return false, false
	}
}

func normalizeUserSearchField(raw string) string {
	raw = strings.TrimSpace(strings.ToLower(raw))
	raw = strings.ReplaceAll(raw, "-", "_")
	raw = strings.ReplaceAll(raw, " ", "_")
	return raw
}

func humanizeUserProfileFieldLabel(field string) string {
	field = strings.TrimSpace(field)
	if field == "" {
		return ""
	}
	parts := strings.FieldsFunc(field, func(r rune) bool {
		return r == '_' || r == '-' || unicode.IsSpace(r)
	})
	for index, part := range parts {
		if part == "" {
			continue
		}
		runes := []rune(strings.ToLower(part))
		runes[0] = unicode.ToUpper(runes[0])
		parts[index] = string(runes)
	}
	return strings.Join(parts, " ")
}

func stringifyUserDirectoryAttribute(raw interface{}) string {
	switch value := raw.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(value)
	case []string:
		return strings.Join(value, ", ")
	case []interface{}:
		values := make([]string, 0, len(value))
		for _, item := range value {
			text := strings.TrimSpace(fmt.Sprint(item))
			if text == "" {
				continue
			}
			values = append(values, text)
		}
		return strings.Join(values, ", ")
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

func toSystemMember(binding *ent.ResourceRoleBinding, user *ent.User) generated.SystemMember {
	member := generated.SystemMember{
		UserId:    binding.UserID,
		Username:  binding.UserID,
		Role:      generated.SystemMemberRole(binding.Role.String()),
		CreatedAt: binding.CreatedAt,
	}
	if user == nil {
		return member
	}
	member.Username = user.Username
	member.Email = user.Email
	member.DisplayName = user.DisplayName
	return member
}
