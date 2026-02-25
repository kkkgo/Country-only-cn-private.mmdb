FROM alpine:edge AS builder
RUN apk upgrade&&apk add --no-cache go git
COPY mmdb.go /tmp/mmdb/
COPY 100000_full.json /tmp/mmdb/
COPY chinaboundary/chinaboundary.go /tmp/mmdb/chinaboundary/
WORKDIR /tmp/mmdb
RUN go mod init mmdb && go mod tidy && go get -u && go build && mkdir -p /mmdb && mv mmdb /mmdb/
RUN git clone https://github.com/maxmind/mmdbverify.git --depth 1 /tmp/mmdbverify/
WORKDIR /tmp/mmdbverify
RUN rm *.mod *.sum && go mod init mmdbverify && go get -u && go build && mv mmdbverify /mmdb/

FROM alpine:edge
RUN apk upgrade&&apk add --no-cache curl unzip git xz bind-tools
COPY build.sh /usr/bin/
COPY test.yaml /usr/bin/
COPY --from=builder /mmdb/* /usr/bin/
COPY --from=sliamb/paopaodns /usr/sbin/mosdns /usr/bin/
RUN chmod +x /usr/bin/*
WORKDIR /data
ENTRYPOINT [ "build.sh" ]
