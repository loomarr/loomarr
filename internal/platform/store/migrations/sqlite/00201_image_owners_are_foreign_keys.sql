-- +goose Up
-- Schema to a 10 (#2793, item 1): every reference is a foreign key. image_refs named its owner as
-- (owner_kind, owner_id) text, which no foreign key can check, so the store had to delete an
-- owner's refs by hand wherever the owner went (a merged clip, a deleted channel, a pruned
-- proposal or library item) and a missed delete pinned an image forever. Each owner kind now has
-- its own table whose owner column is a foreign key that cascades. A clip is re-keyed in place
-- when its bytes are transformed, so its refs follow the new hash (ON UPDATE CASCADE).
--
-- Only refs whose owner still exists are copied; a ref to a gone owner is the leak this fixes.
CREATE TABLE clip_images (
    clip_hash  TEXT NOT NULL REFERENCES clips(hash) ON DELETE CASCADE ON UPDATE CASCADE,
    role       TEXT NOT NULL,
    image_hash TEXT NOT NULL REFERENCES images(hash) ON DELETE CASCADE,
    PRIMARY KEY (clip_hash, role, image_hash)
);
CREATE INDEX idx_clip_images_image ON clip_images (image_hash);

CREATE TABLE filler_split_proposal_images (
    proposal_id TEXT NOT NULL REFERENCES filler_split_proposals(id) ON DELETE CASCADE,
    role        TEXT NOT NULL,
    image_hash  TEXT NOT NULL REFERENCES images(hash) ON DELETE CASCADE,
    PRIMARY KEY (proposal_id, role, image_hash)
);
CREATE INDEX idx_filler_split_proposal_images_image ON filler_split_proposal_images (image_hash);

CREATE TABLE channel_images (
    channel_id TEXT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    role       TEXT NOT NULL,
    image_hash TEXT NOT NULL REFERENCES images(hash) ON DELETE CASCADE,
    PRIMARY KEY (channel_id, role, image_hash)
);
CREATE INDEX idx_channel_images_image ON channel_images (image_hash);

CREATE TABLE library_item_images (
    item_id    TEXT NOT NULL REFERENCES library_items(id) ON DELETE CASCADE,
    role       TEXT NOT NULL,
    image_hash TEXT NOT NULL REFERENCES images(hash) ON DELETE CASCADE,
    PRIMARY KEY (item_id, role, image_hash)
);
CREATE INDEX idx_library_item_images_image ON library_item_images (image_hash);

-- Needs you's candidate posters, owned by the title the candidates are for.
CREATE TABLE library_candidate_images (
    item_id    TEXT NOT NULL REFERENCES library_items(id) ON DELETE CASCADE,
    role       TEXT NOT NULL,
    image_hash TEXT NOT NULL REFERENCES images(hash) ON DELETE CASCADE,
    PRIMARY KEY (item_id, role, image_hash)
);
CREATE INDEX idx_library_candidate_images_image ON library_candidate_images (image_hash);

INSERT INTO clip_images (clip_hash, role, image_hash)
SELECT r.owner_id, r.role, r.image_hash FROM image_refs r
WHERE r.owner_kind = 'clip' AND EXISTS (SELECT 1 FROM clips c WHERE c.hash = r.owner_id);
INSERT INTO filler_split_proposal_images (proposal_id, role, image_hash)
SELECT r.owner_id, r.role, r.image_hash FROM image_refs r
WHERE r.owner_kind = 'filler_split_proposal'
  AND EXISTS (SELECT 1 FROM filler_split_proposals p WHERE p.id = r.owner_id);
INSERT INTO channel_images (channel_id, role, image_hash)
SELECT r.owner_id, r.role, r.image_hash FROM image_refs r
WHERE r.owner_kind = 'channel' AND EXISTS (SELECT 1 FROM channels c WHERE c.id = r.owner_id);
INSERT INTO library_item_images (item_id, role, image_hash)
SELECT r.owner_id, r.role, r.image_hash FROM image_refs r
WHERE r.owner_kind = 'library_item' AND EXISTS (SELECT 1 FROM library_items i WHERE i.id = r.owner_id);
INSERT INTO library_candidate_images (item_id, role, image_hash)
SELECT r.owner_id, r.role, r.image_hash FROM image_refs r
WHERE r.owner_kind = 'library_candidate' AND EXISTS (SELECT 1 FROM library_items i WHERE i.id = r.owner_id);

DROP TABLE image_refs;

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
