package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"

	"github.com/coreos/pkg/flagutil"
	"github.com/golang/glog"

	"github.com/coreos/container-linux-update-operator/pkg/k8sutil"
	"github.com/coreos/container-linux-update-operator/pkg/operator"
	"github.com/coreos/container-linux-update-operator/pkg/version"
)

var (
	beforeRebootAnnotations flagutil.StringSliceFlag
	afterRebootAnnotations  flagutil.StringSliceFlag
	kubeconfig              = flag.String("kubeconfig", "", "Path to a kubeconfig file. Default to the in-cluster config if not provided.")
	autoLabelContainerLinux = flag.Bool("auto-label-container-linux", false, "Auto-label Container Linux nodes with agent=true (convenience)")
	rebootWindowStart       = flag.String("reboot-window-start", "", "Day of week ('Sun', 'Mon', ...; optional) and time of day at which the reboot window starts. E.g. 'Mon 14:00', '11:00'")
	rebootWindowLength      = flag.String("reboot-window-length", "", "Length of the reboot window. E.g. '1h30m'")
	printVersion            = flag.Bool("version", false, "Print version and exit")
	// deprecated
	analyticsEnabled optValue
	manageAgent      = flag.Bool("manage-agent", false, "Manage the associated update-agent")
	agentImageRepo   = flag.String("agent-image-repo", "quay.io/coreos/container-linux-update-operator", "The image to use for the managed agent, without version tag")
)

func main() {
	flag.Var(&beforeRebootAnnotations, "before-reboot-annotations", "List of comma-separated Kubernetes node annotations that must be set to 'true' before a reboot is allowed")
	flag.Var(&afterRebootAnnotations, "after-reboot-annotations", "List of comma-separated Kubernetes node annotations that must be set to 'true' before a node is marked schedulable and the operator lock is released")
	flag.Var(&analyticsEnabled, "analytics", "Send analytics to Google Analytics")

	flag.Set("logtostderr", "true")
	flag.Parse()

	if err := flagutil.SetFlagsFromEnv(flag.CommandLine, "UPDATE_OPERATOR"); err != nil {
		glog.Fatalf("Failed to parse environment variables: %v", err)
	}

	if analyticsEnabled.present {
		glog.Warning("Use of -analytics is deprecated and will be removed. Google Analytics will not be enabled.")
	}

	// respect KUBECONFIG without the prefix as well
	if *kubeconfig == "" {
		*kubeconfig = os.Getenv("KUBECONFIG")
	}

	if *printVersion {
		fmt.Println(version.Format())
		os.Exit(0)
	}

	if *manageAgent {
		glog.Warning("Use of -manage-agent=true is deprecated and will be removed in the future")
	}

	// create Kubernetes client (clientset)
	client, err := k8sutil.GetClient(*kubeconfig)
	if err != nil {
		glog.Fatalf("Failed to create Kubernetes client: %v", err)
	}

	// update-operator
	o, err := operator.New(operator.Config{
		Client:                  client,
		AutoLabelContainerLinux: *autoLabelContainerLinux,
		ManageAgent:             *manageAgent,
		AgentImageRepo:          *agentImageRepo,
		BeforeRebootAnnotations: beforeRebootAnnotations,
		AfterRebootAnnotations:  afterRebootAnnotations,
		RebootWindowStart:       *rebootWindowStart,
		RebootWindowLength:      *rebootWindowLength,
	})
	if err != nil {
		glog.Fatalf("Failed to initialize %s: %v", os.Args[0], err)
	}

	glog.Infof("%s running", os.Args[0])

	// Run operator until the stop channel is closed
	stop := make(chan struct{})
	defer close(stop)

	if err := o.Run(stop); err != nil {
		glog.Fatalf("Error while running %s: %v", os.Args[0], err)
	}
}

// optValue is a flag.Value that detects whether a user passed a flag directly.
type optValue struct {
	value   bool
	present bool
}

func (o *optValue) Set(s string) error {
	v, err := strconv.ParseBool(s)
	o.value = v
	o.present = true
	return err
}

func (o *optValue) String() string {
	return strconv.FormatBool(o.value)
}
