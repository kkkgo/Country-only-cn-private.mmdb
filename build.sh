#!/bin/sh
rm -rf /tmp/*
set -e
git clone https://github.com/17mon/china_ip_list.git --depth 1 /tmp/data/

curl -sLo /tmp/GeoLite2-Country-CSV.zip "https://download.maxmind.com/app/geoip_download?edition_id=GeoLite2-Country-CSV&license_key=${MMDB_KEY}&suffix=zip"
mmdb_hash=$(sha256sum /tmp/GeoLite2-Country-CSV.zip | grep -Eo "[a-zA-Z0-9]{64}" | head -1)
mmdb_down_hash=$(curl -s "https://download.maxmind.com/app/geoip_download?edition_id=GeoLite2-Country-CSV&license_key=${MMDB_KEY}&suffix=zip.sha256" | grep -Eo "[a-zA-Z0-9]{64}" | head -1)
if [ "$mmdb_down_hash" != "$mmdb_hash" ]; then
    cp /mmdb_down_hash_error .
    exit
fi

cd /tmp || exit
unzip GeoLite2-Country-CSV.zip
rm GeoLite2-Country-CSV.zip
mv GeoLite2*/*.csv /tmp/data/
mmdb

if mmdbverify -file /tmp/Country-only-cn-private.mmdb; then
    echo "mmdbverify pass."
else
    echo "mmdbverify failed"
    cp /mmdbverify /
    exit
fi
mmdb_size=$(wc -c <"/tmp/Country-only-cn-private.mmdb")

if [ "$mmdb_size" -gt 200000 ]; then
    echo "mmdb_size pass."
else
    echo "mmdb_size failed"
    cp /mmdb_size /
    exit
fi

mosdns start -d /usr/bin -c test.yaml &
sleep 3
nslookup test1.dns 127.0.0.1
nslookup test2.dns 127.0.0.1
nslookup test3.dns 127.0.0.1
nslookup test4.dns 127.0.0.1
nslookup test5.dns 127.0.0.1
nslookup test6.dns 127.0.0.1
nslookup test7.dns 127.0.0.1
nslookup test8.dns 127.0.0.1
nslookup test9.dns 127.0.0.1
nslookup test0.dns 127.0.0.1
echo "DNS TEST PASS !"
if [ -f /tmp/Country-only-cn-private.mmdb ]; then
    md5sum /tmp/Country-only-cn-private.mmdb | cut -d" " -f1 >/data/Country-only-cn-private.mmdb.md5sum
    mmdb_sha256=$(sha256sum /tmp/Country-only-cn-private.mmdb | cut -d" " -f1)
    echo "$mmdb_sha256" >/data/Country-only-cn-private.mmdb.sha256sum
    echo Gen md5sum:
    cat /data/Country-only-cn-private.mmdb.md5sum
    echo Gen sha256sum:
    cat /data/Country-only-cn-private.mmdb.sha256sum
    cp /tmp/Country-only-cn-private.mmdb /data/Country-only-cn-private.mmdb
    mmdb_cp_sha256=$(sha256sum /data/Country-only-cn-private.mmdb | cut -d" " -f1)
    if mmdbverify -file /data/Country-only-cn-private.mmdb && [ "$mmdb_sha256" = "$mmdb_cp_sha256" ]; then
        echo "Copy mmdbverify pass."
    else
        echo "Copy mmdbverify failed"
        cp /cp_mmdbverify /
        exit
    fi
    mkdir -p /tmp/xz
    cp /tmp/Country-only-cn-private.mmdb /tmp/xz/
    cd /tmp/xz || exit
    xz -9 -k -e /tmp/xz/Country-only-cn-private.mmdb
    sha256sum /tmp/xz/Country-only-cn-private.mmdb.xz | cut -d" " -f1 >/tmp/xz/Country-only-cn-private.mmdb.xz.sha256sum
    mv /tmp/xz/Country-only-cn-private.mmdb.xz /data/Country-only-cn-private.mmdb.xz
    mv /tmp/xz/Country-only-cn-private.mmdb.xz.sha256sum /data/Country-only-cn-private.mmdb.xz.sha256sum
fi
