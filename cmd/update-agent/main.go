test breaking build

package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/coreos/pkg/flagutil"
	"github.com/golang/glog"

	"github.com/coreos/container-linux-update-operator/pkg/agent"
	"github.com/coreos/container-linux-update-operator/pkg/version"
)

var (
	node         = flag.String("node", "", "Kubernetes node name")
	printVersion = flag.Bool("version", false, "Print version and exit")
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

	a, err := agent.New(*node)
	if err != nil {
		glog.Fatalf("Failed to initialize %s: %v", os.Args[0], err)
	}

	glog.Infof("%s running", os.Args[0])

	if err := a.Run(); err != nil {
		glog.Fatalf("Error while running %s: %v", os.Args[0], err)
	}
}
