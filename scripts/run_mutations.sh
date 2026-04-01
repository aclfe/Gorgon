#!/bin/bash

# Run mutations on all example folders with timing

echo "Running mutations on all examples..."
echo ""

for dir in examples/mutations/*/; do
    if [ -d "$dir" ]; then
        dirname=$(basename "$dir")
        echo "====== Running mutations for $dirname ======"
        time bin/gorgon "$dir"
        echo ""
    fi
done

echo "====== All mutations complete ======"
