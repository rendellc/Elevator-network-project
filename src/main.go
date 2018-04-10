package main

import (
	"./msgs"
	"./network"
	"flag"
	"fmt"
	"math/rand"
	"time"
)

var id_ptr = flag.String("id", "noid", "ID for node")

var rnd = rand.New(rand.NewSource(time.Now().UnixNano()))

func main() {
	flag.Parse()

	// OrderHandler channels
	thisElevatorStatusCh := make(chan msgs.Heartbeat)
	allElevatorsHeartbeatCh := make(chan []msgs.Heartbeat)
	downedElevatorsCh := make(chan []msgs.Heartbeat)
	placedOrderCh := make(chan msgs.Order)
	thisTakeOrderCh := make(chan msgs.TakeOrderMsg)
	otherTakeOrderCh := make(chan msgs.TakeOrderMsg)
	safeOrderCh := make(chan msgs.SafeOrderMsg)
	completedOrderCh := make(chan msgs.Order)

	go network.Launch(*id_ptr,
		thisElevatorStatusCh, allElevatorsHeartbeatCh, downedElevatorsCh,
		placedOrderCh, thisTakeOrderCh, otherTakeOrderCh,
		safeOrderCh, completedOrderCh)

	go network.PseudoOrderHandlerAndFsm(*id_ptr,
		thisElevatorStatusCh, allElevatorsHeartbeatCh, downedElevatorsCh,
		placedOrderCh, thisTakeOrderCh, otherTakeOrderCh,
		safeOrderCh, completedOrderCh)

	fmt.Println("Listening")
	// block forever
	//reader := bufio.NewReader(os.Stdin)
	//commands := map[string][]string{"place": []string{"f", "d", "t"}}

	//getIntInput := func() (num int, ok bool) {
	//	input, err := reader.ReadString('\n')
	//	if err != nil {
	//		fmt.Fprintf(os.Stderr, fmt.Sprintf("%v", err))
	//		return 100, false
	//	}

	//	input = strings.TrimSuffix(input, "\n")
	//	num, err = strconv.Atoi(input)
	//	if err != nil {
	//		fmt.Fprintf(os.Stderr, fmt.Sprintf("%v", err))
	//		return 1000, false
	//	}
	//	return num, true
	//}

	for {
		select {}
	}
}
