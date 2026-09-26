#!/usr/bin/env sh
set -eu

project_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$project_root"
set -a
if [ -f .env ]; then . ./.env; else . ./.env.example; fi
set +a

(command -v jq >/dev/null 2>&1) || { echo "jq is required for API validation" >&2; exit 1; }

(cd backend && go test ./... && go test -race ./... && go vet ./... && go build ./...)
(cd frontend && npm install --no-audit --no-fund && npm run typecheck && npm run build)
docker compose config --quiet
docker compose down -v --remove-orphans
docker compose up -d --build

cleanup() { docker compose down -v --remove-orphans; }
if [ "${KEEP_RUNNING:-0}" = "1" ]; then
  trap cleanup INT TERM
else
  trap cleanup EXIT INT TERM
fi

i=0
until curl -fsS "http://127.0.0.1:${BACKEND_PORT:-19515}/healthz" | jq -e '.data.status == "ok" and .data.database == "ready" and .data.redis == "ready"' >/dev/null; do
  i=$((i+1))
  [ "$i" -lt 60 ] || { docker compose logs; exit 1; }
  sleep 2
done
curl -fsS "http://127.0.0.1:${FRONTEND_PORT:-18515}/" >/dev/null

login_token() {
  curl -fsS -X POST "http://127.0.0.1:${BACKEND_PORT:-19515}/api/auth/login" \
    -H 'Content-Type: application/json' \
    -d "{\"username\":\"$1\",\"password\":\"Admin123!\"}" | jq -er '.data.token'
}

admin_token=$(login_token admin)
reviewer_token=$(login_token reviewer)
operator_token=$(login_token operator)
viewer_token=$(login_token viewer)

curl -fsS "http://127.0.0.1:${BACKEND_PORT}/api/session" -H "Authorization: Bearer $viewer_token" \
  | jq -e '.data.role == "viewer" and (.data.requestId | length > 0)' >/dev/null
curl -fsS "http://127.0.0.1:${BACKEND_PORT}/api/parts?page=1&pageSize=20" -H "Authorization: Bearer $viewer_token" \
  | jq -e '.data | length >= 3' >/dev/null

viewer_write_status=$(curl -sS -o /dev/null -w '%{http_code}' -X POST "http://127.0.0.1:${BACKEND_PORT}/api/parts" \
  -H "Authorization: Bearer $viewer_token" -H 'Content-Type: application/json' -d '{}')
[ "$viewer_write_status" = "403" ]

now=$(date -u '+%Y-%m-%dT%H:%M:%SZ')
suffix=$(date +%s)

authorization_payload=$(printf '{"code":"AUTH-SMOKE-%s","name":"Validated component release","description":"Dual-control Compose validation","facility":"Validation Hangar","owner":"Release Desk","category":"engine","riskLevel":"high","metricValue":100,"metricUnit":"percent","effectiveAt":"%s","evidence":"inspection IR-SMOKE and certificate CERT-SMOKE","relatedCode":"PART-SMOKE"}' "$suffix" "$now")
authorization=$(curl -fsS -X POST "http://127.0.0.1:${BACKEND_PORT}/api/authorizations" \
  -H "Authorization: Bearer $operator_token" -H 'Content-Type: application/json' -H 'X-Request-ID: gb515-auth-create' \
  -d "$authorization_payload")
authorization_id=$(printf '%s' "$authorization" | jq -er '.data.id')
authorization_version=$(printf '%s' "$authorization" | jq -er '.data.version')
printf '%s' "$authorization" | jq -e '.data.status == "draft" and .data.version == 1 and .data.revisions[0].actor == "operator" and .data.revisions[0].requestId == "gb515-auth-create"' >/dev/null

authorization_review=$(curl -fsS -X POST "http://127.0.0.1:${BACKEND_PORT}/api/authorizations/${authorization_id}/transition" \
  -H "Authorization: Bearer $operator_token" -H 'Content-Type: application/json' -H 'X-Request-ID: gb515-auth-review' \
  -d "{\"status\":\"review\",\"expectedVersion\":${authorization_version},\"reason\":\"inspection and certificate evidence complete\"}")
authorization_review_version=$(printf '%s' "$authorization_review" | jq -er '.data.version')
printf '%s' "$authorization_review" | jq -e '.data.status == "review" and .data.submittedBy == "operator" and (.data.revisions | length) == 2' >/dev/null

