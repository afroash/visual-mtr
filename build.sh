#!/bin/bash
# Build script for Visual MTR
# This builds the application so it can be run with sudo

echo "Building visual-mtr..."
go build -o visual-mtr .
if [ $? -eq 0 ]; then
    echo "Build successful!"
    echo ""
    echo "To run the application (requires sudo for ICMP):"
    echo "  sudo ./visual-mtr"
    echo ""
    echo "Or run directly with:"
    echo "  sudo -E go run main.go"
else
    echo "Build failed!"
    exit 1
fi

