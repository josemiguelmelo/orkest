package environment

import (
	"errors"
	"os"

	"github.com/joho/godotenv"
)

// LoadEnvironment loads environment variables from an optional dotenv file.
// Existing process environment variables take precedence over file values.
func LoadEnvironment(path string) error {
	if err := godotenv.Load(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.New("cannot load .env: check file access and dotenv syntax")
	}
	return nil
}

// EnvironmentDefault returns a non-empty environment variable value, or fallback otherwise.
func EnvironmentDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
