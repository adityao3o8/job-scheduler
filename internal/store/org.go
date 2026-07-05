package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"codity.ai/scheduler/internal/domain"
)

type OrgRepository struct{ pool *pgxpool.Pool }

func NewOrgRepository(pool *pgxpool.Pool) *OrgRepository {
	return &OrgRepository{pool: pool}
}

func (r *OrgRepository) Create(ctx context.Context, org *domain.Organization) error {
	const q = `
		INSERT INTO organizations (id, name, slug)
		VALUES ($1, $2, $3)
		RETURNING created_at, updated_at`

	if org.ID == uuid.Nil {
		org.ID = uuid.New()
	}
	err := conn(ctx, r.pool).QueryRow(ctx, q, org.ID, org.Name, org.Slug).
		Scan(&org.CreatedAt, &org.UpdatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("org slug %q: %w", org.Slug, domain.ErrConflict)
		}
		return fmt.Errorf("insert org: %w", err)
	}
	return nil
}

func (r *OrgRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Organization, error) {
	const q = `SELECT id, name, slug, created_at, updated_at FROM organizations WHERE id = $1`
	return scanOrg(conn(ctx, r.pool).QueryRow(ctx, q, id))
}

func (r *OrgRepository) GetBySlug(ctx context.Context, slug string) (*domain.Organization, error) {
	const q = `SELECT id, name, slug, created_at, updated_at FROM organizations WHERE slug = $1`
	return scanOrg(conn(ctx, r.pool).QueryRow(ctx, q, slug))
}

func (r *OrgRepository) Update(ctx context.Context, org *domain.Organization) error {
	const q = `
		UPDATE organizations SET name = $2, slug = $3, updated_at = NOW()
		WHERE id = $1
		RETURNING updated_at`

	err := conn(ctx, r.pool).QueryRow(ctx, q, org.ID, org.Name, org.Slug).
		Scan(&org.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("org %s: %w", org.ID, domain.ErrNotFound)
		}
		if isUniqueViolation(err) {
			return fmt.Errorf("org slug %q: %w", org.Slug, domain.ErrConflict)
		}
		return fmt.Errorf("update org: %w", err)
	}
	return nil
}

func (r *OrgRepository) Delete(ctx context.Context, id uuid.UUID) error {
	const q = `DELETE FROM organizations WHERE id = $1`
	tag, err := conn(ctx, r.pool).Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("delete org: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("org %s: %w", id, domain.ErrNotFound)
	}
	return nil
}

func scanOrg(row pgx.Row) (*domain.Organization, error) {
	var o domain.Organization
	err := row.Scan(&o.ID, &o.Name, &o.Slug, &o.CreatedAt, &o.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("scan org: %w", err)
	}
	return &o, nil
}
