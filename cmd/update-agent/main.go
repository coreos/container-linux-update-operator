package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/coreos/pkg/flagutil"
	"github.com/golang/glog"

	"github.com/coreos/container-linux-update-operator/pkg/agent"
	"github.com/coreos/container-linux-update-operator/pkg/version"
)

var (
	node         = flag.String("node", "", "Kubernetes node name")
	printVersion = flag.Bool("version", false, "Print version and exit")
	reapTimeout  = flag.Int("grace-period", 600, "Period of time in seconds given to a pod to terminate when rebooting for an update")
	rebootWait   = flag.Int("reboot-wait", 0, "Period of time in seconds waiting after last pod deletion for reboot")
)

func main() {
	flag.Set("logtostderr", "true")
	flag.Parse()

	if err := flagutil.SetFlagsFromEnv(flag.CommandLine, "UPDATE_AGENT"); err != nil {
		glog.Fatalf("Failed to parse environment variables: %v", err)
	}

	if *printVersion {
		fmt.Println(version.Format())
		os.Exit(0)
	}

	if *node == "" {
		glog.Fatal("-node is required")
	}

	rt := time.Duration(*reapTimeout) * time.Second
	rw := time.Duration(*rebootWait) * time.Second

	glog.Infof("Waiting %v for reboot", rw)
	a, err := agent.New(*node, rt, rw)
	if err != nil {
		glog.Fatalf("Failed to initialize %s: %v", os.Args[0], err)
	}

	glog.Infof("%s running", os.Args[0])

	// Run agent until the stop channel is closed
	stop := make(chan struct{})
	defer close(stop)
	a.Run(stop)
}
