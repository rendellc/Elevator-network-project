package main

import (
	"fmt"
	"net"
	"network"
)

func main() {
	err := network.SendBytes([]byte("This is my msg"), "129.241.187.38:20010")
	if err != nil {
		fmt.Println(err)
	}

	addr, err := net.ResolveUDPAddr("udp", ":20010")
	if err != nil {
		fmt.Println(err)
		return
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer conn.Close()

	buffer := make([]byte, 1024)
	n, _, _ := conn.ReadFromUDP(buffer)

	fmt.Println(string(buffer[:n]))

}
