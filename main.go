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

	type SafePeers struct {
		sync.Mutex
		Peers map[string]Peer
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
	sem := semaphore.NewWeighted(8)
	num := 0
	safePeers := SafePeers{Peers: make(map[string]Peer)}
	ipapiClient := http.Client{}
	for _, file := range files {
		sem.Acquire(context.Background(), 1)
		go func(file fs.FileInfo) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

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
			avgLatency, country := pingPeer(ctx, ipapiClient, endpoint)
			log.Println("Country:", country)
			log.Println("Avg latency:", avgLatency)
			log.Println("Current country leader:", sortedPeers[country].Latency)

			if val, ok := safePeers.Peers[country]; ok {
				safePeers.Lock()
				if val.Latency == 0 || avgLatency < val.Latency {
					safePeers.Peers[country] = Peer{avgLatency, file.Name()}
				}
				safePeers.Unlock()
			} else {
				safePeers.Lock()
				safePeers.Peers[country] = Peer{avgLatency, file.Name()}
				safePeers.Unlock()
			}

			num++
			outputString := strconv.Itoa(num) + ". (" + file.Name() + ") " + avgLatency.String() + "\n"
			fmt.Print(outputString)
		}(file)
	}
	log.Println("Sorting peers:", sortedPeers)

	// Sort sortedPeers by latency

	safePeers.Lock()
	keys := make([]string, 0, len(safePeers.Peers))
	for k := range safePeers.Peers {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return safePeers.Peers[keys[i]].Latency < safePeers.Peers[keys[j]].Latency
	})
	safePeers.Unlock()

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

	safePeers.Lock()
	for _, k := range keys {
		outputString := "(" + k + ") " + safePeers.Peers[k].Latency.String() + "(" + safePeers.Peers[k].Endpoint + ")\n"
		log.Print("Writing to file:", outputString)
		_, err := f.WriteString(outputString)
		if err != nil {
			log.Fatal(err)
		}
	}
	safePeers.Unlock()

	// Inside the top 10 fastest peers section, use the safePeers mutex to access the map
	fmt.Println("\n\n\nTop 10 fastest peers by country:")
	safePeers.Lock()
	for i := 0; i < 10 && i < len(keys); i++ {
		fmt.Printf("(%-10s) %-15s (%s)\n", keys[i], safePeers.Peers[keys[i]].Latency.String(), safePeers.Peers[keys[i]].Endpoint)
	}
	safePeers.Unlock()
	os.Exit(0)

}

// Function that pings the endpoint of the peer and returns the latency
func pingPeer(ctx context.Context, geoClient http.Client, endpoint string) (time.Duration, string) {
	// Trim last 6 characters of endpoint to get the IP address
	endpoint = endpoint[:len(endpoint)-6]

	url := "https://tools.keycdn.com/geo.json?host=" + endpoint
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Fatal(err)
	}
	req.Header.Set("User-Agent", "keycdn-tools:https://example.com")
	resp, err := geoClient.Do(req)
	if err != nil {
		log.Fatal("resp error", err)
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatal("body error", err)
	}
	log.Println("Body:", string(body))
	// Get country from body
	objmap := make(map[string]*json.RawMessage)
	err = json.Unmarshal(body, &objmap)
	if err != nil {
		log.Fatal("unmarshal error", err)
	}
	// Sample data: {"status":"success","description":"Data successfully received.","data":{"geo":{"host":"146.70.116.130","ip":"146.70.116.130","rdns":"146.70.116.130","asn":9009,"isp":"M247 Europe SRL","country_name":"Austria","country_code":"AT","region_name":"Vienna","region_code":"9","city":"Vienna","postal_code":"1230","continent_name":"Europe","continent_code":"EU","latitude":48.1436,"longitude":16.2941,"metro_code":null,"timezone":"Europe\/Vienna","datetime":"2022-10-26 09:32:27"}}}
	// Unmarshal the data.geo.region_code field to get the country code

	err = json.Unmarshal(*objmap["data"], &objmap)
	if err != nil {
		log.Fatal("data un", err)
	}
	err = json.Unmarshal(*objmap["geo"], &objmap)
	if err != nil {
		log.Fatal("geo err", err)
	}
	// Unmarshal region name and if that fails, unmarshal country name, handle if region name is null
	var country string
	err = json.Unmarshal(*objmap["country_name"], &country)
	if err != nil {
		log.Print("country name err", err)
	}

	log.Println("Country name:", country)

	log.Println("Pinging peer:", endpoint)
	pinger, err := ping.NewPinger(endpoint)
	if err != nil {
		panic(err)
	}
	pinger.Count = 3

	done := make(chan struct{})
	go func() {
		err = pinger.Run() // Blocks until finished.
		close(done)
	}()
	select {
	case <-done:
		if err != nil {
			panic(err)
		}
	case <-ctx.Done():
		pinger.Stop()
		return 99999999, "NaN"
	}

	stats := pinger.Statistics() // get send/receive/duplicate/rtt stats
	log.Println("Ping stats:", stats.AvgRtt)

	return stats.AvgRtt, country
}
