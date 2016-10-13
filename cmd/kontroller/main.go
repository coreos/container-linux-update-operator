package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/coreos/pkg/flagutil"

	"github.com/coreos-inc/klocksmith/internal/analytics"
	"github.com/coreos-inc/klocksmith/internal/controller"
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
	if err := flagutil.SetFlagsFromEnv(flags, "KONTROLLER"); err != nil {
		log.Fatalf("Failed to parse environment variables: %v", err)
	}

	if *analyticsEnabled {
		analytics.Enable()
	}

	ko, err := controller.New()
	if err != nil {
		log.Fatalf("Failed to initialize kontroller: %v", err)
	}

	log.Print("kontroller running")

	analytics.ControllerStarted()

	if err := ko.Run(); err != nil {
		log.Fatalf("Error while running klocksmith: %v", err)
	}
}
