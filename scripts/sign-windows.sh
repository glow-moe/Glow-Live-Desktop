#!/usr/bin/env bash
#
# Authenticode-sign a Windows .exe with Azure Trusted / Artifact Signing, from
# Linux — no Windows box or CI needed. Uses jsign (a pure-Java signer) so it
# drops straight into the cross-compiled release flow.
#
# Requirements on the release machine:
#   - jsign >= 6.0            (apt: `jsign`, or the jar from ebourg.github.io/jsign)
#   - a JRE                   (default-jre)
#   - Azure CLI, logged in    (`az login`) with an identity that holds the
#     "Trusted Signing Certificate Profile Signer" role on the signing account
#
# Config (export before calling, e.g. in an untracked ./.signing sourced by the
# release scripts):
#   SIGN_ENDPOINT   region endpoint      (default: West US 2)
#   SIGN_ACCOUNT    signing account name
#   SIGN_PROFILE    certificate profile name
#
# Usage: bash scripts/sign-windows.sh dist/glow-live-vX-windows-x64.exe
set -euo pipefail

FILE="${1:?usage: sign-windows.sh <file.exe>}"
[ -f "$FILE" ] || { echo "sign-windows: no such file: $FILE" >&2; exit 1; }

: "${SIGN_ENDPOINT:=https://wus2.codesigning.azure.net/}"
: "${SIGN_ACCOUNT:?set SIGN_ACCOUNT (Trusted Signing account name)}"
: "${SIGN_PROFILE:?set SIGN_PROFILE (certificate profile name)}"

# Short-lived AAD token for the signing service (the token IS the keystore pass).
TOKEN="$(az account get-access-token \
  --resource https://codesigning.azure.net \
  --query accessToken -o tsv)"

echo "▸ signing $FILE  (Azure Trusted Signing, $SIGN_ACCOUNT/$SIGN_PROFILE)"
jsign \
  --storetype TRUSTEDSIGNING \
  --keystore "$SIGN_ENDPOINT" \
  --storepass "$TOKEN" \
  --alias "$SIGN_ACCOUNT/$SIGN_PROFILE" \
  --tsaurl http://timestamp.acs.microsoft.com/ \
  --tsmode RFC3161 \
  --replace \
  "$FILE"
echo "  ✓ signed + timestamped"
