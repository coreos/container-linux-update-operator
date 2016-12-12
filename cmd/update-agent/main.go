package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/coreos/pkg/flagutil"

	"github.com/coreos-inc/container-linux-update-operator/internal/agent"
)

var (
	// avoid imported pkgs fiddling with flags.
	flags = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	node  = flags.String("node", "", "Kubernetes node name")
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	if err := flags.Parse(os.Args[1:]); err != nil {
		if err == flag.ErrHelp {
			fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
			flags.PrintDefaults()
			os.Exit(1)
		}

		log.Fatalf("Failed to parse arguments: %v", err)
	}
	if err := flagutil.SetFlagsFromEnv(flags, "UPDATE_AGENT"); err != nil {
		log.Fatalf("Failed to parse environment variables: %v", err)
	}

	if *node == "" {
		log.Fatal("-node is required")
	}

	a, err := agent.New(*node)
	if err != nil {
		log.Fatalf("Failed to initialize %s: %v", os.Args[0], err)
	}

	log.Printf("%s running", os.Args[0])

	if err := a.Run(); err != nil {
		log.Fatalf("Error while running %s: %v", os.Args[0], err)
	}
}
