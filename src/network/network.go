package network

import (
	"../comm/bcast"
	"../comm/peers"
	"../msgs"
	"fmt"
	"math/rand"
	"sync"
	"time"
)

var rnd = rand.New(rand.NewSource(time.Now().UnixNano()))

//const server_ip = "129.241.187.38"
const commonPort = 20010
const timeout = 1 * time.Second
const giveupAckwaitTimeout = 5 * time.Second
const N_FLOORS = 4 //import
const N_BUTTONS = 3

func Launch(thisID string,
	/* read */
	thisElevatorHeartbeatCh <-chan msgs.Heartbeat,
	downedElevatorsCh chan<- []msgs.Heartbeat,
	placedOrderCh <-chan msgs.Order,
	broadcastTakeOrderCh <-chan msgs.TakeOrderMsg,
	completedOrderCh <-chan msgs.Order,
	/* write */
	allElevatorsHeartbeatCh chan<- []msgs.Heartbeat,
	thisTakeOrderCh chan<- msgs.TakeOrderMsg,
	safeOrderCh chan<- msgs.SafeOrderMsg,
	completedOrderOtherElevCh chan<- msgs.Order,
	/* sync */
	wg *sync.WaitGroup) {

	placedOrderSendCh := make(chan msgs.PlacedOrderMsg)
	placedOrderAckSendCh := make(chan msgs.PlacedOrderAck)
	takeOrderAckSendCh := make(chan msgs.TakeOrderAck)
	takeOrderSendCh := make(chan msgs.TakeOrderMsg)
	completeOrderSendCh := make(chan msgs.CompleteOrderMsg)
	go bcast.Transmitter(commonPort, placedOrderSendCh, placedOrderAckSendCh, takeOrderAckSendCh, takeOrderSendCh, completeOrderSendCh)

	placedOrderRecvCh := make(chan msgs.PlacedOrderMsg)
	placedOrderAckRecvCh := make(chan msgs.PlacedOrderAck)
	takeOrderAckRecvCh := make(chan msgs.TakeOrderAck)
	takeOrderRecvCh := make(chan msgs.TakeOrderMsg)
	completeOrderRecvCh := make(chan msgs.CompleteOrderMsg)
	go bcast.Receiver(commonPort, placedOrderRecvCh, placedOrderAckRecvCh, takeOrderAckRecvCh, takeOrderRecvCh, completeOrderRecvCh)

	peerTxEnable := make(chan bool)
	peerStatusSendCh := make(chan msgs.Heartbeat)
	go peers.Transmitter(commonPort, peerTxEnable, peerStatusSendCh)

	peerUpdateCh := make(chan peers.PeerUpdate, 1)
	go peers.Receiver(commonPort, peerUpdateCh)

	// bookkeeping variables
	recievedOrders := make(map[int]msgs.Order)    // list of all placed orders to/from all elevators
	placeUnackedOrders := make(map[int]time.Time) // time is time when added
	takeUnackedOrders := make(map[int]time.Time)  // time is time when added
	allOngoingOrders := make(map[int]time.Time)   // time is time when added

	// Wait until all modules are initialized
	wg.Done()
	fmt.Println("[network]: initialized")
	wg.Wait()
	fmt.Println("[network]: starting")
	for {
		select {
		case msg := <-placedOrderRecvCh:
			// store order
			if _, exists := recievedOrders[msg.Order.ID]; exists {
				fmt.Printf("[placedOrderRecvCh]: Warning, order id %v already exists, new order ignored\n", msg.Order.ID)
				break
			}
			recievedOrders[msg.Order.ID] = msg.Order

			if msg.SenderID != thisID { // ignore internal msgs
				// Order transmitted from other node

				// acknowledge order
				ack := msgs.PlacedOrderAck{SenderID: thisID,
					ReceiverID: msg.SenderID,
					Order:      msg.Order}
				fmt.Printf("[placedOrderRecvCh]: Sending ack to %v for order %v\n", ack.ReceiverID, ack.Order.ID)
				placedOrderAckSendCh <- ack
			}
		case order := <-placedOrderCh:
			// This node has sent out an order. Needs to listen for acks
			if ackwait, exists := placeUnackedOrders[order.ID]; exists {
				fmt.Printf("[placedOrderRecvCh]: Warning, ack wait id %v already exists %v\n", order.ID, time.Now().Sub(ackwait))
			}
			placeUnackedOrders[order.ID] = time.Now()

			placedOrderSendCh <- msgs.PlacedOrderMsg{SenderID: thisID, Order: order}

		case msg := <-placedOrderAckRecvCh:
			if msg.ReceiverID == thisID { // ignore msgs to other nodes
				// Acknowledgement recieved from other node
				if _, exists := placeUnackedOrders[msg.Order.ID]; !exists {
					//fmt.Printf("[placedOrderAckRecvCh]: not waiting on ack %+v\n", msg.Order)
					break // Not waiting for acknowledgment
					// TODO: maybe count how often we end up here?
				}

				fmt.Printf("[placedOrderAckRecvCh]: %v acknowledged\n", msg.Order.ID)
				delete(placeUnackedOrders, msg.Order.ID)

				// Order is safe since multiple elevators knows about it, notify orderHandler
				safeMsg := msgs.SafeOrderMsg{SenderID: thisID, ReceiverID: thisID, Order: msg.Order}
				safeOrderCh <- safeMsg
			}
		case msg := <-broadcastTakeOrderCh:
			takeOrderSendCh <- msg
			takeUnackedOrders[msg.Order.ID] = time.Now()

		case msg := <-takeOrderRecvCh:
			if msg.ReceiverID == thisID {
				thisTakeOrderCh <- msg

				ack := msgs.TakeOrderAck{SenderID: thisID, ReceiverID: msg.SenderID, Order: msg.Order}

				takeOrderAckSendCh <- ack
			}

		case msg := <-takeOrderAckRecvCh:
			if msg.ReceiverID == thisID {
				fmt.Printf("[takeOrderAckRecvCh]: Recieved ack: %v\n", msg)
				delete(takeUnackedOrders, msg.Order.ID)
			}

			// contains all ongoing orders from all elevators
			allOngoingOrders[msg.Order.ID] = time.Now()

		case peerUpdate := <-peerUpdateCh:
			if len(peerUpdate.Lost) > 0 {
				var downedElevators []msgs.Heartbeat
				for _, lastHeartbeat := range peerUpdate.Lost {
					fmt.Printf("[peerUpdateCh]: lost %v\n", lastHeartbeat.SenderID)
					downedElevators = append(downedElevators, lastHeartbeat)
				}

				downedElevatorsCh <- downedElevators
			}

			if len(peerUpdate.New) > 0 {
				fmt.Println("[peerUpdateCh]: New: ", peerUpdate.New)
			}

			fmt.Println("[network]: allElevatorsHeartbeatCh write")
			allElevatorsHeartbeatCh <- peerUpdate.Peers
			fmt.Println("[network]: allElevatorsHeartbeatCh done")

		case order := <-completedOrderCh:
			fmt.Println("[orderCompletedCh]: ", order)
			delete(allOngoingOrders, order.ID)
			delete(recievedOrders, order.ID)
			completeOrderSendCh <- msgs.CompleteOrderMsg{Order: order}

		case msg := <-completeOrderRecvCh:
			//if _, exists := allOngoingOrders[msg.Order.ID]; exists {
			fmt.Println("[orderCompletedRecvCh]: ", msg.Order)
			delete(allOngoingOrders, msg.Order.ID)
			delete(recievedOrders, msg.Order.ID)

			if msg.SenderID != thisID {
				// TODO: send to order handler
				completedOrderOtherElevCh <- msg.Order
			}
			//}

		case heartbeat := <-thisElevatorHeartbeatCh:
			// heartbeat may lacks thisID
			heartbeat.SenderID = thisID

			peerStatusSendCh <- heartbeat

		case <-time.After(15 * time.Second):
			fmt.Println("[network]: running")
		}

		// actions that happen on every update
		for orderID, t := range placeUnackedOrders {
			if time.Now().Sub(t) > giveupAckwaitTimeout {
				fmt.Printf("[timeout]: place ack for %v\n", orderID)

				// TODO: should accept order if this happens many times!

				delete(placeUnackedOrders, orderID)
			}
		}

		for orderID, t := range takeUnackedOrders {
			if time.Now().Sub(t) > giveupAckwaitTimeout {
				//fmt.Printf("[timeout]: take ack for %v\n", orderID)
				//msg := msgs.TakeOrderMsg{SenderID: thisID, ReceiverID: thisID,
				//	Order: recievedOrders[orderID]}

				//fmt.Println("[net-timeout]: unack write")
				//thisTakeOrderCh <- msg
				//fmt.Println("[net-timeout]: unack done")

				delete(takeUnackedOrders, orderID)
			}
		}

		for orderID, t := range allOngoingOrders {
			if time.Now().Sub(t) > 30*time.Second {
				//fmt.Printf("[timeout]: complete not recieved for %v\n\t%v\n", orderID, allOngoingOrders)

				//msg := msgs.TakeOrderMsg{SenderID: thisID, ReceiverID: thisID,
				//	Order: recievedOrders[orderID]} // TODO: get information to fill out order floor etc. elevator behaviour shouldn't need this

				//fmt.Println("[net-timeout]: uncomp write")
				//thisTakeOrderCh <- msg
				//fmt.Println("[net-timeout]: uncomp done")
				delete(allOngoingOrders, orderID)
				delete(recievedOrders, orderID)
			}
		}
	}
}
