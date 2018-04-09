package main

import (
	"./comm/bcast"
	"./comm/peers"
	"./msgs"
	"flag"
	"fmt"
	"math"
	"math/rand"
	"time"
)

var rnd = rand.New(rand.NewSource(time.Now().UnixNano()))

//var rnd = rand.New(rand.NewSource(0))

//const server_ip = "129.241.187.38"
const port = 20010
const timeout = 1 * time.Second
const giveupAckwaitTimeout = 5 * time.Second

var id_ptr = flag.String("id", "noid", "ID for node")

func main() {
	flag.Parse()

	orderPlacedSendCh := make(chan msgs.PlaceOrderMsg)
	orderPlacedAckSendCh := make(chan msgs.OrderPlacedAck)
	takeOrderAckSendCh := make(chan msgs.TakeOrderAck)
	takeOrderSendCh := make(chan msgs.TakeOrderMsg)
	go bcast.Transmitter(port, orderPlacedSendCh, orderPlacedAckSendCh, takeOrderAckSendCh, takeOrderSendCh)

	orderPlacedRecvCh := make(chan msgs.PlaceOrderMsg)
	orderPlacedAckRecvCh := make(chan msgs.OrderPlacedAck)
	takeOrderAckRecvCh := make(chan msgs.TakeOrderAck)
	takeOrderRecvCh := make(chan msgs.TakeOrderMsg)
	go bcast.Receiver(port, orderPlacedRecvCh, orderPlacedAckRecvCh, takeOrderAckRecvCh, takeOrderRecvCh)

	peerTxEnable := make(chan bool)
	peerStatusSendCh := make(chan msgs.Heartbeat)
	go peers.Transmitter(port, peerTxEnable, peerStatusSendCh)

	peerUpdateCh := make(chan peers.PeerUpdate, 1)
	go peers.Receiver(port, peerUpdateCh)

	// OrderHandler channels
	thisElevatorHeartbeatCh := make(chan msgs.Heartbeat)
	elevatorsStatusCh := make(chan []msgs.ElevatorStatus)
	downedElevatorsCh := make(chan []msgs.Heartbeat)
	otherTakeOrderCh := make(chan msgs.TakeOrderMsg)
	thisTakeOrderCh := make(chan msgs.TakeOrderMsg)
	acceptOrderCh := make(chan msgs.AcceptOrderMsg)
	completedOrderCh := make(chan msgs.Order)

	// internal for network
	ordersRecieved := make(map[int]msgs.Order)
	placeUnackedOrders := make(map[int]time.Time) // time is time when added
	takeUnackedOrders := make(map[int]time.Time)  // time is time when added

	// pseudo-orderHandler and fsm
	go func(thisElevatorHeartbeatCh chan<- msgs.Heartbeat, elevatorsStatusCh <-chan []msgs.ElevatorStatus, downedElevatorsCh <-chan []msgs.Heartbeat,
		thisTakeOrderChan <-chan msgs.TakeOrderMsg, otherTakeOrderCh chan<- msgs.TakeOrderMsg,
		acceptOrderCh <-chan msgs.AcceptOrderMsg, completedOrderCh chan<- msgs.Order) {

		calculateOrderScore := func(status msgs.ElevatorStatus, order msgs.Order) int {
			floordiff := (int)(math.Abs((float64)(status.Floor - order.Floor)))
			samedirection := status.Direction == order.Direction

			var score int = 1
			score += floordiff
			if status.Stopped {
				score += 2
			}
			if !samedirection {
				score *= 2
			}

			return score
		}

		orders := make(map[int]msgs.Order)
		acceptedOrders := make(map[int]bool)     // used as a set
		thisElevatorOrders := make(map[int]bool) // used as a set
		thisElevatorOrdersUpdated := false

		placeOrderCh := make(chan msgs.Debug_placeOrderMsg)
		dbg_acceptOrderCh := make(chan msgs.Debug_acceptOrderMsg)
		go bcast.Receiver(port, placeOrderCh, dbg_acceptOrderCh)

		const fsmMaxFloor int = 4
		fsmStatus := msgs.ElevatorStatus{ID: *id_ptr, Direction: msgs.Up, Floor: 1 + rnd.Intn(fsmMaxFloor-1)}

		fmt.Println("[fsm] started at: ", fsmStatus)
		thisElevatorHeartbeatCh <- msgs.Heartbeat{SenderID: *id_ptr, Status: fsmStatus, AcceptedOrders: []msgs.Order{}}

		var elevators []msgs.ElevatorStatus

		for {
			select {
			case elevators = <-elevatorsStatusCh:
				fmt.Printf("[orderHandler]: elevators: ")
				for _, elevator := range elevators {
					fmt.Printf("%v ", elevator.ID)
				}
				fmt.Printf("\n")
			case downedElevators := <-downedElevatorsCh:
				for _, lastHeartbeat := range downedElevators {
					// elevator is down
					fmt.Printf("[orderHandler]: down: %+v\n", lastHeartbeat.SenderID)

					// take order this elevator had
					for _, order := range lastHeartbeat.AcceptedOrders {
						orders[order.ID] = order
						thisElevatorOrders[order.ID] = true
					}
				}
			case placeOrder := <-placeOrderCh:
				if placeOrder.RecieverID == *id_ptr {
					order := msgs.PlaceOrderMsg{SenderID: *id_ptr, Order: placeOrder.Order}

					if _, exists := orders[order.Order.ID]; exists {
						fmt.Printf("[orderHandler]: Warning, order id %v already exists, new order ignored", order.Order.ID)
						break
					}

					orders[order.Order.ID] = order.Order
					orderPlacedSendCh <- order
				}
			case acceptOrder := <-dbg_acceptOrderCh:
				if acceptOrder.RecieverID == *id_ptr {
					if _, exists := orders[acceptOrder.Order.ID]; exists {
						acceptedOrders[acceptOrder.Order.ID] = true

						scoreMap := make(map[string]int)
						for _, elevator := range elevators {
							scoreMap[elevator.ID] = calculateOrderScore(elevator, orders[acceptOrder.Order.ID])
						}

						// find best (lowest) score
						bestID := *id_ptr
						for id, score := range scoreMap {
							if score < scoreMap[bestID] {
								bestID = id
							}
						}

						fmt.Printf("[orderHandler]: elevator %v should take order %v (%v)\n", bestID, acceptOrder.Order.ID, scoreMap)
						if bestID != *id_ptr {
							takeOrderMsg := msgs.TakeOrderMsg{SenderID: *id_ptr, RecieverID: bestID, Order: orders[acceptOrder.Order.ID]}
							otherTakeOrderCh <- takeOrderMsg
						} else {
							thisElevatorOrders[acceptOrder.Order.ID] = true
							thisElevatorOrdersUpdated = true // for debugging
						}
					} else {
						fmt.Println("[orderHandler]: order didn't exist")
					}
				}
			case <-time.After(5 * time.Second):
				// pseudo-fsm

				// check if we can complete an order
				fsmStatus.Stopped = false
				for orderID, _ := range thisElevatorOrders {
					if orders[orderID].Floor == fsmStatus.Floor &&
						orders[orderID].Direction == fsmStatus.Direction {
						fmt.Printf("[fsm]: completing order %v\n", orderID)
						fsmStatus.Stopped = true

						// broadcast to network that order is completed
						completedOrderCh <- orders[orderID]

						// remove order from orderHandler/fsm
						delete(thisElevatorOrders, orderID)
						thisElevatorOrdersUpdated = false // for debugging
						delete(acceptedOrders, orderID)
						delete(orders, orderID)
					}
				}

				if !fsmStatus.Stopped {
					if fsmStatus.Floor == fsmMaxFloor {
						fsmStatus.Direction = msgs.Down
					} else if fsmStatus.Floor == 1 {
						fsmStatus.Direction = msgs.Up
					}
					if fsmStatus.Direction == msgs.Up {
						fsmStatus.Floor += 1
					} else if fsmStatus.Direction == msgs.Down {
						fsmStatus.Floor -= 1
					}
				}

				//fmt.Printf("[fsm]: floor=%v, direction=%v\n", fsmStatus.Floor, fsmStatus.Direction)

				var acceptedOrderList []msgs.Order
				for orderID, _ := range acceptedOrders {
					if order, exists := orders[orderID]; exists {
						acceptedOrderList = append(acceptedOrderList, order)
					} else {
						fmt.Printf("[thisElevatorHeartbeatCh]: Warn: orderID %v didn't exist")
					}
				}

				thisElevatorHeartbeatCh <- msgs.Heartbeat{SenderID: *id_ptr, Status: fsmStatus, AcceptedOrders: acceptedOrderList}
			case msg := <-thisTakeOrderCh:
				if _, exists := orders[msg.Order.ID]; !exists {
					fmt.Printf("[thisTakeOrderCh]: didnt have order %v,from before, %v\n", msg.Order.ID, orders)
					orders[msg.Order.ID] = msg.Order
				}
				// error checking
				if orders[msg.Order.ID] != msg.Order {
					fmt.Printf("[thisTakeOrderCh]: had different order with same ID \n\t(my)%+v\n\t(recv)%+v\n", orders[msg.Order.ID], msg.Order)
				}

				acceptedOrders[msg.Order.ID] = true
				thisElevatorOrders[msg.Order.ID] = true
				thisElevatorOrdersUpdated = true // for debugging
			}

			if thisElevatorOrdersUpdated {
				thisElevatorOrdersUpdated = false
				fmt.Printf("[orderhandler]: my orders")
				for orderID, _ := range thisElevatorOrders {
					fmt.Printf(" %v", orderID)
				}
				fmt.Printf("\n")
			}
		}
	}(thisElevatorHeartbeatCh, elevatorsStatusCh, downedElevatorsCh,
		thisTakeOrderCh, otherTakeOrderCh,
		acceptOrderCh, completedOrderCh)

	fmt.Println("Listening")
	for {
		select {
		case msg := <-orderPlacedRecvCh:
			if msg.SenderID != *id_ptr { // ignore internal msgs
				// Order transmitted from other node

				// store order
				if _, exists := ordersRecieved[msg.Order.ID]; exists {
					fmt.Printf("[orderPlacedRecvCh]: Warning, order id %v already exists, new order ignored", msg.Order.ID)
					break
				}
				ordersRecieved[msg.Order.ID] = msg.Order
				//fmt.Println("[orderPlacedRecvCh]:", msg)
				ack := msgs.OrderPlacedAck{SenderID: *id_ptr,
					RecieverID: msg.SenderID,
					Order:      msg.Order}
				fmt.Printf("[orderPlacedRecvCh]: Sending ack to %v for order %v\n", ack.RecieverID, ack.Order.ID)
				orderPlacedAckSendCh <- ack
			} else {
				ordersRecieved[msg.Order.ID] = msg.Order
				// This node has sent out an order. Needs to listen for acks
				if ackwait, exists := placeUnackedOrders[msg.Order.ID]; exists {
					fmt.Printf("[orderPlacedRecvCh]: Warning, ack wait id %v already exists %v\n", msg.Order.ID, ackwait)
				} else {
					placeUnackedOrders[msg.Order.ID] = time.Now()
				}
			}
		case msg := <-orderPlacedAckRecvCh:
			if msg.RecieverID == *id_ptr { // ignore msgs to other nodes
				// Acknowledgement recieved from other node
				if _, ok := placeUnackedOrders[msg.Order.ID]; !ok {
					break // Not waiting for acknowledgment
				}

				fmt.Printf("[orderPlacedAckRecvCh]: %v acknowledged\n", msg.Order.ID)
				delete(placeUnackedOrders, msg.Order.ID)

				// TODO: it is now safe to accept order since more than one elevator know about it
			}
		case msg := <-otherTakeOrderCh:
			takeOrderSendCh <- msg
			takeUnackedOrders[msg.Order.ID] = time.Now()
		case msg := <-takeOrderRecvCh:
			if msg.RecieverID == *id_ptr {
				thisTakeOrderCh <- msg

				ack := msgs.TakeOrderAck{SenderID: *id_ptr, RecieverID: msg.SenderID, Order: msg.Order}
				takeOrderAckSendCh <- ack
			}
		case msg := <-takeOrderAckRecvCh:
			if msg.RecieverID == *id_ptr {
				fmt.Printf("[takeOrderAckRecvCh]: Recieved ack: %v\n", msg)
				delete(takeUnackedOrders, msg.Order.ID)
			}
		case peerUpdate := <-peerUpdateCh:
			if len(peerUpdate.Lost) > 0 {
				var downedElevators []msgs.Heartbeat
				for _, lastHeartbeat := range peerUpdate.Lost {
					downedElevators = append(downedElevators, lastHeartbeat)
				}

				downedElevatorsCh <- downedElevators
			}

			if len(peerUpdate.New) > 0 {
				//fmt.Println("[peerUpdateCh]: New: ", peerUpdate.New)
			}

			elevatorsStatusCh <- peerUpdate.Peers
		case order := <-completedOrderCh:
			fmt.Println("[orderCompletedCh]: ", order)

		case heartbeat := <-thisElevatorHeartbeatCh:
			peerStatusSendCh <- heartbeat

		}

		// actions that happen on every update
		for orderID, t := range placeUnackedOrders {
			if time.Now().Sub(t) > giveupAckwaitTimeout {
				fmt.Printf("[timeout]: place ack for %v\n", orderID)

				delete(placeUnackedOrders, orderID)
			}
		}

		for orderID, t := range takeUnackedOrders {
			if time.Now().Sub(t) > giveupAckwaitTimeout {
				fmt.Printf("[timeout]: take ack for %v\n", orderID)
				msg := msgs.TakeOrderMsg{SenderID: *id_ptr, RecieverID: *id_ptr,
					Order: msgs.Order{ID: orderID}} // TODO: get information to fill out order floor etc. elevator behaviour shouldn't need this
				thisTakeOrderCh <- msg

				delete(takeUnackedOrders, orderID)
			}
		}

	}
}
