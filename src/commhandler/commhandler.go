package commhandler

import (
	"../comm/bcast"
	"../comm/peers"
	"../go-nonblockingchan"
	"../msgs"
	"fmt"
	"sync"
	"time"
)

type OrderState int

// TODO: make these non-exported: ie. _SAFE
const (
	ACKWAIT_PLACED OrderState = iota // this elevator is waiting for an elevator to acknowledge a placed order
	SAFE                             // order has been seen by more than one elevator
	ACKWAIT_TAKE                     // this elevator is waiting for an elevator to acknowledge that it will take the order
	SERVING                          // order is being served by some elevator
)

type StampedOrder struct {
	TimeStamp     time.Time
	TransmitCount int
	PlacedCount   int
	OrderState    OrderState

	OrderMsg msgs.OrderMsg
}

func createStampedOrder(order msgs.Order, os OrderState) *StampedOrder {
	return &StampedOrder{TimeStamp: time.Now(),
		TransmitCount: 1,
		PlacedCount:   1,
		OrderState:    os,
		OrderMsg:      msgs.OrderMsg{Order: order}}
}

const ackwaitTimeout = 3000 * time.Millisecond
const placeAgainTimeIncrement = 10 * time.Second
const otherGiveupTime = 40 * time.Second
const retransmitCountMax = 5       // number of times to retransmit if no ack is recieved
const placedGiveupAndTakeTries = 3 // if no acks are recieved and user tries this many times, take order

func checkAndRetransmit(allOrders map[int]*StampedOrder, orderID int, thisID string,
	placedOrderSendCh chan<- msgs.PlacedOrderMsg, takeOrderSendCh chan<- msgs.TakeOrderMsg,
	safeOrderCh *nbc.NonBlockingChan) {

	if stampedOrder, exists := allOrders[orderID]; !exists {
		fmt.Printf("[network]: check and retransmit for non-existent order\n")
	} else {
		retransmitDuration := time.Duration(stampedOrder.TransmitCount) * ackwaitTimeout
		timeoutTime := stampedOrder.TimeStamp.Add(retransmitDuration)
		if time.Now().After(timeoutTime) {
			// Retransmit order
			if stampedOrder.TransmitCount <= retransmitCountMax {
				//fmt.Printf("[network]: retransmit order %v, place: %+v\n", orderID, allOrders[orderID])

				stampedOrder.TransmitCount += 1
				switch stampedOrder.OrderState {
				case ACKWAIT_PLACED:
					fmt.Printf("[network]: retransmitting place for %v\n", stampedOrder.OrderMsg.Order.ID)
					placedOrderSendCh <- msgs.PlacedOrderMsg{SenderID: thisID,
						Order: stampedOrder.OrderMsg.Order}
				case ACKWAIT_TAKE:
					fmt.Printf("[network]: retransmitting take for %v time %v\n", stampedOrder.OrderMsg.Order.ID, stampedOrder.TransmitCount)
					takeOrderSendCh <- msgs.TakeOrderMsg{SenderID: thisID,
						ReceiverID: stampedOrder.OrderMsg.ReceiverID,
						Order:      stampedOrder.OrderMsg.Order}

				}
			} else {
				if stampedOrder.PlacedCount >= placedGiveupAndTakeTries {
					fmt.Printf("[network]: %v retransmit failed %v times\n", orderID, stampedOrder.PlacedCount)
					switch stampedOrder.OrderState {
					case ACKWAIT_PLACED:
						safeOrderCh.Send <- msgs.SafeOrderMsg{SenderID: thisID,
							ReceiverID: thisID,
							Order:      stampedOrder.OrderMsg.Order}

						allOrders[orderID] = createStampedOrder(stampedOrder.OrderMsg.Order, SERVING)
					case ACKWAIT_TAKE:

					}
				}
			}
		}
	}
}

