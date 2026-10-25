package main

import (
	"fmt"
	"os"
	"strings"
)

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
