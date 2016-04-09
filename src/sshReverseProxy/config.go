// SshReverseProxy - This tag line may change
//
// Copyright 2016 Dolf Schimmel, Freeaqingme.
//
// This Source Code Form is subject to the terms of the two-clause BSD license.
// For its contents, please refer to the LICENSE file.
//
package main

import (
	"github.com/scalingdata/gcfg"
)

type config struct {
	Ssh_Reverse_Proxy struct {
		Exit_On_Panic bool
	}
	File_User_Backend struct {
		Enabled bool
		Path    string
	}
}

func LoadConfig(cfgFile string, cfg *config) {
	err := gcfg.ReadFileInto(cfg, cfgFile)

	if err != nil {
		Log.Fatal("Couldnt read config file: " + err.Error())
	}
}

func DefaultConfig(cfg *config) {
}
