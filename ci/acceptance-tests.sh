#!/bin/bash

set -e
set -u

TTL="${TTL:-60}"
TIMEOUT="${TIMEOUT:-3600}"
INTERVAL="${INTERVAL:-30}"

check_service() {
  elapsed="${TIMEOUT}"
  until [ $elapsed -le 0 ]; do
    status=$(cf service "${SERVICE_INSTANCE_NAME}")
    if echo ${status} | grep "Status: create succeeded"; then
      return 0
    elif echo ${status} | grep "Status: create failed"; then
      return 1
    fi
    let elapsed-="${INTERVAL}"
    sleep "${INTERVAL}"
  done
  return 1
}

# Authenticate
cf api "${CF_API_URL}"
cf auth "${CF_USERNAME}" "${CF_PASSWORD}"

# Create service
opts=$(jq -n --arg domain "${DOMAIN}" --arg origin "${ORIGIN}" '{domain: $domain, origin: $origin}')
cf create-service "${SERVICE_NAME}" "${PLAN_NAME}" "${SERVICE_INSTANCE_NAME}" -c "${opts}"

# Get CNAME instructions
regex="CNAME domain (.*) to (.*)\.$"
message=$(cf service cdn-route-acceptance | grep "^Message: ")
if [[ "${message}" =~ ${regex} ]]; then
  external="${BASH_REMATCH[1]}"
  internal="${BASH_REMATCH[2]}"
else
  echo "Failed to parse message: ${message}"
  exit 1
fi

# Create CNAME record
cat << EOF > ./record-sets.json
{
  "Changes": [
    {
      "Action": "CREATE",
      "ResourceRecordSet": {
        "Name": "${external}.",
        "Type": "CNAME",
        "TTL": "${TTL}",
        "ResourceRecords": [
          {"Value": "${internal}"}
        ]
      }
    }
  ]
}
EOF
aws route53 change-resource-record-sets \
  --hosted-zone-id "${HOSTED_ZONE_ID}" \
  --change-batch ./record-sets.json

# Wait for provision to complete
if ! check_service; then
  echo "Failed to update service ${SERVICE_NAME}"
  exit 1
fi

# Assert same response from app and cdn
app_resp=$(curl "${origin}")
cdn_resp=$(curl "${domain}")
if [[ "${app_resp}" -ne "${cdn_resp}" ]]; then
  echo "Got different responses from app and cdn"
  exit 1
fi
