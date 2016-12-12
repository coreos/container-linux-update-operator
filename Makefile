IMAGE	?= quay.io/coreos/container-linux-update-operator
TAG	?= dev

all:	bin image

bin:	bin/update-agent bin/update-operator

bin/%:
	CGO_ENABLED=0 go build -ldflags '-s -w' -tags netgo -v -o $@ ./cmd/$*

precompile:
	CGO_ENABLED=0 go test -i -tags netgo -v ./...

image:
	docker build -t $(IMAGE):$(TAG) -f Dockerfile .

clean:
	rm -rf bin

.PHONY:	all bin precompile image clean

