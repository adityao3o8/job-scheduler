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

type QueueRepository struct{ pool *pgxpool.Pool }

func NewQueueRepository(pool *pgxpool.Pool) *QueueRepository {
	return &QueueRepository{pool: pool}
}

const queueSelectCols = `q.id, q.project_id, q.name, q.slug, q.retry_policy_id,
	q.priority_default, q.concurrency_limit, q.paused, q.created_at, q.updated_at`

func (r *QueueRepository) Create(ctx context.Context, q *domain.Queue) error {
	const query = `
		INSERT INTO queues (id, project_id, name, slug, retry_policy_id,
			priority_default, concurrency_limit, paused)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING created_at, updated_at`

	if q.ID == uuid.Nil {
		q.ID = uuid.New()
	}
	err := conn(ctx, r.pool).QueryRow(ctx, query,
		q.ID, q.ProjectID, q.Name, q.Slug, q.RetryPolicyID,
		q.PriorityDefault, q.ConcurrencyLimit, q.IsPaused,
	).Scan(&q.CreatedAt, &q.UpdatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("queue slug %q: %w", q.Slug, domain.ErrConflict)
		}
		return fmt.Errorf("insert queue: %w", err)
	}
	return nil
}

// GetByID scopes the lookup through the project's org_id.
func (r *QueueRepository) GetByID(ctx context.Context, id, orgID uuid.UUID) (*domain.Queue, error) {
	q := `SELECT ` + queueSelectCols + `
		FROM queues q
		JOIN projects p ON p.id = q.project_id
		WHERE q.id = $1 AND p.org_id = $2`
	return scanQueue(conn(ctx, r.pool).QueryRow(ctx, q, id, orgID))
}

func (r *QueueRepository) List(ctx context.Context, orgID uuid.UUID, params domain.PageParams, filters domain.QueueFilters) (*domain.Page[domain.Queue], error) {
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

	cond = append(cond, fmt.Sprintf("p.org_id = $%d", argN))
	args = append(args, orgID)
	argN++

	if filters.Name != "" {
		cond = append(cond, fmt.Sprintf("q.name ILIKE $%d", argN))
		args = append(args, "%"+filters.Name+"%")
		argN++
	}
	if filters.IsPaused != nil {
		cond = append(cond, fmt.Sprintf("q.paused = $%d", argN))
		args = append(args, *filters.IsPaused)
		argN++
	}
	if filters.ProjectID != nil {
		cond = append(cond, fmt.Sprintf("q.project_id = $%d", argN))
		args = append(args, *filters.ProjectID)
		argN++
	}
	if cursor != nil {
		cond = append(cond, fmt.Sprintf("(q.created_at, q.id) > ($%d, $%d)", argN, argN+1))
		args = append(args, cursor.CreatedAt, cursor.ID)
		argN += 2
	}

	query := `SELECT ` + queueSelectCols + `
		FROM queues q
		JOIN projects p ON p.id = q.project_id
		WHERE ` + strings.Join(cond, " AND ") +
		` ORDER BY q.created_at ASC, q.id ASC LIMIT ` + fmt.Sprintf("$%d", argN)
	args = append(args, limit+1)

	rows, err := conn(ctx, r.pool).Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list queues: %w", err)
	}
	defer rows.Close()

	var items []domain.Queue
	for rows.Next() {
		qr, err := scanQueueRow(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *qr)
	}

	page := &domain.Page[domain.Queue]{HasMore: len(items) > limit}
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

func (r *QueueRepository) Update(ctx context.Context, q *domain.Queue, orgID uuid.UUID) error {
	const query = `
		UPDATE queues SET
			name             = $3,
			slug             = $4,
			retry_policy_id  = $5,
			priority_default = $6,
			concurrency_limit = $7,
			paused           = $8,
			updated_at       = NOW()
		FROM projects p
		WHERE queues.id = $1 AND queues.project_id = p.id AND p.org_id = $2
		RETURNING queues.updated_at`

	err := conn(ctx, r.pool).QueryRow(ctx, query,
		q.ID, orgID, q.Name, q.Slug, q.RetryPolicyID,
		q.PriorityDefault, q.ConcurrencyLimit, q.IsPaused,
	).Scan(&q.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("queue %s: %w", q.ID, domain.ErrNotFound)
		}
		if isUniqueViolation(err) {
			return fmt.Errorf("queue slug %q: %w", q.Slug, domain.ErrConflict)
		}
		return fmt.Errorf("update queue: %w", err)
	}
	return nil
}

func (r *QueueRepository) Delete(ctx context.Context, id, orgID uuid.UUID) error {
	const q = `
		DELETE FROM queues
		USING projects p
		WHERE queues.id = $1 AND queues.project_id = p.id AND p.org_id = $2`
	tag, err := conn(ctx, r.pool).Exec(ctx, q, id, orgID)
	if err != nil {
		return fmt.Errorf("delete queue: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("queue %s: %w", id, domain.ErrNotFound)
	}
	return nil
}

func (r *QueueRepository) SetPaused(ctx context.Context, id, orgID uuid.UUID, paused bool) error {
	const q = `
		UPDATE queues SET paused = $3, updated_at = NOW()
		FROM projects p
		WHERE queues.id = $1 AND queues.project_id = p.id AND p.org_id = $2`
	tag, err := conn(ctx, r.pool).Exec(ctx, q, id, orgID, paused)
	if err != nil {
		return fmt.Errorf("set paused queue %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("queue %s: %w", id, domain.ErrNotFound)
	}
	return nil
}

func scanQueue(row pgx.Row) (*domain.Queue, error) {
	var q domain.Queue
	err := row.Scan(&q.ID, &q.ProjectID, &q.Name, &q.Slug, &q.RetryPolicyID,
		&q.PriorityDefault, &q.ConcurrencyLimit, &q.IsPaused, &q.CreatedAt, &q.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("scan queue: %w", err)
	}
	return &q, nil
}

func scanQueueRow(rows pgx.Rows) (*domain.Queue, error) {
	var q domain.Queue
	err := rows.Scan(&q.ID, &q.ProjectID, &q.Name, &q.Slug, &q.RetryPolicyID,
		&q.PriorityDefault, &q.ConcurrencyLimit, &q.IsPaused, &q.CreatedAt, &q.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("scan queue row: %w", err)
	}
	return &q, nil
}
