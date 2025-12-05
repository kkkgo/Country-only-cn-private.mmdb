#!/bin/sh
apk update && apk upgrade
rm -rf /tmp/*
set -e
git clone https://github.com/17mon/china_ip_list.git --depth 1 /tmp/data/
IPREX4='([0-9]{1,2}|1[0-9][0-9]|2[0-4][0-9]|25[0-5])\.([0-9]{1,2}|1[0-9][0-9]|2[0-4][0-9]|25[0-5])\.([0-9]{1,2}|1[0-9][0-9]|2[0-4][0-9]|25[0-5])\.([0-9]{1,2}|1[0-9][0-9]|2[0-4][0-9]|25[0-5])'
IPREX6="(([0-9a-fA-F]{1,4}:){7,7}[0-9a-fA-F]{1,4}|([0-9a-fA-F]{1,4}:){1,7}:|([0-9a-fA-F]{1,4}:){1,6}:[0-9a-fA-F]{1,4}|([0-9a-fA-F]{1,4}:){1,5}(:[0-9a-fA-F]{1,4}){1,2}|([0-9a-fA-F]{1,4}:){1,4}(:[0-9a-fA-F]{1,4}){1,3}|([0-9a-fA-F]{1,4}:){1,3}(:[0-9a-fA-F]{1,4}){1,4}|([0-9a-fA-F]{1,4}:){1,2}(:[0-9a-fA-F]{1,4}){1,5}|[0-9a-fA-F]{1,4}:((:[0-9a-fA-F]{1,4}){1,6})|:((:[0-9a-fA-F]{1,4}){1,7}|:)|fe80:(:[0-9a-fA-F]{0,4}){0,4}%[0-9a-zA-Z]{1,}|::(ffff(:0{1,4}){0,1}:){0,1}((25[0-5]|(2[0-4]|1{0,1}[0-9]){0,1}[0-9])\.){3,3}(25[0-5]|(2[0-4]|1{0,1}[0-9]){0,1}[0-9])|([0-9a-fA-F]{1,4}:){1,4}:((25[0-5]|(2[0-4]|1{0,1}[0-9]){0,1}[0-9])\.){3,3}(25[0-5]|(2[0-4]|1{0,1}[0-9]){0,1}[0-9]))"

v4check() {
    if echo "$1" | grep -v "timed out" | grep -v "127.0.0.1" | grep -E "$IPREX4"; then
        echo "$1" pass.
    else
        echo "$1" failed.
        cp dns_check_failed /
        exit
    fi
}
v4checkb() {
    if echo "$1" | grep -v "timed out" | grep -v "127.0.0.1" | grep "NXDOMAIN"; then
        echo "$1" pass.
    else
        echo "$1" failed.
        cp dns_check_failed /
        exit
    fi
}
v6check() {
    if echo "$1" | grep -v "timed out" | grep -v "127.0.0.1" | grep -E "$IPREX6"; then
        echo "$1" pass.
    else
        echo "$1" failed.
        cp dns_check_failed /
        exit
    fi
}

curl -L -u "$YOUR_ACCOUNT_ID":"$YOUR_LICENSE_KEY" 'https://download.maxmind.com/geoip/databases/GeoLite2-City-CSV/download?suffix=zip' -o /tmp/GeoLite2-Country-CSV.zip
mmdb_hash=$(sha256sum /tmp/GeoLite2-Country-CSV.zip | grep -Eo "[a-zA-Z0-9]{64}" | head -1)
mmdb_down_hash=$(curl -sL -u "$YOUR_ACCOUNT_ID":"$YOUR_LICENSE_KEY" 'https://download.maxmind.com/geoip/databases/GeoLite2-City-CSV/download?suffix=zip.sha256' | grep -Eo "[a-zA-Z0-9]{64}" | head -1)
if [ "$mmdb_down_hash" != "$mmdb_hash" ]; then
    cp /mmdb_down_hash_error .
    exit
fi

cd /tmp || exit
unzip GeoLite2-Country-CSV.zip
rm GeoLite2-Country-CSV.zip
mv GeoLite2*/*.csv /tmp/data/

git clone --branch ip-lists --depth 1 https://github.com/gaoyifan/china-operator-ip.git /tmp/china-operator-ip
if [ -f /tmp/china-operator-ip/china6.txt ];then
    cp /tmp/china-operator-ip/china6.txt /tmp/data/china6.txt
else
    echo "china6 download failed."
    cp /china6_download_failed /
    exit
fi

mmdb

if mmdbverify -file /tmp/Country-only-cn-private.mmdb; then
    echo "mmdbverify pass."
else
    echo "mmdbverify failed"
    cp /mmdbverify /
    exit
fi
mmdb_size=$(wc -c <"/tmp/Country-only-cn-private.mmdb")

if [ "$mmdb_size" -gt 190000 ]; then
    echo "mmdb_size pass."
else
    echo "mmdb_size failed:""$mmdb_size"
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
tb=$(dig bad.dns @127.0.0.1 -p53 A)
v4checkb "$tb"
aaaat1=$(dig aaaatest1.dns @127.0.0.1 -p53 AAAA +short)
v6check "$aaaat1"
aaaat2=$(dig aaaatest2.dns @127.0.0.1 -p53 AAAA +short)
v6check "$aaaat2"
aaaat3=$(dig aaaatest3.dns @127.0.0.1 -p53 AAAA +short)
v6check "$aaaat3"
aaaat4=$(dig aaaatest4.dns @127.0.0.1 -p53 AAAA +short)
v6check "$aaaat4"
aaaat5=$(dig aaaatest5.dns @127.0.0.1 -p53 AAAA +short)
v6check "$aaaat5"
aaaat6=$(dig aaaatest6.dns @127.0.0.1 -p53 AAAA +short)
v6check "$aaaat6"
aaaat7=$(dig aaaatest7.dns @127.0.0.1 -p53 AAAA +short)
v6check "$aaaat7"
aaaat8=$(dig aaaatest8.dns @127.0.0.1 -p53 AAAA +short)
v6check "$aaaat8"
aaaat9=$(dig aaaatest9.dns @127.0.0.1 -p53 AAAA +short)
v6check "$aaaat9"
aaaat0=$(dig aaaatest0.dns @127.0.0.1 -p53 AAAA +short)
v6check "$aaaat0"
aaaatb=$(dig aaaabad.dns @127.0.0.1 -p53 AAAA)
v4checkb "$aaaatb"
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
