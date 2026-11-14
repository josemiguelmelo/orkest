package workflowstageintegration

import "errors"

var ErrUnauthenticated = errors.New("invalid integration callback authentication")
