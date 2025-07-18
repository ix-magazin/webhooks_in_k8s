# Makefile
.PHONY: build docker-build docker-push deploy clean

# Konfiguration
IMAGE_NAME := mein-registry/pod-label-mutator
IMAGE_TAG := latest
NAMESPACE := webhook-system
HELM_RELEASE := pod-label-mutator

# Go-Build-Konfiguration
GOOS := linux
GOARCH := amd64
CGO_ENABLED := 0

# Build des Go-Binaries
build:
	@echo "Building webhook binary..."
	GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=$(CGO_ENABLED) go build -o bin/webhook .

# Docker-Image bauen
docker-build: build
	@echo "Building Docker image..."
	docker build -t $(IMAGE_NAME):$(IMAGE_TAG) .

# Docker-Image in die Registry pushen
docker-push: docker-build
	@echo "Pushing Docker image..."
	docker push $(IMAGE_NAME):$(IMAGE_TAG)

# Namespace erstellen, falls er nicht existiert
create-namespace:
	@echo "Creating namespace $(NAMESPACE) if it doesn't exist..."
	kubectl get namespace $(NAMESPACE) || kubectl create namespace $(NAMESPACE)

# Selbstsignierte Zertifikate generieren
generate-certs:
	@echo "Generating self-signed certificates..."
	mkdir -p helm/certs
	openssl genrsa -out helm/certs/ca.key 2048
	openssl req -new -x509 -key helm/certs/ca.key -out helm/certs/ca.crt -days 365 -subj "/CN=Webhook CA"
	openssl genrsa -out helm/certs/tls.key 2048
	openssl req -new -key helm/certs/tls.key -out helm/certs/tls.csr -subj "/CN=$(HELM_RELEASE).$(NAMESPACE).svc"
	openssl x509 -req -in helm/certs/tls.csr -CA helm/certs/ca.crt -CAkey helm/certs/ca.key -CAcreateserial -out helm/certs/tls.crt -days 365

# Webhook mit Helm deployen
deploy: create-namespace generate-certs
	@echo "Deploying webhook using Helm..."
	helm upgrade --install $(HELM_RELEASE) ./helm \
		--namespace $(NAMESPACE) \
		--set image.repository=$(IMAGE_NAME) \
		--set image.tag=$(IMAGE_TAG)

# Aufräumen
clean:
	@echo "Cleaning up..."
	helm uninstall $(HELM_RELEASE) --namespace $(NAMESPACE) || true
	kubectl delete namespace $(NAMESPACE) || true
	rm -rf bin/ helm/certs/