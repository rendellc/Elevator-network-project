package no_main


func main(){
  drv_intern_hall_order := make(chan OrderEvent)
  drv_hall_button_event := make(chan elevio.ButtonEvent)

  go fsm_module(drv_intern_hall_order,drv_hall_button_event)
  go simplified_order_controller(drv_intern_hall_order,drv_hall_button_event)

  for{
  }
}
