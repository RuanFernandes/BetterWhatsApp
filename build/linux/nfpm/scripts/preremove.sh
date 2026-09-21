#!/bin/bash

if command -v apparmor_parser >/dev/null 2>&1 && [ -f /etc/apparmor.d/betterwhatsapp ]; then
  apparmor_parser -R /etc/apparmor.d/betterwhatsapp >/dev/null 2>&1 || true
fi
