package librarystore

import (
	"context"
	"fmt"
	"strings"
)

// LibraryEvidence is which of the asked keywords a Library item's own tags hold, and which of the
// asked people its own credits name (#3006), each in the Library's own spelling.
type LibraryEvidence struct {
	Keywords []string
	People   []string
}

// LibraryEvidenceReader reads what grounds a discovery hit the household already owns.
type LibraryEvidenceReader interface {
	// LibraryEvidence answers each id whose tags or credits name one of keywords or people,
	// compared case-insensitively (SQLite's LOWER folds ASCII only); an id that names none is absent. An episode's own rows are
	// read, not its series'.
	LibraryEvidence(ctx context.Context, ids, keywords, people []string) (map[string]LibraryEvidence, error)
}

func (s *Store) LibraryEvidence(ctx context.Context, ids, keywords, people []string) (map[string]LibraryEvidence, error) {
	out := map[string]LibraryEvidence{}
	keywords, people = lowered(keywords), lowered(people)
	if len(ids) == 0 || len(keywords)+len(people) == 0 {
		return out, nil
	}
	// Both term lists are a query's handful; only the ids are chunked.
	for start := 0; start < len(ids); start += libraryFactsChunk {
		chunk := ids[start:min(start+libraryFactsChunk, len(ids))]
		if len(keywords) > 0 {
			err := s.evidence(ctx, `SELECT t.item_id, t.name FROM library_tags t WHERE t.item_id IN (%s)
				AND LOWER(t.name) IN (%s) ORDER BY t.item_id, t.position`, chunk, keywords, out,
				func(e *LibraryEvidence, name string) { e.Keywords = append(e.Keywords, name) })
			if err != nil {
				return nil, fmt.Errorf("library tag evidence: %w", err)
			}
		}
		if len(people) > 0 {
			err := s.evidence(ctx, `SELECT c.item_id, p.name FROM library_credits c
				JOIN library_people p ON p.id = c.person_id WHERE c.item_id IN (%s)
				AND LOWER(p.name) IN (%s) ORDER BY c.item_id, c.position`, chunk, people, out,
				func(e *LibraryEvidence, name string) {
					if !containsFold(e.People, name) { // one person may hold two credits
						e.People = append(e.People, name)
					}
				})
			if err != nil {
				return nil, fmt.Errorf("library credit evidence: %w", err)
			}
		}
	}
	return out, nil
}

// evidence runs one query whose two %s are the id and term IN lists, adding each row's name.
func (s *Store) evidence(ctx context.Context, query string, ids, terms []string, out map[string]LibraryEvidence,
	add func(*LibraryEvidence, string)) error {
	marks := func(n int) string { return strings.TrimSuffix(strings.Repeat("?, ", n), ", ") }
	args := make([]any, 0, len(ids)+len(terms))
	for _, id := range ids {
		args = append(args, id)
	}
	for _, term := range terms {
		args = append(args, term)
	}
	rows, err := s.h.QueryContext(ctx, s.ph(fmt.Sprintf(query, marks(len(ids)), marks(len(terms)))), args...)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return err
		}
		e := out[id]
		add(&e, name)
		out[id] = e
	}
	return rows.Err()
}

// lowered is terms trimmed, lower-cased and de-duplicated, empties dropped.
func lowered(terms []string) []string {
	var out []string
	for _, term := range terms {
		term = strings.ToLower(strings.TrimSpace(term))
		if term != "" && !containsFold(out, term) {
			out = append(out, term)
		}
	}
	return out
}

func containsFold(values []string, want string) bool {
	for _, v := range values {
		if strings.EqualFold(v, want) {
			return true
		}
	}
	return false
}
