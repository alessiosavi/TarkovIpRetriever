package main

import (
	"bytes"
	"fmt"
	arrayutils "github.com/alessiosavi/GoGPUtils/array"
	fileutils "github.com/alessiosavi/GoGPUtils/files"
	"github.com/alessiosavi/GoGPUtils/helper"
	stringutils "github.com/alessiosavi/GoGPUtils/string"
	"github.com/go-ping/ping"
	"github.com/ip2location/ip2location-go/v9"
	"github.com/schollz/progressbar/v3"
	"io/ioutil"
	"log"
	"os"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type IP struct {
	D1 string
	D2 string
	D3 string
	D4 string
}

func NewIp(ip string) IP {
	var data IP

	split := strings.Split(ip, ".")
	if len(split) != 4 {
		panic("Err not valid ip: " + ip)
	}
	data.D1 = split[0]
	data.D2 = split[1]
	data.D3 = split[2]
	data.D4 = split[3]

	return data
}

func (ip *IP) Get() string {
	return stringutils.JoinSeparator(".", ip.D1, ip.D2, ip.D3, ip.D4)
}

func (ip *IP) GetSignificant() string {
	return stringutils.JoinSeparator(".", ip.D1, ip.D2, ip.D3)
}
func (ip *IP) FilterEquals(targets []IP) []IP {
	var data []IP
	for _, target := range targets {
		if !ip.Equals(target) {
			data = append(data, target)
		}
	}

	return targets
}

func (ip *IP) Equals(target IP) bool {
	return ip.D1 == target.D1 && ip.D2 == target.D2 && ip.D3 == target.D3
}

var MAIN_FOLDER = []string{"C:\\Battlestate Games\\EFT (live)\\Logs", "C:\\Battlestate Games\\EFT\\Logs", "/tmp/Logs"}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile | log.Ltime)
	var files []string
	var err error

	for _, f := range MAIN_FOLDER {
		if fileutils.FileExists(f) {
			if files, err = fileutils.ListFiles(f); err != nil {
				panic(err)
			}
			break
		}
	}

	if len(files) == 0 {
		panic(fmt.Sprintf("No log folder founds in: %s\n", MAIN_FOLDER))
	}

	var sb strings.Builder
	bar := progressbar.Default(int64(len(files)))
	for _, f := range files {
		bar.Add(1)
		data := fileutils.FilterFromFile(f, "Ip:", false)
		if len(data) > 0 {
			for _, d := range data {
				sb.WriteString(d)
			}
		}
	}
	bar.Close()
	re := regexp.MustCompile(`Ip: \d+.\d+.\d+.\d+`)
	match := re.FindAllStringSubmatch(sb.String(), -1)
	var ips []string
	for _, m := range match {
		for _, m1 := range m {
			ips = append(ips, strings.Replace(m1, "Ip: ", "", 1))
		}
	}

	ips = arrayutils.UniqueString(ips)

	if err = ioutil.WriteFile("all_ip.txt", ConcatIp(ips), 0755); err != nil {
		panic(err)
	}

	uniqueIp := FilterUniqueIP(ips)

	data := ConcatIp(uniqueIp)
	if err = ioutil.WriteFile("filtered_ip.txt", data, 0755); err != nil {
		panic(err)
	}

	CheckLatency()
}

func ConcatIp(ips []string) []byte {
	var data []byte
	for _, x := range ips {
		data = append(data, []byte(x)...)
		data = append(data, []byte("\n")...)
	}
	return bytes.Trim(data, "\n")
}

func FilterUniqueIP(ips []string) []string {
	var ipData []IP

	for _, ip := range ips {
		ipData = append(ipData, NewIp(ip))
	}

	var significantIp map[string]struct{} = make(map[string]struct{})
	for _, ip := range ipData {
		significantIp[ip.GetSignificant()] = struct{}{}
	}

	var uniqueIp []string
	for _, ip := range ipData {
		if _, ok := significantIp[ip.GetSignificant()]; ok {
			uniqueIp = append(uniqueIp, ip.Get())
			delete(significantIp, ip.GetSignificant())
		}
	}
	sort.Strings(uniqueIp)
	return uniqueIp
}

