-- Google Sign-In is a separate identity from Sign in with Apple.
-- google_subject stores Google's sub only. Accounts are never matched across providers by email.
-- apple_subject stays unique; multiple Google-only rows may leave it NULL.

ALTER TABLE accounts ALTER COLUMN apple_subject DROP NOT NULL;

ALTER TABLE accounts ADD COLUMN google_subject TEXT UNIQUE;

ALTER TABLE accounts ADD CONSTRAINT accounts_provider_subject_present
  CHECK (apple_subject IS NOT NULL OR google_subject IS NOT NULL);
