package main

import (
	//"./comm/bcast"
	"./comm/bcast"
	"./msgs"
	"flag"
	"fmt"
	"math/rand"
	"time"
)

var rnd = rand.New(rand.NewSource(time.Now().UnixNano()))

// Flags
var port_ptr = flag.Int("port", -1, "port for broadcast")
var elevatorID_ptr = flag.String("rid", "noid", "ID for elevator")
var orderID_ptr = flag.Int("oid", -1, "ID for order")

func main() {
	flag.Parse()

	if *port_ptr < 1024 {
		fmt.Println(fmt.Errorf("Port must be greater than 1024, not %v", *port_ptr))
		return
	}

	if *elevatorID_ptr == "noid" {
		fmt.Println(fmt.Errorf("ElevatorID must be specified (-rid)"))
		return
	}

	if *orderID_ptr == -1 {
		fmt.Println(fmt.Errorf("OrderID must be specified (-oid)"))
		return
	}

	fmt.Println("->")
	fmt.Printf("\tPort: -port=%v\n", *port_ptr)
	fmt.Printf("\televatorID: -rid=%v\n", *elevatorID_ptr)
	fmt.Printf("\tOrderID: -oid=%v\n", *orderID_ptr)

	acceptOrderCh := make(chan msgs.Debug_acceptOrderMsg)

	go bcast.Transmitter(*port_ptr, acceptOrderCh)

	msg := msgs.Debug_acceptOrderMsg{RecieverID: *elevatorID_ptr, OrderID: *orderID_ptr}

	acceptOrderCh <- msg

	time.Sleep(1 * time.Second)
}
