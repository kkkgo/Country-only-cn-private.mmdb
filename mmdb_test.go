package main

import (
	"net"
	"net/netip"
	"os"
	"strings"
	"testing"

	"github.com/maxmind/mmdbwriter"
	"github.com/v2fly/v2ray-core/v5/app/router/routercommon"
	"google.golang.org/protobuf/proto"
)

func TestBuildGfwMatcher(t *testing.T) {
	list := &routercommon.GeoSiteList{
		Entry: []*routercommon.GeoSite{
			{
				CountryCode: "GFW",
				Domain: []*routercommon.Domain{
					{Type: routercommon.Domain_RootDomain, Value: "twitter.com"},
					{Type: routercommon.Domain_Full, Value: "www.google.com"},
				},
			},
		},
	}

	m, total, err := buildGfwMatcher(list)
	if err != nil {
		t.Fatalf("buildGfwMatcher failed: %v", err)
	}
	if total != 2 {
		t.Errorf("expected total=2, got %d", total)
	}

	cases := []struct {
		domain string
		expect bool
	}{
		{"twitter.com", true},             // exact root domain
		{"mobile.twitter.com", true},      // subdomain of root
		{"deep.mobile.twitter.com", true}, // deeper subdomain
		{"nottwitter.com", false},         // no suffix match
		{"www.google.com", true},          // exact full match
		{"sub.www.google.com", false},     // full does not match subdomains
		{"google.com", false},             // parent of full — no match
		{"other.org", false},
	}
	for _, c := range cases {
		got := m.MatchAny(c.domain)
		if got != c.expect {
			t.Errorf("MatchAny(%q) = %v, want %v", c.domain, got, c.expect)
		}
	}
}

func TestInjectGfwFull(t *testing.T) {
	// geosite.dat with GFW entry: root domain:twitter.com and full:www.google.com
	original := &routercommon.GeoSiteList{
		Entry: []*routercommon.GeoSite{
			{
				CountryCode: "GFW",
				Domain: []*routercommon.Domain{
					{Type: routercommon.Domain_RootDomain, Value: "twitter.com"},
					{Type: routercommon.Domain_Full, Value: "www.google.com"},
				},
			},
		},
	}
	origData, err := proto.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}

	geositeFile, err := os.CreateTemp("", "geosite_gfw_test_*.dat")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(geositeFile.Name())
	if err := os.WriteFile(geositeFile.Name(), origData, 0644); err != nil {
		t.Fatal(err)
	}

	// topdomains: twitter.com and mobile.twitter.com match gfw root;
	// www.google.com matches gfw full; google.com and example.com do not match.
	topContent := "twitter.com\nmobile.twitter.com\nwww.google.com\ngoogle.com\nexample.com\n"
	topFile, err := os.CreateTemp("", "topdomains_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(topFile.Name())
	if _, err := topFile.WriteString(topContent); err != nil {
		t.Fatal(err)
	}
	topFile.Close()

	if err := injectGfwFull(geositeFile.Name(), topFile.Name()); err != nil {
		t.Fatalf("injectGfwFull failed: %v", err)
	}

	data, err := os.ReadFile(geositeFile.Name())
	if err != nil {
		t.Fatal(err)
	}
	var list routercommon.GeoSiteList
	if err := proto.Unmarshal(data, &list); err != nil {
		t.Fatal(err)
	}

	var gfwFull *routercommon.GeoSite
	for _, entry := range list.Entry {
		if strings.ToUpper(entry.CountryCode) == "PAOPAODNS_GFWFULL" {
			gfwFull = entry
			break
		}
	}
	if gfwFull == nil {
		t.Fatal("PAOPAODNS_GFWFULL not found in geosite.dat")
	}

	expected := map[string]bool{
		"twitter.com":        true,
		"mobile.twitter.com": true,
		"www.google.com":     true,
	}
	if len(gfwFull.Domain) != len(expected) {
		names := make([]string, 0, len(gfwFull.Domain))
		for _, d := range gfwFull.Domain {
			names = append(names, d.Value)
		}
		t.Fatalf("expected %d domains, got %d: %v", len(expected), len(gfwFull.Domain), names)
	}
	for _, d := range gfwFull.Domain {
		if d.Type != routercommon.Domain_Full {
			t.Errorf("domain %s: expected Full type, got %v", d.Value, d.Type)
		}
		if !expected[d.Value] {
			t.Errorf("unexpected domain in PAOPAODNS_GFWFULL: %s", d.Value)
		}
	}

	// Idempotency: second call must not duplicate the tag
	if err := injectGfwFull(geositeFile.Name(), topFile.Name()); err != nil {
		t.Fatalf("second injectGfwFull failed: %v", err)
	}
	data2, err := os.ReadFile(geositeFile.Name())
	if err != nil {
		t.Fatal(err)
	}
	var list2 routercommon.GeoSiteList
	if err := proto.Unmarshal(data2, &list2); err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, entry := range list2.Entry {
		if strings.ToUpper(entry.CountryCode) == "PAOPAODNS_GFWFULL" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("idempotency: expected 1 PAOPAODNS_GFWFULL entry, got %d", count)
	}
}