operator_approval_status=$(curl -sS -o /dev/null -w '%{http_code}' -X POST "http://127.0.0.1:${BACKEND_PORT}/api/authorizations/${authorization_id}/transition" \
  -H "Authorization: Bearer $operator_token" -H 'Content-Type: application/json' -H 'X-Request-ID: gb515-auth-operator-denied' \
  -d "{\"status\":\"approved\",\"expectedVersion\":${authorization_review_version},\"reason\":\"operator must not self approve\"}")
[ "$operator_approval_status" = "403" ]

authorization_approved=$(curl -fsS -X POST "http://127.0.0.1:${BACKEND_PORT}/api/authorizations/${authorization_id}/transition" \
  -H "Authorization: Bearer $reviewer_token" -H 'Content-Type: application/json' -H 'X-Request-ID: gb515-auth-approve' \
  -d "{\"status\":\"approved\",\"expectedVersion\":${authorization_review_version},\"reason\":\"independent airworthiness release review passed\"}")
printf '%s' "$authorization_approved" | jq -e '.data.status == "approved" and .data.version == 3 and .data.submittedBy == "operator" and .data.reviewedBy == "reviewer" and (.data.revisions | length) == 3 and .data.revisions[2].requestId == "gb515-auth-approve"' >/dev/null

certificate_payload=$(printf '{"code":"CERT-SMOKE-%s","name":"Validated airworthiness certificate","description":"Immutable certificate validation","facility":"Validation Hangar","owner":"Certificate Desk","category":"engine","riskLevel":"medium","metricValue":100,"metricUnit":"percent","effectiveAt":"%s","evidence":"inspection report IR-SMOKE","relatedCode":"PART-SMOKE"}' "$suffix" "$now")
certificate=$(curl -fsS -X POST "http://127.0.0.1:${BACKEND_PORT}/api/certificates" \
  -H "Authorization: Bearer $operator_token" -H 'Content-Type: application/json' -H 'X-Request-ID: gb515-cert-create' \
  -d "$certificate_payload")
certificate_id=$(printf '%s' "$certificate" | jq -er '.data.id')
certificate_version=$(printf '%s' "$certificate" | jq -er '.data.version')

operator_certificate_status=$(curl -sS -o /dev/null -w '%{http_code}' -X POST "http://127.0.0.1:${BACKEND_PORT}/api/certificates/${certificate_id}/transition" \
  -H "Authorization: Bearer $operator_token" -H 'Content-Type: application/json' -H 'X-Request-ID: gb515-cert-operator-denied' \
  -d "{\"status\":\"valid\",\"expectedVersion\":${certificate_version},\"reason\":\"operator must not publish\"}")
[ "$operator_certificate_status" = "403" ]

certificate_valid=$(curl -fsS -X POST "http://127.0.0.1:${BACKEND_PORT}/api/certificates/${certificate_id}/transition" \
  -H "Authorization: Bearer $reviewer_token" -H 'Content-Type: application/json' -H 'X-Request-ID: gb515-cert-publish' \
  -d "{\"status\":\"valid\",\"expectedVersion\":${certificate_version},\"reason\":\"independent certificate evidence review passed\"}")
printf '%s' "$certificate_valid" | jq -e '.data.status == "valid" and .data.version == 2 and .data.preparedBy == "operator" and .data.verifiedBy == "reviewer" and (.data.revisions | length) == 2 and .data.revisions[1].requestId == "gb515-cert-publish"' >/dev/null

curl -fsS "http://127.0.0.1:${BACKEND_PORT}/api/audits/ReleaseAuthorization/${authorization_id}?limit=10" \
  -H "Authorization: Bearer $admin_token" \
  | jq -e '[.data[].requestId] | index("gb515-auth-create") != null and index("gb515-auth-review") != null and index("gb515-auth-approve") != null' >/dev/null

# 部件装机履历：放行 -> 装机 -> 拦截重复装机/重复占位 -> 卸载回检查中
install_part_payload=$(printf '{"code":"INST-SMOKE-%s","name":"装机履历验证部件","description":"Install history validation","facility":"Validation Hangar","owner":"Line Station","category":"engine","riskLevel":"high","metricValue":100,"metricUnit":"percent","effectiveAt":"%s","evidence":"release and install evidence","relatedCode":"PART-SMOKE"}' "$suffix" "$now")
install_part=$(curl -fsS -X POST "http://127.0.0.1:${BACKEND_PORT}/api/parts" \
  -H "Authorization: Bearer $operator_token" -H 'Content-Type: application/json' -H 'X-Request-ID: gb515-install-part-create' \
  -d "$install_part_payload")
