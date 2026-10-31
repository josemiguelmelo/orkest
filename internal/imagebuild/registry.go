package imagebuild

import "strings"

// RegistryMapping holds the registry host as seen by Docker and BuildKit.
// They can differ when BuildKit runs in Compose and Docker uses a host daemon.
type RegistryMapping struct {
	DockerHost   string
	BuildKitHost string
}

func (m RegistryMapping) ForDocker(image string) string {
	return replaceRegistryHost(image, m.BuildKitHost, m.DockerHost)
}

func (m RegistryMapping) ForBuildKit(image string) string {
	return replaceRegistryHost(image, m.DockerHost, m.BuildKitHost)
}

func replaceRegistryHost(image, from, to string) string {
	if from == "" || to == "" || from == to || !strings.HasPrefix(image, from+"/") {
		return image
	}
	return to + strings.TrimPrefix(image, from)
}
