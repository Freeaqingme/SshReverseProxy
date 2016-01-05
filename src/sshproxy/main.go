package main

import (
	"fmt"
	"golang.org/x/crypto/ssh"
	//	"golang.org/x/crypto/ssh/terminal"
	"io/ioutil"
	"log"
	"net"
	"strconv"
)

/**
 * LocalSSH  - The listening side of tihs application
 * RemoteSSH - The connection with the server that contains and provides the actual data
 * Client    - The client that connects with this application
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
		defer rChannel.Close()

		go func() {
			defer func() {
				if r := recover(); r != nil {
					fmt.Println("Recovered in f", r)
				}
			}()
			for {
				select {
				case lRequest := <-lRequests:
					if string(lRequest.Type) != "subsystem" {
						fmt.Println("Ignoring unsupported request type: " + string(lRequest.Type))
						lRequest.Reply(false, nil)
						continue
					}

					fmt.Println("l type " + string(lRequest.Type))
					fmt.Println("l wantReply " + strconv.FormatBool(lRequest.WantReply))
					fmt.Println(string("l payload " + string(lRequest.Payload)))
					reply, err := rChannel.SendRequest(lRequest.Type, lRequest.WantReply, lRequest.Payload)

					if err != nil {
						fmt.Println("Error " + err.Error())
					}
					if lRequest.WantReply {
						lRequest.Reply(reply, nil)
					}
				case rRequest := <-rRequests:
					fmt.Println("type " + string(rRequest.Type))
					fmt.Println("wantReply " + strconv.FormatBool(rRequest.WantReply))
					fmt.Println(string("payload " + string(rRequest.Payload)))
					reply, err := lChannel.SendRequest(rRequest.Type, rRequest.WantReply, rRequest.Payload) // todo: Error checking?
					if err != nil {
						fmt.Println("Error " + err.Error())
					}
					if rRequest.WantReply {
						rRequest.Reply(reply, nil)
					}
				}
			}
		}()

		go func() {
			defer func() {
				if r := recover(); r != nil {
					fmt.Println("Recovered in f", r)
				}
			}()

			Pipe(rChannel, lChannel)
			fmt.Println("Done piping")
		}()

		// This somehow works. Supposedly it closes the connection(?)
		/*		go func() {
				defer lChannel.Close()
				term := terminal.NewTerminal(lChannel, "> ")
				for {
					line, err := term.ReadLine()
					if err != nil {
						break
					}
					fmt.Println(line)
				}
			}()*/
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

// chanFromConn creates a channel from a Conn object, and sends everything it
//  Read()s from the socket to the channel.
// derived from: http://www.stavros.io/posts/proxying-two-connections-go/
func chanFromConn(conn ssh.Channel) chan []byte {
	c := make(chan []byte)

	go func() {
		b := make([]byte, 1024)

		for {
			n, err := conn.Read(b)
			if n > 0 {
				res := make([]byte, n)
				// Copy the buffer so it doesn't get changed while read by the recipient.
				copy(res, b[:n])
				c <- res
			}
			if err != nil {
				c <- nil
				break
			}
		}
	}()

	return c
}

// Pipe creates a full-duplex pipe between the two sockets and transfers data from one to the other.
// derived from: http://www.stavros.io/posts/proxying-two-connections-go/
func Pipe(conn1 ssh.Channel, conn2 ssh.Channel) {
	chan1 := chanFromConn(conn1)
	chan2 := chanFromConn(conn2)

	for {
		select {
		case b1 := <-chan1:
			fmt.Println(fmt.Sprintf("Len b1: %d", len(b1)))
			if b1 == nil {
				return
			} else {
				conn2.Write(b1)
			}
		case b2 := <-chan2:
			fmt.Println(fmt.Sprintf("Len b2: %d", len(b2)))
			if b2 == nil {
				return
			} else {
				conn1.Write(b2)
			}
		}
	}
}
