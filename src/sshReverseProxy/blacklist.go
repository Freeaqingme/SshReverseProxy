// SshReverseProxy - This tag line may change
//
// Copyright 2016 Dolf Schimmel, Freeaqingme.
//
// This Source Code Form is subject to the terms of the two-clause BSD license.
// For its contents, please refer to the LICENSE file.
//
package main

import (
	"bufio"
	"bytes"
	"io"
	"net"
	"os"
	"sync"

	"github.com/fsnotify/fsnotify"
	"time"
)

var Blacklist *blacklist
var blacklistResponse = []byte("HTTP/1.0 400 Bad request" +
	"Cache-Control: no-cache\n" +
	"Connection: close\n" +
	"Content-Type: text/html\n" +
	"Server: Apache/1.3.3.7 (Unix)\n" +
	"\n" +
	"<html><body><h1>400 Bad request</h1>\n" +
	"Your browser sent an invalid request.\n" +
	"</body></html>\n")

type blacklist struct {
	sync.RWMutex
	list map[string]struct{}
}

func startBlacklist(filename string) error {

	Blacklist = &blacklist{
		sync.RWMutex{},
		make(map[string]struct{}),
	}

	if err := updateBlacklistFile(filename); err != nil {
		return err
	}
	if err := watchBlacklistUpdates(filename); err != nil {
		return err
	}

	return nil
}

func watchBlacklistUpdates(filename string) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	go func() {
		for {
			select {
			case event := <-watcher.Events:
				if event.Op&fsnotify.Write == fsnotify.Write {
					if err := updateBlacklistFile(filename); err != nil {
						Log.Error("Could not update blacklist file", err.Error())
					}
				}
			case err := <-watcher.Errors:
				Log.Error(err.Error())
			}
		}
	}()

	return watcher.Add(filename)
}

func updateBlacklistFile(filename string) error {
	Log.Info("Updating Black list...")
	list, err := parseBlacklistFile(filename)
	if err != nil {
		return err
	}

	Blacklist.Lock()
	defer Blacklist.Unlock()

	Blacklist.list = list
	Log.Infof("Black list updated. It now contains %d entries", len(list))

	return nil
}

func parseBlacklistFile(filename string) (map[string]struct{}, error) {
	newList := make(map[string]struct{}, 0)

	file, err := os.OpenFile(filename, os.O_RDONLY, 0)
	if err != nil {
		return newList, err
	}

	defer file.Close()

	r := bufio.NewReader(file)
	for {
		line, err := r.ReadBytes('\n')
		if err != nil && err == io.EOF {
			break
		} else if err != nil {
			return newList, err
		}

		pos := bytes.Index(line, []byte("#"))
		if pos != -1 {
			line = line[0:pos]
		}

		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}

		ip := net.ParseIP(string(line))
		if ip == nil {
			Log.Notice("Could not parse line from blacklist: ", string(line))
			continue
		}

		newList[ip.String()] = struct{}{}
	}

	return newList, nil
}

func isIpBlacklisted(addr net.Addr) bool {
	if Blacklist == nil {
		return false
	}

	Blacklist.RLock()
	defer Blacklist.RUnlock()

	_, exists := Blacklist.list[addr.(*net.TCPAddr).IP.String()]
	return exists
}

func handleBlacklistedConn(conn net.Conn) {
	defer func() {
		if Config.Ssh_Reverse_Proxy.Exit_On_Panic {
			return
		}
		if r := recover(); r != nil {
			Log.Error("Recovered from panic in connection from "+
				conn.RemoteAddr().String()+":", r)
		}
	}()

	Log.Infof(
		"Received connection from blacklisted ip %s, delaying and dropping...",
		conn.RemoteAddr(),
	)

	// Typical timeout at 10 seconds?
	timer := time.NewTimer(9500 * time.Millisecond)
	go func() {
		<-timer.C
		conn.Write(blacklistResponse)
		conn.Close()
	}()

	buf := make([]byte, 1)
	for {
		_, err := conn.Read(buf)
		if err != nil {
			conn.Close()
			return
		}

		// Starve client of resources
		time.Sleep(10 * time.Millisecond)
	}
}
