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
  scope TEXT    NOT NULL,
  ef    INTEGER NOT NULL, -- era.from, 0 when open
  et    INTEGER NOT NULL  -- era.to, 0 when open
);

INSERT INTO scope_era_fold (src, id, idx, scope, ef, et)
SELECT 'channel', id, NULL, json_extract(policy_json, '$.scope'),
       coalesce(json_extract(policy_json, '$.scope.era.from'), 0), coalesce(json_extract(policy_json, '$.scope.era.to'), 0)
  FROM channels WHERE json_type(policy_json, '$.scope.era') = 'object';

INSERT INTO scope_era_fold (src, id, idx, scope, ef, et)
SELECT 'channel-rule', c.id, r.key, json_extract(r.value, '$.what'),
       coalesce(json_extract(r.value, '$.what.era.from'), 0), coalesce(json_extract(r.value, '$.what.era.to'), 0)
  FROM channels c, json_each(c.policy_json, '$.rules') r WHERE json_type(r.value, '$.what.era') = 'object';

INSERT INTO scope_era_fold (src, id, idx, scope, ef, et)
SELECT 'proposal', id, NULL, json_extract(proposal_json, '$.policy.scope'),
       coalesce(json_extract(proposal_json, '$.policy.scope.era.from'), 0), coalesce(json_extract(proposal_json, '$.policy.scope.era.to'), 0)
  FROM proposals WHERE json_type(proposal_json, '$.policy.scope.era') = 'object';

INSERT INTO scope_era_fold (src, id, idx, scope, ef, et)
SELECT 'proposal-rule', p.id, r.key, json_extract(r.value, '$.what'),
       coalesce(json_extract(r.value, '$.what.era.from'), 0), coalesce(json_extract(r.value, '$.what.era.to'), 0)
  FROM proposals p, json_each(p.proposal_json, '$.policy.rules') r WHERE json_type(r.value, '$.what.era') = 'object';

-- An era-bounded scope: each axis is its ranges clipped to the era (dropping the ones it misses),
-- or [era] when the axis was empty or nothing survived.
UPDATE scope_era_fold SET scope = json_set(json_remove(scope, '$.era'), '$.dates', json_object(
  'movieRelease', json(coalesce(
    (SELECT json_group_array(json(CASE WHEN f > 0 AND t > 0 THEN json_object('from', f, 'to', t)
                                       WHEN f > 0 THEN json_object('from', f) ELSE json_object('to', t) END))
       FROM (SELECT max(coalesce(json_extract(r.value, '$.from'), 0), scope_era_fold.ef) AS f,
                    CASE WHEN coalesce(json_extract(r.value, '$.to'), 0) = 0 THEN scope_era_fold.et
                         WHEN scope_era_fold.et = 0 THEN json_extract(r.value, '$.to')
                         ELSE min(json_extract(r.value, '$.to'), scope_era_fold.et) END AS t
               FROM json_each(scope_era_fold.scope, '$.dates.movieRelease') r ORDER BY r.key)
      WHERE t = 0 OR f <= t HAVING count(*) > 0),
    json_array(json(json_extract(scope_era_fold.scope, '$.era'))))),
  'seriesPremiere', json(coalesce(
    (SELECT json_group_array(json(CASE WHEN f > 0 AND t > 0 THEN json_object('from', f, 'to', t)
                                       WHEN f > 0 THEN json_object('from', f) ELSE json_object('to', t) END))
       FROM (SELECT max(coalesce(json_extract(r.value, '$.from'), 0), scope_era_fold.ef) AS f,
                    CASE WHEN coalesce(json_extract(r.value, '$.to'), 0) = 0 THEN scope_era_fold.et
                         WHEN scope_era_fold.et = 0 THEN json_extract(r.value, '$.to')
                         ELSE min(json_extract(r.value, '$.to'), scope_era_fold.et) END AS t
               FROM json_each(scope_era_fold.scope, '$.dates.seriesPremiere') r ORDER BY r.key)
      WHERE t = 0 OR f <= t HAVING count(*) > 0),
    json_array(json(json_extract(scope_era_fold.scope, '$.era'))))),
  'seriesAiring', json(coalesce(
    (SELECT json_group_array(json(CASE WHEN f > 0 AND t > 0 THEN json_object('from', f, 'to', t)
                                       WHEN f > 0 THEN json_object('from', f) ELSE json_object('to', t) END))
       FROM (SELECT max(coalesce(json_extract(r.value, '$.from'), 0), scope_era_fold.ef) AS f,
                    CASE WHEN coalesce(json_extract(r.value, '$.to'), 0) = 0 THEN scope_era_fold.et
                         WHEN scope_era_fold.et = 0 THEN json_extract(r.value, '$.to')
                         ELSE min(json_extract(r.value, '$.to'), scope_era_fold.et) END AS t
               FROM json_each(scope_era_fold.scope, '$.dates.seriesAiring') r ORDER BY r.key)
      WHERE t = 0 OR f <= t HAVING count(*) > 0),
    json_array(json(json_extract(scope_era_fold.scope, '$.era')))))
))
WHERE ef <> 0 OR et <> 0;

-- The any-era range constrains nothing.
UPDATE scope_era_fold SET scope = json_remove(scope, '$.era') WHERE ef = 0 AND et = 0;

UPDATE channels SET policy_json = json_set(policy_json, '$.scope',
    json((SELECT f.scope FROM scope_era_fold f WHERE f.src = 'channel' AND f.id = channels.id)))
 WHERE id IN (SELECT id FROM scope_era_fold WHERE src = 'channel');

-- json(...) around each subquery: a value read back through a subquery loses its JSON subtype, and
-- json_set would store the array as a string.
UPDATE channels SET policy_json = json_set(policy_json, '$.rules', json((
    SELECT json_group_array(json(rule)) FROM (
      SELECT CASE WHEN f.scope IS NULL THEN r.value ELSE json_set(r.value, '$.what', json(f.scope)) END AS rule
        FROM json_each(channels.policy_json, '$.rules') r
        LEFT JOIN scope_era_fold f ON f.src = 'channel-rule' AND f.id = channels.id AND f.idx = r.key
       ORDER BY r.key))))
 WHERE id IN (SELECT id FROM scope_era_fold WHERE src = 'channel-rule');

UPDATE proposals SET proposal_json = json_set(proposal_json, '$.policy.scope',
    json((SELECT f.scope FROM scope_era_fold f WHERE f.src = 'proposal' AND f.id = proposals.id)))
 WHERE id IN (SELECT id FROM scope_era_fold WHERE src = 'proposal');

UPDATE proposals SET proposal_json = json_set(proposal_json, '$.policy.rules', json((
    SELECT json_group_array(json(rule)) FROM (
      SELECT CASE WHEN f.scope IS NULL THEN r.value ELSE json_set(r.value, '$.what', json(f.scope)) END AS rule
        FROM json_each(proposals.proposal_json, '$.policy.rules') r
        LEFT JOIN scope_era_fold f ON f.src = 'proposal-rule' AND f.id = proposals.id AND f.idx = r.key
       ORDER BY r.key))))
 WHERE id IN (SELECT id FROM scope_era_fold WHERE src = 'proposal-rule');

DROP TABLE scope_era_fold;

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
