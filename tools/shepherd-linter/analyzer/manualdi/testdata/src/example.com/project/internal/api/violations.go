package api

import (
	_ "github.com/google/wire" // want `manual DI policy: Wire import`
	_ "github.com/redis/go-redis/v9" // want `manual DI policy: Redis import`

	"example.com/project/internal/repository"
	"example.com/project/internal/service"
)

var _ = service.NewVMService() // want `manual DI policy: constructor call NewVMService\(\) must stay in internal/app composition root`
var _ = &service.VMService{}   // want `manual DI policy: decentralized service.VMService struct wiring is forbidden outside internal/app`

var _ = repository.NewUserRepository() // want `manual DI policy: constructor call NewUserRepository\(\) must stay in internal/app composition root`
var _ = &repository.UserRepository{}   // want `manual DI policy: decentralized repository.UserRepository struct wiring is forbidden outside internal/app`
