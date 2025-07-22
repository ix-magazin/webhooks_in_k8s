# Webhooks in Kubernetes

Quellcode zum Artikel von Michael Stal

iX 9/2025
# iX-tract
* Webhooks dienen zur Erweiterung der Kubernetes-Standardfunktionalität.
* Sie fungieren als HTTP-Callbacks, die der Kubernetes-API-Server aufruft, wenn bestimmte Ereignisse im Cluster auftreten.
* Während Validating (Admission) Webhooks Regeln und Richtlinien prüfen, um Ressourcen wie z.B. Pods freizugeben, können Mutating (Admission) Webhooks auch Änderungen vornehmen.
* Die Implementierung von‚ Webhooks ist zwar komplex, enthält aber immer die gleichen Muster.
