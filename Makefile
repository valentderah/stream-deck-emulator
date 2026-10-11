ifeq ($(OS),Windows_NT)
SHELL := cmd.exe
EXE := .exe
RMDIR := if exist bin rmdir /s /q
else
EXE :=
RMDIR := rm -rf
endif

BIN := bin/emulator$(EXE)
# Flags for the emulator, for example: make run ARGS="-connect plus,neo"
ARGS :=

export CGO_ENABLED := 0

.PHONY: build run vet test check clean

build:
	go build -trimpath -ldflags "-s -w" -o $(BIN) ./cmd/emulator

run: build
	$(BIN) $(ARGS)

vet:
	go vet ./...

test:
	go test ./...

check: vet test

clean:
	$(RMDIR) bin
