package main

import (
	"fmt"
	"log"
	//"strings"
	"math"
	"testing"
	"time"
)

func echoResults(target string, packetsTx, packetsRx int64, minLatency, avgLatency, maxLatency, stdDevLatency float64) {

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
	var packetsTx int64
	var packetsRx int64

	target := "8.8.8.8"

	pingTicker := time.NewTicker(time.Second)
	traceTicker := time.NewTicker(time.Second * 30)
	for {
		select {
		case <-pingTicker.C:
			packetsTx++
			latency, err := sendPing(target, packetsTx)
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
				traceResults, err := ttlTrace(target)
				if err != nil {
					elog.Print(err)
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
						traceResults, err := ttlTrace(target)
						if err != nil {
							elog.Print(err)
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
