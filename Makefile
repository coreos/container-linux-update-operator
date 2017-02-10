.PHONY:	all bin precompile image clean

all:	bin

bin:	bin/update-agent bin/update-operator

bin/%:
	CGO_ENABLED=0 go build -ldflags '-s -w' -tags netgo -o $@ ./cmd/$*

precompile:
	CGO_ENABLED=0 go test -i -tags netgo ./cmd/...

image:
	./build/build-image.sh

clean:
	rm -rf bin

test:
	CGO_ENABLED=0 go test -tags netgo -v ./internal/...

integration-test:
