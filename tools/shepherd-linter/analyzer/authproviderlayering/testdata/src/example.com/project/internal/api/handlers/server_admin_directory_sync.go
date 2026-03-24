package handlers

import (
	_ "example.com/project/internal/repository" // want `edge workspace must not import repository/usecase packages directly: forbidden import`
)

func Handle() {}
