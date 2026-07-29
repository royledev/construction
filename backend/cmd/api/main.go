package main

import (
	"errors"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/royle/construction/backend/internal/delivery/http"
	mysqlrepository "github.com/royle/construction/backend/internal/repository/mysql"
	"github.com/royle/construction/backend/internal/usecase"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

const (
	httpReadHeaderTimeout = 5 * time.Second
	httpReadTimeout       = 15 * time.Second
	// PDF database/HTML/Chromium work is bounded to 45 seconds. The remaining
	// 15 seconds cover transmission of the verified maximum 32 MiB PDF.
	httpWriteTimeout = httpdelivery.DocumentPDFWorkTimeout + 15*time.Second
	httpIdleTimeout  = 60 * time.Second
)

func main() {
	dsn := requiredEnv("DATABASE_DSN")
	db, err := gorm.Open(gormmysql.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("database connection failed: %v", err)
	}
	sessions := httpdelivery.NewSessionManager(requiredEnv("SESSION_SECRET"), os.Getenv("APP_ENV") == "production")
	gotenbergTimeout, err := time.ParseDuration(env("GOTENBERG_TIMEOUT", httpdelivery.GotenbergMaxTimeout.String()))
	if err != nil {
		log.Fatalf("GOTENBERG_TIMEOUT must be a duration: %v", err)
	}
	gotenbergConcurrency, err := strconv.Atoi(env("GOTENBERG_MAX_CONCURRENCY", "2"))
	if err != nil {
		log.Fatalf("GOTENBERG_MAX_CONCURRENCY must be an integer: %v", err)
	}
	gotenberg, err := httpdelivery.NewGotenbergClientWithConcurrency(env("GOTENBERG_URL", "http://gotenberg:3000"), gotenbergTimeout, gotenbergConcurrency)
	if err != nil {
		log.Fatalf("Gotenberg configuration: %v", err)
	}
	repository := mysqlrepository.NewAccountRepository(db)
	auth := usecase.NewAuthUsecase(repository)
	router := httpdelivery.NewRouter(httpdelivery.NewAuthHandler(auth, sessions), httpdelivery.NewMasterHandler(db, gotenberg), sessions, httpdelivery.NewSessionValidator(db), env("CORS_ORIGIN", "http://localhost:5173"))
	server := newHTTPServer(env("PORT", "8080"), router)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func newHTTPServer(port string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              ":" + port,
		Handler:           handler,
		ReadHeaderTimeout: httpReadHeaderTimeout,
		ReadTimeout:       httpReadTimeout,
		WriteTimeout:      httpWriteTimeout,
		IdleTimeout:       httpIdleTimeout,
	}
}

func env(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
func requiredEnv(name string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	log.Fatalf("%s must be set", name)
	return ""
}
