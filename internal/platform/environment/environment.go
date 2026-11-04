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

// EnvironmentDefault returns the environment variable value when set, or fallback otherwise.
func EnvironmentDefault(name, fallback string) string {
	if value, ok := os.LookupEnv(name); ok {
		return value
	}
	return fallback
}
