-- Align reports.reason with OpenAPI / domain (illegal_content).
ALTER TABLE reports DROP CONSTRAINT IF EXISTS reports_reason_check;
ALTER TABLE reports ADD CONSTRAINT reports_reason_check
  CHECK (reason IN ('harassment', 'privacy', 'illegal_content', 'other'));
