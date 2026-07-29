package docker

import "context"

// MockClient is a test double for Client.
type MockClient struct {
	Running    bool
	UsageResp  *Usage
	UsageErr   error
	PruneFunc  func(resourceType string) (int64, error)
	Pruned     []string
	Items      map[string][]DockerItem
	ItemsErr   error
	DeleteFunc func(item DockerItem) error
	Deleted    []DockerItem
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

func (m *MockClient) ListItems(ctx context.Context, itemType string) ([]DockerItem, error) {
	if m.ItemsErr != nil {
		return nil, m.ItemsErr
	}
	items := m.Items[itemType]
	if items == nil {
		return []DockerItem{}, nil
	}
	out := make([]DockerItem, len(items))
	copy(out, items)
	return out, nil
}

func (m *MockClient) DeleteItem(ctx context.Context, item DockerItem) error {
	m.Deleted = append(m.Deleted, item)
	if m.DeleteFunc != nil {
		return m.DeleteFunc(item)
	}
	return nil
}
