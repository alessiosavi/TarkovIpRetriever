package main

import (
	"bytes"
	"fmt"
	fileutils "github.com/alessiosavi/GoGPUtils/files"
	stringutils "github.com/alessiosavi/GoGPUtils/string"
	"github.com/go-ping/ping"
	"github.com/ip2location/ip2location-go/v9"
	"github.com/schollz/progressbar/v3"
	"io/ioutil"
	"log"
	"os"
	"regexp"
	"sort"
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
	log.SetFlags(log.LstdFlags | log.Llongfile | log.Ltime)
	var files []string
	var err error

	for _, f := range MAIN_FOLDER {
		if fileutils.FileExists(f) {
			if files, err = fileutils.ListFile(f); err != nil {
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

	ips = UniqueString(ips)

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

func UniqueString(slice []string) []string {
	var m map[string]struct{} = make(map[string]struct{})
	for _, x := range slice {
		m[x] = struct{}{}
	}
	slice = []string{}
	for x := range m {
		slice = append(slice, x)
	}

	sort.Strings(slice)
	return slice
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
}

func CheckLatency() {
	files := fileutils.FindFiles(".", "_ip.txt", false)
	var ipFiles []string
	var badServers []string
	var packetLoss []string
	var goodServers []string
	for _, f := range files {
		array := fileutils.ReadFileInArray(f)
		ipFiles = append(ipFiles, array...)
	}

	ipFiles = UniqueString(ipFiles)
	sort.Strings(ipFiles)

	dbPath := os.Getenv("ip2location_path")
	if stringutils.IsBlank(dbPath) {
		panic("ip2location_path env var not set!")
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

	for _, ip := range ipFiles {
		bar.Describe(ip)
		bar.Add(1)
		ip = stringutils.Trim(ip)
		pinger, err := ping.NewPinger(ip)
		if err != nil {
			panic(err)
		}
		var s Statistics
		pinger.Count = 50
		pinger.Interval = 200 * time.Millisecond

		pinger.Timeout = ((time.Millisecond * 140) * time.Duration(pinger.Count)) + (time.Duration(pinger.Count) * pinger.Interval)
		err = pinger.Run()
		if err != nil {
			panic(err)
		}
		stats := pinger.Statistics()

		s.Addr = stats.Addr
		s.AvgRtt = float64(stats.AvgRtt) / float64(time.Millisecond)
		s.MaxRtt = float64(stats.MaxRtt) / float64(time.Millisecond)
		s.MinRtt = float64(stats.MinRtt) / float64(time.Millisecond)
		s.PacketsRecv = stats.PacketsRecv
		s.PacketLoss = stats.PacketLoss
		s.PacketsSent = stats.PacketsSent
		s.PacketsRecvDuplicates = stats.PacketsRecvDuplicates

		results, err := db.Get_all(s.Addr)
		if err != nil {
			fmt.Print(err)
			return
		}
		if s.PacketLoss > 0 || s.MaxRtt > 100 {
			if s.PacketLoss > 0 {
				packetLoss = append(packetLoss, results.Country_short+"-"+results.City)
			}
			badServers = append(badServers, results.Country_short+"-"+results.City)
		} else {
			goodServers = append(goodServers, results.Country_short+"-"+results.City)
		}
	}
	bar.Close()
	log.Println("PACKET LOSS:", UniqueString(packetLoss))
	log.Println("BAD SERVERS :", UniqueString(badServers))
	log.Println("GOOD SERVERS :", UniqueString(goodServers))
	ioutil.WriteFile("good_servers.txt", []byte(stringutils.JoinSeparator("\n", goodServers...)), 0755)
	ioutil.WriteFile("bad_servers.txt", []byte(stringutils.JoinSeparator("\n", badServers...)), 0755)
	ioutil.WriteFile("packet_loss_servers.txt", []byte(stringutils.JoinSeparator("\n", packetLoss...)), 0755)

}
