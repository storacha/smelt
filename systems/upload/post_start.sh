#!/bin/sh

PROOF=$(cat /proofs/piri-proof.txt)
DID=$(sprue identity parse /keys/piri.pub)

sleep 1 # Wait for services to be fully up
sprue client admin provider add http://piri:3000 "$PROOF"
sprue client admin provider weight set $DID 100 100
