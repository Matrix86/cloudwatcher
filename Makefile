PROJECT_NAME := "cloudwatcher"
LDFLAGS="-s -w"

all: build-tests

test:
	@go test .

test-coverage:
	@go test -short -coverprofile cover.out -covermode=atomic .
	@cat cover.out >> coverage.txt

coverage:
	@go test -coverprofile=cover.out . && go tool cover -html=cover.out

lint:
	@golint -set_exit_status .

build-tests: clean
	@mkdir -p bin
	go build -o bin/dropbox -v -ldflags=${LDFLAGS} examples/dropbox/dropbox.go
	go build -o bin/gdrive -v -ldflags=${LDFLAGS} examples/gdrive/gdrive.go
	go build -o bin/local -v -ldflags=${LDFLAGS} examples/local/local.go
	go build -o bin/s3 -v -ldflags=${LDFLAGS} examples/s3/s3.go

clean:
	@rm -rf bin