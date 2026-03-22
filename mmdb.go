package main

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"

	"mmdb/chinaboundary"

	"github.com/maxmind/mmdbwriter"
	"github.com/maxmind/mmdbwriter/mmdbtype"
	"github.com/v2fly/v2ray-core/v5/app/router/routercommon"
	"google.golang.org/protobuf/proto"
)

var writer *mmdbwriter.Tree
var cnCIDRs []*routercommon.CIDR
var privateCIDRs []*routercommon.CIDR
var cfCIDRs []*routercommon.CIDR

var CN = mmdbtype.Map{
	"country": mmdbtype.Map{"iso_code": mmdbtype.String("CN")},
}
var PRIVATE = mmdbtype.Map{
	"country": mmdbtype.Map{"iso_code": mmdbtype.String("PRIVATE")},
}
var CLOUDFLARE = mmdbtype.Map{
	"country": mmdbtype.Map{"iso_code": mmdbtype.String("CLOUDFLARE")},
}


func mmdbCloudflare(cidr string) bool {
	_, IPNet, err := net.ParseCIDR(cidr)
	if err != nil {
		log.Printf("invalid cloudflare cidr %s: %v", cidr, err)
		return false
	}
	if err := writer.Insert(IPNet, CLOUDFLARE); err != nil {
		log.Fatal(err)
		return false
	}

	ip := IPNet.IP
	if ip4 := ip.To4(); ip4 != nil {
		ip = ip4
	}
	prefix, _ := IPNet.Mask.Size()
	cfCIDRs = append(cfCIDRs, &routercommon.CIDR{
		Ip:     ip,
		Prefix: uint32(prefix),
	})
	return true
}

type cfIPsResponse struct {
	Result struct {
		IPv4CIDRs []string `json:"ipv4_cidrs"`
		IPv6CIDRs []string `json:"ipv6_cidrs"`
	} `json:"result"`
	Success bool `json:"success"`
}

func importCloudflare() {
	resp, err := http.Get("https://api.cloudflare.com/client/v4/ips")
	if err != nil {
		log.Fatalf("fetch cloudflare ips: %v", err)
	}
	defer resp.Body.Close()

	var data cfIPsResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		log.Fatalf("decode cloudflare ips: %v", err)
	}
	if !data.Success {
		log.Fatal("cloudflare api returned success=false")
	}

	for _, cidr := range data.Result.IPv4CIDRs {
		mmdbCloudflare(cidr)
	}
	for _, cidr := range data.Result.IPv6CIDRs {
		mmdbCloudflare(cidr)
	}
}

func main() {
	writer, _ = mmdbwriter.New(
		mmdbwriter.Options{
			IncludeReservedNetworks: true,
			DatabaseType:            "GeoLite2-Country",
			RecordSize:              24,
			Description:             map[string]string{"en": "GeoLite2 Country database"},
		})
	importCSV("/tmp/data/GeoLite2-City-Blocks-IPv6.csv")
	importCSV("/tmp/data/GeoLite2-City-Blocks-IPv4.csv")
	importTXT("/tmp/data/china6.txt")
	importTXT("/tmp/data/china_ip_list.txt")
	importLocal()
	importCloudflare()
	fh, err := os.Create("/tmp/Country-only-cn-private.mmdb")
	if err != nil {
		log.Fatal(err)
	}
	_, err = writer.WriteTo(fh)
	if err != nil {
		log.Fatal(err)
	}

	cnGeoIP := &routercommon.GeoIP{CountryCode: "cn", Cidr: cnCIDRs}
	privateGeoIP := &routercommon.GeoIP{CountryCode: "private", Cidr: privateCIDRs}
	cfGeoIP := &routercommon.GeoIP{CountryCode: "cloudflare", Cidr: cfCIDRs}
	geoIPList := &routercommon.GeoIPList{
		Entry: []*routercommon.GeoIP{cnGeoIP, privateGeoIP, cfGeoIP},
	}
	datBytes, err := proto.Marshal(geoIPList)
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile("/tmp/CN-local.dat", datBytes, 0644); err != nil {
		log.Fatal(err)
	}
}
