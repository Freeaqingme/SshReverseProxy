// SshReverseProxy - This tag line may change
//
// Copyright 2016 Dolf Schimmel, Freeaqingme.
//
// This Source Code Form is subject to the terms of the two-clause BSD license.
// For its contents, please refer to the LICENSE file.
package main

import (
	"flag"
	"fmt"
	"log/syslog"
	"os"
	"runtime"

	skel "github.com/Freeaqingme/GoDaemonSkeleton"
	"github.com/Freeaqingme/GoDaemonSkeleton/log"
)

var (
	Log    *log.Logger
	Config = *new(config)
)

// Set by linker flags
var (
	buildTag          string
	buildTime         string
	defaultConfigFile = "/etc/sshReverseProxy/sshreverseproxy.conf"
)

func main() {
	app, args := skel.GetApp()

	if app.Name == "version" {
		// We don't want to require config stuff for merely displaying the version
		(*app.Handover)()
		return
	}

	configFile := flag.String("config", defaultConfigFile, "Path to Config File")
	logLevel := flag.String("loglevel", "DEBUG",
		"Log Level. One of: CRITICAL, ERROR, WARNING, NOTICE, INFO, DEBUG)")
	flag.Parse()

	Log = log.Open("sshReverseProxy", *logLevel, syslog.LOG_LOCAL4)

	DefaultConfig(&Config)
	if *configFile != "" {
		LoadConfig(*configFile, &Config)
	}

	os.Args = append([]string{os.Args[0]}, args...)
	(*app.Handover)()
}

func init() {
	handover := func() {
		fmt.Printf(
			"SshReverseProxy - This tag line may change - %s\n\n"+
				"%s\nCopyright (c) 2016, Dolf Schimmel\n"+
				"License BSD-2 clause <%s>\n\n"+
				"Time of Build: %s\n"+
				"Go Version: %s %s/%s\n\n",
			buildTag,
			"https://github.com/Freeaqingme/SshReverseProxy",
			"https://git.io/vVSYI",
			buildTime,
			runtime.Version(),
			runtime.GOOS,
			runtime.GOARCH,
		)
		os.Exit(0)
	}

	skel.AppRegister(&skel.App{
		Name:     "version",
		Handover: &handover,
	})
}
