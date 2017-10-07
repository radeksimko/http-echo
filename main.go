package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"cloud.google.com/go/compute/metadata"
	consulapi "github.com/hashicorp/consul/api"
	nomadapi "github.com/hashicorp/nomad/api"
)

var pugs = map[string]string{
	"a": "https://storage.googleapis.com/radek-devfest-2017/red-pug.jpg",
	"b": "https://storage.googleapis.com/radek-devfest-2017/blue-pug.jpg",
	"c": "https://storage.googleapis.com/radek-devfest-2017/green-pug.jpg",
}

var (
	listenFlag  = flag.String("listen", ":5678", "address and port to listen")
	textFlag    = flag.String("text", "", "text to put on the webpage")
	versionFlag = flag.Bool("version", false, "display version information")

	// stdoutW and stderrW are for overriding in test.
	stdoutW = os.Stdout
	stderrW = os.Stderr
)

func main() {
	flag.Parse()

	// Asking for the version?
	if *versionFlag {
		fmt.Fprintln(stderrW, humanVersion)
		os.Exit(0)
	}

	// Validation
	if *textFlag == "" {
		fmt.Fprintln(stderrW, "Missing -text option!")
		os.Exit(127)
	}

	args := flag.Args()
	if len(args) > 0 {
		fmt.Fprintln(stderrW, "Too many arguments!")
		os.Exit(127)
	}

	// Flag gets printed as a page
	mux := http.NewServeMux()
	mux.HandleFunc("/", httpLog(stdoutW, withAppHeaders(httpEcho(*textFlag))))

	// Health endpoint
	mux.HandleFunc("/health", withAppHeaders(httpHealth()))

	server, err := NewServer(*listenFlag, mux)
	if err != nil {
		log.Printf("[ERR] Error starting server: %s", err)
		os.Exit(127)
	}

	go server.Start()
	log.Printf("Server is listening on %s\n", *listenFlag)

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh)

	for {
		select {
		case s := <-signalCh:
			switch s {
			case syscall.SIGINT:
				log.Printf("[INFO] Received interrupt")
				server.Stop()
				os.Exit(2)
			default:
				log.Printf("[ERR] Unknown signal %v", s)
			}
		}
	}
}

func httpEcho(v string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")

		var zoneLetter string
		zone, err := getZone()
		if err != nil {
			zone = "unknown"
			zoneLetter = ""
		} else {
			zoneParts := strings.Split(zone, "-")
			zoneLetter = zoneParts[len(zoneParts)-1]
		}

		fmt.Fprintln(w, fmt.Sprintf(`<center><img src="%s"></center>`, pugs[zoneLetter]))

		// Consul
		var ip string
		consul, err := getConsulMemberInfo()
		if err != nil {
			ip = fmt.Sprintf("unknown / %s", err)
		} else {
			ip = consul["Addr"].(string)
		}

		// Nomad
		nomad, err := getNomadInfo()
		if err != nil {
			fmt.Fprintf(w, "<center>Served from unknown node (%s)</center>", err)
			return
		}

		nodeId := nomad.Stats["client"]["node_id"]
		cfg := nomad.Config

		dc := cfg["Datacenter"].(string)

		fmt.Fprintf(w, "<br><center>Served from <strong>%s</strong> IP: %s (Zone: %s, DC: %s, region: %s)</center>",
			nodeId, ip, zone, dc, cfg["Region"].(string))
	}
}

func getNomadInfo() (*nomadapi.AgentSelf, error) {
	c, err := nomadapi.NewClient(nomadapi.DefaultConfig())
	if err != nil {
		return nil, err
	}
	self, err := c.Agent().Self()
	if err != nil {
		return nil, err
	}
	return self, nil
}

func getConsulMemberInfo() (map[string]interface{}, error) {
	c, err := consulapi.NewClient(consulapi.DefaultConfig())
	if err != nil {
		return nil, err
	}
	self, err := c.Agent().Self()
	if err != nil {
		return nil, err
	}
	return self["Member"], nil
}

func getZone() (string, error) {
	return metadata.Zone()
}

func httpHealth() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, `{"status":"ok"}`)
	}
}
