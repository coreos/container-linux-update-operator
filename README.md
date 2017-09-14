# Container Linux Update Operator

Container Linux Update Operator is a node reboot controller for Kubernetes running
Container Linux images. When a reboot is needed after updating the system via
[update_engine](https://github.com/coreos/update_engine), the operator will
drain the node before rebooting it.

Container Linux Update Operator fulfills the same purpose as
[locksmith](https://github.com/coreos/locksmith), but has better integration
with Kubernetes by explicitly marking a node as unschedulable and deleting pods
on the node before rebooting.

## Design

[Original proposal](https://docs.google.com/document/d/1DHiB2UDBYRU6QSa2e9mCNla1qBivZDqYjBVn_DvzDWc/edit#)

Container Linux Update Operator is divided into two parts: `update-operator` and `update-agent`.

`update-agent` runs as a DaemonSet on each node, waiting for a `UPDATE_STATUS_UPDATED_NEED_REBOOT` signal via D-Bus from `update_engine`.
It will indicate via [node annotations](./pkg/constants/constants.go) that it needs a reboot.

`update-operator` runs as a Deployment, watching changes to node annotations and reboots the nodes as needed.
It coordinates the reboots of multiple nodes in the cluster, ensuring that not too many are rebooting at once.

Currently, `update-operator` only reboots one node at a time.

## Requirements

- A Kubernetes cluster (>= 1.6) running on Container Linux
- The `update-engine.service` systemd unit on each machine should be unmasked, enabled and started in systemd
- The `locksmithd.service` systemd unit on each machine should be masked and stopped in systemd

To unmask a service, run `systemctl unmask <name>`.
To enable a service, run `systemctl enable <name>`.
To start/stop a service, run `systemctl start <name>` or `systemctl stop <name>` respectively.

## Usage

Create the `update-operator` deployment and `update-agent` daemonset.

```
kubectl create -f examples/update-operator.yaml
kubectl create -f examples/update-agent.yaml
```

## Test

To test that it is working, you can SSH to a node and trigger an update check by running `update_engine_client -check_for_update` or simulate a reboot is needed by running `locksmithctl send-need-reboot`.
