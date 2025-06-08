#!/bin/bash

# Script to generate mocks for all connect service files
# This script enumerates files in ../v1/v1connect and generates mocks using mockgen

set -euo pipefail

V1CONNECT_DIR="../v1/v1connect"
TEST_DIR="./mocks"
PACKAGE_NAME="mocks"

if ! command -v mockgen &> /dev/null; then
    echo "Error: mockgen is not installed or not in PATH"
    exit 1
fi

echo "Generating mocks for connect service files..."

for file in "$V1CONNECT_DIR"/*.go; do
    if [[ -f "$file" ]]; then
        filename=$(basename "$file" .go)
        
        mock_file="$TEST_DIR/${filename}_mock.go"
        
        echo "Generating mock for $filename..."

        mockgen \
            -source="$file" \
            -destination="$mock_file" \
            -package="$PACKAGE_NAME"
        
        echo "✓ Generated: $mock_file"
    fi
done

echo "Mock generation completed!" 