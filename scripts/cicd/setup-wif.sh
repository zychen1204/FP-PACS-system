#!/usr/bin/env bash
# =============================================================================
# setup-wif.sh — One-shot, idempotent setup for GitHub Actions ↔ GCP auth.
#
# Provisions:
#   1. Service account `pacs-gha-deployer@<PROJECT>.iam.gserviceaccount.com`
#      with least-privilege roles for AR push + GKE deploy.
#   2. Workload Identity Pool `github-pool` + OIDC provider `github-provider`
#      that trusts GitHub Actions tokens from a single repository.
#   3. IAM binding that lets that repository's workflows impersonate the SA
#      (no JSON keys are ever issued).
#
# Re-running is safe: every step checks for existing state before creating.
#
# Usage:
#   PROJECT_ID=extreme-water-497313-j8 \
#   GITHUB_REPO=zychen1204/FP-PACS-system \
#   bash scripts/cicd/setup-wif.sh
#
# After it finishes, copy the printed values into GitHub repo settings:
#   Settings → Secrets and variables → Actions → Variables (vars):
#     GCP_PROJECT_ID, GCP_WIF_PROVIDER, GCP_DEPLOYER_SA,
#     GKE_CLUSTER, GKE_REGION, GKE_NAMESPACE, AR_LOCATION, AR_REPO
# =============================================================================

set -euo pipefail

PROJECT_ID=${PROJECT_ID:?PROJECT_ID is required, e.g. extreme-water-497313-j8}
GITHUB_REPO=${GITHUB_REPO:?GITHUB_REPO is required, e.g. zychen1204/FP-PACS-system}

REGION=${REGION:-asia-east1}
CLUSTER_NAME=${CLUSTER_NAME:-pacs-cluster}
NAMESPACE=${NAMESPACE:-pacs}
AR_LOCATION=${AR_LOCATION:-asia-east1}
AR_REPO=${AR_REPO:-pacs}

SA_NAME=${SA_NAME:-pacs-gha-deployer}
SA_EMAIL="${SA_NAME}@${PROJECT_ID}.iam.gserviceaccount.com"

