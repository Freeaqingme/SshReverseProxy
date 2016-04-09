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
	"golang.org/x/crypto/ssh"
	"io"
	"io/ioutil"
	"net"
	"strconv"
	"time"
)

/**
 * Used Terminology
 *   client (c)    -     proxyServer (ps)     -     server (s)
 */

func main() {
	listener, err := net.Listen("tcp", "0.0.0.0:2022")
	fmt.Println("Now listening on :2022")

	if err != nil {
		panic("failed to listen for connection")
	}
	for {
		lnConn, err := listener.Accept()
		if err != nil {
			panic("failed to accept incoming connection")
		}
		go handleLocalSshConn(lnConn)
	}
}

func handleLocalSshConn(lnConn net.Conn) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Println("Recovered from panic: ", r)
		}
	}()

	fmt.Println("Received connection")

	var sClient *ssh.Client
	psConfig := getProxyServerSshConfig(&sClient)
	psConn, psChans, psReqs, err := ssh.NewServerConn(lnConn, psConfig)
	if err != nil {
		panic("Handshake failed " + err.Error())
	}
	defer psConn.Close()
	defer sClient.Close()

	go ssh.DiscardRequests(psReqs)

	for newChannel := range psChans {
		handleChannel(newChannel, sClient)
	}
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
	bytes, err := io.Copy(dst, src)
	if err != nil {
		fmt.Println(err.Error())
	}

	fmt.Println("Bytes exchanged: ", bytes)
	dst.CloseWrite()
}

func getProxyServerSshConfig(rClient **ssh.Client) *ssh.ServerConfig {
	config := &ssh.ServerConfig{
		ServerVersion: "SSH-2.0-SshReverseProxy",
		PasswordCallback: func(c ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
			if c.User() == "dolf" {
				var err error
				*rClient, err = getServerSshClient("localhost", "2222", c.User(), string(pass))
				if err != nil {
					return nil, fmt.Errorf("password rejected for %q: %s", c.User(), err)
				}
				return nil, nil
			}
			return nil, fmt.Errorf("Unknown user %q", c.User())
		},
	}

	privateBytes, err := ioutil.ReadFile("id_rsa")
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
