package main

import (
	"context"
	"dwgd"
	"flag"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/docker/go-plugins-helpers/network"
)

var Version string

var dbFlag = flag.String("d", "/var/lib/dwgd.db", "dwgd db path")
var verboseFlag = flag.Bool("v", false, "verbose mode")
var versionFlag = flag.Bool("version", false, "print the version")
var rootlessFlag = flag.Bool("r", true, "run in rootless compatibility mode")

var pubkeyCmd = flag.NewFlagSet("pubkey", flag.ExitOnError)
var ipFlag = pubkeyCmd.String("i", "", "IP to generate public key")
var seedFlag = pubkeyCmd.String("s", "", "seed to generate public key")

func pubkey(args []string) {
	pubkeyCmd.Parse(args)

	seed := *seedFlag
	ip := *ipFlag

	if seed == "" {
		dwgd.EventsLog.Println("seed is required")
		pubkeyCmd.Usage()
		os.Exit(1)
	}
	if ip == "" {
		dwgd.EventsLog.Println("ip is required")
		os.Exit(1)
	}

	privkey, err := dwgd.GeneratePrivateKey([]byte(seed), net.ParseIP(ip))
	if err != nil {
		dwgd.EventsLog.Printf("Couldn't generate key: %s\n", err)
		os.Exit(1)
	}
	dwgd.EventsLog.Printf("%s\n", privkey.PublicKey().String())
	os.Exit(0)
}

func runDwgd(ctx context.Context, db string) {
	dwgd.DiagnosticsLog.Printf("Using db: %s\n", db)
	d, err := dwgd.NewDwgd(db)
	if err != nil {
		dwgd.DiagnosticsLog.Fatalf("Couldn't initialize driver: %s\n", err)
	}

	h := network.NewHandler(d)
	l, err := dwgd.NewUnixListener(*rootlessFlag)
	if err != nil {
		dwgd.DiagnosticsLog.Fatalf("Couldn't serve on unix socket: %s\n", err)
	}

	go func() {
		dwgd.DiagnosticsLog.Println("Serving on unix socket")
		err = h.Serve(l)
		if err != nil {
			dwgd.DiagnosticsLog.Printf("Couldn't serve on unix socket: %s\n", err)
		}
	}()

	<-ctx.Done()
	dwgd.DiagnosticsLog.Println("Closing driver")
	err = d.Close()
	if err != nil {
		dwgd.DiagnosticsLog.Printf("Error during driver close: %s\n", err)
	}

	dwgd.DiagnosticsLog.Println("Closing listener")
	err = l.Close()
	if err != nil {
		dwgd.DiagnosticsLog.Printf("Error during listener close: %s\n", err)
	}
}

func main() {
	if len(os.Args) >= 2 {
		switch os.Args[1] {
		case "pubkey":
			pubkey(os.Args[2:])
		}
	}

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)

	flag.Parse()

	db := *dbFlag
	if db == "" {
		db = ":memory:"
	}

	verbose := *verboseFlag
	if verbose {
		dwgd.TraceLog.SetOutput(os.Stderr)
	}

	version := *versionFlag
	if version {
		if Version != "" {
			dwgd.EventsLog.Println(Version)
		} else {
			dwgd.EventsLog.Println("(unknown)")
		}
		os.Exit(0)
	}

	ctx, cancel := context.WithCancel(context.Background())
	go runDwgd(ctx, db)

	sig := <-signalCh
	dwgd.DiagnosticsLog.Printf("Received signal: %s", sig.String())
	signal.Stop(signalCh)

	cancel()
	time.Sleep(time.Second)
}
