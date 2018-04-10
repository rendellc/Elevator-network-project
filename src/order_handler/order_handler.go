package order_handler

import (
  "../elevio/elevio"
  "../fsm"
  "fmt"
  "time"
)

func simplified_order_controller(drv_intern_hall_order chan<- OrderEvent, drv_hall_button_event <-chan elevio.ButtonEvent){
  for{
    select{
      // short circuit order controller
    case button_event:= <-drv_hall_button_event:
      fmt.Println("Hall order in transit")
      drv_intern_hall_order <- OrderEvent{button_event.Floor, button_event.Button, true, true}
    }
  }
}
