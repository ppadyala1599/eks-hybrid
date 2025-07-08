#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

cd $(dirname $0)/../..

DEBUG=${DEBUG:-false}
TEST_FILTER=${TEST_FILTER:-'*'}
GOPROXY="${GOPROXY:-https://proxy.golang.org}"

if [ "$DEBUG" = "true" ]; then
  echo "🐛 Debug mode enabled"
fi

if [ -z "${TEST_IMAGE:-}" ]; then
  printf "🛠️ Building test infra image..."
  TEST_IMAGE=$(docker build --build-arg GOPROXY=$GOPROXY -q -f test/integration/infra/Dockerfile .)
  echo "done! Test image: $TEST_IMAGE"
else
  echo "🔍 Using test infra image: $TEST_IMAGE"
fi

FAILED="false"

CASES=$(find test/integration/cases -maxdepth 1 -mindepth 1 -name "$TEST_FILTER")
if [ -z "$CASES" ]; then
  echo "No test case found for filter $TEST_FILTER"
  echo "❌ No tests ran!"
  exit 1
fi

for CASE_DIR in $CASES; do
  CASE_NAME=$(basename $CASE_DIR)
  printf "🧪 Testing $CASE_NAME..."
  CONTAINER_ID=$(docker run \
    -d \
    --rm \
    --privileged \
    -v $PWD/$CASE_DIR:/test-case \
    $TEST_IMAGE)
  LOG_FILE=$(mktemp)
  if docker exec $CONTAINER_ID bash -c "cd /test-case && ./run.sh" > $LOG_FILE 2>&1; then
    echo "passed! ✅"
    if [ "$DEBUG" = "true" ]; then
      cat $LOG_FILE
    fi
  else
    echo "failed! ❌"
    cat $LOG_FILE
    FAILED="true"
  fi
  docker kill $CONTAINER_ID > /dev/null 2>&1
done

if [ "${FAILED}" = "true" ]; then
  echo "❌ Some tests failed!"
  exit 1
else
  echo "✅ All tests passed!"
  exit 0
fi
