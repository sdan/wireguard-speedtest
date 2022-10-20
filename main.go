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
	"golang.org/x/sync/syncmap"
)

func main() {
	// Discards log output, comment out to see log output
	log.SetOutput(ioutil.Discard)

	// sortedPeers := make(map[string]time.Duration)
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
			// sortedPeers[file.Name()] = avgLatency
			sortedPeers.Store(file.Name(), avgLatency)
			outputString := "(" + file.Name() + ") " + avgLatency.String() + "\n"
			fmt.Print(outputString)
		}(file)
	}
	wg.Wait()

	// Sort sortedPeers by latency
	var keys []string

	sortedPeers.Range(func(key, value interface{}) bool {
		// cast value to correct format
		val, ok := value.(string)
		if !ok {
			// this will break iteration
			log.Fatal("Could not cast value to string")
			return false
		}
		keys = append(keys, val)

		// this will continue iterating
		return true
	})
	// Sort sortedPeers by latency
	sort.Slice(keys, func(i, j int) bool {
		// cast value to correct format
		val1, ok := sortedPeers.Load(keys[i])
		if !ok {
			// this will break iteration
			log.Fatal("Could not cast value to string")
			return false
		}
		val2, ok := sortedPeers.Load(keys[j])
		if !ok {
			// this will break iteration
			log.Fatal("Could not cast value to string")
			return false
		}
		return val1.(time.Duration) < val2.(time.Duration)
	})

	// sort.Slice(keys, func(i, j int) bool {
	// 	return sortedPeers.Load([keys[i]]time.Duration) < sortedPeers.Load([keys[j]]time.Duration)
	// })

	f, err := os.Create("stats.txt")
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	// for _, k := range keys {
	// 	outputString := "(" + k + ") " + sortedPeers.Load(k) + "\n"
	// 	f.WriteString(outputString)
	// 	f.Sync()
	// }

	// Interate through sortedPeers and write to stats.txt
	for _, k := range keys {
		// cast value to correct format
		val, ok := sortedPeers.Load(k)
		if !ok {
			// this will break iteration
			log.Fatal("Could not cast value to string")
			return
		}
		outputString := "(" + k + ") " + val.(time.Duration).String() + "\n"
		f.WriteString(outputString)
		f.Sync()
	}

	log.Println("Top 10 fastest peers:")
	for i := 0; i < 10; i++ {
		val, ok := sortedPeers.Load(keys[i])
		if !ok {
			// this will break iteration
			log.Fatal("Could not cast value to string")
			return
		}

		log.Println(strconv.Itoa(i+1)+". ("+keys[i]+")", val.(time.Duration))
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
	pinger.Stop()
	return stats.AvgRtt
}
