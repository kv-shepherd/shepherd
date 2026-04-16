package app

import (
	"example.com/project/internal/repository"
	"example.com/project/internal/service"
)

var _ = service.NewVMService()
var _ = &service.VMService{}
var _ = repository.NewUserRepository()
var _ = &repository.UserRepository{}
