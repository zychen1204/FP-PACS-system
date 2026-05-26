SHELL := /usr/bin/env bash

PROJECT_NAME ?= My Project 71169
PROJECT_ID ?= extreme-water-497313-j8
REGION ?= asia-east1
CLUSTER_NAME ?= pacs-cluster
DB_INSTANCE_NAME ?= pacs-pg16
REDIS_NAME ?= pacs-redis
DB_EDITION ?= ENTERPRISE
DB_TIER ?= db-custom-2-7680
GKE_NUM_NODES ?= 1
GKE_MIN_NODES ?= 1
GKE_MAX_NODES ?= 3
GKE_MACHINE_TYPE ?= e2-standard-2
GKE_DISK_TYPE ?= pd-standard
GKE_DISK_SIZE ?= 30
NAMESPACE ?= pacs
LOCAL_URL ?= http://localhost
GKE_URL ?=
DOMAIN_NAME ?=
ENABLE_HTTPS ?=
STATIC_IP_NAME ?= pacs-ingress-ip
WAIT_FOR_CERT ?= 0
CERT_WAIT_TIMEOUT ?= 3600
BUILD_IMAGES ?= 1
RUN_MIGRATIONS ?= 1
APP_DEPLOYMENTS ?= access-api reporting-api event-processor anomaly-detector mv-refresher frontend
IMAGE_SERVICES ?= access-api event-processor reporting-api anomaly-detector mv-refresher org-sync frontend
APP_REPLICAS ?= 1
CONFIRM ?=

.DEFAULT_GOAL := help

.PHONY: help \
	compose-up compose-down compose-logs compose-ps \
	backend-test backend-vet backend-lint \
	frontend-test smoke smoke-local smoke-gke \
	local-k8s local-k8s-no-build k8s-status k8s-pods \
	gke-config gke-preflight gke-deploy gke-deploy-no-build gke-deploy-frontend gke-status gke-ingress \
	gke-https-demo gke-https-demo-domain gke-https-generate-demo-domain gke-https-ip gke-https-apply gke-https-status gke-https-wait gke-https-smoke \
	gke-logs-frontend gke-logs-access gke-logs-reporting gke-logs-events gke-debug \
	gke-get-credentials gke-app-pause gke-app-resume gke-app-delete \
	gke-resources-list gke-full-cleanup gke-images-list gke-images-delete \
	gke-seed-cloud k6-local k6-gke \
	seed-cloud-shell

