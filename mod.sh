#!/bin/sh

jq -r ".[\"${1}\"]" mod.json