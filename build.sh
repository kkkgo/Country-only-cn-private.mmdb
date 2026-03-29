#!/bin/sh
set -e

RETRY_COUNT=3
RETRY_DELAY=5

retry() {
	local n=0
	until [ $n -ge $RETRY_COUNT ]; do
		"$@" && return 0
		n=$((n + 1))
		echo "Retry $n/$RETRY_COUNT: $*"
		sleep $RETRY_DELAY
	done
	echo "Failed after $RETRY_COUNT retries: $*"
	return 1
}

dl() {
	retry curl -fSL --retry $RETRY_COUNT --retry-delay $RETRY_DELAY "$@"
}

pack_output() {
	local src="$1" name="$2"
	md5sum "$src" | cut -d" " -f1 >"/data/${name}.md5sum"
	local sha256
	sha256=$(sha256sum "$src" | cut -d" " -f1)
	echo "$sha256" >"/data/${name}.sha256sum"
	echo "${name} md5sum: $(cat "/data/${name}.md5sum")"
	echo "${name} sha256sum: ${sha256}"
	cp "$src" "/data/${name}"
	local cp_sha256
	cp_sha256=$(sha256sum "/data/${name}" | cut -d" " -f1)
	if [ "$sha256" != "$cp_sha256" ]; then
		echo "Copy verification failed for ${name}"
		exit 1
	fi
	xz -9 -k -e -f "$src"
	sha256sum "${src}.xz" | cut -d" " -f1 >"/data/${name}.xz.sha256sum"
	mv "${src}.xz" "/data/${name}.xz"
}

apk update && apk upgrade
rm -rf /tmp/*

# Download china_ip_list
retry git clone https://github.com/17mon/china_ip_list.git --depth 1 /tmp/data/

# Download MaxMind GeoLite2
dl -u "$YOUR_ACCOUNT_ID":"$YOUR_LICENSE_KEY" \
	'https://download.maxmind.com/geoip/databases/GeoLite2-City-CSV/download?suffix=zip' \
	-o /tmp/GeoLite2-Country-CSV.zip
mmdb_hash=$(sha256sum /tmp/GeoLite2-Country-CSV.zip | grep -Eo "[a-zA-Z0-9]{64}" | head -1)
mmdb_down_hash=$(dl -u "$YOUR_ACCOUNT_ID":"$YOUR_LICENSE_KEY" \
	'https://download.maxmind.com/geoip/databases/GeoLite2-City-CSV/download?suffix=zip.sha256' | grep -Eo "[a-zA-Z0-9]{64}" | head -1)
if [ "$mmdb_down_hash" != "$mmdb_hash" ]; then
	echo "MaxMind GeoLite2 hash mismatch: expected=$mmdb_down_hash got=$mmdb_hash"
	exit 1
fi

unzip -o /tmp/GeoLite2-Country-CSV.zip -d /tmp
rm /tmp/GeoLite2-Country-CSV.zip
mv /tmp/GeoLite2*/*.csv /tmp/data/

# Download china6
retry git clone --branch ip-lists --depth 1 https://github.com/gaoyifan/china-operator-ip.git /tmp/china-operator-ip
if [ ! -f /tmp/china-operator-ip/china6.txt ]; then
	echo "china6.txt not found after clone"
	exit 1
fi
cp /tmp/china-operator-ip/china6.txt /tmp/data/china6.txt

# Download geosite
retry git clone --branch release --depth 1 https://github.com/MetaCubeX/meta-rules-dat.git /tmp/geosite
if [ ! -f /tmp/geosite/geosite.dat ]; then
	echo "geosite.dat not found after clone"
	exit 1
fi
(cd /tmp/geosite && sha256sum -c geosite.dat.sha256sum)
mmdb -check-geosite /tmp/geosite/geosite.dat

# Download and extract global_mark.dat
retry git clone --depth 1 https://github.com/kkkgo/PaoPao-Pref.git /tmp/paopao-pref
if [ ! -f /tmp/paopao-pref/global_mark.dat ]; then
	echo "global_mark.dat not found after clone"
	exit 1
fi
expected_sha256=$(cat /tmp/paopao-pref/global_mark.dat.sha256sum | tr -d '[:space:]')
actual_sha256=$(sha256sum /tmp/paopao-pref/global_mark.dat | cut -d" " -f1)
if [ "$actual_sha256" != "$expected_sha256" ]; then
	echo "global_mark.dat SHA256 mismatch: expected=$expected_sha256 got=$actual_sha256"
	exit 1
fi
echo "global_mark.dat SHA256 checksum passed"

file_size=$(wc -c < /tmp/paopao-pref/global_mark.dat)
xz_size=$((file_size - 1024))

embedded_sha512=$(tail -c 1024 /tmp/paopao-pref/global_mark.dat | head -c 128 | tr -d '\0')
actual_sha512=$(head -c "$xz_size" /tmp/paopao-pref/global_mark.dat | sha512sum | cut -d" " -f1)
if [ "$actual_sha512" != "$embedded_sha512" ]; then
	echo "global_mark.dat SHA512 internal checksum mismatch"
	exit 1
fi
echo "global_mark.dat SHA512 internal checksum passed"

head -c "$xz_size" /tmp/paopao-pref/global_mark.dat > /tmp/global_mark.xz
xz -d /tmp/global_mark.xz

# Inject PaoPaoDNS tags into geosite.dat
mmdb -inject-geosite /tmp/geosite/geosite.dat -global-mark /tmp/global_mark
mmdb -check-geosite /tmp/geosite/geosite.dat -check-tags "TRACKER,CATEGORY-PUBLIC-TRACKER,PAOPAODNS_GLOBAL_MARK,PAOPAODNS_CN_MARK,PAOPAODNS_SKIP_MARK"

# Re-compute checksums after injection
sha256sum /tmp/geosite/geosite.dat | cut -d" " -f1 >/data/geosite.dat.sha256sum
cp /tmp/geosite/geosite.dat /data/geosite.dat
xz -9 -k -e -f /tmp/geosite/geosite.dat
sha256sum /tmp/geosite/geosite.dat.xz | cut -d" " -f1 >/data/geosite.dat.xz.sha256sum
mv /tmp/geosite/geosite.dat.xz /data/geosite.dat.xz

# Build mmdb and dat
mmdb

# Verify mmdb
mmdbverify -file /tmp/Country-only-cn-private.mmdb
mmdb_size=$(wc -c <"/tmp/Country-only-cn-private.mmdb")
if [ "$mmdb_size" -le 190000 ]; then
	echo "mmdb_size too small: $mmdb_size"
	exit 1
fi
echo "mmdb_size pass: $mmdb_size"

dat_size=$(wc -c <"/tmp/CN-local.dat")
if [ "$dat_size" -le 190000 ]; then
	echo "dat_size too small: $dat_size"
	exit 1
fi
echo "dat_size pass: $dat_size"

# Verify test IPs
mmdb -check-mmdb /tmp/Country-only-cn-private.mmdb
mmdb -check-dat /tmp/CN-local.dat
echo "MMDB AND DAT CHECK PASS !"

# Pack outputs
pack_output /tmp/Country-only-cn-private.mmdb Country-only-cn-private.mmdb
mmdbverify -file /data/Country-only-cn-private.mmdb
echo "Copy mmdbverify pass."

pack_output /tmp/CN-local.dat CN-local.dat
