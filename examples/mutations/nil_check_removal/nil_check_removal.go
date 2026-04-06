package nil_check_removal

type Service struct {
	Name string
}

func (s *Service) Start() string {
	return s.Name + " started"
}

func (s *Service) Stop() {
	s.Name = "stopped"
}

func RunService(svc *Service) string {
	if svc != nil {
		return svc.Start()
	}
	return "no service"
}

func CleanupService(svc *Service) {
	if svc != nil {
		svc.Stop()
	}
}

func GetName(svc *Service) string {
	if svc == nil {
		return "unknown"
	}
	return svc.Name
}

func ProcessItems(items []*Item) int {
	count := 0
	for _, item := range items {
		if item != nil {
			count += item.Value
		}
	}
	return count
}

type Item struct {
	Value int
}
