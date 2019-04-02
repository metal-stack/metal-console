#!/usr/bin/env bash

ssh -p 2222 -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null 192.168.2.25@localhost
