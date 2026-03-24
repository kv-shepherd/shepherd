package provider

import (
	_ "example.com/project/internal/service" // want `provider implementation must not import API/service/jobs/repository/usecase/ent packages directly: forbidden import`
)

func Build() {}
