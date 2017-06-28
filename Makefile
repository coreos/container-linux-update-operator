.PHONY:	all release-bin image clean test vendor
export CGO_ENABLED:=0

VERSION=$(shell ./build/git-version.sh)
RELEASE_VERSION=$(shell cat VERSION)
COMMIT=$(shell git rev-parse HEAD)

REPO=github.com/coreos/container-linux-update-operator
LD_FLAGS="-w -X $(REPO)/pkg/version.Version=$(RELEASE_VERSION) -X $(REPO)/pkg/version.Commit=$(COMMIT)"

IMAGE_REPO?=quay.io/coreos/container-linux-update-operator

all: bin/update-agent bin/update-operator

bin/%:
	go build -o $@ -ldflags $(LD_FLAGS) $(REPO)/cmd/$*

release-bin:
	./build/build-release.sh

test:
	go test -v $(REPO)/pkg/...

image: release-bin
	@sudo docker build --rm=true -t $(IMAGE_REPO):$(VERSION) .

docker-push: image
	@sudo docker push $(IMAGE_REPO):$(VERSION)

vendor:
	glide update --strip-vendor
	glide-vc --use-lock-file --no-tests --only-code

clean:
	rm -rf bin
