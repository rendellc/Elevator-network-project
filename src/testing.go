package main

import "github.com/hectane/go-nonblockingchan"
import "fmt"

type Point struct {
	X int
	Y int
}

func main() {
	a := nbc.New()
	b := nbc.New()
	a.Send <- Point{1, 2}

	//a := make(chan bool)
	//b := make(chan bool)

	for {
		select {
		case v, _ := <-a.Recv:
			fmt.Println("a recv", v)
			b.Send <- v
		case v, _ := <-b.Recv:
			point := v.(Point)
			fmt.Println("b recv", point)
			a.Send <- point.X
		}
	}
}
