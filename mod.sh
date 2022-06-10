#!/bin/sh

jq -re ".[\"${1}\"] | select(.!=null)" mod.json || true