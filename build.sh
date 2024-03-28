#!/bin/sh
apk update && apk upgrade
rm -rf /tmp/*
set -e
git clone https://github.com/17mon/china_ip_list.git --depth 1 /tmp/data/
IPREX4='([0-9]{1,2}|1[0-9][0-9]|2[0-4][0-9]|25[0-5])\.([0-9]{1,2}|1[0-9][0-9]|2[0-4][0-9]|25[0-5])\.([0-9]{1,2}|1[0-9][0-9]|2[0-4][0-9]|25[0-5])\.([0-9]{1,2}|1[0-9][0-9]|2[0-4][0-9]|25[0-5])'
v4check() {
    if echo "$1" | grep -v "timed out" | grep -v "127.0.0.1" | grep -E "$IPREX4"; then
        echo "$1" pass.
    else
        cp dns_check_failed /
        echo "$1" failed.
        exit
    fi
}
curl -L -u "$YOUR_ACCOUNT_ID":"$YOUR_LICENSE_KEY" 'https://download.maxmind.com/geoip/databases/GeoLite2-City-CSV/download?suffix=zip' -o /tmp/GeoLite2-Country-CSV.zip
mmdb_hash=$(sha256sum /tmp/GeoLite2-Country-CSV.zip | grep -Eo "[a-zA-Z0-9]{64}" | head -1)
mmdb_down_hash=$(curl -L -u "$YOUR_ACCOUNT_ID":"$YOUR_LICENSE_KEY" 'https://download.maxmind.com/geoip/databases/GeoLite2-City-CSV/download?suffix=zip.sha256' | grep -Eo "[a-zA-Z0-9]{64}" | head -1)
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
t1=$(dig test1.dns @127.0.0.1 -p53 A +short)
v4check "$t1"
t2=$(dig test2.dns @127.0.0.1 -p53 A +short)
v4check "$t2"
t3=$(dig test3.dns @127.0.0.1 -p53 A +short)
v4check "$t3"
t4=$(dig test4.dns @127.0.0.1 -p53 A +short)
v4check "$t4"
t5=$(dig test5.dns @127.0.0.1 -p53 A +short)
v4check "$t5"
t6=$(dig test6.dns @127.0.0.1 -p53 A +short)
v4check "$t6"
t7=$(dig test7.dns @127.0.0.1 -p53 A +short)
v4check "$t7"
t8=$(dig test8.dns @127.0.0.1 -p53 A +short)
v4check "$t8"
t9=$(dig test9.dns @127.0.0.1 -p53 A +short)
v4check "$t9"
t0=$(dig test0.dns @127.0.0.1 -p53 A +short)
v4check "$t0"
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
