.PHONY: build clean

# Build the binary to the bin folder
build:
	@mkdir -p bin
	go build -o bin/frkr ./cmd/frkr

# Clean the bin folder
clean:
	rm -rf bin


