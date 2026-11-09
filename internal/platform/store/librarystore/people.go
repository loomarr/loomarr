package librarystore

import (
	"context"
	"fmt"
	"time"
)

// LibraryPersonRef is a person whose filmography is due: their library_people id and catalog id.
type LibraryPersonRef struct {
	ID       string
	External LibraryID
}

// LibraryFilmographyDue says which filmographies to fetch now: people linked in Namespace (the
// catalog filmographies come from), never fetched or fetched before StaleBefore, and not held back
// past Now; at most Limit.
type LibraryFilmographyDue struct {
	Namespace        string
	Now, StaleBefore time.Time
	Limit            int
}

// LibraryFilmEntry is one title in a person's filmography (decision 50): a movie or series by its
// id in the catalog the filmography came from, what they did on it, and an actor's character.
type LibraryFilmEntry struct {
	Kind      string // movie or series
	ID        LibraryID
	Title     string
	Year      int
	Role      string
	Character string
}

// LibraryPeopleStore keeps the filmographies of the people the library credits.
type LibraryPeopleStore interface {
	// DueLibraryFilmographies lists the people whose filmography to fetch now: those linked in
	// the catalog's namespace who directed or created a title, are among a title's ten top-billed actors, or are
	// credited on three or more titles (decision 50). One never fetched comes first.
	DueLibraryFilmographies(ctx context.Context, due LibraryFilmographyDue) ([]LibraryPersonRef, error)
	// SaveLibraryFilmography replaces a person's filmography, fetched at at.
	SaveLibraryFilmography(ctx context.Context, personID string, entries []LibraryFilmEntry, at time.Time) error
	// FailLibraryFilmography holds a person back from fetching until retryAt.
	FailLibraryFilmography(ctx context.Context, personID string, retryAt time.Time) error
	// PruneLibraryPeople removes the people no title credits any more, with their filmographies,
	// and counts them.
	PruneLibraryPeople(ctx context.Context) (int, error)
}

func (s *Store) DueLibraryFilmographies(ctx context.Context, due LibraryFilmographyDue) ([]LibraryPersonRef, error) {
	if due.Limit <= 0 || due.Namespace == "" {
		return nil, nil
	}
	rows, err := s.h.QueryContext(ctx, s.ph(`SELECT p.id, p.namespace, p.external_id FROM library_people p
		WHERE p.namespace = ? AND p.external_id <> '' AND p.filmography_retry_at_ms <= ?
		AND (p.filmography_at_ms = 0 OR p.filmography_at_ms < ?)
		AND (EXISTS (SELECT 1 FROM library_credits c WHERE c.person_id = p.id
				AND (c.role IN ('director', 'creator') OR (c.role = 'actor' AND c.position < 10)))
			OR (SELECT COUNT(DISTINCT c.item_id) FROM library_credits c WHERE c.person_id = p.id) >= 3)
		ORDER BY p.filmography_at_ms, p.id LIMIT ?`), due.Namespace, due.Now.UnixMilli(), due.StaleBefore.UnixMilli(), due.Limit)
	if err != nil {
		return nil, fmt.Errorf("due library filmographies: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []LibraryPersonRef
	for rows.Next() {
		var p LibraryPersonRef
		if err := rows.Scan(&p.ID, &p.External.Namespace, &p.External.Value); err != nil {
			return nil, fmt.Errorf("due library filmographies: %w", err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("due library filmographies: %w", err)
	}
	return out, nil
}

func (s *Store) SaveLibraryFilmography(ctx context.Context, personID string, entries []LibraryFilmEntry, at time.Time) error {
	tx, err := s.h.Begin(ctx)
	if err != nil {
		return fmt.Errorf("save library filmography: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, s.ph(`DELETE FROM library_filmography WHERE person_id = ?`), personID); err != nil {
		return fmt.Errorf("save library filmography: %w", err)
	}
	for i, e := range entries {
		if _, err := tx.ExecContext(ctx, s.ph(`INSERT INTO library_filmography (person_id, position, kind, namespace,
			external_id, title, year, role, character_name) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`),
			personID, i, e.Kind, e.ID.Namespace, e.ID.Value, e.Title, e.Year, e.Role, e.Character); err != nil {
			return fmt.Errorf("save library filmography: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, s.ph(`UPDATE library_people SET filmography_at_ms = ?, filmography_retry_at_ms = 0
		WHERE id = ?`), at.UnixMilli(), personID); err != nil {
		return fmt.Errorf("save library filmography: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("save library filmography: %w", err)
	}
	return nil
}

func (s *Store) FailLibraryFilmography(ctx context.Context, personID string, retryAt time.Time) error {
	if _, err := s.h.ExecContext(ctx, s.ph(`UPDATE library_people SET filmography_retry_at_ms = ? WHERE id = ?`),
		retryAt.UnixMilli(), personID); err != nil {
		return fmt.Errorf("fail library filmography: %w", err)
	}
	return nil
}

func (s *Store) PruneLibraryPeople(ctx context.Context) (int, error) {
	res, err := s.h.ExecContext(ctx, `DELETE FROM library_people
		WHERE NOT EXISTS (SELECT 1 FROM library_credits c WHERE c.person_id = library_people.id)`)
	if err != nil {
		return 0, fmt.Errorf("prune library people: %w", err)
	}
	n, err := res.RowsAffected()
	return int(n), err
}