POOL_ID=${POOL_ID:-github-pool}
PROVIDER_ID=${PROVIDER_ID:-github-provider}
ISSUER=${ISSUER:-https://token.actions.githubusercontent.com}

PROJECT_NUMBER=$(gcloud projects describe "$PROJECT_ID" --format='value(projectNumber)')
POOL_RESOURCE="projects/${PROJECT_NUMBER}/locations/global/workloadIdentityPools/${POOL_ID}"
PROVIDER_RESOURCE="${POOL_RESOURCE}/providers/${PROVIDER_ID}"
PRINCIPAL_SET="principalSet://iam.googleapis.com/${POOL_RESOURCE}/attribute.repository/${GITHUB_REPO}"

# Collects IAM commands that fail (typically when run by a non-owner) so we
# can print them at the end for a project owner to apply manually.
PENDING_OWNER_COMMANDS=()
try_iam() {
    local description=$1; shift
    if ! "$@" >/tmp/setup-wif-iam.log 2>&1; then
        echo "   ⚠️  ${description}: requires project owner — queued for manual run"
        local cmd
        cmd=$(printf ' %q' "$@")
        PENDING_OWNER_COMMANDS+=("# ${description}${cmd}")
    else
        echo "   ✅ ${description}"
    fi
}

cat <<EOF
╔════════════════════════════════════════════════════════════════════════╗
║                 GitHub Actions ↔ GCP WIF Setup                          ║
╠════════════════════════════════════════════════════════════════════════╣
║ Project          : ${PROJECT_ID} (number: ${PROJECT_NUMBER})
║ GitHub repository: ${GITHUB_REPO}
║ Service account  : ${SA_EMAIL}
║ WIF pool         : ${POOL_ID}
║ WIF provider     : ${PROVIDER_ID}
║ AR repository    : ${AR_LOCATION}/${AR_REPO}
║ GKE cluster      : ${CLUSTER_NAME} (${REGION})
╚════════════════════════════════════════════════════════════════════════╝
EOF

echo ""
echo "🔧 [1/6] Enabling required Google Cloud APIs..."
gcloud services enable \
    iam.googleapis.com \
    iamcredentials.googleapis.com \
    sts.googleapis.com \
    artifactregistry.googleapis.com \
    container.googleapis.com \
    --project="$PROJECT_ID"
echo "   ✅ APIs enabled"

echo ""
echo "👤 [2/6] Ensuring service account ${SA_EMAIL}..."
if ! gcloud iam service-accounts describe "$SA_EMAIL" --project="$PROJECT_ID" >/dev/null 2>&1; then
    gcloud iam service-accounts create "$SA_NAME" \
        --project="$PROJECT_ID" \
        --display-name="PACS GitHub Actions Deployer" \
        --description="Used by GitHub Actions to push images to AR and roll out GKE deployments."
    echo "   ✅ Service account created"
else
    echo "   ✅ Service account already exists"
fi

echo ""
echo "🛡️  [3/6] Granting least-privilege roles to ${SA_NAME}..."
# Artifact Registry writer + GKE container.developer are bound at project
# scope. Repository-scoped binding would need roles/artifactregistry.admin
# on the caller, which most demo project owners lack; project-scope is
# acceptable because PACS uses a single AR repository.
try_iam "roles/artifactregistry.writer (project scope)" \
    gcloud projects add-iam-policy-binding "$PROJECT_ID" \
        --member="serviceAccount:${SA_EMAIL}" \
        --role="roles/artifactregistry.writer" \
        --condition=None
try_iam "roles/container.developer (project scope)" \
    gcloud projects add-iam-policy-binding "$PROJECT_ID" \
        --member="serviceAccount:${SA_EMAIL}" \
        --role="roles/container.developer" \
        --condition=None

echo ""
echo "🏊 [4/6] Ensuring Workload Identity Pool ${POOL_ID}..."
if gcloud iam workload-identity-pools describe "$POOL_ID" \
        --project="$PROJECT_ID" \
        --location=global >/dev/null 2>&1; then
    echo "   ✅ Pool already exists"
else
    try_iam "create WIF pool ${POOL_ID}" \
        gcloud iam workload-identity-pools create "$POOL_ID" \
            --project="$PROJECT_ID" \
            --location=global \
            --display-name="GitHub Actions pool"
fi

echo ""
echo "🪪 [5/6] Ensuring OIDC provider ${PROVIDER_ID}..."
ATTRIBUTE_MAPPING="google.subject=assertion.sub,attribute.repository=assertion.repository,attribute.actor=assertion.actor,attribute.ref=assertion.ref"
ATTRIBUTE_CONDITION="assertion.repository == '${GITHUB_REPO}'"

if gcloud iam workload-identity-pools providers describe "$PROVIDER_ID" \
        --project="$PROJECT_ID" \
        --location=global \
        --workload-identity-pool="$POOL_ID" >/dev/null 2>&1; then
    # Keep the condition aligned with the desired repository.
    try_iam "update WIF provider condition" \
        gcloud iam workload-identity-pools providers update-oidc "$PROVIDER_ID" \
            --project="$PROJECT_ID" \
            --location=global \
            --workload-identity-pool="$POOL_ID" \
            --attribute-mapping="$ATTRIBUTE_MAPPING" \
            --attribute-condition="$ATTRIBUTE_CONDITION"
else
    try_iam "create WIF OIDC provider ${PROVIDER_ID}" \
        gcloud iam workload-identity-pools providers create-oidc "$PROVIDER_ID" \
            --project="$PROJECT_ID" \
            --location=global \
            --workload-identity-pool="$POOL_ID" \
            --display-name="GitHub OIDC" \
            --issuer-uri="$ISSUER" \
            --attribute-mapping="$ATTRIBUTE_MAPPING" \
            --attribute-condition="$ATTRIBUTE_CONDITION"
fi

echo ""
echo "🤝 [6/6] Binding repository principal to service account..."
try_iam "roles/iam.workloadIdentityUser for ${GITHUB_REPO}" \
    gcloud iam service-accounts add-iam-policy-binding "$SA_EMAIL" \
        --project="$PROJECT_ID" \
        --role="roles/iam.workloadIdentityUser" \
        --member="$PRINCIPAL_SET" \
        --condition=None

if [ ${#PENDING_OWNER_COMMANDS[@]} -ne 0 ]; then
    echo ""
    echo "════════════════════════════════════════════════════════════════════"
    echo " ⚠️  ${#PENDING_OWNER_COMMANDS[@]} IAM binding(s) require a project owner."
    echo "    Ask josh48123@gmail.com or kenneth.lin93@gmail.com to run:"
    echo "════════════════════════════════════════════════════════════════════"
    for cmd in "${PENDING_OWNER_COMMANDS[@]}"; do
        echo "$cmd"
        echo ""
    done
    echo "════════════════════════════════════════════════════════════════════"
fi

cat <<EOF

✅ WIF setup complete.

Copy these into the GitHub repo (Settings → Secrets and variables → Actions → Variables):

  GCP_PROJECT_ID    = ${PROJECT_ID}
  GCP_WIF_PROVIDER  = ${PROVIDER_RESOURCE}
  GCP_DEPLOYER_SA   = ${SA_EMAIL}
  GKE_CLUSTER       = ${CLUSTER_NAME}
  GKE_REGION        = ${REGION}
  GKE_NAMESPACE     = ${NAMESPACE}
  AR_LOCATION       = ${AR_LOCATION}
  AR_REPO           = ${AR_REPO}

No GitHub secrets are required — this setup is fully keyless.

To set them in one shot with the gh CLI:

  gh variable set GCP_PROJECT_ID   --body "${PROJECT_ID}"
  gh variable set GCP_WIF_PROVIDER --body "${PROVIDER_RESOURCE}"
  gh variable set GCP_DEPLOYER_SA  --body "${SA_EMAIL}"
  gh variable set GKE_CLUSTER      --body "${CLUSTER_NAME}"
  gh variable set GKE_REGION       --body "${REGION}"
  gh variable set GKE_NAMESPACE    --body "${NAMESPACE}"
  gh variable set AR_LOCATION      --body "${AR_LOCATION}"
  gh variable set AR_REPO          --body "${AR_REPO}"

EOF
