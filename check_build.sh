#!/bin/bash

for run in {1..20}; do
	make clean | grep "notgoingtofind it"
	make build | grep "notgoingtofind it"
	zsh -c "time bin/gorgon -config=gorgon-example.yml examples" | grep "cpu"
done