install_part_id=$(printf '%s' "$install_part" | jq -er '.data.id')
curl -fsS -X POST "http://127.0.0.1:${BACKEND_PORT}/api/parts/${install_part_id}/transition" \
  -H "Authorization: Bearer $operator_token" -H 'Content-Type: application/json' \
  -d '{"status":"inspection","expectedVersion":1,"reason":"move to inspection for install validation"}' >/dev/null
install_part_released=$(curl -fsS -X POST "http://127.0.0.1:${BACKEND_PORT}/api/parts/${install_part_id}/transition" \
  -H "Authorization: Bearer $operator_token" -H 'Content-Type: application/json' \
  -d '{"status":"released","expectedVersion":2,"reason":"inspection passed, ready for installation"}')
install_part_version=$(printf '%s' "$install_part_released" | jq -er '.data.version')

# 未放行部件不允许走通用迁移直接到 installed
generic_install_status=$(curl -sS -o /dev/null -w '%{http_code}' -X POST "http://127.0.0.1:${BACKEND_PORT}/api/parts/${install_part_id}/transition" \
  -H "Authorization: Bearer $operator_token" -H 'Content-Type: application/json' \
  -d "{\"status\":\"installed\",\"expectedVersion\":${install_part_version},\"reason\":\"generic transition must be rejected\"}")
[ "$generic_install_status" = "422" ]

installed_part=$(curl -fsS -X POST "http://127.0.0.1:${BACKEND_PORT}/api/parts/${install_part_id}/install" \
  -H "Authorization: Bearer $operator_token" -H 'Content-Type: application/json' -H 'X-Request-ID: gb515-install' \
  -d '{"expectedVersion":3,"aircraftModel":"C919","aircraftSerial":"B-SMOKE-1","location":"左翼-1号挂点","installedBy":"operator"}')
printf '%s' "$installed_part" | jq -e '.data.status == "installed" and .data.version == 4 and (.data.installRecords | length) == 1 and .data.installRecords[0].aircraftModel == "C919" and .data.installRecords[0].aircraftSerial == "B-SMOKE-1" and .data.installRecords[0].location == "左翼-1号挂点" and .data.installRecords[0].installedBy == "operator"' >/dev/null

# 在装部件不允许再走通用迁移离开 installed
bypass_status=$(curl -sS -o /dev/null -w '%{http_code}' -X POST "http://127.0.0.1:${BACKEND_PORT}/api/parts/${install_part_id}/transition" \
  -H "Authorization: Bearer $operator_token" -H 'Content-Type: application/json' \
  -d '{"status":"inspection","expectedVersion":4,"reason":"must uninstall before leaving installed"}')
[ "$bypass_status" = "422" ]

# 同一部件重复装机被拒，并返回挡住它的履历记录
duplicate_part_status=$(curl -sS -o /tmp/install-duplicate-part.json -w '%{http_code}' -X POST "http://127.0.0.1:${BACKEND_PORT}/api/parts/${install_part_id}/install" \
  -H "Authorization: Bearer $operator_token" -H 'Content-Type: application/json' \
  -d '{"expectedVersion":4,"aircraftModel":"C919","aircraftSerial":"B-SMOKE-2","location":"右翼-2号挂点","installedBy":"operator"}')
[ "$duplicate_part_status" = "409" ]
jq -e '.error == "install_part_active" and .details.blocker.partCode == ("INST-SMOKE-" + env.suffix) and .details.blocker.aircraftSerial == "B-SMOKE-1" and .details.blocker.location == "左翼-1号挂点"' /tmp/install-duplicate-part.json >/dev/null

# 第二件部件放行后抢占同一架次同一安装位置被拒
occupier_payload=$(printf '{"code":"INST-SMOKE-OCC-%s","name":"装机占位验证部件","description":"Slot occupier","facility":"Validation Hangar","owner":"Line Station","category":"engine","riskLevel":"medium","metricValue":100,"metricUnit":"percent","effectiveAt":"%s","evidence":"release evidence","relatedCode":"PART-SMOKE"}' "$suffix" "$now")
occupier_id=$(curl -fsS -X POST "http://127.0.0.1:${BACKEND_PORT}/api/parts" \
  -H "Authorization: Bearer $operator_token" -H 'Content-Type: application/json' \
  -d "$occupier_payload" | jq -er '.data.id')
