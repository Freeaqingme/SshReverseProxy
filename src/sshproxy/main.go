package main

import (
	"fmt"
	"golang.org/x/crypto/ssh"
	"io"
	"io/ioutil"
	"log"
	"net"
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
	config := getLocalSshConfig()
	listener, err := net.Listen("tcp", "0.0.0.0:2022")
	if err != nil {
		panic("failed to listen for connection")
	}
	for {
		lnConn, err := listener.Accept()
		if err != nil {
			panic("failed to accept incoming connection")
		}
		go handleLocalSshConn(lnConn, config)
	}
}

func handleLocalSshConn(lnConn net.Conn, lConfig *ssh.ServerConfig) {
	defer func() {
		return
		if r := recover(); r != nil {
			fmt.Println("Recovered in f", r)
		}
	}()
	lConn, lChans, lReqs, err := ssh.NewServerConn(lnConn, lConfig)
	if err != nil {
		panic("failed to handshake")
	}
	go ssh.DiscardRequests(lReqs)

	rClient := getRemoteSshClient(lConn.User())
	defer rClient.Close()

	for newChannel := range lChans {
		if newChannel.ChannelType() != "session" {
			newChannel.Reject(ssh.UnknownChannelType, "unknown channel type: "+newChannel.ChannelType())
			continue
		}
		lChannel, lRequests, err := newChannel.Accept()
		if err != nil {
			panic("could not accept channel.")
		}

		rChannel, rRequests, err := rClient.OpenChannel(newChannel.ChannelType(), nil)
		if err != nil {
			panic("Failed to create session: " + err.Error())
		}

		go func(lChannel, rChannel ssh.Channel) {
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
					if err := pipeRequests(lRequest, rChannel); err != nil {
						fmt.Println("Error: " + err.Error())
						continue
					}
				case rRequest, ok := <-rRequests:
					if !ok {
						return
					}
					if err := pipeRequests(rRequest, lChannel); err != nil {
						fmt.Println("Error: " + err.Error())
						continue
					}
				}
			}
		}(lChannel, rChannel)

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
}

func getLocalSshConfig() *ssh.ServerConfig {
	config := &ssh.ServerConfig{
		PasswordCallback: func(c ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
			// Should use constant-time compare (or better, salt+hash) in
			// a production setting.
			if c.User() == "dolf" && string(pass) == "dolf" {
				return nil, nil
			}
			return nil, fmt.Errorf("password rejected for %q", c.User())
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

func getRemoteSshClient(user string) *ssh.Client {
	config := &ssh.ClientConfig{
		User: "dolf",
		Auth: []ssh.AuthMethod{
			ssh.Password("dolf"),
		},
	}
	// Dial your ssh server.
	conn, err := ssh.Dial("tcp", "localhost:2222", config)
	if err != nil {
		log.Fatalf("unable to connect: %s", err)
	}

	return conn
}

func pipeRequests(req *ssh.Request, channel ssh.Channel) error {
	if req == nil {
		return fmt.Errorf("Req == nil? :/")
	}
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
