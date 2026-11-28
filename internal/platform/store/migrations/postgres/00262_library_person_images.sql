-- +goose Up
-- A person's photo in the image service (#4476), owned by the person so removing them frees it.
CREATE TABLE library_person_images (
    person_id  TEXT NOT NULL REFERENCES library_people(id) ON DELETE CASCADE,
    role       TEXT NOT NULL,
    image_hash TEXT NOT NULL REFERENCES images(hash) ON DELETE CASCADE,
    -- The person's photo URL this ref was adopted for. An image shared by identical bytes keeps
    -- only one source URL of its own.
    source_url TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (person_id, role, image_hash)
);
CREATE INDEX idx_library_person_images_image ON library_person_images (image_hash);
