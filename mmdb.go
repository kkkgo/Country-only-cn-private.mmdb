package main

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/netip"
	"os"
	"regexp"
	"strconv"
	"strings"

	"go4.org/netipx"

	"mmdb/chinaboundary"

	"github.com/maxmind/mmdbwriter"
	"github.com/maxmind/mmdbwriter/mmdbtype"
	"github.com/oschwald/maxminddb-golang"
	"github.com/v2fly/v2ray-core/v5/app/router/routercommon"
	"github.com/v2fly/v2ray-core/v5/common/strmatcher"
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

type testCase struct {
	IP       string
	Expected string // "CN", "PRIVATE", "CLOUDFLARE", or "" for none
}

var testIPs = []testCase{
	// CN IPv4
	{"114.114.114.114", "CN"},
	{"119.29.29.29", "CN"},
	{"223.5.5.5", "CN"},
	{"180.76.76.76", "CN"},
	{"101.226.4.6", "CN"},
	{"218.30.118.6", "CN"},
	{"123.125.81.6", "CN"},
	{"140.207.198.6", "CN"},
	{"1.2.4.8", "CN"},
	{"117.50.10.10", "CN"},
	{"52.80.52.52", "CN"},
	// CN IPv6
	{"2400:3200:baba::1", "CN"},
	{"2402:4e00::1", "CN"},
	{"2400:da00::6666", "CN"},
	{"240e:4c:4008::1", "CN"},
	{"2408:8899::8", "CN"},
	{"2409:8088::a", "CN"},
	{"240C::6666", "CN"},
	{"2001:dc7:1000::1", "CN"},
	{"2001:da8:8000:1:202:120:2:100", "CN"},
	{"2001:cc0:2fff:1::6666", "CN"},
	{"2001:da8:208:10::6", "CN"},
	// PRIVATE
	{"221.228.32.13", "PRIVATE"},
	{"183.192.65.101", "PRIVATE"},
	// CLOUDFLARE IPv4
	{"104.21.21.239", "CLOUDFLARE"},
	{"172.67.201.108", "CLOUDFLARE"},
	// CLOUDFLARE IPv6
	{"2606:4700:3037::ac43:c96c", "CLOUDFLARE"},
	{"2606:4700:3034::6815:15ef", "CLOUDFLARE"},
	// Non-matching
	{"8.8.8.8", ""},
	{"2a09:bac1:19f0::1", ""},
}

func checkMMDB(filename string) error {
	db, err := maxminddb.Open(filename)
	if err != nil {
		return fmt.Errorf("failed to open mmdb: %v", err)
	}
	defer db.Close()

	var record struct {
		Country struct {
			ISOCode string `maxminddb:"iso_code"`
		} `maxminddb:"country"`
	}

	failed := 0
	for _, tc := range testIPs {
		ip := net.ParseIP(tc.IP)
		if ip == nil {
			return fmt.Errorf("invalid test IP: %s", tc.IP)
		}
		err := db.Lookup(ip, &record)
		if err != nil {
			return fmt.Errorf("lookup %s failed: %v", tc.IP, err)
		}
		got := record.Country.ISOCode
		if got != tc.Expected {
			fmt.Printf("FAIL: %s expected %q, got %q\n", tc.IP, tc.Expected, got)
			failed++
		} else if tc.Expected != "" {
			fmt.Printf("PASS: %s -> %s\n", tc.IP, got)
		} else {
			fmt.Printf("PASS: %s -> %q (no match)\n", tc.IP, got)
		}
		record.Country.ISOCode = ""
	}
	if failed > 0 {
		return fmt.Errorf("%d test(s) failed", failed)
	}
	return nil
}

func cidrToIPSet(cidrs []*routercommon.CIDR) (*netipx.IPSet, error) {
	var b netipx.IPSetBuilder
	for _, cidr := range cidrs {
		addr, ok := netip.AddrFromSlice(cidr.Ip)
		if !ok {
			continue
		}
		b.AddPrefix(netip.PrefixFrom(addr.Unmap(), int(cidr.Prefix)))
	}
	return b.IPSet()
}

