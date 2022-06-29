#!/bin/sh

if [ -z "${MOD_CONFIG}" ]; then
    MOD_CONFIG=lookup.json
fi

if [ ! -f "${MOD_CONFIG}" ]; then
    >&2 echo config file missing: ${MOD_CONFIG}
    exit 1
fi

jq -re ".[\"$1\"] | select(.!=null)" "$MOD_CONFIG" || true