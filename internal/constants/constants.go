// Package constants has Kubernetes label and annotation constants shared by
// klocksmith and klocksmith-controller.
package constants

const (
	LabelPrefix           = "alpha.coreos.com/update1."
	LabelRebootNeeded     = LabelPrefix + "reboot-needed"
	LabelRebootInProgress = LabelPrefix + "reboot-in-progress"
	LabelOkToReboot       = LabelPrefix + "reboot-ok"
)