func checkDat(filename string) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read dat file: %v", err)
	}
	var geoIPList routercommon.GeoIPList
	if err := proto.Unmarshal(data, &geoIPList); err != nil {
		return fmt.Errorf("failed to unmarshal dat: %v", err)
	}

	// Build per-code IPSets from routercommon.CIDR slices
	codeSets := make(map[string]*netipx.IPSet)
	for _, entry := range geoIPList.Entry {
		code := strings.ToUpper(entry.CountryCode)
		s, err := cidrToIPSet(entry.Cidr)
		if err != nil {
			return fmt.Errorf("build IP set for %s: %v", code, err)
		}
		codeSets[code] = s
	}

	// Verify expected entries exist
	for _, code := range []string{"CN", "PRIVATE", "CLOUDFLARE"} {
		if _, ok := codeSets[code]; !ok {
			return fmt.Errorf("no %q entry found in dat file", code)
		}
	}

	// lookupCode checks ip against each code in priority order.
	// PRIVATE > CLOUDFLARE > CN mirrors the specificity order in the dat file
	// (PRIVATE entries include host /32s that overlap with CN ranges).
	lookupCode := func(ip net.IP) string {
		a, ok := netip.AddrFromSlice(ip)
		if !ok {
			return ""
		}
		a = a.Unmap()
		for _, code := range []string{"PRIVATE", "CLOUDFLARE", "CN"} {
			if s, ok := codeSets[code]; ok && s.Contains(a) {
				return code
			}
		}
		return ""
	}

	failed := 0
	for _, tc := range testIPs {
		ip := net.ParseIP(tc.IP)
		if ip == nil {
			return fmt.Errorf("invalid test IP: %s", tc.IP)
		}
		got := lookupCode(ip)
		expected := strings.ToUpper(tc.Expected)
		if got != expected {
			fmt.Printf("FAIL: %s expected %q, got %q\n", tc.IP, expected, got)
			failed++
		} else if expected != "" {
			fmt.Printf("PASS: %s -> %s\n", tc.IP, got)
		} else {
			fmt.Printf("PASS: %s -> %q (no match)\n", tc.IP, got)
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d test(s) failed", failed)
	}
	return nil
}

func mmdbLocal(cidr string) {
	_, IPNet, err := net.ParseCIDR(cidr)
	if err != nil {
		log.Fatalf("invalid private CIDR %s: %v", cidr, err)
	}
	writer.Insert(IPNet, PRIVATE)

	ip := IPNet.IP
	if ip4 := ip.To4(); ip4 != nil {
		ip = ip4
	}
	prefix, _ := IPNet.Mask.Size()
	privateCIDRs = append(privateCIDRs, &routercommon.CIDR{
		Ip:     ip,
		Prefix: uint32(prefix),
	})
}

func mmdbInsert(cidr string) bool {
	_, IPNet, err := net.ParseCIDR(cidr)
	if err != nil {
		log.Fatal(err)
		return false
	}
	if err := writer.Insert(IPNet, CN); err != nil {
		log.Fatal(err)
		return false
	}

	ip := IPNet.IP
	if ip4 := ip.To4(); ip4 != nil {
		ip = ip4
	}
	prefix, _ := IPNet.Mask.Size()
	cnCIDRs = append(cnCIDRs, &routercommon.CIDR{
		Ip:     ip,
		Prefix: uint32(prefix),
	})

	return true
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
	mmdbLocal("221.228.32.13/32")  //jsfz
	mmdbLocal("183.192.65.101/32") //shfz
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

		if len(record) < 9 {
			continue
		}

		if record[1] == "1814991" && record[2] == "1814991" {
			cidr := record[0]
			mmdbInsert(cidr)
		} else if record[1] != "1814991" && record[2] == "1814991" {

			lat, errLat := strconv.ParseFloat(record[7], 64)
			lng, errLng := strconv.ParseFloat(record[8], 64)

			if errLat == nil && errLng == nil && chinaboundary.IsCN(lat, lng) {
				cidr := record[0]
				mmdbInsert(cidr)
			}
		}
	}
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

	if resp.StatusCode != http.StatusOK {
		log.Fatalf("cloudflare api returned HTTP %d", resp.StatusCode)
	}

	var data cfIPsResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		log.Fatalf("decode cloudflare ips: %v", err)
	}
	if !data.Success {
		log.Fatal("cloudflare api returned success=false")
	}

	totalCIDRs := len(data.Result.IPv4CIDRs) + len(data.Result.IPv6CIDRs)
	if totalCIDRs < 10 || totalCIDRs > 100 {
		log.Fatalf("cloudflare CIDR count out of expected range: %d (expected 10-100)", totalCIDRs)
	}

	for _, cidr := range data.Result.IPv4CIDRs {
		mmdbCloudflare(cidr)
	}
	for _, cidr := range data.Result.IPv6CIDRs {
		mmdbCloudflare(cidr)
	}
}


