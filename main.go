package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"runtime"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/go-ping/ping"
	wireproxy "github.com/octeep/wireproxy"
	"golang.org/x/sync/semaphore"
)

func main() {
	// Discards log output, comment out to see log output
	log.SetOutput(ioutil.Discard)

	type Peer struct {
		Latency  time.Duration
		Endpoint string
	}
	sortedPeers := make(map[string]Peer)

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
	num := 0
	ipapiClient := http.Client{}
	for _, file := range files {
		sem.Acquire(context.Background(), 1)

		go func(file fs.FileInfo) {
			defer sem.Release(1)
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
			avgLatency, country := pingPeer(ipapiClient, endpoint)
			log.Println("Avg latency:", avgLatency)
			log.Println("Country:", country)
			// sortedPeers[file.Name()] = {avgLatency, country}
			// sortedPeers[file.Name()] = Peer{avgLatency, country}
			if avgLatency > sortedPeers[country].Latency {
				sortedPeers[country] = Peer{avgLatency, file.Name()}
			}
			// sortedPeers.Store(file.Name(), avgLatency)
			num++
			outputString := strconv.Itoa(num) + ". (" + file.Name() + ") " + avgLatency.String() + "\n"
			fmt.Print(outputString)
		}(file)
	}
	log.Println("Sorting peers:", sortedPeers)
	wg.Wait()

	// Sort sortedPeers by latency
	keys := make([]string, 0, len(sortedPeers))
	for k := range sortedPeers {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return sortedPeers[keys[i]].Latency < sortedPeers[keys[j]].Latency
	})

	// Print sortedPeers
	log.Println("Sorted peers:")
	for _, k := range keys {
		fmt.Println(k, sortedPeers[k])
	}

	f, err := os.Create("stats.txt")
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	// wg.Wait()

	for _, k := range keys {
		outputString := "(" + k + ") " + sortedPeers[k].Latency.String() + "\n"
		log.Print("Writing to file:", outputString)
		_, err := f.WriteString(outputString)
		if err != nil {
			log.Fatal(err)
		}
	}

	// Print top 10 fastest peers
	fmt.Println("\n\n\nTop 10 fastest peers by country:")
	for i := 0; i < 10; i++ {
		// Print the country and latency and the name of the config file like this: (US) 1.2345ms (us1.config)
		// outputString := "(" + keys[i] + ") " + sortedPeers[keys[i]].Latency.String() + "\n"
		// fmt.Print(outputString)
		fmt.Printf("(%-10s) %-15s (%s)\n", keys[i], sortedPeers[keys[i]].Latency.String(), sortedPeers[keys[i]].Endpoint)
	}
	os.Exit(0)

}

// Function that pings the endpoint of the peer and returns the latency
func pingPeer(geoClient http.Client, endpoint string) (time.Duration, string) {
	// Trim last 6 characters of endpoint to get the IP address
	endpoint = endpoint[:len(endpoint)-6]

	url := "https://ipapi.co/" + endpoint + "/json/"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Fatal(err)
	}
	req.Header.Set("User-Agent", "ipapi.co/#go-v1.3")
	resp, err := geoClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("Body:", string(body))
	// Get country from body
	var objmap map[string]interface{}
	err = json.Unmarshal(body, &objmap)
	if err != nil {
		log.Fatal(err)
	}
	country := objmap["country_name"].(string)
	log.Println("Country:", country)

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
	// Rate limiting wait
	time.Sleep(10 * time.Millisecond)
	return stats.AvgRtt, country
}
