package main

import (
	"fmt"
	"./network/bcast"
)

const server_ip = "129.241.187.38"

type Direction int

const (
	Up Direction = iota
	Down
)

type ElevatorState int

const (
	MovingUp ElevatorState = iota
	MovingDown
	StopUp
	StopDown
)

type Order struct {
	OrderID   int       `json:"order_id"`
	Floor     int       `json:"floor"`
	Direction Direction `json:"direction"`
}

type OrderPlacedMsg struct {
	SourceID int    `json:"source_id"`
	MsgType  string `json:"msg_type"`
	Order    Order  `json:"order"`
	Priority int    `json:"priority"`
}

type OrderPlacedAck struct {
	SourceID int    `json:"source_id"`
	OrderID  int    `json:"order_id"`
	MsgType  string `json:"msg_type"`
	Score    int    `json:"score"`
}

type TakeOrderAck struct {
	OrderID int    `json:"order_id"`
	MsgType string `json:"msg_type"`
}

type Heartbeat struct {
	SourceID       int           `json:"source_id"`
	ElevatorState  ElevatorState `json:"elevator_state"`
	AcceptedOrders []Order       `json:"accepted_orders"`
	TakenOrders    []Order       `json:"taken_orders"`
}

type TakeOrderMsg struct {
	Order Order `json:"order"`
	CmdID int   `json:"cmd_id"` // specify the elevator that should take the order
}

func main() {
	msg1 := OrderPlacedMsg{SourceID: 0, MsgType: "testing type", Order: Order{OrderID: 1234, Floor: 1, Direction: -1}, Priority: 1}
	msg2 := OrderPlacedAck{SourceID: 1, OrderID: 1234, MsgType: "ackack", Score: 666}

	orderPlacedSendCh := make(chan OrderPlacedMsg)
	orderPlacedRecvCh := make(chan OrderPlacedMsg)
	orderPlacedAckSendCh := make(chan OrderPlacedAck)
	orderPlacedAckRecvCh := make(chan OrderPlacedAck)
	/*
	takeOrderAckSendCh := make(chan TakeOrderAck)
	takeOrderAckRecvCh := make(chan TakeOrderAck)
	takeOrderSendCh := make(chan TakeOrderMsg)
	takeOrderRecvCh := make(chan TakeOrderMsg)
	heartbeatSendCh := make(chan Heartbeat)
	heartbeatRecvCh := make(chan Heartbeat)
	*/


	go bcast.Transmitter(20010, orderPlacedSendCh, orderPlacedAckSendCh)
	go bcast.Receiver(20010, orderPlacedRecvCh, orderPlacedAckRecvCh)

	orderPlacedSendCh<-msg1
	orderPlacedAckSendCh<-msg2
	orderPlacedSendCh<-msg1
	orderPlacedAckSendCh<-msg2
	orderPlacedSendCh<-msg1
	fmt.Println("Message transmitted")

	msgRecv1:= <-orderPlacedRecvCh
	fmt.Println(msgRecv1)
	msgRecv2:= <-orderPlacedAckRecvCh
	fmt.Println(msgRecv2)
	msgRecv1= <-orderPlacedRecvCh
	fmt.Println(msgRecv1)
	msgRecv2= <-orderPlacedAckRecvCh
	fmt.Println(msgRecv2)
	msgRecv1= <-orderPlacedRecvCh
	fmt.Println(msgRecv1)
	
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
