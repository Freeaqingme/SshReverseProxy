// SshReverseProxy - This tag line may change
//
// Copyright 2016 Dolf Schimmel, Freeaqingme.
//
// This Source Code Form is subject to the terms of the two-clause BSD license.
// For its contents, please refer to the LICENSE file.
//
package main

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	skel "github.com/Freeaqingme/GoDaemonSkeleton"
	"github.com/Freeaqingme/GoDaemonSkeleton/log"

	backend "sshReverseProxy/UserBackend"
)

/**
 * Used Terminology
 *   client (c)    -     proxyServer (ps)     -     server (s)
 */

func init() {
	handover := daemonStart

	skel.AppRegister(&skel.App{
		Name:     "daemon",
		Handover: &handover,
	})
}

func daemonStart() {
	backend.SetLogger(Log)
	_, err := time.ParseDuration(Config.Ssh_Reverse_Proxy.Auth_Error_Delay)
	if err != nil {
		Log.Fatal("Cannot parse 'auth-error-delay': ", err.Error())
	}

	if !Config.File_User_Backend.Enabled {
		Log.Fatal("No User Backend enabled")
	}

	err = backend.Init(Config.File_User_Backend.Path, Config.File_User_Backend.Min_Entries)
	if err != nil {
		Log.Fatal("Could not load user map: ", err.Error())
	}
	Log.Info(fmt.Sprintf(
		"Successfully loaded user map. It now contains %d entries", backend.GetMapSize(),
	))
	go reloadMapsOnSigHup()

	listener, err := net.Listen("tcp", Config.Ssh_Reverse_Proxy.Listen)

	if err != nil {
		Log.Fatal("Failed to listen on", Config.Ssh_Reverse_Proxy.Listen)
	}
	Log.Info("Now listening on", Config.Ssh_Reverse_Proxy.Listen)

	for {
		lnConn, err := listener.Accept()
		if err != nil {
			Log.Fatal("Failed to accept incoming connection")
		}
		go handleLocalSshConn(lnConn)
	}
}

func reloadMapsOnSigHup() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGHUP)

	for _ = range c {
		Log.Info("Received SIGHUP")
		log.Reopen()

		if err := backend.LoadMap(); err != nil {
			Log.Error("Could not reload user map:", err.Error())
		} else {
			Log.Info(fmt.Sprintf(
				"Successfully reloaded user map. It now contains %d entries", backend.GetMapSize(),
			))
		}
	}
}
