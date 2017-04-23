.PHONY:	all bin precompile image clean test

all:	bin

bin:	bin/update-agent bin/update-operator

bin/%:
	CGO_ENABLED=0 go build \
							-ldflags '-s -w -X github.com/coreos/container-linux-update-operator/pkg/version.Version=$(shell cat VERSION) -X github.com/coreos/container-linux-update-operator/pkg/version.Commit=$(shell git rev-parse HEAD)' \
							-tags netgo -o $@ ./cmd/$*

precompile:
	CGO_ENABLED=0 go test -i -tags netgo ./cmd/...

image:
	./build/build-image.sh

clean:
	rm -rf bin

test:
	CGO_ENABLED=0 go test -tags netgo -v ./pkg/...

vendor:
	glide install --strip-vendor

integration-test:
