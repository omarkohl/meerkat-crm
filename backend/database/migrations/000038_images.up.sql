CREATE TABLE images (
  id          INTEGER PRIMARY KEY,
  user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  contact_id  INTEGER REFERENCES contacts(id)   ON DELETE CASCADE,
  activity_id INTEGER REFERENCES activities(id) ON DELETE CASCADE,
  note_id     INTEGER REFERENCES notes(id)      ON DELETE CASCADE,
  position    INTEGER NOT NULL DEFAULT 0,
  file        TEXT NOT NULL,
  thumb_file  TEXT NOT NULL,
  mime        TEXT NOT NULL,
  width       INTEGER NOT NULL,
  height      INTEGER NOT NULL,
  bytes       INTEGER NOT NULL,
  taken_at    DATETIME,
  crop_x      INTEGER,
  crop_y      INTEGER,
  crop_size   INTEGER,
  created_at  DATETIME NOT NULL,
  updated_at  DATETIME NOT NULL,
  CHECK ((contact_id IS NOT NULL) + (activity_id IS NOT NULL) + (note_id IS NOT NULL) <= 1)
);
CREATE INDEX idx_images_user ON images(user_id);
CREATE INDEX idx_images_contact ON images(contact_id, position);
CREATE INDEX idx_images_activity ON images(activity_id, position);
CREATE INDEX idx_images_note ON images(note_id, position);
ALTER TABLE contacts ADD COLUMN main_image_id INTEGER REFERENCES images(id) ON DELETE SET NULL;