help:
	@printf "PACS Make targets\n\n"
	@printf "Local Docker Compose:\n"
	@printf "  make compose-up          Start local stack with image rebuild\n"
	@printf "  make compose-down        Stop local stack and remove volumes\n"
	@printf "  make compose-logs        Follow compose logs\n"
	@printf "  make compose-ps          Show compose services\n\n"
	@printf "Tests:\n"
	@printf "  make backend-test        Run Go tests\n"
	@printf "  make backend-vet         Run go vet\n"
	@printf "  make backend-lint        Run golangci-lint\n"
	@printf "  make frontend-test       Print browser test runner path\n\n"
	@printf "Smoke tests:\n"
	@printf "  make smoke-local         Check local frontend, APIs, login, and swipe POST\n"
	@printf "  make smoke-gke           Discover GKE ingress URL and run smoke test\n"
	@printf "  make smoke BASE_URL=<url> Check a custom URL\n\n"
	@printf "Kubernetes local:\n"
	@printf "  make local-k8s           Deploy to current local Kubernetes context\n"
	@printf "  make local-k8s-no-build  Deploy local Kubernetes without rebuilding images\n"
	@printf "  make k8s-status          Show local Kubernetes resources\n"
	@printf "  make k6-local            Run local k6 shift-burst Job\n\n"
	@printf "GKE:\n"
	@printf "  make gke-config                                 Show configured GKE variables\n"
	@printf "  make gke-preflight                              Check gcloud, kubectl, and Docker\n"
	@printf "  make gke-deploy                                 Deploy to documented GKE project\n"
	@printf "  make gke-deploy-no-build                        Deploy to GKE without image rebuild\n"
	@printf "  make gke-deploy-frontend                        Build, push, and restart frontend only\n"
	@printf "  make gke-status                                 Show GKE Kubernetes resources\n"
	@printf "  make gke-ingress                                Show GKE ingress\n"
	@printf "  make gke-https-demo                            Configure demo HTTPS with sslip.io and smoke-test it\n"
	@printf "  make gke-https-generate-demo-domain            Create/reuse static IP and print the sslip.io hostname\n"
	@printf "  make gke-https-ip DOMAIN_NAME=<domain>          Reserve/show HTTPS static IP and DNS instruction\n"
	@printf "  make gke-https-apply DOMAIN_NAME=<domain>       Apply HTTPS Ingress, cert, and redirect only\n"
	@printf "  make gke-https-status DOMAIN_NAME=<domain>      Show HTTPS DNS, Ingress, and cert state\n"
	@printf "  make gke-https-wait DOMAIN_NAME=<domain>        Wait for managed certificate to become Active\n"
	@printf "  make gke-https-smoke DOMAIN_NAME=<domain>       Verify HTTP redirect and HTTPS smoke test\n"
	@printf "  make gke-seed-cloud                             Run 90,000 employee cloud seed\n"
	@printf "  make k6-gke                                     Run GKE k6 shift-burst Job\n\n"
	@printf "GKE debugging:\n"
	@printf "  make gke-debug                                  Show pods, ingress, events, and recent logs\n"
	@printf "  make gke-logs-frontend                         Follow frontend Nginx logs\n"
	@printf "  make gke-logs-access                           Follow access-api logs\n"
	@printf "  make gke-logs-reporting                        Follow reporting-api logs\n"
	@printf "  make gke-logs-events                           Show recent Kubernetes events\n\n"
	@printf "GKE pause and cleanup:\n"
	@printf "  make gke-app-pause                             Scale app workloads to zero\n"
	@printf "  make gke-app-resume                            Scale app workloads back to APP_REPLICAS\n"
	@printf "  make gke-app-delete CONFIRM=delete-app         Delete Kubernetes namespace only\n"
	@printf "  make gke-full-cleanup CONFIRM=full-cleanup     Delete GKE, Cloud SQL, and Redis\n"
	@printf "  make gke-images-list                           List GCR images\n"
	@printf "  make gke-images-delete CONFIRM=delete-images   Delete demo GCR images\n"
	@printf "  make gke-resources-list                        List remaining GCP resources\n\n"
	@printf "Useful overrides: PROJECT_ID REGION CLUSTER_NAME DB_INSTANCE_NAME REDIS_NAME NAMESPACE LOCAL_URL GKE_URL DOMAIN_NAME ENABLE_HTTPS STATIC_IP_NAME BUILD_IMAGES RUN_MIGRATIONS APP_REPLICAS\n"
	@printf "GKE_URL is optional; when empty, smoke-gke reads pacs-ingress dynamically.\n"
	@printf "For HTTPS on GKE: DOMAIN_NAME=pacs.example.com make gke-deploy\n"
	@printf "Cloud defaults come from docs/GKEDeploymentReport.md. Example: make gke-deploy PROJECT_ID=my-project-id\n"

compose-up:
	docker compose up -d --build

compose-down:
	docker compose down -v

compose-logs:
	docker compose logs -f

compose-ps:
	docker compose ps

backend-test:
	$(MAKE) -C backend test

backend-vet:
	$(MAKE) -C backend vet

backend-lint:
	$(MAKE) -C backend lint

frontend-test:
	@printf "Open frontend/test-runner.html in a browser to run the frontend tests.\n"

smoke-local:
	$(MAKE) smoke BASE_URL="$(LOCAL_URL)"