curl -fsS -X POST "http://127.0.0.1:${BACKEND_PORT}/api/parts/${occupier_id}/transition" \
  -H "Authorization: Bearer $operator_token" -H 'Content-Type: application/json' \
  -d '{"status":"inspection","expectedVersion":1,"reason":"to inspection"}' >/dev/null
curl -fsS -X POST "http://127.0.0.1:${BACKEND_PORT}/api/parts/${occupier_id}/transition" \
  -H "Authorization: Bearer $operator_token" -H 'Content-Type: application/json' \
  -d '{"status":"released","expectedVersion":2,"reason":"to released"}' >/dev/null
slot_status=$(curl -sS -o /tmp/install-slot.json -w '%{http_code}' -X POST "http://127.0.0.1:${BACKEND_PORT}/api/parts/${occupier_id}/install" \
  -H "Authorization: Bearer $operator_token" -H 'Content-Type: application/json' \
  -d '{"expectedVersion":3,"aircraftModel":"C919","aircraftSerial":"B-SMOKE-1","location":"左翼-1号挂点","installedBy":"operator"}')
[ "$slot_status" = "409" ]
jq -e --arg part "INST-SMOKE-$suffix" '.error == "install_slot_occupied" and .details.blocker.partCode == $part and .details.blocker.recordId == 1' /tmp/install-slot.json >/dev/null

# 卸载必须写原因，缺原因 400
uninstall_no_reason_status=$(curl -sS -o /dev/null -w '%{http_code}' -X POST "http://127.0.0.1:${BACKEND_PORT}/api/parts/${install_part_id}/uninstall" \
  -H "Authorization: Bearer $operator_token" -H 'Content-Type: application/json' \
  -d '{"expectedVersion":4}')
[ "$uninstall_no_reason_status" = "400" ]

# 卸载：部件回检查中，履历记录关闭但保留
uninstalled_part=$(curl -fsS -X POST "http://127.0.0.1:${BACKEND_PORT}/api/parts/${install_part_id}/uninstall" \
  -H "Authorization: Bearer $operator_token" -H 'Content-Type: application/json' -H 'X-Request-ID: gb515-uninstall' \
  -d '{"expectedVersion":4,"reason":"定检到期拆下"}')
printf '%s' "$uninstalled_part" | jq -e '.data.status == "inspection" and .data.version == 5 and (.data.installRecords | length) == 1 and .data.installRecords[0].removedAt != null and .data.installRecords[0].removedBy == "operator" and .data.installRecords[0].removeReason == "定检到期拆下" and .data.installRecords[0].removeRequestId == "gb515-uninstall"' >/dev/null

# 卸载后同一架次位置可被第二件部件占用
slot_freed=$(curl -fsS -X POST "http://127.0.0.1:${BACKEND_PORT}/api/parts/${occupier_id}/install" \
  -H "Authorization: Bearer $operator_token" -H 'Content-Type: application/json' -H 'X-Request-ID: gb515-install-slot-freed' \
  -d '{"expectedVersion":3,"aircraftModel":"C919","aircraftSerial":"B-SMOKE-1","location":"左翼-1号挂点","installedBy":"reviewer"}')
printf '%s' "$slot_freed" | jq -e '.data.status == "installed" and .data.installRecords[0].installedBy == "reviewer"' >/dev/null

# 装机与卸载都进审计
curl -fsS "http://127.0.0.1:${BACKEND_PORT}/api/audits/AircraftPart/${install_part_id}?limit=20" \
  -H "Authorization: Bearer $admin_token" \
  | jq -e '[.data[] | select(.action == "install" or .action == "uninstall")] | length == 2 and (map(.requestId) | index("gb515-install") != null) and (map(.requestId) | index("gb515-uninstall") != null)' >/dev/null
curl -fsS "http://127.0.0.1:${BACKEND_PORT}/api/audit-summary?windowHours=24" -H "Authorization: Bearer $admin_token" \
  | jq -e '(.data.actions | map(select(.action == "install")) | .[0].count // 0) >= 2 and (.data.actions | map(select(.action == "uninstall")) | .[0].count // 0) >= 1' >/dev/null

docker compose ps
[ "${KEEP_RUNNING:-0}" = "1" ] && echo "KEEP_RUNNING=1: containers left running for built-in Browser validation"
