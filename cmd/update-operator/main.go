package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/coreos/pkg/flagutil"
	"github.com/golang/glog"

	"github.com/coreos/container-linux-update-operator/pkg/analytics"
	"github.com/coreos/container-linux-update-operator/pkg/operator"
	"github.com/coreos/container-linux-update-operator/pkg/version"
)

var (
	analyticsEnabled = flag.Bool("analytics", true, "Send analytics to Google Analytics")
	manageAgent      = flag.Bool("manage-agent", true, "Manage the associated update-agent")
	agentImageRepo   = flag.String("agent-image-repo", "quay.io/coreos/container-linux-update-operator", "The image to use for the managed agent, without version tag")
	printVersion     = flag.Bool("version", false, "Print version and exit")
)

func main() {
	flag.Set("logtostderr", "true")
	flag.Parse()

	if err := flagutil.SetFlagsFromEnv(flag.CommandLine, "UPDATE_OPERATOR"); err != nil {
		glog.Fatalf("Failed to parse environment variables: %v", err)
	}

	if *printVersion {
		fmt.Println(version.Format())
		os.Exit(0)
	}

	if *analyticsEnabled {
		analytics.Enable()
	}

	o, err := operator.New(operator.Config{
		ManageAgent:    *manageAgent,
		AgentImageRepo: *agentImageRepo,
	})
	if err != nil {
		glog.Fatalf("Failed to initialize %s: %v", os.Args[0], err)
	}

	glog.Infof("%s running", os.Args[0])

	analytics.ControllerStarted()

	// Run operator until the stop channel is closed
	stop := make(chan struct{})
	defer close(stop)

	if err := o.Run(stop); err != nil {
		glog.Fatalf("error while running %s: %v", os.Args[0], err)
	}
}
