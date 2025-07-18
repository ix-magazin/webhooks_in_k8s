//   main.go
package main

// Folgende Bibliotheken sind notwendig:
import (
    "context"
    "crypto/tls"
    "flag"
    "fmt"
    "net/http"
    "os"
    "os/signal"
    "syscall"
    "time"

    "go.uber.org/zap"
    "go.uber.org/zap/zapcore"
)

// Hauptfunktion, die den Webhook-Server startet
func main() {
    // Konfigurationsparameter lassen sich über Flags definieren
    var (
        certFile   string
        keyFile    string
        port       int
        logLevel   string
        eventsFile string
    )

    // Kommandozeilenparameter definieren und parsen
    // Diese Parameter können beim Start des Webhooks übergeben werden
    flag.StringVar(&certFile, "cert", "/etc/webhook/certs/tls.crt", "Pfad zum TLS-Zertifikat")
    flag.StringVar(&keyFile, "key", "/etc/webhook/certs/tls.key", "Pfad zum TLS-Schlüssel")
    flag.IntVar(&port, "port", 8443, "Port, auf dem der Webhook-Server lauscht")
    flag.StringVar(&logLevel, "log-level", "info", "Log-Level (debug, info, warn, error)")
    flag.StringVar(&eventsFile, "events-file", "events.txt", "Datei zum Protokollieren von Events")
    flag.Parse()

    // Logger initialisieren mit dem angegebenen Log-Level
    logger := initLogger(logLevel)
    defer logger.Sync()

    // Webhook-Server initialisieren
    webhookServer := &WebhookServer{
        Logger:     logger,
        EventsFile: eventsFile,
    }

    // HTTP-Server konfigurieren
    server := &http.Server{
        Addr:      fmt.Sprintf(":%d", port),
        TLSConfig: &tls.Config{MinVersion: tls.VersionTLS12},
    }

    // HTTP-Handler für verschiedene Endpunkte registrieren
    http.HandleFunc("/mutate", webhookServer.MutateHandler)
    http.HandleFunc("/health", webhookServer.HealthHandler)

    // Server in einem separaten Goroutine starten
    go func() {
        logger.Info("Starte Webhook-Server", zap.Int("port", port))
        if err := server.ListenAndServeTLS(certFile, keyFile); err != nil && err != http.ErrServerClosed {
            logger.Fatal("Fehler beim Starten des Servers", zap.Error(err))
        }
    }()

    // Graceful Shutdown konfigurieren
    // Das ermöglicht ein sauberes Herunterfahren des Servers bei Signalen wie SIGTERM
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
    <-sigChan

    logger.Info("Webhook-Server wird heruntergefahren...")
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    if err := server.Shutdown(ctx); err != nil {
        logger.Error("Fehler beim Herunterfahren des Servers", zap.Error(err))
    }

    logger.Info("Webhook-Server wurde sauber heruntergefahren")
}

// initLogger initialisiert den Zap-Logger mit dem angegebenen Log-Level
func initLogger(level string) *zap.Logger {
    // Konfiguration für den Logger erstellen
    // Hier wird das Log-Level basierend auf dem übergebenen Parameter gesetzt
    var logLevel zapcore.Level
    switch level {
    case "debug":
        logLevel = zapcore.DebugLevel
    case "info":
        logLevel = zapcore.InfoLevel
    case "warn":
        logLevel = zapcore.WarnLevel
    case "error":
        logLevel = zapcore.ErrorLevel
    default:
        logLevel = zapcore.InfoLevel
    }

    // Logger-Konfiguration erstellen
    config := zap.Config{
        Level:            zap.NewAtomicLevelAt(logLevel),
        Development:      false,
        Encoding:         "json",
        EncoderConfig:    zap.NewProductionEncoderConfig(),
        OutputPaths:      []string{"stdout"},
        ErrorOutputPaths: []string{"stderr"},
    }

    // Logger erstellen und zurückgeben
    logger, err := config.Build()
    if err != nil {
        panic(fmt.Sprintf("Fehler beim Initialisieren des Loggers: %v", err))
    }
    return logger
}