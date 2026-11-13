package clipstore

import (
	"context"
	"fmt"
	"strings"

	"github.com/loomarr/loomarr/internal/filler/clips"
)

// clipProductLeafFrom is the clip's product-axis leaf assertions. Category is derived from them,
// never stored (#2323).
const clipProductLeafFrom = `FROM clip_tags ct JOIN taxa t ON t.slug = ct.taxon
	WHERE ct.clip_hash = clips.hash AND ct.leaf = TRUE AND t.axis = 'product'`

// clipCategoryExpr is taxonomy.Forest.PrimaryProductLeaf in SQL: the deepest product leaf, ties by
// slug. taxa_closure includes each node's self-pair, so its row count is the node's depth.
const clipCategoryExpr = `COALESCE((SELECT ct.taxon ` + clipProductLeafFrom + `
	ORDER BY (SELECT COUNT(*) FROM taxa_closure tc WHERE tc.descendant = ct.taxon) DESC, ct.taxon ASC
	LIMIT 1), '')`

const clipSelect = `SELECT hash, path, tunarr_program_id, name, name_source, role, placement, era, audience, ` + clipCategoryExpr + `, duration_ms,
	geographic_scope, country, network, station, air_date, geo_evidence,
	source, quality, license, thumbnail, preview, thumb_image_hash, hover_image_hash, language, transcript, brand, visible_text,
	is_composite, parent_hash,
	play_count, last_played_at,
		removed_at, held, updated_at, created_at FROM clips`

func (s *Store) GetClip(ctx context.Context, id string) (Clip, error) {
	c, err := scanClip(s.h.QueryRowContext(ctx, s.h.Rebind(clipSelect+` WHERE hash = ?`), id))
	if err != nil {
		return Clip{}, err
	}
	return s.clipWithTags(ctx, c)
}

// clipWithTags fills a single clip's taxonomy Tags (§10 V45a). Wraps attachTags for the two
// single-row readers so they load tags the same way the listing does — one clip is a one-element
// batch, so there is no separate query shape to keep in step.
func (s *Store) clipWithTags(ctx context.Context, c Clip) (Clip, error) {
	batch := []Clip{c}
	if err := s.attachTags(ctx, batch); err != nil {
		return Clip{}, err
	}
	return batch[0], nil
}

// GetClipByPath looks a clip up by its location under FILLER_DIR.
//
// ⚠ **Needed because a clip's PATH and its IDENTITY stopped being the same string in V38c.**
// Identity is now the content hash (`14365f2b…`), while the path is the sharded location
// (`14/36/14365f2b….mp4`). `GetClip` queries `WHERE hash = ?`, so handing it a path matches
// nothing — silently, as an ordinary not-found.
//
// That is not hypothetical: `/v1/filler/media` did exactly this from V38c until V39 and 404'd for
// every clip in the catalog. Nothing noticed because no client called it until the player shipped.
//
// The media route needs this one rather than the hash lookup because its URL carries the path —
// which is what `ClipDTO` exposes, and what every other clip-shaped route already keys on.
func (s *Store) GetClipByPath(ctx context.Context, path string) (Clip, error) {
	c, err := scanClip(s.h.QueryRowContext(ctx, s.h.Rebind(clipSelect+` WHERE path = ?`), path))
	if err != nil {
		return Clip{}, err
	}
	return s.clipWithTags(ctx, c)
}

