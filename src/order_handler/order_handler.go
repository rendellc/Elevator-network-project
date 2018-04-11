//package order_handler
package main

import (
  "../elevio"
  "../fsm"
  "fmt"
  "bufio"
  "os"
  "strconv"
)

func simplified_order_controller(addHallOrderCh chan<- fsm.OrderEvent, deleteHallOrderCh chan<-fsm.OrderEvent,
  placedHallOrderCh <-chan fsm.OrderEvent, completedHallOrderCh <-chan []fsm.OrderEvent,
  elevatorStatusCh <-chan fsm.Elevator){
  var floor int
  var buttonType elevio.ButtonType

  ch := make(chan string)
	go func(ch chan string) {
		reader := bufio.NewReader(os.Stdin)
		for {
			s, _ := reader.ReadString('\n')
      s = s[:len(s)-1]
			ch <- s
		}
	}(ch)

	for {
		select {

		case stdin := <-ch:
			fmt.Println("Read input from stdin:", stdin)
      i, err := strconv.Atoi(stdin)
      if err !=nil {
        fmt.Println("Something is wrong: ",err)
      }
      fmt.Println("Read input from stdin:", i)
      floor = i/10
      fmt.Println("floor: ", floor)
      if floor<0 || floor > 3 {
        floor=0
      }
      fmt.Println("floor: ", floor)
      buttonType = elevio.ButtonType(i%3)
      deleteHallOrderCh <- fsm.OrderEvent{floor, buttonType, false}
      // short circuit order controller
    case hallOrder:= <-placedHallOrderCh:
      //fmt.Println("Hall order in transit")
      addHallOrderCh <- fsm.OrderEvent{hallOrder.Floor, hallOrder.Button, true}

    case  <- completedHallOrderCh:
      //fmt.Println("Completed orders: ", a)

    case  <- elevatorStatusCh:
      //fmt.Println("Elevator status: ",a)

    //case <-time.After(1 * time.Second):
     //fmt.Println("The most important thing is not to block others")
    }
  }
}

func main(){

  addHallOrderCh := make(chan fsm.OrderEvent)
	deleteHallOrderCh := make(chan fsm.OrderEvent)
	placedHallOrderCh := make(chan fsm.OrderEvent)
	completedHallOrderCh := make(chan []fsm.OrderEvent)
	elevatorStatusCh := make(chan fsm.Elevator)

  //fmt.Println("Order controller kicks in")
  //<-time.After(3 * time.Second)
  go fsm.FSM("localhost:15657",addHallOrderCh, deleteHallOrderCh, placedHallOrderCh, completedHallOrderCh, elevatorStatusCh)
  go simplified_order_controller(addHallOrderCh, deleteHallOrderCh, placedHallOrderCh, completedHallOrderCh, elevatorStatusCh)

  for{
  }

}
