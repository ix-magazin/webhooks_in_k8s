generate-certs:
	@echo "Generating self-signed certificates..."
	mkdir -p helm/certs
	# CA-Schlüssel und -Zertifikat erstellen
	openssl genrsa -out helm/certs/ca.key 2048
	openssl req -new -x509 -key helm/certs/ca.key -out helm/certs/ca.crt -days 365 -subj "/CN=Webhook CA"
	# Server-Schlüssel und CSR erstellen
	openssl genrsa -out helm/certs/tls.key 2048
	openssl req -new -key helm/certs/tls.key -out helm/certs/tls.csr -subj "/CN=$(HELM_RELEASE).$(NAMESPACE).svc"
	# Serverzertifikat mit der CA signieren
	openssl x509 -req -in helm/certs/tls.csr -CA helm/certs/ca.crt -CAkey helm/certs/ca.key -CAcreateserial -out helm/certs/tls.crt -days 365