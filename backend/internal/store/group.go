package store

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/0x2E/fusion/internal/model"
)

func (s *Store) ListGroups() ([]*model.Group, error) {
	rows, err := s.db.Query(`
		SELECT id, name, sort_order, created_at, updated_at
		FROM groups
		ORDER BY sort_order ASC, id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	groups := []*model.Group{}
	for rows.Next() {
		g := &model.Group{}
		if err := rows.Scan(&g.ID, &g.Name, &g.SortOrder, &g.CreatedAt, &g.UpdatedAt); err != nil {
			return nil, err
		}
		groups = append(groups, g)
	}
	return groups, rows.Err()
}

func (s *Store) GetGroup(id int64) (*model.Group, error) {
	g := &model.Group{}
	err := s.db.QueryRow(`
		SELECT id, name, sort_order, created_at, updated_at
		FROM groups
		WHERE id = :id
	`, sql.Named("id", id)).Scan(&g.ID, &g.Name, &g.SortOrder, &g.CreatedAt, &g.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: group", ErrNotFound)
		}
		return nil, fmt.Errorf("get group: %w", err)
	}
	return g, nil
}

func (s *Store) CreateGroup(name string) (*model.Group, error) {
	result, err := s.db.Exec(`
		INSERT INTO groups (name, sort_order)
		VALUES (:name, COALESCE((SELECT MAX(sort_order) FROM groups), 0) + 1)
	`, sql.Named("name", name))
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return s.GetGroup(id)
}

func (s *Store) UpdateGroup(id int64, name string) error {
	result, err := s.db.Exec(`
		UPDATE groups
		SET name = :name, updated_at = unixepoch()
		WHERE id = :id
	`, sql.Named("name", name), sql.Named("id", id))
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("%w: group", ErrNotFound)
	}
	return nil
}

// DeleteGroup removes a group and moves all its feeds to the default group (ID=1).
// The default group itself cannot be deleted to ensure all feeds have a valid group.
func (s *Store) DeleteGroup(id int64) error {
	if id == 1 {
		return fmt.Errorf("%w: cannot delete default group", ErrInvalid)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`UPDATE feeds SET group_id = 1 WHERE group_id = :id`, sql.Named("id", id)); err != nil {
		return err
	}

	result, err := tx.Exec(`DELETE FROM groups WHERE id = :id`, sql.Named("id", id))
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("%w: group", ErrNotFound)
	}

	return tx.Commit()
}

// ReorderGroups persists the caller's explicit folder order across groups.
// Unmentioned groups retain their previous relative order after the specified ones.
func (s *Store) ReorderGroups(groupIDs []int64) error {
	if len(groupIDs) == 0 {
		return fmt.Errorf("%w: group_ids must not be empty", ErrInvalid)
	}

	seen := make(map[int64]struct{}, len(groupIDs))
	for _, id := range groupIDs {
		if id <= 0 {
			return fmt.Errorf("%w: invalid group id", ErrInvalid)
		}
		if _, dup := seen[id]; dup {
			return fmt.Errorf("%w: duplicate group id", ErrInvalid)
		}
		seen[id] = struct{}{}
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	rows, err := tx.Query(`SELECT id FROM groups ORDER BY sort_order ASC, id ASC`)
	if err != nil {
		return err
	}

	existingOrder := []int64{}
	existingSet := make(map[int64]struct{})
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		existingOrder = append(existingOrder, id)
		existingSet[id] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	for _, id := range groupIDs {
		if _, exists := existingSet[id]; !exists {
			return fmt.Errorf("%w: group", ErrNotFound)
		}
	}

	orderedIDs := make([]int64, 0, len(existingOrder))
	orderedIDs = append(orderedIDs, groupIDs...)
	for _, id := range existingOrder {
		if _, ok := seen[id]; !ok {
			orderedIDs = append(orderedIDs, id)
		}
	}

	stmt, err := tx.Prepare(`
		UPDATE groups
		SET sort_order = :sort_order, updated_at = unixepoch()
		WHERE id = :id
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for idx, id := range orderedIDs {
		if _, err := stmt.Exec(sql.Named("sort_order", int64(idx+1)), sql.Named("id", id)); err != nil {
			return err
		}
	}

	return tx.Commit()
}
