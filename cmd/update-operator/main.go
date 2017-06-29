package main

import (
	"context"
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
	agentTolerations flagutil.StringSliceFlag
	analyticsEnabled = flag.Bool("analytics", true, "Send analytics to Google Analytics")
	manageAgent      = flag.Bool("manage-agent", true, "Manage the associated update-agent")
	agentImageRepo   = flag.String("agent-image-repo", "quay.io/coreos/container-linux-update-operator", "The image to use for the managed agent, without version tag")
	printVersion     = flag.Bool("version", false, "Print version and exit")
)

func main() {
	flag.Set("logtostderr", "true")
	flag.Var(&agentTolerations, "tolerations", "Comma separated list of additional tolerations for generated agent DaemonSet spec")
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

	o, err := operator.New()
	if err != nil {
		glog.Fatalf("Failed to initialize %s: %v", os.Args[0], err)
	}

	glog.Infof("%s running", os.Args[0])

	analytics.ControllerStarted()

	if err := o.Run(context.Background(), *manageAgent, *agentImageRepo, operator.AgentTolerations(agentTolerations)); err != nil {
		glog.Fatalf("Error while running %s: %v", os.Args[0], err)
	}
}
