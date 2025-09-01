## Running

### TCP

**Terminal 1 (Server):**
```bash
cd tcp
go run tcp.go -mode=server
```

**Terminal 2 (Client):**
```bash
cd tcp
go run tcp.go -mode=client -file=../test-files/small.txt
```

### UDP

**Terminal 1 (Server):**
```bash
cd udp
go run udp.go -mode=server
```

**Terminal 2 (Client):**
```bash
cd udp
go run udp.go -mode=client -file=../test-files/small.txt
```