func Launch(thisID string, commonPort int,
	/* read */
	elevatorStatusCh *nbc.NonBlockingChan,
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
	updateHeartbeatCh := make(chan msgs.Heartbeat)
	go peers.Transmitter(commonPort, peerTxEnable, updateHeartbeatCh)

	peerUpdateCh := make(chan peers.PeerUpdate, 1)
	go peers.Receiver(commonPort, peerUpdateCh)

	allOrders := make(map[int]*StampedOrder)

	// Wait until all modules are initialized
	wg.Done()
	fmt.Println("[network]: initialized")
	wg.Wait()
	fmt.Println("[network]: starting")
	for {
		select {
		case msg := <-placedOrderRecvCh:
			if msg.SenderID != thisID { // Order transmitted from other node
				allOrders[msg.Order.ID] = createStampedOrder(msg.Order, SAFE)
				// acknowledge order
				ack := msgs.PlacedOrderAck{SenderID: thisID,
					ReceiverID: msg.SenderID,
					Order:      msg.Order}
				placedOrderAckSendCh <- ack
				fmt.Printf("[placedOrderRecvCh]: Sent ack to %v for order %v\n", ack.ReceiverID, ack.Order.ID)
			}

		case msg, _ := <-placedOrderCh.Recv:
			order := msg.(msgs.Order)

			if orderStamped, exists := allOrders[order.ID]; exists {
				fmt.Printf("[network]: existing order placed: state %v %v\n", orderStamped.OrderState, ACKWAIT_PLACED)

				if orderStamped.OrderState == ACKWAIT_PLACED {
					fmt.Println("[network]: old order in ackwait_placed")
					orderStamped.PlacedCount += 1
				}
			} else {
				//fmt.Println("[network]: new order in ACKWAIT_PLACED")
				allOrders[order.ID] = createStampedOrder(order, ACKWAIT_PLACED)
			}

			placedOrderSendCh <- msgs.PlacedOrderMsg{SenderID: thisID, Order: order}

		case msg := <-placedOrderAckRecvCh:
			if msg.ReceiverID == thisID { // ignore msgs to other nodes
				// Acknowledgement recieved from other node
				if _, exists := allOrders[msg.Order.ID]; !exists {
					fmt.Printf("[network]: order %v not found\n", msg.Order.ID)
					break
					// maybe count how often we end up here?
				}
				if orderStamped, _ := allOrders[msg.Order.ID]; orderStamped.OrderState != ACKWAIT_PLACED {
					fmt.Printf("[network]: order %v not in ackwait_placed\n", msg.Order.ID)
					break // Not waiting for acknowledgment
				}

				fmt.Printf("[network]: order %v acknowledged\n", msg.Order.ID)
				allOrders[msg.Order.ID].OrderState = SAFE

				// Order is safe since multiple elevators knows about it, notify orderHandler
				safeMsg := msgs.SafeOrderMsg{SenderID: thisID, ReceiverID: thisID, Order: msg.Order}
				safeOrderCh.Send <- safeMsg
			}
		case msg, _ := <-broadcastTakeOrderCh.Recv:
			orderMsg := msg.(msgs.TakeOrderMsg)
			//takeOrderSendCh <- orderMsg

			allOrders[orderMsg.Order.ID] = createStampedOrder(orderMsg.Order, ACKWAIT_TAKE)

		case msg := <-takeOrderRecvCh:
			allOrders[msg.Order.ID] = createStampedOrder(msg.Order, SERVING)

			if msg.ReceiverID == thisID {
				thisTakeOrderCh.Send <- msg

				ack := msgs.TakeOrderAck{SenderID: thisID, ReceiverID: msg.SenderID, Order: msg.Order}

				takeOrderAckSendCh <- ack
			}

		case msg := <-takeOrderAckRecvCh:
			if msg.ReceiverID == thisID {
				fmt.Printf("[network]: Recieved ack: %v\n", msg)
			}

			allOrders[msg.Order.ID] = createStampedOrder(msg.Order, SERVING)

		case peerUpdate := <-peerUpdateCh:
			if len(peerUpdate.Lost) > 0 {
				var downedElevators []msgs.Heartbeat
				for _, lastHeartbeat := range peerUpdate.Lost {
					fmt.Printf("[network]: lost %v\n", lastHeartbeat.SenderID)
					downedElevators = append(downedElevators, lastHeartbeat)
				}

				downedElevatorsCh.Send <- downedElevators
			}

			if len(peerUpdate.New) > 0 {
				fmt.Println("[network]: New peer: ", peerUpdate.New)
				// TODO: special action?
			}

			allElevatorsHeartbeatCh.Send <- peerUpdate.Peers

		case msg, _ := <-completedOrderCh.Recv:
			order := msg.(msgs.Order)

			fmt.Printf("[network]: completedOrderCh: %v\n", order)
			completeOrderSendCh <- msgs.CompleteOrderMsg{Order: order}
			delete(allOrders, order.ID)

		case msg := <-completeOrderRecvCh:

			fmt.Printf("[network]: completeOrderrecv: %v\n", msg.Order)
			delete(allOrders, msg.Order.ID)

			if msg.SenderID != thisID {
				completedOrderOtherElevCh.Send <- msg.Order
			}

		case msg, _ := <-elevatorStatusCh.Recv:
			heartbeat := msg.(msgs.Heartbeat)

			heartbeat.SenderID = thisID
			updateHeartbeatCh <- heartbeat

		case <-time.After(1000 * time.Millisecond):
			// make sure that below actions are processed regularly
			//fmt.Printf("[network]: all: %+v\n", allOrders)
		}

		// actions that happen on every update
		for orderID, stampedOrder := range allOrders {
			// retransmission if necessary
			checkAndRetransmit(allOrders, orderID, thisID, placedOrderSendCh, takeOrderSendCh, safeOrderCh)

			switch stampedOrder.OrderState {
			case ACKWAIT_PLACED:
				// TODO: verify that PlacedCount is incremented in the above function call
				placeAgainDuration := time.Duration(stampedOrder.PlacedCount) * placeAgainTimeIncrement
				deleteTime := stampedOrder.TimeStamp.Add(placeAgainDuration)

				if time.Now().After(deleteTime) {
					fmt.Println("[network]: delete old order")
					delete(allOrders, orderID)
				}
				//case ACKWAIT_TAKE:
				//	if time.Since(stampedOrder.TimeStamp) > ackwaitTimeout {
				//		fmt.Printf("[timeout]: take ack for %v\n", orderID)
				//		msg := msgs.TakeOrderMsg{SenderID: thisID, ReceiverID: thisID,
				//			Order: allOrders[orderID].OrderMsg.Order}

				//		thisTakeOrderCh.Send <- msg
				//		allOrders[orderID] = createStampedOrder(stampedOrder.OrderMsg.Order, SERVING)
				//	}
			}

			for orderID, stampedOrder := range allOrders {
				// check if order should be given up on
				switch stampedOrder.OrderState {
				default:
					if time.Since(stampedOrder.TimeStamp) > otherGiveupTime {
						fmt.Printf("[timeout]: complete not recieved for %v\n", orderID)

						msg := msgs.TakeOrderMsg{SenderID: thisID,
							ReceiverID: thisID,
							Order:      allOrders[orderID].OrderMsg.Order}

						thisTakeOrderCh.Send <- msg
						allOrders[orderID] = createStampedOrder(stampedOrder.OrderMsg.Order, SERVING)
					}
				}
			}
		}
	}
}