package docker

import "context"

// MockClient is a test double for Client.
type MockClient struct {
	Running    bool
	UsageResp  *Usage
	UsageErr   error
	PruneFunc  func(resourceType string) (int64, error)
	Pruned     []string
}

func (m *MockClient) IsRunning(ctx context.Context) bool {
	return m.Running
}

func (m *MockClient) GetUsage(ctx context.Context) (*Usage, error) {
	return m.UsageResp, m.UsageErr
}

func (m *MockClient) Prune(ctx context.Context, resourceType string) (int64, error) {
	m.Pruned = append(m.Pruned, resourceType)
	if m.PruneFunc != nil {
		return m.PruneFunc(resourceType)
	}
	return 0, nil
}
