-- 兑步 MVP schema (synced-server entities only). Device-only tasks stay on client.

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE accounts (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  nickname        VARCHAR(20) NOT NULL,
  apple_subject   TEXT NOT NULL UNIQUE,
  invite_code     VARCHAR(32) NOT NULL UNIQUE,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE friendships (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  account_a       UUID NOT NULL REFERENCES accounts(id),
  account_b       UUID NOT NULL REFERENCES accounts(id),
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  removed_at      TIMESTAMPTZ,
  CHECK (account_a <> account_b)
);

CREATE UNIQUE INDEX friendships_pair_active_uidx
  ON friendships (LEAST(account_a, account_b), GREATEST(account_a, account_b))
  WHERE removed_at IS NULL;

CREATE TABLE blocks (
  blocker_id      UUID NOT NULL REFERENCES accounts(id),
  blocked_id      UUID NOT NULL REFERENCES accounts(id),
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (blocker_id, blocked_id),
  CHECK (blocker_id <> blocked_id)
);

CREATE TABLE tasks (
  id                              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_account_id                UUID NOT NULL REFERENCES accounts(id),
  sync_state                      TEXT NOT NULL CHECK (sync_state = 'synced'),
  title                           VARCHAR(40) NOT NULL,
  notes                           VARCHAR(500) NOT NULL DEFAULT '',
  status                          TEXT NOT NULL CHECK (status IN ('active', 'completed')),
  designated_verifier_account_id  UUID REFERENCES accounts(id),
  created_at                      TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at                      TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at                    TIMESTAMPTZ,
  upload_consent_at               TIMESTAMPTZ NOT NULL
);

CREATE TABLE stages (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  task_id     UUID NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
  position    INT NOT NULL CHECK (position BETWEEN 1 AND 7),
  title       VARCHAR(40) NOT NULL,
  progress    TEXT NOT NULL CHECK (progress IN ('pending', 'done', 'skipped')),
  done_at     TIMESTAMPTZ,
  skipped_at  TIMESTAMPTZ,
  UNIQUE (task_id, position)
);

CREATE TABLE rewards (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  scope         TEXT NOT NULL CHECK (scope IN ('stage', 'task_completion')),
  task_id       UUID NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
  stage_id      UUID REFERENCES stages(id) ON DELETE CASCADE,
  text          VARCHAR(80) NOT NULL,
  link          VARCHAR(500),
  redeem_state  TEXT NOT NULL CHECK (redeem_state IN (
                  'unrequested', 'pending', 'approved', 'rejected', 'self_redeemed')),
  redeemed_at   TIMESTAMPTZ,
  CHECK (
    (scope = 'stage' AND stage_id IS NOT NULL) OR
    (scope = 'task_completion' AND stage_id IS NULL)
  )
);

CREATE UNIQUE INDEX rewards_one_per_stage_uidx
  ON rewards (stage_id) WHERE scope = 'stage' AND stage_id IS NOT NULL;

CREATE UNIQUE INDEX rewards_one_completion_per_task_uidx
  ON rewards (task_id) WHERE scope = 'task_completion';

CREATE TABLE verification_requests (
  id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  reward_id             UUID NOT NULL REFERENCES rewards(id) ON DELETE CASCADE,
  attempt               INT NOT NULL CHECK (attempt >= 1),
  requester_account_id  UUID NOT NULL REFERENCES accounts(id),
  verifier_account_id   UUID NOT NULL REFERENCES accounts(id),
  status                TEXT NOT NULL CHECK (status IN ('pending', 'approved', 'rejected', 'withdrawn')),
  note                  VARCHAR(200) NOT NULL DEFAULT '',
  resolution_note       VARCHAR(80) NOT NULL DEFAULT '',
  created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
  resolved_at           TIMESTAMPTZ
);

CREATE UNIQUE INDEX verification_one_pending_per_reward_uidx
  ON verification_requests (reward_id) WHERE status = 'pending';

CREATE TABLE screenshots (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  request_id    UUID NOT NULL REFERENCES verification_requests(id) ON DELETE CASCADE,
  storage_key   TEXT NOT NULL,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (request_id)
);

CREATE TABLE in_app_notices (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  account_id    UUID NOT NULL REFERENCES accounts(id),
  kind          TEXT NOT NULL CHECK (kind IN (
                  'verification_pending', 'verification_approved',
                  'verification_rejected', 'verification_withdrawn')),
  request_id    UUID NOT NULL REFERENCES verification_requests(id) ON DELETE CASCADE,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  read_at       TIMESTAMPTZ
);

CREATE TABLE reports (
  id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  reporter_account_id   UUID NOT NULL REFERENCES accounts(id),
  request_id            UUID NOT NULL REFERENCES verification_requests(id),
  reason                TEXT NOT NULL CHECK (reason IN ('harassment', 'privacy', 'illegal_content', 'other')),
  detail                TEXT NOT NULL DEFAULT '',
  created_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE invite_code_failures (
  account_id    UUID NOT NULL REFERENCES accounts(id),
  failed_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX invite_code_failures_account_time_idx
  ON invite_code_failures (account_id, failed_at DESC);
