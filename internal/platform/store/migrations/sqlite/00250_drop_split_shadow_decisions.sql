-- +goose Up
-- #4414: `AutoCuttable` is the split's only gate (#4409), so nothing writes or reads the shadow
-- comparison of the legacy and certified split rules. Its table goes with its indexes.
--
-- The settings rows of the two retired structure-window path keys stay: nothing reads an
-- undeclared key, and an orphan row is harmless.

DROP TABLE filler_split_shadow_decisions;

-- Forward-only (§16).

-- +goose Down
SELECT 1;
