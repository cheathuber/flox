#!/bin/bash

# Script to run a local API test against the flox-backend
# Assumes flox-backend-test binary exists in the current directory
# and that necessary environment variables are set or defaulted.

set -e # Exit immediately if a command exits with a non-zero status.

echo "=== Starting Local API Test ==="

# --- Configuration ---
TEST_BINARY="./flox-backend-test"
TEST_PORT="8099"
TEST_SITES_DIR="./test-sites-local"
TEST_SITE_IP="127.0.0.1"
HEALTH_ENDPOINT="http://localhost:${TEST_PORT}/api/health"
# --- End Configuration ---

# Check if the test binary exists
if [[ ! -f "$TEST_BINARY" ]]; then
    echo "Error: Test binary '$TEST_BINARY' not found. Please run 'make build-test-binary' first."
    exit 1
fi
# Ensure jq is installed for JSON parsing
if ! command -v jq &> /dev/null; then
    echo "Error: 'jq' is required for this test but not found."
    echo "Please install jq (e.g., 'sudo apt install jq' on Ubuntu/Debian)."
    exit 1
fi

# Cleanup function to stop the server and remove temporary files
cleanup() {
    echo "=== Performing Cleanup ==="
    if [[ -f "flox-backend-test.pid" ]]; then
        PID=$(cat flox-backend-test.pid)
        echo "Stopping test server (PID: $PID)..."
        # Try graceful shutdown first
        kill $PID 2>/dev/null || true
        sleep 2
        # Force kill if still running
        if kill -0 $PID 2>/dev/null; then
            echo "Force killing server (PID: $PID)..."
            kill -9 $PID 2>/dev/null || true
        fi
        rm -f flox-backend-test.pid
    else
        echo "No PID file found, server might not have started or already stopped."
    fi

    # Remove test sites directory
    if [[ -d "$TEST_SITES_DIR" ]]; then
        echo "Removing test sites directory: $TEST_SITES_DIR"
        rm -rf "$TEST_SITES_DIR"
    fi

    # Note: We deliberately leave the log file (flox-backend-test.log)
    # for inspection if the test fails. Makefile 'clean-test-binary' removes it.
    echo "=== Cleanup Completed ==="
}

# Set trap to ensure cleanup happens on exit (success or failure)
trap cleanup EXIT

echo "Preparing test environment..."
# Create the sites directory for the test
mkdir -p "$TEST_SITES_DIR"
echo "Using test sites directory: $TEST_SITES_DIR"

echo "Starting test server..."
# Start the server in the background, redirecting output to a log file
SITE_IP="$TEST_SITE_IP" \
SITES_BASE_DIR="$TEST_SITES_DIR" \
FLOX_SERVER_PORT="$TEST_PORT" \
"$TEST_BINARY" > flox-backend-test.log 2>&1 &

# Capture the background job's PID
SERVER_PID=$!
# Save PID to file for cleanup
echo $SERVER_PID > flox-backend-test.pid
echo "Test server started with PID $SERVER_PID (logged to flox-backend-test.log)"

# Give the server a moment to start
echo "Waiting for server to initialize..."
sleep 5

# Check if the process is still running
if ! kill -0 $SERVER_PID 2>/dev/null; then
    echo "Error: Test server process seems to have exited."
    echo "--- Server Log (last 50 lines) ---"
    tail -n 50 flox-backend-test.log || echo "Log file not found or unreadable."
    echo "--- End Server Log ---"
    exit 1
fi
echo "Server appears to be running."

echo "Testing endpoint: $HEALTH_ENDPOINT"
# Make the API call and capture the output and HTTP status code
# -s: silent (no progress meter)
# -w "\n%{http_code}": Append a newline and the HTTP status code to the output
RESPONSE=$(curl -s -w "\n%{http_code}" "$HEALTH_ENDPOINT" 2>&1 || true)
EXIT_CODE=$?
#echo "********* Curl error output: $RESPONSE"
if [ $EXIT_CODE -ne 0 ]; then
    echo "Error: curl command failed with exit code $EXIT_CODE"
    echo "Curl error output: $RESPONSE"
    exit 1
fi

# The last line of RESPONSE is the HTTP status code
HTTP_CODE=$(echo "$RESPONSE" | tail -n 1)
# Everything except the last line is the JSON body
JSON_BODY=$(echo "$RESPONSE" | head -n -1)

echo "Received HTTP Status Code: $HTTP_CODE"
echo "Received JSON Body: $JSON_BODY"

# --- Assertions ---
TEST_FAILED=0

# Check HTTP status code
if [[ "$HTTP_CODE" -ne 200 ]]; then
    echo "FAIL: Expected HTTP 200, got $HTTP_CODE"
    TEST_FAILED=1
else
    echo "PASS: HTTP Status Code is 200"
fi

# Parse JSON and check status field
if [[ -n "$JSON_BODY" ]]; then
    STATUS=$(echo "$JSON_BODY" | jq -r '.status' 2>/dev/null)
    if [[ "$?" -ne 0 ]]; then
        echo "FAIL: Could not parse JSON response to extract 'status' field."
        echo "Raw response body: $JSON_BODY"
        TEST_FAILED=1
    elif [[ "$STATUS" != "OK" ]]; then
        echo "FAIL: Expected JSON status 'OK', got '$STATUS'"
        TEST_FAILED=1
    else
        echo "PASS: JSON status field is 'OK'"
    fi

    # Optionally, check the version field format (basic check)
    VERSION=$(echo "$JSON_BODY" | jq -r '.version' 2>/dev/null)
    if [[ "$?" -eq 0 ]] && [[ -n "$VERSION" ]] && [[ "$VERSION" != "null" ]]; then
        echo "INFO: Server version reported: $VERSION"
    else
        echo "WARN: Could not extract or invalid version from JSON response."
    fi
else
    echo "FAIL: Received empty JSON body"
    TEST_FAILED=1
fi
# --- End Assertions ---

if [[ $TEST_FAILED -eq 1 ]]; then
    echo "=== TEST FAILED ==="
    echo "--- Server Log (last 50 lines) ---"
    tail -n 50 flox-backend-test.log || echo "Log file not found or unreadable."
    echo "--- End Server Log ---"
    exit 1 # Exit with error code to indicate failure
else
    echo "=== ALL TESTS PASSED ==="
    # Server will be stopped by the cleanup trap
fi