smoke-gke:
	@url="$(GKE_URL)"; \
	if [ -z "$$url" ] && [ -n "$(DOMAIN_NAME)" ]; then \
		url="https://$(DOMAIN_NAME)"; \
	fi; \
	if [ -z "$$url" ]; then \
		address=$$(kubectl get ingress pacs-ingress -n "$(NAMESPACE)" -o jsonpath='{.status.loadBalancer.ingress[0].ip}{.status.loadBalancer.ingress[0].hostname}'); \
		if [ -z "$$address" ]; then \
			printf "Could not discover GKE ingress address. Run: make gke-ingress\n"; \
			exit 1; \
		fi; \
		url="http://$$address"; \
	fi; \
	make smoke BASE_URL="$$url"

smoke:
	@if [ -z "$(BASE_URL)" ]; then \
		printf "BASE_URL is required. Example: make smoke BASE_URL=http://localhost\n"; \
		exit 1; \
	fi
	@printf "Smoke testing %s\n" "$(BASE_URL)"
	@curl -fsS "$(BASE_URL)/" >/dev/null
	@curl -fsS "$(BASE_URL)/api/healthz" >/dev/null
	@curl -fsS "$(BASE_URL)/api/report-healthz" >/dev/null
	@curl -fsS -H "Content-Type: application/json" \
		-d '{"badge_id":"B-000001"}' \
		"$(BASE_URL)/v1/dev/login" >/tmp/pacs-smoke-login.json
	@badge="B-SMOKE-$$(date +%s)"; \
	status=$$(curl -sS -o /tmp/pacs-smoke-swipe.json -w "%{http_code}" \
		-H "Content-Type: application/json" \
		-d "{\"badge_id\":\"$$badge\",\"site_id\":\"SMOKE\",\"gate_id\":\"Gate-1A\",\"direction\":\"IN\"}" \
		"$(BASE_URL)/v1/swipe"); \
	if [ "$$status" != "200" ] && [ "$$status" != "403" ]; then \
		printf "Swipe POST failed with HTTP %s\n" "$$status"; \
		cat /tmp/pacs-smoke-swipe.json; \
		exit 1; \
	fi
	@printf "Smoke test OK. Note: opening /v1/swipe in a browser is GET; the real endpoint is POST.\n"

local-k8s:
	NAMESPACE="$(NAMESPACE)" BUILD_IMAGES="$(BUILD_IMAGES)" RUN_MIGRATIONS="$(RUN_MIGRATIONS)" ./deploy-local-k8s.sh

local-k8s-no-build:
	$(MAKE) local-k8s BUILD_IMAGES=0

k8s-status:
	kubectl get all,ingress,pdb -n "$(NAMESPACE)"

k8s-pods:
	kubectl get pods -n "$(NAMESPACE)" -o wide

gke-config:
	@printf "PROJECT_NAME=%s\n" "$(PROJECT_NAME)"
	@printf "PROJECT_ID=%s\n" "$(PROJECT_ID)"
	@printf "REGION=%s\n" "$(REGION)"
	@printf "CLUSTER_NAME=%s\n" "$(CLUSTER_NAME)"
	@printf "DB_INSTANCE_NAME=%s\n" "$(DB_INSTANCE_NAME)"
	@printf "REDIS_NAME=%s\n" "$(REDIS_NAME)"
	@printf "DB_EDITION=%s\n" "$(DB_EDITION)"
	@printf "DB_TIER=%s\n" "$(DB_TIER)"
	@printf "GKE_NUM_NODES=%s\n" "$(GKE_NUM_NODES)"
	@printf "GKE_MIN_NODES=%s\n" "$(GKE_MIN_NODES)"
	@printf "GKE_MAX_NODES=%s\n" "$(GKE_MAX_NODES)"
	@printf "GKE_MACHINE_TYPE=%s\n" "$(GKE_MACHINE_TYPE)"
	@printf "GKE_DISK_TYPE=%s\n" "$(GKE_DISK_TYPE)"
	@printf "GKE_DISK_SIZE=%s\n" "$(GKE_DISK_SIZE)"
	@printf "NAMESPACE=%s\n" "$(NAMESPACE)"
	@printf "DOMAIN_NAME=%s\n" "$(DOMAIN_NAME)"
	@printf "ENABLE_HTTPS=%s\n" "$(ENABLE_HTTPS)"
	@printf "STATIC_IP_NAME=%s\n" "$(STATIC_IP_NAME)"
	@printf "WAIT_FOR_CERT=%s\n" "$(WAIT_FOR_CERT)"
	@printf "CERT_WAIT_TIMEOUT=%s\n" "$(CERT_WAIT_TIMEOUT)"
	@printf "BUILD_IMAGES=%s\n" "$(BUILD_IMAGES)"
	@printf "APP_DEPLOYMENTS=%s\n" "$(APP_DEPLOYMENTS)"
	@printf "IMAGE_SERVICES=%s\n" "$(IMAGE_SERVICES)"

