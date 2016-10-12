package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/coreos/pkg/flagutil"

	"github.com/coreos-inc/klocksmith/internal/klocksmith"
)

var (
	// avoid imported pkgs fiddling with flags.
	flags = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
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
	if err := flagutil.SetFlagsFromEnv(flags, "KLOCKSMITH_CONTROLLER"); err != nil {
		log.Fatalf("Failed to parse environment variables: %v", err)
	}

	ko, err := klocksmith.NewKontroller()
	if err != nil {
		log.Fatalf("Failed to initialize klocksmith-controller: %v", err)
	}

	log.Print("klocksmith-controller running")

	if err := ko.Run(); err != nil {
		log.Fatalf("Error while running klocksmith-controller: %v", err)
	}
}
