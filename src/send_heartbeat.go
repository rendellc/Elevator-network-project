package main

import (
	"./msgs"
	"./network/peers"
	"flag"
	"fmt"
	"time"
)

const server_ip = "129.241.187.38"

func main() {
	sourceIDPtr := flag.String("SourceID", "0", "sender identifier")
	portPtr := flag.Int("Port", 20010, "sender identifier")

	flag.Parse()
	fmt.Println(*sourceIDPtr)
	fmt.Println(*portPtr)

	peerTxEnable := make(chan bool)
	peerStatusSendCh := make(chan msgs.Heartbeat)
	go peers.Transmitter(*portPtr, peerTxEnable, peerStatusSendCh)

	heartbeat := msgs.Heartbeat{SourceID: *sourceIDPtr,
		ElevatorState:  msgs.StopDown,
		AcceptedOrders: []msgs.Order{},
		TakenOrders:    []msgs.Order{}}

	peerTxEnable <- true
	peerStatusSendCh <- heartbeat
	time.Sleep(1 * time.Second)

	/*
		err := network.SendBytes([]byte("Message sending test"), server_ip+":20010")
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
		conn.SetReadDeadline(time.Now().Add(1 * time.Second))
		n, _, err := conn.ReadFromUDP(buffer)
		if err != nil {
			fmt.Println(err)
		}

		fmt.Println(string(buffer[:n]))

		msg := OrderPlacedMsg{SourceID: 0, MsgType: "testing type", Order: Order{OrderID: 1234, Floor: 1, Direction: -1}, Priority: 1}

		data, err := json.MarshalIndent(msg, "", " ")
		if err != nil {
			fmt.Printf("JSON failed")
		}
		fmt.Printf("%s\n", data)

		var un_msg OrderPlacedMsg
		json.Unmarshal(data, &un_msg)

		fmt.Println(un_msg)
	*/
}
