.PHONY: build clean

# Build the binary to the bin folder
build:
	@mkdir -p bin
	go build -o bin/frkr ./cmd/frkr


# Clean the bin folder
clean:
	rm -rf bin

test: test-unit test-integration

test-unit:
	go test -v ./cmd/...

test-integration:
	go test -v ./tests/integration/...