// Statistics represent the stats of a currently running or finished
// pinger operation.
type Statistics struct {
	// PacketsRecv is the number of packets received.
	PacketsRecv int

	// PacketsSent is the number of packets sent.
	PacketsSent int

	// PacketsRecvDuplicates is the number of duplicate responses there were to a sent packet.
	PacketsRecvDuplicates int

	// PacketLoss is the percentage of packets lost.
	PacketLoss float64

	// Addr is the string address of the host being pinged.
	Addr string

	// MinRtt is the minimum round-trip time sent via this pinger.
	MinRtt float64

	// MaxRtt is the maximum round-trip time sent via this pinger.
	MaxRtt float64

	// AvgRtt is the average round-trip time sent via this pinger.
	AvgRtt float64

	Location string
}

// FIXME: Have to be taken the server with higer ping
func UniqueStats(stats []Statistics) []Statistics {
	var unique map[string]Statistics = make(map[string]Statistics)
	for _, s := range stats {
		if _, ok := unique[s.Location]; !ok {
			unique[s.Location] = s
		} else {
			if unique[s.Location].MaxRtt > s.MaxRtt || unique[s.Location].PacketLoss > s.PacketLoss {
				unique[s.Location] = s
			}
		}
	}

	var servers []Statistics

	for _, v := range unique {
		servers = append(servers, v)
	}
	return servers
}

