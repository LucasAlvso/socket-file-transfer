FROM golang:1.21-alpine

RUN apk add --no-cache \
    tcpdump \
    iproute2 \
    wireshark-common \
    tshark \
    iperf3 \
    netcat-openbsd \
    curl \
    bash

WORKDIR /app

COPY . .

RUN mkdir -p uploads

RUN cd tcp && go build -o tcp tcp.go
RUN cd udp && go build -o udp udp.go

RUN echo '#!/bin/bash\ncd /app/tcp && ./tcp -mode=server' > /usr/local/bin/tcp-server && chmod +x /usr/local/bin/tcp-server
RUN echo '#!/bin/bash\ncd /app/tcp && ./tcp -mode=client -file="$1"' > /usr/local/bin/tcp-client && chmod +x /usr/local/bin/tcp-client
RUN echo '#!/bin/bash\ncd /app/udp && ./udp -mode=server' > /usr/local/bin/udp-server && chmod +x /usr/local/bin/udp-server
RUN echo '#!/bin/bash\ncd /app/udp && ./udp -mode=client -file="$1"' > /usr/local/bin/udp-client && chmod +x /usr/local/bin/udp-client

EXPOSE 8080 8081

CMD ["/bin/bash"]
