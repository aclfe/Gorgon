#!/bin/bash
make clean
make build
output=$(zsh -c "time bin/gorgon -config=gorgon-example.yml" 2>&1)