func CheckLatency() {
	files := fileutils.FindFiles(".", "_ip.txt", false)
	var ipFiles []string
	var badServers []Statistics
	var packetLoss []Statistics
	var goodServers []Statistics

	for _, f := range files {
		array := fileutils.ReadFileInArray(f)
		ipFiles = append(ipFiles, array...)
	}

	ipFiles = arrayutils.UniqueString(ipFiles)
	sort.Strings(ipFiles)

	dbPath := os.Getenv("ip2location_path")
	nRequest := os.Getenv("n_request")
	intervalS := os.Getenv("interval")
	pingLimitS := os.Getenv("ping")

	if stringutils.IsBlank(nRequest) {
		nRequest = "50"
	}

	if stringutils.IsBlank(intervalS) {
		intervalS = "150"
	}

	if stringutils.IsBlank(pingLimitS) {
		pingLimitS = "100"
	}

	interval, err := strconv.Atoi(intervalS)
	if err != nil {
		panic(err)
	}

	pingLimit, err := strconv.Atoi(pingLimitS)
	if err != nil {
		panic(err)
	}

	if stringutils.IsBlank(dbPath) {
		if fileutils.FileExists("IP2LOCATION-LITE-DB11.BIN") {
			dbPath = "IP2LOCATION-LITE-DB11.BIN"
		} else {
			panic("ip2location_path env var not set!")

		}
	}
	if !fileutils.FileExists(dbPath) {
		panic(dbPath + " path not found")
	}
	db, err := ip2location.OpenDB(dbPath)
	if err != nil {
		fmt.Print(err)
		return
	}

	bar := progressbar.Default(int64(len(ipFiles)))
	defer bar.Close()

	for _, ip := range ipFiles {
		bar.Describe(ip)
		ip = stringutils.Trim(ip)
		pinger, err := ping.NewPinger(ip)
		if err != nil {
			panic(err)
		}

		pinger.Size = 548
		pinger.SetPrivileged(true)
		var s Statistics
		pinger.Count, err = strconv.Atoi(nRequest)
		if err != nil {
			panic(err)
		}
		pinger.Interval = time.Duration(interval) * time.Millisecond
		pinger.Timeout = ((time.Millisecond * time.Duration(pingLimit)) * time.Duration(pinger.Count)) + (time.Duration(pinger.Count) * pinger.Interval)

		if err = pinger.Run(); err != nil {
			panic(err)
		}
		stats := pinger.Statistics()

		s.Addr = stats.Addr
		s.AvgRtt = float64(stats.AvgRtt / time.Millisecond)
		s.MaxRtt = float64(stats.MaxRtt / time.Millisecond)
		s.MinRtt = float64(stats.MinRtt / time.Millisecond)
		s.PacketsRecv = stats.PacketsRecv
		s.PacketLoss = stats.PacketLoss * float64(stats.PacketsSent) / 100.0
		s.PacketsSent = stats.PacketsSent
		s.PacketsRecvDuplicates = stats.PacketsRecvDuplicates

		results, err := db.Get_all(s.Addr)
		if err != nil {
			fmt.Print(err)
			return
		}
		s.Location = results.Country_short + "-" + results.City
		if s.PacketLoss > 0 {
			packetLoss = append(packetLoss, s)
		} else if s.MaxRtt > float64(pingLimit) {
			badServers = append(badServers, s)
		} else {
			goodServers = append(goodServers, s)
		}
		bar.Add(1)
	}

	packetLoss = UniqueStats(packetLoss)
	badServers = UniqueStats(badServers)
	goodServers = UniqueStats(goodServers)

	goodServers = removeServersByLocation(goodServers, badServers...)
	goodServers = removeServersByLocation(goodServers, packetLoss...)
	sort.Slice(packetLoss, func(i, j int) bool {
		return packetLoss[i].MaxRtt < packetLoss[j].MaxRtt
	})

	sort.Slice(badServers, func(i, j int) bool {
		return badServers[i].MaxRtt < badServers[j].MaxRtt
	})
	sort.Slice(goodServers, func(i, j int) bool {
		return goodServers[i].MaxRtt < goodServers[j].MaxRtt
	})

	log.Println("PACKET LOSS:", helper.MarshalIndent(packetLoss))
	log.Println("BAD SERVERS :", helper.MarshalIndent(badServers))
	log.Println("GOOD SERVERS :", helper.MarshalIndent(goodServers))

	if !fileutils.IsDir("result") {
		if err = fileutils.CreateDir("result"); err != nil {
			panic(err)
		}
	}

	var sPacket []byte
	var sBad []byte
	var sGood []byte

	sPacket = append(sPacket, []byte(helper.MarshalIndent(packetLoss))...)
	sBad = append(sBad, []byte(helper.MarshalIndent(badServers))...)
	sGood = append(sGood, []byte(helper.MarshalIndent(goodServers))...)

	now := time.Now().Format("02-01-06_3:04:05")
	fname := path.Join("result", now)
	fileutils.CreateDir(fname)
	if len(goodServers) > 0 {
		ioutil.WriteFile(path.Join(fname, "good_servers.txt"), sGood, 0755)
	}
	if len(badServers) > 0 {
		ioutil.WriteFile(path.Join(fname, "bad_servers.txt"), sBad, 0755)
	}

	if len(packetLoss) > 0 {
		ioutil.WriteFile(path.Join(fname, "packet_loss_servers.txt"), sPacket, 0755)
	}

}

func removeServersByLocation(goodServers []Statistics, wrongServers ...Statistics) []Statistics {

	var goodServerMap map[string]Statistics = make(map[string]Statistics)
	var wrongServerMap map[string]Statistics = make(map[string]Statistics)

	for _, m := range goodServers {
		goodServerMap[m.Location] = m
	}

	for _, m := range wrongServers {
		wrongServerMap[m.Location] = m
	}

	for _, m := range wrongServers {
		delete(goodServerMap, m.Location)
	}

	goodServers = nil

	for _, m := range goodServerMap {
		goodServers = append(goodServers, m)
	}

	return goodServers
}
