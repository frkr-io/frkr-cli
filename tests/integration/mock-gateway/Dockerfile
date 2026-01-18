FROM golang:1.24-alpine

WORKDIR /app
COPY main.go .

# Frkr-common dependency might be tricky if it's local.
# For this value-add task, we will try to minimize external deps in the mock gateway 
# OR we need to make sure the context includes the necessary modules.
# Since the mock gateway code I wrote imports "github.com/frkr-io/frkr-common/messages",
# we need to handle that.
# 
# OPTION 1: Copy the whole repo into the build context. Expensive.
# OPTION 2: Remove the dependency from the mock gateway (redefine the struct).
# 
# I choose Option 2 for simplicity and speed of the test.

ENV GOPROXY=direct
RUN go mod init mock-gateway
RUN go get github.com/frkr-io/frkr-proto@v0.3.0
RUN go get google.golang.org/grpc
RUN go get google.golang.org/protobuf
RUN go mod tidy
RUN go build -o server main.go

CMD ["./server"]
