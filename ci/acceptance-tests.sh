#!/bin/bash

set -e
set -u

# Set defaults
TTL="${TTL:-60}"
CDN_TIMEOUT="${CDN_TIMEOUT:-7200}"

suffix="${RANDOM}"
DOMAIN=$(printf "${DOMAIN}" "${suffix}")
SERVICE_INSTANCE_NAME=$(printf "${SERVICE_INSTANCE_NAME}" "${suffix}")

# Authenticate
cf api "${CF_API_URL}"
cf auth "${CF_USERNAME}" "${CF_PASSWORD}"

# Target
cf target -o "${CF_ORGANIZATION}" -s "${CF_SPACE}"

# Create service
opts=$(jq -n --arg domain "${DOMAIN}" --arg origin "${ORIGIN}" '{domain: $domain, origin: $origin}')
cf create-service "${SERVICE_NAME}" "${PLAN_NAME}" "${SERVICE_INSTANCE_NAME}" -c "${opts}"

# Get CNAME instructions
regex="CNAME domain (.*) to (.*)\.$"

elapsed=60
until [ "${elapsed}" -le 0 ]; do
  message=$(cf service "${SERVICE_INSTANCE_NAME}" | grep "^Message: ")
  if [[ "${message}" =~ ${regex} ]]; then
    external="${BASH_REMATCH[1]}"
    internal="${BASH_REMATCH[2]}"
    break
  fi
  let elapsed-=5
  sleep 5
done
if [ -z "${internal}" ]; then
  echo "Failed to parse message: ${message}"
  exit 1
fi

# Create CNAME record
cat << EOF > ./create-cname.json
{
  "Changes": [
    {
      "Action": "CREATE",
      "ResourceRecordSet": {
        "Name": "${external}.",
        "Type": "CNAME",
        "TTL": ${TTL},
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
  --change-batch file://./create-cname.json

# Wait for provision to complete
elapsed="${CDN_TIMEOUT}"
until [ "${elapsed}" -le 0 ]; do
  status=$(cf service "${SERVICE_INSTANCE_NAME}" | grep "^Status: ")
  if [[ "${status}" =~ "succeeded" ]]; then
    updated="true"
    break
  elif [[ "${status}" =~ "failed" ]]; then
    echo "Failed to create service"
    exit 1
  fi
  let elapsed-=30
  sleep 30
done
if [ "${updated}" != "true" ]; then
  echo "Failed to update service ${SERVICE_NAME}"
  exit 1
fi

elapsed="${CDN_TIMEOUT}"
until [ "${elapsed}" -le 0 ]; do
  if cdn_resp=$(curl "https://${DOMAIN}"); then
    break
  fi
  let elapsed-=30
  sleep 30
done
if [ -z "${elapsed}"} ]; then
  echo "Failed to load ${DOMAIN}"
  exit 1
fi

# Assert same response from app and cdn
app_resp=$(curl "https://${ORIGIN}")
if [ "${app_resp}" != "${cdn_resp}" ]; then
  echo "Got different responses from app and cdn"
  exit 1
fi

# Delete CNAME record
cat << EOF > ./delete-cname.json
{
  "Changes": [
    {
      "Action": "DELETE",
      "ResourceRecordSet": {
        "Name": "${external}.",
        "Type": "CNAME",
        "TTL": ${TTL},
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
  --change-batch file://./delete-cname.json

# Delete service
cf delete-service -f "${SERVICE_INSTANCE_NAME}"

# Wait for deprovision to complete
elapsed="${CDN_TIMEOUT}"
until [ "${elapsed}" -le 0 ]; do
  if cf service "${SERVICE_INSTANCE_NAME}" | grep "not found"; then
    deleted="true"
    break
  fi
  let elapsed-=30
  sleep 30
done
if [ "${deleted}" != "true" ]; then
  echo "Failed to delete service ${SERVICE_NAME}"
  exit 1
fi
