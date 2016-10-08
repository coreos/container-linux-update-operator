# Klocksmith

Klocksmith is a node reboot controller for Kubernetes on CoreOS. When a reboot
is needed after updating the system via update_engine, klocksmith will drain
the node before rebooting it.

Klocksmith fulfills the same purpose as
[locksmith](https://github.com/coreos/locksmith), but has better integration
with Kubernetes by explicitly marking a node as unschedulable and deleting pods
on the node before rebooting.

## Design

Klocksmith is divided into two parts - `klocksmith` and `kontroller`.

`klocksmith` runs on each node, waiting for a
`UPDATE_STATUS_UPDATED_NEED_REBOOT` signal via dbus from `update_engine`. It
will indicate via labels on the node that it needs a reboot.

`kontroller` will watch changes to node labels, and reboot the nodes as needed.
It coordinates the reboots of multiple nodes in the cluster, ensuring that not
too many are rebooting at once.

Currently, `kontroller` only reboots one node at a time.

## Requirements

- Working Kubernetes >= 1.4 on CoreOS
- `update-engine.service` should be unmasked, enabled and started in systemd
- `locksmithd.service` should be masked and stopped in systemd

## Usage

To run klocksmith and kontroller, simply run

```
kubectl create -f klocksmith.yaml
```

To test that it is working, you can simulate that a reboot is needed by sshing
to the node and running `locksmithctl send-need-reboot`.

