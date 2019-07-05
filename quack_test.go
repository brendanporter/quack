package quack

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"runtime"
	"strings"

	//"strings"
	"math"
	"testing"
	"time"

	"golang.org/x/net/ipv4"
)

type UnhealthyPingResult struct {
	Target      string
	Latency     float64
	Time        int64
	MessageType string `json:"mt"`
}

type TraceResult struct {
	Hops        []PingResult
	MessageType string `json:"mt"`
}

type PathsResult struct {
	Paths       map[string]*PathStats
	MessageType string `json:"mt"`
}

type HostsResult struct {
	Hosts       map[string]*HostStats
	MessageType string `json:"mt"`
}

var pingResults map[string][]PingResult

var pingResultChan chan PingResult

var resultRequestChan chan int
var resultResponseChan chan []byte

var ttlTraceResultChan chan []PingResult
var unhealthyPingResultChan chan PingResult

var pathResultsChan chan *PathStats
var pathResultsRequestChan chan int
var pathResultsResponseChan chan map[string]*PathStats

var hostResultsChan chan *PingResult
var hostResultsRequestChan chan int
var hostResultsResponseChan chan map[string]*HostStats

func resultProcessor() {

	pingResults = make(map[string][]PingResult)
	pingResultChan = make(chan PingResult, 10)

	resultResponseChan = make(chan []byte, 10)
	resultRequestChan = make(chan int, 10)
	ttlTraceResultChan = make(chan []PingResult, 10)
	unhealthyPingResultChan = make(chan PingResult, 10)

	pathResultsChan = make(chan *PathStats, 10)
	pathResultsRequestChan = make(chan int, 10)
	pathResultsResponseChan = make(chan map[string]*PathStats, 10)

	hostResultsChan = make(chan *PingResult, 10)
	hostResultsRequestChan = make(chan int, 10)
	hostResultsResponseChan = make(chan map[string]*HostStats, 10)

	paths := make(map[string]*PathStats)
	hosts := make(map[string]*HostStats)

	for i := 0; i < 100; i++ {
		select {
		case pr := <-pingResultChan:

			if pr.ICMPMessage.Type == ipv4.ICMPTypeDestinationUnreachable {
				fmt.Printf("Destination network unreachable\n")
				continue
			}

			var color string = CLR_W
			if pr.Latency < 40.0 {
				color = CLR_G
			} else if pr.Latency > 65.0 && pr.Latency < 150.0 {
				color = CLR_Y
			} else if pr.Latency > 150.0 {
				color = CLR_R
			}

			barCount := int((pr.Latency / 2000) * 80)

			if runtime.GOOS != "windows" {
				fmt.Printf("%d bytes from %s: icmp_seq=%d ttl=%d time=%s%.3f ms |%s|%s\n", pr.Size, pr.Peer, pr.Sequence, pr.TTL, color, pr.Latency, strings.Repeat("-", barCount), CLR_W)

			} else {
				fmt.Printf("%d bytes from %s: icmp_seq=%d ttl=%d time=%.3f ms |%s|\n", pr.Size, pr.Peer, pr.Sequence, pr.TTL, pr.Latency, strings.Repeat("-", barCount))
			}

			pingResults[pr.Target] = append(pingResults[pr.Target], pr)
			_, err := json.Marshal(pr)
			if err != nil {
				log.Print(err)
			}

		case traceResult := <-ttlTraceResultChan:

			tr := TraceResult{
				Hops:        traceResult,
				MessageType: "hops",
			}
			_, err := json.Marshal(tr)
			if err != nil {
				log.Print(err)
			}

		case unhealthyPingResult := <-unhealthyPingResultChan:

			if unhealthyPingResult.Time < 100000 {
				break
			}
			upr := UnhealthyPingResult{
				Target:      unhealthyPingResult.Target,
				Latency:     unhealthyPingResult.Latency,
				Time:        unhealthyPingResult.Time,
				MessageType: "unhealthyPingResult",
			}
			_, err := json.Marshal(upr)
			if err != nil {
				log.Print(err)
			}

		case <-resultRequestChan:
			resultsJSON, err := json.Marshal(pingResults)
			if err != nil {
				log.Print(err)
			}

			resultResponseChan <- resultsJSON

		case nps := <-pathResultsChan:

			pathName := nps.PathName

			if _, ok := paths[pathName]; !ok {
				paths[pathName] = nps
			} else {
				paths[pathName].AvgLatency = (paths[pathName].AvgLatency + nps.AvgLatency) / 2
				paths[pathName].MaxLatencyAvg = (paths[pathName].MaxLatencyAvg + nps.MaxLatencyAvg) / 2
				paths[pathName].TripCount++
			}

		case pr := <-hostResultsChan:

			hostName := pr.Target

			if _, ok := hosts[hostName]; !ok {

				hostDNSNames, err := net.LookupAddr(hostName)
				if err != nil {
					log.Print(err)
				}

				var hostDNSName string

				if len(hostDNSNames) > 0 {
					hostDNSName = hostDNSNames[0]
				}

				hosts[hostName] = &HostStats{}
				if pr.Latency > 0 {
					hosts[hostName].MinLatency = pr.Latency
				} else {
					hosts[hostName].MinLatency = 9999
				}
				hosts[hostName].AvgLatency = pr.Latency
				hosts[hostName].HostName = hostName
				hosts[hostName].HostDNSName = hostDNSName

			} else {

				if hosts[hostName].MinLatency > pr.Latency {
					hosts[hostName].MinLatency = pr.Latency
				}

				if hosts[hostName].MaxLatency < pr.Latency {
					hosts[hostName].MaxLatency = pr.Latency
				}

				hosts[hostName].AvgLatency = (hosts[hostName].AvgLatency + pr.Latency) / 2

			}

			hosts[hostName].TripCount++

			if pr.Latency > 700 {
				hosts[hostName].HighLatency700++
			} else if pr.Latency > 400 {
				hosts[hostName].HighLatency400++
			} else if pr.Latency > 100 {
				hosts[hostName].HighLatency100++
			}

		case <-pathResultsRequestChan:

			prr := make(map[string]*PathStats)

			for k, v := range paths {
				prr[k] = v
			}

			pathResultsResponseChan <- paths

		case <-hostResultsRequestChan:

			hrr := make(map[string]*HostStats)

			for k, v := range hosts {
				hrr[k] = v
			}

			hostResultsResponseChan <- hosts

		}

	}
}

