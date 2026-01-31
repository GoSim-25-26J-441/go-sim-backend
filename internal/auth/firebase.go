package auth

import (
	"context"
	"fmt"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/auth"
	"google.golang.org/api/option"

	"github.com/GoSim-25-26J-441/go-sim-backend/config"
)

// InitializeFirebase initializes the Firebase Admin SDK and returns an Auth client
func InitializeFirebase(cfg *config.FirebaseConfig) (*auth.Client, error) {
	if cfg.CredentialsPath == "" {
		return nil, fmt.Errorf("FIREBASE_CREDENTIALS_PATH is required")
	}

	opt := option.WithCredentialsFile(cfg.CredentialsPath)
	app, err := firebase.NewApp(context.Background(), nil, opt)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Firebase app: %w", err)
	}

	authClient, err := app.Auth(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to get Auth client: %w", err)
	}

	return authClient, nil
}
