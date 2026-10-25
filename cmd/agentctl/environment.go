package main

import (
	"errors"
	"os"

	"github.com/joho/godotenv"
)

// loadEnvironment leaves existing process variables intact. A missing local
// development file is optional; malformed files fail without logging their contents.
func loadEnvironment(path string) error {
	if err := godotenv.Load(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.New("cannot load .env: check file access and dotenv syntax")
	}
	return nil
}

func environmentDefault(name, fallback string) string {
	if value, ok := os.LookupEnv(name); ok {
		return value
	}
	return fallback
}
