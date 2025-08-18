.PHONY: build install install-loader

build:
	go build -o build/solver_monitor ./cmd/solver_monitor
	go build -o build/data_loader ./cmd/data_loader

install:
	go install ./cmd/solver_monitor

install-loader:
	go install ./cmd/data_loader
