#!/bin/bash
sshpass -p 1234 ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o ConnectTimeout=6 user@192.168.2.15 "echo 1234 | devel-su sh -c \"$1\"" 2>&1 | grep -v -e '^Password:' -e '^Warning:'
