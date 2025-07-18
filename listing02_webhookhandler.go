// webhook.go
package main

import (
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "os"
    "time"

    "go.uber.org/zap"
    admissionv1 "k8s.io/api/admission/v1"
    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/runtime"
    "k8s.io/apimachinery/pkg/runtime/serializer"
)

// WebhookServer enthält die Konfiguration für den Webhook-Server
type WebhookServer struct {
    Logger     *zap.Logger
    EventsFile string
}

// Globale Variablen für die Deserialisierung von Kubernetes-Objekten
var (
    runtimeScheme = runtime.NewScheme()
    codecs        = serializer.NewCodecFactory(runtimeScheme)
    deserializer  = codecs.UniversalDeserializer()
)

// MutateHandler ist die Hauptfunktion, die für die Verarbeitung eingehender 
// Admission-Requests zuständig ist
func (wh *WebhookServer) MutateHandler(w http.ResponseWriter, r *http.Request) {
    // Logger für diesen Handler initialisieren
    logger := wh.Logger.With(zap.String("handler", "mutate"))
    logger.Info("Verarbeite eingehenden Mutating-Webhook-Request")
    
    // Request-Body lesen
    body, err := io.ReadAll(r.Body)
    if err != nil {
        // Fehlerbehandlung ist kritisch für die Stabilität des Webhooks
        // Ein fehlgeschlagener Request könnte sonst die Deployment-Pipeline blockieren
        logger.Error("Konnte Request-Body nicht lesen", zap.Error(err))
        http.Error(w, "Konnte Request-Body nicht lesen", http.StatusBadRequest)
        return
    }
    
    // Content-Type prüfen
    contentType := r.Header.Get("Content-Type")
    if contentType != "application/json" {
        logger.Error("Ungültiger Content-Type", zap.String("contentType", contentType))
        http.Error(w, "Ungültiger Content-Type, erwartet application/json", http.StatusUnsupportedMediaType)
        return
    }
    
    // AdmissionReview-Objekt deserialisieren
    // Kubernetes sendet alle Webhook-Anfragen als AdmissionReview-Objekte
    admissionReview := &admissionv1.AdmissionReview{}
    if _, _, err := deserializer.Decode(body, nil, admissionReview); err != nil {
        logger.Error("Konnte AdmissionReview nicht deserialisieren", zap.Error(err))
        http.Error(w, "Ungültiges AdmissionReview-Objekt", http.StatusBadRequest)
        return
    }
    
    // Prüfen, ob der Request vorhanden ist
    if admissionReview.Request == nil {
        logger.Error("Leerer AdmissionReview.Request")
        http.Error(w, "Leerer AdmissionReview.Request", http.StatusBadRequest)
        return
    }
    
    // Request-Daten extrahieren
    request := admissionReview.Request
    
    // Wichtige Metadaten zur Anfrage loggen
    logger.Info("Verarbeite Admission-Request",
        zap.String("uid", string(request.UID)),
        zap.String("kind", request.Kind.String()),
        zap.String("namespace", request.Namespace),
        zap.String("name", request.Name),
        zap.String("operation", string(request.Operation)))
    
    // Event in Datei protokollieren
    wh.logEvent(fmt.Sprintf("Admission-Request: UID=%s, Kind=%s, Namespace=%s, Name=%s, Operation=%s",
        request.UID, request.Kind.String(), request.Namespace, request.Name, request.Operation))
    
    // Nur Pods verarbeiten
    if request.Kind.Kind != "Pod" {
        logger.Info("Ignoriere Nicht-Pod-Ressource", zap.String("kind", request.Kind.Kind))
        wh.sendAdmissionResponse(w, admissionReview, nil, "")
        return
    }
    
    // Pod-Objekt aus dem Request extrahieren
    var pod corev1.Pod
    if err := json.Unmarshal(request.Object.Raw, &pod); err != nil {
        logger.Error("Konnte Pod-Objekt nicht deserialisieren", zap.Error(err))
        wh.sendAdmissionResponse(w, admissionReview, nil, "Konnte Pod-Objekt nicht deserialisieren")
        return
    }
    
    // Prüfen, ob das Label "changed" existiert und den Wert "false" hat
    if value, exists := pod.Labels["changed"]; !exists || value != "false" {
        logger.Info("Pod hat kein Label 'changed=false', keine Mutation notwendig")
        wh.sendAdmissionResponse(w, admissionReview, nil, "")
        return
    }
    
    // Patch erstellen, um das Label zu ändern
    // Hier kommt JSON-Patch zum Einsatz, um das Label zu aktualisieren
    patch := []map[string]interface{}{
        {
            "op":    "replace",
            "path":  "/metadata/labels/changed",
            "value": "true",
        },
    }
    
    // Patch in JSON-Format umwandeln
    patchBytes, err := json.Marshal(patch)
    if err != nil {
        logger.Error("Konnte Patch nicht serialisieren", zap.Error(err))
        wh.sendAdmissionResponse(w, admissionReview, nil, "Interner Fehler beim Erstellen des Patches")
        return
    }
    
    // Event protokollieren
    wh.logEvent(fmt.Sprintf("Pod %s/%s: Label 'changed' von 'false' auf 'true' geändert",
        pod.Namespace, pod.Name))
    
    // Erfolgreiche Antwort mit Patch senden
    logger.Info("Sende Mutation-Response mit Patch",
        zap.String("namespace", pod.Namespace),
        zap.String("name", pod.Name),
        zap.String("patch", string(patchBytes)))
    
    patchType := admissionv1.PatchTypeJSONPatch
    wh.sendAdmissionResponse(w, admissionReview, &patchBytes, "", &patchType)
}

