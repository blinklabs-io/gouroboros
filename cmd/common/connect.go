package common

import (
	"crypto/tls"
	"fmt"
	"net"
	"os"
)

func CreateClientConnection(f *GlobalFlags) net.Conn {
	var err error
	var conn net.Conn
	var dialProto string
	var dialAddress string
	if f.Socket != "" {
		dialProto = "unix"
		dialAddress = f.Socket
	} else if f.Address != "" {
		dialProto = "tcp"
		dialAddress = f.Address
	} else {
		fmt.Printf("You must specify one of -socket or -address\n\n")
		f.Flagset.PrintDefaults()
		os.Exit(1)
	}
	if f.UseTls {
		conn, err = tls.Dial(dialProto, dialAddress, nil)
	} else {
		conn, err = net.Dial(dialProto, dialAddress)
	}
	if err != nil {
		fmt.Printf("Connection failed: %s\n", err)
		os.Exit(1)
	}
	return conn
}
