package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/coreos/pkg/flagutil"

	"github.com/coreos-inc/container-linux-update-operator/internal/analytics"
	"github.com/coreos-inc/container-linux-update-operator/internal/operator"
)

var (
	// avoid imported pkgs fiddling with flags.
	flags            = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	analyticsEnabled = flags.Bool("analytics", true, "Send analytics to Google Analytics")
	//max    = flags.Int("max", 1, "Maximum number of nodes to reboot")
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

	if err := flagutil.SetFlagsFromEnv(flags, "UPDATE_OPERATOR"); err != nil {
		log.Fatalf("Failed to parse environment variables: %v", err)
	}

	if *analyticsEnabled {
		analytics.Enable()
	}

	o, err := operator.New()
	if err != nil {
		log.Fatalf("Failed to initialize %s: %v", os.Args[0], err)
	}

	log.Printf("%s running", os.Args[0])

	analytics.ControllerStarted()

	if err := o.Run(); err != nil {
		log.Fatalf("Error while running %s: %v", os.Args[0], err)
	}
}
