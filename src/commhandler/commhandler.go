package commhandler

import (
	"../comm/bcast"
	"../comm/peers"
	"../go-nonblockingchan"
	"../msgs"
	"log"
	"os"
	"sync"
	"time"
)

var Info *log.Logger

type OrderState int

const (
	ACKWAIT_PLACED   OrderState = iota // this elevator is waiting for an elevator to acknowledge a placed order
	SAFE                               // order has been seen by more than one elevator
	ACKWAIT_TAKE                       // this elevator is waiting for an elevator to acknowledge that it will take the order
	SERVING                            // order is being served by some elevator
	ACKWAIT_COMPLETE                   // order has been completed by this elevator and elevator is waiting for completed_ack from order master
)

func (s OrderState) String() string {
	switch s {
	case ACKWAIT_PLACED:
		return "ACKWAIT_PLACED"
	case SAFE:
		return "SAFE"
	case ACKWAIT_TAKE:
		return "ACKWAIT_TAKE"
	case SERVING:
		return "SERVING"
	case ACKWAIT_COMPLETE:
		return "ACKWAIT_COMPLETE"
	default:
		return "someorderstate"
	}
}

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

const ackwaitTimeout = 100 * time.Millisecond
const placeAgainTimeIncrement = 10 * time.Second
const giveupOtherElevTimeout = 40 * time.Second
const timeoutCheckMaxPeriod = 100 * time.Millisecond
const retransmitCountMax = 5       // number of times to retransmit if no ack is recieved
const placedGiveupAndTakeTries = 3 // if no acks are recieved and user tries this many times, take order


func checkAndRetransmit(allOrders map[int]*StampedOrder, orderID int, thisID string,
	placedOrderSend_bcastCh chan<- msgs.PlacedOrderMsg, takeOrderSend_bcastCh chan<- msgs.TakeOrderMsg, completeOrderSend_bcastCh chan<- msgs.CompleteOrderMsg,
	takeOrder_orderhandlerCh *nbc.NonBlockingChan, redundantOrder_orderhandlerCh *nbc.NonBlockingChan) {

	if stampedOrder, exists := allOrders[orderID]; !exists {
		Info.Printf("check and retransmit for non-existent order\n")
	} else {
		retransmitDuration := time.Duration(stampedOrder.TransmitCount) * ackwaitTimeout
		timeoutTime := stampedOrder.TimeStamp.Add(retransmitDuration)
		if time.Now().After(timeoutTime) {
			// Retransmit order
			if stampedOrder.TransmitCount <= retransmitCountMax {
				stampedOrder.TransmitCount += 1
				switch stampedOrder.OrderState {
				case ACKWAIT_PLACED:
					Info.Printf("retransmitting place for %v for time %v\n", stampedOrder.OrderMsg.Order.ID, stampedOrder.TransmitCount)
					placedOrderSend_bcastCh <- msgs.PlacedOrderMsg{SenderID: thisID,
						Order: stampedOrder.OrderMsg.Order}
				case ACKWAIT_TAKE:
					Info.Printf("retransmitting take for %v time %v\n", stampedOrder.OrderMsg.Order.ID, stampedOrder.TransmitCount)
					takeOrderSend_bcastCh <- msgs.TakeOrderMsg{SenderID: thisID,
						ReceiverID: stampedOrder.OrderMsg.ReceiverID,
						Order:      stampedOrder.OrderMsg.Order}
				case ACKWAIT_COMPLETE:
					Info.Printf("retransmitting complete for order %+v time %v\n", stampedOrder.OrderMsg.Order.ID, stampedOrder.TransmitCount)
					orderMasterID := stampedOrder.OrderMsg.SenderID
					completeOrderSend_bcastCh <- msgs.CompleteOrderMsg{SenderID: thisID, ReceiverID: orderMasterID,
						Order: stampedOrder.OrderMsg.Order}
				case SERVING:
				case SAFE:
				default:
					Info.Printf("no retransmission set up for this order state: %v\n", stampedOrder.OrderState)
				}
			} else {
				// "Give-up actions"
				switch stampedOrder.OrderState {
				case ACKWAIT_PLACED:
					if stampedOrder.PlacedCount >= placedGiveupAndTakeTries {
						Info.Printf("%v retransmit (ackplaced) failed %v times\n", orderID, stampedOrder.PlacedCount)

						redundantOrder_orderhandlerCh.Send <- msgs.RedundantOrderMsg{SenderID: thisID,
							ReceiverID: thisID,
							Order:      stampedOrder.OrderMsg.Order}

						allOrders[orderID] = createStampedOrder(stampedOrder.OrderMsg.Order, SERVING)
					}

				case ACKWAIT_TAKE:
					Info.Printf("%v retransmit (acktake) failed. Take it\n", orderID)
					takeOrder_orderhandlerCh.Send <- msgs.TakeOrderMsg{SenderID: thisID,
						ReceiverID: thisID,
						Order:      stampedOrder.OrderMsg.Order}

					allOrders[orderID] = createStampedOrder(stampedOrder.OrderMsg.Order, SERVING)
				case ACKWAIT_COMPLETE:
					Info.Printf("%v retransmit (ackcomplete) failed. Order deleted.\n", orderID)
					delete(allOrders, stampedOrder.OrderMsg.Order.ID)

				}
			}
		}
	}
}

