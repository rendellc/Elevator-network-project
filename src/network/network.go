package network

import (
	"../comm/bcast"
	"../comm/peers"
	"../msgs"
	"fmt"
	"../go-nonblockingchan"
	"math/rand"
	"sync"
	"time"
)

type StampedOrder struct {
	StampTime time.Time
	Order msgs.Order
}

var rnd = rand.New(rand.NewSource(time.Now().UnixNano()))

const commonPort = 20010
const timeout = 1 * time.Second
const placedAckwaitTimeout = 100 * time.Millisecond
const N_FLOORS = 4
const N_BUTTONS = 3

func Launch(thisID string,
	/* read */
	thisElevatorHeartbeatCh *nbc.NonBlockingChan,
	downedElevatorsCh *nbc.NonBlockingChan,
	placedOrderCh *nbc.NonBlockingChan,
	broadcastTakeOrderCh *nbc.NonBlockingChan,
	completedOrderCh *nbc.NonBlockingChan,
	/* write */
	allElevatorsHeartbeatCh *nbc.NonBlockingChan,
	thisTakeOrderCh *nbc.NonBlockingChan,
	safeOrderCh *nbc.NonBlockingChan,
	completedOrderOtherElevCh *nbc.NonBlockingChan,
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
	placeUnackedOrders := make(map[int]StampedOrder) // time is time when added
	takeUnackedOrders := make(map[int]StampedOrder)  // time is time when added
	allOngoingOrders := make(map[int]StampedOrder)   // time is time when added

	placedTryAgainCount := make(map[int]int)

	// Wait until all modules are initialized
	wg.Done()
	fmt.Println("[network]: initialized")
	wg.Wait()
	fmt.Println("[network]: starting")
	for {
		select {
		case msg := <-placedOrderRecvCh:
			// store order
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
		case msg, _ := <-placedOrderCh.Recv:
			order := msg.(msgs.Order)
			// This node has sent out an order. Needs to listen for acks
			placeUnackedOrders[order.ID] = StampedOrder{StampTime: time.Now(), Order: order}

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
				safeOrderCh.Send <- safeMsg
			}
		case msg, _ := <-broadcastTakeOrderCh.Recv:
			takeOrderMsg := msg.(msgs.TakeOrderMsg)
			takeOrderSendCh <- takeOrderMsg
			takeUnackedOrders[takeOrderMsg.Order.ID] = StampedOrder{StampTime: time.Now(),
				Order: takeOrderMsg.Order}

		case msg := <-takeOrderRecvCh:
			if msg.ReceiverID == thisID {
				thisTakeOrderCh.Send <- msg

				ack := msgs.TakeOrderAck{SenderID: thisID, ReceiverID: msg.SenderID, Order: msg.Order}

				takeOrderAckSendCh <- ack
			}

		case msg := <-takeOrderAckRecvCh:
			if msg.ReceiverID == thisID {
				fmt.Printf("[network]: Recieved ack: %v\n", msg)
				delete(takeUnackedOrders, msg.Order.ID)
			}

			// contains all ongoing orders from all elevators
			allOngoingOrders[msg.Order.ID] = StampedOrder{StampTime: time.Now(),
				Order: msg.Order}

		case peerUpdate := <-peerUpdateCh:
			if len(peerUpdate.Lost) > 0 {
				var downedElevators []msgs.Heartbeat
				for _, lastHeartbeat := range peerUpdate.Lost {
					fmt.Printf("[peerUpdateCh]: lost %v\n", lastHeartbeat.SenderID)
					downedElevators = append(downedElevators, lastHeartbeat)
				}

				downedElevatorsCh.Send <- downedElevators
			}

			if len(peerUpdate.New) > 0 {
				fmt.Println("[peerUpdateCh]: New: ", peerUpdate.New)
				// TODO: special action?
			}

			allElevatorsHeartbeatCh.Send <- peerUpdate.Peers

		case msg, _ := <-completedOrderCh.Recv:
			order := msg.(msgs.Order)
			fmt.Printf("[network]: completedOrderCh: %v\n", order)
			delete(allOngoingOrders, order.ID)
			delete(takeUnackedOrders, order.ID)
			delete(placeUnackedOrders, order.ID)
			delete(recievedOrders, order.ID)
			completeOrderSendCh <- msgs.CompleteOrderMsg{Order: order}

		case msg := <-completeOrderRecvCh:

			fmt.Printf("[network]: completeOrderrecv: %v\n", msg.Order)
			delete(allOngoingOrders, msg.Order.ID)
			delete(recievedOrders, msg.Order.ID)

			if msg.SenderID != thisID {
				// TODO: send to order handler
				completedOrderOtherElevCh.Send <- msg.Order
			}
			//}

		case msg, _ := <-thisElevatorHeartbeatCh.Recv:
			heartbeat := msg.(msgs.Heartbeat)
			// heartbeat may lacks thisID
			heartbeat.SenderID = thisID

			peerStatusSendCh <- heartbeat
		case <-time.After(100 * time.Millisecond):
			//fmt.Println("[network]: running")
		}

		// actions that happen on every update
		for orderID, stampedOrder := range placeUnackedOrders {
			if time.Now().Sub(stampedOrder.StampTime) > placedAckwaitTimeout {

				// Try again
				if placedTryAgainCount[orderID] < 3 {
					placedTryAgainCount[orderID] += 1
					placeUnackedOrders[orderID] = StampedOrder{StampTime: time.Now(), // update stamp time
						Order: stampedOrder.Order}
					placedOrderSendCh <- msgs.PlacedOrderMsg{SenderID: thisID,
						Order: stampedOrder.Order}
				} else {
					fmt.Printf("[network]: ack timeout on order %v\n", orderID)
					thisTakeOrderCh.Send <- msgs.TakeOrderMsg{SenderID: thisID,
						ReceiverID: thisID,
						Order: stampedOrder.Order}
					delete(placedTryAgainCount, orderID)
					delete(placeUnackedOrders, orderID)
				}
			}
		}

	for orderID, stampedOrder := range takeUnackedOrders {
		if time.Now().Sub(stampedOrder.StampTime) > placedAckwaitTimeout {
			fmt.Printf("[timeout]: take ack for %v\n", orderID)
			msg := msgs.TakeOrderMsg{SenderID: thisID, ReceiverID: thisID,
				Order: recievedOrders[orderID]}

			thisTakeOrderCh.Send <- msg
			delete(takeUnackedOrders, orderID)
		}
	}

	for orderID, stampedOrder := range allOngoingOrders {
		if time.Now().Sub(stampedOrder.StampTime) > 30*time.Second {
			fmt.Printf("[timeout]: complete not recieved for %v\n\t%v\n", orderID, allOngoingOrders)

			msg := msgs.TakeOrderMsg{SenderID: thisID, ReceiverID: thisID,
				Order: recievedOrders[orderID]} // TODO: get information to fill out order floor etc. elevator behaviour shouldn't need this

			thisTakeOrderCh.Send <- msg
			delete(allOngoingOrders, orderID)
			delete(recievedOrders, orderID)
		}
	}
}
}
