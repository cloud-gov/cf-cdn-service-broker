#!/bin/bash

set -e
set -u
set -x

# Set defaults
TTL="${TTL:-60}"
CDN_TIMEOUT="${CDN_TIMEOUT:-7200}"

suffix="${RANDOM}"
DOMAIN=$(printf "${DOMAIN}" "${suffix}")
SERVICE_INSTANCE_NAME=$(printf "${SERVICE_INSTANCE_NAME}" "${suffix}")

path="$(dirname $0)"

# Authenticate
cf api "${CF_API_URL}"
(set +x; cf auth "${CF_USERNAME}" "${CF_PASSWORD}")

# Target
cf target -o "${CF_ORGANIZATION}" -s "${CF_SPACE}"

# Create private domain
cf create-domain "${CF_ORGANIZATION}" "${DOMAIN}"

# Create service
opts=$(jq -n --arg domain "${DOMAIN}" '{domain: $domain}')
cf create-service "${SERVICE_NAME}" "${PLAN_NAME}" "${SERVICE_INSTANCE_NAME}" -c "${opts}"

http_regex="CNAME or ALIAS domain (.*) to (.*) or"
dns_regex="name: (.*), value: (.*), ttl: (.*)"

elapsed=300
until [ "${elapsed}" -le 0 ]; do
  message=$(cf service "${SERVICE_INSTANCE_NAME}" | grep -e "^Message: " -e "^name: ")
  if [[ "${message}" =~ ${http_regex} ]]; then
    domain_external="${BASH_REMATCH[1]}"
    domain_internal="${BASH_REMATCH[2]}"
  fi
  if [[ "${message}" =~ ${dns_regex} ]]; then
    txt_name="${BASH_REMATCH[1]}"
    txt_value="${BASH_REMATCH[2]}"
    txt_ttl="${BASH_REMATCH[3]}"
  fi
  if [ -n "${domain_external:-}" ] && [ -n "${txt_name:-}" ]; then
    break
  fi
  let elapsed-=5
  sleep 5
done
if [ -z "${domain_internal:-}" ] || [ -z "${txt_name:-}" ]; then
  echo "Failed to parse message: ${message}"
  exit 1
fi

# Create DNS record(s)
cat << EOF > ./create-cname.json
{
  "Changes": [
    {
      "Action": "CREATE",
      "ResourceRecordSet": {
        "Name": "${domain_external}.",
        "Type": "CNAME",
        "TTL": ${TTL},
        "ResourceRecords": [
          {"Value": "${domain_internal}"}
        ]
      }
    }
  ]
}
EOF

if [ "${CHALLENGE_TYPE}" = "DNS-01" ]; then
  cat << EOF > ./create-txt.json
  {
    "Changes": [
      {
        "Action": "CREATE",
        "ResourceRecordSet": {
          "Name": "${txt_name}",
          "Type": "TXT",
          "TTL": ${txt_ttl},
          "ResourceRecords": [
            {"Value": "\"${txt_value}\""}
          ]
        }
      }
    ]
  }
EOF
fi

if [ "${CHALLENGE_TYPE}" = "HTTP-01" ]; then
  aws route53 change-resource-record-sets \
    --hosted-zone-id "${HOSTED_ZONE_ID}" \
    --change-batch file://./create-cname.json
elif [ "${CHALLENGE_TYPE}" = "DNS-01" ]; then
  aws route53 change-resource-record-sets \
    --hosted-zone-id "${HOSTED_ZONE_ID}" \
    --change-batch file://./create-txt.json
fi

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
  let elapsed-=60
  sleep 60
done
if [ "${updated}" != "true" ]; then
  echo "Failed to update service ${SERVICE_NAME}"
  exit 1
fi

# Create CNAME after provisioning if using DNS-01 challenge
if [ "${CHALLENGE_TYPE}" = "DNS-01" ]; then
  aws route53 change-resource-record-sets \
    --hosted-zone-id "${HOSTED_ZONE_ID}" \
    --change-batch file://./create-cname.json
fi

# Push test app
cat << EOF > "${path}/app/manifest.yml"
---
applications:
- name: cdn-broker-test-${CHALLENGE_TYPE}
  buildpack: staticfile_buildpack
  domain: ${DOMAIN}
  no-hostname: true
EOF

cf push -f "${path}/app/manifest.yml" -p "${path}/app"

# Assert expected response from cdn
elapsed="${CDN_TIMEOUT}"
until [ "${elapsed}" -le 0 ]; do
  if curl "https://${DOMAIN}" | grep "CDN Broker Test"; then
    break
  fi
  let elapsed-=60
  sleep 60
done
if [ -z "${elapsed}" ]; then
  echo "Failed to load ${DOMAIN}"
  exit 1
fi

# Delete private domain
cf delete-domain -f "${DOMAIN}"

# Delete DNS record(s)
cat << EOF > ./delete-cname.json
{
  "Changes": [
    {
      "Action": "DELETE",
      "ResourceRecordSet": {
        "Name": "${domain_external}.",
        "Type": "CNAME",
        "TTL": ${TTL},
        "ResourceRecords": [
          {"Value": "${domain_internal}"}
        ]
      }
    }
  ]
}
EOF
if [ "${CHALLENGE_TYPE}" = "DNS-01" ]; then
  cat << EOF > ./delete-txt.json
  {
    "Changes": [
      {
        "Action": "DELETE",
        "ResourceRecordSet": {
          "Name": "${txt_name}.",
          "Type": "TXT",
          "TTL": ${txt_ttl},
          "ResourceRecords": [
            {"Value": "${txt_value}"}
          ]
        }
      }
    ]
  }
EOF

aws route53 change-resource-record-sets \
  --hosted-zone-id "${HOSTED_ZONE_ID}" \
  --change-batch file://./delete-cname.json
elif [ "${CHALLENGE_TYPE}" = "DNS-01" ]; then
  aws route53 change-resource-record-sets \
    --hosted-zone-id "${HOSTED_ZONE_ID}" \
    --change-batch file://./delete-txt.json
fi

# Delete service
cf delete-service -f "${SERVICE_INSTANCE_NAME}"

# Wait for deprovision to complete
elapsed="${CDN_TIMEOUT}"
until [ "${elapsed}" -le 0 ]; do
  if cf service "${SERVICE_INSTANCE_NAME}" | grep "not found"; then
    deleted="true"
    break
  fi
  let elapsed-=60
  sleep 60
done
if [ "${deleted}" != "true" ]; then
  echo "Failed to delete service ${SERVICE_NAME}"
  exit 1
fi
