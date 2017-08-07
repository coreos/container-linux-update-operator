# Node Labels and Annotations

The CLUO `update-operator` and `update-agent` manage a set of node labels and annotations to coordinate reboots among nodes receiving `update_engine` updates. CLUO label and annotation names are prefixed with "container-linux-update.v1.coreos.com/" to avoid conflicts.

A few labels may be set directly by admins to customize behavior. These are called out below. Other CLUO labels and annotations reflect coordinated state changes and should **not** be directly modified.

## Update Operator (Coordinator)

**Labels**

| name  | example    | setter | description |
|-------|------------|--------|---------------|
| agent | true/false | admin, update-operator | When the `auto-label-container-linux` compatability mode is enabled (via flag), the `update-operator` sets agent true on Container Linux nodes. This is a convenient label that users may node selector upon, if desired. |
| reboot-paused  | true/false | admin | May be set to true by an admin so the `update-operator` will ignore a node. Note that CLUO only coordinates reboots, `update_engine` still installs updates which are applied when a node reboots (e.g. powerloss). |
| before-reboot | true | update-operator | The `update-operator` sets the `before-reboot` label when a machine want to reboot. It signifies that the before-reboot checks should run on the node, if there are any. |
| after-reboot | true | update-operator | The `update-operator` sets the `after-reboot` label when a machine has completed it's reboot. It signifies that the after-reboot checks should run on the node, if there are any. |

**Annotations**

| name      | example    | setter | description |
|-----------|------------|--------|-------------|
| reboot-ok | true/false | update-operator | Annotates nodes the `update-operator` has permitted to reboot |

## Update Agent

**Labels**

| name | example | setter           | description |
|------|---------|------------------|-------------|
| id   | coreos  |  update-agent    | Reflects the ID in `/etc/os-release` |
| version | 1497.7.0 | update-agent | Reflects the VERSION in `/etc/os-release` |
| group | stable | update-agent     | Reflects the GROUP in `/usr/share/coreos/update.conf` or `/etc/coreos/update.conf` |

**Annotations**

| name | example | setter           | description |
|------|---------|------------------|-------------|
| reboot-needed  | true/false | update-agent | Set to true to request a coordinated reboot |
| reboot-in-progress | true/false | update-agent | Set to true to indicate a reboot is in progress |
| status | UPDATE_STATUS_IDLE | update-agent | Reflects the `update_engine` CurrentOperation status value |
| new-version       | 0.0.0      | update-agent | Reflects the `update_engine` NewVersion status value |
| last-checked-time | 1501621307 | update-agent | Reflects the `update_engine` LastCheckedTime status value |
