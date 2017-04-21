package main

import (
	"context"
	"flag"
	"os"

	"github.com/coreos/pkg/flagutil"
	"github.com/golang/glog"

	"github.com/coreos/container-linux-update-operator/pkg/analytics"
	"github.com/coreos/container-linux-update-operator/pkg/operator"
)

var (
	analyticsEnabled = flag.Bool("analytics", true, "Send analytics to Google Analytics")
	manageAgent      = flag.Bool("manage-agent", true, "manage the associated update-agent")
)

func main() {
	flag.Set("logtostderr", "true")
	flag.Parse()

	if err := flagutil.SetFlagsFromEnv(flag.CommandLine, "UPDATE_OPERATOR"); err != nil {
		glog.Fatalf("Failed to parse environment variables: %v", err)
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

	if err := o.Run(context.Background()); err != nil {
		glog.Fatalf("Error while running %s: %v", os.Args[0], err)
	}
}
