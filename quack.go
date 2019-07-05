package quack

import (
	"bytes"
	"fmt"
	"log"
	"net"
	"os"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"

	//"strconv"

	"time"
)

const CLR_0 = "\x1b[30;1m"
const CLR_R = "\x1b[31;1m"
const CLR_G = "\x1b[32;1m"
const CLR_Y = "\x1b[33;1m"
const CLR_B = "\x1b[34;1m"
const CLR_M = "\x1b[35;1m"
const CLR_C = "\x1b[36;1m"
const CLR_W = "\x1b[37;1m"
const CLR_N = "\x1b[0m"

type PingResult struct {
	Target      string
	Latency     float64
	Time        int64
	ICMPMessage *icmp.Message
	Size        int
	TTL         int
	Sequence    int
	Peer        net.Addr
}

var thirtySampleRollingLatency []float64

type HostStats struct {
	HostName       string
	HostDNSName    string
	AvgLatency     float64
	MaxLatency     float64
	MinLatency     float64
	TripCount      int
	HighLatency100 int
	HighLatency400 int
	HighLatency700 int
}

type PathStats struct {
	PathName      string
	MaxLatencyAvg float64
	AvgLatency    float64
	TripCount     int
}

func TTLTrace(targetIP string) ([]PingResult, error) {

	var latency float64

	c, err := net.ListenPacket("ip4:1", "0.0.0.0") // ICMP for IPv4
	if err != nil {
		return nil, err
	}
	defer c.Close()

	traceResults := make(map[int]PingResult)
	var ttls []int

	for i := 1; i < 64; i++ {

		//log.Printf("Tracing TTL %d", i)

		ttls = append(ttls, i)

		conn := ipv4.NewPacketConn(c)
		if err != nil {
			return nil, err
		}

		if err := conn.SetControlMessage(ipv4.FlagTTL|ipv4.FlagSrc|ipv4.FlagDst|ipv4.FlagInterface, true); err != nil {
			log.Fatal(err)
		}

		wm := icmp.Message{
			Type: ipv4.ICMPTypeEcho,
			Code: 0,
			Body: &icmp.Echo{
				ID:   os.Getpid() & 0xffff,
				Seq:  i,
				Data: []byte("HELLO-R-U-THERE-TTLTRACE-TTLTRACE-TTLTRACE-TTLTRACE-BYE!"),
			},
		}
		wb, err := wm.Marshal(nil)
		if err != nil {
			return nil, err
		}

		conn.SetDeadline(time.Now().Add(time.Second * 2))
		if err != nil {
			return nil, err
		}

		if err := conn.SetTTL(i); err != nil {
			return nil, err
		}

		start := time.Now()
		if _, err := conn.WriteTo(wb, nil, &net.IPAddr{IP: net.ParseIP(targetIP)}); err != nil {
			return nil, err
		}

	LISTENTRACE:
		rb := make([]byte, 128)
		n, peer, err := c.ReadFrom(rb)
		if err != nil {
			//return nil, err
			continue
		}
		rm, err := icmp.ParseMessage(ipv4.ICMPTypeEcho.Protocol(), rb[:n])
		if err != nil {
			return nil, err
		}

		if bytes.Contains(rb[:n], []byte("HELLO-R-U-THERE-KNOCK-KNOCK-QUACK-QUACK-QUACK-QUACK-BYE!")) && time.Since(start).Seconds() < 2 {
			goto LISTENTRACE
		}

		latency = float64(time.Since(start).Nanoseconds()) / 1000000

		switch rm.Type {
		case ipv4.ICMPTypeEchoReply:

			pr := PingResult{
				Target:  peer.String(),
				Latency: latency,
				Time:    time.Now().Unix(),
			}

			traceResults[i] = pr

		case ipv4.ICMPTypeDestinationUnreachable:
			fmt.Printf("Destination network unreachable")

		case ipv4.ICMPTypeTimeExceeded:
			pr := PingResult{
				Target:  peer.String(),
				Latency: latency,
				Time:    time.Now().Unix(),
			}

			traceResults[i] = pr

		default:
			log.Printf("got %+v; want echo reply", rm)
		}

		if targetIP == peer.String() {
			break
		}
	}

	var traceResultSlice []PingResult

	for _, ttl := range ttls {
		traceResultSlice = append(traceResultSlice, traceResults[ttl])
	}

	return traceResultSlice, nil
}

func SendPing(targetIP string, seq, size int, pingResultChan chan PingResult) (float64, error) {

	var latency float64

	c, err := net.ListenPacket("ip4:1", "0.0.0.0") // ICMP for IPv4
	if err != nil {
		log.Printf("listen err, %s", err)
		return latency, err
	}
	defer c.Close()

	conn := ipv4.NewPacketConn(c)
	if err != nil {
		return latency, err
	}

	if err := conn.SetControlMessage(ipv4.FlagTTL|ipv4.FlagSrc|ipv4.FlagDst|ipv4.FlagInterface, true); err != nil {
		return latency, err
	}

	payload := []byte("HELLO-R-U-THERE-KNOCK-KNOCK-QUACK-QUACK-QUACK-QUACK-BYE!")

	if size > 56 {
		payload = append(payload, bytes.Repeat([]byte("X"), int(size)-56)...)
	} else {
		size = 56
	}

	wm := icmp.Message{
		Type: ipv4.ICMPTypeEcho,
		Code: 0,
		Body: &icmp.Echo{
			ID:   os.Getpid() & 0xffff,
			Seq:  int(seq),
			Data: payload,
		},
	}
	wb, err := wm.Marshal(nil)
	if err != nil {
		return latency, err
	}

	conn.SetDeadline(time.Now().Add(time.Second * 2))
	if err != nil {
		return latency, err
	}

	if err := conn.SetTTL(122); err != nil {
		return latency, err
	}

	start := time.Now()
	if _, err := conn.WriteTo(wb, nil, &net.IPAddr{IP: net.ParseIP(targetIP)}); err != nil {
		log.Printf("WriteTo err, %s", err)
		return latency, err
	}

LISTEN:

	rb := make([]byte, 128)
	n, peer, err := c.ReadFrom(rb)
	if err != nil {
		return latency, err
	}

	_ = peer

	rm, err := icmp.ParseMessage(ipv4.ICMPTypeEcho.Protocol(), rb[:n])
	if err != nil {
		return latency, err
	}

	if !bytes.Contains(rb[:n], []byte("HELLO-R-U-THERE-KNOCK-KNOCK-QUACK-QUACK-QUACK-QUACK-BYE!")) && time.Since(start).Seconds() < 2 {
		goto LISTEN
	}

	latency = float64(time.Since(start).Nanoseconds()) / 1000000

	ttl, err := conn.TTL()
	if err != nil {
		return latency, err
	}

	pr := PingResult{
		Target:      targetIP,
		Latency:     latency,
		Time:        time.Now().Unix(),
		ICMPMessage: rm,
		Size:        size,
		TTL:         ttl,
		Peer:        peer,
	}

	if rm.Type == ipv4.ICMPTypeTimeExceeded {
		pr.Latency = 2000
	}

	pingResultChan <- pr

	return latency, nil
}
