package main

import (
	"./msgs"
	"./network"
	"bufio"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"
)

var id_ptr = flag.String("id", "noid", "ID for node")

var rnd = rand.New(rand.NewSource(time.Now().UnixNano()))

func main() {
	flag.Parse()

	// fsm
	thisElevatorStatusCh := make(chan msgs.ElevatorStatus)

	// OrderHandler channels
	otherElevatorsStatusCh := make(chan []msgs.ElevatorStatus)
	downedElevatorsCh := make(chan []msgs.Heartbeat)
	placedOrderCh := make(chan msgs.Order)
	thisTakeOrderCh := make(chan msgs.TakeOrderMsg)
	otherTakeOrderCh := make(chan msgs.TakeOrderMsg)
	safeOrderCh := make(chan msgs.SafeOrderMsg)
	completedOrderCh := make(chan msgs.Order)

	go network.Launch(*id_ptr,
		thisElevatorStatusCh, otherElevatorsStatusCh, downedElevatorsCh,
		placedOrderCh, thisTakeOrderCh, otherTakeOrderCh,
		safeOrderCh, completedOrderCh)

	go network.PseudoOrderHandlerAndFsm(*id_ptr,
		thisElevatorStatusCh, otherElevatorsStatusCh, downedElevatorsCh,
		placedOrderCh, thisTakeOrderCh, otherTakeOrderCh,
		safeOrderCh, completedOrderCh)

	fmt.Println("Listening")
	// block forever
	reader := bufio.NewReader(os.Stdin)
	commands := map[string][]string{"place": []string{"f", "d", "t"}}

	getIntInput := func() (num int, ok bool) {
		input, err := reader.ReadString('\n')
		if err != nil {
			fmt.Fprintf(os.Stderr, fmt.Sprintf("%v", err))
			return 100, false
		}

		input = strings.TrimSuffix(input, "\n")
		num, err = strconv.Atoi(input)
		if err != nil {
			fmt.Fprintf(os.Stderr, fmt.Sprintf("%v", err))
			return 1000, false
		}
		return num, true
	}

	for {
		// simple command line interface

		//var options map[string]string
		input, _ := reader.ReadString('\n')
		fields := strings.Fields(input)
		if len(fields) == 0 {
			continue
		}
		cmd := fields[0]
		if _, valid := commands[cmd]; !valid {
			fmt.Println("invalid cmd")
			continue
		}

		switch cmd {
		case "place":
			options := commands[cmd]
			fmt.Println("Options: ", options)

			fmt.Print("f: ")
			var f int
			ok := false
			if f, ok = getIntInput(); !ok {
				fmt.Println("invalid floor", f)
				continue
			}

			fmt.Print("d (-1=down, 1=up): ")
			var d int = 1000
			if d, ok = getIntInput(); !ok {
				fmt.Println("invalid direction input", d)
				continue
			}

			var dir msgs.Direction
			switch d {
			case -1:
				dir = msgs.Down
			case 1:
				dir = msgs.Up
			default:
				fmt.Println("invalid direction", d)
				continue
			}

			id := rnd.Intn(1000000)
			fmt.Printf("i (%v): ", id)
			if idTemp, ok := getIntInput(); ok {
				id = idTemp
			}

			order := msgs.Order{ID: id, Floor: f, Direction: dir}
			fmt.Printf("Order: %+v\n", order)
			placedOrderCh <- order
		}
	}
}
