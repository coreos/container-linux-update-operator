.PHONY:	all bin image clean test vendor
export CGO_ENABLED:=0

VERSION=$(shell cat VERSION)
COMMIT=$(shell git rev-parse HEAD)

REPO=github.com/coreos/container-linux-update-operator
LD_FLAGS="-w -X $(REPO)/pkg/version.Version=$(VERSION) -X $(REPO)/pkg/version.Commit=$(COMMIT)"

all: bin

bin: bin/update-agent bin/update-operator

bin/%:
	go build -o $@ -ldflags $(LD_FLAGS) $(REPO)/cmd/$*

image:
	./build/build-image.sh

clean:
	rm -rf bin

test:
	go test -v $(REPO)/pkg/...

vendor:
	glide update --strip-vendor