func echoResults(target string, packetsTx, packetsRx int, minLatency, avgLatency, maxLatency, stdDevLatency float64) {

	fmt.Print("\n")
	log.Printf("--- %s ping statistics ---", target)
	fmt.Printf("%d packets transmitted, %d packets received, %.1f%% packet loss\n", packetsTx, packetsRx, (float64(packetsTx-packetsRx) / float64(packetsTx) * 100))
	fmt.Printf("round-trip min/avg/max/stddev = %.3f/%.3f/%.3f/%.3f ms\n", minLatency, avgLatency, maxLatency, stdDevLatency)
	fmt.Printf("View charted results at: http://localhost:14445\n\n")
}

func TestSendPing(t *testing.T) {

	go resultProcessor()

	var maxLatency float64
	var minLatency float64 = 99999.9
	var avgLatency float64
	var stdDevLatency float64
	var packetsTx int
	var packetsRx int

	target := "8.8.8.8"

	pingTicker := time.NewTicker(time.Second)
	traceTicker := time.NewTicker(time.Second * 30)
	for {
		select {
		case <-pingTicker.C:
			packetsTx++
			latency, err := SendPing(target, packetsTx, pingResultChan)
			if err != nil {
				log.Print(err)
				fmt.Printf("Request timeout for icmp_seq %d\n", packetsTx)
				latency = 2000.0
			} else {
				packetsRx++
			}

			if avgLatency == 0 {
				avgLatency = latency
			} else {
				avgLatency = (avgLatency + latency) / 2
			}

			stdDevLatency += math.Pow(latency-avgLatency, 2)
			stdDevLatency = math.Sqrt(stdDevLatency / 2)

			if latency < minLatency {
				minLatency = latency
			}

			if latency > maxLatency {
				maxLatency = latency
			}

			if packetsTx%30 == 0 {
				echoResults(target, packetsTx, packetsRx, minLatency, avgLatency, maxLatency, stdDevLatency)
			}

			//time.Sleep(time.Duration(time.Second.Nanoseconds() - int64(latency*1000000)))

			if len(thirtySampleRollingLatency) == 30 {
				thirtySampleRollingLatency = thirtySampleRollingLatency[1:]
			}
			thirtySampleRollingLatency = append(thirtySampleRollingLatency, latency)

		case <-traceTicker.C:
			go func() {
				traceResults, err := TTLTrace(target)
				if err != nil {
					log.Print(err)
				}

				ttlTraceResultChan <- traceResults

				var lastLatency float64
				var highLatencyHosts []PingResult
				var latencyHighWaterMark float64

				for i, traceResult := range traceResults {

					log.Printf("TTL %d result: %#v", i+1, traceResult)

					if lastLatency == 0 {
						lastLatency = traceResult.Latency
						continue
					}

					if math.Abs(traceResult.Latency-lastLatency) > 100 && traceResult.Latency != 0 && traceResult.Latency > latencyHighWaterMark {
						if i > 0 && traceResults[i-1].Latency > 100 {
							highLatencyHosts = append(highLatencyHosts, traceResults[i-1])
						}
						highLatencyHosts = append(highLatencyHosts, traceResult)
					}
					lastLatency = traceResult.Latency

					if traceResult.Latency > latencyHighWaterMark {
						latencyHighWaterMark = traceResult.Latency
					}

				}

				// If latency was high, perform additional ttlTraces to collect more path observations

				if latencyHighWaterMark > 100 {
					log.Printf("High latency of %.1fms detected. Performing additional traces.", latencyHighWaterMark)
					for x := 0; x < 4; x++ {
						traceResults, err := TTLTrace(target)
						if err != nil {
							log.Print(err)
						}

						_ = traceResults

						time.Sleep(time.Millisecond * 200)
					}
				}

				for _, highLatencyHost := range highLatencyHosts {
					log.Printf("Found potentially unhealthy host in trace: %#v", highLatencyHost)

					unhealthyPingResultChan <- highLatencyHost
				}

			}()
		}
	}

}
