package main

import (
	"./commhandler"
	"./fsm"
	"./go-nonblockingchan"
	"./orderhandler"
	"flag"
	"fmt"
	"os"
	"sync"
)

var id_ptr = flag.String("id", "noid", "ID for node")
var elevServerAddr_ptr = flag.String("addr", "localhost:15657", "Port for node")
var commonPort_ptr = flag.Int("bport", 20010, "Port for all broadcasts")

var wg sync.WaitGroup

const N_FLOORS = 4 //import
const N_BUTTONS = 3

func main() {
	flag.Parse()

	if *id_ptr == "noid" {
		fmt.Println("Specify id")
		os.Exit(1)
	}

	// Three modules in wait group
	wg.Add(3)

	// Channels: FSM -> OrderHandler
	elevatorStatusCh := nbc.New()         //make(chan fsm.Elevator)
	placedHallOrderCh := nbc.New()        //make(chan fsm.OrderEvent)
	completedHallOrdersThisElevCh := nbc.New() //make(chan []fsm.OrderEvent)

	// Channels: OrderHandler -> FSM
	addHallOrderCh := nbc.New()    //make(chan fsm.OrderEvent)
	deleteHallOrderCh := nbc.New() //make(chan fsm.OrderEvent)
	updateLightsCh := nbc.New()    //make(chan [N_FLOORS][N_BUTTONS]bool)

	// Channels: OrderHandler -> Network
	broadcastTakeOrderCh := nbc.New()    //make(chan msgs.TakeOrderMsg)
	placedOrderCh := nbc.New()           //make(chan msgs.Order)
	completedOrderCh := nbc.New()        //make(chan msgs.Order)
	thisElevatorHeartbeatCh := nbc.New() //make(chan msgs.Heartbeat)

	// Channels: Network -> OrderHandler
	allElevatorsHeartbeatCh := nbc.New()   //make(chan []msgs.Heartbeat)
	safeOrderCh := nbc.New()               //make(chan msgs.SafeOrderMsg)
	thisTakeOrderCh := nbc.New()           //make(chan msgs.TakeOrderMsg)
	downedElevatorsCh := nbc.New()         //make(chan []msgs.Heartbeat)
	completedOrderOtherElevCh := nbc.New() //make(chan msgs.Order)

	// Channels: Network -> FSM
	// (none)

	// FSM -> Network
	// (none)

	go commhandler.Launch(*id_ptr, *commonPort_ptr,
		thisElevatorHeartbeatCh, downedElevatorsCh, placedOrderCh,
		broadcastTakeOrderCh, completedOrderCh,
		allElevatorsHeartbeatCh, thisTakeOrderCh, safeOrderCh,
		completedOrderOtherElevCh, &wg)

	go orderhandler.OrderHandler(*id_ptr,
		elevatorStatusCh, allElevatorsHeartbeatCh, placedHallOrderCh,
		safeOrderCh, thisTakeOrderCh, downedElevatorsCh,
		completedHallOrdersThisElevCh, completedOrderOtherElevCh,
		addHallOrderCh, broadcastTakeOrderCh, placedOrderCh, deleteHallOrderCh,
		completedOrderCh, thisElevatorHeartbeatCh, updateLightsCh, &wg)

	go fsm.FSM(*elevServerAddr_ptr, addHallOrderCh, deleteHallOrderCh,
		updateLightsCh, placedHallOrderCh, completedHallOrdersThisElevCh,
		elevatorStatusCh, &wg)

	for {
		select {}
	}
}
