#!/bin/bash

# Socket File Transfer Lab Experiments
echo "🚀 Socket File Transfer Lab Experiments"
echo "========================================"

# Colors for pretty output
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m'

print_header() {
    echo -e "${BLUE}$1${NC}"
    echo "----------------------------------------"
}

run_test() {
    local protocol=$1
    local condition=$2
    local file=$3
    
    echo -e "${YELLOW}Testing: $protocol - $condition - $file${NC}"
    
    # Start packet capture
    tshark -i lo -w "/tmp/${protocol}_${condition}_${file}.pcap" &
    TSHARK_PID=$!
    sleep 1
    
    # Start server in background
    if [ "$protocol" = "TCP" ]; then
        cd /app/tcp && ./tcp -mode=server &
    else
        cd /app/udp && ./udp -mode=server &
    fi
    SERVER_PID=$!
    sleep 3
    
    # Run client with timeout
    start_time=$(date +%s.%N)
    if [ "$protocol" = "TCP" ]; then
        cd /app/tcp && timeout 60 ./tcp -mode=client -file="/app/test-files/$file"
        CLIENT_EXIT_CODE=$?
    else
        cd /app/udp && timeout 60 ./udp -mode=client -file="/app/test-files/$file"
        CLIENT_EXIT_CODE=$?
    fi
    end_time=$(date +%s.%N)
    
    # Calculate transfer time
    transfer_time=$(echo "$end_time - $start_time" | bc -l)
    
    # Stop capture and server
    sleep 2
    kill $TSHARK_PID 2>/dev/null
    kill $SERVER_PID 2>/dev/null
    
    # Wait for processes to terminate
    wait $TSHARK_PID 2>/dev/null
    wait $SERVER_PID 2>/dev/null
    sleep 1
    
    # Analyze capture
    packet_count=$(tshark -r "/tmp/${protocol}_${condition}_${file}.pcap" 2>/dev/null | wc -l)
    
    if [ $CLIENT_EXIT_CODE -eq 0 ]; then
        echo -e "${GREEN}✓ Transfer Time: ${transfer_time}s${NC}"
        echo -e "${GREEN}✓ Packets: $packet_count${NC}"
        echo -e "${GREEN}✓ Status: SUCCESS${NC}"
    elif [ $CLIENT_EXIT_CODE -eq 124 ]; then
        echo -e "${RED}✗ Transfer Time: ${transfer_time}s (TIMEOUT)${NC}"
        echo -e "${YELLOW}✓ Packets: $packet_count${NC}"
        echo -e "${RED}✗ Status: TIMEOUT${NC}"
    else
        echo -e "${RED}✗ Transfer Time: ${transfer_time}s (ERROR)${NC}"
        echo -e "${YELLOW}✓ Packets: $packet_count${NC}"
        echo -e "${RED}✗ Status: ERROR (exit code: $CLIENT_EXIT_CODE)${NC}"
    fi
    echo ""
}

# Main experiment runner
main() {
    print_header "📊 BASELINE EXPERIMENTS"
    
    for file in "small.txt" "medium.txt" "large.txt"; do
        run_test "TCP" "baseline" "$file"
        run_test "UDP" "baseline" "$file"
    done
    
    print_header "📉 PACKET LOSS EXPERIMENTS (5%)"
    tc qdisc add dev lo root netem loss 5% 2>/dev/null
    
    run_test "TCP" "loss5" "large.txt"
    run_test "UDP" "loss5" "large.txt"
    
    tc qdisc del dev lo root 2>/dev/null
    
    print_header "⏱️  LATENCY EXPERIMENTS (100ms)"
    tc qdisc add dev lo root netem delay 100ms 10ms 2>/dev/null
    
    run_test "TCP" "latency100" "large.txt"
    run_test "UDP" "latency100" "large.txt"
    
    tc qdisc del dev lo root 2>/dev/null
    
    print_header "📋 EXPERIMENT SUMMARY"
    echo "Capture files saved in /tmp/"
    ls -la /tmp/*.pcap 2>/dev/null || echo "No capture files found"
    
    echo -e "${GREEN}✅ All experiments completed!${NC}"
    echo "Use 'tshark -r filename.pcap' to analyze captures"
}

main "$@"
