// Package dockerrecipes provides the installation recipe shared by official and composed images.
package dockerrecipes

import _ "embed"

// InstallRuntime contains the controlled runtime installer.
//go:embed install-runtime.sh
var InstallRuntime []byte
