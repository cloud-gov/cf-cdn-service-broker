#!/bin/bash 

set -euo pipefail

function run_command() {
    local command=$1
    local message=$2

    echo -n "${message}... "

    if ! output=$(bash -c "${command}" 2>&1); then
        echo -e "failed.\n\n"
        echo "Command Failed: ${command}"
        echo ""
        echo "Output: ${output}"
        exit 1
    fi
    echo "done."
}

command -v jq >/dev/null 2>&1 || { echo >&2 "jq is required but it's not installed.  Aborting."; exit 1; }
command -v bosh >/dev/null 2>&1 || { echo >&2 "bosh is required but it's not installed.  Aborting."; exit 1; }
command -v cut >/dev/null 2>&1 || { echo >&2 "cut is required but it's not installed.  Aborting."; exit 1; }


bosh vms --json | jq -r '.Tables[0].Rows[] | select(.instance|startswith("cdn_broker/")) | .instance' | while read -r instance; do
    run_command "bosh ssh ${instance} sudo monit stop cdn-broker" "stopping cdn-broker on ${instance}"
    run_command "bosh ssh ${instance} sudo monit stop cdn-cron" "stopping cdn-cron on ${instance}"
    run_command "bosh scp ./amd64/cdn-broker ${instance}:/tmp/cdn-broker" "copying cdn-broker binary to tmp on ${instance}"
    run_command "bosh scp ./amd64/cdn-cron ${instance}:/tmp/cdn-cron" "copying cdn-cron binary to tmp on ${instance}"
    run_command "bosh ssh ${instance} sudo mv /tmp/cdn-broker /var/vcap/packages/cdn-broker/bin/cdn-broker" "moving cdn-broker binary from tmp to packages on ${instance}"
    run_command "bosh ssh ${instance} sudo mv /tmp/cdn-cron /var/vcap/packages/cdn-broker/bin/cdn-cron" "moving cdn-cron binary from tmp to packages on ${instance}"
    run_command "bosh ssh ${instance} sudo monit start cdn-broker" "starting cdn-broker on ${instance}"
    run_command "bosh ssh ${instance} sudo monit start cdn-cron" "starting cdn-cron on ${instance}"
done

