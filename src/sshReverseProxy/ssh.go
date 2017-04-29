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
	"strconv"
	"time"

	"golang.org/x/crypto/ssh"

	backend "sshReverseProxy/UserBackend"
)

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
	holdOff := func() {
		duration, _ := time.ParseDuration(Config.Ssh_Reverse_Proxy.Auth_Error_Delay)
		time.Sleep(duration)
	}

	callback := func(c ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
		host, port := backend.GetServerForUser(c.User())
		if host != "" {
			var err error
			*rClient, err = getServerSshClient(host, port, c.User(), string(pass))
			if err != nil {
				Log.Info(fmt.Sprintf("Could not authorize %q on %s: %s",
					c.User(), c.RemoteAddr().String(), err))
				holdOff()
				return nil, fmt.Errorf("Could not authorize %q on %s: %s",
					c.User(), c.RemoteAddr().String(), err)
			}
			return nil, nil
		}
		Log.Info(fmt.Sprintf("Unknown user %q on %s", c.User(), c.RemoteAddr().String()))
		holdOff()
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
		User:            user,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
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
