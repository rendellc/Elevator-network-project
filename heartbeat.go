package main

import(
	"fmt"
	"time"
	//"net"
)

func main() {
	heartbeat_frequency := 10.0
	//max_number_messages := 20

	fmt.Printf("Heartbeat frequency is %g Hz \n", heartbeat_frequency)
	period := int(1000/heartbeat_frequency)
	period_d := time.Duration(period)*time.Millisecond
	fmt.Print("The period is ", period_d, ".\n")
	//stop_time := max_number_messages*period+1
	//stop_time_d := time.Duration(stop_time)*time.Millisecond
	ticker := time.NewTicker(period_d)
	
	go func() {
	var i int = 1
	for t := range ticker.C {
		fmt.Println("Message ", i, " at ", t)
		i=i+1
	}
	}()

	fmt.Scanln()
	//time.Sleep(stop_time_d)
	//ticker.Stop()
	fmt.Println("Last message sent")

}
