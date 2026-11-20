BIN      := bin/parkingtwin
OUT      := outputs
FIGURES  := docs/figures
PYTHON   ?= python3

.PHONY: all build test run plots clean

all: test run plots

build:
	go build -o $(BIN) ./cmd/parkingtwin

test:
	go vet ./...
	go test ./...

run: build
	$(BIN) -scenarios scenarios/scenarios.json -out $(OUT)

plots:
	$(PYTHON) analysis/plots.py $(OUT) $(FIGURES)

clean:
	rm -rf bin $(OUT)
