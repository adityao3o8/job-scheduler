package api

import (
	"context"
	"net/http"
	"regexp"

	"codity.ai/scheduler/internal/domain"
	"codity.ai/scheduler/pkg/auth"
)

type AuthHandler struct {
	users    domain.UserRepository
	orgs     domain.OrgRepository
	tx       domain.TxManager
	jwt      *auth.JWTService
	demoAuth bool
}

func NewAuthHandler(users domain.UserRepository, orgs domain.OrgRepository, tx domain.TxManager, jwt *auth.JWTService, demoAuth bool) *AuthHandler {
	return &AuthHandler{users: users, orgs: orgs, tx: tx, jwt: jwt, demoAuth: demoAuth}
}

type registerRequest struct {
	OrgName  string `json:"org_name"`
	OrgSlug  string `json:"org_slug"`
	Email    string `json:"email"`
	Name     string `json:"name"`
	Password string `json:"password"`
}

type authResponse struct {
	Token string       `json:"token"`
	User  *domain.User `json:"user"`
}

var slugRe = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := readJSON(r, &req); err != nil {
		mapDomainError(w, err)
		return
	}

	if errs := validateRegister(req); len(errs) > 0 {
		writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "invalid input", errs)
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "internal server error", nil)
		return
	}

	org := &domain.Organization{Name: req.OrgName, Slug: req.OrgSlug}
	user := &domain.User{Email: req.Email, Name: req.Name, PasswordHash: hash, Role: "admin"}

	err = h.tx.WithTx(r.Context(), func(ctx context.Context) error {
		if err := h.orgs.Create(ctx, org); err != nil {
			return err
		}
		user.OrgID = org.ID
		return h.users.Create(ctx, user)
	})
	if err != nil {
		mapDomainError(w, err)
		return
	}

	token, err := h.jwt.GenerateToken(domain.Claims{
		UserID: user.ID, OrgID: org.ID, Email: user.Email, Role: user.Role,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "internal server error", nil)
		return
	}

	writeJSON(w, http.StatusCreated, authResponse{Token: token, User: user})
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := readJSON(r, &req); err != nil {
		mapDomainError(w, err)
		return
	}

	if req.Email == "" || req.Password == "" {
		writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "email and password required", nil)
		return
	}

	var user *domain.User
	var err error

	if h.demoAuth {
		user, err = h.resolveDemoUser(r.Context(), req.Email)
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE",
				"demo login requires seeded data — run make seed", nil)
			return
		}
	} else {
		user, err = h.users.GetByEmail(r.Context(), req.Email)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid credentials", nil)
			return
		}

		if err := auth.CheckPassword(user.PasswordHash, req.Password); err != nil {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid credentials", nil)
			return
		}
	}

	token, err := h.jwt.GenerateToken(domain.Claims{
		UserID: user.ID, OrgID: user.OrgID, Email: user.Email, Role: user.Role,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "internal server error", nil)
		return
	}

	writeJSON(w, http.StatusOK, authResponse{Token: token, User: user})
}

const demoFallbackEmail = "admin@demo.com"

// resolveDemoUser maps any recruiter login to a real DB user so the API stays authorized.
// Uses the entered email when it exists; otherwise falls back to the seeded demo admin.
func (h *AuthHandler) resolveDemoUser(ctx context.Context, email string) (*domain.User, error) {
	user, err := h.users.GetByEmail(ctx, email)
	if err == nil {
		return user, nil
	}
	return h.users.GetByEmail(ctx, demoFallbackEmail)
}

func validateRegister(r registerRequest) map[string]string {
	errs := make(map[string]string)
	if r.OrgName == "" {
		errs["org_name"] = "required"
	}
	if !slugRe.MatchString(r.OrgSlug) {
		errs["org_slug"] = "must be lowercase alphanumeric with hyphens"
	}
	if r.Email == "" {
		errs["email"] = "required"
	}
	if r.Name == "" {
		errs["name"] = "required"
	}
	if len(r.Password) < 8 {
		errs["password"] = "minimum 8 characters"
	}
	return errs
}
