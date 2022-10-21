package main

import (
	"context"
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
	"golang.org/x/sync/semaphore"
	"golang.org/x/sync/syncmap"
)

func main() {
	// Discards log output, comment out to see log output
	log.SetOutput(ioutil.Discard)

	sortedPeers := syncmap.Map{}

	// 1. Gets all the files that end in .config path in the config directory
	files, err := ioutil.ReadDir("./config")
	if err != nil {
		log.Fatal(err)
	}
	path, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	var wg sync.WaitGroup
	wg.Add(len(files))
	sem := semaphore.NewWeighted(int64(runtime.NumCPU()))

	for _, file := range files {
		sem.Acquire(context.Background(), 1)
		go func(file fs.FileInfo) {
			defer wg.Done()
			defer sem.Release(1)

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
			// sortedPeers[file.Name()] = avgLatency
			sortedPeers.Store(file.Name(), avgLatency)
			outputString := "(" + file.Name() + ") " + avgLatency.String() + "\n"
			fmt.Print(outputString)
		}(file)
	}
	wg.Wait()

	// Sort sortedPeers by latency
	m := map[string]time.Duration{}
	sortedPeers.Range(func(key, value interface{}) bool {
		m[fmt.Sprint(key)] = value.(time.Duration)
		return true
	})
	// Sort m map by value
	keys := make([]string, 0, len(m))

	for k := range m {
		keys = append(keys, k)
	}

	sort.Slice(keys, func(i, j int) bool {
		return m[keys[i]] < m[keys[j]]
	})

	// Print sortedPeers
	log.Println("Sorted peers:")
	for _, k := range keys {
		fmt.Println(k, m[k])
	}

	f, err := os.Create("stats.txt")
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	for _, k := range keys {
		outputString := "(" + k + ") " + m[k].String() + "\n"
		_, err := f.WriteString(outputString)
		if err != nil {
			log.Fatal(err)
		}
	}

	// Print top 10 fastest peers
	fmt.Println("\n\n\nTop 10 fastest peers:")
	for i := 0; i < 10; i++ {
		outputString := strconv.Itoa(i+1) + ". (" + (keys[i]) + ") " + (m[keys[i]]).String() + "\n"
		fmt.Print(outputString)
	}

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
