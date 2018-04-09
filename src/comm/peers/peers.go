package peers

import (
	"../../msgs"
	"../conn"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"sort"
	"time"
)

type PeerUpdate struct {
	Peers msgs.ElevatorStatusSlice
	New   string
	Lost  msgs.HeartbeatSlice
}

type observation struct {
	Time      time.Time
	Heartbeat msgs.Heartbeat
}

const interval = 100 * time.Millisecond
const timeout = 1000 * time.Millisecond //50 * time.Millisecond

func Transmitter(port int, transmitEnable <-chan bool, statusCh <-chan msgs.Heartbeat) {

	conn := conn.DialBroadcastUDP(port)
	addr, _ := net.ResolveUDPAddr("udp4", fmt.Sprintf("255.255.255.255:%d", port))

	enable := true
	statusRecieved := false
	var recivedStatus msgs.Heartbeat
	for {
		select {
		case enable = <-transmitEnable:
		case recivedStatus = <-statusCh:
			statusRecieved = true
		case <-time.After(interval):
		}
		if enable && statusRecieved {
			serialized, err := json.Marshal(recivedStatus)
			if err != nil {
				log.Println("[peer]", err)
				continue
			}
			_, err = conn.WriteTo(serialized, addr)
			if err != nil {
				log.Println("[peer]", err)
				continue
			}
		}
	}
}

func Receiver(port int, peerUpdateCh chan<- PeerUpdate /*, statusCh chan<- msgs.Heartbeat*/) {

	var buf [1024]byte
	var p PeerUpdate
	lastSeen := make(map[string]observation)

	conn := conn.DialBroadcastUDP(port)

	for {
		updated := false

		conn.SetReadDeadline(time.Now().Add(interval))
		n, _, _ := conn.ReadFrom(buf[0:])
		data := buf[:n]
		var heartbeat msgs.Heartbeat
		json.Unmarshal(data, &heartbeat)

		id := heartbeat.SenderID

		// Adding new connection
		p.New = ""
		if id != "" {
			if _, idExists := lastSeen[id]; !idExists {
				p.New = id
				updated = true
			}

			lastSeen[id] = observation{Time: time.Now(), Heartbeat: heartbeat}
		}

		// Removing dead connection
		p.Lost = make(msgs.HeartbeatSlice, 0)
		for k, v := range lastSeen {
			if time.Now().Sub(v.Time) > timeout {
				updated = true
				p.Lost = append(p.Lost, v.Heartbeat)
				delete(lastSeen, k)
			}
		}

		// Sending update
		if updated {
			p.Peers = make(msgs.ElevatorStatusSlice, 0, len(lastSeen))

			for _, v := range lastSeen {
				p.Peers = append(p.Peers, v.Heartbeat.Status)
			}

			sort.Sort(p.Peers)
			sort.Sort(p.Lost)
			peerUpdateCh <- p
		}
	}
}
