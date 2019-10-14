#!/usr/bin/env bash

set -eu
set -o pipefail

pr_title=$(jq -r '.pull_request.title' < ${GITHUB_EVENT_PATH})

read pr_prefix rest <<<${pr_title}

source "$(dirname ${BASH_SOURCE})/common.sh"

pr_type=$(cr::symbol-type ${pr_prefix})

summary=""
conclusion="success"
if [[ ${pr_type} == "unknown" ]]; then
    summary="You must specify an emoji at the beginning of the PR to indicate what kind of change this is.\nValid emoji: ${cr_all_pattern}.\nYou specified '${pr_prefix}'.\nSee VERSIONING.md for more information."
    conclusion="failure"
else
    summary="PR is a ${pr_type} change (${pr_prefix})."
fi

echo ::set-env name=CONCLUSION::${conclusion}
echo ::set-env name=SUMMARY::${summary}
