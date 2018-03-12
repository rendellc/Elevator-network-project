package main

import (
	"./comm/bcast"
	"./comm/peers"
	"./msgs"
	"flag"
	"fmt"
	"time"
)

//const server_ip = "129.241.187.38"
const port = 20010
const timeout = 1 * time.Second

var id_ptr = flag.String("id", "noid", "ID for node")

func main() {
	flag.Parse()

	orderPlacedSendCh := make(chan msgs.OrderPlacedMsg)
	orderPlacedAckSendCh := make(chan msgs.OrderPlacedAck)
	takeOrderAckSendCh := make(chan msgs.TakeOrderAck)
	takeOrderSendCh := make(chan msgs.TakeOrderMsg)
	go bcast.Transmitter(port, orderPlacedSendCh, orderPlacedAckSendCh, takeOrderAckSendCh, takeOrderSendCh)

	orderPlacedRecvCh := make(chan msgs.OrderPlacedMsg)
	orderPlacedAckRecvCh := make(chan msgs.OrderPlacedAck)
	takeOrderAckRecvCh := make(chan msgs.TakeOrderAck)
	takeOrderRecvCh := make(chan msgs.TakeOrderMsg)
	go bcast.Receiver(port, orderPlacedRecvCh, orderPlacedAckRecvCh, takeOrderAckRecvCh, takeOrderRecvCh)

	peerTxEnable := make(chan bool)
	peerStatusSendCh := make(chan msgs.Heartbeat)
	go peers.Transmitter(port, peerTxEnable, peerStatusSendCh)

	//peerStatusCh := make(chan msgs.Heartbeat)
	peerUpdateCh := make(chan peers.PeerUpdate, 1)
	go peers.Receiver(port, peerUpdateCh)

	heartbeat := msgs.Heartbeat{SourceID: *id_ptr,
		ElevatorState:  msgs.StopDown,
		AcceptedOrders: []msgs.Order{},
		TakenOrders:    []msgs.Order{msgs.Order{ID: 0, Floor: 3, Direction: msgs.Up}}}

	peerStatusSendCh <- heartbeat
	fmt.Println("Listening")

	orders_recieved := make(map[int]msgs.Order)
	waiting_for_ack := make(map[int]msgs.Order)

	for {
		select {
		case msg := <-orderPlacedRecvCh:
			if msg.SourceID != *id_ptr {
				// Order transmitted from other node
				fmt.Println("[orderPlacedRecvCh]:", msg)
				// store order
				if _, ok := orders_recieved[msg.Order.ID]; ok {
					fmt.Printf("[orderPlacedRecvCh]: Warning, order id %i already exists, new order ignored", msg.Order.ID)
				} else {
					orders_recieved[msg.Order.ID] = msg.Order
				}

				ack := msgs.OrderPlacedAck{SourceID: *id_ptr,
					Order: msg.Order,
					Score: 50} // TODO: scoring system
				orderPlacedAckSendCh <- ack
			} else {
				// This node has sent out an order. Needs to listen for acks
				if _, ok := waiting_for_ack[msg.Order.ID]; ok {
					fmt.Printf("[orderPlacedRecvCh]: Warning, ack wait id %i already exists, new order ignored", msg.Order.ID)
				} else {
					waiting_for_ack[msg.Order.ID] = msg.Order
				}
			}
		case msg := <-orderPlacedAckRecvCh:
			if msg.SourceID != *id_ptr { // ignore internal msgs
				// Acknowledgement recieved from other node
				fmt.Println("[orderPlacedAckRecvCh]:", msg)

				if _, ok := waiting_for_ack[msg.Order.ID]; ok {
					break // Not waiting for acknowledgment
				}

				fmt.Println("[orderPlacedAckRecvCh]: Expected acknowledgment recieved")

			}
		case peerUpdate := <-peerUpdateCh:
			fmt.Println("[peerUpdateCh]:", peerUpdate)
		}

		if len(orders_recieved) > 0 {
			fmt.Println("orders_recieved map")
			for key, value := range orders_recieved {
				fmt.Printf("\t%v -> %+v\n", key, value)

				// TODO: Communicate order to fsm

				delete(orders_recieved, key)
			}
		}

		if len(waiting_for_ack) > 0 {
			fmt.Println("waiting_for_ack map")
			for key, value := range waiting_for_ack {
				fmt.Printf("\t%v -> %+v\n", key, value)

				// TODO: Logic

				delete(waiting_for_ack, key)
			}
		}
	}
}
