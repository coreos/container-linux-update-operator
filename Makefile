IMAGE	?= quay.io/coreos/container-linux-update-operator
TAG	?= dev

all:	bin image

bin:	bin/update-agent bin/update-operator

bin/%:
	CGO_ENABLED=0 go build -ldflags '-s -w' -tags netgo -v -o $@ ./cmd/$*

precompile:
	CGO_ENABLED=0 go test -i -tags netgo -v ./cmd/...

image:
	docker build -t $(IMAGE):$(TAG) -f Dockerfile .

clean:
	rm -rf bin

test:
	CGO_ENABLED=0 go test -tags netgo -v ./internal/...

vendor:
	glide install --strip-vendor

integration-test:

.PHONY:	all bin precompile image clean

