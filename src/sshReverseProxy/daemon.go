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
	"io"
	"io/ioutil"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	skel "github.com/Freeaqingme/GoDaemonSkeleton"
	"golang.org/x/crypto/ssh"
	backend "sshReverseProxy/UserBackend"
	"github.com/Freeaqingme/GoDaemonSkeleton/log"
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
	if !Config.File_User_Backend.Enabled {
		Log.Fatal("No User Backend enabled")
	}

	err := backend.Init(Config.File_User_Backend.Path, Config.File_User_Backend.Min_Entries)
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
		log.Reopen("")

		if err := backend.LoadMap(); err != nil {
			Log.Error("Could not reload user map:", err.Error())
		} else {
			Log.Info(fmt.Sprintf(
				"Successfully reloaded user map. It now contains %d entries", backend.GetMapSize(),
			))
		}
	}
}

func handleLocalSshConn(lnConn net.Conn) {
	defer func() {
		if Config.Ssh_Reverse_Proxy.Exit_On_Panic {
			return
		}
		if r := recover(); r != nil {
			Log.Error("Recovered from panic in connection from "+
				lnConn.RemoteAddr().String()+":", r)
		}
	}()

	Log.Info("Received connection from", lnConn.RemoteAddr())

	var sClient *ssh.Client
	psConfig := getProxyServerSshConfig(&sClient)
	psConn, psChans, psReqs, err := ssh.NewServerConn(lnConn, psConfig)
	if err != nil {
		Log.Info("Could not establish connection with " + lnConn.RemoteAddr().String() + ": " + err.Error())
		return
	}
	defer psConn.Close()
	defer sClient.Close()

	go ssh.DiscardRequests(psReqs)

	for newChannel := range psChans {
		handleChannel(newChannel, sClient)
	}

	Log.Info("Lost connection with", lnConn.RemoteAddr())
}

func handleChannel(newChannel ssh.NewChannel, rClient *ssh.Client) {
	if newChannel.ChannelType() != "session" {
		newChannel.Reject(ssh.UnknownChannelType, "unknown channel type: "+newChannel.ChannelType())
		return
	}
	psChannel, psRequests, err := newChannel.Accept()
	if err != nil {
		panic("could not accept channel.")
	}

	sChannel, sRequests, err := rClient.OpenChannel(newChannel.ChannelType(), nil)
	if err != nil {
		panic("Failed to create session: " + err.Error())
	}

	go pipeRequests(psChannel, sChannel, psRequests, sRequests)
	time.Sleep(50 * time.Millisecond)
	go pipe(sChannel, psChannel)
	go pipe(psChannel, sChannel)
}

func pipe(dst, src ssh.Channel) {
	_, err := io.Copy(dst, src)
	if err != nil {
		fmt.Println(err.Error())
	}

	dst.CloseWrite()
}

func getProxyServerSshConfig(rClient **ssh.Client) *ssh.ServerConfig {
	callback := func(c ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
		host, port := backend.GetServerForUser(c.User())
		if host != "" {
			var err error
			*rClient, err = getServerSshClient(host, port, c.User(), string(pass))
			if err != nil {
				Log.Info(fmt.Sprintf("Could not authorize %q on %s: %s",
					c.User(), c.RemoteAddr().String(), err))
				return nil, fmt.Errorf("Could not authorize %q on %s: %s",
					c.User(), c.RemoteAddr().String(), err)
			}
			return nil, nil
		}
		Log.Info(fmt.Sprintf("Unknown user %q on %s", c.User(), c.RemoteAddr().String()))
		return nil, fmt.Errorf("Unknown user %q on %s", c.User(), c.RemoteAddr().String())
	}

	config := &ssh.ServerConfig{
		ServerVersion:    "SSH-2.0-SshReverseProxy",
		PasswordCallback: callback,
	}

	privateBytes, err := ioutil.ReadFile(Config.Ssh_Reverse_Proxy.Key_File)
	if err != nil {
		panic("Failed to load private key")
	}

	private, err := ssh.ParsePrivateKey(privateBytes)
	if err != nil {
		panic("Failed to parse private key")
	}

	config.AddHostKey(private)
	return config
}

func getServerSshClient(host, portStr, user, password string) (*ssh.Client, error) {
	config := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.Password(password),
		},
	}
	port, err := strconv.ParseInt(portStr, 10, 64)
	if err != nil {
		panic("Port must be an integer")
	}

	conn, err := ssh.Dial("tcp", fmt.Sprintf("%s:%d", host, port), config)
	if err != nil {
		return nil, fmt.Errorf("unable to connect: " + err.Error())
	}

	return conn, nil
}

func pipeRequests(psChannel, sChannel ssh.Channel, psRequests, sRequests <-chan *ssh.Request) {
	defer func() {
		return
		if r := recover(); r != nil {
			fmt.Println("Recovered in f", r)
		}
	}()
	defer sChannel.Close()
	defer psChannel.Close()

	for {
		select {
		case lRequest, ok := <-psRequests:
			if !ok {
				return
			}
			if err := forwardRequest(lRequest, sChannel); err != nil {
				fmt.Println("Error: " + err.Error())
				continue
			}
		case rRequest, ok := <-sRequests:
			if !ok {
				return
			}
			if err := forwardRequest(rRequest, psChannel); err != nil {
				fmt.Println("Error: " + err.Error())
				continue
			}
		}
	}
}

func forwardRequest(req *ssh.Request, channel ssh.Channel) error {
	if string(req.Type) != "subsystem" && string(req.Type) != "exit-status" {
		req.Reply(false, nil)

		if req.Type == "env" {
			return nil
		}

		return fmt.Errorf("Ignoring unsupported request type: %s", string(req.Type))
	}
	reply, err := channel.SendRequest(req.Type, req.WantReply, req.Payload)
	if err != nil {
		return err
	}
	if req.WantReply {
		req.Reply(reply, nil)
	}

	return nil
}