func TestParseGlobalMark(t *testing.T) {
	content := `domain:google.com
domain:github.com

#@domain:baidu.com
#@domain:qq.com

##@@domain:example-cdn.com
##@@domain:another-cdn.com
`
	tmpFile, err := os.CreateTemp("", "global_mark_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()

	global, cnRule, cnMarked, err := parseGlobalMark(tmpFile.Name())
	if err != nil {
		t.Fatalf("parseGlobalMark failed: %v", err)
	}

	if len(global) != 2 {
		t.Errorf("expected 2 global entries, got %d", len(global))
	}
	if global[0] != "domain:google.com" || global[1] != "domain:github.com" {
		t.Errorf("unexpected global entries: %v", global)
	}

	if len(cnRule) != 2 {
		t.Errorf("expected 2 cnRule entries, got %d", len(cnRule))
	}
	if cnRule[0] != "domain:baidu.com" || cnRule[1] != "domain:qq.com" {
		t.Errorf("unexpected cnRule entries: %v", cnRule)
	}

	if len(cnMarked) != 2 {
		t.Errorf("expected 2 cnMarked entries, got %d", len(cnMarked))
	}
	if cnMarked[0] != "domain:example-cdn.com" || cnMarked[1] != "domain:another-cdn.com" {
		t.Errorf("unexpected cnMarked entries: %v", cnMarked)
	}
}

func TestParseGlobalMarkPrefixOrder(t *testing.T) {
	// ##@@domain: must NOT be misclassified as #@domain:
	content := "##@@domain:overlap.com\n#@domain:normal.com\n"

	tmpFile, err := os.CreateTemp("", "global_mark_prefix_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()

	global, cnRule, cnMarked, err := parseGlobalMark(tmpFile.Name())
	if err != nil {
		t.Fatalf("parseGlobalMark failed: %v", err)
	}

	if len(global) != 0 {
		t.Errorf("expected 0 global entries, got %d", len(global))
	}
	if len(cnRule) != 1 || cnRule[0] != "domain:normal.com" {
		t.Errorf("expected cnRule=[domain:normal.com], got %v", cnRule)
	}
	if len(cnMarked) != 1 || cnMarked[0] != "domain:overlap.com" {
		t.Errorf("expected cnMarked=[domain:overlap.com], got %v", cnMarked)
	}
}

func TestDomainsToGeoSite(t *testing.T) {
	lines := []string{"domain:example.com", "domain:test.org"}
	site := domainsToGeoSite("paopaodns_global_mark", lines)

	if site.CountryCode != "PAOPAODNS_GLOBAL_MARK" {
		t.Errorf("expected tag PAOPAODNS_GLOBAL_MARK, got %s", site.CountryCode)
	}
	if len(site.Domain) != 2 {
		t.Fatalf("expected 2 domains, got %d", len(site.Domain))
	}
	for _, d := range site.Domain {
		if d.Type != routercommon.Domain_RootDomain {
			t.Errorf("expected Domain_Domain type, got %v", d.Type)
		}
		if strings.HasPrefix(d.Value, "domain:") {
			t.Errorf("domain: prefix should be stripped, got %s", d.Value)
		}
	}
	if site.Domain[0].Value != "example.com" {
		t.Errorf("expected example.com, got %s", site.Domain[0].Value)
	}
	if site.Domain[1].Value != "test.org" {
		t.Errorf("expected test.org, got %s", site.Domain[1].Value)
	}
}

func TestInjectGlobalMark(t *testing.T) {
	// Create a minimal geosite.dat with one dummy entry
	original := &routercommon.GeoSiteList{
		Entry: []*routercommon.GeoSite{
			{
				CountryCode: "TRACKER",
				Domain: []*routercommon.Domain{
					{Type: routercommon.Domain_RootDomain, Value: "tracker.example.com"},
				},
			},
		},
	}
	origData, err := proto.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}

	geositeFile, err := os.CreateTemp("", "geosite_test_*.dat")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(geositeFile.Name())
	if err := os.WriteFile(geositeFile.Name(), origData, 0644); err != nil {
		t.Fatal(err)
	}

	// Create global_mark text file
	markContent := "domain:google.com\n#@domain:baidu.com\n##@@domain:cdn.com\n"
	markFile, err := os.CreateTemp("", "global_mark_inject_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(markFile.Name())
	if _, err := markFile.WriteString(markContent); err != nil {
		t.Fatal(err)
	}
	markFile.Close()

	// First injection
	if err := injectGlobalMark(geositeFile.Name(), markFile.Name()); err != nil {
		t.Fatalf("injectGlobalMark failed: %v", err)
	}

	// Verify
	data, err := os.ReadFile(geositeFile.Name())
	if err != nil {
		t.Fatal(err)
	}
	var list routercommon.GeoSiteList
	if err := proto.Unmarshal(data, &list); err != nil {
		t.Fatal(err)
	}

	// Should have 4 entries: TRACKER + 3 new tags
	if len(list.Entry) != 4 {
		t.Fatalf("expected 4 entries, got %d", len(list.Entry))
	}

	tags := make(map[string]int)
	for _, entry := range list.Entry {
		tags[strings.ToUpper(entry.CountryCode)] = len(entry.Domain)
	}

	if tags["TRACKER"] != 1 {
		t.Errorf("TRACKER should have 1 domain, got %d", tags["TRACKER"])
	}
	if tags["PAOPAODNS_GLOBAL_MARK"] != 1 {
		t.Errorf("PAOPAODNS_GLOBAL_MARK should have 1 domain, got %d", tags["PAOPAODNS_GLOBAL_MARK"])
	}
	if tags["PAOPAODNS_CN_MARK"] != 1 {
		t.Errorf("PAOPAODNS_CN_MARK should have 1 domain, got %d", tags["PAOPAODNS_CN_MARK"])
	}
	if tags["PAOPAODNS_SKIP_MARK"] != 1 {
		t.Errorf("PAOPAODNS_SKIP_MARK should have 1 domain, got %d", tags["PAOPAODNS_SKIP_MARK"])
	}

	// Second injection - idempotency check
	if err := injectGlobalMark(geositeFile.Name(), markFile.Name()); err != nil {
		t.Fatalf("second injectGlobalMark failed: %v", err)
	}

	data2, err := os.ReadFile(geositeFile.Name())
	if err != nil {
		t.Fatal(err)
	}
	var list2 routercommon.GeoSiteList
	if err := proto.Unmarshal(data2, &list2); err != nil {
		t.Fatal(err)
	}

	if len(list2.Entry) != 4 {
		t.Errorf("idempotency: expected 4 entries after second injection, got %d", len(list2.Entry))
	}
}

// initWriter initialises the global mmdbwriter tree for tests that call mmdb* functions.
func initWriter(t *testing.T) {
	t.Helper()
	var err error
	writer, err = mmdbwriter.New(mmdbwriter.Options{
		IncludeReservedNetworks: true,
		DatabaseType:            "GeoLite2-Country",
		RecordSize:              24,
	})
	if err != nil {
		t.Fatalf("mmdbwriter.New: %v", err)
	}
}

func TestCidrToIPSet(t *testing.T) {
	cidrs := []*routercommon.CIDR{
		{Ip: net.ParseIP("1.0.0.0").To4(), Prefix: 8},  // IPv4
		{Ip: net.ParseIP("2400::").To16(), Prefix: 32}, // IPv6
	}
	s, err := cidrToIPSet(cidrs)
	if err != nil {
		t.Fatalf("cidrToIPSet failed: %v", err)
	}

	// IPv4 containment
	hit4, _ := netip.AddrFromSlice(net.ParseIP("1.2.3.4").To4())
	if !s.Contains(hit4) {
		t.Errorf("expected 1.2.3.4 to be contained in 1.0.0.0/8")
	}
	miss4, _ := netip.AddrFromSlice(net.ParseIP("2.0.0.1").To4())
	if s.Contains(miss4) {
		t.Errorf("expected 2.0.0.1 NOT to be contained")
	}

	// IPv6 containment
	hit6, _ := netip.AddrFromSlice(net.ParseIP("2400::1").To16())
	hit6 = hit6.Unmap()
	if !s.Contains(hit6) {
		t.Errorf("expected 2400::1 to be contained in 2400::/32")
	}
	miss6, _ := netip.AddrFromSlice(net.ParseIP("2401::1").To16())
	miss6 = miss6.Unmap()
	if s.Contains(miss6) {
		t.Errorf("expected 2401::1 NOT to be contained")
	}

	// IPv4-mapped IPv6 (e.g. from net.ParseIP without .To4()) is handled via Unmap
	mapped, _ := netip.AddrFromSlice(net.ParseIP("1.2.3.4")) // 16-byte form
	if !s.Contains(mapped.Unmap()) {
		t.Errorf("expected IPv4-mapped IPv6 1.2.3.4 to be contained after Unmap")
	}
}

func TestCheckGeosite(t *testing.T) {
	list := &routercommon.GeoSiteList{
		Entry: []*routercommon.GeoSite{
			{
				CountryCode: "TRACKER",
				Domain: []*routercommon.Domain{
					{Type: routercommon.Domain_RootDomain, Value: "tracker.example.com"},
				},
			},
			{
				CountryCode: "CATEGORY-PUBLIC-TRACKER",
				Domain: []*routercommon.Domain{
					{Type: routercommon.Domain_RootDomain, Value: "opentracker.example.com"},
				},
			},
		},
	}
	data, err := proto.Marshal(list)
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.CreateTemp("", "geosite_check_*.dat")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	if err := os.WriteFile(f.Name(), data, 0644); err != nil {
		t.Fatal(err)
	}

	// All mandatory tags present
	if err := checkGeosite(f.Name(), []string{"TRACKER", "CATEGORY-PUBLIC-TRACKER"}); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
	// Empty mandatory tag list
	if err := checkGeosite(f.Name(), nil); err != nil {
		t.Errorf("expected no error with empty tag list, got: %v", err)
	}
	// Missing mandatory tag
	if err := checkGeosite(f.Name(), []string{"TRACKER", "MISSING-TAG"}); err == nil {
		t.Error("expected error for missing mandatory tag, got nil")
	}
}

func TestCheckGeositeInvalidRegex(t *testing.T) {
	list := &routercommon.GeoSiteList{
		Entry: []*routercommon.GeoSite{
			{
				CountryCode: "TEST",
				Domain: []*routercommon.Domain{
					{Type: routercommon.Domain_Regex, Value: "[invalid(regex"},
				},
			},
		},
	}
	data, err := proto.Marshal(list)
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.CreateTemp("", "geosite_regex_*.dat")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	if err := os.WriteFile(f.Name(), data, 0644); err != nil {
		t.Fatal(err)
	}
	if err := checkGeosite(f.Name(), nil); err == nil {
		t.Error("expected error for invalid regex, got nil")
	}
}

func TestMmdbInsert(t *testing.T) {
	cnCIDRs = nil
	initWriter(t)

	if !mmdbInsert("1.0.1.0/24") {
		t.Error("mmdbInsert returned false for valid CIDR")
	}
	if len(cnCIDRs) != 1 {
		t.Fatalf("expected 1 cnCIDR, got %d", len(cnCIDRs))
	}
	if cnCIDRs[0].Prefix != 24 {
		t.Errorf("expected prefix 24, got %d", cnCIDRs[0].Prefix)
	}
	if len(cnCIDRs[0].Ip) != 4 {
		t.Errorf("expected IPv4 (4 bytes), got %d bytes", len(cnCIDRs[0].Ip))
	}
}

func TestMmdbLocal(t *testing.T) {
	privateCIDRs = nil
	initWriter(t)

	mmdbLocal("10.0.0.0/8")
	if len(privateCIDRs) != 1 {
		t.Fatalf("expected 1 privateCIDR, got %d", len(privateCIDRs))
	}
	if privateCIDRs[0].Prefix != 8 {
		t.Errorf("expected prefix 8, got %d", privateCIDRs[0].Prefix)
	}
	if len(privateCIDRs[0].Ip) != 4 {
		t.Errorf("expected IPv4 (4 bytes), got %d bytes", len(privateCIDRs[0].Ip))
	}
}

func TestMmdbCloudflare(t *testing.T) {
	cfCIDRs = nil
	initWriter(t)

	if !mmdbCloudflare("104.16.0.0/13") {
		t.Error("mmdbCloudflare returned false for valid CIDR")
	}
	if len(cfCIDRs) != 1 {
		t.Fatalf("expected 1 cfCIDR, got %d", len(cfCIDRs))
	}
	if cfCIDRs[0].Prefix != 13 {
		t.Errorf("expected prefix 13, got %d", cfCIDRs[0].Prefix)
	}
}

func TestImportLocal(t *testing.T) {
	privateCIDRs = nil
	initWriter(t)

	importLocal()
	// importLocal inserts 21 hardcoded private/special-use CIDRs
	const wantCIDRs = 21
	if len(privateCIDRs) != wantCIDRs {
		t.Errorf("expected %d privateCIDRs from importLocal, got %d", wantCIDRs, len(privateCIDRs))
	}
}

func TestImportTXT(t *testing.T) {
	cnCIDRs = nil
	initWriter(t)

	// Only valid CIDRs; empty lines are skipped by importTXT.
	content := "1.0.2.0/24\n2.0.2.0/24\n\n"
	f, err := os.CreateTemp("", "import_txt_*.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	f.Close()

	importTXT(f.Name())
	if len(cnCIDRs) != 2 {
		t.Errorf("expected 2 cnCIDRs from importTXT, got %d", len(cnCIDRs))
	}
}

func TestImportCSV(t *testing.T) {
	cnCIDRs = nil
	initWriter(t)

	// Fields: cidr, registered_country_geoname_id, represented_country_geoname_id,
	//         ...(4 unused)..., latitude, longitude, ...
	// 1814991 is the GeoName ID for mainland China.
	// Row 1: both IDs = 1814991 → direct insert
	// Row 2: represented=1814991, registered≠1814991, lat/lng inside China → insert
	// Row 3: both IDs = 0 → skip
	// Row 4: registered=1814991, represented=0 → skip (second condition not met)
	// Row 5: fewer than 9 columns → skip
	content := strings.Join([]string{
		"1.0.3.0/24,1814991,1814991,x,x,x,x,39.9,116.4,x",
		"2.0.3.0/24,0,1814991,x,x,x,x,39.9,116.4,x",
		"3.0.3.0/24,0,0,x,x,x,x,39.9,116.4,x",
		"4.0.3.0/24,1814991,0,x,x,x,x,39.9,116.4,x",
		"tooshort,1814991",
	}, "\n") + "\n"

	f, err := os.CreateTemp("", "import_csv_*.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	f.Close()

	importCSV(f.Name())
	// Row 1 (direct) + Row 2 (lat/lng Beijing, inside CN boundary)
	if len(cnCIDRs) != 2 {
		t.Errorf("expected 2 cnCIDRs from importCSV, got %d", len(cnCIDRs))
	}
}
