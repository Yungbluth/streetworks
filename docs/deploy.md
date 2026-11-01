# Google Cloud setup for the API

One-time setup for the API (permit records and search). The map is on GitHub Pages and doesn't need any of this.

I want this to cost nothing. Google's own page on capping costs says even following their example "doesn't guarantee that you won't spend more than your budget", and budget alerts show up hours late. So a burst of traffic can cost money before anything reacts. What I did to keep it small:

- One Cloud Run instance at most, no minimum, CPU only while a request is running, 10 second timeout, 256 MiB.
- Responses are gzipped and capped at 2 MB, search returns 50 hits max, and each client gets 10 requests a second. The client is the last address in `X-Forwarded-For` (Google adds that one itself, so it can't be faked). IPv6 clients count by /64.
- Artifact Registry keeps the 2 newest images of each kind. Images are built on GitHub's runners, not Cloud Build.
- $1 budget with email at 50%, 90% and 100%.
- Every budget message goes to a small private service (`cmd/killswitch`). The first one that shows any cost detaches the billing account from the project. The map still works because Pages serves it. Only permit records and search go away until billing is back.

Do 8 to 10 in one go so it isn't running for long without the kill switch.


## Before you start

You need `gcloud` and `gh`, both logged in, and a billing account you administer. Everything below runs in one shell and uses these:

```sh
PROJECT_ID=streetworks-CHANGE-ME   # 6-30 lowercase letters, digits, hyphens, unique across Google Cloud
BILLING_ACCOUNT_ID=CHANGE-ME       # ACCOUNT_ID column of `gcloud billing accounts list`
REGION=us-west1
```

New shell? Set them again, plus `PROJECT_NUMBER` after step 1.

## 1. Project and billing

```sh
gcloud projects create "$PROJECT_ID" --name="Streetworks"
gcloud config set project "$PROJECT_ID"
gcloud billing projects link "$PROJECT_ID" --billing-account="$BILLING_ACCOUNT_ID"
PROJECT_NUMBER=$(gcloud projects describe "$PROJECT_ID" --format='value(projectNumber)')
```

The free tier still needs a billing account. Nothing runs until step 8 so nothing costs anything before then.

## 2. APIs

```sh
gcloud services enable run.googleapis.com artifactregistry.googleapis.com \
  iam.googleapis.com iamcredentials.googleapis.com sts.googleapis.com \
  cloudbilling.googleapis.com pubsub.googleapis.com billingbudgets.googleapis.com
```

## 3. Image repository

```sh
gcloud artifacts repositories create streetworks \
  --repository-format=docker --location="$REGION" \
  --description="Streetworks API and kill switch images"
```

Cleanup policy: a Keep rule beats a Delete rule, so this deletes everything except the 2 newest versions of each image. Runs about once a day.

```sh
cat > cleanup-policy.json <<'EOP'
[
  {"name": "delete-old", "action": {"type": "Delete"}, "condition": {"tagState": "any"}},
  {"name": "keep-2-newest", "action": {"type": "Keep"}, "mostRecentVersions": {"keepCount": 2}}
]
EOP
gcloud artifacts repositories set-cleanup-policies streetworks \
  --location="$REGION" --policy=cleanup-policy.json
rm cleanup-policy.json
```

## 4. Let GitHub Actions sign in without a key

Workload Identity Federation swaps the workflow's GitHub token for short-lived Google credentials, so there's no key to leak. The provider only accepts jobs on `main` in this repo that use the `gcp` environment.

```sh
gcloud iam workload-identity-pools create github \
  --location=global --display-name="GitHub Actions"

gcloud iam workload-identity-pools providers create-oidc streetworks \
  --location=global --workload-identity-pool=github \
  --display-name="yungbluth/streetworks" \
  --issuer-uri="https://token.actions.githubusercontent.com" \
  --attribute-mapping="google.subject=assertion.sub,attribute.repository=assertion.repository" \
  --attribute-condition='attribute.repository == "yungbluth/streetworks" && assertion.ref == "refs/heads/main" && assertion.environment == "gcp"'
```

## 5. Service accounts

`deploy` is what the workflow signs in as. The API and the kill switch run as their own accounts.

```sh
gcloud iam service-accounts create deploy --display-name="GitHub Actions deploy"
gcloud iam service-accounts create streetworks-api --display-name="Streetworks API (runtime)"
gcloud iam service-accounts create killswitch --display-name="Billing kill switch (runtime)"

DEPLOY_SA="deploy@$PROJECT_ID.iam.gserviceaccount.com"
```

The API account gets no roles. The deploy account can manage Cloud Run, push images and act as the two runtime accounts:

```sh
gcloud projects add-iam-policy-binding "$PROJECT_ID" \
  --member="serviceAccount:$DEPLOY_SA" --role=roles/run.admin

gcloud artifacts repositories add-iam-policy-binding streetworks --location="$REGION" \
  --member="serviceAccount:$DEPLOY_SA" --role=roles/artifactregistry.writer

for sa in streetworks-api killswitch; do
  gcloud iam service-accounts add-iam-policy-binding "$sa@$PROJECT_ID.iam.gserviceaccount.com" \
    --member="serviceAccount:$DEPLOY_SA" --role=roles/iam.serviceAccountUser
done

gcloud iam service-accounts add-iam-policy-binding "$DEPLOY_SA" \
  --role=roles/iam.workloadIdentityUser \
  --member="principalSet://iam.googleapis.com/projects/$PROJECT_NUMBER/locations/global/workloadIdentityPools/github/attribute.repository/yungbluth/streetworks"
```

## 6. Kill switch permissions

To disable billing Google wants Project Billing Manager and Browser on the project, plus Billing Account Viewer on the billing account. The kill switch gets those and nothing else.

```sh
KILLSWITCH_SA="killswitch@$PROJECT_ID.iam.gserviceaccount.com"

gcloud projects add-iam-policy-binding "$PROJECT_ID" \
  --member="serviceAccount:$KILLSWITCH_SA" --role=roles/billing.projectManager
gcloud projects add-iam-policy-binding "$PROJECT_ID" \
  --member="serviceAccount:$KILLSWITCH_SA" --role=roles/browser
gcloud billing accounts add-iam-policy-binding "$BILLING_ACCOUNT_ID" \
  --member="serviceAccount:$KILLSWITCH_SA" --role=roles/billing.viewer
```

## 7. Tell the workflow where to deploy

None of these are secrets. Until `GCP_PROJECT` is set the workflow only builds and tests.

```sh
REPO=yungbluth/streetworks
gh variable set GCP_PROJECT --repo "$REPO" --body "$PROJECT_ID"
gh variable set GCP_REGION --repo "$REPO" --body "$REGION"
gh variable set GCP_WIF_PROVIDER --repo "$REPO" \
  --body "projects/$PROJECT_NUMBER/locations/global/workloadIdentityPools/github/providers/streetworks"
gh variable set GCP_DEPLOY_SA --repo "$REPO" --body "$DEPLOY_SA"
```

Also set Settings > Pages > Source to GitHub Actions.

## 8. Run the workflow once

```sh
gh workflow run deploy.yml --repo "$REPO" --ref main
gh run watch --repo "$REPO"
```

It runs the tests, deploys the API and checks `/health`, deploys the kill switch, then publishes the page. First run is about ten minutes, mostly downloading the city's data.

```sh
API_URL=$(gcloud run services describe streetworks-api --region="$REGION" --format='value(status.url)')
curl "$API_URL/health"    # should print ok
```

Cloud Run also logs every request with the full URL, which would keep whatever people type into search. Exclude that log for the API (the API's own log line leaves the query out):

```sh
FILTER="logName=\"projects/$PROJECT_ID/logs/run.googleapis.com%2Frequests\" AND resource.labels.service_name=\"streetworks-api\""
gcloud logging sinks update _Default \
  --add-exclusion="name=streetworks-api-requests,filter=$FILTER"
gcloud logging sinks describe _Default
```

## 9. Budget messages to the kill switch

The kill switch is private, only an account with Cloud Run Invoker can call it. Pub/Sub signs its pushes as `budget-push`.

```sh
gcloud pubsub topics create budget

gcloud iam service-accounts create budget-push --display-name="Pushes budget messages to the kill switch"
PUSH_SA="budget-push@$PROJECT_ID.iam.gserviceaccount.com"
gcloud run services add-iam-policy-binding killswitch --region="$REGION" \
  --member="serviceAccount:$PUSH_SA" --role=roles/run.invoker

KILLSWITCH_URL=$(gcloud run services describe killswitch --region="$REGION" --format='value(status.url)')
gcloud pubsub subscriptions create killswitch \
  --topic=budget \
  --push-endpoint="$KILLSWITCH_URL" \
  --push-auth-service-account="$PUSH_SA"
```

## 10. Budget

```sh
gcloud billing budgets create \
  --billing-project="$PROJECT_ID" \
  --billing-account="$BILLING_ACCOUNT_ID" \
  --display-name="Streetworks" \
  --budget-amount=1USD \
  --filter-projects="projects/$PROJECT_ID" \
  --threshold-rule=percent=0.5 \
  --threshold-rule=percent=0.9 \
  --threshold-rule=percent=1.0 \
  --notifications-rule-pubsub-topic="projects/$PROJECT_ID/topics/budget"
```

The thresholds email the billing admins. The topic gets a message several times a day regardless, each with the month's cost so far, and that's what the kill switch reads. Cloud Billing should already be allowed to publish to it, check:

```sh
gcloud pubsub topics get-iam-policy budget
```
