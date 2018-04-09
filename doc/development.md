# Development

A Go 1.7+ environment is required. Docker and rkt (with SELinux set to Permissive) should be installed.

## Binaries

Develop the `update-operator` and `update-agent` apps locally.

```
make
```

Build  the `update-operator` and `update-agent` binaries in a build image with rkt.

```
make release-bin
```

## Container Image

Build a container image.

```
make image
```

Push a development image to a personal image repository to test it on a cluster. You may need to run `sudo docker login quay.io` if you haven't already.

```
make docker-push IMAGE_REPO=quay.io/USERNAME/container-linux-update-operator
```

Switch your personal repository to be public so images can be pulled.

## Verificaton

### Cluster

Deploy a Container Linux Kubernetes cluster that satisfies the [requirements](README.md#requirements).

* [QEMU/KVM](https://github.com/coreos/matchbox/tree/master/examples/terraform/bootkube-install)
* [Tectonic](https://github.com/coreos/tectonic-installer)

In particular, be sure to mask `locksmithd.service` on every Container Linux node.

```
sudo systemctl mask locksmithd.service --now
```

### Deploy

Edit `examples/deploy/update-operator.yaml` and `examples/deploy/update-agent.yaml` to refer to the development image. Create the `update-operator` deployment and `update-agent` daemonset.

```
kubectl apply -f examples/deploy -R
```

### Checks

Verify the `update-operator` is able to acquire a leader lock.

```sh
$ kubectl logs container-linux-update-operator-1096583598-x1m6t -n kube-system
I0622 18:42:13.594217       1 main.go:46] /bin/update-operator running
I0622 18:42:13.791474       1 leaderelection.go:179] attempting to acquire leader lease...
I0622 18:42:13.840638       1 leaderelection.go:189] successfully acquired lease kube-system/container-linux-update-operator-lock```
```

**update-agent**

Each `update-agent` pod listens to D-Bus to determine when `update-engine.service` requests a reboot

Watch the logs of the `update-agent` pod on a particular node.

```
$ kubectl get pods -o wide -n kube-system
$ kubectl logs container-linux-update-agent-ds-0h36z -f -n kube-system
I0622 20:06:08.888033       1 main.go:42] /bin/update-agent running
I0622 20:06:08.888174       1 agent.go:117] Setting info labels
I0622 20:06:08.978089       1 agent.go:123] Marking node as schedulable
I0622 20:06:08.994266       1 agent.go:134] Setting annotations map[string]string{"container-linux-update.v1.coreos.com/reboot-in-progress":"false", "container-linux-update.v1.coreos.com/reboot-needed":"false"}
I0622 20:06:09.016814       1 agent.go:149] Waiting for ok-to-reboot from controller...
I0622 20:06:09.017977       1 agent.go:211] Beginning to watch update_engine status
I0622 20:06:09.023410       1 agent.go:68] Updating status
```

Send a fake 'need reboot' signal, as though `update-engine.service` had requested a reboot or actually check for a Container Linux update (if cluster was deployed with older version).

```sh
$ ssh core@node.example.com
$ locksmithctl send-need-reboot
```

Alternately, check for a Container Linux update if the cluster was deployed with an older version of Container Linux.

```sh
$ ssh core@node.example.com
$ update_engine_client -check_for_update
$ update_engine_client -status
```

Verify `update-agent` receives the signal and annotates the node. Verify `update-operator` allows the node to reboot. Verify `update-agent` drains the node and reboots the host.

## Vendor

Install [glide](https://github.com/Masterminds/glide) and [glide-vc](https://github.com/sgotti/glide-vc) to manage dependencies in the `vendor` directory.

```sh
go get -u github.com/Masterminds/glide
go get -u github.com/sgotti/glide-vc
```

Edit `glide.yaml` add or update a dependency and make vendor.

```sh
make vendor
```

