#!/bin/bash
# Helper script to run visual-mtr with sudo
# This preserves the PATH environment variable so Go can be found

sudo env PATH="$PATH" go run main.go "$@"

