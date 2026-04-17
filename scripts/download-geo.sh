#!/bin/bash
set -e

echo "Downloading GeoIP and GeoSite databases..."
mkdir -p data

curl -L -o data/geoip.dat https://github.com/v2fly/geoip/releases/latest/download/geoip.dat
curl -L -o data/geosite.dat https://github.com/v2fly/domain-list-community/releases/latest/download/dlc.dat

echo "Done."
