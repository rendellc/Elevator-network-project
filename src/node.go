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

	peerUpdateCh := make(chan peers.PeerUpdate, 1)
	go peers.Receiver(port, peerUpdateCh)

	heartbeat := msgs.Heartbeat{SenderID: *id_ptr,
		ElevatorState:  msgs.StopDown,
		AcceptedOrders: []msgs.Order{}}

	peerStatusSendCh <- heartbeat

	// OrderHandler channels
	downedElevatorsCh := make(chan []msgs.Heartbeat)

	ordersRecieved := make(map[int]msgs.Order)
	unacknowledgedOrders := make(map[int]msgs.Order)

	// pseudo-orderHandler
	go func(downedElevatorsCh <-chan []msgs.Heartbeat) {
		for {
			select {
			case downedElevators := <-downedElevatorsCh:
				for _, elevator := range downedElevators {
					fmt.Printf("[orderHandler]: down: %+v\n", elevator)
				}
			}
		}
	}(downedElevatorsCh)

	fmt.Println("Listening")
	for {
		select {
		case msg := <-orderPlacedRecvCh:
			if msg.SenderID != *id_ptr { // ignore internal msgs
				// Order transmitted from other node

				// store order
				if _, ok := ordersRecieved[msg.Order.ID]; ok {
					fmt.Printf("[orderPlacedRecvCh]: Warning, order id %v already exists, new order ignored", msg.Order.ID)
					break
				}
				ordersRecieved[msg.Order.ID] = msg.Order
				if msg.RecieverID == *id_ptr {
					fmt.Println("[orderPlacedRecvCh]:", msg)
					ack := msgs.OrderPlacedAck{SenderID: *id_ptr,
						RecieverID: msg.SenderID,
						Order:      msg.Order,
						Score:      50} // TODO: scoring system
					fmt.Printf("[orderPlacedRecvCh]: Sending ack to %v for order %v\n", ack.RecieverID, ack.Order.ID)
					orderPlacedAckSendCh <- ack
				}
			} else {
				ordersRecieved[msg.Order.ID] = msg.Order
				// This node has sent out an order. Needs to listen for acks
				if _, ok := unacknowledgedOrders[msg.Order.ID]; ok {
					fmt.Printf("[orderPlacedRecvCh]: Warning, ack wait id %i already exists, new order ignored\n", msg.Order.ID)
				} else {
					unacknowledgedOrders[msg.Order.ID] = msg.Order
				}
			}
		case msg := <-orderPlacedAckRecvCh:
			if msg.RecieverID == *id_ptr { // ignore msgs to other nodes
				// Acknowledgement recieved from other node
				if _, ok := unacknowledgedOrders[msg.Order.ID]; !ok {
					break // Not waiting for acknowledgment
				}

				fmt.Println("[orderPlacedAckRecvCh]: Acknowledgment recieved")
				delete(unacknowledgedOrders, msg.Order.ID)

				// TODO: it is now safe to accept order since more than one elevator know about it
			}
		case peerUpdate := <-peerUpdateCh:
			if len(peerUpdate.Lost) > 0 {

				var downedElevators []msgs.Heartbeat
				for _, elevatorID := range peerUpdate.Lost {
					downedHeartbeat := msgs.Heartbeat{SenderID: elevatorID,
						ElevatorState:  msgs.StopDown,
						AcceptedOrders: []msgs.Order{}}

					downedElevators = append(downedElevators, downedHeartbeat)
				}

				downedElevatorsCh <- downedElevators
			}
			if len(peerUpdate.New) > 0 {
				fmt.Println("[peerUpdateCh]: New: ", peerUpdate.New)
			}
		}

		if len(ordersRecieved) > 0 {
			fmt.Println("ordersRecieved map")
			for key, value := range ordersRecieved {
				var _ = value
				fmt.Printf("\t%v -> %+v\n", key, value)

				// TODO: Communicate order to orderhandler

				//delete(ordersRecieved, key)
			}
		}

		if len(unacknowledgedOrders) > 0 {
			fmt.Println("unacknowledgedOrders map")
			for key, value := range unacknowledgedOrders {
				var _ = value
				fmt.Printf("\t%v -> %+v\n", key, value)

				// TODO: Logic

				//delete(unacknowledgedOrders, key)
			}
		}

	}
}
