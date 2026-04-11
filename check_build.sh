#!/bin/bash

for run in {1..20}; do
	make clean | grep "notgoingtofind it"
	make build | grep "notgoingtofind it"
	output=$(zsh -c "time bin/gorgon -config=gorgon-example.yml examples" 2>&1)
	echo "$output" | grep -A 2 "Mutation Score"
	echo "$output" | grep "cpu"
	echo "-----"
done
