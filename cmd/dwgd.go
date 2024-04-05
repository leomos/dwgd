package main

import (
	"flag"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/leomos/dwgd"
)

var Version string

var cfg = dwgd.NewConfig()

func init() {
	flag.StringVar(&cfg.Db, "d", cfg.Db, "dwgd db path")
	flag.BoolVar(&cfg.Verbose, "v", cfg.Verbose, "verbose mode")
	flag.BoolVar(&cfg.Rootless, "r", cfg.Rootless, "run in rootless compatibility mode")
}

var versionFlag = flag.Bool("version", false, "print the version")

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

	privkey := dwgd.GeneratePrivateKey([]byte(seed), net.ParseIP(ip))

	dwgd.EventsLog.Printf("%s\n", privkey.PublicKey().String())
	os.Exit(0)
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

	if cfg.Db == "" {
		cfg.Db = ":memory:"
	}

	if cfg.Verbose {
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

	dwgd.TraceLog.Printf("Running with the following configuration: %+v\n", cfg)
	plugin, err := dwgd.NewDwgd(cfg)
	if err != nil {
		dwgd.DiagnosticsLog.Fatalf("Couldn't initialize plugin: %s\n", err)
	}
	err = plugin.Start()
	if err != nil {
		dwgd.DiagnosticsLog.Fatalf("Couldn't start plugin: %s\n", err)
	}

	sig := <-signalCh
	dwgd.DiagnosticsLog.Printf("Received signal: %s", sig.String())
	signal.Stop(signalCh)

	err = plugin.Stop()
	if err != nil {
		dwgd.DiagnosticsLog.Printf("Couldn't stop plugin: %s\n", err)
	}
}