func parseGlobalMark(filename string) (global, cnRule, cnMarked []string, err error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, nil, nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		switch {
		case strings.HasPrefix(line, "##@@domain:"):
			domain := strings.TrimPrefix(line, "##@@")
			cnMarked = append(cnMarked, domain)
		case strings.HasPrefix(line, "#@domain:"):
			domain := strings.TrimPrefix(line, "#@")
			cnRule = append(cnRule, domain)
		case strings.HasPrefix(line, "domain:"):
			global = append(global, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, nil, err
	}
	return global, cnRule, cnMarked, nil
}

func domainsToGeoSite(tag string, domainLines []string) *routercommon.GeoSite {
	var domains []*routercommon.Domain
	for _, line := range domainLines {
		value := strings.TrimPrefix(line, "domain:")
		if value == "" {
			continue
		}
		domains = append(domains, &routercommon.Domain{
			Type:  routercommon.Domain_RootDomain,
			Value: value,
		})
	}
	return &routercommon.GeoSite{
		CountryCode: strings.ToUpper(tag),
		Domain:      domains,
	}
}

func injectGlobalMark(geositePath, globalMarkPath string) error {
	global, cnRule, cnMarked, err := parseGlobalMark(globalMarkPath)
	if err != nil {
		return fmt.Errorf("parse global_mark: %v", err)
	}
	if len(global) == 0 && len(cnRule) == 0 && len(cnMarked) == 0 {
		return fmt.Errorf("global_mark file is empty or contains no recognized entries")
	}
	fmt.Printf("Parsed global_mark: global=%d, cn_rule=%d, cn_marked=%d\n",
		len(global), len(cnRule), len(cnMarked))

	data, err := os.ReadFile(geositePath)
	if err != nil {
		return fmt.Errorf("read geosite.dat: %v", err)
	}
	var list routercommon.GeoSiteList
	if err := proto.Unmarshal(data, &list); err != nil {
		return fmt.Errorf("unmarshal geosite.dat: %v", err)
	}

	paoTags := map[string]bool{
		"PAOPAODNS_GLOBAL_MARK": true,
		"PAOPAODNS_CN_MARK":     true,
		"PAOPAODNS_SKIP_MARK":   true,
	}
	var filtered []*routercommon.GeoSite
	for _, entry := range list.Entry {
		if !paoTags[strings.ToUpper(entry.CountryCode)] {
			filtered = append(filtered, entry)
		}
	}
	list.Entry = filtered

	list.Entry = append(list.Entry,
		domainsToGeoSite("PAOPAODNS_GLOBAL_MARK", global),
		domainsToGeoSite("PAOPAODNS_CN_MARK", cnRule),
		domainsToGeoSite("PAOPAODNS_SKIP_MARK", cnMarked),
	)

	outData, err := proto.Marshal(&list)
	if err != nil {
		return fmt.Errorf("marshal geosite.dat: %v", err)
	}
	if err := os.WriteFile(geositePath, outData, 0644); err != nil {
		return fmt.Errorf("write geosite.dat: %v", err)
	}
	return nil
}

func buildGfwMatcher(list *routercommon.GeoSiteList) (strmatcher.IndexMatcher, uint32, error) {
	m := strmatcher.NewMphIndexMatcher()
	var total uint32
	for _, site := range list.Entry {
		if strings.ToUpper(site.CountryCode) != "GFW" {
			continue
		}
		for _, d := range site.Domain {
			var t strmatcher.Type
			switch d.Type {
			case routercommon.Domain_Full:
				t = strmatcher.Full
			case routercommon.Domain_RootDomain:
				t = strmatcher.Domain
			case routercommon.Domain_Regex:
				t = strmatcher.Regex
			case routercommon.Domain_Plain:
				t = strmatcher.Substr
			default:
				continue
			}
			matcher, err := t.NewDomainPattern(d.Value)
			if err != nil {
				return nil, 0, fmt.Errorf("build gfw matcher for %q: %v", d.Value, err)
			}
			m.Add(matcher)
			total++
		}
	}
	if total == 0 {
		return nil, 0, fmt.Errorf("geosite:gfw not found or empty")
	}
	if err := m.Build(); err != nil {
		return nil, 0, fmt.Errorf("build gfw index matcher: %v", err)
	}
	return m, total, nil
}

func injectGfwFull(geositePath, topdomainsPath string) error {
	data, err := os.ReadFile(geositePath)
	if err != nil {
		return fmt.Errorf("read geosite.dat: %v", err)
	}
	var list routercommon.GeoSiteList
	if err := proto.Unmarshal(data, &list); err != nil {
		return fmt.Errorf("unmarshal geosite.dat: %v", err)
	}

	gfwMatcher, total, err := buildGfwMatcher(&list)
	if err != nil {
		return err
	}
	fmt.Printf("geosite:gfw: %d rules loaded into MphIndexMatcher\n", total)

	// Filter topdomains against geosite:gfw
	f, err := os.Open(topdomainsPath)
	if err != nil {
		return fmt.Errorf("open topdomains: %v", err)
	}
	defer f.Close()

	var gfwFullDomainList []*routercommon.Domain
	seen := make(map[string]bool)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		domain := strings.TrimSpace(strings.ToLower(scanner.Text()))
		if domain == "" || seen[domain] {
			continue
		}
		seen[domain] = true
		if gfwMatcher.MatchAny(domain) {
			gfwFullDomainList = append(gfwFullDomainList, &routercommon.Domain{
				Type:  routercommon.Domain_Full,
				Value: domain,
			})
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan topdomains: %v", err)
	}
	fmt.Printf("PAOPAODNS_GFWFULL: matched %d domains from topdomains\n", len(gfwFullDomainList))
	if len(gfwFullDomainList) == 0 {
		return fmt.Errorf("no topdomains matched geosite:gfw — check if gfw rules or topdomains format changed")
	}

	// Remove existing PAOPAODNS_GFWFULL entry (idempotent)
	var filtered []*routercommon.GeoSite
	for _, entry := range list.Entry {
		if strings.ToUpper(entry.CountryCode) != "PAOPAODNS_GFWFULL" {
			filtered = append(filtered, entry)
		}
	}
	list.Entry = append(filtered, &routercommon.GeoSite{
		CountryCode: "PAOPAODNS_GFWFULL",
		Domain:      gfwFullDomainList,
	})

	outData, err := proto.Marshal(&list)
	if err != nil {
		return fmt.Errorf("marshal geosite.dat: %v", err)
	}
	if err := os.WriteFile(geositePath, outData, 0644); err != nil {
		return fmt.Errorf("write geosite.dat: %v", err)
	}
	return nil
}

func checkGeosite(filename string, mandatoryTags []string) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return err
	}
	var list routercommon.GeoSiteList
	if err := proto.Unmarshal(data, &list); err != nil {
		return err
	}

	foundTags := make(map[string]bool)
	for _, tag := range mandatoryTags {
		tr := strings.TrimSpace(tag)
		if tr != "" {
			foundTags[strings.ToUpper(tr)] = false
		}
	}

	for _, site := range list.Entry {
		siteCode := strings.ToUpper(site.CountryCode)
		if _, ok := foundTags[siteCode]; ok {
			foundTags[siteCode] = true
		}

		for _, domain := range site.Domain {
			if domain.Type == routercommon.Domain_Regex {
				_, err := regexp.Compile(domain.Value)
				if err != nil {
					return fmt.Errorf("invalid regex in [%s]: %s - %v", site.CountryCode, domain.Value, err)
				}
			}
		}
	}

	for tag, found := range foundTags {
		if !found {
			return fmt.Errorf("geosite:%s not found", tag)
		}
	}

	return nil
}

