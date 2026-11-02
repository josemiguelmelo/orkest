package task

import (
	"strings"

	modelerrors "github.com/josemiguelmelo/orkest/internal/core/model/errors"
)

func ValidateWorkOptions(sessionMode, promptText, commandID string) error {
	if sessionMode != "" && sessionMode != "continue" && sessionMode != "new" {
		return modelerrors.ErrInvalid
	}
	if len(promptText) > 256*1024 || len(commandID) > 200 || (commandID != "" && strings.TrimSpace(commandID) == "") {
		return modelerrors.ErrInvalid
	}
	return nil
}
