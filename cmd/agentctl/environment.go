package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

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

func appendGrantedEnvironmentVariable(env *[]string, name string) error {
	if strings.TrimSpace(name) == "" || strings.Contains(name, "=") {
		return fmt.Errorf("invalid environment variable name")
	}
	value, ok := os.LookupEnv(name)
	if !ok {
		return fmt.Errorf("environment variable %s is unset", name)
	}
	*env = append(*env, name+"="+value)
	return nil
}
