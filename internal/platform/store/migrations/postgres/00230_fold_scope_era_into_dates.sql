-- +goose Up
-- #2147: a scope's `era` is a shortcut for `dates` (#1817, #1877), resolved until now on every
-- decode. This folds the stored ones into `dates` once, so the decode alias can go. Nothing writes
-- `era` any more: the API rejects it, the backend marshals no Era field, and the suggester refuses
-- it. The fold is what the alias did (schedule.withEra): the era is ANDed into each date axis, an
-- empty axis becomes [era], an axis the era misses entirely becomes [era], and the any-era range
-- ({} or 0–0) constrains nothing and is simply removed.
--
-- Scopes live on channels.policy_json ($.scope and $.rules[].what) and proposals.proposal_json
-- ($.policy.scope and $.policy.rules[].what). `$.filler.era` and `$.scores.era` are other fields.

CREATE TEMP TABLE scope_era_fold (
  src   TEXT    NOT NULL, -- channel | channel-rule | proposal | proposal-rule
  id    TEXT    NOT NULL,
  idx   INTEGER,          -- the rule's index; NULL for the top-level scope
  scope JSONB   NOT NULL,
  ef    INTEGER NOT NULL, -- era.from, 0 when open
  et    INTEGER NOT NULL  -- era.to, 0 when open
);