func CommHandler(thisID string, commonPort int,
	/* read */
	thisElevatorHeartbeat_orderhandlerCh *nbc.NonBlockingChan,
	downedElevators_orderhandlerCh *nbc.NonBlockingChan,
	placedOrder_orderhandlerCh *nbc.NonBlockingChan,
	assignOrder_orderhandlerCh *nbc.NonBlockingChan,
	completedOrder_orderhandlerCh *nbc.NonBlockingChan,
	/* write */
	allElevatorsHeartbeat_orderhandlerCh *nbc.NonBlockingChan,
	takeOrder_orderhandlerCh *nbc.NonBlockingChan,
	redundantOrder_orderhandlerCh *nbc.NonBlockingChan,
	completedHallOrderOtherElev_orderhandlerCh *nbc.NonBlockingChan,
	/* sync */
	wg *sync.WaitGroup) {

	Info = log.New(os.Stdout, "[network]: ", 0)

	placedOrderSend_bcastCh := make(chan msgs.PlacedOrderMsg)
	placedOrderAckSend_bcastCh := make(chan msgs.PlacedOrderAck)
	takeOrderSend_bcastCh := make(chan msgs.TakeOrderMsg)
	takeOrderAckSend_bcastCh := make(chan msgs.TakeOrderAck)
	completeOrderSend_bcastCh := make(chan msgs.CompleteOrderMsg)
	completeOrderAckSend_bcastCh := make(chan msgs.CompleteOrderAck)
	go bcast.Transmitter(commonPort,
		placedOrderSend_bcastCh, placedOrderAckSend_bcastCh,
		takeOrderAckSend_bcastCh, takeOrderSend_bcastCh,
		completeOrderSend_bcastCh, completeOrderAckSend_bcastCh)

	placedOrderRecv_bcastCh := make(chan msgs.PlacedOrderMsg)
	placedOrderAckRecv_bcastCh := make(chan msgs.PlacedOrderAck)
	takeOrderRecv_bcastCh := make(chan msgs.TakeOrderMsg)
	takeOrderAckRecv_bcastCh := make(chan msgs.TakeOrderAck)
	completeOrderRecv_bcastCh := make(chan msgs.CompleteOrderMsg)
	completeOrderAckRecv_bcastCh := make(chan msgs.CompleteOrderAck)
	go bcast.Receiver(commonPort,
		placedOrderRecv_bcastCh, placedOrderAckRecv_bcastCh,
		takeOrderAckRecv_bcastCh, takeOrderRecv_bcastCh,
		completeOrderRecv_bcastCh, completeOrderAckRecv_bcastCh)

	txEnable_peerCh := make(chan bool)
	updateHeartbeat_peerCh := make(chan msgs.Heartbeat)
	go peers.Transmitter(commonPort, txEnable_peerCh, updateHeartbeat_peerCh)

	updates_peerCh := make(chan peers.PeerUpdate, 1)
	go peers.Receiver(commonPort, updates_peerCh)

	allOrders := make(map[int]*StampedOrder)

	// Wait until all modules are initialized
	wg.Done()
	Info.Println("initialized")
	wg.Wait()
	Info.Println("starting")

	for {
		select {
		case msg := <-placedOrderRecv_bcastCh:
			if msg.SenderID != thisID { // Order transmitted from other node
				allOrders[msg.Order.ID] = createStampedOrder(msg.Order, SAFE)
				allOrders[msg.Order.ID].OrderMsg.SenderID = msg.SenderID

				// acknowledge order
				ack := msgs.PlacedOrderAck{SenderID: thisID,
					ReceiverID: msg.SenderID,
					Order:      msg.Order}
				placedOrderAckSend_bcastCh <- ack
				Info.Printf("sent ack to %v for order %v\n", ack.ReceiverID, ack.Order.ID)
			}

		case msg, _ := <-placedOrder_orderhandlerCh.Recv:
			order := msg.(msgs.Order)

			if orderStamped, exists := allOrders[order.ID]; exists && orderStamped.OrderState == ACKWAIT_PLACED {
				Info.Printf("unacked order %v placed again %v\n", orderStamped.OrderMsg.Order.ID, orderStamped.PlacedCount)
				orderStamped.TimeStamp = time.Now()
				orderStamped.TransmitCount = 1
				orderStamped.PlacedCount += 1
			} else {
				allOrders[order.ID] = createStampedOrder(order, ACKWAIT_PLACED)
				allOrders[order.ID].OrderMsg.SenderID = thisID
			}
			placedOrderSend_bcastCh <- msgs.PlacedOrderMsg{SenderID: thisID, Order: order}

		case msg := <-placedOrderAckRecv_bcastCh:
			if msg.ReceiverID == thisID {
				// acknowledgement recieved from other node
				if _, exists := allOrders[msg.Order.ID]; !exists {
					Info.Printf("order %v not found\n", msg.Order.ID)
					break
				}
				if orderStamped, _ := allOrders[msg.Order.ID]; orderStamped.OrderState != ACKWAIT_PLACED {
					Info.Printf("not awaiting place ack for order %v\n", msg.Order.ID)
					break
				}

				Info.Printf("order %v acknowledged\n", msg.Order.ID)
				allOrders[msg.Order.ID].OrderState = SAFE

				// order is redundant since multiple elevators know about it, notify orderHandler
				redundantMsg := msgs.RedundantOrderMsg{SenderID: thisID, ReceiverID: thisID, Order: msg.Order}
				redundantOrder_orderhandlerCh.Send <- redundantMsg
			}

		case msg, _ := <-assignOrder_orderhandlerCh.Recv:
			orderMsg := msg.(msgs.TakeOrderMsg)

			orderMsg.SenderID = thisID
			takeOrderSend_bcastCh <- orderMsg

			Info.Printf("elevator %v should take %v\n", orderMsg.ReceiverID, orderMsg.Order.ID)

			allOrders[orderMsg.Order.ID] = createStampedOrder(orderMsg.Order, ACKWAIT_TAKE)
			allOrders[orderMsg.Order.ID].OrderMsg.ReceiverID = orderMsg.ReceiverID

		case msg := <-takeOrderRecv_bcastCh:

			if msg.ReceiverID == thisID {
				allOrders[msg.Order.ID] = createStampedOrder(msg.Order, SERVING)
				allOrders[msg.Order.ID].OrderMsg.SenderID = msg.SenderID

				Info.Printf("this elevator takes order %v\n", msg.Order.ID)
				takeOrder_orderhandlerCh.Send <- msg

				ack := msgs.TakeOrderAck{SenderID: thisID, ReceiverID: msg.SenderID, Order: msg.Order}

				takeOrderAckSend_bcastCh <- ack
			}

		case msg := <-takeOrderAckRecv_bcastCh:
			if msg.ReceiverID == thisID {
				Info.Printf("recieved take ack for order %+v from %v\n", msg.Order, msg.SenderID)
			}

			allOrders[msg.Order.ID] = createStampedOrder(msg.Order, SERVING)

		case peerUpdate := <-updates_peerCh:
			if len(peerUpdate.Lost) > 0 {
				var downedElevators []msgs.Heartbeat
				for _, lastHeartbeat := range peerUpdate.Lost {
					Info.Printf("lost %v\n", lastHeartbeat.SenderID)
					downedElevators = append(downedElevators, lastHeartbeat)
				}
				downedElevators_orderhandlerCh.Send <- downedElevators
			}

			if len(peerUpdate.New) > 0 {
				Info.Println("new peer: ", peerUpdate.New)
				// for loop over completedOnBehalf
				// if a new elevator == order.MasterID
					// send completed order
					// 
			}

			allElevatorsHeartbeat_orderhandlerCh.Send <- peerUpdate.Peers

		case msg, _ := <-completedOrder_orderhandlerCh.Recv:
			order := msg.(msgs.Order)

			if _, exists := allOrders[order.ID]; exists {
				Info.Printf("order %v completed by this elevator\n", order)
				completeOrderSend_bcastCh <- msgs.CompleteOrderMsg{SenderID: thisID,
					Order: order}
				allOrders[order.ID] = createStampedOrder(order, ACKWAIT_COMPLETE)
				allOrders[order.ID].OrderMsg.SenderID = thisID
			}

		case msg := <-completeOrderRecv_bcastCh:

			if msg.SenderID != thisID {
				//if msg.Order.MasterID == thisID {
					completeOrderAckSend_bcastCh <- msgs.CompleteOrderAck{SenderID: thisID,
						ReceiverID: msg.SenderID,
						Order:      msg.Order}
				//}
				

				Info.Printf("order %v completed by %v\n", msg.Order, msg.SenderID)
				completedHallOrderOtherElev_orderhandlerCh.Send <- msg.Order
				//if master is in online{
					delete(allOrders, msg.Order.ID)
				//} else {
				//		append(completedOnBehalfOfOther, order)
				//}
			}

		case msg := <-completeOrderAckRecv_bcastCh:

			if msg.SenderID != thisID {
				if stampedOrder, exists := allOrders[msg.Order.ID]; exists {
					if stampedOrder.OrderState == ACKWAIT_COMPLETE {
						Info.Printf("complete order ack for %v from %v\n", msg.Order, msg.SenderID)
						delete(allOrders, msg.Order.ID)
					}
				}
			}

		case msg, _ := <-thisElevatorHeartbeat_orderhandlerCh.Recv:
			heartbeat := msg.(msgs.Heartbeat)

			heartbeat.SenderID = thisID
			updateHeartbeat_peerCh <- heartbeat


		case <- time.After(timeoutCheckMaxPeriod):
			// guarantees that the statements below are run sufficiently often.
		}

		// actions that happen on every update
		for orderID, stampedOrder := range allOrders {
			// retransmission if necessary
			checkAndRetransmit(allOrders, orderID, thisID,
				placedOrderSend_bcastCh, takeOrderSend_bcastCh, completeOrderSend_bcastCh,
				takeOrder_orderhandlerCh, redundantOrder_orderhandlerCh)
			placeAgainDuration := time.Duration(stampedOrder.PlacedCount) * placeAgainTimeIncrement
			deleteTime := stampedOrder.TimeStamp.Add(placeAgainDuration)

			if stampedOrder.OrderState == ACKWAIT_PLACED && time.Now().After(deleteTime) {
				Info.Printf("delete old order: %v\n", orderID)
				delete(allOrders, orderID)
			}
		}

		for orderID, stampedOrder := range allOrders {
			if time.Since(stampedOrder.TimeStamp) > giveupOtherElevTimeout {
				Info.Printf("complete not recieved for %v\n", orderID)

				msg := msgs.TakeOrderMsg{SenderID: thisID,
					ReceiverID: thisID,
					Order:      allOrders[orderID].OrderMsg.Order}

				takeOrder_orderhandlerCh.Send <- msg
				allOrders[orderID] = createStampedOrder(stampedOrder.OrderMsg.Order, SERVING)
			}
		}
	}
}
