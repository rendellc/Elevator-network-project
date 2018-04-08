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
var orderID_ptr = flag.Int("oid", rnd.Intn(10000), "ID for order")
var floor_ptr = flag.Int("floor", -1, "Floor")
var direction_ptr = (*msgs.Direction)(flag.Int("dir", int(msgs.Up), fmt.Sprintf("Direction of order: %v for up, %v for down", msgs.Up, msgs.Down)))
var orderType_ptr = (*msgs.OrderType)(flag.String("type", string(msgs.HallCall), fmt.Sprintf("Type of order: %v for CabCall, %v for HallCall", msgs.CabCall, msgs.HallCall)))

func main() {
	flag.Parse()

	if *port_ptr < 1024 {
		fmt.Println(fmt.Errorf("Port must be greater than 1024, not %v", *port_ptr))
		return
	}

	if *orderType_ptr != msgs.HallCall {
		fmt.Println(fmt.Errorf("Order type \"%v\" not implemented", *orderType_ptr))
		return
	}

	fmt.Println("Order sent ->")
	fmt.Printf("\tPort: -port=%v\n", *port_ptr)
	fmt.Printf("\televatorID: -rid=%v\n", *elevatorID_ptr)
	fmt.Printf("\tOrderID: -oid=%v\n", *orderID_ptr)
	fmt.Printf("\tFloor: -floor=%+v\n", *floor_ptr)
	fmt.Printf("\tDirection: -dir=%+v\n", *direction_ptr)
	fmt.Printf("\tOrderType: -type=%+v\n", *orderType_ptr)

	placeOrderCh := make(chan msgs.Debug_placeOrderMsg)

	go bcast.Transmitter(*port_ptr, placeOrderCh)

	msg := msgs.Debug_placeOrderMsg{RecieverID: *elevatorID_ptr,
		Order: msgs.Order{ID: *orderID_ptr, Floor: *floor_ptr, Direction: *direction_ptr}}

	placeOrderCh <- msg

	time.Sleep(1 * time.Second)
}
