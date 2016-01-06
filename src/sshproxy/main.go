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
 * L left/local  -   SshProxy   -    R Right/Remote
 *
 * LocalSSH  - The listening side of tihs application
 * RemoteSSH - The connection with the server that contains and provides the actual data
 * Client    - The client that connects with this application
 *
 */

func main() {
	listener, err := net.Listen("tcp", "0.0.0.0:2022")
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

	var rClient *ssh.Client
	lConfig := getLocalSshConfig(&rClient)
	lConn, lChans, lReqs, err := ssh.NewServerConn(lnConn, lConfig)
	if err != nil {
		panic("Handshake failed " + err.Error())
	}
	defer lConn.Close()
	defer rClient.Close()

	go ssh.DiscardRequests(lReqs)

	for newChannel := range lChans {
		handleChannel(newChannel, rClient)
	}
}

func handleChannel(newChannel ssh.NewChannel, rClient *ssh.Client) {
	if newChannel.ChannelType() != "session" {
		newChannel.Reject(ssh.UnknownChannelType, "unknown channel type: "+newChannel.ChannelType())
		return
	}
	lChannel, lRequests, err := newChannel.Accept()
	if err != nil {
		panic("could not accept channel.")
	}

	rChannel, rRequests, err := rClient.OpenChannel(newChannel.ChannelType(), nil)
	if err != nil {
		panic("Failed to create session: " + err.Error())
	}

	go pipeRequests(lChannel, rChannel, lRequests, rRequests)

	time.Sleep(50 * time.Millisecond)
	pipe := func(dst io.Writer, src io.Reader) {
		bytes, err := io.Copy(dst, src)
		if err != nil {
			fmt.Println(err.Error())
		}

		fmt.Println("Bytes exchanged: ", bytes)
		rChannel.CloseWrite()
		lChannel.CloseWrite()
	}

	go pipe(rChannel, lChannel)
	go pipe(lChannel, rChannel)
}

func getLocalSshConfig(rClient **ssh.Client) *ssh.ServerConfig {
	config := &ssh.ServerConfig{
		ServerVersion: "SSH-2.0-SshReverseProxy",
		PasswordCallback: func(c ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
			if c.User() == "dolf" {
				var err error
				*rClient, err = getRemoteSshClient("localhost", "2222", c.User(), string(pass))
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

func getRemoteSshClient(host, portStr, user, password string) (*ssh.Client, error) {
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

func pipeRequests(lChannel, rChannel ssh.Channel, lRequests, rRequests <-chan *ssh.Request) {
	defer func() {
		return
		if r := recover(); r != nil {
			fmt.Println("Recovered in f", r)
		}
	}()
	defer rChannel.Close()
	defer lChannel.Close()

	for {
		select {
		case lRequest, ok := <-lRequests:
			if !ok {
				return
			}
			if err := forwardRequest(lRequest, rChannel); err != nil {
				fmt.Println("Error: " + err.Error())
				continue
			}
		case rRequest, ok := <-rRequests:
			if !ok {
				return
			}
			if err := forwardRequest(rRequest, lChannel); err != nil {
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
