package main

import (
	"flag"
	"os"

	"github.com/coreos/pkg/flagutil"
	"github.com/golang/glog"

	"github.com/coreos-inc/container-linux-update-operator/internal/agent"
)

var (
	node = flag.String("node", "", "Kubernetes node name")
)

func main() {
	flag.Set("logtostderr", "true")
	flag.Parse()

	if err := flagutil.SetFlagsFromEnv(flag.CommandLine, "UPDATE_AGENT"); err != nil {
		glog.Fatalf("Failed to parse environment variables: %v", err)
	}

	if *node == "" {
		glog.Fatal("-node is required")
	}

	a, err := agent.New(*node)
	if err != nil {
		glog.Fatalf("Failed to initialize %s: %v", os.Args[0], err)
	}

	glog.Infof("%s running", os.Args[0])

	if err := a.Run(); err != nil {
		glog.Fatalf("Error while running %s: %v", os.Args[0], err)
	}
}