gke-preflight: require-project-id
	@command -v gcloud >/dev/null 2>&1 || { printf "Missing gcloud. Install Google Cloud CLI first.\n"; exit 1; }
	@command -v kubectl >/dev/null 2>&1 || { printf "Missing kubectl. Install kubectl first.\n"; exit 1; }
	@gcloud auth list --filter=status:ACTIVE --format="value(account)" >/dev/null
	@if [ "$(BUILD_IMAGES)" = "1" ]; then \
		command -v docker >/dev/null 2>&1 || { printf "Missing docker. Install Docker Desktop and enable WSL integration.\n"; exit 1; }; \
		docker info >/dev/null 2>&1 || { \
			printf "Docker is not available in this shell.\n"; \
			printf "For WSL: start Docker Desktop, then enable Settings > Resources > WSL Integration for this distro.\n"; \
			printf "Or skip image builds if images already exist: make gke-deploy-no-build\n"; \
			exit 1; \
		}; \
	fi
	@printf "GKE preflight OK.\n"

gke-deploy: require-project-id gke-preflight
	BUILD_IMAGES="$(BUILD_IMAGES)" \
	DB_EDITION="$(DB_EDITION)" \
	DB_TIER="$(DB_TIER)" \
	GKE_NUM_NODES="$(GKE_NUM_NODES)" \
	GKE_MIN_NODES="$(GKE_MIN_NODES)" \
	GKE_MAX_NODES="$(GKE_MAX_NODES)" \
	GKE_MACHINE_TYPE="$(GKE_MACHINE_TYPE)" \
	GKE_DISK_TYPE="$(GKE_DISK_TYPE)" \
	GKE_DISK_SIZE="$(GKE_DISK_SIZE)" \
	DOMAIN_NAME="$(DOMAIN_NAME)" \
	ENABLE_HTTPS="$(ENABLE_HTTPS)" \
	STATIC_IP_NAME="$(STATIC_IP_NAME)" \
	WAIT_FOR_CERT="$(WAIT_FOR_CERT)" \
	CERT_WAIT_TIMEOUT="$(CERT_WAIT_TIMEOUT)" \
	./deploy-to-gke.sh "$(PROJECT_ID)" "$(REGION)" "$(CLUSTER_NAME)" "$(DB_INSTANCE_NAME)" "$(REDIS_NAME)"

gke-deploy-no-build:
	$(MAKE) gke-deploy BUILD_IMAGES=0

gke-deploy-frontend: require-project-id
	@command -v docker >/dev/null 2>&1 || { printf "Missing docker. Install Docker Desktop and enable WSL integration.\n"; exit 1; }
	@docker info >/dev/null 2>&1 || { printf "Docker is not available in this shell.\n"; exit 1; }
	@mkdir -p "$${DOCKER_CONFIG:-/tmp/pacs-docker-config}"
	@gcloud auth print-access-token | DOCKER_CONFIG="$${DOCKER_CONFIG:-/tmp/pacs-docker-config}" docker login -u oauth2accesstoken --password-stdin https://gcr.io >/dev/null
	DOCKER_CONFIG="$${DOCKER_CONFIG:-/tmp/pacs-docker-config}" docker build -t "gcr.io/$(PROJECT_ID)/pacs-frontend:latest" ./frontend
	DOCKER_CONFIG="$${DOCKER_CONFIG:-/tmp/pacs-docker-config}" docker push "gcr.io/$(PROJECT_ID)/pacs-frontend:latest"
	kubectl rollout restart deployment/frontend -n "$(NAMESPACE)"
	kubectl rollout status deployment/frontend -n "$(NAMESPACE)" --timeout=180s

