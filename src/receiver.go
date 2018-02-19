package main

import (
	"fmt"
	//"encoding/json"
	//"./network/bcast"
	"./network/peers"
	"./msgtype"
)

const server_ip = "129.241.187.38"


func main() {
	//msg1 := OrderPlacedMsg{SourceID: 0, MsgType: "testing type", Order: Order{OrderID: 1234, Floor: 1, Direction: -1}, Priority: 1}
	//msg2 := OrderPlacedAck{SourceID: 1, OrderID: 1234, MsgType: "ackack", Score: 666}
	//beat := Heartbeat{SourceID: 3, ElevatorState: MovingUp, AcceptedOrders: []Order{Order{OrderID: 1234, Floor: 1, Direction: Down}}, TakenOrders: []Order{Order{OrderID: 1234, Floor: 1, Direction: Down}}}
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

	//peerTxEnable := make(chan bool)
	//peerStatusSendCh := make(chan []byte)
	peerStatusCh := make(chan msgtype.Heartbeat)
	peerUpdateCh := make(chan peers.PeerUpdate)


	//go bcast.Transmitter(20010, orderPlacedSendCh, orderPlacedAckSendCh)
	//go bcast.Receiver(20010, orderPlacedRecvCh, orderPlacedAckRecvCh)
	//go peers.Transmitter(20010, "testid", peerTxEnable, peerStatusSendCh)
	go peers.Receiver(20010, peerUpdateCh, peerStatusCh)
/*
	orderPlacedSendCh<-msg1
	orderPlacedAckSendCh<-msg2
	orderPlacedSendCh<-msg1
	orderPlacedAckSendCh<-msg2
	orderPlacedSendCh<-msg1
	
*/	

	//jsonbeat, _ := json.Marshal(beat)
	//peerStatusSendCh <- jsonbeat
	fmt.Println("Listening")

	for{
		select{
		//ase msgRecv1:= <-orderPlacedRecvCh:
		//	fmt.Println(msgRecv1)
		//case msgRecv2:= <-orderPlacedAckRecvCh:
		//	fmt.Println(msgRecv2)
		case peerUpdate := <-peerUpdateCh:
			fmt.Println(peerUpdate)
		case peerStatus := <-peerStatusCh:
			fmt.Printf("%+v\n",peerStatus)
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
