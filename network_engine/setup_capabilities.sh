#!/bin/bash

# Setup script for CalcuLatency Go Network Engine
echo "Setting up CalcuLatency Go Network Engine..."

# Check if running on Linux
if [[ "$OSTYPE" != "linux-gnu"* ]]; then
    echo "Warning: This setup script is designed for Linux. Some features may not work on other systems."
fi

# Check if running as root
if [[ $EUID -eq 0 ]]; then
    echo "Running as root - all network features will be available"
else
    echo "Not running as root - attempting to set up capabilities..."
    
    # Check if setcap is available
    if command -v setcap >/dev/null 2>&1; then
        echo "Setting network capabilities for the binary..."
        sudo setcap cap_net_raw+ep ./network-engine
        echo "Capabilities set. You can now run without sudo."
    else
        echo "setcap not available. You'll need to run with sudo for full functionality."
    fi
    
    # Check ping group range for unprivileged ICMP
    if [[ -f /proc/sys/net/ipv4/ping_group_range ]]; then
        current_range=$(cat /proc/sys/net/ipv4/ping_group_range)
        echo "Current ping group range: $current_range"
        
        if [[ "$current_range" != "0	2147483647" ]]; then
            echo "To enable unprivileged ICMP, run:"
            echo "sudo sysctl -w net.ipv4.ping_group_range='0 2147483647'"
        fi
    fi
fi

echo "Setup complete!"
echo ""
echo "To build and run:"
echo "  make build"
echo "  sudo ./network-engine  # or just ./network-engine if capabilities are set"
echo ""
echo "API will be available at http://localhost:8080"
