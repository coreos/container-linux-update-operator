# Container Linux Update Operator

Container Linux Update Operator is a node reboot controller for Kubernetes on Container Linux Distro.
When a reboot is needed after updating the system via [update_engine](https://github.com/coreos/update_engine),
the operator will drain the node before rebooting it.

Container Linux Update Operator fulfills the same purpose as
[locksmith](https://github.com/coreos/locksmith), but has better integration
with Kubernetes by explicitly marking a node as unschedulable and deleting pods
on the node before rebooting.

## Design

[Original proposal](https://docs.google.com/document/d/1DHiB2UDBYRU6QSa2e9mCNla1qBivZDqYjBVn_DvzDWc/edit#)

Container Linux Update Operator is divided into two parts - `update-operator` and `update-agent`.

`update-agent` runs on each node, waiting for a `UPDATE_STATUS_UPDATED_NEED_REBOOT` signal via dbus from `update_engine`.
It will indicate via [node annotations](./pkg/constants/constants.go) that it needs a reboot.

`update-operator` will watch changes to node annotations, and reboot the nodes as needed.
It coordinates the reboots of multiple nodes in the cluster, ensuring that not too many are rebooting at once.

Currently, `update-operator` only reboots one node at a time.

## Requirements

- Working Kubernetes >= 1.6 on CoreOS
- `update-engine.service` should be unmasked, enabled and started in systemd
- `locksmithd.service` should be masked and stopped in systemd

## Usage

To start `update-operator` and `update-agent`:

```
# Open examples/components.yaml and edit the image tag.
kubectl create -f examples/components.yaml
```

To test that it is working, you can simulate that a reboot is needed by sshing to the node and running `locksmithctl send-need-reboot`.
