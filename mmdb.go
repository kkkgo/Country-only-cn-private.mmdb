package main

import (
	"bufio"
	"encoding/csv"
	"io"
	"log"
	"net"
	"os"
	"strings"

	"github.com/maxmind/mmdbwriter"
	"github.com/maxmind/mmdbwriter/inserter"
	"github.com/maxmind/mmdbwriter/mmdbtype"
)

var writer *mmdbwriter.Tree
var CN = mmdbtype.Map{
	"country": mmdbtype.Map{"iso_code": mmdbtype.String("CN")},
}
var PRIVATE = mmdbtype.Map{
	"country": mmdbtype.Map{"iso_code": mmdbtype.String("PRIVATE")},
}

func mmdbLocal(cidr string) {
	_, IP, _ := net.ParseCIDR(cidr)
	writer.InsertFunc(IP, inserter.TopLevelMergeWith(PRIVATE))
}
func mmdbInsert(cidr string) bool {
	_, IP, err := net.ParseCIDR(cidr)
	if err != nil {
		log.Fatal(err)
		return false
	}
	if err := writer.InsertFunc(IP, inserter.TopLevelMergeWith(CN)); err != nil {
		log.Fatal(err)
		return false
	}
	return true
}
func importLocal() {
	mmdbLocal("0.0.0.0/8")
	mmdbLocal("10.0.0.0/8")
	mmdbLocal("100.64.0.0/10")
	mmdbLocal("127.0.0.0/8")
	mmdbLocal("169.254.0.0/16")
	mmdbLocal("172.16.0.0/12")
	mmdbLocal("192.0.0.0/24")
	mmdbLocal("192.0.2.0/24")
	mmdbLocal("192.88.99.0/24")
	mmdbLocal("192.168.0.0/16")
	mmdbLocal("198.18.0.0/15")
	mmdbLocal("198.51.100.0/24")
	mmdbLocal("203.0.113.0/24")
	mmdbLocal("224.0.0.0/4")
	mmdbLocal("240.0.0.0/4")
	mmdbLocal("255.255.255.255/32")
	mmdbLocal("::1/128")
	mmdbLocal("fc00::/7")
	mmdbLocal("fe80::/10")
}
func importTXT(filename string) {
	file, err := os.Open(filename)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) > 0 {
			cidr := fields[0]
			mmdbInsert(cidr)
		}
	}
	if err := scanner.Err(); err != nil {
		log.Fatal(err)
	}
}
func importCSV(filename string) {
	file, err := os.Open(filename)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()
	reader := csv.NewReader(file)

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatal(err)
		}
		if record[1] == "1814991" {
			cidr := record[0]
			mmdbInsert(cidr)
		}
	}
}
func main() {
	Description := make(map[string]string)
	Description["CN"] = "CN"
	Description["PRIVATE"] = "PRIVATE"
	writer, _ = mmdbwriter.New(
		mmdbwriter.Options{
			IncludeReservedNetworks: true,
			DatabaseType:            "mmdb",
			DisableIPv4Aliasing:     true,
			IPVersion:               6,
			RecordSize:              24,
			Description:             Description,
		})
	importLocal()
	importTXT("/tmp/data/china_ip_list.txt")
	importCSV("/tmp/data/GeoLite2-Country-Blocks-IPv6.csv")
	fh, err := os.Create("/tmp/Country-only-cn-private.mmdb")
	if err != nil {
		log.Fatal(err)
	}
	_, err = writer.WriteTo(fh)
	if err != nil {
		log.Fatal(err)
	}
}
