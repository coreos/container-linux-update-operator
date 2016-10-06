IMAGE	?= quay.io/coreos/klocksmith
TAG	?= dev

all:	bin image

bin:	bin/klocksmith bin/kontroller

bin/%:
	CGO_ENABLED=0 go build -ldflags '-s -w' -tags netgo -v -o $@ ./cmd/$*

precompile:
	CGO_ENABLED=0 go test -i -tags netgo -v ./...

image:
	docker build -t $(IMAGE):$(TAG) -f Dockerfile .

clean:
	rm -rf bin

.PHONY:	all bin precompile image clean

