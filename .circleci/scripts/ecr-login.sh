#!/usr/bin/env bash
set -euo pipefail

ECR_POWER_USER_ROLE="arn:aws:iam::149465543054:role/ECRPowerUserRole"

# Assume ECRPowerUserRole and write temp credentials to BASH_ENV.
# jq output is redirected — credentials are never printed to stdout.
aws sts assume-role \
  --role-arn "$ECR_POWER_USER_ROLE" \
  --role-session-name bb-portal-build \
  | jq -r '"export AWS_ACCESS_KEY_ID=" + .Credentials.AccessKeyId,
            "export AWS_SECRET_ACCESS_KEY=" + .Credentials.SecretAccessKey,
            "export AWS_SESSION_TOKEN=" + .Credentials.SessionToken' \
  >> "$BASH_ENV"

# Source updated credentials before logging in
source "$BASH_ENV"

aws ecr get-login-password --region "$AWS_DEFAULT_REGION" | \
  crane auth login "$ECR_REGISTRY" --username AWS --password-stdin
