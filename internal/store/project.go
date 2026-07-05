package store

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"codity.ai/scheduler/internal/domain"
	"codity.ai/scheduler/pkg/pagination"
)

type ProjectRepository struct{ pool *pgxpool.Pool }

func NewProjectRepository(pool *pgxpool.Pool) *ProjectRepository {
	return &ProjectRepository{pool: pool}
}

const projCols = `id, org_id, name, slug, description, created_at, updated_at`

func (r *ProjectRepository) Create(ctx context.Context, p *domain.Project) error {
	const q = `
		INSERT INTO projects (id, org_id, name, slug, description)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING created_at, updated_at`

	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	err := conn(ctx, r.pool).QueryRow(ctx, q,
		p.ID, p.OrgID, p.Name, p.Slug, p.Description,
	).Scan(&p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("project slug %q: %w", p.Slug, domain.ErrConflict)
		}
		return fmt.Errorf("insert project: %w", err)
	}
	return nil
}

func (r *ProjectRepository) GetByID(ctx context.Context, id, orgID uuid.UUID) (*domain.Project, error) {
	q := `SELECT ` + projCols + ` FROM projects WHERE id = $1 AND org_id = $2`
	return scanProject(conn(ctx, r.pool).QueryRow(ctx, q, id, orgID))
}

func (r *ProjectRepository) List(ctx context.Context, orgID uuid.UUID, params domain.PageParams, nameFilter string) (*domain.Page[domain.Project], error) {
	limit := pagination.ClampLimit(params.Limit)
	cursor, err := pagination.DecodeCursor(params.Cursor)
	if err != nil {
		return nil, fmt.Errorf("decode cursor: %w", err)
	}

	var (
		args []any
		cond []string
	)
	argN := 1

	cond = append(cond, fmt.Sprintf("org_id = $%d", argN))
	args = append(args, orgID)
	argN++

	if nameFilter != "" {
		cond = append(cond, fmt.Sprintf("name ILIKE $%d", argN))
		args = append(args, "%"+nameFilter+"%")
		argN++
	}
	if cursor != nil {
		cond = append(cond, fmt.Sprintf("(created_at, id) > ($%d, $%d)", argN, argN+1))
		args = append(args, cursor.CreatedAt, cursor.ID)
		argN += 2
	}

	q := `SELECT ` + projCols + ` FROM projects WHERE ` +
		strings.Join(cond, " AND ") +
		` ORDER BY created_at ASC, id ASC LIMIT ` + fmt.Sprintf("$%d", argN)
	args = append(args, limit+1)

	rows, err := conn(ctx, r.pool).Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	defer rows.Close()

	var items []domain.Project
	for rows.Next() {
		p, err := scanProjectRow(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *p)
	}

	page := &domain.Page[domain.Project]{HasMore: len(items) > limit}
	if page.HasMore {
		items = items[:limit]
	}
	if len(items) > 0 && page.HasMore {
		last := items[len(items)-1]
		page.NextCursor = pagination.EncodeCursor(last.CreatedAt, last.ID)
	}
	page.Items = items
	return page, nil
}

func (r *ProjectRepository) Update(ctx context.Context, p *domain.Project) error {
	const q = `
		UPDATE projects
		SET name = $3, slug = $4, description = $5, updated_at = NOW()
		WHERE id = $1 AND org_id = $2
		RETURNING updated_at`

	err := conn(ctx, r.pool).QueryRow(ctx, q,
		p.ID, p.OrgID, p.Name, p.Slug, p.Description,
	).Scan(&p.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("project %s: %w", p.ID, domain.ErrNotFound)
		}
		if isUniqueViolation(err) {
			return fmt.Errorf("project slug %q: %w", p.Slug, domain.ErrConflict)
		}
		return fmt.Errorf("update project: %w", err)
	}
	return nil
}

func (r *ProjectRepository) Delete(ctx context.Context, id, orgID uuid.UUID) error {
	const q = `DELETE FROM projects WHERE id = $1 AND org_id = $2`
	tag, err := conn(ctx, r.pool).Exec(ctx, q, id, orgID)
	if err != nil {
		return fmt.Errorf("delete project: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("project %s: %w", id, domain.ErrNotFound)
	}
	return nil
}

func scanProject(row pgx.Row) (*domain.Project, error) {
	var p domain.Project
	err := row.Scan(&p.ID, &p.OrgID, &p.Name, &p.Slug, &p.Description, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("scan project: %w", err)
	}
	return &p, nil
}

func scanProjectRow(rows pgx.Rows) (*domain.Project, error) {
	var p domain.Project
	err := rows.Scan(&p.ID, &p.OrgID, &p.Name, &p.Slug, &p.Description, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("scan project row: %w", err)
	}
	return &p, nil
}
