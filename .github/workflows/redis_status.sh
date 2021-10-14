#!/bin/bash

while true; do
    aws elasticache describe-replication-groups --replication-group-id $1 || echo "status query failed"
    sleep 10
done