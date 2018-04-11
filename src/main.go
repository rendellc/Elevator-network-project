package main

import (
	"./msgs"
	"./network"
	"flag"
	"math/rand"
	"time"
)

var id_ptr = flag.String("id", "noid", "ID for node")
var simAddr_ptr = flag.String("addr", "noid", "Port for node")

var rnd = rand.New(rand.NewSource(time.Now().UnixNano()))

const N_FLOORS = 4 //import
const N_BUTTONS = 3

func main() {
	flag.Parse()

	// OrderHandler channels
	thisElevatorHeartbeatCh := make(chan msgs.Heartbeat)
	allElevatorsHeartbeatCh := make(chan []msgs.Heartbeat)
	downedElevatorsCh := make(chan []msgs.Heartbeat)
	placedOrderCh := make(chan msgs.Order)
	thisTakeOrderCh := make(chan msgs.TakeOrderMsg)
	otherTakeOrderCh := make(chan msgs.TakeOrderMsg)
	safeOrderCh := make(chan msgs.SafeOrderMsg)
	completedOrderCh := make(chan msgs.Order)
	//turnOnLightsCh := make(chan [N_FLOORS][N_BUTTONS]bool)

	go network.Launch(*id_ptr,
		thisElevatorHeartbeatCh, allElevatorsHeartbeatCh, downedElevatorsCh,
		placedOrderCh, thisTakeOrderCh, otherTakeOrderCh,
		safeOrderCh, completedOrderCh)

	go network.PseudoOrderHandlerAndFsm(*id_ptr, *simAddr_ptr,
		thisElevatorHeartbeatCh, allElevatorsHeartbeatCh, downedElevatorsCh,
		placedOrderCh, thisTakeOrderCh, otherTakeOrderCh,
		safeOrderCh, completedOrderCh) //, turnOnLightsCh)

	for {
		select {}
	}
}