INSERT INTO scope_era_fold (src, id, idx, scope, ef, et)
SELECT 'channel', id, NULL, policy_json -> 'scope',
       coalesce((policy_json #>> '{scope,era,from}')::int, 0), coalesce((policy_json #>> '{scope,era,to}')::int, 0)
  FROM channels WHERE jsonb_typeof(policy_json #> '{scope,era}') = 'object';

INSERT INTO scope_era_fold (src, id, idx, scope, ef, et)
SELECT 'channel-rule', c.id, r.ord - 1, r.value -> 'what',
       coalesce((r.value #>> '{what,era,from}')::int, 0), coalesce((r.value #>> '{what,era,to}')::int, 0)
  FROM channels c,
       jsonb_array_elements(CASE WHEN jsonb_typeof(c.policy_json -> 'rules') = 'array' THEN c.policy_json -> 'rules'
                                 ELSE '[]'::jsonb END) WITH ORDINALITY AS r(value, ord)
 WHERE jsonb_typeof(r.value #> '{what,era}') = 'object';

INSERT INTO scope_era_fold (src, id, idx, scope, ef, et)
SELECT 'proposal', id, NULL, proposal_json #> '{policy,scope}',
       coalesce((proposal_json #>> '{policy,scope,era,from}')::int, 0), coalesce((proposal_json #>> '{policy,scope,era,to}')::int, 0)
  FROM proposals WHERE jsonb_typeof(proposal_json #> '{policy,scope,era}') = 'object';

INSERT INTO scope_era_fold (src, id, idx, scope, ef, et)
SELECT 'proposal-rule', p.id, r.ord - 1, r.value -> 'what',
       coalesce((r.value #>> '{what,era,from}')::int, 0), coalesce((r.value #>> '{what,era,to}')::int, 0)
  FROM proposals p,
       jsonb_array_elements(CASE WHEN jsonb_typeof(p.proposal_json #> '{policy,rules}') = 'array'
                                 THEN p.proposal_json #> '{policy,rules}' ELSE '[]'::jsonb END) WITH ORDINALITY AS r(value, ord)
 WHERE jsonb_typeof(r.value #> '{what,era}') = 'object';

-- An era-bounded scope: each axis is its ranges clipped to the era (dropping the ones it misses),
-- or [era] when the axis was empty or nothing survived.
UPDATE scope_era_fold SET scope = (scope - 'era') || jsonb_build_object('dates', jsonb_build_object(
  'movieRelease', coalesce(
    (SELECT jsonb_agg(CASE WHEN f > 0 AND t > 0 THEN jsonb_build_object('from', f, 'to', t)
                           WHEN f > 0 THEN jsonb_build_object('from', f) ELSE jsonb_build_object('to', t) END ORDER BY ord)
       FROM (SELECT x.ord, greatest(coalesce((x.r ->> 'from')::int, 0), scope_era_fold.ef) AS f,
                    CASE WHEN coalesce((x.r ->> 'to')::int, 0) = 0 THEN scope_era_fold.et
                         WHEN scope_era_fold.et = 0 THEN (x.r ->> 'to')::int
                         ELSE least((x.r ->> 'to')::int, scope_era_fold.et) END AS t
               FROM jsonb_array_elements(CASE WHEN jsonb_typeof(scope_era_fold.scope #> '{dates,movieRelease}') = 'array'
                                              THEN scope_era_fold.scope #> '{dates,movieRelease}' ELSE '[]'::jsonb END)
                    WITH ORDINALITY AS x(r, ord)) s
      WHERE t = 0 OR f <= t),
    jsonb_build_array(scope_era_fold.scope -> 'era')),
  'seriesPremiere', coalesce(
    (SELECT jsonb_agg(CASE WHEN f > 0 AND t > 0 THEN jsonb_build_object('from', f, 'to', t)
                           WHEN f > 0 THEN jsonb_build_object('from', f) ELSE jsonb_build_object('to', t) END ORDER BY ord)
       FROM (SELECT x.ord, greatest(coalesce((x.r ->> 'from')::int, 0), scope_era_fold.ef) AS f,
                    CASE WHEN coalesce((x.r ->> 'to')::int, 0) = 0 THEN scope_era_fold.et
                         WHEN scope_era_fold.et = 0 THEN (x.r ->> 'to')::int
                         ELSE least((x.r ->> 'to')::int, scope_era_fold.et) END AS t
               FROM jsonb_array_elements(CASE WHEN jsonb_typeof(scope_era_fold.scope #> '{dates,seriesPremiere}') = 'array'
                                              THEN scope_era_fold.scope #> '{dates,seriesPremiere}' ELSE '[]'::jsonb END)
                    WITH ORDINALITY AS x(r, ord)) s
      WHERE t = 0 OR f <= t),
    jsonb_build_array(scope_era_fold.scope -> 'era')),
  'seriesAiring', coalesce(
    (SELECT jsonb_agg(CASE WHEN f > 0 AND t > 0 THEN jsonb_build_object('from', f, 'to', t)
                           WHEN f > 0 THEN jsonb_build_object('from', f) ELSE jsonb_build_object('to', t) END ORDER BY ord)
       FROM (SELECT x.ord, greatest(coalesce((x.r ->> 'from')::int, 0), scope_era_fold.ef) AS f,
                    CASE WHEN coalesce((x.r ->> 'to')::int, 0) = 0 THEN scope_era_fold.et
                         WHEN scope_era_fold.et = 0 THEN (x.r ->> 'to')::int
                         ELSE least((x.r ->> 'to')::int, scope_era_fold.et) END AS t
               FROM jsonb_array_elements(CASE WHEN jsonb_typeof(scope_era_fold.scope #> '{dates,seriesAiring}') = 'array'
                                              THEN scope_era_fold.scope #> '{dates,seriesAiring}' ELSE '[]'::jsonb END)
                    WITH ORDINALITY AS x(r, ord)) s
      WHERE t = 0 OR f <= t),
    jsonb_build_array(scope_era_fold.scope -> 'era'))
))
WHERE ef <> 0 OR et <> 0;

-- The any-era range constrains nothing.
UPDATE scope_era_fold SET scope = scope - 'era' WHERE ef = 0 AND et = 0;

UPDATE channels c SET policy_json = jsonb_set(c.policy_json, '{scope}', f.scope)
  FROM scope_era_fold f WHERE f.src = 'channel' AND f.id = c.id;

UPDATE channels c SET policy_json = jsonb_set(c.policy_json, '{rules}', (
    SELECT jsonb_agg(CASE WHEN f.scope IS NULL THEN r.value ELSE jsonb_set(r.value, '{what}', f.scope) END ORDER BY r.ord)
      FROM jsonb_array_elements(c.policy_json -> 'rules') WITH ORDINALITY AS r(value, ord)
      LEFT JOIN scope_era_fold f ON f.src = 'channel-rule' AND f.id = c.id AND f.idx = r.ord - 1))
 WHERE c.id IN (SELECT id FROM scope_era_fold WHERE src = 'channel-rule');

UPDATE proposals p SET proposal_json = jsonb_set(p.proposal_json, '{policy,scope}', f.scope)
  FROM scope_era_fold f WHERE f.src = 'proposal' AND f.id = p.id;

UPDATE proposals p SET proposal_json = jsonb_set(p.proposal_json, '{policy,rules}', (
    SELECT jsonb_agg(CASE WHEN f.scope IS NULL THEN r.value ELSE jsonb_set(r.value, '{what}', f.scope) END ORDER BY r.ord)
      FROM jsonb_array_elements(p.proposal_json #> '{policy,rules}') WITH ORDINALITY AS r(value, ord)
      LEFT JOIN scope_era_fold f ON f.src = 'proposal-rule' AND f.id = p.id AND f.idx = r.ord - 1))
 WHERE p.id IN (SELECT id FROM scope_era_fold WHERE src = 'proposal-rule');

DROP TABLE scope_era_fold;

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
