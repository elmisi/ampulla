#!/bin/sh
set -e

DSN_KEY="bc3efd409b8a493cb2ce4a74194b738c"
HOST="ampulla.elmisi.com"
PROJECT_ID="1"
EVENT_ID=$(cat /proc/sys/kernel/random/uuid | tr -d '-')

MESSAGE="${1:-test error from script}"

curl -s -X POST "https://${HOST}/api/${PROJECT_ID}/envelope/" \
  -H 'Content-Type: application/x-sentry-envelope' \
  -H "X-Sentry-Auth: Sentry sentry_version=7, sentry_key=${DSN_KEY}" \
  --data-binary "$(printf '%s\n%s\n%s' \
    "{\"event_id\":\"${EVENT_ID}\",\"dsn\":\"https://${DSN_KEY}@${HOST}/${PROJECT_ID}\"}" \
    '{"type":"event"}' \
    "{\"event_id\":\"${EVENT_ID}\",\"timestamp\":\"$(date -u +%Y-%m-%dT%H:%M:%SZ)\",\"platform\":\"other\",\"level\":\"error\",\"exception\":{\"values\":[{\"type\":\"Error\",\"value\":\"${MESSAGE}\",\"stacktrace\":{\"frames\":[{\"filename\":\"test-event.sh\",\"function\":\"main\",\"lineno\":1}]}}]}}"
  )"

echo
