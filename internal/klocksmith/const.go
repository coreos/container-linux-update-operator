package klocksmith

const (
	labelPrefix           = "alpha.coreos.com/update1."
	labelRebootNeeded     = labelPrefix + "reboot-needed"
	labelRebootInProgress = labelPrefix + "reboot-in-progress"
	labelOkToReboot       = labelPrefix + "reboot-ok"
)
