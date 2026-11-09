package model

import (
	"fmt"
	"path"
	"strings"
)

type Mount struct {
	Source   string `json:"source"`
	Target   string `json:"target"`
	ReadOnly bool   `json:"readOnly"`
}

type Settings struct {
	Enabled bool    `json:"enabled"`
	Mounts  []Mount `json:"mounts"`
}

func ValidSource(value string) bool {
	return strings.HasPrefix(value, "/") && path.Clean(value) == value && value != "/" &&
		!strings.ContainsRune(value, '\x00') && len(value) <= 4096
}

func ValidTarget(value string) bool {
	if len(value) > 4096 {
		return false
	}
	if strings.HasPrefix(value, "~/") {
		return ValidSource(strings.TrimPrefix(value, "~"))
	}
	return ValidSource(value) && strings.HasPrefix(value, "/mnt/host/")
}

func Validate(mounts []Mount) error {
	if len(mounts) > 32 {
		return fmt.Errorf("at most 32 host mounts are allowed")
	}
	for i, m := range mounts {
		if !ValidSource(m.Source) || !ValidTarget(m.Target) {
			return fmt.Errorf("mount sources must be absolute clean paths and targets must be beneath /mnt/host or ~/")
		}
		for _, other := range mounts[:i] {
			if m.Target == other.Target || strings.HasPrefix(m.Target, other.Target+"/") || strings.HasPrefix(other.Target, m.Target+"/") {
				return fmt.Errorf("mount targets must not overlap")
			}
		}
	}
	return nil
}