// sendAdmissionResponse sendet eine AdmissionResponse zurück an den API-Server
func (wh *WebhookServer) sendAdmissionResponse(
    w http.ResponseWriter,
    admissionReview *admissionv1.AdmissionReview,
    patch *[]byte,
    message string,
    patchType *admissionv1.PatchType,
) {
    // Neue AdmissionResponse erstellen
    response := &admissionv1.AdmissionResponse{
        Allowed: true,
        UID:     admissionReview.Request.UID,
    }
    
    // Wenn ein Patch vorhanden ist, diesen zur Response hinzufügen
    if patch != nil {
        response.Patch = *patch
        response.PatchType = patchType
    }
    
    // Wenn eine Fehlermeldung vorhanden ist, diese zur Response hinzufügen
    if message != "" {
        response.Result = &metav1.Status{
            Message: message,
        }
    }
    
    // AdmissionReview mit Response erstellen
    admissionReview.Response = response
    
    // Response serialisieren und zurücksenden
    resp, err := json.Marshal(admissionReview)
    if err != nil {
        wh.Logger.Error("Konnte AdmissionReview nicht serialisieren", zap.Error(err))
        http.Error(w, "Interner Fehler", http.StatusInternalServerError)
        return
    }
    
    // HTTP-Header setzen
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK)
    
    // Response-Body schreiben
    if _, err := w.Write(resp); err != nil {
        wh.Logger.Error("Fehler beim Schreiben der Response", zap.Error(err))
    }
}

// HealthHandler implementiert einen einfachen Health-Check-Endpunkt
func (wh *WebhookServer) HealthHandler(w http.ResponseWriter, r *http.Request) {
    w.WriteHeader(http.StatusOK)
    w.Write([]byte("OK"))
}

// logEvent protokolliert ein Event in die konfigurierte Datei
func (wh *WebhookServer) logEvent(message string) {
    // Wenn keine Datei konfiguriert ist, nichts tun
    if wh.EventsFile == "" {
        return
    }
    
    // Zeitstempel für das Event erstellen
    timestamp := time.Now().Format(time.RFC3339)
    eventLine := fmt.Sprintf("[%s] %s\n", timestamp, message)
    
    // Datei im Append-Modus öffnen
    file, err := os.OpenFile(wh.EventsFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
    if err != nil {
        wh.Logger.Error("Konnte Events-Datei nicht öffnen", zap.Error(err), zap.String("file", wh.EventsFile))
        return
    }
    defer file.Close()
    
    // Event in die Datei schreiben
    if _, err := file.WriteString(eventLine); err != nil {
        wh.Logger.Error("Konnte Event nicht in Datei schreiben", zap.Error(err))
    }
}
