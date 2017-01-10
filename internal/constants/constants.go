// Package constants has Kubernetes label and annotation constants shared by
// klocksmith and klocksmith-controller.
package constants

const (
	LabelPrefix           = "container-linux-update.v1.coreos.com/"
	LabelRebootNeeded     = LabelPrefix + "reboot-needed"
	LabelRebootInProgress = LabelPrefix + "reboot-in-progress"
	LabelOkToReboot       = LabelPrefix + "reboot-ok"

	AnnotationPrefix  = LabelPrefix
	AnnotationID      = AnnotationPrefix + "id"
	AnnotationGroup   = AnnotationPrefix + "group"
	AnnotationVersion = AnnotationPrefix + "version"
)