gke-status:
	kubectl get all,ingress,pdb -n "$(NAMESPACE)"

gke-ingress:
	kubectl get ingress pacs-ingress -n "$(NAMESPACE)"

gke-https-demo-domain: require-project-id
	@if ! gcloud compute addresses describe "$(STATIC_IP_NAME)" --global --project="$(PROJECT_ID)" >/dev/null 2>&1; then \
		gcloud compute addresses create "$(STATIC_IP_NAME)" --global --project="$(PROJECT_ID)" >/dev/null; \
	fi
	@ip=$$(gcloud compute addresses describe "$(STATIC_IP_NAME)" --global --project="$(PROJECT_ID)" --format='value(address)'); \
	domain="$${ip//./-}.sslip.io"; \
	printf "%s\n" "$$domain"

gke-https-generate-demo-domain: gke-https-demo-domain

gke-https-demo: require-project-id
	@if ! gcloud compute addresses describe "$(STATIC_IP_NAME)" --global --project="$(PROJECT_ID)" >/dev/null 2>&1; then \
		gcloud compute addresses create "$(STATIC_IP_NAME)" --global --project="$(PROJECT_ID)" >/dev/null; \
	fi; \
	ip=$$(gcloud compute addresses describe "$(STATIC_IP_NAME)" --global --project="$(PROJECT_ID)" --format='value(address)'); \
	domain="$${ip//./-}.sslip.io"; \
	printf "Demo HTTPS domain: %s\n" "$$domain"; \
	make gke-https-apply DOMAIN_NAME="$$domain" PROJECT_ID="$(PROJECT_ID)" REGION="$(REGION)" CLUSTER_NAME="$(CLUSTER_NAME)" STATIC_IP_NAME="$(STATIC_IP_NAME)" NAMESPACE="$(NAMESPACE)"; \
	make gke-https-wait DOMAIN_NAME="$$domain" CERT_WAIT_TIMEOUT="$(CERT_WAIT_TIMEOUT)" NAMESPACE="$(NAMESPACE)"; \
	make gke-https-smoke DOMAIN_NAME="$$domain" NAMESPACE="$(NAMESPACE)"

gke-https-ip: require-project-id require-domain-name
	@if ! gcloud compute addresses describe "$(STATIC_IP_NAME)" --global --project="$(PROJECT_ID)" >/dev/null 2>&1; then \
		gcloud compute addresses create "$(STATIC_IP_NAME)" --global --project="$(PROJECT_ID)"; \
	fi
	@ip=$$(gcloud compute addresses describe "$(STATIC_IP_NAME)" --global --project="$(PROJECT_ID)" --format='value(address)'); \
	printf "Static IP: %s\n" "$$ip"; \
	printf "Create DNS A record: %s -> %s\n" "$(DOMAIN_NAME)" "$$ip"; \
	if command -v getent >/dev/null 2>&1; then \
		dns_ips=$$(getent ahostsv4 "$(DOMAIN_NAME)" | awk '{print $$1}' | sort -u | tr '\n' ' ' || true); \
		if [ -n "$$dns_ips" ]; then \
			printf "Current DNS A values: %s\n" "$$dns_ips"; \
		else \
			printf "Current DNS A values: <not found yet>\n"; \
		fi; \
	fi

gke-https-apply: require-project-id require-domain-name gke-https-ip
	gcloud container clusters get-credentials "$(CLUSTER_NAME)" --project="$(PROJECT_ID)" --region="$(REGION)"
	sed -e 's|DOMAIN_NAME|$(DOMAIN_NAME)|g' -e 's|STATIC_IP_NAME|$(STATIC_IP_NAME)|g' k8s/08-ingress-https.yaml | kubectl apply -f -
	@printf "Applied HTTPS Ingress for https://%s\n" "$(DOMAIN_NAME)"
	@printf "Now run: make gke-https-status DOMAIN_NAME=%s\n" "$(DOMAIN_NAME)"

