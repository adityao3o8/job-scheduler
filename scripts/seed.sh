#!/usr/bin/env bash
#
# Seed demo data into the scheduler.
# Usage: ./scripts/seed.sh [API_URL]
#   API_URL defaults to http://localhost:8080
#
set -euo pipefail

API="${1:-http://localhost:8080}"
echo "==> Seeding against $API"

# ── 1. Register org + user ───────────────────────────────────────────────────

echo "--- Registering demo org and user"
REGISTER=$(curl -sf -X POST "$API/auth/register" \
  -H 'Content-Type: application/json' \
  -d '{
    "org_name": "Demo Corp",
    "org_slug": "demo",
    "email":    "admin@demo.com",
    "name":     "Demo Admin",
    "password": "demodemo123"
  }' 2>/dev/null || true)

# If registration fails (already exists), login instead.
if [ -z "$REGISTER" ] || ! echo "$REGISTER" | grep -q '"token"'; then
  echo "    (org exists, logging in)"
  REGISTER=$(curl -sf -X POST "$API/auth/login" \
    -H 'Content-Type: application/json' \
    -d '{"email":"admin@demo.com","password":"demodemo123"}')
fi

TOKEN=$(echo "$REGISTER" | grep -o '"token":"[^"]*"' | cut -d'"' -f4)
if [ -z "$TOKEN" ]; then
  echo "FATAL: could not obtain auth token" >&2
  exit 1
fi
echo "    token: ${TOKEN:0:20}..."

AUTH="Authorization: Bearer $TOKEN"

# ── 2. Create a project ─────────────────────────────────────────────────────

echo "--- Creating project"
PROJ=$(curl -sf -X POST "$API/projects" \
  -H 'Content-Type: application/json' -H "$AUTH" \
  -d '{"name":"Platform","slug":"platform"}' 2>/dev/null || \
  curl -sf "$API/projects?limit=1" -H "$AUTH" | grep -o '"id":"[^"]*"' | head -1)

PROJECT_ID=$(echo "$PROJ" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
echo "    project_id: $PROJECT_ID"

# ── 3. Create queues ────────────────────────────────────────────────────────

create_queue() {
  local name=$1 slug=$2 concurrency=$3
  local body="{\"project_id\":\"$PROJECT_ID\",\"name\":\"$name\",\"slug\":\"$slug\",\"concurrency_limit\":$concurrency,\"priority_default\":5}"
  local resp
  resp=$(curl -sf -X POST "$API/queues" \
    -H 'Content-Type: application/json' -H "$AUTH" \
    -d "$body" 2>/dev/null || true)
  if [ -z "$resp" ] || ! echo "$resp" | grep -q '"id"'; then
    resp=$(curl -sf "$API/queues?name=$name&limit=1" -H "$AUTH")
    echo "$resp" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4
  else
    echo "$resp" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4
  fi
}

echo "--- Creating queues"
Q_EMAILS=$(create_queue "Emails" "emails" 10)
Q_WEBHOOKS=$(create_queue "Webhooks" "webhooks" 5)
Q_REPORTS=$(create_queue "Reports" "reports" 3)
echo "    emails=$Q_EMAILS  webhooks=$Q_WEBHOOKS  reports=$Q_REPORTS"

# ── 4. Helper to submit a job ────────────────────────────────────────────────

submit() {
  local queue_id=$1 payload=$2
  shift 2
  local extra=""
  for arg in "$@"; do extra="$extra,$arg"; done
  local body="{\"payload\":$payload$extra}"
  curl -sf -X POST "$API/queues/$queue_id/jobs" \
    -H 'Content-Type: application/json' -H "$AUTH" \
    -d "$body" >/dev/null
}

# ── 5. Immediate jobs (processed right away) ─────────────────────────────────

echo "--- Submitting immediate jobs"
for i in $(seq 1 8); do
  submit "$Q_EMAILS" "{\"type\":\"sleep\",\"duration_ms\":$((200 + RANDOM % 800))}"
done
echo "    8 sleep jobs → emails queue"

for i in $(seq 1 4); do
  submit "$Q_WEBHOOKS" "{\"type\":\"sleep\",\"duration_ms\":$((100 + RANDOM % 400))}"
done
echo "    4 sleep jobs → webhooks queue"

# ── 6. Delayed jobs ─────────────────────────────────────────────────────────

echo "--- Submitting delayed jobs"
submit "$Q_REPORTS" '{"type":"sleep","duration_ms":1000}' '"delay_seconds":10'
submit "$Q_REPORTS" '{"type":"sleep","duration_ms":500}'  '"delay_seconds":20'
submit "$Q_REPORTS" '{"type":"sleep","duration_ms":2000}' '"delay_seconds":30'
echo "    3 delayed jobs → reports queue (10s, 20s, 30s)"

# ── 7. Recurring (cron) jobs ─────────────────────────────────────────────────

echo "--- Submitting recurring jobs"
submit "$Q_EMAILS" '{"type":"sleep","duration_ms":200}' '"cron_expr":"*/2 * * * *"'
echo "    1 cron job (every 2 min) → emails queue"

# ── 8. Failing jobs (will retry then DLQ) ────────────────────────────────────

echo "--- Submitting failing jobs"
submit "$Q_WEBHOOKS" '{"type":"always_fail","message":"simulated timeout"}' '"max_attempts":3'
submit "$Q_WEBHOOKS" '{"type":"always_fail","message":"502 bad gateway"}'   '"max_attempts":2'
submit "$Q_EMAILS"   '{"type":"always_fail","message":"SMTP relay error"}'  '"max_attempts":2'
echo "    3 always_fail jobs (will exhaust retries → DLQ)"

# ── 9. Idempotent job (submit twice, second is a no-op) ─────────────────────

echo "--- Submitting idempotent job (twice)"
submit "$Q_EMAILS" '{"type":"sleep","duration_ms":100}' '"idempotency_key":"payment-abc-123"'
submit "$Q_EMAILS" '{"type":"sleep","duration_ms":100}' '"idempotency_key":"payment-abc-123"'
echo "    idempotency_key=payment-abc-123 (second submit returns existing)"

# ── 10. Batch ────────────────────────────────────────────────────────────────

echo "--- Submitting batch"
curl -sf -X POST "$API/queues/$Q_REPORTS/jobs/batch" \
  -H 'Content-Type: application/json' -H "$AUTH" \
  -d '{
    "jobs": [
      {"payload":{"type":"sleep","duration_ms":300}},
      {"payload":{"type":"sleep","duration_ms":600}},
      {"payload":{"type":"sleep","duration_ms":900},"priority":10}
    ]
  }' >/dev/null
echo "    3-job batch → reports queue"

# ── Done ─────────────────────────────────────────────────────────────────────

TOTAL=22
echo ""
echo "==> Seeded $TOTAL jobs across 3 queues."
echo "    Open http://localhost:3000 to see the dashboard."
echo "    Login: admin@demo.com / demodemo123"
echo ""
echo "    Jobs will start processing immediately via the workers."
echo "    Failing jobs will exhaust retries and appear in the DLQ within ~30s."
