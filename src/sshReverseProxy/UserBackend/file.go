// SshReverseProxy - This tag line may change
//
// Copyright 2016 Dolf Schimmel, Freeaqingme.
//
// This Source Code Form is subject to the terms of the two-clause BSD license.
// For its contents, please refer to the LICENSE file.
package UserBackend

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"strings"
	"sync"

	"github.com/Freeaqingme/GoDaemonSkeleton/log"
)

var fileMapPath string

var fileMap map[string]string
var fileMapLock sync.RWMutex
var minEntries int
var Log *log.Logger

func Init(path string, entriesRequired int) error {
	fileMapPath = path
	minEntries = entriesRequired

	return LoadMap()
}

func LoadMap() error {
	contents, err := ioutil.ReadFile(fileMapPath)
	if err != nil {
		return fmt.Errorf("Could not read file %s: %s", fileMapPath, err.Error())
	}
	lines := bytes.Split(contents, []byte("\n"))

	newMap := make(map[string]string)
	for i, line := range lines {
		lineParts := bytes.Split(bytes.TrimSpace(line), []byte(" "))
		if len(lineParts[0]) == 0 {
			continue
		}

		user := string(lineParts[:1][0])
		host := string(lineParts[len(lineParts)-1:][0])

		if _, alreadyExists := newMap[user]; alreadyExists {
			Log.Noticef("User %s was redefined on line %d", user, i+1)
		}

		newMap[user] = host
	}

	if len(newMap) < minEntries {
		return fmt.Errorf("New Map only contains %d entries, which is less than the set minimum %d",
			len(newMap), minEntries)
	}

	fileMapLock.Lock()
	defer fileMapLock.Unlock()
	fileMap = newMap

	return nil
}

func GetServerForUser(user string) (host, port string) {
	fileMapLock.RLock()
	host, found := fileMap[user]
	fileMapLock.RUnlock()
	if !found {
		return "", ""
	}

	if strings.Contains(host, ":") {
		return strings.Split(host, ":")[0], strings.Split(host, ":")[1]
	}

	return host, "22"
}

func GetMapSize() int {
	fileMapLock.RLock()
	defer fileMapLock.RUnlock()

	return len(fileMap)
}

func SetLogger(logger *log.Logger) {
	Log = logger
}