gke-https-status: require-project-id require-domain-name
	@ip=$$(gcloud compute addresses describe "$(STATIC_IP_NAME)" --global --project="$(PROJECT_ID)" --format='value(address)' 2>/dev/null || true); \
	if [ -n "$$ip" ]; then \
		printf "Static IP: %s\n" "$$ip"; \
		if command -v getent >/dev/null 2>&1; then \
			dns_ips=$$(getent ahostsv4 "$(DOMAIN_NAME)" | awk '{print $$1}' | sort -u | tr '\n' ' ' || true); \
			printf "DNS A values for %s: %s\n" "$(DOMAIN_NAME)" "$${dns_ips:-<not found>}"; \
		fi; \
	else \
		printf "Static IP '%s' does not exist yet. Run: make gke-https-ip DOMAIN_NAME=%s\n" "$(STATIC_IP_NAME)" "$(DOMAIN_NAME)"; \
	fi
	@kubectl get ingress pacs-ingress -n "$(NAMESPACE)" -o wide || true
	@kubectl get managedcertificate pacs-managed-cert -n "$(NAMESPACE)" -o wide || true
	@kubectl get frontendconfig pacs-frontend-config -n "$(NAMESPACE)" -o yaml || true

gke-https-wait: require-domain-name
	@deadline=$$((SECONDS + $(CERT_WAIT_TIMEOUT))); \
	while true; do \
		status=$$(kubectl get managedcertificate pacs-managed-cert -n "$(NAMESPACE)" -o jsonpath='{.status.certificateStatus}' 2>/dev/null || true); \
		printf "ManagedCertificate status: %s\n" "$${status:-<not created>}"; \
		if [ "$$status" = "Active" ]; then \
			printf "HTTPS ready: https://%s\n" "$(DOMAIN_NAME)"; \
			exit 0; \
		fi; \
		if [ "$$SECONDS" -ge "$$deadline" ]; then \
			printf "Timed out waiting for certificate. Run: kubectl describe managedcertificate pacs-managed-cert -n %s\n" "$(NAMESPACE)"; \
			exit 1; \
		fi; \
		sleep 30; \
	done

gke-https-smoke: require-domain-name
	@redirect=$$(curl -sS -o /dev/null -w '%{http_code} %{redirect_url}' "http://$(DOMAIN_NAME)/"); \
	printf "HTTP redirect: %s\n" "$$redirect"; \
	case "$$redirect" in \
		"308 https://$(DOMAIN_NAME):443/"|"301 https://$(DOMAIN_NAME):443/"|"308 https://$(DOMAIN_NAME)/"|"301 https://$(DOMAIN_NAME)/") ;; \
		*) printf "Unexpected HTTP redirect result\n"; exit 1 ;; \
	esac
	$(MAKE) smoke BASE_URL="https://$(DOMAIN_NAME)"

gke-logs-frontend:
	kubectl logs -f -n "$(NAMESPACE)" deploy/frontend --tail=100

gke-logs-access:
	kubectl logs -f -n "$(NAMESPACE)" deploy/access-api --tail=100

gke-logs-reporting:
	kubectl logs -f -n "$(NAMESPACE)" deploy/reporting-api -c reporting-api --tail=100

gke-logs-events:
	kubectl get events -n "$(NAMESPACE)" --sort-by=.lastTimestamp

gke-debug:
	kubectl get pods,svc,ingress,deploy,job -n "$(NAMESPACE)"
	kubectl describe ingress pacs-ingress -n "$(NAMESPACE)"
	kubectl get events -n "$(NAMESPACE)" --sort-by=.lastTimestamp
	kubectl logs -n "$(NAMESPACE)" deploy/frontend --tail=80
	kubectl logs -n "$(NAMESPACE)" deploy/access-api --tail=80
	kubectl logs -n "$(NAMESPACE)" deploy/reporting-api -c reporting-api --tail=80

