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
	ACKWAIT_PLACED   OrderState = iota // this elevator is waiting for an elevator to acknowledge a placed order
	SAFE                               // order has been seen by more than one elevator
	ACKWAIT_TAKE                       // this elevator is waiting for an elevator to acknowledge that it will take the order
	SERVING                            // order is being served by some elevator
	ACKWAIT_COMPLETE                   // order has been completed by this elevator and elevator is waiting for completed_ack from order master
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

const ackwaitTimeout = 500 * time.Millisecond
const placeAgainTimeIncrement = 10 * time.Second
const otherGiveupTime = 40 * time.Second
const retransmitCountMax = 5       // number of times to retransmit if no ack is recieved
const placedGiveupAndTakeTries = 3 // if no acks are recieved and user tries this many times, take order

func checkAndRetransmit(allOrders map[int]*StampedOrder, orderID int, thisID string,
	placedOrderSendCh chan<- msgs.PlacedOrderMsg, takeOrderSendCh chan<- msgs.TakeOrderMsg, completeOrderSendCh chan<- msgs.CompleteOrderMsg,
	safeOrderCh *nbc.NonBlockingChan) {

	if stampedOrder, exists := allOrders[orderID]; !exists {
		fmt.Printf("[network]: check and retransmit for non-existent order\n")
	} else {
		retransmitDuration := time.Duration(stampedOrder.TransmitCount) * ackwaitTimeout
		timeoutTime := stampedOrder.TimeStamp.Add(retransmitDuration)
		if time.Now().After(timeoutTime) {
			// Retransmit order
			if stampedOrder.TransmitCount <= retransmitCountMax {
				//fmt.Printf("[network]: retransmit order %v, : %+v\n", orderID, allOrders[orderID])

				stampedOrder.TransmitCount += 1
				switch stampedOrder.OrderState {
				case ACKWAIT_PLACED:
					fmt.Printf("[network]: retransmitting place for %v for time %v\n", stampedOrder.OrderMsg.Order.ID, stampedOrder.TransmitCount)
					placedOrderSendCh <- msgs.PlacedOrderMsg{SenderID: thisID,
						Order: stampedOrder.OrderMsg.Order}
				case ACKWAIT_TAKE:
					fmt.Printf("[network]: retransmitting take for %v time %v\n", stampedOrder.OrderMsg.Order.ID, stampedOrder.TransmitCount)
					takeOrderSendCh <- msgs.TakeOrderMsg{SenderID: thisID,
						ReceiverID: stampedOrder.OrderMsg.ReceiverID,
						Order:      stampedOrder.OrderMsg.Order}
				case ACKWAIT_COMPLETE:
					fmt.Printf("[network]: retransmitting complete for order %+v time %v\n", stampedOrder.OrderMsg.Order.ID, stampedOrder.TransmitCount)

					completeOrderSendCh <- msgs.CompleteOrderMsg(stampedOrder.OrderMsg)
				default:
					fmt.Printf("[network]: no retransmission set up for this order state: %v\n", stampedOrder.OrderState)
				}
			} else {
				// "Give-up actions"
				switch stampedOrder.OrderState {
				case ACKWAIT_PLACED:
					if stampedOrder.PlacedCount >= placedGiveupAndTakeTries {
						fmt.Printf("[network]: %v retransmit failed %v times\n", orderID, stampedOrder.PlacedCount)
					}
				case ACKWAIT_TAKE:
					safeOrderCh.Send <- msgs.SafeOrderMsg{SenderID: thisID,
						ReceiverID: thisID,
						Order:      stampedOrder.OrderMsg.Order}

					allOrders[orderID] = createStampedOrder(stampedOrder.OrderMsg.Order, SERVING)
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
	takeOrderSendCh := make(chan msgs.TakeOrderMsg)
	takeOrderAckSendCh := make(chan msgs.TakeOrderAck)
	completeOrderSendCh := make(chan msgs.CompleteOrderMsg)
	completeOrderAckSendCh := make(chan msgs.CompleteOrderAck)
	go bcast.Transmitter(commonPort, placedOrderSendCh, placedOrderAckSendCh, takeOrderAckSendCh, takeOrderSendCh, completeOrderSendCh, completeOrderAckSendCh)

	placedOrderRecvCh := make(chan msgs.PlacedOrderMsg)
	placedOrderAckRecvCh := make(chan msgs.PlacedOrderAck)
	takeOrderRecvCh := make(chan msgs.TakeOrderMsg)
	takeOrderAckRecvCh := make(chan msgs.TakeOrderAck)
	completeOrderRecvCh := make(chan msgs.CompleteOrderMsg)
	completeOrderAckRecvCh := make(chan msgs.CompleteOrderAck)
	go bcast.Receiver(commonPort, placedOrderRecvCh, placedOrderAckRecvCh, takeOrderAckRecvCh, takeOrderRecvCh, completeOrderRecvCh, completeOrderAckRecvCh)

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
				allOrders[msg.Order.ID].OrderMsg.SenderID = msg.SenderID

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
					fmt.Printf("[network]: unacked order placed again: %v\n", orderStamped.OrderMsg.Order.ID)
					orderStamped.TransmitCount = 1
					orderStamped.PlacedCount += 1
				}
			} else {
				//fmt.Println("[network]: new order in ACKWAIT_PLACED")
				allOrders[order.ID] = createStampedOrder(order, ACKWAIT_PLACED)
			}

			placedOrderSendCh <- msgs.PlacedOrderMsg{SenderID: thisID, Order: order}

		case msg := <-placedOrderAckRecvCh:
			if msg.ReceiverID == thisID {
				// Acknowledgement recieved from other node
				if _, exists := allOrders[msg.Order.ID]; !exists {
					fmt.Printf("[network]: order %v not found\n", msg.Order.ID)
					break
					// maybe count how often we end up here?
				}
				if orderStamped, _ := allOrders[msg.Order.ID]; orderStamped.OrderState != ACKWAIT_PLACED {
					fmt.Printf("[network]: not awaiting place ack for order %v\n", msg.Order.ID)
					break
				}

				fmt.Printf("[network]: order %v acknowledged\n", msg.Order.ID)
				allOrders[msg.Order.ID].OrderState = SAFE

				// Order is safe since multiple elevators knows about it, notify orderHandler
				safeMsg := msgs.SafeOrderMsg{SenderID: thisID, ReceiverID: thisID, Order: msg.Order}
				safeOrderCh.Send <- safeMsg
			}
		case msg, _ := <-broadcastTakeOrderCh.Recv:
			orderMsg := msg.(msgs.TakeOrderMsg)
			takeOrderSendCh <- orderMsg

			fmt.Printf("[network]: elevator %v should take %v\n", orderMsg.ReceiverID, orderMsg.Order.ID)

			allOrders[orderMsg.Order.ID] = createStampedOrder(orderMsg.Order, ACKWAIT_TAKE)
			allOrders[orderMsg.Order.ID].OrderMsg.ReceiverID = orderMsg.ReceiverID

		case msg := <-takeOrderRecvCh:

			if msg.ReceiverID == thisID {
				allOrders[msg.Order.ID] = createStampedOrder(msg.Order, SERVING)
				fmt.Printf("[network]: this elevator should take order %v\n", msg.Order.ID)
				thisTakeOrderCh.Send <- msg

				ack := msgs.TakeOrderAck{SenderID: thisID, ReceiverID: msg.SenderID, Order: msg.Order}

				takeOrderAckSendCh <- ack
			}

		case msg := <-takeOrderAckRecvCh:
			if msg.ReceiverID == thisID {
				fmt.Printf("[network]: Recieved ack for order %+v\n", msg)
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
			}

			allElevatorsHeartbeatCh.Send <- peerUpdate.Peers

		case msg, _ := <-completedOrderCh.Recv:
			order := msg.(msgs.Order)

			fmt.Printf("[network]: (from orderhandler) completedOrderCh: %v\n", order)
			if _, exists := allOrders[order.ID]; exists {
				completeOrderSendCh <- msgs.CompleteOrderMsg{SenderID: thisID,
					Order: order}
				allOrders[order.ID] = createStampedOrder(order, ACKWAIT_COMPLETE)
				allOrders[order.ID].OrderMsg.SenderID = thisID
			} else {
				fmt.Printf("[network]: complete unknown order %v\n", order.ID)
			}

		case msg := <-completeOrderRecvCh:

			if msg.SenderID != thisID {
				// acknowledge completed order
				completeOrderAckSendCh <- msgs.CompleteOrderAck{SenderID: thisID,
					ReceiverID: msg.SenderID,
					Order:      msg.Order}

				//fmt.Printf("[network]: complete order recv: %+v\n", msg.Order)

				fmt.Printf("[network]: complete forwarded: %v\n", msg.Order)
				completedOrderOtherElevCh.Send <- msg.Order

				delete(allOrders, msg.Order.ID)
			}

		case msg := <-completeOrderAckRecvCh:

			if stampedOrder, exists := allOrders[msg.Order.ID]; exists {
				if stampedOrder.OrderState == ACKWAIT_COMPLETE {
					if msg.SenderID != thisID {
						fmt.Printf("[network]: complete order ack: %v\n", msg.Order)
					}
				} else {
					fmt.Printf("[network]: not expecting complete ack for order %v\n", msg.Order)
				}
			} else {
				fmt.Printf("[network]: order %v not in allOrders\n", msg.Order)
			}

			delete(allOrders, msg.Order.ID)

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
			checkAndRetransmit(allOrders, orderID, thisID, placedOrderSendCh, takeOrderSendCh, completeOrderSendCh, safeOrderCh)
			// TODO: verify that PlacedCount is incremented in the above function call
			placeAgainDuration := time.Duration(stampedOrder.PlacedCount) * placeAgainTimeIncrement
			deleteTime := stampedOrder.TimeStamp.Add(placeAgainDuration)

			if stampedOrder.OrderState == ACKWAIT_PLACED && time.Now().After(deleteTime) {
				fmt.Printf("[network]: delete old order: %v\n", orderID)
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
