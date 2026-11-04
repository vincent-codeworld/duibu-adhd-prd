-- Ownership / attach metadata for real screenshot uploads (local disk MVP).
CREATE TABLE screenshot_uploads (
  key                   TEXT PRIMARY KEY,
  uploader_account_id   UUID NOT NULL REFERENCES accounts(id),
  content_type          TEXT NOT NULL CHECK (content_type IN ('image/jpeg', 'image/png', 'image/webp')),
  byte_size             INT NOT NULL CHECK (byte_size > 0 AND byte_size <= 4194304),
  created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
  attached_request_id   UUID NULL REFERENCES verification_requests(id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX screenshot_uploads_one_attach_uidx
  ON screenshot_uploads (attached_request_id)
  WHERE attached_request_id IS NOT NULL;

CREATE INDEX screenshot_uploads_uploader_idx
  ON screenshot_uploads (uploader_account_id);
