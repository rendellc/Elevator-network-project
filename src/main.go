package main

import (
	"./msgs"
	"./network"
	"flag"
	"fmt"
)

var id_ptr = flag.String("id", "noid", "ID for node")

func main() {
	flag.Parse()

	// fsm
	thisElevatorStatusCh := make(chan msgs.ElevatorStatus)

	// OrderHandler channels
	otherElevatorsStatusCh := make(chan []msgs.ElevatorStatus)
	downedElevatorsCh := make(chan []msgs.Heartbeat)
	thisTakeOrderCh := make(chan msgs.TakeOrderMsg)
	otherTakeOrderCh := make(chan msgs.TakeOrderMsg)
	safeOrderCh := make(chan msgs.SafeOrderMsg)
	completedOrderCh := make(chan msgs.Order)

	go network.Launch(*id_ptr,
		thisElevatorStatusCh, otherElevatorsStatusCh, downedElevatorsCh,
		thisTakeOrderCh, otherTakeOrderCh,
		safeOrderCh, completedOrderCh)

	go network.PseudoOrderHandlerAndFsm(*id_ptr,
		thisElevatorStatusCh, otherElevatorsStatusCh, downedElevatorsCh,
		thisTakeOrderCh, otherTakeOrderCh,
		safeOrderCh, completedOrderCh)

	fmt.Println("Listening")
	// block forever
	for {
		select {} // minimizes cpu usage
	}

}
