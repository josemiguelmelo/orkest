package httpapi

import (
	"net/http"

	"github.com/josemiguelmelo/orkest/internal/contracts/runtimecatalog"
)

type runtimeProviderView struct {
	Runtime       runtime.Runtime               `json:"runtime"`
	Configuration []runtime.ConfigurationOption `json:"configuration"`
}

func listRuntimeProviders(w http.ResponseWriter, _ *http.Request) {
	providers := runtime.Providers()
	result := make([]runtimeProviderView, 0, len(providers))
	for _, provider := range providers {
		result = append(result, runtimeProviderView{Runtime: provider.Runtime(), Configuration: provider.ConfigurationSchema()})
	}
	respond(w, result, nil, http.StatusOK)
}