func main() {
	checkPath := flag.String("check-geosite", "", "check geosite.dat file")
	checkTags := flag.String("check-tags", "TRACKER,CATEGORY-PUBLIC-TRACKER", "comma separated list of mandatory tags to check")
	checkMMDBPath := flag.String("check-mmdb", "", "check mmdb file with test IPs")
	checkDatPath := flag.String("check-dat", "", "check dat file with test IPs")
	injectGeositePath := flag.String("inject-geosite", "", "path to geosite.dat to inject new tags into")
	globalMarkPath := flag.String("global-mark", "", "path to decompressed global_mark text file")
	injectGfwFullPath := flag.String("inject-gfw-full", "", "path to geosite.dat to inject PAOPAODNS_GFWFULL tag")
	topdomainsPath := flag.String("topdomains", "", "path to topdomains file (one domain per line)")
	flag.Parse()

	if *checkPath != "" {
		var tags []string
		if *checkTags != "" {
			tags = strings.Split(*checkTags, ",")
		}
		if err := checkGeosite(*checkPath, tags); err != nil {
			log.Fatalf("geosite check failed: %v", err)
		}
		fmt.Printf("geosite check passed: %s\n", *checkPath)
		return
	}

	if *injectGeositePath != "" {
		if *globalMarkPath == "" {
			log.Fatal("-global-mark is required with -inject-geosite")
		}
		if err := injectGlobalMark(*injectGeositePath, *globalMarkPath); err != nil {
			log.Fatalf("inject global mark failed: %v", err)
		}
		fmt.Printf("global_mark tags injected into: %s\n", *injectGeositePath)
		return
	}

	if *injectGfwFullPath != "" {
		if *topdomainsPath == "" {
			log.Fatal("-topdomains is required with -inject-gfw-full")
		}
		if err := injectGfwFull(*injectGfwFullPath, *topdomainsPath); err != nil {
			log.Fatalf("inject gfw full failed: %v", err)
		}
		fmt.Printf("PAOPAODNS_GFWFULL injected into: %s\n", *injectGfwFullPath)
		return
	}

	if *checkMMDBPath != "" {
		if err := checkMMDB(*checkMMDBPath); err != nil {
			log.Fatalf("mmdb check failed: %v", err)
		}
		fmt.Printf("mmdb check passed: %s\n", *checkMMDBPath)
		return
	}

	if *checkDatPath != "" {
		if err := checkDat(*checkDatPath); err != nil {
			log.Fatalf("dat check failed: %v", err)
		}
		fmt.Printf("dat check passed: %s\n", *checkDatPath)
		return
	}

	var err error
	writer, err = mmdbwriter.New(
		mmdbwriter.Options{
			IncludeReservedNetworks: true,
			DatabaseType:            "GeoLite2-Country",
			RecordSize:              24,
			Description:             map[string]string{"en": "GeoLite2 Country database"},
		})
	if err != nil {
		log.Fatal(err)
	}
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
		fh.Close()
		log.Fatal(err)
	}
	if err := fh.Close(); err != nil {
		log.Fatal(err)
	}

	cnGeoIP := &routercommon.GeoIP{
		CountryCode: "cn",
		Cidr:        cnCIDRs,
	}
	privateGeoIP := &routercommon.GeoIP{
		CountryCode: "private",
		Cidr:        privateCIDRs,
	}
	cfGeoIP := &routercommon.GeoIP{
		CountryCode: "cloudflare",
		Cidr:        cfCIDRs,
	}
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