gke-get-credentials: require-project-id
	gcloud container clusters get-credentials "$(CLUSTER_NAME)" --project="$(PROJECT_ID)" --region="$(REGION)"

gke-app-pause:
	@for deployment in $(APP_DEPLOYMENTS); do \
		kubectl scale deployment -n "$(NAMESPACE)" "$$deployment" --replicas=0; \
	done

gke-app-resume:
	@for deployment in $(APP_DEPLOYMENTS); do \
		kubectl scale deployment -n "$(NAMESPACE)" "$$deployment" --replicas="$(APP_REPLICAS)"; \
	done

gke-app-delete:
	@if [ "$(CONFIRM)" != "delete-app" ]; then \
		printf "This deletes namespace '%s'. Run: make gke-app-delete CONFIRM=delete-app\n" "$(NAMESPACE)"; \
		exit 1; \
	fi
	kubectl delete namespace "$(NAMESPACE)"

gke-resources-list: require-project-id
	gcloud container clusters list --project="$(PROJECT_ID)"
	gcloud sql instances list --project="$(PROJECT_ID)"
	gcloud redis instances list --project="$(PROJECT_ID)" --region="$(REGION)"

gke-full-cleanup: require-project-id
	@if [ "$(CONFIRM)" != "full-cleanup" ]; then \
		printf "This deletes GKE cluster '%s', Cloud SQL '%s', and Redis '%s' in project '%s'. Run: make gke-full-cleanup CONFIRM=full-cleanup\n" "$(CLUSTER_NAME)" "$(DB_INSTANCE_NAME)" "$(REDIS_NAME)" "$(PROJECT_ID)"; \
		exit 1; \
	fi
	gcloud container clusters delete "$(CLUSTER_NAME)" --project="$(PROJECT_ID)" --region="$(REGION)" --quiet
	gcloud sql instances delete "$(DB_INSTANCE_NAME)" --project="$(PROJECT_ID)" --quiet
	gcloud redis instances delete "$(REDIS_NAME)" --project="$(PROJECT_ID)" --region="$(REGION)" --quiet

gke-images-list: require-project-id
	gcloud container images list --repository="gcr.io/$(PROJECT_ID)"

gke-images-delete: require-project-id
	@if [ "$(CONFIRM)" != "delete-images" ]; then \
		printf "This deletes demo container images from gcr.io/%s. Run: make gke-images-delete CONFIRM=delete-images\n" "$(PROJECT_ID)"; \
		exit 1; \
	fi
	@for service in $(IMAGE_SERVICES); do \
		gcloud container images delete "gcr.io/$(PROJECT_ID)/pacs-$$service:latest" --quiet; \
	done

gke-seed-cloud:
	kubectl exec -i -n "$(NAMESPACE)" pod/db-tools -c psql -- psql -v ON_ERROR_STOP=1 < scripts/cloud_migrations/0104_cloud_seed.up.sql

k6-local:
	kubectl delete job -n "$(NAMESPACE)" k6-shift-burst --ignore-not-found
	kubectl apply -f k8s/local/05-k6-load-test.yaml
	kubectl logs -f -n "$(NAMESPACE)" job/k6-shift-burst

k6-gke:
	kubectl delete job -n "$(NAMESPACE)" k6-shift-burst --ignore-not-found
	kubectl apply -f k8s/07-k6-load-test.yaml
	kubectl logs -f -n "$(NAMESPACE)" job/k6-shift-burst

seed-cloud-shell:
	kubectl exec -it -n "$(NAMESPACE)" pod/db-tools -c psql -- sh

.PHONY: require-project-id
require-project-id:
	@if [ -z "$(PROJECT_ID)" ]; then \
		printf "PROJECT_ID is required. Example: make gke-deploy PROJECT_ID=extreme-water-497313-j8\n"; \
		exit 1; \
	fi

.PHONY: require-domain-name
require-domain-name:
	@if [ -z "$(DOMAIN_NAME)" ]; then \
		printf "DOMAIN_NAME is required. Example: make gke-https-apply DOMAIN_NAME=pacs.example.com\n"; \
		exit 1; \
	fi
