package main

import (
	"fmt"
	//"encoding/json"
	//"./network/bcast"
	"./msgtype"
	"./network/peers"
)

const server_ip = "129.241.187.38"

func main() {
	//msg1 := OrderPlacedMsg{SourceID: 0, MsgType: "testing type", Order: Order{OrderID: 1234, Floor: 1, Direction: -1}, Priority: 1}
	//msg2 := OrderPlacedAck{SourceID: 1, OrderID: 1234, MsgType: "ackack", Score: 666}
	//orderPlacedSendCh := make(chan OrderPlacedMsg)
	//orderPlacedRecvCh := make(chan msgtype.OrderPlacedMsg)
	//orderPlacedAckSendCh := make(chan OrderPlacedAck)
	//orderPlacedAckRecvCh := make(chan msgtype.OrderPlacedAck)
	/*
		takeOrderAckSendCh := make(chan TakeOrderAck)
		takeOrderAckRecvCh := make(chan TakeOrderAck)
		takeOrderSendCh := make(chan TakeOrderMsg)
		takeOrderRecvCh := make(chan TakeOrderMsg)
		heartbeatSendCh := make(chan Heartbeat)
		heartbeatRecvCh := make(chan Heartbeat)
	*/

	peerTxEnable := make(chan bool)
	peerStatusSendCh := make(chan []byte)
	peerStatusCh := make(chan msgtype.Heartbeat)
	peerUpdateCh := make(chan peers.PeerUpdate)

	go bcast.Transmitter(20010, orderPlacedSendCh, orderPlacedAckSendCh)
	go bcast.Receiver(20010, orderPlacedRecvCh, orderPlacedAckRecvCh)
	go peers.Transmitter(20010, "testid", peerTxEnable, peerStatusSendCh)
	go peers.Receiver(20010, peerUpdateCh, peerStatusCh)

	fmt.Println("Listening")

	for {
		select {
		case msgRecv1 := <-orderPlacedRecvCh:
			fmt.Println(msgRecv1)
		case msgRecv2 := <-orderPlacedAckRecvCh:
			fmt.Println(msgRecv2)
		case peerUpdate := <-peerUpdateCh:
			fmt.Println(peerUpdate)
		case peerStatus := <-peerStatusCh:
			fmt.Printf("%+v\n", peerStatus)
		}
	}

}
