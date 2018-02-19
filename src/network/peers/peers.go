package peers

import (
	"../conn"
	"fmt"
	"net"
	"sort"
	"time"
	"encoding/json"
	"../../msgtype"
)

type PeerUpdate struct {
	Peers []string
	New   string
	Lost  []string
}

const interval = 15 * time.Millisecond
const timeout = 50 * time.Millisecond

func Transmitter(port int, transmitEnable <-chan bool, statusCh <-chan msgtype.Heartbeat) {

	conn := conn.DialBroadcastUDP(port)
	addr, _ := net.ResolveUDPAddr("udp4", fmt.Sprintf("129.241.187.255:%d", port))

	enable := true
	statusRecieved := false
	var recivedStatus msgtype.Heartbeat
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
				continue
			}
			conn.WriteTo(serialized, addr)
		}
	}
}

func Receiver(port int, peerUpdateCh chan<- PeerUpdate, statusCh chan<- msgtype.Heartbeat) {

	var buf [1024]byte
	var p PeerUpdate
	lastSeen := make(map[string]time.Time)

	conn := conn.DialBroadcastUDP(port)

	for {
		updated := false

		conn.SetReadDeadline(time.Now().Add(interval))
		n, _, _ := conn.ReadFrom(buf[0:])
		data := buf[:n]
		var heartbeat msgtype.Heartbeat
		json.Unmarshal(data, &heartbeat)

		go func(heartbeat msgtype.Heartbeat){
			statusCh <- heartbeat
		}(heartbeat)


		id := heartbeat.SourceID

		// Adding new connection
		p.New = ""
		if id != "" {
			if _, idExists := lastSeen[id]; !idExists {
				p.New = id
				updated = true
			}

			lastSeen[id] = time.Now()
		}

		// Removing dead connection
		p.Lost = make([]string, 0)
		for k, v := range lastSeen {
			if time.Now().Sub(v) > timeout {
				updated = true
				p.Lost = append(p.Lost, k)
				delete(lastSeen, k)
			}
		}

		// Sending update
		if updated {
			p.Peers = make([]string, 0, len(lastSeen))

			for k, _ := range lastSeen {
				p.Peers = append(p.Peers, k)
			}

			sort.Strings(p.Peers)
			sort.Strings(p.Lost)
			peerUpdateCh <- p
		}
	}
}
