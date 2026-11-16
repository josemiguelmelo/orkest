package platformadmin

import "context"

type Overview struct {
	EnabledUsers   int64 `json:"enabledUsers"`
	Organizations  int64 `json:"organizations"`
	Workspaces     int64 `json:"workspaces"`
	RunsLast30Days int64 `json:"runsLast30Days"`
}

type OverviewRepository interface {
	PlatformOverview(context.Context) (Overview, error)
}

type OverviewService struct {
	repository OverviewRepository
}

func NewOverviewService(repository OverviewRepository) *OverviewService {
	return &OverviewService{repository: repository}
}

func (s *OverviewService) Get(ctx context.Context) (Overview, error) {
	return s.repository.PlatformOverview(ctx)
}