// ListClips applies the filter as ANDed WHERE clauses (zero fields omitted). The
// UntaggedOnly flag adds needsDetailsWhere (a commercial missing any match tag).
//
// Ordering comes from clipOrderBy (always total — see its tie-break warning), and `Limit`/`Offset`
// page the result. ⚠ An unknown `Sort` is an ERROR (ErrUnknownClipSort), never a fall-back.
func (s *Store) ListClips(ctx context.Context, f ClipFilter) ([]Clip, error) {
	where, args := clipWhere(f)
	q := clipSelect
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	order, err := clipOrderBy(f)
	if err != nil {
		return nil, err
	}
	q += order
	// ⚠ `Limit == 0` renders NO clause at all — see the field's own warning. Pod assembly reaches
	// here with the zero filter and must get the whole catalog.
	if f.Limit > 0 {
		q += " LIMIT ?"
		args = append(args, f.Limit)
		if f.Offset > 0 {
			q += " OFFSET ?"
			args = append(args, f.Offset)
		}
	}

	rows, err := s.h.QueryContext(ctx, s.h.Rebind(q), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	clips, err := scanClips(rows)
	if err != nil {
		return nil, err
	}
	if err := s.attachTags(ctx, clips); err != nil {
		return nil, err
	}
	return clips, nil
}

// CountClips answers "how many match?" without materialising the rows.
//
// ⚠ Shares `clipWhere` with ListClips rather than restating the predicate. Several callers wanted
// only a number and were loading every column of every row to take `len()` of the slice — on a
// large catalog that is the dominant cost of rendering a settings page. A second hand-written
// WHERE here would be free to drift from the one the listing (and the tagging job) actually use,
// which is the failure this deliberately forecloses.
func (s *Store) CountClips(ctx context.Context, f ClipFilter) (int, error) {
	where, args := clipWhere(f)
	q := `SELECT COUNT(*) FROM clips`
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	var n int
	if err := s.h.QueryRowContext(ctx, s.h.Rebind(q), args...).Scan(&n); err != nil {
		return 0, fmt.Errorf("count clips: %w", err)
	}
	return n, nil
}

// CountClipsBySource returns the clip count per source — the Sources page's whole question.
//
// ⚠ Grouped in SQL. This was every row of the catalog loaded into Go to build a map of counters;
// the answer is one small map either way, so the load was pure waste. Honours the same default
// exclusions as the listing (removed and held clips are out unless asked for) so the per-source
// numbers add up to the catalog total a caller sees elsewhere.
func (s *Store) CountClipsBySource(ctx context.Context, f ClipFilter) (map[string]int, error) {
	where, args := clipWhere(f)
	q := `SELECT source, COUNT(*) FROM clips`
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += " GROUP BY source"

	rows, err := s.h.QueryContext(ctx, s.h.Rebind(q), args...)
	if err != nil {
		return nil, fmt.Errorf("count clips by source: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]int{}
	for rows.Next() {
		var src string
		var n int
		if err := rows.Scan(&src, &n); err != nil {
			return nil, err
		}
		out[src] = n
	}
	return out, rows.Err()
}

// clipWhere builds the ANDed predicate for a ClipFilter, shared by every query that has to mean
// the same thing by "matching clips" — the listing, the counts, and the per-source rollup.
func clipWhere(f ClipFilter) ([]string, []any) {
	var where []string
	var args []any
	if f.Role != "" {
		where = append(where, "role = ?")
		args = append(args, string(f.Role))
	}
	if f.Era > 0 {
		where = append(where, "era = ?")
		args = append(args, f.Era)
	}
	if f.Audience != "" {
		where = append(where, "audience = ?")
		args = append(args, string(f.Audience))
	}
	if f.Category != "" {
		where = append(where, clipCategoryExpr+" = ?")
		args = append(args, f.Category)
	}
	if f.GeographicScope != "" {
		where = append(where, "geographic_scope = ?")
		args = append(args, string(f.GeographicScope))
	}
	if f.Country != "" {
		where = append(where, "UPPER(country) = UPPER(?)")
		args = append(args, strings.TrimSpace(f.Country))
	}
	if f.Taxon != "" {
		where = append(where, "EXISTS (SELECT 1 FROM clip_tags tx WHERE tx.clip_hash = clips.hash AND tx.taxon = ?)")
		args = append(args, f.Taxon)
	}
	if f.WithoutTaxonomyTags {
		where = append(where, "NOT EXISTS (SELECT 1 FROM clip_tags tx WHERE tx.clip_hash = clips.hash AND tx.leaf = ?)")
		args = append(args, true)
	}
	if f.WithoutTaxonomyAxis != "" {
		where = append(where, `NOT EXISTS (SELECT 1 FROM clip_tags tx
			JOIN taxa tt ON tt.slug = tx.taxon
			WHERE tx.clip_hash = clips.hash AND tx.leaf = ? AND tt.axis = ?)`)
		args = append(args, true, string(f.WithoutTaxonomyAxis))
	}
	if f.Query != "" {
		// LOWER() on both sides so the match is case-insensitive on BOTH dialects:
		// SQLite's LIKE folds case for ASCII by default while Postgres's does not, and
		// a search that behaves differently per backend is exactly the dialect fork the
		// store rules forbid. The term is escaped for LIKE metacharacters so a user
		// typing "%" searches for a percent sign rather than matching everything.
		//
		// ⚠ Widened past `name` in V51d — a catalog search that could not find "Kellogg's" on a
		// clip whose brand IS Kellogg's was a search box that looked broken. The columns come from
		// a FIXED list below, never from input; only the bound term is user data.
		//
		// ⚠ No FTS, deliberately: SQLite FTS5 and Postgres tsvector are different engines with
		// different tokenizers and ranking, so adopting them forces this function to branch on
		// dialect and the conformance suite to assert equivalent-but-not-identical results per
		// backend. One suite, two backends (§5) rules that out. LIKE over four columns is slower
		// in theory and indistinguishable at household scale.
		like := "%" + escapeLike(f.Query) + "%"
		cols := []string{"name", "brand", "visible_text"}
		if f.QueryTranscript {
			cols = append(cols, "transcript")
		}
		ors := make([]string, 0, len(cols)+1)
		for _, col := range cols {
			ors = append(ors, "LOWER("+col+") LIKE LOWER(?) ESCAPE '\\'")
			args = append(args, like)
		}
		// Tags match through an EXISTS rather than a JOIN: a clip with three matching tags must be
		// ONE row, and a join would return it three times — which CountClips would then count
		// three times, so the total and the rows would disagree. `clip_tags` is indexed both ways.
		ors = append(ors, "EXISTS (SELECT 1 FROM clip_tags ct WHERE ct.clip_hash = clips.hash AND LOWER(ct.taxon) LIKE LOWER(?) ESCAPE '\\')")
		args = append(args, like)
		where = append(where, "("+strings.Join(ors, " OR ")+")")
	}
	if len(f.Hashes) > 0 {
		ph := make([]string, len(f.Hashes))
		for i, h := range f.Hashes {
			ph[i] = "?"
			args = append(args, h)
		}
		where = append(where, "hash IN ("+strings.Join(ph, ",")+")")
	}
	if !f.IncludeRemoved {
		where = append(where, "removed_at = 0")
	}
	// ⚠ THE chokepoint for the clip lifecycle (§10 V38). Held clips are excluded here, once,
	// rather than at each of the many callers — pod assembly, coverage, the filler-list builder,
	// the catalog listing, search. Filtering per call site is how one of them gets forgotten and
	// an unreviewed clip airs.
	//
	// ⚠ Compared as a BOUND PARAMETER, never a literal. `held` is INTEGER on sqlite and BOOLEAN
	// on Postgres (migration 00031), so `held = 0` is a type error on one of them — the same
	// dialect trap V37's 00029 hit with a literal `1`. Binding lets the driver render each.
	if f.HeldOnly {
		where = append(where, "held = ?")
		args = append(args, true)
	} else if !f.IncludeHeld {
		where = append(where, "held = ?")
		args = append(args, false)
		where = append(where, "placement <> ?")
		args = append(args, string(clips.PlacementNotPlayable))
	}
	// ⚠ THE composite chokepoint (§10 V45), the same shape as the held block above and for the same
	// reason: pod assembly loads the catalog here with a zero filter, so a composite excluded ONCE
	// here can never air. Bound, never a literal — `is_composite` is BOOLEAN on Postgres and INTEGER
	// on sqlite (00039), the recurring dialect trap.
	if f.CompositesOnly {
		where = append(where, "is_composite = ?")
		args = append(args, true)
	} else if !f.IncludeComposites {
		where = append(where, "is_composite = ?")
		args = append(args, false)
	}
	// ⚠ ParentHash WINS over TopLevelOnly rather than both being ANDed. They are contradictory —
	// "the segments of break X" and "clips with no parent" share no row — so ANDing them returns
	// the empty set, which renders as "this break has no segments" on a break that has twenty.
	// A lineage query is by construction not a top-level query; saying so here beats letting a
	// caller that sets both get a confident, wrong, empty answer.
	if f.ParentHash != "" {
		where = append(where, "parent_hash = ?")
		args = append(args, f.ParentHash)
	} else if f.TopLevelOnly {
		// Bound, not the literal '', for the same reason every other comparison here is bound.
		where = append(where, "parent_hash = ?")
		args = append(args, "")
	}
	if f.UntaggedOnly {
		where = append(where, needsDetailsWhere)
		args = append(args, string(clips.Commercial))
	}
	return where, args
}

// needsDetailsWhere is clipcatalog.Clip.NeedsDetails in SQL: a COMMERCIAL missing any match tag
// (the negation of Tagged: era > 0, audience and category set). The kind is bound, never a literal,
// so it is the same constant the Go predicate reads. testClipNeedsDetailsAgreesWithSQL pins the two
// to the same clips on both backends; the wire's ClipDTO.needsDetails comes from the Go side.
const needsDetailsWhere = "role = ? AND (era = 0 OR audience = '' OR NOT EXISTS (SELECT 1 " + clipProductLeafFrom + "))"
