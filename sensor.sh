#!/usr/bin/env bash
set -euo pipefail

# 1) Build the sensor simulator binary
echo "Building sensor simulator..."
go build -o sensor-sim /home/leonardo/GolandProjects/sdcc_project/internal/sensor-simulator/cmd/main.go
echo "Built ./sensor-sim"

# 2) Prepare logs directory
LOG_DIR="./logs"
mkdir -p "$LOG_DIR"
echo "Logs will go into $LOG_DIR/"

# 3) Launch 6 sensor instances (3 per field)
INTERVAL="10s"  # adjust as needed
for i in {1..6}; do
  if [ "$i" -le 3 ]; then
    FIELD="field1"
  else
    FIELD="field2"
  fi

  SENSOR_ID="sensor${i}"
  CLIENT_ID="pub${i}"
  LOG_FILE="$LOG_DIR/${SENSOR_ID}.log"

  echo "Starting $SENSOR_ID in $FIELD (client=$CLIENT_ID)..."
  ./sensor-sim \
    -field-id="$FIELD" \
    -sensor-id="$SENSOR_ID" \
    -client-id="$CLIENT_ID" \
    -interval="$INTERVAL" \
    > "$LOG_FILE" 2>&1 &

  echo "  → PID $!  (logs: $LOG_FILE)"
done

echo "All 6 sensors started."