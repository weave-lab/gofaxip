// This file is part of the GOfax.IP project - https://github.com/gonicus/gofaxip
// Copyright (C) 2014 GONICUS GmbH, Germany - http://www.gonicus.de
//
// This program is free software; you can redistribute it and/or
// modify it under the terms of the GNU General Public License
// as published by the Free Software Foundation; version 2
// of the License.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program; if not, write to the Free Software
// Foundation, Inc., 51 Franklin Street, Fifth Floor, Boston, MA  02110-1301, USA.

package main

import (
	"flag"
	"fmt"
	"gofaxlib"
	"gofaxlib/logger"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const (
	DEFAULT_CONFIGFILE = "/etc/gofax.conf"
	PRODUCT_NAME       = "GOfax.IP"
	MODEM_PREFIX       = "freeswitch"
)

var (
	config_file  = flag.String("c", DEFAULT_CONFIGFILE, "GOfax configuration file")
	show_version = flag.Bool("version", false, "Show version information")

	usage = fmt.Sprintf("Usage: %s -version | [-c configfile]", os.Args[0])

	// Version can be set at build time using:
	//    -ldflags "-X main.version 0.42"
	version string

	devmanager *manager
)

func init() {
	if version == "" {
		version = "development version"
	}

	flag.Usage = func() {
		log.Printf("%s %s\n%s\n", PRODUCT_NAME, version, usage)
		flag.PrintDefaults()
	}
}

func main() {
	flag.Parse()

	if *show_version {
		fmt.Println(version)
		os.Exit(1)
	}

	logger.Logger.Printf("%v gofaxd %v starting", PRODUCT_NAME, version)
	gofaxlib.LoadConfig(*config_file)

	if err := os.Chdir(gofaxlib.Config.Hylafax.Spooldir); err != nil {
		logger.Logger.Print(err)
		log.Fatal(err)
	}

	// Shut down receiving lines when killed
	sigchan := make(chan os.Signal, 1)
	signal.Notify(sigchan, syscall.SIGTERM, syscall.SIGINT)

	// Start modem device manager
	var err error
	devmanager, err = NewManager(MODEM_PREFIX, gofaxlib.Config.Hylafax.Modems)
	if err != nil {
		logger.Logger.Fatal(err)
	}

	// Start event socket server to handle incoming calls
	server := NewEventSocketServer()
	server.Start()

	// Block until something happens
	select {
	case err := <-server.Errors():
		logger.Logger.Fatal(err)
	case sig := <-sigchan:
		logger.Logger.Print("Received ", sig, ", killing all channels")
		server.Kill()
		devmanager.SetAllDown()
		time.Sleep(3 * time.Second)
		logger.Logger.Print("Terminating")
		os.Exit(1)
	}

}
