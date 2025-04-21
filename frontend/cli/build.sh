#!/bin/bash

set -euo pipefail

go build -o construct .
cp construct /usr/local/bin/construct
