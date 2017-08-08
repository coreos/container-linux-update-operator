# Before and After Reboot Checks

CLUO can require custom node annotations before a node is allowed to reboot or
before a node is allowed to become schedulable after a reboot. 

## Configuring `update-operator`

Configure `update-operator` with comma-separated lists of
`--before-reboot-annotations` and `--after-refoot-annotations` that should be
required.

```bash
command:
- "/bin/update-operator"
- "--before-reboot-annotations=anno1,anno2"
- "--after-reboot-annotations=anno3,anno4"
```

## Before and After Reboot Labels

The `update-operator` labels nodes that are about to reboot with
`container-linux-update.v1.coreos.com/before-reboot=true` and labels nodes which
have just completed rebooting (but are not yet marked as scheduable) with
`container-linux-update.v1.coreos.com/after-reboot=true`. If you've required
before or after reboot annotations, `update-operator` will wait until all
the respective annotations are applied before proceeding.

## Making a Custom Check

Write your logic to perform custom before-reboot or after-reboot behavior. When
successful, ensure your code sets the annotations you've passed to
`update-operator`. When your logic finds an issue, leaving the annotations unset
will ensure cluster upgrades halt at the problematic node for a user to
intervene.

It is recommended that custom checks be implemented by a container image and
deployed using a [DaemonSet][1] with a [node selector][2] on the before-reboot
or after-reboot labels. 

```
spec:
  nodeSelector:
    container-linux-update.v1.coreos.com/before-reboot: "true"
```

Be sure your image can handle being rescheduled to a node on which it has
previously been run as the `update-operator` does not remove the before-reboot
and after-reboot labels instantaneously.

* [examples/before-reboot-daemonset.yaml][3]
* [examples/after-reboot-daemonset.yaml][4]

[1]: https://kubernetes.io/docs/concepts/workloads/controllers/daemonset/
[2]: https://kubernetes.io/docs/concepts/configuration/assign-pod-node/#nodeselector
[3]: ../examples/before-reboot-daemonset.yaml
[4]: ../examples/after-reboot-daemonset.yaml
