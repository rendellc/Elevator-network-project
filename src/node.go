package main

import (
	//"encoding/json"
	"./msgs"
	"./network/bcast"
	"./network/peers"
	"fmt"
)

const server_ip = "129.241.187.38"

func main() {
	//msg1 := OrderPlacedMsg{SourceID: 0, MsgType: "testing type", Order: Order{OrderID: 1234, Floor: 1, Direction: -1}, Priority: 1}
	//msg2 := OrderPlacedAck{SourceID: 1, OrderID: 1234, MsgType: "ackack", Score: 666}

	orderPlacedSendCh := make(chan msgs.OrderPlacedMsg)
	orderPlacedAckSendCh := make(chan msgs.OrderPlacedAck)
	takeOrderAckSendCh := make(chan msgs.TakeOrderAck)
	takeOrderSendCh := make(chan msgs.TakeOrderMsg)
	go bcast.Transmitter(20010, orderPlacedSendCh, orderPlacedAckSendCh, takeOrderAckSendCh, takeOrderSendCh)

	orderPlacedRecvCh := make(chan msgs.OrderPlacedMsg)
	orderPlacedAckRecvCh := make(chan msgs.OrderPlacedAck)
	takeOrderAckRecvCh := make(chan msgs.TakeOrderAck)
	takeOrderRecvCh := make(chan msgs.TakeOrderMsg)
	go bcast.Receiver(20010, orderPlacedRecvCh, orderPlacedAckRecvCh, takeOrderAckRecvCh, takeOrderRecvCh)

	peerTxEnable := make(chan bool)
	peerStatusSendCh := make(chan msgs.Heartbeat)
	go peers.Transmitter(20010, peerTxEnable, peerStatusSendCh)

	//peerStatusCh := make(chan msgs.Heartbeat)
	peerUpdateCh := make(chan peers.PeerUpdate, 1)
	go peers.Receiver(20010, peerUpdateCh)

	heartbeat := msgs.Heartbeat{SourceID: "1",
		ElevatorState:  msgs.StopDown,
		AcceptedOrders: []msgs.Order{},
		TakenOrders:    []msgs.Order{msgs.Order{OrderID: 0, Floor: 3, Direction: msgs.Up}}}

	peerStatusSendCh <- heartbeat
	fmt.Println("Listening")

	for {
		select {
		case msgRecv1 := <-orderPlacedRecvCh:
			fmt.Println(msgRecv1)
		case msgRecv2 := <-orderPlacedAckRecvCh:
			fmt.Println(msgRecv2)
		case peerUpdate := <-peerUpdateCh:
			fmt.Println(peerUpdate)
		}
	}

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
