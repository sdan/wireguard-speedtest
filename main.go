package main

import (
	"fmt"
	"io/fs"
	"io/ioutil"
	"log"
	"os"
	"runtime"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/go-ping/ping"
	wireproxy "github.com/octeep/wireproxy"
)

func main() {
	// Discards log output, comment out to see log output
	log.SetOutput(ioutil.Discard)

	sortedPeers := make(map[string]time.Duration)

	// 1. Gets all the files that end in .config path in the config directory
	files, err := ioutil.ReadDir("./config")
	if err != nil {
		log.Fatal(err)
	}
	path, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	sem := make(chan struct{}, runtime.NumCPU())

	var wg sync.WaitGroup
	wg.Add(len(files))

	for _, file := range files {
		go func(file fs.FileInfo) {
			sem <- struct{}{}
			defer func() { <-sem }()
			defer wg.Done()

			log.Println("Files in config directory:", file.Name())
			filePath := path + "/config/" + file.Name()
			log.Println("File path:", filePath)

			config, err := wireproxy.ParseConfig(filePath)
			if err != nil {
				log.Fatal(err)
			}
			log.Println("Config:", config)
			log.Println("Config.Device:", config.Device)
			log.Println("Config.Device.Peers:", config.Device.Peers)
			log.Println("Config.Device.Peers[0]:", config.Device.Peers[0].Endpoint)
			endpoint := config.Device.Peers[0].Endpoint
			avgLatency := pingPeer(endpoint)
			sortedPeers[file.Name()] = avgLatency
			outputString := "(" + file.Name() + ") " + avgLatency.String() + "\n"
			fmt.Print(outputString)
		}(file)
	}
	wg.Wait()

	// Sort sortedPeers by latency
	var keys []string
	for k := range sortedPeers {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return sortedPeers[keys[i]] < sortedPeers[keys[j]]
	})

	f, err := os.Create("stats.txt")
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	for _, k := range keys {
		outputString := "(" + k + ") " + sortedPeers[k].String() + "\n"
		f.WriteString(outputString)
		f.Sync()
	}

	fmt.Println("Top 10 fastest peers:")
	for i := 0; i < 10; i++ {
		fmt.Println(strconv.Itoa(i+1)+". ("+keys[i]+")", sortedPeers[keys[i]])
	}
	close(sem)

	// 2.

}

// Function that pings the endpoint of the peer and returns the latency
func pingPeer(endpoint string) time.Duration {
	// Trim last 6 characters of endpoint to get the IP address
	endpoint = endpoint[:len(endpoint)-6]
	log.Println("Pinging peer:", endpoint)
	pinger, err := ping.NewPinger(endpoint)
	if err != nil {
		panic(err)
	}
	pinger.Count = 3
	err = pinger.Run() // Blocks until finished.
	if err != nil {
		panic(err)
	}
	stats := pinger.Statistics() // get send/receive/duplicate/rtt stats
	log.Println("Ping stats:", stats.AvgRtt)
	return stats.AvgRtt
}
