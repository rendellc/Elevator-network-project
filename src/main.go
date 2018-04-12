package main

import (
	"./msgs"
	"./network"
	"flag"
	"math/rand"
	"sync"
	"time"
)

var id_ptr = flag.String("id", "noid", "ID for node")
var elevServerAddr_ptr = flag.String("addr", "noid", "Port for node")

var rnd = rand.New(rand.NewSource(time.Now().UnixNano()))

var wg sync.WaitGroup

const N_FLOORS = 4 //import
const N_BUTTONS = 3

func main() {
	flag.Parse()

	// Three modules in wait group
	wg.Add(3)

	// Channels: FSM -> OrderHandler
	elevatorStatusCh := make(chan fsm.Elevator)
	placedHallOrderCh := make(chan msgs.Order)
	completedOrderThisElevCh := make(chan msgs.Order)

	// Channels: OrderHandler -> FSM
	addHallOrderCh := make(chan fsm.OrderEvent)
	deleteHallOrderCh := make(chan fsm.OrderEvent)
	turnOnLightsCh := make(chan [N_FLOORS][N_BUTTONS]bool)

	// Channels: OrderHandler -> Network
	broadcastTakeOrderCh := make(chan msgs.TakeOrderMsg)
	placedOrderCh := make(chan msgs.Order)
	completedOrderCh := make(chan msgs.Order)
	thisElevatorHeartbeatCh := make(chan msgs.Heartbeat)

	// Channels: Network -> OrderHandler
	allElevatorsHeartbeatCh := make(chan msgs.Heartbeat)
	safeOrderCh := make(chan msgs.SafeOrderMsg)
	thisTakeOrderCh := make(chan msgs.TakeOrderMsg)
	downedElevatorsCh := make(chan []msgs.Heartbeat)
	completedOrderOtherElevCh := make(chan msgs.Order)

	// Channels: Network -> FSM
	// (none)

	// FSM -> Network
	// (none)

	go network.Launch(*id_ptr,
		thisElevatorHeartbeatCh, downedElevatorsCh, placedOrderCh,
		broadcastTakeOrderCh, completedOrderCh,
		allElevatorsHeartbeatCh, thisTakeOrderCh, safeOrderCh,
		orderCompletedOtherElevCh,
		&wg)

	go order.OrderHandler(*id_ptr,
		elevatorStatusCh, allElevatorsHeartbeatCh, placedHallOrderCh, safeOrderCh,
		thisTakeOrderCh, downedElevatorsCh, completedOrderThisElevCh,
		completedOrderOtherElevCh,
		addHallOrderCh, broadcastTakeOrderCh, placedOrderCh, deleteHallOrderCh,
		completedOrderCh, thisElevatorHeartbeatCh, turnOnLightsCh,
		&wg)

	go fsm.FSM(elevServerAddr, addHallOrderCh, deleteHallOrderCh,
		placedHallOrderCh, completedHallOrderCh,
		elevatorStatusCh, turnOnLightsCh, &wg)

	for {
		select {}
	}
}
