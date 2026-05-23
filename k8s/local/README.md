# Local Kubernetes deployment

This local path does not require a GCP project and does not create cloud
resources. It runs Postgres, Redis, PACS services, db-tools, and k6 in a local
Kubernetes cluster.

## Prerequisite

Use either Docker Desktop Kubernetes or kind.

Docker Desktop:

1. Open Docker Desktop settings.
2. Enable Kubernetes.
3. Wait until `kubectl config current-context` prints a context.

kind:

```bash
kind create cluster --name pacs-local
```

## Deploy

From the repo root:

```bash
./deploy-local-k8s.sh
```

The script builds local Docker images, creates ConfigMaps from SQL and k6
files, runs migrations, and deploys the services.

## Seed PostgreSQL

```bash
kubectl exec -it -n pacs pod/db-tools -c psql -- sh
psql -v ON_ERROR_STOP=1 -f /cloud-seed/0104_cloud_seed.up.sql
```

## Run k6

```bash
kubectl delete job -n pacs k6-shift-burst --ignore-not-found
kubectl apply -f k8s/local/05-k6-load-test.yaml
kubectl logs -f -n pacs job/k6-shift-burst
```

## Optional UI

```bash
kubectl port-forward -n pacs svc/frontend 8088:80
```

Open http://localhost:8088.

## Reset

```bash
kubectl delete namespace pacs
```
